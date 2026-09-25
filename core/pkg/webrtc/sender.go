package webrtc

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/rtp"

	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

// Sender is the production AU pipeline between the Kotlin capture path and a
// Pion peer connection (DEC-021): Queue → PSI → packetization → shaping →
// per-packet RTP emission by a single writer.
//
// Shaper placement (R1, resolved against pion/webrtc v4.2.20 source):
// TrackLocalStaticRTP.WriteRTP overwrites SSRC and PayloadType from the
// negotiated binding but passes SequenceNumber, Timestamp, Marker and payload
// through verbatim, and does no pacing, reordering or repacketization.
// Shaping per RTP packet immediately before WriteRTP therefore preserves
// ordering, timestamps, markers and fragmentation by construction WITHOUT
// taking ownership of SSRC/PT away from Pion. Sample-level WriteSample was
// rejected: Pion's H264Payloader repacketizes there (STAP-A aggregation, its
// own marker and fragmentation rules), which would discard the DEC-021
// packetization semantics (1200 B FU-A budget, marker-on-last-AU-packet,
// PSI-prepended IDRs).
//
// Concurrency: Push may be called from the capture goroutine; Start spawns
// the single writer; Stop is safe from any goroutine and concurrent with
// Push. A Sender is single-use: Start once, Stop once.
type Sender struct {
	queue    *rtpmedia.Queue
	cache    *rtpmedia.Cache
	pkt      *rtpmedia.Packetizer
	shaper   *rtpmedia.Shaper
	sink     Sink
	ownQueue bool
	psi      bool

	seq     atomic.Uint32 // per-sender sequence space (Pion owns SSRC/PT only)
	started bool

	wg     sync.WaitGroup
	cancel context.CancelFunc
	stopMu sync.Mutex

	// Counters (atomics: Push and stats readers are concurrent).
	PushedAUs      atomic.Int64
	DroppedAUs     atomic.Int64 // queue drop policy or packetizer rejection
	SentAUs        atomic.Int64
	SentPackets    atomic.Int64
	SentBytes      atomic.Int64 // RTP payload bytes (header excluded)
	ReinjectedIDRs atomic.Int64 // PSI completions (mirror of cache counter)
	PSIInBandIDRs  atomic.Int64
	SendErrors     atomic.Int64 // sink write failures (transport lost)
	MaxSendLatency atomic.Int64 // worst push→last-packet-sent latency, ns
}

// SenderConfig configures NewSender. Zero fields use the DEC-021 measured
// defaults.
type SenderConfig struct {
	QueueDepth   int  // 0 → 256 (spike-validated)
	ShaperKbps   int  // 0 → 4000; negative disables shaping
	ShaperBurstK int  // 0 → 3000
	PSIReinject  bool // re-inject cached SPS/PPS ahead of forwarded IDRs
	// PSICache overrides the per-sender parameter-set cache. The phone-side
	// transport passes a cache it owns and learns at AU admission, so
	// parameter sets survive the offer-time transport rebuild (Phase 6 Slice
	// 3A): the encoder emits SPS/PPS exactly once per codec lifetime, and a
	// per-Sender cache would be empty after every mediaRelease/mediaInit.
	PSICache *rtpmedia.Cache
}

// Sink is the RTP emission point. In production it is the bound
// *webrtc.TrackLocalStaticRTP (Session exposes it after the offer/answer
// exchange); tests use in-memory recorders. Emit is called from the single
// writer goroutine only.
type Sink interface {
	Emit(pkt rtp.Packet) error
}

// SinkFunc adapts a function to Sink.
type SinkFunc func(rtp.Packet) error

// Emit implements Sink.
func (f SinkFunc) Emit(pkt rtp.Packet) error { return f(pkt) }

// NewSender builds a sender pipeline. If q is nil a queue is created from
// the config depth; when the caller supplies the queue it stays owner of its
// lifecycle (Sender.Stop then only stops the loop).
func NewSender(q *rtpmedia.Queue, cfg SenderConfig) *Sender {
	ownQueue := q == nil
	if q == nil {
		q = rtpmedia.NewQueue(cfg.QueueDepth)
	}
	cache := cfg.PSICache
	if cache == nil {
		cache = &rtpmedia.Cache{}
	}
	return &Sender{
		queue:    q,
		cache:    cache,
		pkt:      rtpmedia.NewPacketizer(0), // 1200 B budget (DEC-021)
		shaper:   rtpmedia.NewShaper(rtpmedia.ShaperConfig{Kbps: cfg.ShaperKbps, BurstKbit: cfg.ShaperBurstK}),
		ownQueue: ownQueue,
		psi:      cfg.PSIReinject,
	}
}

