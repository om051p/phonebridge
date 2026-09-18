// spike04-receiver — Linux-side receiver for Spike 04 (EXPERIMENTAL, isolated).
//
// Answers a spike-local signaling exchange, joins ICE/DTLS with the Android
// Pion engine, receives the H.264 track, depacketizes RFC 6184 into Annex-B
// access units, and records end-to-end evidence:
//   - RTP seq continuity / duplicates / late packets
//   - AU rate, kbps, keyframe rate, RTP timestamp monotonicity + deltas
//   - data-channel ping RTT (device RTT only: DTLS+SRTP+UDP, no network RTT)
//   - first access unit dump (framing forensic) + full .h264 capture
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/pion/interceptor"
	"github.com/pion/rtp/codecs"
	"github.com/pion/webrtc/v4"
)

type stats struct {
	packets      atomic.Int64
	bytes        atomic.Int64
	dupSeq       atomic.Int64
	seqGaps      atomic.Int64
	latePackets  atomic.Int64
	fragPackets  atomic.Int64 // FU packets seen (st=28)
	singleNALUs  atomic.Int64 // st 1..23
	nalType5     atomic.Int64
	nalType7     atomic.Int64
	nalType8     atomic.Int64
	accessUnits  atomic.Int64
	lastSeq      atomic.Int64 // -1 = unset
	lastTs       atomic.Int64
	tsBackward   atomic.Int64
	markerAUs    atomic.Int64
	firstRxAUs   atomic.Int64 // host ns of first AU
	lastRxAUs    atomic.Int64
	rttUs        atomic.Int64
	connectedAt  atomic.Int64
	disconnected atomic.Int64
}

var (
	st        stats
	mediaLock sync.Mutex

	h264File      *os.File
	auIdxFile    *os.File // AU-boundary index: one line per access unit
	firstAUDumped bool
	receivedBytes atomic.Int64

	// One-way delay instrumentation. The RTP timestamp is the sender's
	// CLOCK_MONOTONIC in microseconds mapped to 90 kHz with no base reset, so
	// (local_monotonic_us - rtp_ts_us) is a CONSTANT clock offset plus the true
	// one-way delay. The absolute offset cannot be known without synchronised
	// clocks, but the delay *variation* -- and delay above the observed minimum --
	// is measurable exactly. Over a long run, wall-clock drift biases it slowly.
	delayMu       sync.Mutex
	delaySamples  []int64
	minDelayUs    int64
	delayDropped  int64
)

const maxDelaySamples = 400_000

func recDelay(us int64) {
	delayMu.Lock()
	if minDelayUs == 0 || us < minDelayUs {
		minDelayUs = us
	}
	if len(delaySamples) < maxDelaySamples {
		delaySamples = append(delaySamples, us)
	} else {
		delayDropped++
	}
	delayMu.Unlock()
}

// annexBHasIDR reports whether an Annex-B access unit contains an IDR slice
// (nal_unit_type 5). Used to record GOP structure in the AU index so the host
// can reproduce the sender's frame grouping without re-deriving it.
// annexBCountPS counts SPS (7) and PPS (8) NAL units in an Annex-B access unit.
func annexBCountPS(au []byte) (sps, pps int) {
	for _, n := range splitAnnexB(au) {
		if len(n) == 0 {
			continue
		}
		switch n[0] & 0x1F {
		case 7:
			sps++
		case 8:
			pps++
		}
	}
	return sps, pps
}

