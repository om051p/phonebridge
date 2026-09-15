// Package main — Spike 01 FFI branch (EXPERIMENTAL, throwaway).
//
// Built with `-buildmode=c-shared`, this produces a .so that Dart loads with
// dart:ffi, so the Go core runs *inside* the Flutter process instead of behind
// a socket. The exports below exist only to measure what that costs and what it
// gives up (see docs/spikes/01-flutter-go-ipc-linux-results.md).
//
// Marshalling contract (deliberate, and a finding in itself):
// every export writes into a CALLER-OWNED buffer and returns the byte count.
// Nothing allocates on the C heap, so Dart never has to free Go memory and
// there is no cross-language ownership question. The escape hatch that this
// removes (returning C.CString and requiring a PBSpikeFree) is exactly the kind
// of footgun the UDS+gRPC branch avoids entirely, because protobuf owns the
// buffers on both sides.
//
// This file must stay `package main` with an empty main(): cgo only emits
// //export symbols for package main in c-shared mode.
package main

/*
#include <stdint.h>
*/
import "C"

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// version is reported through PBSpikeVersion.
var version = "0.0.0-spike01"

var (
	// eventSeq counts "events produced by the Go core", the thing a real daemon
	// would push to the UI (clipboard change, notification, incoming file).
	eventSeq atomic.Uint64

	tickerOnce sync.Once
	tickerNs   atomic.Int64
	goroutines atomic.Int64
)

// PBSpikeVersion writes the version string into buf (capacity bufLen) and
// returns the number of bytes written, or -1 if bufLen is too small.
//
//export PBSpikeVersion
func PBSpikeVersion(buf *C.char, bufLen C.int) C.int {
	return copyInto(buf, bufLen, version)
}

// PBSpikePing echoes nonce back. This is the FFI equivalent of the gRPC Ping:
// one call, no serialisation, same process.
//
//export PBSpikePing
func PBSpikePing(nonce C.uint64_t) C.uint64_t {
	return nonce
}

// PBSpikeEcho copies inLen bytes from in to out (capacity outCap) and returns
// the number of bytes copied. Models binary-safe payloads (clipboard images,
// file chunks) without any struct marshalling.
//
//export PBSpikeEcho
func PBSpikeEcho(in *C.char, inLen C.int, out *C.char, outCap C.int) C.int {
	if in == nil || out == nil || inLen <= 0 || outCap <= 0 {
		return 0
	}
	n := int(inLen)
	if n > int(outCap) {
		n = int(outCap)
	}
	src := unsafeSlice(in, int(inLen))
	dst := unsafeSlice(out, int(outCap))
	return C.int(copy(dst, src[:n]))
}

// PBSpikeStartTicker starts a Go-side producer that bumps the event counter
// every intervalMicros. Idempotent. Returns the current sequence.
//
// This simulates the *real* shape of the problem: the Go core discovers
// something asynchronously and the UI must learn about it. Under FFI there is no
// transport to push over, so the exports below only let Dart *poll*.
//
//export PBSpikeStartTicker
func PBSpikeStartTicker(intervalMicros C.int64_t) C.uint64_t {
	tickerOnce.Do(func() {
		tickerNs.Store(int64(intervalMicros))
		go func() {
			for {
				time.Sleep(time.Duration(tickerNs.Load()) * time.Microsecond)
				eventSeq.Add(1)
			}
		}()
		// A second goroutine, so the Go runtime visibly runs in-process and so
		// the counter is not the only thing happening on the Go scheduler.
		go func() {
			for {
				time.Sleep(50 * time.Millisecond)
				goroutines.Store(int64(runtime.NumGoroutine()))
			}
		}()
	})
	return C.uint64_t(eventSeq.Load())
}

// PBSpikePollEvent returns the newest event sequence. Polling is the only
// Go -> Flutter channel this branch has without a Dart API DL callback shim.
//
// A poller that is slower than the producer cannot see intermediate events:
// this API silently coalesces them. That limitation is the finding.
//
//export PBSpikePollEvent
func PBSpikePollEvent() C.uint64_t {
	return C.uint64_t(eventSeq.Load())
}

// PBSpikeBlock sleeps for micros microseconds *on the caller's thread*, which
// is how every synchronous FFI call behaves. Called from the UI isolate it
// freezes Flutter for the whole duration: the reason a real FFI design would
// have to move every call onto a helper isolate.
//
//export PBSpikeBlock
func PBSpikeBlock(micros C.int64_t) {
	time.Sleep(time.Duration(micros) * time.Microsecond)
}

// PBSpikeGoroutines reports runtime.NumGoroutine(). Non-zero proves the Go
// runtime, scheduler and GC live inside the Flutter process.
//
//export PBSpikeGoroutines
func PBSpikeGoroutines() C.int {
	return C.int(runtime.NumGoroutine())
}

// PBSpikeFree is intentionally absent: no export allocates C memory, so nothing
// needs freeing. See the marshalling contract above.

// unsafeSlice views n bytes at p as a Go slice. cgo hands Go a raw pointer into
// Dart-owned memory; the caller's contract (inLen/outCap) is what makes the
// length trustworthy, so both callers clamp before slicing.
func unsafeSlice(p *C.char, n int) []byte {
	if p == nil || n <= 0 {
		return nil
	}
	return unsafe.Slice((*byte)(unsafe.Pointer(p)), n)
}

// copyInto writes s into a caller-owned buffer, returning the byte count or -1
// if the buffer is too small (the caller then retries with a bigger buffer).
func copyInto(buf *C.char, bufLen C.int, s string) C.int {
	if buf == nil || bufLen <= 0 {
		return -1
	}
	dst := unsafeSlice(buf, int(bufLen))
	if len(s) > len(dst) {
		return -1
	}
	return C.int(copy(dst, s))
}

func main() {}
