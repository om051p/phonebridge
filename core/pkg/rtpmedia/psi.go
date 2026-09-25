package rtpmedia

import "sync"

// H.264 parameter-set (SPS/PPS) CSD cache and IDR re-injection
// (DEC-021 obligation 2; Spike 04 BURST-AND-PSI.md).
//
// Measured on the validated device: the encoder emits parameter sets exactly
// once per stream (in the CSD access unit; 1 of 57,464 AUs carried them), so
// a receiver that joins late, reconnects or loses the stream-start AU can
// never decode. The transport obligation is to cache the parameter sets and
// prepend them to every forwarded IDR that lacks them, making every IDR an
// independently decodable random-access point (≈0.3% overhead at 2.5 Mbps;
// SPS 18 B / PPS 5 B on SM7475).
//
// Concurrency: Prepare is safe for concurrent use (the queue consumer and a
// stats/control goroutine may both call it); the hot path takes one mutex.

// Cache holds the cached H.264 parameter sets (start codes stripped).
type Cache struct {
	mu        sync.Mutex
	sps, pps  []byte
	haveSPSPP bool

	// Counters.
	CachedSPS    int64 // bytes cached (0 = never cached)
	CachedPPS    int64 // bytes cached (0 = never cached)
	SPSUpdates   int64 // mid-stream SPS replacements (initial population not counted)
	PPSUpdates   int64 // mid-stream PPS replacements (initial population not counted)
	InjectedIDRs int64 // IDRs that got parameter sets prepended
	InBandIDRs   int64 // IDRs that already carried complete SPS+PPS
	IDRsNoCache  int64 // IDRs seen before any parameter set was cached
}

// HasParameterSets reports whether both an SPS and a PPS have been cached.
func (c *Cache) HasParameterSets() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.haveSPSPP
}

// SPS returns a copy of the cached SPS (nil if none).
func (c *Cache) SPS() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.sps == nil {
		return nil
	}
	return append([]byte(nil), c.sps...)
}

// PPS returns a copy of the cached PPS (nil if none).
func (c *Cache) PPS() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pps == nil {
		return nil
	}
	return append([]byte(nil), c.pps...)
}

// Prepare classifies one access unit and, when needed, completes it:
//
//   - SPS (type 7) / PPS (type 8) NALs passing through are cached (any
//     source: the CSD AU or in-band repetition; changed bytes replace the
//     cache — resolution changes come with new parameter sets);
//   - when the AU carries an IDR slice and already has SPS or PPS itself, it
//     is returned untouched (in-band parameter sets are never duplicated);
//   - when the AU carries an IDR and has neither, and both are cached, the
//     cached copies are prepended;
//   - an IDR before any parameter set was cached cannot be completed — it is
//     returned untouched and counted in IDRsNoCache (surfaced to stats so
//     operators see undecodable-random-access risk).
//
// The input NALs must be start-code-stripped (see SplitAnnexB). The returned
// slice may alias the input when no re-injection happened; when it does
// inject, it returns a fresh slice and never mutates the input.
//
// Boundary: an AU that already carries SPS+PPS is passed through untouched even
// if those sets are implausible (they are the producer's bytes, not ours, and a
// complete-pair AU is never duplicated). Only the *cached* sets — the ones this
// cache would inject — are validity-checked (see learnLocked).
func (c *Cache) Prepare(nals [][]byte) [][]byte {
	cl := classify(nals)

	c.mu.Lock()
	defer c.mu.Unlock()

	// Update the cache from whatever passes through.
	c.learnLocked(cl, nals)

	// Non-IDR AUs pass through unchanged (their parameter sets, if any, were
	// learned above).
	if !cl.hasIDR {
		return nals
	}
	// IDR AU with complete in-band parameter sets: never duplicated.
	if cl.hasSPS && cl.hasPPS {
		c.InBandIDRs++
		return nals
	}
	if !c.haveSPSPP {
		// Nothing to complete from: this IDR cannot be made independently
		// decodable. Counted so operators see the random-access gap.
		c.IDRsNoCache++
		return nals
	}
	// Complete the AU with the missing parameter sets. Insertion preserves
	// canonical order in every emitted stream: the cached SPS (when missing)
	// goes first; the cached PPS goes after any in-band leading parameter
	// sets/SEI and before the first slice NAL — SPS always precedes PPS
	// (decoders discard a PPS whose SPS has not been seen).
	ins := 0
	for i, n := range nals {
		if len(n) == 0 {
			continue
		}
		t := NALType(n[0])
		if t == NALTypeSPS || t == NALTypePPS || t == NALTypeSEI {
			ins = i + 1
		} else {
			break
		}
	}
	out := make([][]byte, 0, len(nals)+2)
	if !cl.hasSPS {
		out = append(out, c.sps)
	}
	out = append(out, nals[:ins]...)
	if !cl.hasPPS {
		out = append(out, c.pps)
	}
	out = append(out, nals[ins:]...)
	c.InjectedIDRs++
	return out
}

