package receiver

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"

	"github.com/om051p/phonebridge/core/pkg/rtpmedia"
)

// FrameSink defines the destination consumer for assembled Annex-B access units.
type FrameSink interface {
	// WriteAU delivers an assembled access unit. Implementations must not mutate au.Data.
	WriteAU(au rtpmedia.AccessUnit) error
	// Close releases any underlying file or subprocess handles.
	Close() error
}

// NullSink discards access units while tracking write counts for benchmarking.
type NullSink struct {
	AUs   atomic.Int64
	Bytes atomic.Int64
}

// NewNullSink creates an empty NullSink.
func NewNullSink() *NullSink {
	return &NullSink{}
}

func (n *NullSink) WriteAU(au rtpmedia.AccessUnit) error {
	n.AUs.Add(1)
	n.Bytes.Add(int64(len(au.Data)))
	return nil
}

func (n *NullSink) Close() error {
	return nil
}

// FileSink records an Annex-B H.264 bitstream to a .h264 file, with an optional
// companion .idx sidecar recording per-AU boundaries and metadata.
type FileSink struct {
	mu        sync.Mutex
	h264File  *os.File
	auIdxFile *os.File
	idx       int64
}

// NewFileSink creates a FileSink writing to h264Path, and optionally auIdxPath if non-empty.
func NewFileSink(h264Path, auIdxPath string) (*FileSink, error) {
	hf, err := os.Create(h264Path)
	if err != nil {
		return nil, fmt.Errorf("receiver: create h264 file: %w", err)
	}

	var idxf *os.File
	if auIdxPath != "" {
		idxf, err = os.Create(auIdxPath)
		if err != nil {
			_ = hf.Close()
			return nil, fmt.Errorf("receiver: create idx file: %w", err)
		}
	}

	return &FileSink{
		h264File:  hf,
		auIdxFile: idxf,
	}, nil
}

func (f *FileSink) WriteAU(au rtpmedia.AccessUnit) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.h264File == nil {
		return fmt.Errorf("receiver: file sink already closed")
	}

	if _, err := f.h264File.Write(au.Data); err != nil {
		return err
	}

	if f.auIdxFile != nil {
		idrInt := 0
		if au.IsKeyframe {
			idrInt = 1
		}
		// Format: idx bytes rtp_ts idr marker(1) sps pps
		_, _ = fmt.Fprintf(f.auIdxFile, "%d %d %d %d 1 %d %d\n",
			f.idx, len(au.Data), au.Timestamp, idrInt, au.SPSCount, au.PPSCount)
	}
	f.idx++
	return nil
}

func (f *FileSink) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()

	var firstErr error
	if f.h264File != nil {
		if err := f.h264File.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		f.h264File = nil
	}
	if f.auIdxFile != nil {
		if err := f.auIdxFile.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		f.auIdxFile = nil
	}
	return firstErr
}

// PipeSink streams access unit bitstreams into the stdin of an external command
// (e.g. ffplay, gst-launch, or ffmpeg).
type PipeSink struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	mu     sync.Mutex
	closed bool
}

// NewPipeSink launches an external process with stdin connected as a FrameSink.
func NewPipeSink(binary string, args ...string) (*PipeSink, error) {
	binPath, err := exec.LookPath(binary)
	if err != nil {
		return nil, fmt.Errorf("receiver: binary %q not found in PATH: %w", binary, err)
	}

	cmd := exec.Command(binPath, args...)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("receiver: create stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("receiver: start command %q: %w", binary, err)
	}

	return &PipeSink{
		cmd:   cmd,
		stdin: stdin,
	}, nil
}

func (p *PipeSink) WriteAU(au rtpmedia.AccessUnit) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return fmt.Errorf("receiver: pipe sink closed")
	}

	_, err := p.stdin.Write(au.Data)
	if err != nil {
		p.closed = true
		return fmt.Errorf("receiver: pipe write failed: %w", err)
	}
	return nil
}

// Close shuts the pipe down. The child gets stdin EOF and a short grace
// period to exit on its own; players like ffplay do NOT exit on stdin EOF
// (they merely stop reading commands), so an unbounded Wait() hung session
// teardown forever (found in Phase 2 acceptance). After the grace period the
// process is killed — Close is a teardown, not a negotiation.
func (p *PipeSink) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.closed {
		return nil
	}
	p.closed = true

	var firstErr error
	if err := p.stdin.Close(); err != nil {
		firstErr = err
	}
	if p.cmd != nil && p.cmd.Process != nil {
		done := make(chan struct{})
		go func() {
			_ = p.cmd.Wait()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			_ = p.cmd.Process.Kill()
			<-done
		}
	}
	return firstErr
}

// NewDisplaySink launches ffplay configured for minimal latency display on Linux.
func NewDisplaySink(windowTitle string, lowLatency bool) (*PipeSink, error) {
	args := []string{
		"-f", "h264",
		"-probesize", "32",
		"-fpsprobesize", "0",
		"-sync", "ext",
	}
	if lowLatency {
		args = append(args,
			"-fflags", "nobuffer",
			"-flags", "low_delay",
			"-framedrop",
		)
	}
	if windowTitle != "" {
		args = append(args, "-window_title", windowTitle)
	}
	args = append(args, "-")

	return NewPipeSink("ffplay", args...)
}

// FFmpegVerifySink streams the bitstream into ffmpeg in headless mode
// to verify zero bitstream syntax / decoding errors.
type FFmpegVerifySink struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stderr bytes.Buffer
	mu     sync.Mutex
	closed bool
	aus    int64
	bytes  int64
}

// NewFFmpegVerifySink creates a sink that decodes the stream with ffmpeg and reports errors.
func NewFFmpegVerifySink() (*FFmpegVerifySink, error) {
	binPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, fmt.Errorf("receiver: ffmpeg not found in PATH: %w", err)
	}

	// Run ffmpeg with error logging only, reading from stdin and discarding output (-f null -)
	cmd := exec.Command(binPath,
		"-v", "error",
		"-f", "h264",
		"-i", "-",
		"-f", "null", "-",
	)

	s := &FFmpegVerifySink{cmd: cmd}
	cmd.Stderr = &s.stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("receiver: ffmpeg stdin pipe: %w", err)
	}
	s.stdin = stdin

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("receiver: start ffmpeg: %w", err)
	}

	return s, nil
}

func (s *FFmpegVerifySink) WriteAU(au rtpmedia.AccessUnit) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return fmt.Errorf("receiver: verify sink closed")
	}

	s.aus++
	s.bytes += int64(len(au.Data))
	_, err := s.stdin.Write(au.Data)
	return err
}

// Close finishes the stream and checks whether ffmpeg encountered any decoding errors.
func (s *FFmpegVerifySink) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return nil
	}
	s.closed = true

	_ = s.stdin.Close()
	waitErr := s.cmd.Wait()

	stderrOutput := s.stderr.String()
	if waitErr != nil || len(stderrOutput) > 0 {
		return fmt.Errorf("receiver: ffmpeg decode error (exit: %v, stderr: %q)", waitErr, stderrOutput)
	}
	return nil
}

// Stats returns the number of AUs and bytes sent to the verification decoder.
func (s *FFmpegVerifySink) Stats() (aus, bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.aus, s.bytes
}
