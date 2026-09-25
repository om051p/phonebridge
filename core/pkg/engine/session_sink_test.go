package engine

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/om051p/phonebridge/core/pkg/receiver"
	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

// unclassifiedSink is a FrameSink of a type the classifier does not know: it
// must report SinkKindUnspecified rather than guessing a kind.
type unclassifiedSink struct{}

func (unclassifiedSink) WriteAU(rtpmedia.AccessUnit) error { return nil }
func (unclassifiedSink) Close() error                      { return nil }

// The sink classification must be visible in a snapshot while the session is
// live, and must NOT survive a terminal transition: a snapshot reporting
// ffplay after Stop would be the exact stale-state bug this field exists to
// prevent.
func TestSession_SinkKindReportedWhileActiveAndClearedOnStop(t *testing.T) {
	h := newNegotiationHarness(t, accepted, nil)
	h.sess.SetSinkKind(SinkKindDisplay)

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}

	snap := h.sess.Snapshot()
	if snap.SinkKind != SinkKindDisplay {
		t.Errorf("snapshot sink kind = %s, want DISPLAY", snap.SinkKind)
	}
	if !snap.SinkActive {
		t.Error("snapshot sink active = false while connected, want true")
	}

	if err := h.sess.Stop("test complete"); err != nil {
		t.Fatalf("stop: %v", err)
	}

	snap = h.sess.Snapshot()
	if snap.SinkKind != SinkKindUnspecified {
		t.Errorf("snapshot sink kind after stop = %s, want UNSPECIFIED", snap.SinkKind)
	}
	if snap.SinkActive {
		t.Error("snapshot sink active = true after stop, want false")
	}
}

// Failure is terminal too: the classification recorded before connect must be
// cleared when the connect path fails, never reported by a later snapshot.
func TestSession_SinkKindClearedOnFailure(t *testing.T) {
	h := newNegotiationHarness(t, accepted, nil)
	h.sess.SetSinkKind(SinkKindDisplay)

	if err := h.connect(t); err != nil {
		t.Fatalf("connect: %v", err)
	}

	h.sess.Fail(ReasonTransportFailed, errors.New("forced test failure"))

	snap := h.sess.Snapshot()
	if snap.State != StateFailed {
		t.Fatalf("snapshot state = %s, want FAILED", snap.State)
	}
	if snap.SinkKind != SinkKindUnspecified {
		t.Errorf("snapshot sink kind after failure = %s, want UNSPECIFIED", snap.SinkKind)
	}
	if snap.SinkActive {
		t.Error("snapshot sink active = true after failure, want false")
	}
}

// classifySinkKind maps the concrete sinks the manager can choose; the display
// sink is classified by the manager branch that launches it (it is a
// *PipeSink by construction), which is exercised via StartSession instead.
func TestClassifySinkKind(t *testing.T) {
	fileSink, err := receiver.NewFileSink(
		filepath.Join(t.TempDir(), "cap.h264"),
		filepath.Join(t.TempDir(), "cap.idx"),
	)
	if err != nil {
		t.Fatalf("NewFileSink: %v", err)
	}
	t.Cleanup(func() { _ = fileSink.Close() })

	pipeSink, err := receiver.NewPipeSink("cat")
	if err != nil {
		t.Skipf("cannot launch pipe subprocess in this environment: %v", err)
	}
	t.Cleanup(func() { _ = pipeSink.Close() })

	cases := []struct {
		name string
		sink receiver.FrameSink
		want SinkKind
	}{
		{"null sink", receiver.NewNullSink(), SinkKindNull},
		{"file sink", fileSink, SinkKindFile},
		{"pipe sink", pipeSink, SinkKindPipe},
		{"unknown type", unclassifiedSink{}, SinkKindUnspecified},
		{"nil", nil, SinkKindUnspecified},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifySinkKind(tc.sink); got != tc.want {
				t.Errorf("classifySinkKind(%T) = %s, want %s", tc.sink, got, tc.want)
			}
		})
	}
}
