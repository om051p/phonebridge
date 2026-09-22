package transfer

import (
	"bytes"
	"crypto/sha256"
	"os"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// rawReceiver drives the inbound half of the state machine directly: the test
// crafts wire frames, so a protocol violation can be produced exactly instead of
// hoping the peer implementation misbehaves.
type rawReceiver struct {
	eng     *Engine
	ch      *memChannel
	dest    *FileDestination
	destDir string
}

func newRawReceiver(t *testing.T, tune func(*Config)) *rawReceiver {
	t.Helper()
	destDir := t.TempDir()
	dest, err := NewFileDestination(FileDestinationConfig{Dir: destDir})
	if err != nil {
		t.Fatalf("destination: %v", err)
	}
	cfg := Config{
		ChunkSize:        testChunkSize,
		HighWatermark:    testHighWater,
		LowWatermark:     testLowWater,
		OfferTimeout:     2 * time.Second,
		StallTimeout:     2 * time.Second,
		ResultFloor:      2 * time.Second,
		ProgressInterval: 2 * time.Millisecond,
		Destination:      dest,
	}
	if tune != nil {
		tune(&cfg)
	}
	eng, err := NewEngine(cfg)
	if err != nil {
		t.Fatalf("engine: %v", err)
	}
	t.Cleanup(eng.Close)
	ch := newMemChannel(testLowWater)
	eng.AttachChannel(ch)
	return &rawReceiver{eng: eng, ch: ch, dest: dest, destDir: destDir}
}

func (r *rawReceiver) inject(t *testing.T, frame *phonebridgev1.TransferFrame) {
	t.Helper()
	wire, err := EncodeFrame(frame)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	r.eng.OnFrame(wire)
}

func (r *rawReceiver) offer(t *testing.T, id, name string, size uint64, chunkSize uint32) *phonebridgev1.FileOffer {
	t.Helper()
	offer := &phonebridgev1.FileOffer{
		TransferId: id,
		Filename:   name,
		MimeType:   "application/octet-stream",
		SizeBytes:  size,
		ChunkSize:  chunkSize,
	}
	r.inject(t, OfferFrame(offer))
	return offer
}

func (r *rawReceiver) chunk(t *testing.T, id string, index, offset uint64, data []byte) {
	t.Helper()
	r.inject(t, ChunkFrame(&phonebridgev1.FileChunk{
		TransferId: id,
		ChunkIndex: index,
		Offset:     offset,
		Data:       data,
	}))
}

func (r *rawReceiver) complete(t *testing.T, id string, size uint64, digest []byte) {
	t.Helper()
	r.inject(t, CompleteFrame(&phonebridgev1.FileComplete{
		TransferId:   id,
		SizeBytes:    size,
		Sha256Digest: digest,
	}))
}

func (r *rawReceiver) acceptFrames(t *testing.T) []*phonebridgev1.FileAccept {
	t.Helper()
	var out []*phonebridgev1.FileAccept
	for _, wire := range r.ch.Frames() {
		frame, err := DecodeFrame(wire)
		if err != nil {
			t.Fatalf("decode captured frame: %v", err)
		}
		if accept := frame.GetAccept(); accept != nil {
			out = append(out, accept)
		}
	}
	return out
}

func (r *rawReceiver) resultFrames(t *testing.T) []*phonebridgev1.FileResult {
	t.Helper()
	var out []*phonebridgev1.FileResult
	for _, wire := range r.ch.Frames() {
		frame, err := DecodeFrame(wire)
		if err != nil {
			t.Fatalf("decode captured frame: %v", err)
		}
		if result := frame.GetResult(); result != nil {
			out = append(out, result)
		}
	}
	return out
}

// startTransfer offers `total` bytes in 128-byte chunks and reports the id.
func startRawTransfer(t *testing.T, r *rawReceiver, total int) string {
	t.Helper()
	const chunkSize = 128
	id, err := NewTransferID()
	if err != nil {
		t.Fatalf("id: %v", err)
	}
	r.offer(t, id, "payload.bin", uint64(total), chunkSize)
	accepts := r.acceptFrames(t)
	if len(accepts) != 1 || !accepts[0].Accept {
		t.Fatalf("offer was not accepted: %+v", accepts)
	}
	return id
}

func sendValidChunks(t *testing.T, r *rawReceiver, id string, total int, chunkSize int) []byte {
	t.Helper()
	hasher := sha256.New()
	full := bytes.Repeat([]byte{0xA5}, total)
	for offset, index := 0, uint64(0); offset < total; index++ {
		end := offset + chunkSize
		if end > total {
			end = total
		}
		part := full[offset:end]
		r.chunk(t, id, index, uint64(offset), part)
		_, _ = hasher.Write(part)
		offset = end
	}
	return hasher.Sum(nil)
}

func TestInbound_HappyPathCommits(t *testing.T) {
	r := newRawReceiver(t, nil)
	id := startRawTransfer(t, r, 300)
	digest := sendValidChunks(t, r, id, 300, 128)
	r.complete(t, id, 300, digest)

	info := waitForState(t, r.eng, id, StateComplete, 5*time.Second)
	if info.SavedName != "payload.bin" {
		t.Fatalf("saved name = %q", info.SavedName)
	}
	if got := readFile(t, r.destDir+"/payload.bin"); !bytes.Equal(got, bytes.Repeat([]byte{0xA5}, 300)) {
		t.Fatalf("committed bytes differ from what was sent")
	}
	results := r.resultFrames(t)
	if len(results) != 1 || !results[0].Committed {
		t.Fatalf("expected exactly one committed FileResult, got %+v", results)
	}
	assertNoStagedPartials(t, r.dest)
}

func TestInbound_ChunkViolationsAbort(t *testing.T) {
	const total = 300
	cases := []struct {
		name   string
		mutate func(t *testing.T, r *rawReceiver, id string, digest []byte)
	}{
		{"gap", func(t *testing.T, r *rawReceiver, id string, _ []byte) {
			r.chunk(t, id, 0, 0, bytes.Repeat([]byte{1}, 128))
			r.chunk(t, id, 2, 256, bytes.Repeat([]byte{1}, 44))
		}},
		{"duplicate", func(t *testing.T, r *rawReceiver, id string, _ []byte) {
			r.chunk(t, id, 0, 0, bytes.Repeat([]byte{1}, 128))
			r.chunk(t, id, 0, 0, bytes.Repeat([]byte{1}, 128))
		}},
		{"offset mismatch", func(t *testing.T, r *rawReceiver, id string, _ []byte) {
			r.chunk(t, id, 0, 0, bytes.Repeat([]byte{1}, 128))
			r.chunk(t, id, 1, 0, bytes.Repeat([]byte{1}, 128))
		}},
		{"oversized chunk", func(t *testing.T, r *rawReceiver, id string, _ []byte) {
			r.chunk(t, id, 0, 0, bytes.Repeat([]byte{1}, 129))
		}},
		{"overrun", func(t *testing.T, r *rawReceiver, id string, _ []byte) {
			r.chunk(t, id, 0, 0, bytes.Repeat([]byte{1}, 128))
			r.chunk(t, id, 1, 128, bytes.Repeat([]byte{1}, 128))
			r.chunk(t, id, 2, 256, bytes.Repeat([]byte{1}, 128))
		}},
		{"short non-final chunk", func(t *testing.T, r *rawReceiver, id string, _ []byte) {
			r.chunk(t, id, 0, 0, bytes.Repeat([]byte{1}, 100))
		}},
		{"empty chunk", func(t *testing.T, r *rawReceiver, id string, _ []byte) {
			r.chunk(t, id, 0, 0, nil)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRawReceiver(t, nil)
			id := startRawTransfer(t, r, total)
			tc.mutate(t, r, id, nil)

			info := waitForState(t, r.eng, id, StateFailed, 5*time.Second)
			if info.ReasonCode != ReasonProtocolError {
				t.Fatalf("reason = %s, want PROTOCOL_ERROR (%q)", info.ReasonCode, info.ErrorMessage)
			}
			if entries := destEntries(t, r.destDir); len(entries) != 0 {
				t.Fatalf("aborted transfer committed a file: %v", entries)
			}
			assertNoStagedPartials(t, r.dest)
			results := r.resultFrames(t)
			if len(results) != 1 || results[0].Committed || results[0].Code != phonebridgev1.Code_CODE_INVALID_ARGUMENT {
				t.Fatalf("expected one INVALID_ARGUMENT result, got %+v", results)
			}
		})
	}
}

func TestInbound_ChecksumMismatchLeavesNoFile(t *testing.T) {
	r := newRawReceiver(t, nil)
	id := startRawTransfer(t, r, 300)
	sendValidChunks(t, r, id, 300, 128)

	wrong := make([]byte, sha256.Size)
	r.complete(t, id, 300, wrong)

	info := waitForState(t, r.eng, id, StateFailed, 5*time.Second)
	if info.ReasonCode != ReasonChecksumMismatch {
		t.Fatalf("reason = %s, want CHECKSUM_MISMATCH", info.ReasonCode)
	}
	if entries := destEntries(t, r.destDir); len(entries) != 0 {
		t.Fatalf("a corrupt transfer must not commit: %v", entries)
	}
	assertNoStagedPartials(t, r.dest)
	results := r.resultFrames(t)
	if len(results) != 1 || results[0].Committed || results[0].Code != phonebridgev1.Code_CODE_CHECKSUM_MISMATCH {
		t.Fatalf("expected a CHECKSUM_MISMATCH result, got %+v", results)
	}
}

func TestInbound_SizeMismatchLeavesNoFile(t *testing.T) {
	r := newRawReceiver(t, nil)
	id := startRawTransfer(t, r, 300)
	digest := sendValidChunks(t, r, id, 300, 128)

	// Same bytes, but the completion claims a different length.
	r.complete(t, id, 128, digest)

	info := waitForState(t, r.eng, id, StateFailed, 5*time.Second)
	if info.ReasonCode != ReasonChecksumMismatch {
		t.Fatalf("reason = %s, want CHECKSUM_MISMATCH", info.ReasonCode)
	}
	if entries := destEntries(t, r.destDir); len(entries) != 0 {
		t.Fatalf("mismatched size must not commit: %v", entries)
	}
}

func TestInbound_DeclaredOfferDigestIsChecked(t *testing.T) {
	r := newRawReceiver(t, nil)
	id, err := NewTransferID()
	if err != nil {
		t.Fatalf("id: %v", err)
	}
	declared := bytes.Repeat([]byte{0x11}, sha256.Size)
	r.inject(t, OfferFrame(&phonebridgev1.FileOffer{
		TransferId:   id,
		Filename:     "declared.bin",
		SizeBytes:    300,
		ChunkSize:    128,
		Sha256Digest: declared, // deliberately wrong
	}))
	digest := sendValidChunks(t, r, id, 300, 128)
	r.complete(t, id, 300, digest)

	info := waitForState(t, r.eng, id, StateFailed, 5*time.Second)
	if info.ReasonCode != ReasonChecksumMismatch {
		t.Fatalf("reason = %s, want CHECKSUM_MISMATCH", info.ReasonCode)
	}
}

func TestInbound_OfferRefusals(t *testing.T) {
	t.Run("unsafe filename", func(t *testing.T) {
		r := newRawReceiver(t, nil)
		r.offer(t, "abc", "../escape.bin", 10, 128)
		accepts := r.acceptFrames(t)
		if len(accepts) != 1 || accepts[0].Accept || accepts[0].Code != phonebridgev1.Code_CODE_UNSAFE_FILENAME {
			t.Fatalf("expected an UNSAFE_FILENAME refusal, got %+v", accepts)
		}
		if _, ok := r.eng.Get("abc"); !ok {
			t.Fatalf("a refused offer should still be visible in the history")
		}
	})

	t.Run("too large", func(t *testing.T) {
		r := newRawReceiver(t, func(cfg *Config) { cfg.MaxFileSize = 100 })
		r.offer(t, "abc", "big.bin", 101, 128)
		accepts := r.acceptFrames(t)
		if len(accepts) != 1 || accepts[0].Accept || accepts[0].Code != phonebridgev1.Code_CODE_FILE_TOO_LARGE {
			t.Fatalf("expected a FILE_TOO_LARGE refusal, got %+v", accepts)
		}
	})

	t.Run("bad chunk size", func(t *testing.T) {
		r := newRawReceiver(t, nil)
		r.offer(t, "abc", "chunk.bin", 10, MaxReceiveChunkSize+1)
		accepts := r.acceptFrames(t)
		if len(accepts) != 1 || accepts[0].Accept || accepts[0].Code != phonebridgev1.Code_CODE_INVALID_ARGUMENT {
			t.Fatalf("expected an INVALID_ARGUMENT refusal, got %+v", accepts)
		}
	})

	t.Run("bad declared digest", func(t *testing.T) {
		r := newRawReceiver(t, nil)
		r.inject(t, OfferFrame(&phonebridgev1.FileOffer{
			TransferId:   "abc",
			Filename:     "digest.bin",
			SizeBytes:    10,
			ChunkSize:    128,
			Sha256Digest: []byte{1, 2, 3},
		}))
		accepts := r.acceptFrames(t)
		if len(accepts) != 1 || accepts[0].Accept || accepts[0].Code != phonebridgev1.Code_CODE_INVALID_ARGUMENT {
			t.Fatalf("expected an INVALID_ARGUMENT refusal, got %+v", accepts)
		}
	})

	t.Run("no destination configured", func(t *testing.T) {
		eng, err := NewEngine(Config{})
		if err != nil {
			t.Fatalf("engine: %v", err)
		}
		defer eng.Close()
		ch := newMemChannel(0)
		eng.AttachChannel(ch)
		wire, _ := EncodeFrame(OfferFrame(&phonebridgev1.FileOffer{
			TransferId: "abc",
			Filename:   "x.bin",
			SizeBytes:  1,
			ChunkSize:  128,
		}))
		eng.OnFrame(wire)
		frames := ch.Frames()
		if len(frames) != 1 {
			t.Fatalf("expected one refusal frame, got %d", len(frames))
		}
		decoded, err := DecodeFrame(frames[0])
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got := decoded.GetAccept(); got == nil || got.Accept || got.Code != phonebridgev1.Code_CODE_UNAVAILABLE {
			t.Fatalf("expected an UNAVAILABLE refusal, got %+v", got)
		}
	})
}

func TestInbound_DuplicateOfferAndBusyRefused(t *testing.T) {
	r := newRawReceiver(t, nil)
	id := startRawTransfer(t, r, 300)

	// Replay of a live id: the in-flight check precedes the duplicate check, so
	// the honest answer is BUSY (there is a transfer with that id in flight).
	r.offer(t, id, "payload.bin", 300, 128)
	accepts := r.acceptFrames(t)
	last := accepts[len(accepts)-1]
	if last.Accept || last.Code != phonebridgev1.Code_CODE_TRANSFER_BUSY {
		t.Fatalf("a replay of a live id must be refused as BUSY, got %+v", last)
	}

	// A second, distinct offer while one is in flight.
	r.offer(t, "second-id", "other.bin", 300, 128)
	accepts = r.acceptFrames(t)
	last = accepts[len(accepts)-1]
	if last.Accept || last.Code != phonebridgev1.Code_CODE_TRANSFER_BUSY {
		t.Fatalf("a second inbound transfer must be refused as BUSY, got %+v", last)
	}

	entries, err := os.ReadDir(r.dest.PartialDir())
	if err != nil {
		t.Fatalf("read staging: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("refused offers must not create staged files, staging holds %v", entries)
	}
}

func TestInbound_ReplayedCompletedOfferRefused(t *testing.T) {
	r := newRawReceiver(t, nil)
	id := startRawTransfer(t, r, 300)
	digest := sendValidChunks(t, r, id, 300, 128)
	r.complete(t, id, 300, digest)
	waitForState(t, r.eng, id, StateComplete, 5*time.Second)

	before := len(r.acceptFrames(t))
	r.offer(t, id, "payload.bin", 300, 128)
	accepts := r.acceptFrames(t)
	if len(accepts) != before+1 {
		t.Fatalf("the replayed offer was not answered")
	}
	last := accepts[len(accepts)-1]
	if last.Accept || last.Code != phonebridgev1.Code_CODE_INVALID_ARGUMENT {
		t.Fatalf("a replayed completed offer must be refused as a duplicate, got %+v", last)
	}
}

func TestInbound_UnknownChunkIsIgnored(t *testing.T) {
	r := newRawReceiver(t, nil)
	id := startRawTransfer(t, r, 300)

	// A chunk for an id we never accepted must not disturb the live transfer.
	r.chunk(t, "not-a-transfer", 0, 0, bytes.Repeat([]byte{9}, 128))

	digest := sendValidChunks(t, r, id, 300, 128)
	r.complete(t, id, 300, digest)
	waitForState(t, r.eng, id, StateComplete, 5*time.Second)
}

func TestInbound_MalformedAndFutureFramesFailTheSoleTransfer(t *testing.T) {
	cases := []struct {
		name   string
		wire   []byte
		reason Reason
	}{
		{"malformed", []byte{0xff, 0xff, 0xff}, ReasonProtocolError},
		{"oversized", make([]byte, MaxFrameBytes+1), ReasonProtocolError},
		{"future version", mustMarshal(&phonebridgev1.TransferFrame{
			Version: FrameVersion + 1,
			Body:    &phonebridgev1.TransferFrame_Chunk{Chunk: &phonebridgev1.FileChunk{TransferId: "x", Data: []byte{1}}},
		}), ReasonIncompatibleVersion},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := newRawReceiver(t, nil)
			id := startRawTransfer(t, r, 300)
			r.eng.OnFrame(tc.wire)
			info := waitForState(t, r.eng, id, StateFailed, 5*time.Second)
			if info.ReasonCode != tc.reason {
				t.Fatalf("reason = %s, want %s", info.ReasonCode, tc.reason)
			}
			assertNoStagedPartials(t, r.dest)
		})
	}
}

