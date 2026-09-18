// Package main — Spike 04 Android Go/Pion c-shared bridge (EXPERIMENTAL).
//
// In-process JNI boundary (DEC-019 pattern) between the Kotlin capture layer
// and a Go engine running Pion WebRTC. Built with -buildmode=c-shared into
// libphonebridge_spike04.so. Isolated spike code, not production.
//
// Kotlin calls (class dev.phonebridge.spike04.GoBridge):
//   nativeStart()                 — create the engine + peer connection
//   nativeCreateOffer() -> sdp    — non-trickle offer with all ICE candidates
//   nativeSetRemoteAnswer(sdp)    — apply the receiver's answer
//   nativeOffer(offerSdp) -> sdp  — ALTERNATE direction: Android answers
//   nativeOnFrame(ptsUs, au, key) — push one access unit: Annex-B bytes, CSD
//                                   (SPS/PPS) concatenated ahead of the first IDR
//   nativeStats() -> json         — counters (push/drop/send/queue/latency)
//   nativeStop()                  — close peer connection, stop goroutines
//
// Engine: bounded queue (drop policy per DEC-020) -> single-writer send loop
// -> NAL split (Annex-B stripped) -> RFC 6184 packetization (single NALU or
// FU-A) -> RTP (90 kHz timestamps straight from MediaCodec PTS) -> DTLS/ICE.
package main

/*
#include <jni.h>
#include <stdlib.h>

#if defined(__ANDROID__)
#include <android/log.h>
static void goLog(const char *s) {
    __android_log_print(ANDROID_LOG_INFO, "Spike04Go", "%s", s);
}
#else
#include <stdio.h>
static void goLog(const char *s) {
    fprintf(stderr, "Spike04Go: %s\n", s);
}
#endif

static int isNull(jobject obj) { return obj == NULL ? 1 : 0; }

static jbyteArray nullByteArray() { return NULL; }

static char* getUTFChars(JNIEnv *env, jstring str) {
    if (!str) return NULL;
    return (char*)(*env)->GetStringUTFChars(env, str, NULL);
}

static void releaseUTFChars(JNIEnv *env, jstring str, const char *chars) {
    if (str && chars) (*env)->ReleaseStringUTFChars(env, str, chars);
}

static jstring newStringUTF(JNIEnv *env, const char *str) {
    if (!str) return NULL;
    return (*env)->NewStringUTF(env, str);
}

static int getByteArrayLen(JNIEnv *env, jbyteArray arr) {
    if (!arr) return 0;
    return (int)(*env)->GetArrayLength(env, arr);
}

static void getByteArray(JNIEnv *env, jbyteArray arr, char *buf, int maxLen) {
    if (!arr || !buf) return;
    int len = (int)(*env)->GetArrayLength(env, arr);
    if (len > maxLen) len = maxLen;
    if (len > 0) (*env)->GetByteArrayRegion(env, arr, 0, len, (jbyte*)buf);
}

static JavaVM* fetchJavaVM(JNIEnv *env) {
    JavaVM *vm = NULL;
    if ((*env)->GetJavaVM(env, &vm) != 0) return NULL;
    return vm;
}

static void throwIllegalState(JNIEnv *env, const char *msg) {
    if (!env || !msg) return;
    jclass cls = (*env)->FindClass(env, "java/lang/IllegalStateException");
    if (cls != NULL) (*env)->ThrowNew(env, cls, msg);
}
*/
import "C"

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/pion/interceptor"
	"github.com/pion/rtp"
	"github.com/pion/webrtc/v4"
)

func goLog(s string) { c := C.CString(s); C.goLog(c); C.free(unsafe.Pointer(c)) }

var (
	mu     sync.Mutex
	engine *engineCore
)

