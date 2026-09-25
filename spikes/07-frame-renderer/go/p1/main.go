// p1 — Spike 07 / Phase 6 Slice 3 prototype P1.
//
// Measures the daemon-side decode/encode cost of the proposed "Annex-B →
// ffmpeg → JPEG frames" renderer tap, against the repo's existing validated
// display path (ffplay consumes the same Annex-B with the same libavcodec
// input flags, so the decode cost measured here is shared with the current
// renderer; everything above it is the added cost of the in-app path).
//
// Modes (all shell out to the system ffmpeg; stdlib only):
//
//	decode   — decode floor: ffmpeg -benchmark -f h264 -i S -f null -
//	mjpeg    — unscaled: Annex-B → MJPEG at quality q; throughput + JPEG sizes
//	i420     — unscaled: Annex-B → rawvideo yuv420p; throughput + bytes/frame
//	rtt      — paced at fps: per-frame AU→output latency (mjpeg or framecrc)
//	recovery — segA then IDR-aligned segB (DEC-021 re-join model) vs a
//	           mid-GOP negative control; counts decoder errors on stderr
//
// Output lines start with "RESULT " for machine extraction.
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ---------------------------------------------------------------------------
// Annex-B parsing
// ---------------------------------------------------------------------------

type au struct {
	data []byte
	idr  bool
}

// splitNALs returns NAL units without their start codes.
func splitNALs(b []byte) [][]byte {
	var nals [][]byte
	i := 0
	for i < len(b)-2 {
		if b[i] == 0 && b[i+1] == 0 {
			sc := 0
			if i+3 < len(b) && b[i+2] == 1 {
				sc = 3
			} else if i+4 <= len(b) && b[i+2] == 0 && b[i+3] == 1 {
				sc = 4
			}
			if sc > 0 {
				start := i + sc
				// next start code
				j := start
				for j < len(b)-2 {
					if b[j] == 0 && b[j+1] == 0 && ((j+3 <= len(b) && b[j+2] == 1) || (j+4 <= len(b) && b[j+2] == 0 && b[j+3] == 1)) {
						break
					}
					j++
				}
				if j > len(b)-2 {
					j = len(b)
				}
				if start < j {
					nals = append(nals, b[start:j])
				}
				i = j
				continue
			}
		}
		i++
	}
	return nals
}

// splitAUs groups NALs into access units. Assumes one slice per picture
// (libx264 default) and no B-frames: a slice NAL starts a new picture.
// Parameter-set/SEI NALs are attached to the picture that FOLLOWS them, which
// is what makes an IDR-aligned segment carry its own SPS/PPS — the DEC-021
// re-injection model the recovery mode tests.
func splitAUs(nals [][]byte) []au {
	var aus []au
	var pending, cur []byte
	curHasSlice := false
	hasIDR := func(b []byte) bool {
		for _, n := range splitNALs(b) {
			if len(n) > 0 && n[0]&0x1f == 5 {
				return true
			}
		}
		return false
	}
	flush := func() {
		if curHasSlice && len(cur) > 0 {
			aus = append(aus, au{data: cur, idr: hasIDR(cur)})
		}
		cur = nil
		curHasSlice = false
	}
	for _, n := range nals {
		if len(n) == 0 {
			continue
		}
		nt := n[0] & 0x1f
		if nt == 1 || nt == 5 {
			if curHasSlice {
				flush()
			}
			cur = append(cur, pending...)
			pending = nil
			cur = append(cur, 0, 0, 0, 1)
			cur = append(cur, n...)
			curHasSlice = true
		} else if curHasSlice {
			pending = append(pending, 0, 0, 0, 1)
			pending = append(pending, n...)
		} else {
			cur = append(cur, 0, 0, 0, 1)
			cur = append(cur, n...)
		}
	}
	flush()
	if len(pending) > 0 {
		// Trailing headers with no picture: harmless, drop them.
		_ = pending
	}
	return aus
}

func loadAUs(path string) []au {
	b, err := os.ReadFile(path)
	if err != nil {
		fatalf("read %s: %v", path, err)
	}
	aus := splitAUs(splitNALs(b))
	if len(aus) == 0 {
		fatalf("no access units parsed from %s", path)
	}
	idr := 0
	for _, a := range aus {
		if a.idr {
			idr++
		}
	}
	fmt.Printf("INFO parsed %s: %d AUs, %d IDR, %d bytes\n", path, len(aus), idr, len(b))
	return aus
}

