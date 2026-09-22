package transfer

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

func TestConfigDefaultsAndValidation(t *testing.T) {
	cfg := Config{}.withDefaults()
	if cfg.ChunkSize != DefaultChunkSize {
		t.Fatalf("chunk size = %d, want %d", cfg.ChunkSize, DefaultChunkSize)
	}
	if cfg.MaxFileSize != DefaultMaxFileSize {
		t.Fatalf("max file size = %d, want %d", cfg.MaxFileSize, DefaultMaxFileSize)
	}
	if cfg.LowWatermark >= cfg.HighWatermark {
		t.Fatalf("low watermark %d must be below high watermark %d", cfg.LowWatermark, cfg.HighWatermark)
	}
	if cfg.ProgressInterval != DefaultProgressInterval {
		t.Fatalf("progress interval = %v", cfg.ProgressInterval)
	}

	// A low-watermark at or above the ceiling would never signal a drain.
	bad := Config{HighWatermark: 1024, LowWatermark: 4096}.withDefaults()
	if bad.LowWatermark >= bad.HighWatermark {
		t.Fatalf("low watermark was not repaired: %d >= %d", bad.LowWatermark, bad.HighWatermark)
	}

	if _, err := NewEngine(Config{ResultFloor: time.Minute, ResultCap: time.Second}); err == nil {
		t.Fatalf("a result floor above the cap must be rejected")
	}
	if _, err := NewEngine(Config{ChunkSize: 10 * MaxReceiveChunkSize}); err != nil {
		t.Fatalf("chunk size larger than the receive cap should clamp, not fail: %v", err)
	}
}

func TestResultTimeoutScalesWithSize(t *testing.T) {
	cfg := Config{}.withDefaults()
	small := cfg.resultTimeout(1024)
	if small != DefaultResultFloorTimeout {
		t.Fatalf("small-file timeout = %v, want the floor %v", small, DefaultResultFloorTimeout)
	}
	// 4 GiB at 100 MB/s needs ~86 s, which must exceed the 60 s floor.
	large := cfg.resultTimeout(4 << 30)
	if large <= small {
		t.Fatalf("large-file timeout %v should exceed the floor %v", large, small)
	}
	if huge := cfg.resultTimeout(1 << 40); huge != DefaultResultCapTimeout {
		t.Fatalf("huge-file timeout = %v, want the cap %v", huge, DefaultResultCapTimeout)
	}
}

func TestFailureCodeReasonMapping(t *testing.T) {
	reasons := []Reason{
		ReasonNone, ReasonNoSession, ReasonUnsupportedPeer, ReasonBusy, ReasonUnsafeFilename,
		ReasonTooLarge, ReasonChecksumMismatch, ReasonStorageFailed, ReasonInterrupted,
		ReasonCancelledByUser, ReasonProtocolError, ReasonIncompatibleVersion,
	}
	for _, r := range reasons {
		code := CodeForReason(r)
		if code == phonebridgev1.Code_CODE_UNSPECIFIED {
			t.Fatalf("reason %s maps to an unspecified code", r)
		}
		back := ReasonForCode(code)
		if back == ReasonUnspecified {
			t.Fatalf("code %s does not map back to a reason", code)
		}
	}
	// The two cancel reasons deliberately share one wire code.
	if CodeForReason(ReasonCancelledByUser) != CodeForReason(ReasonCancelledByPeer) {
		t.Fatalf("cancel reasons should share a wire code")
	}
	if ReasonForCode(phonebridgev1.Code_CODE_TRANSFER_CANCELLED) != ReasonCancelledByPeer {
		t.Fatalf("a peer cancel must classify as CANCELLED_BY_PEER")
	}
	if ReasonForCode(phonebridgev1.Code_CODE_OK) != ReasonNone {
		t.Fatalf("CODE_OK should classify as NONE")
	}
}

func TestEngineRequiresChannelBeforeSending(t *testing.T) {
	eng, err := NewEngine(Config{})
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	defer eng.Close()

	path := filepath.Join(t.TempDir(), "file.bin")
	if err := os.WriteFile(path, []byte("data"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := eng.SendFile(context.Background(), path, ""); err == nil {
		t.Fatalf("SendFile without a transfer channel must fail")
	} else if f, ok := IsFailure(err); !ok || f.Reason != ReasonNoSession {
		t.Fatalf("error = %v, want a NO_SESSION failure", err)
	}
	if err := eng.Cancel(context.Background(), "unknown"); err == nil {
		t.Fatalf("Cancel of an unknown transfer must fail")
	}
	if eng.ChannelReady() {
		t.Fatalf("ChannelReady must be false without an attached channel")
	}
}

func TestSendFileInputValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.bin")
	if err := os.WriteFile(path, make([]byte, 2048), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	eng, err := NewEngine(Config{MaxFileSize: 512})
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	defer eng.Close()
	ch := newMemChannel(0)
	eng.AttachChannel(ch)

	cases := []struct {
		name string
		path string
		want Reason
	}{
		{"missing", filepath.Join(dir, "nope.bin"), ReasonStorageFailed},
		{"directory", dir, ReasonStorageFailed},
		{"too large", path, ReasonTooLarge},
	}
	for _, tc := range cases {
		_, err := eng.SendFile(context.Background(), tc.path, "")
		if err == nil {
			t.Fatalf("%s: expected a failure", tc.name)
		}
		f, ok := IsFailure(err)
		if !ok {
			t.Fatalf("%s: error is not a typed Failure: %v", tc.name, err)
		}
		if f.Reason != tc.want {
			t.Fatalf("%s: reason = %s, want %s", tc.name, f.Reason, tc.want)
		}
	}

	// An unsafe display name is refused even though the file itself is fine.
	small := Config{MaxFileSize: 1 << 20}
	eng2, err := NewEngine(small)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	defer eng2.Close()
	eng2.AttachChannel(newMemChannel(0))
	if _, err := eng2.SendFile(context.Background(), path, "../escape.bin"); err == nil {
		t.Fatalf("unsafe filename must be refused")
	} else if f, ok := IsFailure(err); !ok || f.Reason != ReasonUnsafeFilename {
		t.Fatalf("unsafe filename error = %v", err)
	}

	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "link.bin")
		if err := os.Symlink(path, link); err != nil {
			t.Skipf("cannot create symlink: %v", err)
		}
		if _, err := eng2.SendFile(context.Background(), link, ""); err == nil {
			t.Fatalf("a symlink source must be refused (DEC-024)")
		}
	}
}

