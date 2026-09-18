package webrtc

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/pion/rtp"

	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

// goldenAU is one access unit of the Spike 04 capture slice with its
// receiver-recorded metadata (idx bytes rtp_ts idr marker spsN ppsN).
type goldenAU struct {
	idx   int
	data  []byte
	rtpTS uint32
	idr   bool
}

// loadGoldenAUs reads the real capture slice kept in the rtpmedia package's
// testdata (40 AUs cut from the burst-shaped spike run).
func loadGoldenAUs(t *testing.T) []goldenAU {
	t.Helper()
	base := "../../pkg/rtpmedia/testdata/spike04-shaped4-1-slice"
	blob, err := os.ReadFile(base + ".h264")
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(base + ".idx")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var aus []goldenAU
	sc := bufio.NewScanner(f)
	off := 0
	for sc.Scan() {
		fs := strings.Fields(sc.Text())
		if len(fs) < 7 {
			continue
		}
		idx, err1 := strconv.Atoi(fs[0])
		size, err2 := strconv.Atoi(fs[1])
		ts64, err4 := strconv.ParseUint(fs[2], 10, 32)
		idr, err3 := strconv.Atoi(fs[3])
		if err1 != nil || err2 != nil || err3 != nil || err4 != nil {
			continue
		}
		if off+size > len(blob) {
			t.Fatal("index sizes exceed capture")
		}
		aus = append(aus, goldenAU{idx: idx, data: blob[off : off+size : off+size], rtpTS: uint32(ts64), idr: idr == 1})
		off += size
	}
	if len(aus) == 0 {
		t.Fatal("no AUs loaded")
	}
	return aus
}

// stripParameterSets removes SPS/PPS NALs, simulating the encoder's native
// behaviour (parameter sets only once at stream start, not on every IDR).
func stripParameterSets(au []byte) []byte {
	nals := rtpmedia.SplitAnnexB(au)
	var kept [][]byte
	for _, n := range nals {
		if !rtpmedia.IsParameterSet(n) {
			kept = append(kept, n)
		}
	}
	if len(kept) == len(nals) {
		return au
	}
	var out []byte
	for _, n := range kept {
		out = append(out, 0, 0, 0, 1)
		out = append(out, n...)
	}
	return out
}

// recordingSink collects emitted packets (thread-safe: Emit is called from
// the single writer while assertions read from the test goroutine).
type recordingSink struct {
	mu       sync.Mutex
	packets  []rtp.Packet
	failFrom int // 0 = never fail; else emit an error once this many packets were stored
}

func (r *recordingSink) Emit(p rtp.Packet) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failFrom > 0 && len(r.packets) >= r.failFrom {
		return errors.New("sink: transport gone")
	}
	r.packets = append(r.packets, p)
	return nil
}

func (r *recordingSink) snapshot() []rtp.Packet {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]rtp.Packet, len(r.packets))
	copy(out, r.packets)
	return out
}

func (r *recordingSink) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.packets)
}

// waitCond polls cond until true or the timeout elapses.
func waitCond(cond func() bool, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(2 * time.Millisecond)
	}
	return cond()
}

func newTestSender(t *testing.T, cfg SenderConfig, sink Sink) *Sender {
	t.Helper()
	s := NewSender(nil, cfg)
	s.SetSink(sink)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	return s
}