// counters — atomics so nativeOnFrame stays lock-light on the hot path
var (
	nPushed        atomic.Int64 // access units handed over JNI
	nKeyframes     atomic.Int64 // access units with SPS/PPS (we mark first AU) — refined below
	nSentAUs       atomic.Int64
	nSentPackets   atomic.Int64
	nSentBytes     atomic.Int64
	nNALUs         atomic.Int64 // NAL units split out of pushed AUs
	nDroppedNonKey atomic.Int64
	nDroppedKey    atomic.Int64
	nRTPPackets    atomic.Int64 // RTP packets marshaled
	nPanics        atomic.Int64
	firstHostNs    atomic.Int64 // host clock at first push (session anchor)
	lastPtsUs      atomic.Int64
	maxQDepth      atomic.Int64
	maxFrameBytes  atomic.Int64
	maxPushSendUs  atomic.Int64 // worst (push -> RTP written) latency
	capacityFrames atomic.Int64

	// burst shaper evidence
	shapeMaxDelayUs  atomic.Int64 // worst single-packet wait introduced by the shaper
	shapeHeldPackets atomic.Int64 // packets that had to wait for tokens (>0 wait)
	shapeHeldNs      atomic.Int64 // total wait time, for mean-hold

	// parameter-set (SPS/PPS) re-injection evidence
	psiCachedSPS  atomic.Int64 // bytes cached from CSD (0 = never cached)
	psiCachedPPS  atomic.Int64
	psiInjected   atomic.Int64 // IDRs that got parameter sets prepended
	psiSkipStart  atomic.Int64 // IDRs that already carried SPS/PPS (in-band)
)

const (
	queueCap      = 256 // bounded queue: ~256 access units
	rtpMaxPayload = 1200
	ptH264        = 96
)

// ---- burst shaper (spike-local experiment) ----------------------------------
//
// A token bucket applied PER RTP PACKET inside the single-writer send loop, so
// AU ordering, RTP timestamps, marker bits, FU-A fragmentation and IDR
// integrity are all preserved by construction: nothing is reordered or split
// differently — packets are only *delayed*.
//
// Configuration comes from Kotlin via nativeStart(kbps, burstK, psi):
//
//   kbps   sustained send rate in kbps (0 = disabled)
//   burstK bucket depth in kbit (default 3000 ≈ 0.75 s of headroom at 4 Mbps)
//
// The 4 Mbps ceiling is measurement-derived: the worst observed wake burst was
// ≈5.9× steady (352 pkt/s × ~1160 B ≈ 4.1 Mbps), so anything lower would
// deliberately throttle normal IDR-carrying seconds too.
const (
	shapeDefaultKbps   = 4000
	shapeDefaultBurstK = 3000
)

// Set from JNI before the engine is created (see nativeStart).
var (
	goShapeKbpsSetting   = 0
	goShapeBurstSetting  = shapeDefaultBurstK
	goPsiReinjectSetting = true
)

// tokenBucket: one packet's worth of tokens (pktBytes*8) per emit.
// Refill rate = shapeKbps; capacity = shapeBurstK kbit. Added cost of a packet
// that waited is measured as shapeDelayNs for the evidence tables.
type tokenBucket struct {
	mu         sync.Mutex
	capBits    int64
	tokens     int64
	refillBits int64 // bits per second
	last       time.Time
	enabled    bool
}

// maxIdleCredit bounds how much idle time counts as credit (500 ms). Without
// it, tokens accumulated during a 10 s screen-off would release the wake burst
// at full line rate — the precise scenario the shaper exists for.
const maxIdleCredit = 500 * time.Millisecond

func newTokenBucket(kbps, burstK int) *tokenBucket {
	if kbps <= 0 {
		return &tokenBucket{enabled: false}
	}
	capBits := int64(burstK) * 1000
	if capBits <= 0 {
		capBits = shapeDefaultBurstK * 1000
	}
	return &tokenBucket{
		capBits:    capBits,
		tokens:     capBits, // start full
		refillBits: int64(kbps) * 1000,
		last:       time.Now(),
		enabled:    true,
	}
}

