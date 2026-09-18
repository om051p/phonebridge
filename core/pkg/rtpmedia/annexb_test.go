package rtpmedia

import (
	"bytes"
	"reflect"
	"testing"
)

var sc3 = []byte{0, 0, 1}
var sc4 = []byte{0, 0, 0, 1}

func nal(t byte, payload ...byte) []byte {
	return append([]byte{t}, payload...)
}

func join(parts ...[]byte) []byte {
	var out []byte
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func TestSplitAnnexBThreeByteStartCodes(t *testing.T) {
	au := join(sc3, nal(7, 1, 2), sc3, nal(8, 3), sc3, nal(5, 4, 5, 6))
	got := SplitAnnexB(au)
	want := [][]byte{nal(7, 1, 2), nal(8, 3), nal(5, 4, 5, 6)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSplitAnnexBFourByteStartCodes(t *testing.T) {
	au := join(sc4, nal(7, 1, 2), sc4, nal(8, 3), sc4, nal(5, 4))
	got := SplitAnnexB(au)
	want := [][]byte{nal(7, 1, 2), nal(8, 3), nal(5, 4)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSplitAnnexBMixedStartCodes(t *testing.T) {
	// Real MediaCodec output uses 4-byte codes, but 3-byte codes can appear
	// (e.g. after re-muxing). Both must coexist in one stream.
	au := join(sc4, nal(7, 1), sc3, nal(8, 2), sc4, nal(5, 3))
	got := SplitAnnexB(au)
	want := [][]byte{nal(7, 1), nal(8, 2), nal(5, 3)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// TestSplitAnnexBTrailingZerosAreSeparators: zero bytes after a NAL belong to
// the next start code (or padding), never to the payload.
func TestSplitAnnexBTrailingZerosAreSeparators(t *testing.T) {
	au := join(sc4, nal(7, 1), []byte{0, 0}, sc4, nal(5, 2), []byte{0, 0, 0})
	got := SplitAnnexB(au)
	want := [][]byte{nal(7, 1), nal(5, 2)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// TestSplitAnnexBEmulationPreventionNotStartCode: 00 00 03 inside a NAL is
// payload (emulation prevention), not a start code. Also 00 00 02 (legal
// payload byte pair followed by non-zero) must not split.
func TestSplitAnnexBEmulationPreventionNotStartCode(t *testing.T) {
	// slice with embedded 00 00 03 01 (MP byte triple + first MB byte)
	inner := []byte{1, 0, 0, 3, 1, 9}
	au := join(sc4, append(nal(1), inner...))
	got := SplitAnnexB(au)
	if len(got) != 1 || !bytes.Equal(got[0], append(nal(1), inner...)) {
		t.Fatalf("emulation prevention split the NAL: got %v", got)
	}
	inner2 := []byte{1, 0, 0, 2, 1}
	au2 := join(sc4, append(nal(1), inner2...))
	got2 := SplitAnnexB(au2)
	if len(got2) != 1 || !bytes.Equal(got2[0], append(nal(1), inner2...)) {
		t.Fatalf("00 00 02 split the NAL: got %v", got2)
	}
}

// TestSplitAnnexBAdjacentStartCodesSkipped: empty NALs (two codes in a row)
// are dropped, not surfaced.
func TestSplitAnnexBAdjacentStartCodesSkipped(t *testing.T) {
	au := join(sc4, sc4, nal(7, 1), sc3, sc4, nal(5, 2))
	got := SplitAnnexB(au)
	want := [][]byte{nal(7, 1), nal(5, 2)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSplitAnnexBLeadingGarbageIgnored(t *testing.T) {
	au := join([]byte{0xAB, 0xCD}, sc4, nal(7, 1))
	got := SplitAnnexB(au)
	want := [][]byte{nal(7, 1)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestSplitAnnexBEmptyAndHeaderless(t *testing.T) {
	if got := SplitAnnexB(nil); got != nil {
		t.Fatalf("nil input: got %v", got)
	}
	if got := SplitAnnexB([]byte{1, 2, 3, 0, 0}); got != nil {
		t.Fatalf("no start code: got %v", got)
	}
	// Start code with nothing after it: no payload, no NALs.
	if got := SplitAnnexB(sc4); got != nil {
		t.Fatalf("dangling start code: got %v", got)
	}
}

// TestSplitAnnexBLongZeroRun: a run of >4 zeros between NALs must not merge
// or duplicate NALs.
func TestSplitAnnexBLongZeroRun(t *testing.T) {
	au := join(sc4, nal(7, 1), []byte{0, 0, 0, 0, 0, 0}, sc4, nal(5, 2))
	got := SplitAnnexB(au)
	want := [][]byte{nal(7, 1), nal(5, 2)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// TestSplitAnnexBZeroRunAtEnd: trailing zeros only, after the last NAL.
func TestSplitAnnexBZeroRunAtEnd(t *testing.T) {
	au := join(sc4, nal(7, 9, 9), []byte{0, 0, 0, 0})
	got := SplitAnnexB(au)
	want := [][]byte{nal(7, 9, 9)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

// TestSplitAnnexBIteratorEquivalence: the iterator API must return the same
// sequence as the convenience wrapper, and be exhaustible.
func TestSplitAnnexBIteratorEquivalence(t *testing.T) {
	au := join(sc4, nal(7, 1), sc3, nal(8, 2), sc4, nal(5, 3, 3))
	wrap := SplitAnnexB(au)
	it := NewAnnexBIter(au)
	var iter [][]byte
	for {
		n, ok := it.Next()
		if !ok {
			break
		}
		iter = append(iter, n)
		// Exhaustion must be stable.
		if n == nil {
			t.Fatal("Next returned ok with nil NAL")
		}
	}
	if !reflect.DeepEqual(wrap, iter) {
		t.Fatalf("iterator %v != wrapper %v", iter, wrap)
	}
	if _, ok := it.Next(); ok {
		t.Fatal("exhausted iterator returned another NAL")
	}
}

// TestSplitAnnexBNALsSubsliceOfInput documents the aliasing contract.
func TestSplitAnnexBNALsSubsliceOfInput(t *testing.T) {
	au := join(sc4, nal(7, 1, 2, 3))
	nals := SplitAnnexB(au)
	if len(nals) != 1 {
		t.Fatalf("got %d NALs", len(nals))
	}
	// The NAL shares backing storage with au.
	if &nals[0][0] != &au[4] {
		t.Fatal("NAL is not a subslice of the input buffer")
	}
}