// Queue exposes the AU queue for the capture-side producer (Push only).
func (s *Sender) Queue() *rtpmedia.Queue { return s.queue }

// Cache exposes the PSI cache (CSD learning, stats).
func (s *Sender) Cache() *rtpmedia.Cache { return s.cache }

// SetSink installs the emission point. It must be called before Start and
// not concurrently with streaming.
func (s *Sender) SetSink(sn Sink) { s.sink = sn }

// Push enqueues one access unit (hot path from JNI). It never blocks and
// returns false when the queue's drop policy rejected the frame or the
// sender is stopped. The Annex-B AU buffer is handed over ownership: the
// caller must not reuse it after Push returns.
func (s *Sender) Push(au []byte, ptsUs int64, key bool) bool {
	ok := s.queue.Push(rtpmedia.Frame{Data: au, PTSUs: ptsUs, Key: key, PushNs: time.Now().UnixNano()})
	if !ok {
		s.DroppedAUs.Add(1)
		return false
	}
	s.PushedAUs.Add(1)
	return true
}

// Start launches the single-writer send loop. The initial sequence number is
// drawn from the host clock (RFC 3550 recommends a random start to make
// inter-stream collision unlikely; the transport-relevant property is
// monotonicity within the stream, which the atomic guarantees).
func (s *Sender) Start() error {
	if s.sink == nil {
		return errors.New("webrtc: sender started without a sink")
	}
	s.stopMu.Lock()
	defer s.stopMu.Unlock()
	if s.started {
		return errors.New("webrtc: sender already started")
	}
	s.started = true
	s.seq.Store(uint32(time.Now().UnixNano()&0x7fffffff) | 1)
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.wg.Add(1)
	go s.sendLoop(ctx)
	return nil
}

// sendLoop is the single writer: dequeue → PSI → packetize → shape → emit.
// Exactly one goroutine runs this, so the shaper's blocking wait delays
// packets without reordering (DEC-021 obligation 1 by construction).
func (s *Sender) sendLoop(ctx context.Context) {
	defer s.wg.Done()
	for {
		f, ok := s.queue.PopWait(ctx)
		if !ok {
			return
		}
		s.process(f)
	}
}

// process runs one AU through PSI → packetizer → shaper → sink.
func (s *Sender) process(f rtpmedia.Frame) {
	var nals [][]byte
	if s.psi {
		nals = s.cache.Prepare(rtpmedia.SplitAnnexB(f.Data))
		s.ReinjectedIDRs.Store(s.cache.InjectedIDRs)
		s.PSIInBandIDRs.Store(s.cache.InBandIDRs)
	} else {
		nals = rtpmedia.SplitAnnexB(f.Data)
	}

	pkts, err := s.pkt.PacketizeAU(nals)
	if err != nil {
		// Invalid encoder output must not poison the transport: drop the AU
		// whole (a half-sent AU is undecodable anyway) and count it.
		s.DroppedAUs.Add(1)
		return
	}
	ts := rtpmedia.RTPTimestamp(f.PTSUs)
	for i := range pkts {
		p := rtp.Packet{
			Header: rtp.Header{
				Version:        2,
				SequenceNumber: uint16(s.seq.Add(1) - 1),
				Timestamp:      ts,
				Marker:         pkts[i].Marker,
			},
			Payload: pkts[i].Payload,
		}
		// Per-packet shaping at the emission point (see type doc). The wait
		// happens BEFORE the sink write; ordering and header semantics are
		// untouched. The 12 B header estimate mirrors the spike's accounting.
		s.shaper.Wait(len(p.Payload) + 12)
		if err := s.sink.Emit(p); err != nil {
			// Transport failure mid-AU: stop sending this AU (a partially sent
			// AU is undecodable anyway) and surface the failure to stats.
			s.SendErrors.Add(1)
			return
		}
		s.SentPackets.Add(1)
		s.SentBytes.Add(int64(len(p.Payload)))
	}
	s.SentAUs.Add(1)
	if f.PushNs != 0 {
		if us := (time.Now().UnixNano() - f.PushNs) / 1e3; us > 0 {
			for {
				m := s.MaxSendLatency.Load()
				if us <= m || s.MaxSendLatency.CompareAndSwap(m, us) {
					break
				}
			}
		}
	}
}

// Stop cancels the send loop, waits for it to exit and — when the Sender
// created its own queue — closes it. A caller-supplied queue stays open and
// remains the caller's lifecycle. Safe from any goroutine, concurrent with
// Push, and idempotent. Frames still queued at Stop time are discarded — a
// stopped session owes nothing (DEC-020/021 teardown semantics).
func (s *Sender) Stop() {
	s.stopMu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.stopMu.Unlock()
	if s.ownQueue {
		s.queue.Close()
	}
	s.wg.Wait()
}