// wait returns how long the caller had to wait for the packet's tokens.
func (tb *tokenBucket) wait(nBytes int) time.Duration {
	if tb == nil || !tb.enabled {
		return 0
	}
	start := time.Now()
	need := int64(nBytes) * 8
	tb.mu.Lock()
	defer tb.mu.Unlock()
	for {
		now := time.Now()
		elapsed := now.Sub(tb.last)
		tb.last = now
		// Refill capped at capacity. Uncapped accumulation (e.g. a screen-off
		// silence) would let a single packet consume idle time as instant credit,
		// defeating the burst cap exactly when it matters.
		if elapsed > maxIdleCredit {
			elapsed = maxIdleCredit
		}
		tb.tokens += elapsed.Nanoseconds() * tb.refillBits / 1e9
		if tb.tokens > tb.capBits {
			tb.tokens = tb.capBits
		}
		if tb.tokens >= need {
			tb.tokens -= need
			return time.Since(start)
		}
		deficit := need - tb.tokens
		sleepNs := deficit * 1e9 / tb.refillBits
		tb.mu.Unlock()
		time.Sleep(time.Duration(sleepNs))
		tb.mu.Lock()
	}
}

type frame struct {
	data   []byte
	ptsUs  int64
	key    bool
	pushNs int64 // host clock at JNI ingestion (for push->send latency)
}

// bounded queue with the DEC-020 drop policy:
//   - non-key frame on a full queue -> dropped (incoming is cheapest to lose)
//   - key frame on a full queue    -> drop-oldest until it fits (never stall on I-frames)
type frameQueue struct {
	mu    sync.Mutex
	items []frame
	depth atomic.Int64
}

func (q *frameQueue) push(f frame) {
	q.mu.Lock()
	if len(q.items) >= queueCap {
		if f.key {
			for len(q.items) >= queueCap {
				drop := q.items[0]
				q.items = q.items[1:]
				if drop.key {
					nDroppedKey.Add(1)
				} else {
					nDroppedNonKey.Add(1)
				}
			}
			q.items = append(q.items, f)
		} else {
			q.mu.Unlock()
			nDroppedNonKey.Add(1)
			q.depth.Store(int64(len(q.items)))
			return
		}
	} else {
		q.items = append(q.items, f)
	}
	d := int64(len(q.items))
	q.mu.Unlock()
	q.depth.Store(d)
	for {
		m := maxQDepth.Load()
		if d <= m || maxQDepth.CompareAndSwap(m, d) {
			break
		}
	}
}

func (q *frameQueue) pop() (frame, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if len(q.items) == 0 {
		return frame{}, false
	}
	f := q.items[0]
	q.items = q.items[1:]
	q.depth.Store(int64(len(q.items)))
	return f, true
}

type engineCore struct {
	pc     *webrtc.PeerConnection
	tr     *webrtc.TrackLocalStaticRTP
	dc     *webrtc.DataChannel
	queue  frameQueue
	cancel context.CancelFunc
	wg     sync.WaitGroup

	ssrc atomic.Uint32
	pt   atomic.Uint32
	seq  atomic.Uint32

	shaper    *tokenBucket
	psi       psiState
	psiActive bool
}

