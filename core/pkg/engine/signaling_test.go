package engine

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/webrtc"
)

func TestSignalingClient_OfferAnswerStop(t *testing.T) {
	offerReceived := false
	answerReceived := false
	stopReceived := false

	mux := http.NewServeMux()
	mux.HandleFunc("/session/offer", func(w http.ResponseWriter, r *http.Request) {
		offerReceived = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sdpPayload{
			Type: "offer",
			SDP:  "v=0\r\no=- 123 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n",
		})
	})
	mux.HandleFunc("/session/answer", func(w http.ResponseWriter, r *http.Request) {
		answerReceived = true
		var payload sdpPayload
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if payload.Type != "answer" {
			http.Error(w, "invalid sdp type", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/session/stop", func(w http.ResponseWriter, r *http.Request) {
		stopReceived = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	srv := httptest.NewServer(mux)
	defer srv.Close()

	endpoint := strings.TrimPrefix(srv.URL, "http://")
	client := NewSignalingClient(2 * time.Second)

	ctx := context.Background()

	// 1. Test RequestOffer
	desc, err := client.RequestOffer(ctx, endpoint)
	if err != nil {
		t.Fatalf("RequestOffer failed: %v", err)
	}
	if !offerReceived || desc.Type != pion.SDPTypeOffer || !strings.Contains(desc.SDP, "v=0") {
		t.Fatalf("unexpected offer: %+v", desc)
	}

	// 2. Test SendAnswer
	answer := pion.SessionDescription{
		Type: pion.SDPTypeAnswer,
		SDP:  "v=0\r\no=- 456 2 IN IP4 127.0.0.1\r\ns=-\r\nt=0 0\r\n",
	}
	if err := client.SendAnswer(ctx, endpoint, answer); err != nil {
		t.Fatalf("SendAnswer failed: %v", err)
	}
	if !answerReceived {
		t.Fatal("expected answerReceived to be true")
	}

	// 3. Test StopSession
	if err := client.StopSession(ctx, endpoint, "test done"); err != nil {
		t.Fatalf("StopSession failed: %v", err)
	}
	if !stopReceived {
		t.Fatal("expected stopReceived to be true")
	}
}

func TestSession_SignalingConnectAndStreamLoopback(t *testing.T) {
	// Create a sender (mimicking Android MediaTransport)
	sender := webrtc.NewSender(nil, webrtc.SenderConfig{
		QueueDepth: 128,
		ShaperKbps: 4000,
	})
	sess, err := webrtc.NewSession(webrtc.SessionConfig{
		IncludeLoopback: true,
	}, sender)
	if err != nil {
		t.Fatalf("create webrtc sender session: %v", err)
	}
	defer sess.Stop()

	// Offer created by sender (Android)
	offer, err := sess.CreateOffer()
	if err != nil {
		t.Fatalf("sender CreateOffer: %v", err)
	}

	// Mock Android HTTP signaling server
	mux := http.NewServeMux()
	mux.HandleFunc("/session/offer", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(sdpPayload{
			Type: offer.Type.String(),
			SDP:  offer.SDP,
		})
	})
	mux.HandleFunc("/session/answer", func(w http.ResponseWriter, r *http.Request) {
		var p sdpPayload
		_ = json.NewDecoder(r.Body).Decode(&p)
		err := sess.SetRemoteAnswer(pion.SessionDescription{
			Type: pion.SDPTypeAnswer,
			SDP:  p.SDP,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_ = sess.Start()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/session/stop", func(w http.ResponseWriter, r *http.Request) {
		sess.Stop()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	httpSrv := httptest.NewServer(mux)
	defer httpSrv.Close()

	endpoint := strings.TrimPrefix(httpSrv.URL, "http://")

	reg := discovery.NewDeviceRegistry(discovery.DefaultRegistryConfig(), nil)
	reg.Upsert(discovery.Device{
		ID:   "test-phone",
		Name: "Test Phone",
	})

	cfg := DefaultSessionConfig()
	cfg.TargetDeviceID = "test-phone"
	cfg.ConnectTimeout = 3 * time.Second

	var transitions []SessionState
	s := NewSession("sess-e2e", cfg, reg, func(oldState, newState SessionState, reason string) {
		transitions = append(transitions, newState)
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Connect to the mock Android device
	if err := s.Connect(ctx, endpoint, receiver.NewNullSink()); err != nil {
		t.Fatalf("Connect failed: %v", err)
	}

	// Wait for WebRTC connection to reach StateConnected
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && s.State() != StateConnected && s.State() != StateStreaming {
		time.Sleep(20 * time.Millisecond)
	}

	st := s.State()
	if st != StateConnected && st != StateStreaming {
		t.Fatalf("expected StateConnected or StateStreaming, got %v (transitions=%v)", st, transitions)
	}

	// Now stop session
	if err := s.Stop("test complete"); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	if s.State() != StateStopped {
		t.Fatalf("expected StateStopped, got %v", s.State())
	}
}