func TestInbound_MalformedWithoutTransferIsIgnored(t *testing.T) {
	r := newRawReceiver(t, nil)
	r.eng.OnFrame([]byte{0xff, 0xff})                                               // nothing in flight
	r.eng.OnFrame(mustMarshal(&phonebridgev1.TransferFrame{Version: FrameVersion})) // no body
	if len(r.ch.Frames()) != 0 {
		t.Fatalf("an unattributable frame must not produce traffic")
	}
}

func TestInbound_StallTimeoutAbortsAndCleansUp(t *testing.T) {
	r := newRawReceiver(t, func(cfg *Config) { cfg.StallTimeout = 150 * time.Millisecond })
	id := startRawTransfer(t, r, 300)

	info := waitForState(t, r.eng, id, StateFailed, 5*time.Second)
	if info.ReasonCode != ReasonInterrupted {
		t.Fatalf("reason = %s, want INTERRUPTED", info.ReasonCode)
	}
	assertNoStagedPartials(t, r.dest)
	results := r.resultFrames(t)
	if len(results) != 1 || results[0].Committed {
		t.Fatalf("a stalled transfer must be reported as not committed, got %+v", results)
	}
}

func TestInbound_PeerCancelAbortsAndCleansUp(t *testing.T) {
	r := newRawReceiver(t, nil)
	id := startRawTransfer(t, r, 300)
	r.chunk(t, id, 0, 0, bytes.Repeat([]byte{1}, 128))

	r.inject(t, CancelFrame(&phonebridgev1.FileCancel{
		TransferId: id,
		Code:       phonebridgev1.Code_CODE_TRANSFER_CANCELLED,
		Reason:     "user cancelled",
	}))

	info := waitForState(t, r.eng, id, StateCancelled, 5*time.Second)
	if info.ReasonCode != ReasonCancelledByPeer {
		t.Fatalf("reason = %s, want CANCELLED_BY_PEER", info.ReasonCode)
	}
	assertNoStagedPartials(t, r.dest)
	if entries := destEntries(t, r.destDir); len(entries) != 0 {
		t.Fatalf("cancel left destination entries: %v", entries)
	}
}
