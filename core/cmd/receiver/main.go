package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

type MultiSink struct {
	sinks []receiver.FrameSink
}

func (m *MultiSink) WriteAU(au rtpmedia.AccessUnit) error {
	var firstErr error
	for _, s := range m.sinks {
		if err := s.WriteAU(au); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (m *MultiSink) Close() error {
	var firstErr error
	for _, s := range m.sinks {
		if err := s.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

type sdpMessage struct {
	Type string `json:"type"`
	SDP  string `json:"sdp"`
}

func main() {
	httpAddr := flag.String("http", ":7804", "Signaling HTTP listen address")
	enableDisplay := flag.Bool("display", false, "Launch ffplay window for live display")
	h264Path := flag.String("h264", "", "Save received Annex-B H.264 stream to file")
	auIdxPath := flag.String("auidx", "", "Save AU index sidecar to file")
	enableVerify := flag.Bool("verify", false, "Run headless ffmpeg decode validation")
	duration := flag.Int("duration", 0, "Duration in seconds before graceful exit (0 = run forever)")
	windowTitle := flag.String("title", "PhoneBridge Screen Mirror", "Window title for ffplay display")
	_ = flag.String("mode", "serve", "Operation mode (compatibility)")
	_ = flag.String("dump", "", "First AU dump path (compatibility)")
	flag.Parse()

	var sinks []receiver.FrameSink

	if *enableDisplay {
		ds, err := receiver.NewDisplaySink(*windowTitle, true)
		if err != nil {
			log.Fatalf("failed to launch display sink: %v", err)
		}
		sinks = append(sinks, ds)
		log.Printf("[receiver] display sink active (ffplay)")
	}

	if *h264Path != "" {
		fs, err := receiver.NewFileSink(*h264Path, *auIdxPath)
		if err != nil {
			log.Fatalf("failed to create file sink: %v", err)
		}
		sinks = append(sinks, fs)
		log.Printf("[receiver] file sink recording to %s", *h264Path)
	}

	var verifySink *receiver.FFmpegVerifySink
	if *enableVerify {
		vs, err := receiver.NewFFmpegVerifySink()
		if err != nil {
			log.Fatalf("failed to create ffmpeg verify sink: %v", err)
		}
		verifySink = vs
		sinks = append(sinks, vs)
		log.Printf("[receiver] ffmpeg headless verification active")
	}

	var primarySink receiver.FrameSink
	if len(sinks) == 0 {
		primarySink = receiver.NewNullSink()
		log.Printf("[receiver] null sink active (metrics only)")
	} else if len(sinks) == 1 {
		primarySink = sinks[0]
	} else {
		primarySink = &MultiSink{sinks: sinks}
	}

	recv, err := receiver.NewReceiver(receiver.Config{
		Sink:            primarySink,
		IncludeLoopback: true,
		OnStateChange: func(st pion.PeerConnectionState) {
			log.Printf("[receiver] peer connection state: %s", st)
		},
	})
	if err != nil {
		log.Fatalf("failed to initialize receiver: %v", err)
	}
	defer func() {
		_ = recv.Close()
		if verifySink != nil {
			if err := verifySink.Close(); err != nil {
				log.Printf("[receiver] ERROR: ffmpeg verify detected decode error: %v", err)
			} else {
				aus, b := verifySink.Stats()
				log.Printf("[receiver] FFmpeg decode verification: SUCCESS (decoded %d AUs, %d bytes with 0 errors)", aus, b)
			}
		}
	}()

	mux := http.NewServeMux()

	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		st, dropped := recv.Stats()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"packets":      st.Packets,
			"bytes_rtp":    st.BytesRTP,
			"bytes_h264":   st.BytesH264,
			"access_units": st.AccessUnits,
			"keyframes":    st.Keyframes,
			"seq_gaps":     st.SeqGaps,
			"dup_seq":      st.DupSeq,
			"late_packets": st.LatePackets,
			"ts_backward":  st.TSBackward,
			"sps":          st.NALTypeSPS,
			"pps":          st.NALTypePPS,
			"dropped_aus":  dropped,
			"state":        recv.ConnectionState().String(),
		})
	})

	mux.HandleFunc("/offer", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read request body", http.StatusBadRequest)
			return
		}

		var msg sdpMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			// Try bare string
			msg = sdpMessage{Type: "offer", SDP: string(body)}
		}

		log.Printf("[receiver] received SDP offer (%d bytes)", len(msg.SDP))
		answer, err := recv.SetRemoteOffer(pion.SessionDescription{
			Type: pion.SDPTypeOffer,
			SDP:  msg.SDP,
		})
		if err != nil {
			log.Printf("[receiver] SetRemoteOffer error: %v", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		log.Printf("[receiver] generated SDP answer (%d bytes)", len(answer.SDP))
		resp, _ := json.Marshal(sdpMessage{
			Type: answer.Type.String(),
			SDP:  answer.SDP,
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(resp)
	})

	server := &http.Server{
		Addr:    *httpAddr,
		Handler: mux,
	}

	go func() {
		log.Printf("[receiver] signaling server listening on %s", *httpAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[receiver] HTTP server error: %v", err)
		}
	}()

	// Periodic stats reporter
	stopTicker := make(chan struct{})
	go func() {
		t := time.NewTicker(1 * time.Second)
		defer t.Stop()
		sec := 0
		for {
			select {
			case <-t.C:
				sec++
				st, dropped := recv.Stats()
				if st.Packets > 0 {
					log.Printf("[receiver] t=%ds %s dropped=%d", sec, st, dropped)
				}
			case <-stopTicker:
				return
			}
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	if *duration > 0 {
		log.Printf("[receiver] running for %d seconds...", *duration)
		select {
		case <-time.After(time.Duration(*duration) * time.Second):
			log.Printf("[receiver] duration elapsed, shutting down")
		case sig := <-sigChan:
			log.Printf("[receiver] received signal %v, shutting down", sig)
		}
	} else {
		log.Printf("[receiver] running until interrupted (Ctrl+C)...")
		sig := <-sigChan
		log.Printf("[receiver] received signal %v, shutting down", sig)
	}

	close(stopTicker)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)

	st, dropped := recv.Stats()
	fmt.Printf("\n=== FINAL RECEIVER STATS ===\n")
	fmt.Printf("%s\n", st)
	fmt.Printf("Queue Dropped AUs: %d\n", dropped)
	fmt.Printf("============================\n\n")

	statsMap := map[string]any{
		"packets":      st.Packets,
		"bytes_rtp":    st.BytesRTP,
		"bytes_h264":   st.BytesH264,
		"access_units": st.AccessUnits,
		"keyframes":    st.Keyframes,
		"seq_gaps":     st.SeqGaps,
		"dup_seq":      st.DupSeq,
		"late_packets": st.LatePackets,
		"ts_backward":  st.TSBackward,
		"sps":          st.NALTypeSPS,
		"pps":          st.NALTypePPS,
		"dropped_aus":  dropped,
	}
	b, _ := json.Marshal(statsMap)
	fmt.Println("STATS " + string(b))
}