// TestSenderGoldenPipelineOrderingMarkerTimestamps streams the 40 golden AUs
// through the full pipeline (PSI on, small pacing gap) and verifies the
// DEC-021 wire semantics end to end at the sink: sequence continuity,
// per-AU timestamps, marker-on-last-AU-packet, and SPS/PPS re-injection.
func TestSenderGoldenPipelineOrderingMarkerTimestamps(t *testing.T) {
	aus := loadGoldenAUs(t)
	sink := &recordingSink{}
	s := newTestSender(t, SenderConfig{PSIReinject: true}, sink)

	// Seed the PSI cache with the stream-start parameter sets (production
	// learns them from the CSD AU), then push every AU stripped of its
	// in-band parameter sets — the re-injected stream must reproduce the
	// original wire bytes.
	if csd := s.Cache().Prepare(rtpmedia.SplitAnnexB(aus[0].data)); csd == nil {
		t.Fatal("CSD prepare failed")
	}
	for i, au := range aus {
		data := stripParameterSets(au.data)
		if !s.Push(data, int64(i)*33334, au.idr) {
			t.Fatalf("AU %d rejected (no drops expected at depth 256)", au.idx)
		}
		time.Sleep(time.Millisecond) // ≈30 fps feed, real cadence
	}
	if !waitCond(func() bool { return int(s.SentAUs.Load()) == len(aus) }, 5*time.Second) {
		t.Fatalf("sent %d/%d AUs (dropped %d)", s.SentAUs.Load(), len(aus), s.DroppedAUs.Load())
	}

	pkts := sink.snapshot()
	if len(pkts) == 0 {
		t.Fatal("no packets emitted")
	}

	// 1. Sequence continuity: +1 across the whole stream.
	for i := 1; i < len(pkts); i++ {
		want := pkts[i-1].SequenceNumber + 1
		if pkts[i].SequenceNumber != want {
			t.Fatalf("seq discontinuity at %d: %d -> %d", i, pkts[i-1].SequenceNumber, pkts[i].SequenceNumber)
		}
	}

	// 2. Reassemble AUs on markers and check timestamp/marker semantics.
	type auPkt struct {
		ts      uint32
		markers int
		pkts    int
	}
	var ausOut []auPkt
	var cur auPkt
	open := false
	for _, p := range pkts {
		if !open {
			cur = auPkt{ts: p.Timestamp}
			open = true
		}
		if p.Timestamp != cur.ts {
			t.Fatalf("timestamp changed mid-AU: %d -> %d", cur.ts, p.Timestamp)
		}
		cur.pkts++
		if p.Marker {
			cur.markers++
			ausOut = append(ausOut, cur)
			open = false
		}
	}
	if open {
		t.Fatal("stream ended with an unterminated AU (missing marker)")
	}
	if len(ausOut) != len(aus) {
		t.Fatalf("reconstructed %d AUs, want %d", len(ausOut), len(aus))
	}
	for i, a := range ausOut {
		if a.markers != 1 {
			t.Fatalf("AU %d carries %d markers", i, a.markers)
		}
		if i > 0 && a.ts <= ausOut[i-1].ts {
			t.Fatalf("timestamp not monotonic at AU %d: %d <= %d", i, a.ts, ausOut[i-1].ts)
		}
	}
	// Feed was 33,334 µs ≈ 3000 RTP ticks per AU.
	if d := ausOut[1].ts - ausOut[0].ts; d < 2999 || d > 3001 {
		t.Fatalf("AU timestamp delta = %d, want ≈3000", d)
	}

	// 3. PSI: with all in-band parameter sets stripped pre-push, the first
	// two payloads of every IDR AU must be the re-injected SPS and PPS.
	cache := s.Cache()
	if !cache.HasParameterSets() || cache.SPS() == nil || cache.PPS() == nil {
		t.Fatal("cache never learned parameter sets")
	}
	sps, pps := cache.SPS(), cache.PPS()
	idrCount := 0
	nalsOf := func(auStart int, ts uint32) [][]byte {
		var nals [][]byte
		var cur []byte
		open := false
		for _, p := range pkts[auStart:] {
			if p.Timestamp != ts {
				break
			}
			ty := rtpmedia.NALType(p.Payload[0])
			switch {
			case ty >= 1 && ty <= 23:
				nals = append(nals, p.Payload)
				open = false
			case ty == rtpmedia.NALTypeFUA:
				if !open {
					cur = append([]byte{p.Payload[0]&0x60 | p.Payload[1]&0x1F}, p.Payload[2:]...)
					open = true
				} else {
					cur = append(cur, p.Payload[2:]...)
				}
			}
			if open && p.Marker {
				nals = append(nals, cur)
			}
		}
		return nals
	}
	off := 0
	for i, a := range ausOut {
		_ = i
		nals := nalsOf(off, a.ts)
		off += a.pkts
		isIDR := false
		for _, n := range nals {
			if rtpmedia.NALType(n[0]) == rtpmedia.NALTypeIDR {
				isIDR = true
			}
		}
		if !isIDR {
			continue
		}
		idrCount++
		if len(nals) < 3 || !nalsEqual(nals[0], sps) || !nalsEqual(nals[1], pps) {
			t.Fatalf("IDR AU not preceded by re-injected SPS+PPS (got %d NALs)", len(nals))
		}
	}
	if idrCount != 5 {
		t.Fatalf("found %d IDR AUs, want 5 (golden capture IDRs at 0/8/16/24/32)", idrCount)
	}
	if s.DroppedAUs.Load() != 0 {
		t.Fatalf("DroppedAUs = %d, want 0 under normal load", s.DroppedAUs.Load())
	}
}

func nalsEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestSenderPSIOffLeavesStreamUntouched verifies the flag actually gates the
// behaviour: with PSI off, a slice AU rides the wire byte-untouched (no
// parameter sets prepended, no re-classification).
func TestSenderPSIOffLeavesStreamUntouched(t *testing.T) {
	sink := &recordingSink{}
	s := newTestSender(t, SenderConfig{PSIReinject: false}, sink)
	nal := []byte{0x41, 1, 2, 3} // non-IDR slice, small → single NALU packet
	if !s.Push(append([]byte{0, 0, 0, 1}, nal...), 1000, false) {
		t.Fatal("push rejected")
	}
	if !waitCond(func() bool { return int(s.SentAUs.Load()) == 1 }, 3*time.Second) {
		t.Fatal("AU not sent")
	}
	pkts := sink.snapshot()
	if len(pkts) != 1 {
		t.Fatalf("got %d packets, want 1 (single NALU)", len(pkts))
	}
	if rtpmedia.NALType(pkts[0].Payload[0]) != rtpmedia.NALTypeSlice {
		t.Fatalf("first payload type %d, want non-IDR slice", rtpmedia.NALType(pkts[0].Payload[0]))
	}
	if !bytes.Equal(pkts[0].Payload, nal) {
		t.Fatalf("payload modified with PSI off: %x, want %x", pkts[0].Payload, nal)
	}
}

