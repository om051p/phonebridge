// p2server — Spike 07 / Phase 6 Slice 3 prototype P2 (server side).
//
// Streams synthetic or real-JPEG "frames" to the Flutter bench client through
// the EXACT production local-IPC path: localipc.NewServer → SO_PEERCRED +
// bearer-token interceptors → StreamEvents server stream → gRPC over a Unix
// domain socket. BroadcastEnvelope is the same fan-out the daemon uses for
// session/clipboard/transfer events, so marshal/serialize/flow-control costs
// are production costs.
//
// Payload framing (bench-local, not a protocol proposal): ClipboardUpdate
// payload bytes = [8-byte big-endian send time µs][frame bytes...].
//
// Modes:
//
//	flood — push as fast as the subscriber channel drains (transport ceiling);
//	       server-side drops are counted via the Config.Logf hook.
//	paced — one message per fps tick, for latency/age measurement.
package main

import (
	"context"
	"encoding/binary"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/om051p/phonebridge/core/pkg/localipc"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

func main() {
	socket := flag.String("socket", "", "unix socket path")
	token := flag.String("token", "", "token file path")
	mode := flag.String("mode", "paced", "flood | paced")
	bytesN := flag.Int("bytes", 65536, "synthetic payload bytes (flood mode)")
	jpegDir := flag.String("jpeg", "", "directory of .jpg frames (paced mode)")
	fps := flag.Int("fps", 30, "paced messages per second")
	duration := flag.Duration("duration", 20*time.Second, "broadcast duration")
	delay := flag.Duration("delay", 12*time.Second, "wait after Ready before broadcasting")
	flag.Parse()

	if *socket == "" || *token == "" {
		fmt.Fprintln(os.Stderr, "-socket and -token are required")
		os.Exit(2)
	}

	var drops atomic.Int64
	srv, err := localipc.NewServer(localipc.Config{
		SocketPath:    *socket,
		TokenPath:     *token,
		ServerVersion: "p2-bench",
		Logf: func(format string, args ...any) {
			if strings.Contains(format, "dropping") {
				drops.Add(1)
			}
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "new server: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		if err := srv.Serve(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "serve: %v\n", err)
			os.Exit(1)
		}
	}()
	<-srv.Ready()
	fmt.Printf("RESULT p2server ready socket=%s mode=%s delay=%s\n", *socket, *mode, *delay)

	time.Sleep(*delay)
	if ctx.Err() != nil {
		return
	}

	// Frame corpus for paced mode: cycle real JPEGs (P1-quality).
	var frames [][]byte
	if *jpegDir != "" {
		ents, err := filepath.Glob(filepath.Join(*jpegDir, "*.jpg"))
		if err != nil || len(ents) == 0 {
			fmt.Fprintf(os.Stderr, "no .jpg files in %s\n", *jpegDir)
			os.Exit(1)
		}
		sort.Strings(ents)
		for _, p := range ents {
			b, err := os.ReadFile(p)
			if err != nil {
				fmt.Fprintf(os.Stderr, "read %s: %v\n", p, err)
				os.Exit(1)
			}
			frames = append(frames, b)
		}
	}

	mkEnvelope := func(payload []byte) *phonebridgev1.Envelope {
		body := make([]byte, 8+len(payload))
		binary.BigEndian.PutUint64(body[:8], uint64(time.Now().UnixMicro()))
		copy(body[8:], payload)
		return &phonebridgev1.Envelope{
			Version: 1,
			Payload: &phonebridgev1.Envelope_ClipboardUpdate{
				ClipboardUpdate: &phonebridgev1.ClipboardUpdate{
					MimeType: "application/octet-stream",
					Payload:  body,
				},
			},
		}
	}

	t0 := time.Now()
	var sent int
	var sentBytes int64

	switch *mode {
	case "flood":
		synth := make([]byte, *bytesN)
		for i := range synth {
			synth[i] = byte(i * 7)
		}
		env := mkEnvelope(synth) // timestamp fixed at start; flood measures throughput, not age
		for time.Since(t0) < *duration && ctx.Err() == nil {
			srv.BroadcastEnvelope(env)
			sent++
			sentBytes += int64(8 + *bytesN)
		}
	case "paced":
		if len(frames) == 0 {
			fmt.Fprintln(os.Stderr, "paced mode requires -jpeg")
			os.Exit(2)
		}
		tick := time.NewTicker(time.Second / time.Duration(*fps))
		defer tick.Stop()
		i := 0
		for time.Since(t0) < *duration && ctx.Err() == nil {
			<-tick.C
			env := mkEnvelope(frames[i%len(frames)])
			srv.BroadcastEnvelope(env)
			sent++
			sentBytes += int64(len(frames[i%len(frames)]) + 8)
			i++
		}
	default:
		fmt.Fprintf(os.Stderr, "unknown mode %s\n", *mode)
		os.Exit(2)
	}

	// Give the subscriber channel a moment to drain before shutdown.
	time.Sleep(500 * time.Millisecond)
	elapsed := time.Since(t0).Seconds()
	fmt.Printf("RESULT p2server mode=%s sent=%d sent_bytes=%d elapsed_s=%.2f enqueue_rate_per_s=%.1f subscriber_drops=%d\n",
		*mode, sent, sentBytes, elapsed, float64(sent)/elapsed, drops.Load())
	cancel()
	time.Sleep(300 * time.Millisecond)
}
