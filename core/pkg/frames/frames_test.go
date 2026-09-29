package frames

// Phase 6 Slice 3A: the frame pipeline's own contract — chunking at the
// 64 KiB ceiling, the hub's session lifecycle (no stale frame after the
// session ends), latest-wins backpressure, and the P4 degradation chain
// (missing ffmpeg degrades the tap, never the sink chain).

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

func TestFrameChunksRespect64KiBCeiling(t *testing.T) {
	jpeg := bytes.Repeat([]byte{0x11, 0x22}, 100_000) // 200,000 bytes
	f := NewFrame(0, jpeg, 720, 1600)

	chunks := f.Chunks()
	want := (len(jpeg) + MaxChunkBytes - 1) / MaxChunkBytes
	if len(chunks) != want {
		t.Fatalf("chunks = %d, want %d", len(chunks), want)
	}
	var joined []byte
	for i, c := range chunks {
		if len(c) > MaxChunkBytes {
			t.Errorf("chunk %d is %d bytes, exceeds 64 KiB", i, len(c))
		}
		joined = append(joined, c...)
	}
	if !bytes.Equal(joined, jpeg) {
		t.Errorf("reassembled %d bytes, want byte-identical %d", len(joined), len(jpeg))
	}

	// A zero-length JPEG still frames the chunk contract.
	if empty := NewFrame(0, nil, 0, 0); len(empty.Chunks()) != 1 {
		t.Errorf("empty frame chunks = %d, want 1", len(empty.Chunks()))
	}
}

func TestHubSessionLifecycleHasNoStaleFrames(t *testing.T) {
	h := NewHub()

	// Subscribing before a session must block, not deliver.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	if _, err := h.Subscribe(ctx); err == nil {
		t.Fatal("Subscribe before BeginSession returned a channel")
	}
	cancel()

	tok := h.BeginSession()

	ch, err := h.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	h.Publish(NewFrame(0, []byte("a"), 1, 1))
	h.Publish(NewFrame(0, []byte("b"), 1, 1))

	first := <-ch
	if first.ID != 1 {
		t.Errorf("first frame id = %d, want 1", first.ID)
	}
	second := <-ch
	if second.ID != 2 {
		t.Errorf("second frame id = %d, want 2", second.ID)
	}

	// Session ends: the channel closes (no stale frame can be received),
	// and publishes afterwards are dropped.
	if err := h.EndSession(tok); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	if err := h.EndSession(tok); err != nil {
		t.Fatalf("EndSession (idempotent): %v", err)
	}
	for range ch {
		t.Fatal("received a frame after EndSession")
	}
	published, dropped := h.Stats()
	if published != 2 || dropped != 0 {
		t.Errorf("stats = (%d, %d), want (2, 0)", published, dropped)
	}
	h.Publish(NewFrame(0, []byte("stale"), 1, 1))
	if h.Active() {
		t.Error("hub still active after EndSession")
	}

	// The next session opens a fresh id space and releases new waiters.
	ctx2, cancel2 := context.WithTimeout(context.Background(), 100*time.Millisecond)
	if _, err := h.Subscribe(ctx2); err == nil {
		t.Fatal("Subscribe between sessions returned a channel")
	}
	cancel2()

	h.BeginSession()
	ch2, err := h.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("second-session Subscribe: %v", err)
	}
	// The ended session's token is inert: a late EndSession carrying it must
	// not close the live session's subscription (cross-session stall guard).
	if err := h.EndSession(tok); !errors.Is(err, ErrSessionNotCurrent) {
		t.Errorf("stale EndSession error = %v, want ErrSessionNotCurrent", err)
	}
	if !h.Active() {
		t.Error("stale EndSession closed the live session's window")
	}
	h.Publish(NewFrame(0, []byte("fresh"), 1, 1))
	fresh := <-ch2
	if fresh.ID != 1 {
		t.Errorf("second-session first id = %d, want 1 (rebased)", fresh.ID)
	}
	if string(fresh.JPEG) != "fresh" {
		t.Errorf("second-session payload = %q", fresh.JPEG)
	}
}

