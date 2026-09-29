package frames

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

// Typed frame-pipeline reasons (surfaced as frames_reason on the session
// snapshot; empty string means healthy). Upper-case to match the existing
// SessionReason/code convention over local IPC.
const (
	ReasonFFmpegMissing = "FFMPEG_MISSING" // no ffmpeg binary: tap degraded off
	ReasonFFmpegExited  = "FFMPEG_EXITED"  // converter died repeatedly; gave up
	// ReasonParamSetsMissing is set by the session snapshot (not the tap):
	// IDRs arrived that could never be completed with SPS/PPS, so the stream
	// is undecodable and the tap cannot emit frames.
	ReasonParamSetsMissing = "PARAM_SETS_MISSING"
)

// maxConverterRestarts bounds consecutive ffmpeg restarts before the tap
// declares FFMPEG_EXITED (a successful frame run resets the counter).
const maxConverterRestarts = 5

// TapSink is the FrameTap / tee: it forwards every AccessUnit to the inner
// display/null/file sink first (existing sinks keep working unchanged and
// are never delayed by frame load), then hands a private copy to the
// converter pipeline with a non-blocking latest-wins queue.
//
// Pipeline (one goroutine family per tap):
//
//	WriteAU → auCh (cap 1, drain-then-push) → ffmpeg -threads 1 -q:v 3
//	→ JPEG split (SOI/EOI) → jpegDimensions → Hub.Publish (latest-wins)
//
// Missing ffmpeg degrades exactly like the display path (ReasonFFmpegMissing
// + tap off, inner sink unaffected) — the P4 graceful-degradation chain.
// The converter restarts with backoff on crashes and gives up after
// maxConverterRestarts consecutive failures (ReasonFFmpegExited).
type TapSink struct {
	inner receiver.FrameSink
	hub   *Hub

	auCh     chan []byte
	stop     chan struct{}
	done     chan struct{}
	wake     chan struct{}
	stopOnce sync.Once
	closed   atomic.Bool

	ffmpegPath string
	converting atomic.Bool // true while an ffmpeg process is meant to be running
	// watched mirrors StreamFrames subscriber presence. Conversion only happens
	// while it is true: with no viewer, an ffmpeg run is pure cost (a second
	// decoder per session) whose output is published to nobody.
	watched atomic.Bool
	// processUp is true exactly while an ffmpeg child process is alive. It is
	// separate from converting because "the pipeline wants a converter" and "a
	// decoder is actually resident" are different questions: the first drives the
	// AU copy policy, the second is what costs CPU and memory.
	processUp     atomic.Bool
	clearObserver func() // detaches from the hub on Close

	reasonMu sync.Mutex
	reason   string

	// Metrics (diagnostics/tests).
	auIn      atomic.Int64
	auDropped atomic.Int64
	framesOut atomic.Int64
	restarts  atomic.Int64
}

// NewTapSink wraps inner with the frame tap. hub may be nil (tap becomes a
// pass-through tee with no publisher). Returns a usable sink even when
// ffmpeg is missing — degraded, never failing (P4).
func NewTapSink(inner receiver.FrameSink, hub *Hub) *TapSink {
	if inner == nil {
		inner = receiver.NewNullSink()
	}
	t := &TapSink{
		inner: inner,
		hub:   hub,
		auCh:  make(chan []byte, 1), // exactly one pending AU: latest-wins
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
		wake:  make(chan struct{}, 1),
	}

	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.setReason(ReasonFFmpegMissing)
		close(t.done) // no converter will ever run
		return t
	}
	t.ffmpegPath = path

	// Lazy conversion: the converter exists only while at least one StreamFrames
	// client is watching. Previously it was started here, at construction, so
	// every session ran a full H.264 → MJPEG transcode whose output had no
	// subscriber to reach. A nil hub can never report a watcher, so it stays a
	// pure pass-through tee.
	if hub != nil {
		t.clearObserver = hub.SetSubscriberObserver(t.onSubscriberPresence)
	}
	go t.convertLoop()
	return t
}

// onSubscriberPresence is the hub's watcher-presence callback. It only records
// the level and wakes the converter loop; the loop re-reads the level, so a
// notification can never be lost to a race. It never blocks and never calls back
// into the hub.
func (t *TapSink) onSubscriberPresence(active bool) {
	t.watched.Store(active)
	select {
	case t.wake <- struct{}{}:
	default:
	}
}

// Running reports whether the converter is active and an ffmpeg child process
// is resident. False is the expected state when nobody is watching (and after
// Close): that is the whole point of lazy conversion.
func (t *TapSink) Running() bool { return t.converting.Load() && t.processUp.Load() }

// ProcessUp reports whether an ffmpeg child process is alive right now.
func (t *TapSink) ProcessUp() bool { return t.processUp.Load() }

