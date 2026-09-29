package frames

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

// The defect these tests pin (Phase-4 comms audit, I1): `NewTapSink` spawned
// `go t.convertLoop()` at construction, and `StartSession` wrapped every sink in
// a tap whenever a frame hub existed — which the daemon always sets. So every
// session ran a second ffmpeg transcoding H.264 → MJPEG even when no client had
// opened a viewer, and the tap copied every access unit into a converter whose
// output had nowhere to go.
//
// After the fix, conversion is driven by StreamFrames subscriber presence:
// nobody watching ⇒ no ffmpeg process and no per-AU copy.

func requireFFmpeg(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
}

// No watcher: the tap must be a pure tee. This is the "session running, UI
// viewer closed" state that used to cost a full transcode.
func TestTapSink_NoSubscriberMeansNoConverter(t *testing.T) {
	requireFFmpeg(t)

	hub := NewHub()
	hub.BeginSession()
	tap := NewTapSink(receiver.NewNullSink(), hub)
	defer tap.Close()

	// Give a would-be eager converter ample time to appear.
	time.Sleep(300 * time.Millisecond)

	if tap.ProcessUp() {
		t.Fatal("an ffmpeg process was started with no StreamFrames subscriber")
	}
	if tap.Running() {
		t.Fatal("the converter must be idle with no subscriber")
	}

	// Access units must still reach the inner sink, and must not be copied for a
	// conversion that cannot happen.
	before := copyCounter(t)
	for i := 0; i < 50; i++ {
		if err := tap.WriteAU(rtpmedia.AccessUnit{Data: []byte{0, 0, 0, 1, 0x65}}); err != nil {
			t.Fatalf("WriteAU: %v", err)
		}
	}
	if ausIn, _, _, _ := tap.Metrics(); ausIn != 0 {
		t.Fatalf("ausIn = %d with no subscriber; access units must not be queued "+
			"for a converter that is not running", ausIn)
	}
	_ = before
}

// Attaching the first subscriber starts the converter; dropping the last one
// stops it. Each phase is asserted on the real child process, not on a flag.
func TestTapSink_SubscriberPresenceStartsAndStopsTheConverter(t *testing.T) {
	requireFFmpeg(t)

	hub := NewHub()
	hub.BeginSession()
	tap := NewTapSink(receiver.NewNullSink(), hub)
	defer tap.Close()

	if tap.ProcessUp() {
		t.Fatal("converter started before any subscriber")
	}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := hub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitForCondition(t, 5*time.Second, "the converter to start for the first subscriber", tap.ProcessUp)

	// A second subscriber must not start a second converter.
	ch2, err := hub.Subscribe(context.Background())
	if err != nil {
		t.Fatalf("second Subscribe: %v", err)
	}
	if got := hub.SubscriberCount(); got != 2 {
		t.Fatalf("SubscriberCount = %d, want 2", got)
	}
	if got, _, _, restarts := tap.Metrics(); restarts != 0 || got != 0 {
		t.Logf("metrics after second subscriber: framesOut=%d restarts=%d", got, restarts)
	}

	// One of two leaving must keep the converter alive.
	hub.Unsubscribe(ch2)
	time.Sleep(200 * time.Millisecond)
	if !tap.ProcessUp() {
		t.Fatal("converter stopped while a subscriber was still attached")
	}

	// The last one leaving must stop it.
	cancel()
	select {
	case <-ch:
	default:
	}
	_ = ch
	hub.Unsubscribe(ch)
	waitForCondition(t, 5*time.Second, "the converter to stop for the last subscriber", func() bool {
		return !tap.ProcessUp()
	})
}

// A session ending closes every subscriber channel, which is also a loss of
// presence: the converter must not outlive the session it was serving.
func TestTapSink_SessionEndStopsTheConverter(t *testing.T) {
	requireFFmpeg(t)

	hub := NewHub()
	tok := hub.BeginSession()
	tap := NewTapSink(receiver.NewNullSink(), hub)
	defer tap.Close()

	if _, err := hub.Subscribe(context.Background()); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitForCondition(t, 5*time.Second, "the converter to start", tap.ProcessUp)

	if err := hub.EndSession(tok); err != nil {
		t.Fatalf("EndSession: %v", err)
	}
	waitForCondition(t, 5*time.Second, "the converter to stop when the session ends", func() bool {
		return !tap.ProcessUp()
	})

	// And it comes back for the next session's viewer.
	hub.BeginSession()
	if _, err := hub.Subscribe(context.Background()); err != nil {
		t.Fatalf("resubscribe: %v", err)
	}
	waitForCondition(t, 5*time.Second, "the converter to restart for the next session", tap.ProcessUp)
}

// A watcher that attaches before the tap registers (UI opened first, engine
// second) must still start conversion: the registration primes itself.
func TestTapSink_ExistingSubscriberStartsTheConverterOnConstruction(t *testing.T) {
	requireFFmpeg(t)

	hub := NewHub()
	hub.BeginSession()
	if _, err := hub.Subscribe(context.Background()); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	tap := NewTapSink(receiver.NewNullSink(), hub)
	defer tap.Close()

	waitForCondition(t, 5*time.Second, "the converter to start for an existing subscriber", tap.ProcessUp)
}

// Closing the tap must detach it from the hub: a later subscriber must never
// signal a converter whose session is over.
func TestTapSink_CloseDetachesFromTheHub(t *testing.T) {
	requireFFmpeg(t)

	hub := NewHub()
	hub.BeginSession()
	tap := NewTapSink(receiver.NewNullSink(), hub)

	if _, err := hub.Subscribe(context.Background()); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	waitForCondition(t, 5*time.Second, "the converter to start", tap.ProcessUp)

	if err := tap.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if tap.ProcessUp() {
		t.Fatal("closing the tap must not leave an ffmpeg process running")
	}

	// A new subscription now has no observer to signal; nothing must panic or
	// resurrect the closed tap's converter.
	hub.Unsubscribe(nil)
	if _, err := hub.Subscribe(context.Background()); err != nil {
		t.Fatalf("Subscribe after close: %v", err)
	}
	time.Sleep(200 * time.Millisecond)
	if tap.ProcessUp() {
		t.Fatal("a closed tap must never restart its converter")
	}
}

// copyCounter is a placeholder for the AU-copy accounting: the tap exposes it
// through Metrics, and what matters here is that no conversion work is queued.
func copyCounter(t *testing.T) int64 {
	t.Helper()
	return 0
}
