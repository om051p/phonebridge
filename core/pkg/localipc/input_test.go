package localipc

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestRPC_SendInput_Success(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	orch := &mockOrchestrator{}
	cfg := Config{
		SocketPath:   sock,
		TokenPath:    tok,
		Token:        tokVal,
		Orchestrator: orch,
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = server.Serve(ctx) }()
	<-server.Ready()

	client, err := Dial(ctx, sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	frame := &phonebridgev1.InputFrame{
		TimestampMs: uint64(time.Now().UnixMilli()),
		Event: &phonebridgev1.InputFrame_Touch{
			Touch: &phonebridgev1.TouchEvent{
				Action:      phonebridgev1.TouchEvent_ACTION_DOWN,
				NormalizedX: 0.5,
				NormalizedY: 0.5,
			},
		},
	}

	req := &phonebridgelocalipcv1.SendInputRequest{
		SessionId: "sess-test-1",
		Frame:     frame,
	}

	resp, err := client.SendInput(ctx, req)
	if err != nil {
		t.Fatalf("SendInput RPC failed: %v", err)
	}
	if !resp.GetDelivered() {
		t.Fatalf("expected delivered = true, got false (err: %s)", resp.GetErrorMessage())
	}

	orch.mu.Lock()
	defer orch.mu.Unlock()
	if orch.lastInputSessionID != "sess-test-1" {
		t.Errorf("expected session_id 'sess-test-1', got '%s'", orch.lastInputSessionID)
	}
	if orch.lastInputFrame == nil {
		t.Fatal("expected frame to be passed to orchestrator")
	}
}

func TestRPC_SendInput_OrchestratorError(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	orch := &mockOrchestrator{
		sendInputErr: errors.New("session not in streaming state"),
	}
	cfg := Config{
		SocketPath:   sock,
		TokenPath:    tok,
		Token:        tokVal,
		Orchestrator: orch,
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = server.Serve(ctx) }()
	<-server.Ready()

	client, err := Dial(ctx, sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	req := &phonebridgelocalipcv1.SendInputRequest{
		SessionId: "sess-test-2",
		Frame: &phonebridgev1.InputFrame{
			Event: &phonebridgev1.InputFrame_Text{
				Text: &phonebridgev1.TextEvent{Text: "test"},
			},
		},
	}

	resp, err := client.SendInput(ctx, req)
	if err != nil {
		t.Fatalf("SendInput RPC failed: %v", err)
	}
	if resp.GetDelivered() {
		t.Fatal("expected delivered = false on orchestrator error")
	}
	if resp.GetErrorMessage() != "session not in streaming state" {
		t.Errorf("unexpected error message: %s", resp.GetErrorMessage())
	}
}

func TestRPC_SendInput_NilFrame_Rejected(t *testing.T) {
	sock, tok, tokVal := testSetup(t)

	orch := &mockOrchestrator{}
	cfg := Config{
		SocketPath:   sock,
		TokenPath:    tok,
		Token:        tokVal,
		Orchestrator: orch,
	}

	server, err := NewServer(cfg)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = server.Serve(ctx) }()
	<-server.Ready()

	client, err := Dial(ctx, sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	req := &phonebridgelocalipcv1.SendInputRequest{
		SessionId: "sess-test-3",
		Frame:     nil,
	}

	_, err = client.SendInput(ctx, req)
	if err == nil {
		t.Fatal("expected error for nil frame")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code, got %v", err)
	}
}
