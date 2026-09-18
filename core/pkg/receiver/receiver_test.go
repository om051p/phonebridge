package receiver

import (
	"bufio"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
	"github.com/om051p/phonebridge/core/pkg/webrtc"
)

type sliceAU struct {
	data  []byte
	rtpTS uint32
	idr   bool
}

func loadSliceAUs(t *testing.T, base string) []sliceAU {
	t.Helper()
	blob, err := os.ReadFile(base + ".h264")
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	f, err := os.Open(base + ".idx")
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	defer f.Close()

	var aus []sliceAU
	sc := bufio.NewScanner(f)
	offset := 0
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) < 7 {
			continue
		}
		var r [7]int
		ok := true
		for i := range r {
			v, err := strconv.Atoi(fields[i])
			if err != nil {
				ok = false
				break
			}
			r[i] = v
		}
		if !ok {
			continue
		}
		size := r[1]
		if offset+size > len(blob) {
			t.Fatalf("AU %d: index sizes exceed capture", r[0])
		}
		aus = append(aus, sliceAU{
			data:  blob[offset : offset+size],
			rtpTS: uint32(r[2]),
			idr:   r[3] == 1,
		})
		offset += size
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan index: %v", err)
	}
	return aus
}

func TestReceiverLoopbackGoldenCapture(t *testing.T) {
	aus := loadSliceAUs(t, "../rtpmedia/testdata/spike04-shaped4-1-slice")

	// Determine if ffmpeg is available on the system
	hasFFmpeg := false
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		hasFFmpeg = true
	}

	var verifySink *FFmpegVerifySink
	var sink FrameSink
	if hasFFmpeg {
		var err error
		verifySink, err = NewFFmpegVerifySink()
		if err != nil {
			t.Fatalf("NewFFmpegVerifySink: %v", err)
		}
		sink = verifySink
	} else {
		sink = NewNullSink()
	}

	// 1. Configure production Receiver
	recv, err := NewReceiver(Config{
		IncludeLoopback: true,
		Sink:            sink,
		QueueDepth:      256,
	})
	if err != nil {
		t.Fatalf("NewReceiver: %v", err)
	}
	defer recv.Close()

	// 2. Configure production Sender Session
	sender := webrtc.NewSender(nil, webrtc.SenderConfig{
		PSIReinject: true,
		ShaperKbps:  0, // Disable rate limiter for fast integration test
	})
	sess, err := webrtc.NewSession(webrtc.SessionConfig{
		IncludeLoopback: true,
		PortMin:         45200,
		PortMax:         45299,
	}, sender)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	defer sess.Stop()

	// Seed sender parameter sets from AU 0 (CSD)
	sender.Cache().Prepare(rtpmedia.SplitAnnexB(aus[0].data))

	// 3. Negotiate SDP: Sender offers, Receiver answers
	offer, err := sess.CreateOffer()
	if err != nil {
		t.Fatalf("CreateOffer: %v", err)
	}

	answer, err := recv.SetRemoteOffer(offer)
	if err != nil {
		t.Fatalf("SetRemoteOffer: %v", err)
	}

	if err := sess.SetRemoteAnswer(answer); err != nil {
		t.Fatalf("SetRemoteAnswer: %v", err)
	}

	// 4. Wait for ICE and DTLS connection
	if err := recv.WaitForState(pion.PeerConnectionStateConnected, 5*time.Second); err != nil {
		t.Fatalf("wait for connected: %v", err)
	}

	// 5. Start sender streaming loop (Pion fires OnTrack only after the first RTP packet arrives)
	if err := sess.Start(); err != nil {
		t.Fatalf("sender Start: %v", err)
	}

	// Push golden frames through sender in background
	go func() {
		for _, au := range aus {
			ptsUs := int64(au.rtpTS) * 100 / 9
			if !sender.Push(au.data, ptsUs, au.idr) {
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()

	if err := recv.WaitForTrack(5 * time.Second); err != nil {
		t.Fatalf("wait for track: %v", err)
	}

	// 6. Wait for all frames to be received and processed
	deadline := time.Now().Add(5 * time.Second)
	var finalStats rtpmedia.StreamStats
	var finalDrops int64
	for time.Now().Before(deadline) {
		finalStats, finalDrops = recv.Stats()
		if finalStats.AccessUnits >= int64(len(aus)) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Logf("Receiver stats: %s (dropped=%d)", finalStats, finalDrops)

	if finalStats.AccessUnits < int64(len(aus)) {
		t.Errorf("expected at least %d AUs, got %d", len(aus), finalStats.AccessUnits)
	}
	if finalStats.SeqGaps != 0 {
		t.Errorf("expected 0 sequence gaps, got %d", finalStats.SeqGaps)
	}
	if finalStats.DupSeq != 0 {
		t.Errorf("expected 0 duplicate sequences, got %d", finalStats.DupSeq)
	}
	if finalStats.LatePackets != 0 {
		t.Errorf("expected 0 late packets, got %d", finalStats.LatePackets)
	}
	if finalStats.Keyframes == 0 {
		t.Errorf("expected keyframes > 0, got %d", finalStats.Keyframes)
	}
	if finalDrops != 0 {
		t.Errorf("expected 0 queue drops, got %d", finalDrops)
	}

	// 7. Verify clean decode with ffmpeg
	if verifySink != nil {
		if err := recv.Close(); err != nil {
			t.Errorf("recv.Close with ffmpeg verify failed: %v", err)
		}
		vAUs, vBytes := verifySink.Stats()
		t.Logf("FFmpeg verify decoded %d AUs (%d bytes) with ZERO errors", vAUs, vBytes)
	}
}