func newEngine() (*engineCore, error) {
	m := &webrtc.SettingEngine{}
	// Pin the local UDP port range so the phone sits behind one predictable socket.
	_ = m.SetEphemeralUDPPortRange(40000, 40100)

	api := webrtc.NewAPI(
		webrtc.WithSettingEngine(*m),
		webrtc.WithInterceptorRegistry(&interceptor.Registry{}), // bare send: no NACK/TWCC buffering of pre-encoded RTP
	)
	pc, err := api.NewPeerConnection(webrtc.Configuration{ICEServers: []webrtc.ICEServer{}})
	if err != nil {
		return nil, fmt.Errorf("peerconnection: %w", err)
	}
	codec := webrtc.RTPCodecCapability{
		MimeType:    webrtc.MimeTypeH264,
		ClockRate:   90000,
		SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
	}
	lt, errTrack := webrtc.NewTrackLocalStaticRTP(codec, "spike04-video", "spike04")
	if errTrack != nil {
		pc.Close()
		return nil, fmt.Errorf("newtrack: %w", errTrack)
	}
	tr, errTrack := pc.AddTrack(lt)
	if errTrack != nil {
		pc.Close()
		return nil, fmt.Errorf("addtrack: %w", errTrack)
	}
	_ = tr
	ordered := true
	maxRetransmits := uint16(0)
	dc, err := pc.CreateDataChannel("control", &webrtc.DataChannelInit{Ordered: &ordered, MaxRetransmits: &maxRetransmits})
	if err != nil {
		pc.Close()
		return nil, fmt.Errorf("datachannel: %w", err)
	}
	dc.OnMessage(func(msg webrtc.DataChannelMessage) { handleControl(dc, msg.Data) })
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		goLog("ice state: " + s.String())
	})
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		goLog("pc state: " + s.String())
	})

	ctx, cancel := context.WithCancel(context.Background())
	e := &engineCore{pc: pc, tr: lt, dc: dc, cancel: cancel}
	e.ssrc.Store(0x53040401)
	e.pt.Store(ptH264)
	e.seq.Store(uint32(time.Now().UnixNano() & 0x7fff))
	capacityFrames.Store(queueCap)
	e.shaper = newTokenBucket(goShapeKbpsSetting, goShapeBurstSetting)
	e.psiActive = goPsiReinjectSetting
	if e.psiActive {
		goLog(fmt.Sprintf("psi re-injection enabled (prepend to every forwarded IDR)"))
	}
	if e.shaper.enabled {
		goLog(fmt.Sprintf("burst shaper enabled: %d kbps sustained, %d kbit bucket", goShapeKbpsSetting, goShapeBurstSetting))
	}
	e.wg.Add(1)
	go e.sendLoop(ctx)
	return e, nil
}

// sendLoop drains the bounded queue. In this spike the "pacer" is the bounded
// queue + drop policy + single-writer send loop; the burst property we need
// under a 39.6 Mbps wake burst is: queue fills, excess non-key frames drop,
// key frames always get through, and the Kotlin capture path never blocks.
func (e *engineCore) sendLoop(ctx context.Context) {
	defer e.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		f, ok := e.queue.pop()
		if !ok {
			time.Sleep(2 * time.Millisecond)
			continue
		}
		e.send(f)
	}
}

func (e *engineCore) send(f frame) {
	if us := time.Since(time.Unix(0, f.pushNs)).Microseconds(); us > 0 {
		for {
			m := maxPushSendUs.Load()
			if us <= m || maxPushSendUs.CompareAndSwap(m, us) {
				break
			}
		}
	}
	n := packetizeAccessUnit(e, f.ptsUs, f.data, f.key)
	_ = n
	if firstHostNs.Load() == 0 {
		firstHostNs.CompareAndSwap(0, time.Now().UnixNano())
	}
}

// packetizeAccessUnit splits an Annex-B access unit into NAL units (start
// codes stripped), packetizes each (single NALU, or FU-A when large), and sets
// the RTP marker only on the final packet of the access unit (RFC 6184).
func packetizeAccessUnit(e *engineCore, ptsUs int64, au []byte, key bool) int {
	nals := splitNALs(au)
	if len(nals) == 0 {
		return 0
	}
	// Parameter-set re-injection: a receiver that joins late, reconnects or
	// loses the stream-start AU can never decode — SPS/PPS otherwise appear
	// exactly once (measured: 1 of 57,464 AUs). Prepend the cached parameter
	// sets to every forwarded IDR so each IDR is an independently decodable
	// random-access point. In-band parameter sets are recognised and counted
	// rather than duplicated.
	nals, hadPSI := e.psi.prepare(nals, key)
	if hadPSI {
		psiSkipStart.Add(1)
	}
	nNALUs.Add(int64(len(nals)))
	if key {
		nKeyframes.Add(1)
	}
	ts := uint32(uint64(ptsUs) * 9 / 100) // µs -> 90 kHz
	sent := 0
	total := len(nals)
	for i, nal := range nals {
		lastNAL := i == total-1
		if len(nal) == 0 {
			continue
		}
		if len(nal) <= rtpMaxPayload {
			if e.emit(ts, nal, lastNAL) {
				sent++
			}
			continue
		}
		if (nal[0] & 0x1F) == 0 || (nal[0]&0x1F) > 23 {
			// not a single-NAL-unit type (aggregation/FRAGMENT in input) —
			// unexpected from MediaCodec; pass through in one packet so the
			// receiver diag sees it rather than mangling it.
			if e.emit(ts, nal, lastNAL) {
				sent++
			}
			continue
		}
		// FU-A (RFC 6184 §5.8)
		nri := nal[0] & 0x60
		nalType := nal[0] & 0x1F
		indicator := nri | 28
		first := true
		for off := 1; off < len(nal); {
			chunk := rtpMaxPayload - 2
			if len(nal)-off < chunk {
				chunk = len(nal) - off
			}
			end := off+chunk >= len(nal)
			fuHdr := nalType
			if first {
				fuHdr |= 0x80
			}
			if end {
				fuHdr |= 0x40
			}
			payload := make([]byte, 0, chunk+2)
			payload = append(payload, indicator, fuHdr)
			payload = append(payload, nal[off:off+chunk]...)
			marker := end && lastNAL
			if e.emit(ts, payload, marker) {
				sent++
			}
			first = false
			off += chunk
		}
	}
	nSentAUs.Add(1)
	lastPtsUs.Store(ptsUs)
	return sent
}