// ---------------------------------------------------------------------------
// ffmpeg helpers
// ---------------------------------------------------------------------------

func ffmpegBase(threads int) []string {
	// NO -fflags nobuffer: measured empirically (spike 07 P1) to make the raw
	// H.264 demuxer drop 30-60 frames per run. The production ffplay sink pairs
	// nobuffer with -framedrop (display-realtime dropping is desired there);
	// a measuring/IPC tap must not silently lose AUs.
	args := []string{"-hide_banner", "-probesize", "32", "-analyzeduration", "0"}
	if threads > 0 {
		args = append(args, "-threads", fmt.Sprint(threads))
	}
	return args
}

func writeAUs(w io.WriteCloser, aus []au, pace time.Duration) ([]time.Time, error) {
	times := make([]time.Time, len(aus))
	var ticker *time.Ticker
	var tickC <-chan time.Time
	if pace > 0 {
		ticker = time.NewTicker(pace)
		tickC = ticker.C
		defer ticker.Stop()
	}
	for i, a := range aus {
		if tickC != nil {
			<-tickC
		}
		if _, err := w.Write(a.data); err != nil {
			return times[:i], err
		}
		times[i] = time.Now()
	}
	return times, nil
}

func childCPU(ps *os.ProcessState) time.Duration {
	if ps == nil {
		return 0
	}
	ru, ok := ps.SysUsage().(*syscall.Rusage)
	if !ok {
		return 0
	}
	return time.Duration(ru.Utime.Sec+ru.Stime.Sec)*time.Second +
		time.Duration(ru.Utime.Usec+ru.Stime.Usec)*time.Microsecond
}

// ---------------------------------------------------------------------------
// stats
// ---------------------------------------------------------------------------

func pct(sorted []int, p float64) int {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p / 100 * float64(len(sorted)))
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func summarizeSizes(name string, sizes []int) {
	if len(sizes) == 0 {
		fmt.Printf("RESULT %s count=0\n", name)
		return
	}
	s := append([]int(nil), sizes...)
	sort.Ints(s)
	sum := 0
	for _, v := range s {
		sum += v
	}
	fmt.Printf("RESULT %s count=%d avg=%d p50=%d p95=%d max=%d min=%d total=%d\n",
		name, len(s), sum/len(s), pct(s, 50), pct(s, 95), s[len(s)-1], s[0], sum)
}

func latStats(name string, lat []time.Duration) {
	if len(lat) == 0 {
		fmt.Printf("RESULT %s count=0\n", name)
		return
	}
	ms := make([]int, len(lat))
	for i, d := range lat {
		ms[i] = int(d / time.Microsecond)
	}
	sort.Ints(ms)
	sum := 0
	for _, v := range ms {
		sum += v
	}
	fmt.Printf("RESULT %s count=%d avg_ms=%.2f p50_ms=%.2f p95_ms=%.2f max_ms=%.2f min_ms=%.2f\n",
		name, len(ms), float64(sum)/float64(len(ms))/1000,
		float64(pct(ms, 50))/1000, float64(pct(ms, 95))/1000,
		float64(ms[len(ms)-1])/1000, float64(ms[0])/1000)
}

func fatalf(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "FATAL "+f+"\n", a...)
	os.Exit(1)
}

// ---------------------------------------------------------------------------
// JPEG frame scanner (SOI/EOI; FF is byte-stuffed in entropy data so FFD9
// only marks a real end-of-image)
// ---------------------------------------------------------------------------

type jpegScanner struct {
	buf []byte
}

// feed appends bytes and returns completed JPEG payloads.
func (s *jpegScanner) feed(p []byte) [][]byte {
	s.buf = append(s.buf, p...)
	var out [][]byte
	for {
		soi := bytes.Index(s.buf, []byte{0xff, 0xd8})
		if soi < 0 {
			if len(s.buf) > 2 {
				s.buf = s.buf[len(s.buf)-2:]
			}
			return out
		}
		if soi > 0 {
			s.buf = s.buf[soi:]
		}
		eoi := bytes.Index(s.buf[2:], []byte{0xff, 0xd9})
		if eoi < 0 {
			return out
		}
		end := 2 + eoi + 2
		out = append(out, append([]byte(nil), s.buf[:end]...))
		s.buf = s.buf[end:]
	}
}

