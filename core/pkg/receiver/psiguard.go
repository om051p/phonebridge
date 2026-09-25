package receiver

import (
	"sync"

	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

// PSIGuardSink is a FrameSink decorator that guarantees every IDR it forwards
// is independently decodable whenever valid SPS/PPS has previously been
// received on this session's stream (Phase 6 Slice 3A).
//
// It exists because the wire cannot be trusted to carry parameter sets: the
// phone-side encoder emits SPS/PPS exactly once per codec lifetime (the CSD
// AU), and the production signaling path rebuilds the transport around every
// offer/reconnect, so a stream can arrive with IDRs that lack parameter sets.
// The guard learns SPS/PPS from any AU that carries them (in-band repetition,
// the CSD AU, or a fixed sender) and prepends the cached copies to IDRs that
// lack them — the receiver-side counterpart of the sender-side DEC-021
// re-injection, surviving transport rebuilds because it lives with the sink,
// which the session keeps open across reconnects (DEC-022).
//
// It never fabricates parameter sets: an IDR that arrives before any SPS/PPS
// was ever seen is forwarded unchanged and counted in IDRsNoCache, which the
// session surfaces as frames_reason=PARAM_SETS_MISSING.
//
// Lifetime: owned by the session (outermost wrapper of the chosen sink),
// closed exactly once at final teardown — a new session builds a new guard,
// so cache state never leaks across sessions.
type PSIGuardSink struct {
	inner FrameSink
	cache rtpmedia.Cache

	mu         sync.Mutex
	reinjected int64 // AUs whose Data was completed before forwarding
}

// NewPSIGuardSink wraps inner with session-scoped parameter-set recovery.
func NewPSIGuardSink(inner FrameSink) *PSIGuardSink {
	if inner == nil {
		inner = NewNullSink()
	}
	return &PSIGuardSink{inner: inner}
}

// WriteAU learns any parameter sets the AU carries, completes an IDR that
// lacks them when the cache has them, and forwards the result to the inner
// sink. Non-IDR AUs are forwarded unchanged (their parameter sets, if any,
// were learned above).
func (g *PSIGuardSink) WriteAU(au rtpmedia.AccessUnit) error {
	nals := rtpmedia.SplitAnnexB(au.Data)
	completed := g.cache.Prepare(nals)
	if len(completed) != len(nals) {
		// Prepare only ever inserts, so a length change means re-injection
		// happened and returned a fresh slice.
		au.Data = rtpmedia.JoinAnnexB(completed)
		sps, pps := 0, 0
		for _, n := range completed {
			if len(n) == 0 {
				continue
			}
			switch rtpmedia.NALType(n[0]) {
			case rtpmedia.NALTypeSPS:
				sps++
			case rtpmedia.NALTypePPS:
				pps++
			}
		}
		au.SPSCount, au.PPSCount = sps, pps
		g.mu.Lock()
		g.reinjected++
		g.mu.Unlock()
	}
	return g.inner.WriteAU(au)
}

// Close closes the inner sink (the session closes the chain exactly once).
func (g *PSIGuardSink) Close() error {
	return g.inner.Close()
}

// Inner returns the wrapped sink (used to reach the frame tap beneath the
// guard when composing snapshot diagnostics).
func (g *PSIGuardSink) Inner() FrameSink { return g.inner }

// Stats returns the parameter-set cache counters for this session.
func (g *PSIGuardSink) Stats() rtpmedia.CacheStats { return g.cache.Stats() }

// Reinjected reports how many AUs were completed before forwarding.
func (g *PSIGuardSink) Reinjected() int64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.reinjected
}

// NeedsParamSets reports the diagnosed condition: IDRs were forwarded that
// could not be completed because no SPS/PPS was ever received on this
// session. True means the stream is undecodable as received — the typed
// frames_reason=PARAM_SETS_MISSING condition.
func (g *PSIGuardSink) NeedsParamSets() bool {
	st := g.cache.Stats()
	return !st.HaveSPSPP && st.IDRsNoCache > 0
}