func TestAttachChannelReplacementInterrupts(t *testing.T) {
	p := newPair(t, pairOptions{})

	path, _ := writeSource(t, p.srcDir, "replaced.bin", 1<<20)
	p.senderCh.PauseAfter(1)
	id, err := p.senderEng.SendFile(context.Background(), path, "")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	waitForState(t, p.senderEng, id, StateActive, 5*time.Second)

	// Reconnect: a new channel replaces the old one, so the in-flight transfer
	// is interrupted rather than continued on the new transport.
	p.senderEng.AttachChannel(newMemChannel(0))
	info := waitForState(t, p.senderEng, id, StateFailed, 5*time.Second)
	if info.ReasonCode != ReasonInterrupted {
		t.Fatalf("reason = %s, want INTERRUPTED", info.ReasonCode)
	}
	if p.senderEng.ChannelReady() != true {
		t.Fatalf("the replacement channel should be ready")
	}
}

func TestHistoryIsBoundedAndNewestFirst(t *testing.T) {
	p := newPair(t, pairOptions{tune: func(cfg *Config) { cfg.HistoryLimit = 2 }})

	var ids []string
	for i, name := range []string{"one.bin", "two.bin", "three.bin"} {
		path, _ := writeSource(t, p.srcDir, name, 4096+i)
		id, err := p.senderEng.SendFile(context.Background(), path, "")
		if err != nil {
			t.Fatalf("%s: send: %v", name, err)
		}
		ids = append(ids, id)
		waitForState(t, p.senderEng, id, StateComplete, 20*time.Second)
	}

	listed := p.senderEng.List()
	if len(listed) != 2 {
		t.Fatalf("history length = %d, want 2", len(listed))
	}
	if listed[0].TransferID != ids[2] || listed[1].TransferID != ids[1] {
		t.Fatalf("history is not newest-first: %+v", listed)
	}
	if _, ok := p.senderEng.Get(ids[0]); ok {
		t.Fatalf("the oldest transfer should have been evicted")
	}
	if info, ok := p.senderEng.Get(ids[2]); !ok || info.State != StateComplete {
		t.Fatalf("Get on a remembered transfer failed: %+v ok=%v", info, ok)
	}
}

func TestCloseInterruptsEveryActiveTransfer(t *testing.T) {
	p := newPair(t, pairOptions{})

	path, _ := writeSource(t, p.srcDir, "closing.bin", 1<<20)
	p.senderCh.PauseAfter(1)
	id, err := p.senderEng.SendFile(context.Background(), path, "")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	waitForState(t, p.senderEng, id, StateActive, 5*time.Second)
	waitForAnyState(t, p.receiverEng, DirectionInbound, StateActive, 5*time.Second)

	p.senderEng.Close()
	p.receiverEng.Close()

	sent := waitForState(t, p.senderEng, id, StateFailed, 5*time.Second)
	if sent.ReasonCode != ReasonInterrupted {
		t.Fatalf("sender reason = %s, want INTERRUPTED", sent.ReasonCode)
	}
	recv := waitForState(t, p.receiverEng, id, StateFailed, 5*time.Second)
	if recv.ReasonCode != ReasonInterrupted {
		t.Fatalf("receiver reason = %s, want INTERRUPTED", recv.ReasonCode)
	}
}

func TestFailureErrorFormatting(t *testing.T) {
	f := newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "disk full")
	if f.Error() == "" || f.Code != phonebridgev1.Code_CODE_STORAGE_FAILED {
		t.Fatalf("unexpected failure rendering: %v", f)
	}
	wrapped := &Failure{Code: phonebridgev1.Code_CODE_INTERNAL, Reason: ReasonUnspecified, Err: errors.New("boom")}
	if _, ok := IsFailure(wrapped); !ok {
		t.Fatalf("IsFailure must recognise a Failure")
	}
	if errors.Unwrap(wrapped) == nil {
		t.Fatalf("Failure must unwrap its cause")
	}
	var nilFailure *Failure
	if nilFailure.Error() == "" {
		t.Fatalf("nil Failure must render something")
	}
}