// TestHubSupersededSessionCannotEndItsSuccessor is the Slice 3B cross-session
// stall regression, expressed as an ordering: session 1's terminal callback
// lands AFTER session 2 has begun. That is reachable in production because a
// session reports terminal states from a callback that runs after the
// transition is applied and its lock released, and the manager accepts session
// 2 precisely because session 1's state is already terminal.
//
// Before the token fix this permanently closed session 2's frame window: the
// daemon kept reporting STREAMING with AUs flowing, but no JPEG could reach a
// subscriber until another BeginSession — i.e. until the daemon restarted.
func TestHubSupersededSessionCannotEndItsSuccessor(t *testing.T) {
	h := NewHub()
	tok1 := h.BeginSession()
	ch1, err := h.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("session 1 Subscribe: %v", err)
	}
	h.Publish(NewFrame(0, []byte("s1"), 1, 1))
	if f := <-ch1; string(f.JPEG) != "s1" {
		t.Fatalf("session 1 payload = %q", f.JPEG)
	}

	// Session 2 begins while session 1 is still open (session 1's teardown
	// callback has not run yet). It must get its OWN token and its own window.
	tok2 := h.BeginSession()
	if tok2 == tok1 {
		t.Fatalf("session 2 reused session 1's token %d", tok2)
	}
	if !h.Active() {
		t.Fatal("hub inactive right after session 2 began")
	}

	// Session 1's subscribers are closed by the supersede, so no stale frame
	// outlives its session.
	select {
	case f, ok := <-ch1:
		if ok {
			t.Errorf("session 1 channel delivered %q after supersede", f.JPEG)
		}
	case <-time.After(time.Second):
		t.Error("session 1 channel was not closed by the supersede")
	}

	ch2, err := h.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("session 2 Subscribe: %v", err)
	}

	// The late terminal callback for session 1.
	if err := h.EndSession(tok1); !errors.Is(err, ErrSessionNotCurrent) {
		t.Errorf("stale EndSession(1) error = %v, want ErrSessionNotCurrent", err)
	}
	if !h.Active() {
		t.Fatal("stale EndSession(1) closed session 2's window (cross-session stall)")
	}
	select {
	case <-ch2:
		t.Fatal("stale EndSession(1) closed session 2's subscription")
	default:
	}

	// Proof the window really is alive: a frame published now still reaches
	// session 2's subscriber, with a fresh id space.
	h.Publish(NewFrame(0, []byte("s2"), 1, 1))
	select {
	case f := <-ch2:
		if f.ID != 1 {
			t.Errorf("session 2 frame id = %d, want 1 (rebased)", f.ID)
		}
		if string(f.JPEG) != "s2" {
			t.Errorf("session 2 payload = %q", f.JPEG)
		}
	case <-time.After(time.Second):
		t.Fatal("session 2 received no frame: its window is dead")
	}

	// And session 2's own teardown still works.
	if err := h.EndSession(tok2); err != nil {
		t.Errorf("EndSession(2) = %v, want nil", err)
	}
	if h.Active() {
		t.Error("hub still active after session 2's own EndSession")
	}
}

// TestHubEndSessionIfCurrentRejectsStaleToken pins the atomic ownership check
// the manager relies on: it must refuse a token that is no longer current, so
// a caller validating against external state cannot close a successor's window
// in the gap between its check and the call.
func TestHubEndSessionIfCurrentRejectsStaleToken(t *testing.T) {
	h := NewHub()
	tok1 := h.BeginSession()

	if err := h.EndSessionIfCurrent(0); !errors.Is(err, ErrSessionNotCurrent) {
		t.Errorf("EndSessionIfCurrent(0) = %v, want ErrSessionNotCurrent", err)
	}
	if !h.Active() {
		t.Error("EndSessionIfCurrent(0) closed the live session")
	}

	tok2 := h.BeginSession() // supersedes tok1
	if err := h.EndSessionIfCurrent(tok1); !errors.Is(err, ErrSessionNotCurrent) {
		t.Errorf("EndSessionIfCurrent(stale) = %v, want ErrSessionNotCurrent", err)
	}
	if !h.Active() {
		t.Error("EndSessionIfCurrent(stale) closed the live session")
	}

	if err := h.EndSessionIfCurrent(tok2); err != nil {
		t.Errorf("EndSessionIfCurrent(current) = %v, want nil", err)
	}
	if h.Active() {
		t.Error("EndSessionIfCurrent(current) left the session open")
	}
}

func TestHubLatestWinsDropsOldest(t *testing.T) {
	h := NewHub()
	h.BeginSession()
	ch, err := h.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// The subscriber reads nothing: five publishes into a cap-2 buffer must
	// keep the two newest and count the drops (bounded memory, visible gaps).
	for i := 0; i < 5; i++ {
		h.Publish(NewFrame(0, []byte{byte(i)}, 1, 1))
	}

	got := []uint64{(<-ch).ID, (<-ch).ID}
	if got[0] != 4 || got[1] != 5 {
		t.Errorf("delivered ids = %v, want [4 5] (newest two)", got)
	}
	_, dropped := h.Stats()
	if dropped != 3 {
		t.Errorf("dropped = %d, want 3", dropped)
	}
}

