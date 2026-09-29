package localipc

// Daemon shutdown latency: a REGRESSION test for the defect found during the
// Phase-4 comms audit.
//
// The defect: `Serve`'s shutdown path called `closeAllSubscribers()` (which only
// closes StreamEvents subscriber channels) and then `GracefulStop()`.
// GracefulStop waits for every in-flight RPC, and StreamFrames is a long-lived
// server-stream RPC whose handler parked inside `frames.Hub.Subscribe(ctx)`
// until either a frame session began or the CLIENT's context was cancelled.
// Nothing on the shutdown path cancelled that context, and StreamFrames was not
// registered in `s.subscribers`, so the daemon could not stop gracefully while
// a Flutter client held StreamFrames open: it paid the full 3 s timeout and
// hard-`Stop()`ed the gRPC server.
//
// Production evidence (before the fix): the user journal shows
//   Sep 28 23:02:23 x1 phonebridge-daemon[2725]: graceful stop timed out; forcing stop
//
// Measured before/after on this machine, with one StreamFrames client attached:
//   before: 3.0024 s  (the GracefulStop timeout, exactly)
//   after:  433 µs    (≈ the idle path: 123 µs)
//
// The fix lives in server.go: frame subscriptions are derived from a
// server-owned context that shutdown cancels, so the handler returns on its own
// and the hub subscription is released instead of waiting for a client.

import (
	"context"
	"testing"
	"time"

	"github.com/om051p/phonebridge/core/pkg/frames"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
)

// measureShutdown starts a server, optionally attaches a frame-stream client,
// then cancels the daemon context and returns how long Serve took to return
// along with the frame hub it was serving.
func measureShutdown(t *testing.T, withFrameStream bool) (time.Duration, *frames.Hub) {
	t.Helper()

	sock, tok, tokVal := testSetup(t)
	hub := frames.NewHub()
	cfg := Config{
		SocketPath:    sock,
		TokenPath:     tok,
		Token:         tokVal,
		ServerVersion: "0.1.0-test",
		Frames:        hub,
	}
	_, cancel, errCh := startTestServer(t, cfg)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	client, err := Dial(ctx, sock, tokVal)
	if err != nil {
		cancel()
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	if withFrameStream {
		// The handler immediately parks in Hub.Subscribe: no session is open, so
		// it waits for one (or for its context to be cancelled).
		if _, err := client.StreamFrames(ctx, &phonebridgelocalipcv1.StreamFramesRequest{}); err != nil {
			cancel()
			t.Fatalf("open StreamFrames: %v", err)
		}
		// Let the RPC reach the server before shutting down.
		time.Sleep(200 * time.Millisecond)
	}

	start := time.Now()
	cancel()
	select {
	case <-errCh:
	case <-time.After(15 * time.Second):
		t.Fatalf("Serve did not return within 15s of cancel")
	}
	return time.Since(start), hub
}

// Control: shutdown with no long-lived streams attached is immediate.
func TestShutdown_NoStreamsIsImmediate(t *testing.T) {
	elapsed, _ := measureShutdown(t, false)
	t.Logf("shutdown with no streams: %v", elapsed)
	if elapsed > 1*time.Second {
		t.Errorf("idle shutdown took %v; want < 1s", elapsed)
	}
}

// The regression: a StreamFrames client must not delay daemon shutdown.
// Before the fix this took the full 3 s GracefulStop timeout.
func TestShutdown_StreamFramesDoesNotDelayGracefulStop(t *testing.T) {
	elapsed, _ := measureShutdown(t, true)
	t.Logf("shutdown with a StreamFrames client: %v", elapsed)
	if elapsed > 1*time.Second {
		t.Errorf("shutdown with a frame stream took %v; want < 1s: shutdown must "+
			"cancel frame subscriptions rather than wait for the client", elapsed)
	}
}

// And the subscription must actually be released: a handler that returns while
// leaving its channel registered would keep the hub buffering frames into a
// channel nobody reads, for the rest of the process's life.
func TestShutdown_StreamFramesReleasesItsHubSubscription(t *testing.T) {
	// A session must be open for Subscribe to hand out a channel at all.
	sock, tok, tokVal := testSetup(t)
	hub := frames.NewHub()
	hub.BeginSession()
	cfg := Config{
		SocketPath:    sock,
		TokenPath:     tok,
		Token:         tokVal,
		ServerVersion: "0.1.0-test",
		Frames:        hub,
	}
	_, cancel, errCh := startTestServer(t, cfg)

	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	client, err := Dial(ctx, sock, tokVal)
	if err != nil {
		cancel()
		t.Fatalf("dial: %v", err)
	}
	defer client.Close()

	if _, err := client.StreamFrames(ctx, &phonebridgelocalipcv1.StreamFramesRequest{}); err != nil {
		cancel()
		t.Fatalf("open StreamFrames: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && hub.SubscriberCount() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if got := hub.SubscriberCount(); got != 1 {
		cancel()
		t.Fatalf("expected exactly 1 frame subscriber, got %d", got)
	}

	cancel()
	select {
	case <-errCh:
	case <-time.After(10 * time.Second):
		t.Fatalf("Serve did not return within 10s of cancel")
	}

	// EndSession is not needed: the handler must have unsubscribed on its way
	// out, not relied on the session ending.
	if got := hub.SubscriberCount(); got != 0 {
		t.Fatalf("frame subscriptions leaked across shutdown: %d still registered", got)
	}
	_ = tok
}