// ---------------------------------------------------------------------------
// modes
// ---------------------------------------------------------------------------

var benchLine = regexp.MustCompile(`bench: utime=([0-9.]+)s? stime=([0-9.]+)s? rtime=([0-9.]+)s?`)
var frameLine = regexp.MustCompile(`frame=\s*(\d+)`)

func runDecode(path string) {
	cmd := exec.Command("ffmpeg",
		append(ffmpegBase(0), "-benchmark", "-v", "info", "-f", "h264", "-i", path, "-f", "null", "-")...)
	var stderr bytes.Buffer
	cmd.Stdout = io.Discard
	cmd.Stderr = &stderr
	t0 := time.Now()
	if err := cmd.Run(); err != nil {
		fatalf("decode: %v\n%s", err, stderr.String())
	}
	wall := time.Since(t0)
	out := stderr.String()
	var ut, st, rt float64
	if m := benchLine.FindStringSubmatch(out); m != nil {
		fmt.Sscan(m[1], &ut)
		fmt.Sscan(m[2], &st)
		fmt.Sscan(m[3], &rt)
	}
	frames := 0
	if ms := frameLine.FindAllStringSubmatch(out, -1); len(ms) > 0 {
		fmt.Sscan(ms[len(ms)-1][1], &frames)
	}
	if rt == 0 {
		rt = wall.Seconds()
	}
	fmt.Printf("RESULT decode frames=%d wall_s=%.3f bench_rtime_s=%.3f cpu_s=%.3f cores=%.2f decode_fps=%.1f\n",
		frames, wall.Seconds(), rt, ut+st, (ut+st)/rt, float64(frames)/rt)
}

func runUnscaled(path, outFmt string, q, threads, w, h int) {
	aus := loadAUs(path)

	args := append(ffmpegBase(threads), "-v", "error", "-f", "h264", "-i", "-")
	var pipeArgs []string
	switch outFmt {
	case "mjpeg":
		pipeArgs = []string{"-f", "mjpeg", "-q:v", fmt.Sprint(q)}
	case "rawvideo":
		pipeArgs = []string{"-f", "rawvideo", "-pix_fmt", "yuv420p"}
	default:
		fatalf("unknown out format %s", outFmt)
	}

	cmd := exec.Command("ffmpeg", append(args, append(pipeArgs, "pipe:1")...)...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		fatalf("%v", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		fatalf("%v", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		fatalf("%v", err)
	}
	t0 := time.Now()

	var wg sync.WaitGroup
	wg.Add(1)
	var writeErr error
	go func() {
		defer wg.Done()
		_, writeErr = writeAUs(stdin, aus, 0)
		_ = stdin.Close()
	}()

	sizes := []int{}
	frameCount := 0
	var rawTotal int64
	switch outFmt {
	case "mjpeg":
		sc := &jpegScanner{}
		buf := make([]byte, 1<<16)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				for _, f := range sc.feed(buf[:n]) {
					sizes = append(sizes, len(f))
					frameCount++
				}
			}
			if err != nil {
				break
			}
		}
	default:
		buf := make([]byte, 1<<16)
		for {
			n, err := stdout.Read(buf)
			rawTotal += int64(n)
			if err != nil {
				break
			}
		}
	}

	wg.Wait()
	waitErr := cmd.Wait()
	cpu := childCPU(cmd.ProcessState)
	_ = writeErr
	if waitErr != nil {
		fatalf("ffmpeg %s: %v\n%s", outFmt, waitErr, stderr.String())
	}

	if outFmt == "rawvideo" {
		bpf := int64(w * h * 3 / 2)
		fmt.Printf("INFO rawvideo total_bytes=%d bytes_per_frame=%d\n", rawTotal, bpf)
		if rawTotal%bpf != 0 {
			fmt.Printf("WARN rawvideo total %% bytes_per_frame != 0\n")
		}
		fmt.Printf("RESULT rawvideo frames=%d bytes_per_frame=%d total_bytes=%d per_frame_MiB=%.3f\n",
			rawTotal/bpf, bpf, rawTotal, float64(bpf)/(1<<20))
		fmt.Printf("RESULT rawvideo_30fps_MBps=%.1f rawvideo_60fps_MBps=%.1f\n",
			float64(bpf)*30/(1<<20), float64(bpf)*60/(1<<20))
		return
	}

	{
		var idr, non []int
		for i, s := range sizes {
			if i < len(aus) && aus[i].idr {
				idr = append(idr, s)
			} else {
				non = append(non, s)
			}
		}
		summarizeSizes("mjpeg_idr", idr)
		summarizeSizes("mjpeg_p", non)
		summarizeSizes("mjpeg_all", sizes)
		if len(sizes) != len(aus) {
			fmt.Printf("WARN mjpeg frame count %d != AU count %d\n", len(sizes), len(aus))
		}
		fmt.Printf("RESULT mjpeg q=%d frames=%d wall_s=%.3f flood_fps=%.1f cpu_s=%.3f\n",
			q, len(sizes), time.Since(t0).Seconds(), float64(len(sizes))/time.Since(t0).Seconds(), cpu.Seconds())
	}
}

