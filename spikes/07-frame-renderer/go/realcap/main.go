// realcap — Spike 07 gate-3 capture from the REAL POCO F5 (23049PCD8I).
//
// Drives the production engine (discovery → trust check → HTTP signaling →
// Pion receiver) exactly like the daemon does, with one deviation: the sink
// is a FileSink so the live Annex-B stream is recorded for offline analysis
// by the P1 harness (q2/q3/q5 sweep). Read-only toward production code.
//
// Exit codes: 0 ok, 3 session failed, 4 never reached STREAMING.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/engine"
	"github.com/om051p/phonebridge/core/pkg/receiver"
)

func main() {
	out := flag.String("out", "", "output .h264 path (required)")
	duration := flag.Duration("duration", 45*time.Second, "capture time at STREAMING")
	width := flag.Int("width", 720, "requested width")
	height := flag.Int("height", 1600, "requested height")
	fps := flag.Int("fps", 30, "requested fps")
	bitrate := flag.Int("bitrate", 4000, "requested kbps")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}
	_ = os.MkdirAll(filepath.Dir(*out), 0o755)

	hostname, _ := os.Hostname()
	identity, err := crypto.LoadOrGenerateIdentity(crypto.DefaultIdentityPath(), hostname, "linux")
	if err != nil {
		fmt.Printf("identity: %v\n", err)
		os.Exit(2)
	}
	trust, err := crypto.NewTrustStore(crypto.DefaultTrustStorePath())
	if err != nil {
		fmt.Printf("trust store: %v\n", err)
		os.Exit(2)
	}
	fmt.Printf("INFO identity=%s trusted_devices=%d\n", identity.DeviceID[:16], len(trust.List()))

	disc, err := discovery.NewDiscovery(discovery.Config{
		DeviceID:        identity.DeviceID,
		DeviceName:      identity.DisplayName,
		Port:            0,
		IncludeLoopback: true,
		Version:         "1",
		Capabilities:    []string{"SCREEN"},
	})
	if err != nil {
		fmt.Printf("discovery: %v\n", err)
		os.Exit(2)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = disc.Start(ctx) }()
	defer disc.Close()

	// Locate the target: first non-stale device, preferring a trusted one.
	var deviceID string
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) && deviceID == "" {
		for _, d := range disc.Registry().List() {
			if d.IsStale {
				continue
			}
			if e, ok := trust.Get(d.ID); ok && !e.Revoked {
				deviceID = d.ID
				fmt.Printf("INFO target %s (%s) at %v:%d\n", d.Name, d.ID[:16], d.Addresses, d.Port)
				break
			}
		}
		if deviceID == "" {
			time.Sleep(500 * time.Millisecond)
		}
	}
	if deviceID == "" {
		fmt.Println("RESULT realcap outcome=NO_TRUSTED_DEVICE")
		os.Exit(4)
	}

	cfg := engine.DefaultSessionConfig()
	cfg.Identity = identity
	cfg.TrustStore = trust
	cfg.DiscoveryTimeout = 15 * time.Second
	cfg.ConnectTimeout = 15 * time.Second

	var factoryCalls int
	mgr := engine.NewSessionManager(cfg, disc, nil, func(evt engine.SessionEvent) {
		fmt.Printf("EVENT state=%s reason=%q error=%q code=%s\n",
			evt.State, evt.Reason, evt.ErrorMessage, evt.ReasonCode)
	})
	mgr.SetIdentity(identity)
	mgr.SetTrustStore(trust)
	mgr.SetDiscovery(disc)
	mgr.SetSinkFactory(func() (receiver.FrameSink, error) {
		factoryCalls++
		return receiver.NewFileSink(*out, *out+".idx")
	})

	sess, err := mgr.StartSession(ctx, deviceID, engine.MediaParams{
		Width: *width, Height: *height, FPS: *fps, BitrateKbps: *bitrate, Codec: "h264",
	})
	if err != nil {
		fmt.Printf("RESULT realcap outcome=START_FAILED err=%q\n", err)
		os.Exit(3)
	}
	_ = sess

	// Wait for STREAMING (or terminal failure).
	lastState := engine.SessionState(-1)
	streamed := false
	waitDeadline := time.Now().Add(75 * time.Second)
	for time.Now().Before(waitDeadline) {
		snap, err := mgr.GetSessionState("")
		if err == nil {
			if snap.State != lastState {
				fmt.Printf("STATE %s reason=%s error=%q sink=%s actual=%+v known=%v\n",
					snap.State, snap.ReasonCode, snap.ErrorMessage, snap.SinkKind,
					snap.Actual, snap.ActualKnown)
				lastState = snap.State
			}
			if snap.State == engine.StateStreaming {
				streamed = true
				break
			}
			if snap.State == engine.StateFailed || snap.State == engine.StateStopped {
				fmt.Printf("RESULT realcap outcome=TERMINAL state=%s reason=%s error=%q\n",
					snap.State, snap.ReasonCode, snap.ErrorMessage)
				os.Exit(3)
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !streamed {
		fmt.Println("RESULT realcap outcome=NEVER_STREAMED (consent dialog likely waiting for a tap on the phone)")
		os.Exit(4)
	}

	// Capture window.
	time.Sleep(*duration)

	snap, _ := mgr.GetSessionState("")
	fmt.Printf("STATS packets=%d bytes_h264=%d access_units=%d keyframes=%d seq_gaps=%d dup=%d late=%d dropped=%d sink=%s/%v\n",
		snap.Stats.Packets, snap.Stats.BytesH264, snap.Stats.AccessUnits, snap.Stats.Keyframes,
		snap.Stats.SeqGaps, snap.Stats.DupSeq, snap.Stats.LatePackets, snap.DroppedAUs,
		snap.SinkKind, snap.SinkActive)

	if err := mgr.StopSession("", "capture complete"); err != nil {
		fmt.Printf("stop: %v\n", err)
	}
	time.Sleep(500 * time.Millisecond)

	if fi, err := os.Stat(*out); err == nil {
		fmt.Printf("RESULT realcap outcome=OK file=%s bytes=%d factory_calls=%d state=%s actual=%+v\n",
			*out, fi.Size(), factoryCalls, snap.State, snap.Actual)
	} else {
		fmt.Printf("RESULT realcap outcome=FILE_MISSING err=%v factory_calls=%d\n", err, factoryCalls)
		os.Exit(3)
	}
}