// WriteAU forwards to the inner sink first, then enqueues a copy for
// conversion. Never blocks; enqueuing is skipped when the tap is closed or
// no converter is running.
func (t *TapSink) WriteAU(au rtpmedia.AccessUnit) error {
	err := t.inner.WriteAU(au)
	if t.closed.Load() || !t.converting.Load() {
		return err
	}
	t.auIn.Add(1)
	b := make([]byte, len(au.Data))
	copy(b, au.Data)
	// Latest-wins: keep exactly one pending AU; a full queue means the
	// converter is behind, so the stale AU is dropped (bounded latency, and
	// prediction-unsafe drops self-heal at the next IDR — P1/P2 policy).
	select {
	case t.auCh <- b:
	default:
		select {
		case <-t.auCh:
		default:
		}
		select {
		case t.auCh <- b:
		default:
			t.auDropped.Add(1)
		}
	}
	return err
}

// Close stops the converter and closes the inner sink. Idempotent.
func (t *TapSink) Close() error {
	t.stopOnce.Do(func() {
		t.closed.Store(true)
		// Detach before stopping: a signalled observer would otherwise poke a tap
		// that is going away, and the hub must not keep a dead tap registered.
		if t.clearObserver != nil {
			t.clearObserver()
		}
		close(t.stop)
	})
	<-t.done
	return t.inner.Close()
}

// Reason returns the typed degradation reason ("" while healthy).
func (t *TapSink) Reason() string {
	t.reasonMu.Lock()
	defer t.reasonMu.Unlock()
	return t.reason
}

func (t *TapSink) setReason(r string) {
	t.reasonMu.Lock()
	t.reason = r
	t.reasonMu.Unlock()
}

// Metrics returns (aus in, frames out, au drops, converter restarts).
func (t *TapSink) Metrics() (ausIn, framesOut, auDrops, restarts int64) {
	return t.auIn.Load(), t.framesOut.Load(), t.auDropped.Load(), t.restarts.Load()
}

// convertLoop owns the ffmpeg process lifecycle: park while nobody is watching,
// run while watched, restart with backoff on crashes, and give up after
// maxConverterRestarts consecutive failures.
func (t *TapSink) convertLoop() {
	defer close(t.done)
	defer t.converting.Store(false)

	consecutiveFailures := 0
	backoff := 200 * time.Millisecond
	for {
		if !t.awaitWatcher() {
			return // closing
		}

		t.converting.Store(true)
		produced, err := t.runOnce()
		t.converting.Store(false)

		if err == nil || errors.Is(err, errStopped) {
			return // stopped
		}
		if errors.Is(err, errIdle) {
			// The last watcher left: park and wait for the next one. Not a
			// failure, so the restart budget is not spent on it.
			consecutiveFailures = 0
			backoff = 200 * time.Millisecond
			continue
		}
		if produced > 0 {
			consecutiveFailures = 0
			backoff = 200 * time.Millisecond
		}
		consecutiveFailures++
		if consecutiveFailures >= maxConverterRestarts {
			t.setReason(ReasonFFmpegExited)
			// Drain pending AUs so WriteAU never accumulates stale copies.
			for {
				select {
				case <-t.auCh:
					continue
				case <-t.stop:
					return
				default:
					return
				}
			}
		}
		select {
		case <-t.stop:
			return
		case <-time.After(backoff):
		}
		if backoff < 2*time.Second {
			backoff *= 2
		}
	}
}

// awaitWatcher parks the converter until someone is watching. It re-reads the
// presence level rather than counting wake-ups, so a notification that arrives
// while the loop is not parked is never lost. Returns false when the tap closes.
func (t *TapSink) awaitWatcher() bool {
	for !t.watched.Load() {
		select {
		case <-t.stop:
			return false
		case <-t.wake:
		}
	}
	return true
}

var (
	errStopped = errors.New("frames: converter stopped")
	// errIdle ends one ffmpeg session because no watcher is left. It is not a
	// failure: the converter parks and starts a fresh process for the next
	// watcher, which is also what makes a stale process impossible to leave behind.
	errIdle = errors.New("frames: converter idle (no stream subscriber)")
)

// ffmpegArgs mirrors the P1-validated tap flags: aggressive probe (fast
// start on a mid-GOP pipe), -threads 1 (auto threads add ~0.5 s residency),
// q3 MJPEG out, and NEVER -fflags nobuffer (silently drops AUs).
func ffmpegArgs() []string {
	return []string{
		"-hide_banner", "-loglevel", "error",
		"-probesize", "32", "-analyzeduration", "0",
		"-threads", "1",
		"-f", "h264", "-i", "pipe:0",
		"-f", "mjpeg", "-q:v", "3",
		"pipe:1",
	}
}