// Learn absorbs any parameter sets carried by nals without classifying the AU
// for completion and without touching the IDR counters — the admission-side
// counterpart of Prepare. It exists so a producer (the phone-side transport)
// can capture SPS/PPS the moment an AU is admitted, even if that AU's queue
// is later discarded before send (the CSD AU is emitted exactly once per
// codec lifetime; see the Slice 3A PSI root cause).
//
// It is safe to call in any transport state (it touches the cache only) and it
// is idempotent: re-learning identical bytes changes no counter. Only sets that
// pass the plausibility bounds are cached (see learnLocked), and completeness
// (HasParameterSets) requires both a cached SPS and a cached PPS.
func (c *Cache) Learn(nals [][]byte) {
	cl := classify(nals)
	if !cl.hasSPS && !cl.hasPPS {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.learnLocked(cl, nals)
}

// learnLocked performs the SPS/PPS caching pass. Caller holds c.mu.
//
// Corrupt/truncated parameter sets are refused rather than cached: they would
// otherwise be prepended to every later IDR, turning a typed
// PARAM_SETS_MISSING diagnosis into a stream that is silently undecodable.
// The bounds are the syntactic minimums of ITU-T H.264 §7.3.2.1 (an SPS
// carries profile_idc, constraint flags and level_idc before any ue(v) field —
// never under 4 bytes) and §7.3.2.2 (a PPS carries at least one ue(v) id pair
// plus the rbsp trailing bits — never under 2 bytes). Real encoder sets are far
// larger (SM7475: SPS 18 B / PPS 5 B, DEC-021).
func (c *Cache) learnLocked(cl auClass, nals [][]byte) {
	const minSPSBytes, minPPSBytes = 4, 2
	if cl.hasSPS || cl.hasPPS {
		for _, n := range nals {
			if len(n) == 0 {
				continue
			}
			switch NALType(n[0]) {
			case NALTypeSPS:
				if len(n) < minSPSBytes {
					continue // truncated/corrupt: never cached, never re-injected
				}
				if c.sps == nil {
					c.sps = append([]byte(nil), n...)
					c.CachedSPS = int64(len(c.sps))
				} else if !equalBytes(c.sps, n) {
					c.sps = append([]byte(nil), n...)
					c.CachedSPS = int64(len(c.sps))
					c.SPSUpdates++ // mid-stream change (e.g. resolution switch)
				}
			case NALTypePPS:
				if len(n) < minPPSBytes {
					continue // truncated/corrupt: never cached, never re-injected
				}
				if c.pps == nil {
					c.pps = append([]byte(nil), n...)
					c.CachedPPS = int64(len(c.pps))
				} else if !equalBytes(c.pps, n) {
					c.pps = append([]byte(nil), n...)
					c.CachedPPS = int64(len(c.pps))
					c.PPSUpdates++ // mid-stream change
				}
			}
		}
	}
	// Completeness follows what is actually cached, not what the AU claimed:
	// a refused corrupt set must never make the cache look usable, and a
	// duplicate AU must not change the answer.
	if c.sps != nil && c.pps != nil {
		c.haveSPSPP = true
	}
}

// CacheStats is an atomic snapshot of a Cache's counters (diagnostics).
type CacheStats struct {
	HaveSPSPP    bool
	CachedSPS    int64
	CachedPPS    int64
	SPSUpdates   int64
	PPSUpdates   int64
	InjectedIDRs int64
	InBandIDRs   int64
	IDRsNoCache  int64
}

// Stats returns a consistent snapshot of the cache counters.
func (c *Cache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{
		HaveSPSPP:    c.haveSPSPP,
		CachedSPS:    c.CachedSPS,
		CachedPPS:    c.CachedPPS,
		SPSUpdates:   c.SPSUpdates,
		PPSUpdates:   c.PPSUpdates,
		InjectedIDRs: c.InjectedIDRs,
		InBandIDRs:   c.InBandIDRs,
		IDRsNoCache:  c.IDRsNoCache,
	}
}

func equalBytes(a, b []byte) bool {
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