// splitAnnexB splits an Annex-B byte stream into NAL units (start codes
// stripped). Receiver-side twin of the sender's splitNALs; only used for
// classification, never for re-fragmentation.
func splitAnnexB(au []byte) [][]byte {
	n := len(au)
	type sc struct{ start, payload int }
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

func annexBHasIDR(au []byte) bool {
	for i := 0; i+3 < len(au); i++ {
		var hdr int
		if au[i] == 0 && au[i+1] == 0 && au[i+2] == 1 {
			hdr = i + 3
		} else if i+4 < len(au) && au[i] == 0 && au[i+1] == 0 && au[i+2] == 0 && au[i+3] == 1 {
			hdr = i + 4
		} else {
			continue
		}
		if hdr < len(au) && au[hdr]&0x1f == 5 {
			return true
		}
		i = hdr - 1
	}
	return false
}

func tsDelta(a, b uint32) int64 {
	d := int64(a) - int64(b)
	if d > 1<<31 {
		d -= 1 << 32
	} else if d < -(1 << 31) {
		d += 1 << 32
	}
	return d
}

func main() {
	mode := flag.String("mode", "serve", "serve | file")
	httpAddr := flag.String("http", ":7804", "HTTP signaling address (mode=serve)")
	offerFile := flag.String("in", "", "offer SDP file (mode=file)")
	answerFile := flag.String("out", "", "answer SDP file (mode=file)")
	dump := flag.String("dump", "results/first-au.bin", "first access unit dump")
	h264Path := flag.String("h264", "results/received.h264", "annex-B capture of all received AUs")
	auIdxPath := flag.String("auidx", "", "access-unit index sidecar (one line per AU: idx bytes rtp_ts idr marker sps pps)")
	duration := flag.Int("duration", 0, "seconds before printing final stats and exiting (0 = run forever)")
	flag.Parse()

	if *auIdxPath != "" {
		f, err := os.Create(*auIdxPath)
		if err != nil {
			log.Fatalf("auidx file: %v", err)
		}
		auIdxFile = f
		defer f.Close()
	}

	if *mode == "file" {
		runFile(*offerFile, *answerFile, *dump, *h264Path, *duration)
		return
	}
	runServe(*httpAddr, *dump, *h264Path, *duration)
}

func newPeerConnection() (*webrtc.PeerConnection, *webrtc.TrackLocalStaticRTP) {
	m := &webrtc.SettingEngine{}
	_ = m.SetEphemeralUDPPortRange(50000, 50100)
	api := webrtc.NewAPI(webrtc.WithSettingEngine(*m), webrtc.WithInterceptorRegistry(&interceptor.Registry{}))
	pc, err := api.NewPeerConnection(webrtc.Configuration{ICEServers: []webrtc.ICEServer{}})
	if err != nil {
		log.Fatalf("peerconnection: %v", err)
	}
	codec := webrtc.RTPCodecCapability{
		MimeType:    webrtc.MimeTypeH264,
		ClockRate:   90000,
		SDPFmtpLine: "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f",
	}
	lt, err := webrtc.NewTrackLocalStaticRTP(codec, "spike04-video", "spike04")
	if err != nil {
		log.Fatalf("newtrack: %v", err)
	}
	// AddTrack as a receiver-style remote track placeholder is unnecessary;
	// we accept the sender's track via OnTrack.
	return pc, lt
}

func wire(pc *webrtc.PeerConnection, dump, h264Path string) {
	var dc *webrtc.DataChannel
	pc.OnDataChannel(func(d *webrtc.DataChannel) {
		dc = d
		d.OnMessage(func(msg webrtc.DataChannelMessage) {
			var m map[string]any
			if json.Unmarshal(msg.Data, &m) != nil {
				return
			}
			switch m["type"] {
			case "pong":
				if tx, ok := m["tx_epoch_ms"].(float64); ok {
					st.rttUs.Store((time.Now().UnixMilli()-int64(tx))*1000)
					log.Printf("DC rtt_ms=%.1f phone_stats=%s", float64(st.rttUs.Load())/1000.0, shortJSON(m["stats"]))
				}
			}
		})
	})
	pc.OnICEConnectionStateChange(func(s webrtc.ICEConnectionState) {
		log.Printf("ice state: %s", s)
	})
	pc.OnConnectionStateChange(func(s webrtc.PeerConnectionState) {
		log.Printf("pc state: %s", s)
		if s == webrtc.PeerConnectionStateConnected {
			st.connectedAt.Store(time.Now().UnixNano())
			go pingLoop(dc)
		}
		if s == webrtc.PeerConnectionStateFailed || s == webrtc.PeerConnectionStateClosed {
			st.disconnected.Store(time.Now().UnixNano())
		}
	})
	pc.OnTrack(func(track *webrtc.TrackRemote, _ *webrtc.RTPReceiver) {
		log.Printf("track: %s %s ssrc=%d pt=%d", track.Codec().MimeType, track.Codec().SDPFmtpLine, track.SSRC(), track.PayloadType())
		depacketizer := &codecs.H264Packet{}
		var currentAU []byte
		var currentTs uint32
		f, errH := os.Create(h264Path)
		if errH != nil {
			log.Fatalf("h264 file: %v", errH)
		}
		h264File = f
		defer f.Close()
		// RTP sequence numbers start at a random value per RFC 3550, so the first
		// packet must establish the baseline rather than be compared against 0.
		// (Without this, the first packet is counted as a ~64k sequence gap.)
		st.lastSeq.Store(-1)
		for {
			pkt, _, err := track.ReadRTP()
			if err != nil {
				log.Printf("track read end: %v", err)
				return
			}
			st.packets.Add(1)
			st.bytes.Add(int64(len(pkt.Payload)) + 12)
			// seq continuity
			ls := st.lastSeq.Load()
			if ls >= 0 {
				expect := uint16(ls) + 1
				if pkt.SequenceNumber == expect {
				} else if pkt.SequenceNumber == uint16(ls) {
					st.dupSeq.Add(1)
				} else {
					// gap or reorder; classify by distance
					d := int16(pkt.SequenceNumber - expect)
					if d < 0 {
						st.latePackets.Add(1)
					} else {
						st.seqGaps.Add(int64(d))
					}
				}
			}
			st.lastSeq.Store(int64(pkt.SequenceNumber))
			// ts monotonicity (on AU boundaries ~ marker)
			lt := st.lastTs.Load()
			if lt != 0 {
				if tsDelta(pkt.Timestamp, uint32(lt)) < 0 {
					st.tsBackward.Add(1)
				}
			}
			// fragment/NAL type census
			if len(pkt.Payload) >= 2 && pkt.Payload[0]&0x1F == 28 {
				st.fragPackets.Add(1)
			} else if len(pkt.Payload) >= 1 {
				t := pkt.Payload[0] & 0x1F
				if t >= 1 && t <= 23 {
					st.singleNALUs.Add(1)
					if t == 5 {
						st.nalType5.Add(1)
					}
					if t == 7 {
						st.nalType7.Add(1)
					}
					if t == 8 {
						st.nalType8.Add(1)
					}
				}
			}
			fragment, err := depacketizer.Unmarshal(pkt.Payload)
			if err != nil || len(fragment) == 0 {
				continue
			}
			if pkt.Timestamp != currentTs && len(currentAU) > 0 {
				flushAU(&currentAU, dump, currentTs, false)
			}
			currentTs = pkt.Timestamp
			st.lastTs.Store(int64(pkt.Timestamp))
			currentAU = append(currentAU, fragment...)
			if pkt.Marker {
				flushAU(&currentAU, dump, pkt.Timestamp, true)
			}
		}
	})
}

func flushAU(au *[]byte, dump string, ts uint32, marker bool) {
	if len(*au) == 0 {
		return
	}
	idx := st.accessUnits.Load()
	st.accessUnits.Add(1)
	recDelay(time.Now().UnixNano()/1000 - int64(ts)*100/9)
	if auIdxFile != nil {
		idr := 0
		if annexBHasIDR(*au) {
			idr = 1
		}
		// Count parameter sets per AU so re-injection is verifiable on the wire
		// (idx bytes rtp_ts idr marker sps pps).
		spsN, ppsN := annexBCountPS(*au)
		mk := 0
		if marker {
			mk = 1
		}
		_, _ = fmt.Fprintf(auIdxFile, "%d %d %d %d %d %d %d\n", idx, len(*au), ts, idr, mk, spsN, ppsN)
	}
	if st.firstRxAUs.Load() == 0 {
		st.firstRxAUs.Store(time.Now().UnixNano())
		_ = os.WriteFile(dump, *au, 0o644)
		log.Printf("first AU dumped: %d bytes -> %s", len(*au), dump)
	}
	st.lastRxAUs.Store(time.Now().UnixNano())
	if h264File != nil {
		n, _ := h264File.Write(*au)
		receivedBytes.Add(int64(n))
	}
	*au = (*au)[:0]
}

func pingLoop(dc *webrtc.DataChannel) {
	if dc == nil {
		return
	}
	t := time.NewTicker(5 * time.Second)
	defer t.Stop()
	for range t.C {
		b, _ := json.Marshal(map[string]any{"type": "ping", "tx_epoch_ms": time.Now().UnixMilli()})
		if err := dc.SendText(string(b)); err != nil {
			return
		}
	}
}

func shortJSON(v any) string {
	b, _ := json.Marshal(v)
	if len(b) > 300 {
		b = b[:300]
	}
	return string(b)
}

func printStats() {
	au := st.accessUnits.Load()
	pkt := st.packets.Load()
	secs := 0.0
	if f := st.firstRxAUs.Load(); f != 0 {
		l := st.lastRxAUs.Load()
		if l == 0 {
			l = time.Now().UnixNano()
		}
		secs = float64(l-f) / 1e9
	}
	auRate := 0.0
	kbps := 0.0
	if secs > 0 {
		auRate = float64(au) / secs
		kbps = float64(receivedBytes.Load()*8) / secs / 1000
	}
	out := map[string]any{
		"packets":       pkt,
		"bytes_rtp":     st.bytes.Load(),
		"bytes_h264":    receivedBytes.Load(),
		"access_units":  au,
		"au_per_s":      auRate,
		"kbps_payload":  kbps,
		"single_nalus":  st.singleNALUs.Load(),
		"frag_packets":  st.fragPackets.Load(),
		"nal_idr":       st.nalType5.Load(),
		"nal_sps":       st.nalType7.Load(),
		"nal_pps":       st.nalType8.Load(),
		"dup_seq":       st.dupSeq.Load(),
		"seq_gaps":      st.seqGaps.Load(),
		"late_packets":  st.latePackets.Load(),
		"ts_backward":   st.tsBackward.Load(),
		"rtt_us_dc":     st.rttUs.Load(),
		"duration_s":    secs,
	}

	delayMu.Lock()
	rel := make([]int64, len(delaySamples))
	for i, v := range delaySamples {
		rel[i] = v - minDelayUs
	}
	dropped := delayDropped
	delayMu.Unlock()
	sort.Slice(rel, func(i, j int) bool { return rel[i] < rel[j] })
	n := len(rel)
	pick := func(p float64) int64 {
		if n == 0 {
			return 0
		}
		i := int(p * float64(n-1))
		if i < 0 {
			i = 0
		}
		return rel[i]
	}
	relMax := int64(0)
	if n > 0 {
		relMax = rel[n-1]
	}
	// Relative one-way delay: absolute value needs synchronised clocks; this is
	// "delay above the best observed sample", which does not.
	out["rel_delay_us"] = map[string]any{
		"samples": n, "dropped": dropped,
		"p50": pick(0.50), "p90": pick(0.90), "p99": pick(0.99), "max": relMax,
	}

	b, _ := json.Marshal(out)
	fmt.Println("STATS " + string(b))
}

func buildAnswer(pc *webrtc.PeerConnection, offerSdp string) (string, error) {
	offer := webrtc.SessionDescription{Type: webrtc.SDPTypeOffer, SDP: offerSdp}
	if err := pc.SetRemoteDescription(offer); err != nil {
		return "", fmt.Errorf("SetRemoteDescription: %w", err)
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("CreateAnswer: %w", err)
	}
	gatherComplete := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("SetLocalDescription: %w", err)
	}
	<-gatherComplete
	return pc.LocalDescription().SDP, nil
}

