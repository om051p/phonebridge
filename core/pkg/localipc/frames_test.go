package localipc

// Phase 6 Slice 3A: StreamFrames contract tests over real local IPC.
//
// They pin the three properties the Flutter consumer relies on:
//   - every message carries <= 64 KiB of JPEG and the chunk sequence
//     (chunk_index/chunk_count/last_chunk) reassembles the frame exactly;
//   - frame ids are rebased per subscription starting at 1 while gaps
//     (daemon-side latest-wins drops) stay visible as jumps;
//   - a session end closes the channel, so no stale frame can be delivered
//     after termination, and the handler simply waits for the next session.

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/frames"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
)

// sequentialFrameSource hands out one channel per "session" and then blocks,
// mirroring frames.Hub.Subscribe's contract (wait for the next session or
// the client's cancellation).
type sequentialFrameSource struct {
	mu   sync.Mutex
	chs  []chan *frames.Frame
	next int
}

func (s *sequentialFrameSource) Subscribe(ctx context.Context) (<-chan *frames.Frame, error) {
	s.mu.Lock()
	if s.next < len(s.chs) {
		ch := s.chs[s.next]
		s.next++
		s.mu.Unlock()
		return ch, nil
	}
	s.mu.Unlock()
	<-ctx.Done()
	return nil, ctx.Err()
}

func startFramesServer(t *testing.T, src FrameSource) (sock, tokVal string, cancel func(), errCh <-chan error) {
	t.Helper()
	sockPath, tokPath, tok := testSetup(t)
	cfg := Config{
		SocketPath:       sockPath,
		TokenPath:        tokPath,
		Token:            tok,
		ServerVersion:    "1.0.0",
		DaemonGeneration: 7,
		Frames:           src,
	}
	_, c, ch := startTestServer(t, cfg)
	return sockPath, tok, c, ch
}

func TestRPC_StreamFrames_ChunkingRebaseAndSessionEnd(t *testing.T) {
	session1 := make(chan *frames.Frame, 8)
	session2 := make(chan *frames.Frame, 8)
	src := &sequentialFrameSource{chs: []chan *frames.Frame{session1, session2}}

	sock, tokVal, cancel, errCh := startFramesServer(t, src)
	defer func() {
		cancel()
		<-errCh
	}()

	client, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	ctx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()

	stream, err := client.StreamFrames(ctx, &phonebridgelocalipcv1.StreamFramesRequest{})
	if err != nil {
		t.Fatalf("StreamFrames: %v", err)
	}

	// Session 1, frame 1: a JPEG larger than one message so it must chunk.
	big := bytes.Repeat([]byte{0xAB, 0xCD}, 50_000) // 100,000 bytes
	session1 <- frames.NewFrame(1, big, 720, 1600)
	// Daemon-side drop, then frame 5: the gap must survive reassembly.
	session1 <- frames.NewFrame(5, []byte("second"), 720, 1600)
	close(session1) // session ends: no stale frame may follow

	// Session 2 starts a fresh id space.
	session2 <- frames.NewFrame(7, []byte("next-session"), 720, 1600)
	close(session2)

	recvN := func(n int) []*phonebridgelocalipcv1.StreamFramesResponse {
		t.Helper()
		out := make([]*phonebridgelocalipcv1.StreamFramesResponse, 0, n)
		for i := 0; i < n; i++ {
			resp, err := stream.Recv()
			if err != nil {
				t.Fatalf("Recv %d/%d: %v", i+1, n, err)
			}
			out = append(out, resp)
		}
		return out
	}

	// Frame 1 chunks: expect 100,000 / 65,536 → 2 messages.
	var chunked []*phonebridgelocalipcv1.StreamFramesResponse
	for {
		resp := recvN(1)[0]
		chunked = append(chunked, resp)
		if resp.GetLastChunk() {
			break
		}
		if len(chunked) > 8 {
			t.Fatal("frame 1 never terminated with last_chunk")
		}
	}

	var reassembled []byte
	for i, c := range chunked {
		if c.GetFrameId() != 1 {
			t.Errorf("chunk %d frame_id = %d, want 1", i, c.GetFrameId())
		}
		if c.GetChunkIndex() != uint32(i) {
			t.Errorf("chunk %d index = %d, want %d", i, c.GetChunkIndex(), i)
		}
		if c.GetChunkCount() != uint32(len(chunked)) {
			t.Errorf("chunk %d count = %d, want %d", i, c.GetChunkCount(), len(chunked))
		}
		if len(c.GetJpeg()) > frames.MaxChunkBytes {
			t.Errorf("chunk %d is %d bytes, exceeds the 64 KiB rule", i, len(c.GetJpeg()))
		}
		if c.GetWidth() != 720 || c.GetHeight() != 1600 {
			t.Errorf("chunk %d dims = %dx%d, want 720x1600", i, c.GetWidth(), c.GetHeight())
		}
		if c.GetLastChunk() != (i == len(chunked)-1) {
			t.Errorf("chunk %d last_chunk = %v", i, c.GetLastChunk())
		}
		reassembled = append(reassembled, c.GetJpeg()...)
	}
	if !bytes.Equal(reassembled, big) {
		t.Errorf("reassembled %d bytes, want %d (byte-identical)", len(reassembled), len(big))
	}
	if len(chunked) != 2 {
		t.Errorf("frame 1 took %d chunks, want 2", len(chunked))
	}

	// Frame 5: id gap (1 → 5) survives as the client-visible drop signal.
	resp := recvN(1)[0]
	if resp.GetFrameId() != 5 {
		t.Errorf("second frame_id = %d, want 5 (gap preserved)", resp.GetFrameId())
	}
	if string(resp.GetJpeg()) != "second" {
		t.Errorf("second frame payload = %q", resp.GetJpeg())
	}

	// Session 2 rebases to 1: nothing from session 1 leaks.
	resp = recvN(1)[0]
	if resp.GetFrameId() != 1 {
		t.Errorf("post-rebase frame_id = %d, want 1", resp.GetFrameId())
	}
	if string(resp.GetJpeg()) != "next-session" {
		t.Errorf("stale frame delivered across sessions: %q", resp.GetJpeg())
	}

	// Both sessions ended and the source now blocks: the handler must be
	// parked in Subscribe, so a Recv would hang — cancellation returns.
	streamCancel()
	deadline := time.After(3 * time.Second)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			if _, err := stream.Recv(); err != nil {
				return
			}
		}
	}()
	select {
	case <-done:
	case <-deadline:
		t.Fatal("StreamFrames did not terminate after cancellation")
	}
}

func TestRPC_StreamFrames_UnimplementedWithoutHub(t *testing.T) {
	sock, tokVal, cancel, errCh := startFramesServer(t, nil)
	defer func() {
		cancel()
		<-errCh
	}()

	client, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	stream, err := client.StreamFrames(context.Background(),
		&phonebridgelocalipcv1.StreamFramesRequest{})
	if err != nil {
		t.Fatalf("StreamFrames call: %v", err)
	}
	if _, err := stream.Recv(); err == nil || err == io.EOF {
		t.Fatalf("Recv error = %v, want Unimplemented", err)
	} else if !bytes.Contains([]byte(err.Error()), []byte("Unimplemented")) &&
		!bytes.Contains([]byte(err.Error()), []byte("frame streaming not configured")) {
		t.Errorf("Recv error = %q, want Unimplemented/frame streaming not configured", err)
	}
}