// ---------------------------------------------------------------------------
// parameter-set (SPS/PPS) state — see psiState.prepare
// ---------------------------------------------------------------------------

type psiState struct {
	mu          sync.Mutex
	sps, pps    []byte
	haveSPSPPS  bool
}

// prepare classifies and, when enabled, completes the NAL list of one AU.
//   - learns SPS (type 7) / PPS (type 8) whenever they pass through (CSD AU or
//     any in-band repetition),
//   - when reinject is on and the AU carries an IDR slice (type 5) without its
//     own SPS/PPS, prepends the cached copies so the IDR decodes standalone,
//   - returns the (possibly new) NAL slice and whether the AU already carried
//     parameter sets itself.
func (p *psiState) prepare(nals [][]byte, key bool) ([][]byte, bool) {
	hasSPS, hasPPS, hasIDR := false, false, false
	for _, n := range nals {
		if len(n) == 0 {
			continue
		}
		switch n[0] & 0x1F {
		case 7:
			hasSPS = true
		case 8:
			hasPPS = true
		case 5:
			hasIDR = true
		}
	}
	inBand := hasSPS || hasPPS
	p.mu.Lock()
	if hasSPS && len(nals) > 0 {
		for _, n := range nals {
			if len(n) > 0 && n[0]&0x1F == 7 {
				if p.sps == nil || !bytesEqual(p.sps, n) {
					p.sps = append([]byte(nil), n...)
					psiCachedSPS.Store(int64(len(p.sps)))
				}
			}
		}
	}
	if hasPPS {
		for _, n := range nals {
			if len(n) > 0 && n[0]&0x1F == 8 {
				if p.pps == nil || !bytesEqual(p.pps, n) {
					p.pps = append([]byte(nil), n...)
					psiCachedPPS.Store(int64(len(p.pps)))
				}
			}
		}
	}
	if hasSPS && hasPPS {
		p.haveSPSPPS = true
	}
	if !p.haveSPSPPS || !hasIDR || inBand {
		p.mu.Unlock()
		return nals, inBand
	}
	p.mu.Unlock()
	// Prepend SPS+PPS ahead of the IDR NALs. New slice: nals is shared with the
	// queue's frame buffer — never mutate in place.
	out := make([][]byte, 0, len(nals)+2)
	if p.sps != nil {
		out = append(out, p.sps)
	}
	if p.pps != nil {
		out = append(out, p.pps)
	}
	out = append(out, nals...)
	psiInjected.Add(1)
	return out, false
}