func runServe(httpAddr, dump, h264Path string, duration int) {
	mu := sync.Mutex{}
	var pcRef *webrtc.PeerConnection
	http.HandleFunc("/offer", func(w http.ResponseWriter, r *http.Request) {
		var req struct{ SDP string `json:"sdp"` }
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", 400)
			return
		}
		mu.Lock()
		if pcRef != nil {
			_ = pcRef.Close()
		}
		pc, _ := newPeerConnection()
		wire(pc, dump, h264Path)
		pcRef = pc
		mu.Unlock()
		answer, err := buildAnswer(pc, req.SDP)
		if err != nil {
			http.Error(w, err.Error(), 500)
			log.Printf("answer error: %v", err)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"sdp": answer})
		log.Printf("offer handled (%d bytes)", len(req.SDP))
	})
	go func() {
		log.Printf("signaling listening on %s", httpAddr)
		log.Fatal(http.ListenAndServe(httpAddr, nil))
	}()

	stop := make(chan struct{})
	// SIGTERM/SIGINT flush final stats. A wall-clock budget alone cannot be used
	// for a driver-driven run: consent can take far longer than any fixed budget,
	// so the receiver must outlive it and be told when to finish.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-sigCh
		log.Printf("signal received — flushing stats")
		close(stop)
	}()
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	start := time.Now()
	for {
		select {
		case <-stop:
			printStats()
			return
		case <-t.C:
			el := time.Since(start)
			if duration > 0 && int(el.Seconds()) >= duration {
				printStats()
				return
			}
			// Always emit the per-second health line: it is the only record of
			// 0-fps windows and wake bursts, and a non-zero -duration used to
			// suppress it entirely, losing that evidence.
			log.Printf("t=%.0fs packets=%d aus=%d gaps=%d late=%d", el.Seconds(),
				st.packets.Load(), st.accessUnits.Load(), st.seqGaps.Load(), st.latePackets.Load())
		}
	}
}

func runFile(offerFile, answerFile, dump, h264Path string, duration int) {
	if offerFile == "" || answerFile == "" {
		log.Fatal("mode=file requires -in and -out")
	}
	offerSdp, err := os.ReadFile(offerFile)
	if err != nil {
		log.Fatalf("read offer: %v", err)
	}
	pc, _ := newPeerConnection()
	wire(pc, dump, h264Path)
	answer, err := buildAnswer(pc, string(offerSdp))
	if err != nil {
		log.Fatalf("answer: %v", err)
	}
	if err := os.WriteFile(answerFile, []byte(answer), 0o644); err != nil {
		log.Fatalf("write answer: %v", err)
	}
	log.Printf("answer written to %s — waiting for media", answerFile)
	t := time.NewTicker(1 * time.Second)
	defer t.Stop()
	start := time.Now()
	for range t.C {
		if duration > 0 && int(time.Since(start).Seconds()) >= duration {
			printStats()
			return
		}
		log.Printf("t=%.0fs packets=%d aus=%d gaps=%d late=%d", time.Since(start).Seconds(),
			st.packets.Load(), st.accessUnits.Load(), st.seqGaps.Load(), st.latePackets.Load())
	}
}