// runOnce executes one converter session: it consumes auCh until the tap is
// stopped or the process fails, publishing every JPEG it emits. Returns the
// number of frames produced (resets the failure backoff) and the exit cause.
func (t *TapSink) runOnce() (int, error) {
	cmd := exec.Command(t.ffmpegPath, ffmpegArgs()...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return 0, fmt.Errorf("frames: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("frames: stdout pipe: %w", err)
	}
	var stderrTail stderrTail
	cmd.Stderr = &stderrTail
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("frames: start ffmpeg: %w", err)
	}
	t.processUp.Store(true)
	defer t.processUp.Store(false)

	// runStop ends THIS ffmpeg session without stopping the tap: the writer
	// goroutine is parked in its select, not blocked on a write, so closing the
	// child's stdin alone would leave it waiting forever and deadlock the
	// teardown that is waiting for writerDone.
	runStop := make(chan struct{})
	kill := func() {
		select {
		case <-runStop:
		default:
			close(runStop)
		}
		_ = cmd.Process.Kill()
		_ = stdin.Close()
		_ = cmd.Wait()
	}

	// Writer: auCh → stdin, interruptible by stop or this run's end.
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		for {
			select {
			case <-t.stop:
				return
			case <-runStop:
				return
			case au, ok := <-t.auCh:
				if !ok {
					return
				}
				if _, err := stdin.Write(au); err != nil {
					return
				}
			}
		}
	}()

	// Reader: stdout → JPEG frames → hub.
	type readResult struct {
		frames int
		err    error
	}
	readDone := make(chan readResult, 1)
	go func() {
		frames, err := t.readJPEGs(stdout, stderrTail.String)
		readDone <- readResult{frames, err}
	}()

	var res readResult
	for {
		select {
		case <-t.stop:
			kill()
			<-writerDone
			<-readDone
			return 0, errStopped
		case res = <-readDone:
			kill()
			<-writerDone
			if !t.closed.Load() && !t.watched.Load() {
				return res.frames, errIdle
			}
			return res.frames, res.err
		case <-t.wake:
			if t.watched.Load() {
				continue // another watcher arrived; keep this process running
			}
			// Last watcher left: end this ffmpeg session rather than leave an
			// idle decoder running for a stream nobody is watching.
			kill()
			<-writerDone
			<-readDone
			return 0, errIdle
		}
	}
}

// readJPEGs splits the MJPEG byte stream into frames and publishes them.
// JPEG byte-stuffing guarantees FFD8/FFD9 appear only as SOI/EOI markers.
func (t *TapSink) readJPEGs(r io.Reader, logf func() string) (int, error) {
	var acc []byte
	buf := make([]byte, 64*1024)
	frames := 0
	for {
		n, err := r.Read(buf)
		if n > 0 {
			acc = append(acc, buf[:n]...)
			var published int
			acc, published = t.drainJPEGs(acc)
			frames += published
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return frames, fmt.Errorf("frames: ffmpeg stdout closed (%s)", logf())
			}
			return frames, fmt.Errorf("frames: ffmpeg read: %w", err)
		}
	}
}

// drainJPEGs publishes every complete JPEG in acc and returns the remainder.
func (t *TapSink) drainJPEGs(acc []byte) ([]byte, int) {
	published := 0
	for {
		soi := bytes.Index(acc, []byte{0xFF, 0xD8})
		if soi < 0 {
			// Keep a one-byte overlap for a split FFD8, bound the residue.
			if len(acc) > 1 {
				acc = acc[len(acc)-1:]
			}
			return acc, published
		}
		if soi > 0 {
			acc = acc[soi:]
		}
		eoi := bytes.Index(acc[2:], []byte{0xFF, 0xD9})
		if eoi < 0 {
			// Incomplete frame; bound the residue so a corrupt stream can
			// never grow the accumulator without limit.
			const maxAcc = 8 << 20
			if len(acc) > maxAcc {
				acc = acc[len(acc)-1:]
			}
			return acc, published
		}
		end := 2 + eoi + 2
		jpeg := make([]byte, end)
		copy(jpeg, acc[:end])
		acc = acc[end:]
		t.publish(jpeg)
		published++
	}
}

func (t *TapSink) publish(jpeg []byte) {
	w, h, ok := jpegDimensions(jpeg)
	if !ok || w == 0 || h == 0 {
		return // malformed frame: drop rather than hand the UI garbage
	}
	t.framesOut.Add(1)
	if t.hub != nil {
		t.hub.Publish(NewFrame(0, jpeg, w, h))
	}
}

// stderrTail retains the last stderr bytes for diagnostics without ever
// blocking the child (a full stderr pipe would deadlock ffmpeg).
type stderrTail struct {
	mu  sync.Mutex
	buf []byte
}

func (s *stderrTail) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	const max = 4096
	s.buf = append(s.buf, p...)
	if len(s.buf) > max {
		s.buf = s.buf[len(s.buf)-max:]
	}
	return len(p), nil
}

// String returns the retained tail (diagnostics on failure).
func (s *stderrTail) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.buf)
}