func runRTT(path, proto string, q, fps, n, threads int, extra []string) {
	aus := loadAUs(path)
	if n > 0 && n < len(aus) {
		aus = aus[:n]
	}
	args := append(ffmpegBase(threads), extra...)
	args = append(args, "-v", "error", "-f", "h264", "-i", "-")
	var outArgs []string
	switch proto {
	case "mjpeg":
		outArgs = []string{"-f", "mjpeg", "-q:v", fmt.Sprint(q)}
	case "framecrc":
		outArgs = []string{"-f", "framecrc"}
	default:
		fatalf("unknown proto %s", proto)
	}
	cmd := exec.Command("ffmpeg", append(args, append(outArgs, "pipe:1")...)...)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		fatalf("%v", err)
	}

	times := make([]time.Time, len(aus))
	arrivals := make(chan []time.Time, 1)

	// reader
	go func() {
		var ts []time.Time
		switch proto {
		case "mjpeg":
			sc := &jpegScanner{}
			buf := make([]byte, 1<<16)
			for {
				nRead, err := stdout.Read(buf)
				if nRead > 0 {
					for range sc.feed(buf[:nRead]) {
						ts = append(ts, time.Now())
					}
				}
				if err != nil {
					break
				}
			}
		case "framecrc":
			br := bufio.NewReader(stdout)
			for {
				line, err := br.ReadString('\n')
				if len(line) > 0 && line[0] != '#' && strings.Contains(line, ",") {
					ts = append(ts, time.Now())
				}
				if err != nil {
					break
				}
			}
		}
		arrivals <- ts
	}()

	pace := time.Second / time.Duration(fps)
	wStart := time.Now()
	wTimes, wErr := writeAUs(stdin, aus, pace)
	_ = stdin.Close()
	wallPace := time.Since(wStart)
	copy(times, wTimes)
	_ = wErr

	ts := <-arrivals
	if err := cmd.Wait(); err != nil {
		fatalf("ffmpeg rtt: %v\n%s", err, stderr.String())
	}

	if len(ts) != len(aus) {
		fmt.Printf("WARN %s output frames=%d != input AUs=%d (pairing by index is approximate)\n",
			proto, len(ts), len(aus))
	}
	warmup := 3
	var lat []time.Duration
	for i := warmup; i < len(ts) && i < len(times); i++ {
		if times[i].IsZero() {
			continue
		}
		lat = append(lat, ts[i].Sub(times[i]))
	}
	latStats("rtt_"+proto, lat)
	if len(ts) > 1 {
		achieved := float64(len(ts)-1) / ts[len(ts)-1].Sub(ts[0]).Seconds()
		fmt.Printf("RESULT rtt proto=%s q=%d target_fps=%d achieved_fps=%.2f threads=%d write_wall_s=%.3f frames=%d extra=%q\n",
			proto, q, fps, achieved, threads, wallPace.Seconds(), len(ts), strings.Join(extra, " "))
	}
}

var errRe = regexp.MustCompile(`(?i)error|corrupt|invalid|bytestream`)

