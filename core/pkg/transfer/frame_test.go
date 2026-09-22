package transfer

import (
	"bytes"
	"encoding/hex"
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

func TestNewTransferIDIsRandomHex(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 256; i++ {
		id, err := NewTransferID()
		if err != nil {
			t.Fatalf("NewTransferID: %v", err)
		}
		if len(id) != 32 {
			t.Fatalf("transfer id %q is %d chars, want 32", id, len(id))
		}
		if _, err := hex.DecodeString(id); err != nil {
			t.Fatalf("transfer id %q is not hex: %v", id, err)
		}
		if seen[id] {
			t.Fatalf("duplicate transfer id %q", id)
		}
		seen[id] = true
	}
}

func TestSanitizeFilename(t *testing.T) {
	valid := []string{"report.pdf", "a.bin", "file with spaces.txt", "UPPER.TXT", "über.txt", "x"}
	for _, name := range valid {
		got, err := SanitizeFilename(name)
		if err != nil {
			t.Fatalf("%q should be accepted: %v", name, err)
		}
		if got != name {
			t.Fatalf("SanitizeFilename(%q) = %q", name, got)
		}
	}

	invalid := []string{
		"",
		"   ",
		".",
		"..",
		".hidden",
		"../escape",
		"dir/file",
		`dir\file`,
		"nul\x00name",
		"ctrl\x01name",
		string(bytes.Repeat([]byte("a"), maxFilenameBytes+1)),
	}
	for _, name := range invalid {
		if _, err := SanitizeFilename(name); err == nil {
			t.Fatalf("%q should be refused (DEC-024 path safety)", name)
		} else if f, ok := IsFailure(err); !ok || f.Code != phonebridgev1.Code_CODE_UNSAFE_FILENAME {
			t.Fatalf("%q refused with %v, want CODE_UNSAFE_FILENAME", name, err)
		}
	}

	trimmed, err := SanitizeFilename("  padded.bin  ")
	if err != nil || trimmed != "padded.bin" {
		t.Fatalf("surrounding whitespace should be trimmed: %q err=%v", trimmed, err)
	}
}

func TestMimeForName(t *testing.T) {
	if got := MimeForName("song.mp3"); !strings.HasPrefix(got, "audio/") {
		t.Fatalf("MimeForName(song.mp3) = %q, want an audio type", got)
	}
	if got := MimeForName("no-extension"); got != "" {
		t.Fatalf("MimeForName(no-extension) = %q, want empty", got)
	}
}

func TestFrameRoundTrip(t *testing.T) {
	offer := &phonebridgev1.FileOffer{
		TransferId:   "abc",
		Filename:     "report.pdf",
		MimeType:     "application/pdf",
		SizeBytes:    1000,
		Sha256Digest: make([]byte, 32),
		ChunkSize:    65536,
		CreatedAtMs:  42,
	}
	for _, frame := range []*phonebridgev1.TransferFrame{
		OfferFrame(offer),
		AcceptFrame(&phonebridgev1.FileAccept{TransferId: "abc", Accept: true, Code: phonebridgev1.Code_CODE_OK}),
		ChunkFrame(&phonebridgev1.FileChunk{TransferId: "abc", ChunkIndex: 3, Offset: 12, Data: []byte("hello")}),
		CompleteFrame(&phonebridgev1.FileComplete{TransferId: "abc", SizeBytes: 1000, Sha256Digest: make([]byte, 32)}),
		ResultFrame(&phonebridgev1.FileResult{TransferId: "abc", Committed: true, SavedName: "report.pdf"}),
		CancelFrame(&phonebridgev1.FileCancel{TransferId: "abc", Code: phonebridgev1.Code_CODE_TRANSFER_CANCELLED}),
	} {
		wire, err := EncodeFrame(frame)
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		decoded, err := DecodeFrame(wire)
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if decoded.Version != FrameVersion {
			t.Fatalf("decoded version = %d, want %d", decoded.Version, FrameVersion)
		}
		switch frame.Body.(type) {
		case *phonebridgev1.TransferFrame_Offer:
			if decoded.GetOffer().GetFilename() != "report.pdf" || decoded.GetOffer().GetSizeBytes() != 1000 {
				t.Fatalf("offer round trip lost fields: %v", decoded.GetOffer())
			}
		case *phonebridgev1.TransferFrame_Chunk:
			if !bytes.Equal(decoded.GetChunk().GetData(), []byte("hello")) {
				t.Fatalf("chunk payload was not preserved: %v", decoded.GetChunk())
			}
			if decoded.GetChunk().GetChunkIndex() != 3 || decoded.GetChunk().GetOffset() != 12 {
				t.Fatalf("chunk numbering was not preserved: %v", decoded.GetChunk())
			}
		case *phonebridgev1.TransferFrame_Accept:
			if !decoded.GetAccept().GetAccept() {
				t.Fatalf("accept flag was not preserved")
			}
		case *phonebridgev1.TransferFrame_Complete:
			if decoded.GetComplete().GetSizeBytes() != 1000 {
				t.Fatalf("complete size was not preserved")
			}
		case *phonebridgev1.TransferFrame_Result:
			if !decoded.GetResult().GetCommitted() || decoded.GetResult().GetSavedName() != "report.pdf" {
				t.Fatalf("result was not preserved")
			}
		case *phonebridgev1.TransferFrame_Cancel:
			if decoded.GetCancel().GetCode() != phonebridgev1.Code_CODE_TRANSFER_CANCELLED {
				t.Fatalf("cancel code was not preserved")
			}
		}
	}
}

// TestTransferFrameWireFieldNumbers pins the wire contract: the field numbers are
// what make `phonebridge.v1` compatible across releases, so a change here is a
// breaking protocol change, not a refactor.
func TestTransferFrameWireFieldNumbers(t *testing.T) {
	wire, err := EncodeFrame(ChunkFrame(&phonebridgev1.FileChunk{TransferId: "abc", ChunkIndex: 1, Offset: 2, Data: []byte("x")}))
	if err != nil {
		t.Fatalf("encode: %v", err)
	}

	seen := map[protowire.Number]bool{}
	for len(wire) > 0 {
		num, typ, n := protowire.ConsumeTag(wire)
		if n < 0 {
			t.Fatalf("malformed tag while scanning the frame")
		}
		seen[num] = true
		valLen := protowire.ConsumeFieldValue(num, typ, wire[n:])
		if valLen < 0 {
			t.Fatalf("malformed field %d", num)
		}
		wire = wire[n+valLen:]
	}
	if !seen[1] {
		t.Fatalf("TransferFrame.version must stay field 1")
	}
	if !seen[12] {
		t.Fatalf("TransferFrame.chunk must stay field 12")
	}

	if got := (&phonebridgev1.TransferFrame{}).ProtoReflect().Descriptor().Fields().ByName("chunk").Number(); got != 12 {
		t.Fatalf("chunk field number = %d, want 12", got)
	}
	fileChunkFields := (&phonebridgev1.FileChunk{}).ProtoReflect().Descriptor().Fields()
	for name, want := range map[string]int{"transfer_id": 1, "chunk_index": 2, "offset": 3, "data": 4} {
		if got := int(fileChunkFields.ByName(protoreflect.Name(name)).Number()); got != want {
			t.Fatalf("FileChunk.%s = field %d, want %d", name, got, want)
		}
	}
}

func TestDecodeFrameRejections(t *testing.T) {
	cases := []struct {
		name   string
		wire   []byte
		code   phonebridgev1.Code
		reason Reason
	}{
		{"empty", nil, phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError},
		{"malformed", []byte{0xff, 0xff, 0xff, 0xff}, phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError},
		{"oversized", make([]byte, MaxFrameBytes+1), phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError},
		{"no body", mustMarshal(&phonebridgev1.TransferFrame{Version: FrameVersion}), phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonProtocolError},
		{"future version", mustMarshal(&phonebridgev1.TransferFrame{Version: FrameVersion + 1, Body: &phonebridgev1.TransferFrame_Cancel{Cancel: &phonebridgev1.FileCancel{TransferId: "x"}}}), phonebridgev1.Code_CODE_INCOMPATIBLE_VERSION, ReasonIncompatibleVersion},
	}
	for _, tc := range cases {
		_, err := DecodeFrame(tc.wire)
		if err == nil {
			t.Fatalf("%s: expected rejection", tc.name)
		}
		f, ok := IsFailure(err)
		if !ok {
			t.Fatalf("%s: error is not a typed Failure: %v", tc.name, err)
		}
		if f.Code != tc.code || f.Reason != tc.reason {
			t.Fatalf("%s: got code=%s reason=%s, want code=%s reason=%s", tc.name, f.Code, f.Reason, tc.code, tc.reason)
		}
	}
}

func mustMarshal(frame *phonebridgev1.TransferFrame) []byte {
	wire, err := proto.Marshal(frame)
	if err != nil {
		panic(err)
	}
	return wire
}