func bytesEqual(a, b []byte) bool {
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

func (e *engineCore) emit(ts uint32, payload []byte, marker bool) bool {
	// Burst shaper: acquire tokens for this packet BEFORE marshaling. Blocking
	// here (single-writer loop) delays without reordering — sequence numbers,
	// timestamps and markers stay exactly as unshaped.
	if held := e.shaper.wait(len(payload) + 12); held > 0 {
		shapeHeldPackets.Add(1)
		us := held.Microseconds()
		shapeHeldNs.Add(int64(held))
		for {
			m := shapeMaxDelayUs.Load()
			if us <= m || shapeMaxDelayUs.CompareAndSwap(m, us) {
				break
			}
		}
	}
	pkt := rtp.Packet{
		Header: rtp.Header{
			Version:        2,
			PayloadType:    uint8(e.pt.Load()),
			SequenceNumber: uint16(e.seq.Add(1) - 1),
			Timestamp:      ts,
			SSRC:           e.ssrc.Load(),
			Marker:         marker,
		},
		Payload: payload,
	}
	nRTPPackets.Add(1)
	if err := e.tr.WriteRTP(&pkt); err == nil {
		nSentPackets.Add(1)
		nSentBytes.Add(int64(pkt.MarshalSize()))
		return true
	}
	return false
}

// splitNALs splits an Annex-B byte stream into NAL units with start codes
// removed. Handles 3-byte (00 00 01) and 4-byte (00 00 00 01) start codes and
// trims trailing zero bytes that belong to the next start code or padding.
func splitNALs(au []byte) [][]byte {
	n := len(au)
	type sc struct{ start, payload int } // start = first 00 of code, payload = first NAL byte
	var positions []sc
	for i := 0; i+2 < n; i++ {
		if au[i] == 0 && au[i+1] == 0 && au[i+2] == 1 {
			start := i
			if i > 0 && au[i-1] == 0 {
				start = i - 1
			}
			positions = append(positions, sc{start, i + 3})
			i += 2
		}
	}
	if len(positions) == 0 {
		return nil
	}
	nals := make([][]byte, 0, len(positions))
	for k, p := range positions {
		end := n
		if k+1 < len(positions) {
			end = positions[k+1].start
		}
		for end > p.payload && au[end-1] == 0 {
			end--
		}
		if end > p.payload {
			nals = append(nals, au[p.payload:end:end])
		}
	}
	return nals
}

func handleControl(dc *webrtc.DataChannel, data []byte) {
	var cmd map[string]any
	if err := json.Unmarshal(data, &cmd); err != nil {
		return
	}
	switch cmd["type"] {
	case "ping":
		reply := map[string]any{
			"type":        "pong",
			"tx_epoch_ms": cmd["tx_epoch_ms"],
			"rx_epoch_ms": time.Now().UnixMilli(),
			"stats":       statsMap(),
		}
		b, _ := json.Marshal(reply)
		_ = dc.SendText(string(b))
	}
}

func statsMap() map[string]any {
	return map[string]any{
		"pushed":           nPushed.Load(),
		"keyframes":        nKeyframes.Load(),
		"sent_aus":         nSentAUs.Load(),
		"sent_packets":     nSentPackets.Load(),
		"sent_bytes":       nSentBytes.Load(),
		"rtp_packets":      nRTPPackets.Load(),
		"nalus":            nNALUs.Load(),
		"dropped_nonkey":   nDroppedNonKey.Load(),
		"dropped_key":      nDroppedKey.Load(),
		"panics":           nPanics.Load(),
		"queue_depth":      engineQueueDepth(),
		"queue_max":        maxQDepth.Load(),
		"queue_cap":        capacityFrames.Load(),
		"max_frame_b":      maxFrameBytes.Load(),
		"max_push2send_us": maxPushSendUs.Load(),
		// shaper + PSI evidence
		"shape_kbps":         goShapeKbpsSetting,
		"shape_burst_kbit":   goShapeBurstSetting,
		"shape_held_packets": shapeHeldPackets.Load(),
		"shape_max_delay_us": shapeMaxDelayUs.Load(),
		"shape_mean_held_us": meanHeldUs(),
		"psi_cached_sps_b":   psiCachedSPS.Load(),
		"psi_cached_pps_b":   psiCachedPPS.Load(),
		"psi_injected_idrs":  psiInjected.Load(),
		"psi_inband_idrs":    psiSkipStart.Load(),
	}
}

func meanHeldUs() int64 {
	n := shapeHeldPackets.Load()
	if n == 0 {
		return 0
	}
	return shapeHeldNs.Load() / n
}

func engineQueueDepth() int64 {
	mu.Lock()
	e := engine
	mu.Unlock()
	if e == nil {
		return 0
	}
	return e.queue.depth.Load()
}

func currentEngine() *engineCore {
	mu.Lock()
	defer mu.Unlock()
	return engine
}

func startEngine() error {
	mu.Lock()
	defer mu.Unlock()
	if engine != nil {
		return nil
	}
	e, err := newEngine()
	if err != nil {
		return err
	}
	engine = e
	return nil
}

func offerAnswer(offerSdp string) (string, error) {
	e := currentEngine()
	if e == nil {
		return "", fmt.Errorf("engine not started")
	}
	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSdp}
	if err := e.pc.SetRemoteDescription(offer); err != nil {
		return "", fmt.Errorf("SetRemoteDescription: %w", err)
	}
	answer, err := e.pc.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("CreateAnswer: %w", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(e.pc)
	if err = e.pc.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("SetLocalDescription: %w", err)
	}
	<-gatherComplete // non-trickle: complete answer with all candidates
	return e.pc.LocalDescription().SDP, nil
}