func TestTapSinkDegradesWhenFFmpegMissing(t *testing.T) {
	t.Setenv("PATH", "") // exec.LookPath fails: the P4 scenario

	inner := receiver.NewNullSink()
	tap := NewTapSink(inner, NewHub())
	defer tap.Close()

	if tap.Reason() != ReasonFFmpegMissing {
		t.Fatalf("reason = %q, want %q", tap.Reason(), ReasonFFmpegMissing)
	}

	// The tap still works as a tee: the inner sink is unaffected.
	err := tap.WriteAU(rtpmedia.AccessUnit{Data: []byte{0, 0, 0, 1, 0x65}})
	if err != nil {
		t.Errorf("WriteAU: %v", err)
	}
	if err := tap.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if tap.Reason() != ReasonFFmpegMissing {
		t.Errorf("reason after close = %q", tap.Reason())
	}
}

// TestTapSinkConvertsAnnexBToJPEG exercises the production tee → ffmpeg →
// JPEG path end to end on a synthetic stream (the real-device path is
// validated separately). Skips when ffmpeg is absent (P4: optional dep).
func TestTapSinkConvertsAnnexBToJPEG(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}

	annexB, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=128x96:rate=10:duration=1",
		"-c:v", "libx264", "-pix_fmt", "yuv420p", "-f", "h264", "pipe:1").Output()
	if err != nil {
		t.Fatalf("generate h264: %v", err)
	}

	hub := NewHub()
	hub.BeginSession()
	inner := receiver.NewNullSink()
	tap := NewTapSink(inner, hub)
	defer tap.Close()

	if r := tap.Reason(); r != "" {
		t.Fatalf("tap degraded unexpectedly: %q", r)
	}

	// Subscribe before feeding so no published frame can be missed (the hub
	// only fans out to registered subscribers, latest-wins per buffer). This is
	// also what starts the converter: conversion is lazy and only happens while
	// a StreamFrames subscriber exists.
	ch, err := hub.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	// Wait for the ffmpeg process to be up before feeding: a converter started
	// mid-stream can only resume at the next keyframe, and this synthetic stream
	// has exactly one.
	waitForCondition(t, 5*time.Second, "the converter process to start", tap.ProcessUp)

	// Feed the stream as AUs split on Annex-B start codes.
	positions := startCodePositions(annexB)
	if len(positions) < 5 {
		t.Fatalf("generated stream has only %d AUs", len(positions))
	}
	for i, pos := range positions {
		end := len(annexB)
		if i+1 < len(positions) {
			end = positions[i+1]
		}
		if err := tap.WriteAU(rtpmedia.AccessUnit{Data: annexB[pos:end]}); err != nil {
			t.Errorf("WriteAU %d: %v", i, err)
		}
		time.Sleep(15 * time.Millisecond) // let the converter drain (latest-wins)
	}

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, framesOut, _, _ := tap.Metrics(); framesOut > 0 {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	_, framesOut, _, restarts := tap.Metrics()
	if framesOut == 0 {
		ausIn, out, drops, rs := tap.Metrics()
		t.Fatalf("no JPEG frames produced (ausIn=%d framesOut=%d drops=%d restarts=%d)",
			ausIn, out, drops, rs)
	}
	if restarts != 0 {
		t.Errorf("converter restarted %d times on a healthy stream", restarts)
	}
	if tap.Reason() != "" {
		t.Errorf("reason = %q, want healthy", tap.Reason())
	}

	// A published frame must be a self-describing JPEG with real dimensions.
	select {
	case f := <-ch:
		w, hgt, ok := jpegDimensions(f.JPEG)
		if !ok || w != 128 || hgt != 96 {
			t.Errorf("jpeg dims = (%d, %d, ok=%v), want 128x96", w, hgt, ok)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no frame published to the hub")
	}
}

// waitForCondition polls a predicate. Used where the alternative is a
// timing-dependent sleep around process startup.
func waitForCondition(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func startCodePositions(b []byte) []int {
	var out []int
	for i := 0; i+4 <= len(b); i++ {
		if b[i] == 0 && b[i+1] == 0 && b[i+2] == 0 && b[i+3] == 1 {
			out = append(out, i)
			i += 3
		}
	}
	return out
}
