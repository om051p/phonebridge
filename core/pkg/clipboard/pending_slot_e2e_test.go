package clipboard_test

// Focused tests for the pending-local-clipboard cold-start contract (DEC-023).
//
// The Android side holds a just-read clip in a Kotlin-side slot while the
// session is negotiated; these tests pin the engine-level guarantees that slot
// relies on:
//
//   - a duplicate channel-open (reconnect, re-offer, repeated callback) must
//     never make the PEER apply the same item twice — the first receipt wins
//     and later copies are echoes;
//   - a fresh session with nothing pending must send nothing;
//   - a failed transport send must surface as an error while the item stays
//     current, so the channel-open resync can still deliver it.

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/clipboard"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// TestE2E_DuplicateChannelOpen_PeerAppliesOnce pins TEST C of the cold-start
// contract: the clipboard DataChannel can announce itself more than once
// (reconnect after a stale slot release, ICE restart, repeated callback), and
// each open makes the engine re-send its current item by design. The peer-side
// guarantee is what matters: the re-sent item is an echo there and the platform
// clipboard is written exactly once.
func TestE2E_DuplicateChannelOpen_PeerAppliesOnce(t *testing.T) {
	ctx := context.Background()
	pair := setupLoopbackPair(t, nil, nil)
	defer pair.close()

	payload := []byte("PBSESS-CHANNEL-OPEN-TWICE")

	if _, err := pair.androidEng.OnLocalCopy(ctx, "text/plain;charset=utf-8", payload, uint64(time.Now().UnixMilli())); err != nil {
		t.Fatalf("OnLocalCopy: %v", err)
	}
	if written := pair.linuxPlat.waitForWrite(3 * time.Second); written == nil {
		t.Fatalf("desktop never applied the local item")
	}

	// The channel-open callback fires twice more (reconnect / re-offer).
	if err := pair.androidEng.OnDataChannelOpen(ctx); err != nil {
		t.Fatalf("OnDataChannelOpen #2: %v", err)
	}
	if err := pair.androidEng.OnDataChannelOpen(ctx); err != nil {
		t.Fatalf("OnDataChannelOpen #3: %v", err)
	}

	// Give any duplicate application a chance to land before asserting.
	time.Sleep(300 * time.Millisecond)

	if got := pair.linuxPlat.writeCount(); got != 1 {
		t.Fatalf("desktop applied the item %d times across duplicate channel opens; want exactly 1", got)
	}
	if written := pair.linuxPlat.lastWrite(); written == nil || string(written.Payload) != string(payload) {
		t.Fatalf("desktop holds the wrong item after duplicate channel opens")
	}
}

// TestE2E_FreshSessionNoPendingItem_SendsNothing pins TEST F: a session that
// comes up with no clipboard state on either side must not produce traffic —
// both engines run their channel-open flush against an empty current item.
func TestE2E_FreshSessionNoPendingItem_SendsNothing(t *testing.T) {
	pair := setupLoopbackPair(t, nil, nil)
	defer pair.close()

	// setupLoopbackPair already waited for both clipboard DataChannels to open,
	// which ran each engine's channel-open flush exactly once with no item.
	if got := pair.androidPlat.writeCount(); got != 0 {
		t.Fatalf("android platform received %d writes on a fresh session; want 0", got)
	}
	if got := pair.linuxPlat.writeCount(); got != 0 {
		t.Fatalf("linux platform received %d writes on a fresh session; want 0", got)
	}
}

// TestEngine_SendFailure_ItemStaysCurrent pins the engine half of TEST G: when
// the transport refuses an update, OnLocalCopy must report the failure (so the
// Kotlin slot keeps holding) AND the item must remain the engine's current item
// — that is what lets the channel-open resync deliver it once the transport is
// genuinely up, even if the Kotlin retry hits the identical-digest no-op.
func TestEngine_SendFailure_ItemStaysCurrent(t *testing.T) {
	ctx := context.Background()
	sendFailures := 0
	eng, err := clipboard.NewEngine(clipboard.EngineConfig{
		Role:     clipboard.RoleMobile,
		Platform: newMockPlatform(),
		Transport: clipboard.TransportFunc(func(ctx context.Context, update *phonebridgev1.ClipboardUpdate) error {
			sendFailures++
			return errors.New("datachannel not available")
		}),
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	payload := []byte("PBSESS-SEND-FAILED")
	if _, err := eng.OnLocalCopy(ctx, "text/plain;charset=utf-8", payload, uint64(time.Now().UnixMilli())); err == nil {
		t.Fatalf("OnLocalCopy must surface the transport failure")
	}
	if sendFailures != 1 {
		t.Fatalf("transport was attempted %d times; want exactly 1", sendFailures)
	}
	item := eng.CurrentItem()
	if item == nil || string(item.Payload) != string(payload) {
		t.Fatalf("failed item must stay current so the channel-open resync can deliver it")
	}
}


// TestEngine_PlatformWriteFailureSurfacesError pins the desktop half of the
// session-liveness contract: when the platform adapter refuses a remote update
// (e.g. a failed SET_SELECTION after a Mutter session loss), OnRemoteClipboard
// must return that error so the session layer can log the drop — never report
// success for an update that never reached the desktop clipboard.
func TestEngine_PlatformWriteFailureSurfacesError(t *testing.T) {
	ctx := context.Background()
	eng, err := clipboard.NewEngine(clipboard.EngineConfig{
		Role: clipboard.RoleDesktop,
		Platform: clipboard.PlatformAdapterFunc(func(ctx context.Context, item *clipboard.Item) error {
			return errors.New("clipboard: helper rejected SET_SELECTION: Clipboard not enabled")
		}),
	})
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}

	digest := sha256.Sum256([]byte("PBSESS-DESKTOP-WRITE-FAIL"))
	update := &phonebridgev1.ClipboardUpdate{
		MimeType:     "text/plain;charset=utf-8",
		Payload:      []byte("PBSESS-DESKTOP-WRITE-FAIL"),
		Sha256Digest: digest[:],
		CopiedAtMs:   uint64(time.Now().UnixMilli()),
	}

	if err := eng.OnRemoteClipboard(ctx, update); err == nil {
		t.Fatalf("a failed platform write must surface as an error, not success")
	}
}
