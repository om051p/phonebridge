// Package frames implements the Phase 6 Slice 3 in-app frame pipeline:
//
//	AccessUnit → TapSink (tee) → ffmpeg MJPEG (q3) → latest-wins
//	→ Hub → StreamFrames (UDS/gRPC) → Flutter decode-on-arrival
//
// The package never touches session control: frame load is bounded by
// latest-wins queues everywhere, and no stage can block the receiver's AU
// path, StreamEvents, clipboard, transfer or reconnect handling.
package frames

import "time"

// MaxChunkBytes is the per-message JPEG ceiling carried over
// StreamFramesResponse (DEC-018 bulk rule: <= 64 KiB per message).
const MaxChunkBytes = 64 * 1024

// Frame is one complete JPEG frame ready for delivery to StreamFrames
// subscribers. A Frame is immutable after construction; Chunks are views
// into JPEG, so all subscribers share one buffer (bounded memory under
// latest-wins fan-out).
type Frame struct {
	// ID is the hub-wide monotonic frame id within one session (resets per
	// session). Subscriptions rebase it to start at 1 (proto contract) while
	// preserving gaps, which are the client-visible drop signal.
	ID             uint64
	Width          uint32
	Height         uint32
	JPEG           []byte
	SentUnixMicros uint64
	chunks         [][]byte
}

// NewFrame builds a Frame and pre-chunks the JPEG into <= MaxChunkBytes
// slices. width/height come from the JPEG SOF marker (self-describing).
func NewFrame(id uint64, jpeg []byte, width, height uint32) *Frame {
	f := &Frame{
		ID:             id,
		Width:          width,
		Height:         height,
		JPEG:           jpeg,
		SentUnixMicros: uint64(time.Now().UnixMicro()),
	}
	for off := 0; off < len(jpeg); off += MaxChunkBytes {
		end := off + MaxChunkBytes
		if end > len(jpeg) {
			end = len(jpeg)
		}
		f.chunks = append(f.chunks, jpeg[off:end])
	}
	if len(f.chunks) == 0 {
		// A zero-length JPEG still frames the chunk contract (one chunk).
		f.chunks = [][]byte{{}}
	}
	return f
}

// Chunks returns the <=64 KiB slices composing the frame (never empty).
func (f *Frame) Chunks() [][]byte { return f.chunks }

// jpegDimensions extracts the frame size from a baseline/progressive JPEG
// SOF marker. Returns ok=false when no SOF marker is found (malformed).
func jpegDimensions(jpeg []byte) (width, height uint32, ok bool) {
	// Minimal marker walk: FFD8, then segments FFnn + len16 until a SOF
	// (FFC0..FFC3, FFC5..FFC7, FFC9..FFCB, FFCD..FFCF).
	if len(jpeg) < 4 || jpeg[0] != 0xFF || jpeg[1] != 0xD8 {
		return 0, 0, false
	}
	i := 2
	for i+3 < len(jpeg) {
		if jpeg[i] != 0xFF {
			i++ // resync (should not happen between markers)
			continue
		}
		marker := jpeg[i+1]
		switch {
		case marker == 0xD8 || marker == 0xD9 || marker == 0x01 ||
			(marker >= 0xD0 && marker <= 0xD7):
			i += 2 // standalone marker
			continue
		case isSOFMarker(marker):
			if i+9 > len(jpeg) {
				return 0, 0, false
			}
			h := uint32(jpeg[i+5])<<8 | uint32(jpeg[i+6])
			w := uint32(jpeg[i+7])<<8 | uint32(jpeg[i+8])
			return w, h, true
		default:
			if i+3 > len(jpeg) {
				return 0, 0, false
			}
			segLen := int(jpeg[i+2])<<8 | int(jpeg[i+3])
			if segLen < 2 {
				return 0, 0, false
			}
			i += 2 + segLen
		}
	}
	return 0, 0, false
}

func isSOFMarker(m byte) bool {
	switch {
	case m >= 0xC0 && m <= 0xC3,
		m >= 0xC5 && m <= 0xC7,
		m >= 0xC9 && m <= 0xCB,
		m >= 0xCD && m <= 0xCF:
		return true
	}
	return false
}
