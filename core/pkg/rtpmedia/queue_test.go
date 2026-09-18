package rtpmedia

import (
	"fmt"
	"runtime"
	"sync"
	"testing"
)

func mkFrame(sz int, key bool, pts int64) Frame {
	return Frame{Data: make([]byte, sz), PTSUs: pts, Key: key}
}

func TestQueuePushPopFIFO(t *testing.T) {
	q := NewQueue(4)
	for i := 0; i < 4; i++ {
		if !q.Push(mkFrame(10, false, int64(i))) {
			t.Fatalf("push %d rejected on non-full queue", i)
		}
	}
	for i := 0; i < 4; i++ {
		f, ok := q.Pop()
		if !ok {
			t.Fatalf("pop %d: queue unexpectedly empty", i)
		}
		if f.PTSUs != int64(i) {
			t.Fatalf("pop %d: got PTS %d, want FIFO order", i, f.PTSUs)
		}
	}
	if _, ok := q.Pop(); ok {
		t.Fatal("pop from empty queue returned a frame")
	}
}

func TestQueueDropsIncomingNonKeyWhenFull(t *testing.T) {
	q := NewQueue(4)
	for i := 0; i < 4; i++ {
		q.Push(mkFrame(10, false, int64(i)))
	}
	// Non-key on full queue: incoming frame dropped, queue untouched.
	if q.Push(mkFrame(10, false, 99)) {
		t.Fatal("non-key push onto full queue was admitted")
	}
	if q.Len() != 4 {
		t.Fatalf("queue length changed on dropped push: %d", q.Len())
	}
	f, _ := q.Pop()
	if f.PTSUs != 0 {
		t.Fatalf("oldest frame should still be PTS 0, got %d", f.PTSUs)
	}
	if q.Dropped != 1 {
		t.Fatalf("Dropped = %d, want 1", q.Dropped)
	}
	if q.Pushed != 5 {
		t.Fatalf("Pushed = %d, want 5", q.Pushed)
	}
}

func TestQueueKeyframeEvictsOldest(t *testing.T) {
	q := NewQueue(4)
	for i := 0; i < 4; i++ {
		q.Push(mkFrame(10, false, int64(i)))
	}
	// Key on full queue: evict oldest until it fits.
	if !q.Push(mkFrame(10, true, 100)) {
		t.Fatal("key push onto full queue was rejected")
	}
	if q.Len() != 4 {
		t.Fatalf("queue length after key push = %d, want 4", q.Len())
	}
	f, _ := q.Pop()
	if f.PTSUs != 1 {
		t.Fatalf("first frame after eviction should be PTS 1, got %d", f.PTSUs)
	}
	if q.Evicted != 1 || q.EvictedK != 0 {
		t.Fatalf("Evicted = %d, EvictedK = %d, want 1/0", q.Evicted, q.EvictedK)
	}
	if q.Dropped != 0 {
		t.Fatalf("Dropped = %d, want 0", q.Dropped)
	}
}

func TestQueueKeyframeEvictionCountEvictedKeys(t *testing.T) {
	q := NewQueue(1)
	q.Push(mkFrame(10, true, 0))
	q.Push(mkFrame(10, true, 1)) // evicts key PTS 0
	if q.Evicted != 1 || q.EvictedK != 1 {
		t.Fatalf("Evicted = %d, EvictedK = %d, want 1/1", q.Evicted, q.EvictedK)
	}
}

func TestQueueDepthHighWater(t *testing.T) {
	q := NewQueue(8)
	for i := 0; i < 5; i++ {
		q.Push(mkFrame(10, false, int64(i)))
	}
	for i := 0; i < 3; i++ {
		q.Pop()
	}
	q.Push(mkFrame(10, false, 50))
	if q.Depth != 5 {
		t.Fatalf("Depth high-water = %d, want 5", q.Depth)
	}
	if q.Cap() != 8 {
		t.Fatalf("Cap = %d, want 8", q.Cap())
	}
}

func TestQueueDefaultCap(t *testing.T) {
	if q := NewQueue(0); q.Cap() != 256 {
		t.Fatalf("default cap = %d, want 256 (spike-validated)", q.Cap())
	}
	if q := NewQueue(-3); q.Cap() != 256 {
		t.Fatalf("negative cap fallback = %d, want 256", q.Cap())
	}
}

// TestQueueConcurrentPushPop exercises the producer/consumer race under -race.
// Producers finish before the consumer terminates (WaitGroup + closed
// channel), so consumed + evicted + dropped must equal pushed exactly.
func TestQueueConcurrentPushPop(t *testing.T) {
	q := NewQueue(16)
	const producers, perProducer = 4, 500
	var wgProducers sync.WaitGroup
	for p := 0; p < producers; p++ {
		wgProducers.Add(1)
		go func(p int) {
			defer wgProducers.Done()
			for i := 0; i < perProducer; i++ {
				q.Push(mkFrame(8, i%50 == 0, int64(p*perProducer+i)))
			}
		}(p)
	}
	done := make(chan struct{})
	go func() {
		wgProducers.Wait()
		close(done) // after close, no further Push can occur
	}()
	consumed := make(chan int, 1)
	go func() {
		n := 0
		for {
			if _, ok := q.Pop(); ok {
				n++
				continue
			}
			select {
			case <-done:
				// Producers are finished; one last drain check covers a
				// push that landed between the failed pop and the close.
				if _, ok := q.Pop(); ok {
					n++
					continue
				}
				consumed <- n
				return
			default:
				runtime.Gosched()
			}
		}
	}()
	n := <-consumed
	// Everything must be accounted for: consumed + evicted + dropped == pushed.
	if got := int64(n) + q.Evicted + q.Dropped; got != q.Pushed {
		t.Fatalf("accounting: consumed(%d)+evicted(%d)+dropped(%d) != pushed(%d)", n, q.Evicted, q.Dropped, q.Pushed)
	}
	if q.Pushed != producers*perProducer {
		t.Fatalf("pushed = %d, want %d", q.Pushed, producers*perProducer)
	}
}

func TestQueueZeroLenFramesAllowed(t *testing.T) {
	q := NewQueue(2)
	if !q.Push(Frame{PTSUs: 1}) {
		t.Fatal("zero-length frame rejected (legal: classification is by flag)")
	}
	f, ok := q.Pop()
	if !ok || f.PTSUs != 1 {
		t.Fatalf("pop zero-length frame: ok=%v f=%+v", ok, f)
	}
}

func ExampleQueue_dropPolicy() {
	q := NewQueue(1)
	fmt.Println(q.Push(Frame{Key: false})) // fills the queue
	fmt.Println(q.Push(Frame{Key: false})) // full: non-key dropped
	fmt.Println(q.Push(Frame{Key: true}))  // full: key evicts oldest
	fmt.Println(q.Dropped, q.Evicted, q.Len())
	// Output:
	// true
	// false
	// true
	// 1 1 1
}