// TestSenderDropsInvalidAUWhole: encoder output that the packetizer rejects
// (RFC 6184 transport type in the byte stream) is dropped whole and counted,
// never half-sent.
func TestSenderDropsInvalidAUWhole(t *testing.T) {
	sink := &recordingSink{}
	s := newTestSender(t, SenderConfig{}, sink)
	bad := append([]byte{0, 0, 0, 1, 0x1C}, make([]byte, 64)...) // NAL type 28 from "encoder"
	good := []byte{0, 0, 0, 1, 0x41, 1, 2, 3}
	s.Push(bad, 1000, false)
	s.Push(good, 2000, false)
	if !waitCond(func() bool { return int(s.SentAUs.Load()) == 1 }, 3*time.Second) {
		t.Fatal("valid AU after invalid one was not sent")
	}
	if s.DroppedAUs.Load() != 1 {
		t.Fatalf("DroppedAUs = %d, want 1", s.DroppedAUs.Load())
	}
	if n := sink.count(); n != 1 {
		t.Fatalf("sink got %d packets, want exactly the valid AU's single packet", n)
	}
} // TestSenderShaperLimitsBurstRate: with a small bucket/rate, emission is
// visibly stretched and sequence order is preserved (shaping delays, never
// reorders). Uses a real golden IDR AU (≈8 kB → 7 RTP packets incl. FU-A),
// whose fragment payloads actually exceed the token cost that synthetic
// small-NAL tests miss.
func TestSenderShaperLimitsBurstRate(t *testing.T) {
	aus := loadGoldenAUs(t)
	sink := &recordingSink{}

	// 2 Mbps refill, ~12-packet bucket: 10 large AUs (~30 packets) must
	// visibly stall once the initial bucket drains. Wall-clock assertions
	// stay loose (CI jitter); exact math lives in the rtpmedia fake-clock
	// tests.
	s := newTestSender(t, SenderConfig{ShaperKbps: 2000, ShaperBurstK: 96}, sink)
	start := time.Now()
	for i := 0; i < 10; i++ {
		if !s.Push(aus[i].data, int64(i)*33334, aus[i].idr) {
			t.Fatalf("AU %d push rejected", i)
		}
	}
	if !waitCond(func() bool { return int(s.SentAUs.Load()) == 10 }, 5*time.Second) {
		t.Fatalf("sent %d/10 AUs", s.SentAUs.Load())
	}
	elapsed := time.Since(start)
	if elapsed < 20*time.Millisecond {
		t.Fatalf("10 large AUs exited in %v — shaper did not engage", elapsed)
	}
	if elapsed > 2*time.Second {
		t.Fatalf("IDR took %v — shaper over-throttles", elapsed)
	}
	if s.shaper.HeldPackets == 0 {
		t.Fatal("shaper held no packets for a large IDR at 2 Mbps")
	}
	pkts := sink.snapshot()
	for i := 1; i < len(pkts); i++ {
		if pkts[i].SequenceNumber != pkts[i-1].SequenceNumber+1 {
			t.Fatal("shaper reordered packets")
		}
	}
}

// TestSenderSendErrorStopsStream: a dead transport is terminal — the writer
// stops instead of spinning errors through the queue.
func TestSenderSendErrorStopsStream(t *testing.T) {
	sink := &recordingSink{failFrom: 2}
	s := NewSender(nil, SenderConfig{})
	s.SetSink(sink)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 20; i++ {
		s.Push([]byte{0, 0, 0, 1, 0x41, byte(i)}, int64(i)*33334, false)
	}
	if !waitCond(func() bool { return s.SendErrors.Load() >= 1 }, 3*time.Second) {
		t.Fatal("send error never surfaced")
	}
	time.Sleep(100 * time.Millisecond)
	if got := sink.count(); got != 2 {
		t.Fatalf("sink stored %d packets after transport failure, want exactly 2", got)
	}
	s.Stop() // must not hang; writer already exited on the error path
	s.Stop() // idempotent
}

// TestSenderConcurrentStop: Stop from multiple goroutines while frames are
// in flight must be deadlock- and panic-free (-race exercises the locks).
func TestSenderConcurrentStop(t *testing.T) {
	sink := &recordingSink{}
	s := NewSender(nil, SenderConfig{ShaperKbps: 100}) // slow: frames still queued at Stop
	s.SetSink(sink)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		s.Push(make([]byte, 1200), int64(i)*33334, i%30 == 0)
	}
	var wg sync.WaitGroup
	for g := 0; g < 4; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.Stop()
		}()
	}
	wg.Wait()
	if s.SentAUs.Load() > 50 {
		t.Fatal("impossible send count")
	}
}

// TestSenderStopClosesOnlyOwnedQueue: a caller-supplied queue stays open
// after Sender.Stop (the caller owns its lifecycle).
func TestSenderStopClosesOnlyOwnedQueue(t *testing.T) {
	q := rtpmedia.NewQueue(4)
	s := NewSender(q, SenderConfig{})
	s.SetSink(&recordingSink{})
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	s.Stop()
	if q.Closed() {
		t.Fatal("caller-owned queue was closed by Sender.Stop")
	}
	s2 := NewSender(nil, SenderConfig{})
	s2.SetSink(&recordingSink{})
	if err := s2.Start(); err != nil {
		t.Fatal(err)
	}
	s2.Stop()
	if !s2.Queue().Closed() {
		t.Fatal("owned queue not closed by Sender.Stop")
	}
}