func runRecovery(path string) {
	aus := loadAUs(path)

	// segA: first 90 AUs. segB: first IDR-aligned window starting at AU >= 300.
	// segC: same as segB but starting 5 AUs *into* the GOP (mid-GOP negative control).
	segA := aus[:90]
	findIDR := func(min int) int {
		for i := min; i < len(aus)-95; i++ {
			if aus[i].idr {
				return i
			}
		}
		return -1
	}
	b := findIDR(300)
	c := findIDR(600)
	if b < 0 || c < 0 {
		fatalf("stream too short for recovery segments")
	}
	segB := aus[b : b+90]
	segC := aus[c+5 : c+95]
	fmt.Printf("INFO recovery segA=0..89 segB starts at AU %d (idr=%v) segC starts at AU %d (idr=%v)\n",
		b, segB[0].idr, c+5, segC[0].idr)

	run := func(name string, seq []au, wantClean bool) {
		cmd := exec.Command("ffmpeg", append(ffmpegBase(0),
			"-v", "info", "-f", "h264", "-i", "-", "-f", "null", "-")...)
		stdin, _ := cmd.StdinPipe()
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		cmd.Stdout = io.Discard
		if err := cmd.Start(); err != nil {
			fatalf("%v", err)
		}
		for _, a := range seq {
			_, _ = stdin.Write(a.data)
		}
		_ = stdin.Close()
		waitErr := cmd.Wait()
		out := stderr.String()
		errCount := 0
		var samples []string
		for _, line := range strings.Split(out, "\n") {
			if errRe.MatchString(line) {
				errCount++
				if len(samples) < 5 {
					samples = append(samples, strings.TrimSpace(line))
				}
			}
		}
		frames := 0
		if ms := frameLine.FindAllStringSubmatch(out, -1); len(ms) > 0 {
			fmt.Sscan(ms[len(ms)-1][1], &frames)
		}
		clean := errCount == 0 && waitErr == nil
		fmt.Printf("RESULT recovery seq=%s in_aus=%d out_frames=%d stderr_err_lines=%d clean=%v ffmpeg_err=%v\n",
			name, len(seq), frames, errCount, clean, waitErr != nil)
		for _, s := range samples {
			fmt.Printf("INFO recovery[%s] stderr: %s\n", name, s)
		}
		if wantClean && !clean {
			fmt.Printf("WARN %s expected clean decode but was not (see ffmpeg stderr)\n", name)
		}
	}

	run("A_then_B_idr_rejoin", append(append([]au{}, segA...), segB...), true)
	run("A_then_C_mid_gop", append(append([]au{}, segA...), segC...), false)
}

// ---------------------------------------------------------------------------

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: p1 <decode|mjpeg|i420|rtt|recovery> [flags]")
		os.Exit(2)
	}
	mode := os.Args[1]
	fs := flag.NewFlagSet(mode, flag.ExitOnError)
	in := fs.String("i", "", "input .h264")
	q := fs.Int("q", 3, "MJPEG quality (ffmpeg -q:v 2..31, lower = better)")
	fps := fs.Int("fps", 30, "paced fps for rtt")
	n := fs.Int("n", 0, "max AUs for rtt (0 = all)")
	threads := fs.Int("threads", 0, "decoder threads (0 = auto)")
	size := fs.String("size", "720x1600", "WxH of stream (i420 mode)")
	extraStr := fs.String("extra", "", "extra ffmpeg input args, space-separated")
	proto := fs.String("proto", "mjpeg", "rtt protocol: mjpeg|framecrc")
	_ = fs.Parse(os.Args[2:])
	if *in == "" {
		fatalf("-i is required")
	}

	switch mode {
	case "decode":
		runDecode(*in)
	case "mjpeg":
		runUnscaled(*in, "mjpeg", *q, *threads, 0, 0)
	case "i420":
		var w, h int
		if _, err := fmt.Sscanf(*size, "%dx%d", &w, &h); err != nil || w == 0 || h == 0 {
			fatalf("bad -size %q", *size)
		}
		runUnscaled(*in, "rawvideo", 0, *threads, w, h)
	case "rtt":
		runRTT(*in, *proto, *q, *fps, *n, *threads, strings.Fields(*extraStr))
	case "recovery":
		runRecovery(*in)
	default:
		fatalf("unknown mode %s", mode)
	}
}