func stopEngine() {
	mu.Lock()
	e := engine
	engine = nil
	mu.Unlock()
	if e == nil {
		return
	}
	e.cancel()
	e.wg.Wait()
	_ = e.pc.Close()
	goLog("engine stopped")
}

// createOfferSdp makes Android the SDP offerer (the phone knows when capture
// starts; the Linux receiver is a stateless HTTP answerer). Non-trickle: the
// returned SDP contains all ICE candidates.
func createOfferSdp() (string, error) {
	e := currentEngine()
	if e == nil {
		return "", fmt.Errorf("engine not started")
	}
	offer, err := e.pc.CreateOffer(nil)
	if err != nil {
		return "", fmt.Errorf("CreateOffer: %w", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(e.pc)
	if err = e.pc.SetLocalDescription(offer); err != nil {
		return "", fmt.Errorf("SetLocalDescription: %w", err)
	}
	<-gatherComplete // non-trickle: complete offer with all candidates
	return e.pc.LocalDescription().SDP, nil
}

func setRemoteAnswerSdp(answerSdp string) error {
	e := currentEngine()
	if e == nil {
		return fmt.Errorf("engine not started")
	}
	answer := webrtc.SessionDescription{Type: webrtc.SDPTypeAnswer, SDP: answerSdp}
	if err := e.pc.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("SetRemoteDescription: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// JNI exports (package dev.phonebridge.spike04, class GoBridge)
// ---------------------------------------------------------------------------

//export Java_dev_phonebridge_spike04_GoBridge_nativeStart
func Java_dev_phonebridge_spike04_GoBridge_nativeStart(env *C.JNIEnv, clazz C.jclass, jKbps C.jint, jBurstK C.jint, jPsi C.jboolean) C.jboolean {
	_ = clazz
	defer func() {
		if r := recover(); r != nil {
			nPanics.Add(1)
			C.throwIllegalState(env, C.CString(fmt.Sprintf("nativeStart panic: %v", r)))
		}
	}()
	if C.fetchJavaVM(env) == nil {
		C.throwIllegalState(env, C.CString("GetJavaVM failed"))
		return C.JNI_FALSE
	}
	// Settings must be captured BEFORE the engine is created (the engine reads
	// them in newEngine); re-setting them mid-session would race the send loop.
	goShapeKbpsSetting = int(jKbps)
	goShapeBurstSetting = int(jBurstK)
	if goShapeBurstSetting <= 0 {
		goShapeBurstSetting = shapeDefaultBurstK
	}
	goPsiReinjectSetting = jPsi != 0
	if err := startEngine(); err != nil {
		C.throwIllegalState(env, C.CString(err.Error()))
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_spike04_GoBridge_nativeOffer
func Java_dev_phonebridge_spike04_GoBridge_nativeOffer(env *C.JNIEnv, clazz C.jclass, jOfferSdp C.jstring) C.jstring {
	_ = clazz
	defer func() {
		if r := recover(); r != nil {
			nPanics.Add(1)
			C.throwIllegalState(env, C.CString(fmt.Sprintf("nativeOffer panic: %v", r)))
		}
	}()
	c := C.getUTFChars(env, jOfferSdp)
	if c == nil {
		C.throwIllegalState(env, C.CString("offer sdp is null"))
		return C.jstring(0)
	}
	offerSdp := C.GoString(c)
	C.releaseUTFChars(env, jOfferSdp, c)
	answer, err := offerAnswer(offerSdp)
	if err != nil {
		C.throwIllegalState(env, C.CString(err.Error()))
		return C.jstring(0)
	}
	return C.newStringUTF(env, C.CString(answer))
}

//export Java_dev_phonebridge_spike04_GoBridge_nativeCreateOffer
func Java_dev_phonebridge_spike04_GoBridge_nativeCreateOffer(env *C.JNIEnv, clazz C.jclass) C.jstring {
	_ = clazz
	defer func() {
		if r := recover(); r != nil {
			nPanics.Add(1)
			C.throwIllegalState(env, C.CString(fmt.Sprintf("nativeCreateOffer panic: %v", r)))
		}
	}()
	sdp, err := createOfferSdp()
	if err != nil {
		C.throwIllegalState(env, C.CString(err.Error()))
		return C.jstring(0)
	}
	return C.newStringUTF(env, C.CString(sdp))
}

//export Java_dev_phonebridge_spike04_GoBridge_nativeSetRemoteAnswer
func Java_dev_phonebridge_spike04_GoBridge_nativeSetRemoteAnswer(env *C.JNIEnv, clazz C.jclass, jAnswerSdp C.jstring) C.jboolean {
	_ = clazz
	defer func() {
		if r := recover(); r != nil {
			nPanics.Add(1)
			C.throwIllegalState(env, C.CString(fmt.Sprintf("nativeSetRemoteAnswer panic: %v", r)))
		}
	}()
	c := C.getUTFChars(env, jAnswerSdp)
	if c == nil {
		C.throwIllegalState(env, C.CString("answer sdp is null"))
		return C.JNI_FALSE
	}
	answerSdp := C.GoString(c)
	C.releaseUTFChars(env, jAnswerSdp, c)
	if err := setRemoteAnswerSdp(answerSdp); err != nil {
		C.throwIllegalState(env, C.CString(err.Error()))
		return C.JNI_FALSE
	}
	return C.JNI_TRUE
}

//export Java_dev_phonebridge_spike04_GoBridge_nativeOnFrame
func Java_dev_phonebridge_spike04_GoBridge_nativeOnFrame(env *C.JNIEnv, clazz C.jclass, jPtsUs C.jlong, jData C.jbyteArray, jKey C.jboolean) {
	_ = clazz
	_ = env
	defer func() {
		if r := recover(); r != nil {
			nPanics.Add(1)
		}
	}()
	e := currentEngine()
	if e == nil {
		return
	}
	n := C.getByteArrayLen(env, jData)
	if n <= 0 {
		return
	}
	buf := make([]byte, n)
	C.getByteArray(env, jData, (*C.char)(unsafe.Pointer(&buf[0])), n)
	if int64(n) > maxFrameBytes.Load() {
		maxFrameBytes.Store(int64(n))
	}
	f := frame{data: buf, ptsUs: int64(jPtsUs), key: jKey != 0, pushNs: time.Now().UnixNano()}
	nPushed.Add(1)
	e.queue.push(f)
}

//export Java_dev_phonebridge_spike04_GoBridge_nativeStats
func Java_dev_phonebridge_spike04_GoBridge_nativeStats(env *C.JNIEnv, clazz C.jclass) C.jstring {
	_ = clazz
	b, err := json.Marshal(statsMap())
	if err != nil {
		C.throwIllegalState(env, C.CString(err.Error()))
		return C.jstring(0)
	}
	return C.newStringUTF(env, C.CString(string(b)))
}

//export Java_dev_phonebridge_spike04_GoBridge_nativeStop
func Java_dev_phonebridge_spike04_GoBridge_nativeStop(env *C.JNIEnv, clazz C.jclass) C.jboolean {
	_ = clazz
	_ = env
	stopEngine()
	return C.JNI_TRUE
}

func main() {}
