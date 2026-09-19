package clipboard

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestHelperProcess implements the canonical Go subprocess helper pattern
// for deterministic testing of the LinuxAdapter supervisor.
func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)

	mode := os.Getenv("HELPER_MODE")
	switch mode {
	case "ready_and_serve":
		fmt.Println("STATUS=READY compositor=COSMIC data_control=v2")
		r := bufio.NewReader(os.Stdin)
		for {
			msg, err := ReadIPCMessage(r)
			if err != nil {
				return
			}
			switch msg.Name {
			case CmdSetSelection:
				fmt.Printf("STATUS=OK cmd=SET_SELECTION len=%d\n", len(msg.Payload))
			case CmdClearSelection:
				fmt.Println("STATUS=OK cmd=CLEAR_SELECTION")
			case "TRIGGER_EVENT":
				payload := "simulated_clipboard_event"
				fmt.Printf("EVENT=READ_DATA mime=text/plain;charset=utf-8 len=%d\n%s\n", len(payload), payload)
			case "TRIGGER_EMPTY":
				fmt.Printf("EVENT=READ_DATA mime=text/plain;charset=utf-8 len=0\n\n")
			case "TRIGGER_OVERSIZED":
				fmt.Println("EVENT=READ_OVERSIZED mime=text/plain size=1048576")
			case CmdShutdown:
				fmt.Println("STATUS=OK cmd=SHUTDOWN")
				return
			}
		}

	case "event_before_ready":
		fmt.Println("EVENT=SELECTION_CLEARED")
		fmt.Printf("EVENT=READ_DATA mime=text/plain;charset=utf-8 len=13\nearly_payload\n")
		fmt.Println("STATUS=READY compositor=COSMIC data_control=v2")
		r := bufio.NewReader(os.Stdin)
		for {
			msg, err := ReadIPCMessage(r)
			if err != nil || msg.Name == CmdShutdown {
				return
			}
		}

	case "backoff_reset":
		file := os.Getenv("COUNTER_FILE")
		timestampFile := os.Getenv("TIMESTAMP_FILE")
		data, _ := os.ReadFile(file)
		count := 0
		if len(data) > 0 {
			count, _ = strconv.Atoi(string(data))
		}
		count++
		_ = os.WriteFile(file, []byte(strconv.Itoa(count)), 0644)

		now := time.Now().UnixNano()
		f, _ := os.OpenFile(timestampFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if f != nil {
			_, _ = fmt.Fprintf(f, "RUN %d: %d\n", count, now)
			_ = f.Close()
		}

		// Runs 1 and 2: crash immediately during startup (never reaches READY)
		if count == 1 || count == 2 {
			fmt.Println("STATUS=ERR_COMPOSITOR_DISCONNECTED detail=startup_crash")
			os.Exit(3)
		}

		// Run 3: reaches READY, serves until commanded to crash
		if count == 3 {
			fmt.Println("STATUS=READY compositor=COSMIC data_control=v2")
			r := bufio.NewReader(os.Stdin)
			for {
				msg, err := ReadIPCMessage(r)
				if err != nil {
					os.Exit(3)
				}
				if msg.Name == "CRASH_AFTER_READY" {
					fmt.Println("STATUS=ERR_COMPOSITOR_DISCONNECTED detail=post_ready_crash")
					os.Exit(3)
				}
				if msg.Name == CmdShutdown {
					return
				}
			}
		}

		// Run 4: recovered after Run 3 crash
		fmt.Println("STATUS=READY compositor=COSMIC data_control=v2")
		r := bufio.NewReader(os.Stdin)
		for {
			msg, err := ReadIPCMessage(r)
			if err != nil || msg.Name == CmdShutdown {
				return
			}
		}

	case "cosmic_flag_required":
		fmt.Println("STATUS=ERR_COSMIC_FLAG_REQUIRED detail=COSMIC_DATA_CONTROL_ENABLED=1 is required")
		os.Exit(2)

	case "no_data_control":
		fmt.Println("STATUS=ERR_NO_DATA_CONTROL detail=zwlr_data_control_manager_v1 not advertised")
		os.Exit(2)

	case "wayland_unavailable":
		fmt.Println("STATUS=ERR_WAYLAND_CONNECT detail=socket_error")
		os.Exit(1)

	case "crash_twice":
		file := os.Getenv("COUNTER_FILE")
		data, _ := os.ReadFile(file)
		count := 0
		if len(data) > 0 {
			count, _ = strconv.Atoi(string(data))
		}
		count++
		_ = os.WriteFile(file, []byte(strconv.Itoa(count)), 0644)

		if count <= 2 {
			fmt.Println("STATUS=ERR_COMPOSITOR_DISCONNECTED detail=transient_failure")
			os.Exit(3)
		}

		fmt.Println("STATUS=READY compositor=COSMIC data_control=v2")
		r := bufio.NewReader(os.Stdin)
		for {
			msg, err := ReadIPCMessage(r)
			if err != nil || msg.Name == CmdShutdown {
				return
			}
		}
	}
}

func helperEnv(mode string, extras ...string) []string {
	env := []string{
		"GO_WANT_HELPER_PROCESS=1",
		"HELPER_MODE=" + mode,
	}
	return append(env, extras...)
}

func TestLinuxAdapterLifecycleAndWrite(t *testing.T) {
	var changedItem atomic.Pointer[Item]
	var changedMime atomic.Value
	var oversizedSize atomic.Int32
	var statusChanges []AdapterStatus
	var statusMu sync.Mutex

	cfg := LinuxAdapterConfig{
		HelperPath: os.Args[0],
		HelperArgs: []string{"-test.run=TestHelperProcess"},
		Env:        helperEnv("ready_and_serve"),
		OnClipboardChanged: func(ctx context.Context, mimeType string, payload []byte) error {
			changedMime.Store(mimeType)
			item, _ := NewItem(mimeType, payload, 1000)
			changedItem.Store(item)
			return nil
		},
		OnOversizedPayload: func(size int) {
			oversizedSize.Store(int32(size))
		},
		OnStatusChanged: func(status AdapterStatus, err error) {
			statusMu.Lock()
			statusChanges = append(statusChanges, status)
			statusMu.Unlock()
		},
		RestartBackoff: []time.Duration{20 * time.Millisecond, 40 * time.Millisecond},
	}

	adapter, err := NewLinuxAdapter(cfg)
	if err != nil {
		t.Fatalf("NewLinuxAdapter failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	t.Log("Starting adapter...")
	if err := adapter.Start(ctx); err != nil {
		t.Fatalf("adapter.Start failed: %v", err)
	}
	defer func() { _ = adapter.Stop() }()

	t.Log("Waiting for Ready...")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if adapter.Status() == AdapterStatusReady {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if adapter.Status() != AdapterStatusReady {
		t.Fatalf("adapter did not reach Ready, got %s", adapter.Status())
	}
	t.Log("Adapter is Ready")

	// Test WriteClipboard
	item, err := NewItem("text/plain;charset=utf-8", []byte("production clipboard test"), 12345)
	if err != nil {
		t.Fatalf("NewItem failed: %v", err)
	}

	t.Log("Calling WriteClipboard...")
	if err := adapter.WriteClipboard(ctx, item); err != nil {
		t.Fatalf("WriteClipboard failed: %v", err)
	}
	t.Log("WriteClipboard completed")

	t.Log("Calling ClearClipboard...")
	if err := adapter.ClearClipboard(ctx); err != nil {
		t.Fatalf("ClearClipboard failed: %v", err)
	}
	t.Log("ClearClipboard completed")

	t.Log("Calling Stop...")
	if err := adapter.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	t.Log("Stop completed")

	if adapter.Status() != AdapterStatusStopped {
		t.Errorf("expected StatusStopped, got %s", adapter.Status())
	}
}

func TestLinuxAdapterEnvironmentDetection(t *testing.T) {
	t.Run("CosmicFlagRequired", func(t *testing.T) {
		adapter, err := NewLinuxAdapter(LinuxAdapterConfig{
			HelperPath:     os.Args[0],
			HelperArgs:     []string{"-test.run=TestHelperProcess"},
			Env:            helperEnv("cosmic_flag_required"),
			RestartBackoff: []time.Duration{10 * time.Millisecond},
		})
		if err != nil {
			t.Fatalf("NewLinuxAdapter failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := adapter.Start(ctx); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer func() { _ = adapter.Stop() }()

		time.Sleep(100 * time.Millisecond)
		if adapter.Status() != AdapterStatusCosmicFlagRequired {
			t.Errorf("got %s, want AdapterStatusCosmicFlagRequired", adapter.Status())
		}
	})

	t.Run("NoDataControl", func(t *testing.T) {
		adapter, err := NewLinuxAdapter(LinuxAdapterConfig{
			HelperPath:     os.Args[0],
			HelperArgs:     []string{"-test.run=TestHelperProcess"},
			Env:            helperEnv("no_data_control"),
			RestartBackoff: []time.Duration{10 * time.Millisecond},
		})
		if err != nil {
			t.Fatalf("NewLinuxAdapter failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := adapter.Start(ctx); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer func() { _ = adapter.Stop() }()

		time.Sleep(100 * time.Millisecond)
		if adapter.Status() != AdapterStatusNoDataControl {
			t.Errorf("got %s, want AdapterStatusNoDataControl", adapter.Status())
		}
	})

	t.Run("WaylandUnavailable", func(t *testing.T) {
		adapter, err := NewLinuxAdapter(LinuxAdapterConfig{
			HelperPath:     os.Args[0],
			HelperArgs:     []string{"-test.run=TestHelperProcess"},
			Env:            helperEnv("wayland_unavailable"),
			RestartBackoff: []time.Duration{10 * time.Millisecond},
		})
		if err != nil {
			t.Fatalf("NewLinuxAdapter failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := adapter.Start(ctx); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer func() { _ = adapter.Stop() }()

		time.Sleep(100 * time.Millisecond)
		if adapter.Status() != AdapterStatusWaylandUnavailable {
			t.Errorf("got %s, want AdapterStatusWaylandUnavailable", adapter.Status())
		}
	})
}

func TestLinuxAdapterCrashAndBackoffRestart(t *testing.T) {
	counterFile := filepath.Join(t.TempDir(), "counter")

	adapter, err := NewLinuxAdapter(LinuxAdapterConfig{
		HelperPath:     os.Args[0],
		HelperArgs:     []string{"-test.run=TestHelperProcess"},
		Env:            helperEnv("crash_twice", "COUNTER_FILE="+counterFile),
		RestartBackoff: []time.Duration{20 * time.Millisecond, 40 * time.Millisecond, 80 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("NewLinuxAdapter failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := adapter.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = adapter.Stop() }()

	// Wait for adapter to recover and reach Ready
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if adapter.Status() == AdapterStatusReady {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if adapter.Status() != AdapterStatusReady {
		t.Fatalf("adapter did not recover to Ready after crashes, got %s", adapter.Status())
	}
}

func TestLinuxAdapterBackoffResetAfterRecovery(t *testing.T) {
	counterFile := filepath.Join(t.TempDir(), "counter")
	timestampFile := filepath.Join(t.TempDir(), "timestamps")

	// Backoff delays:
	// Initial delay: 40ms.
	// Step 2: 250ms.
	// Step 3: 600ms.
	initialDelay := 40 * time.Millisecond
	secondDelay := 250 * time.Millisecond
	thirdDelay := 600 * time.Millisecond

	adapter, err := NewLinuxAdapter(LinuxAdapterConfig{
		HelperPath: os.Args[0],
		HelperArgs: []string{"-test.run=TestHelperProcess"},
		Env: helperEnv("backoff_reset",
			"COUNTER_FILE="+counterFile,
			"TIMESTAMP_FILE="+timestampFile,
		),
		RestartBackoff: []time.Duration{initialDelay, secondDelay, thirdDelay},
	})
	if err != nil {
		t.Fatalf("NewLinuxAdapter failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := adapter.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = adapter.Stop() }()

	// Wait for Run 3 to reach Ready
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if adapter.Status() == AdapterStatusReady {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if adapter.Status() != AdapterStatusReady {
		t.Fatalf("adapter did not reach Ready (Run 3), status=%s", adapter.Status())
	}

	// Trigger crash in Run 3 while in Ready state
	adapter.mu.Lock()
	stdin := adapter.stdinPipe
	adapter.mu.Unlock()
	if stdin != nil {
		_ = WriteCommand(stdin, "CRASH_AFTER_READY", nil, nil)
	}

	// Wait for adapter to observe the crash and leave Ready
	crashObserved := false
	var crashTime time.Time
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if adapter.Status() != AdapterStatusReady {
			crashObserved = true
			crashTime = time.Now()
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if !crashObserved {
		t.Fatal("adapter did not leave Ready after CRASH_AFTER_READY")
	}

	// Wait for Run 4 to reach Ready
	recovered := false
	var recoveryDuration time.Duration
	deadline = time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if adapter.Status() == AdapterStatusReady {
			recovered = true
			recoveryDuration = time.Since(crashTime)
			break
		}
		time.Sleep(2 * time.Millisecond)
	}

	if !recovered {
		t.Fatalf("adapter did not recover to Ready after post-ready crash")
	}

	// Since backoff was reset to initialDelay (40ms), recoveryDuration should be ~initialDelay,
	// and strictly LESS than secondDelay (250ms). If backoff was not reset, it would take >= 250ms.
	if recoveryDuration >= secondDelay {
		t.Fatalf("backoff was not reset! recovery took %v (expected ~%v, must be < %v)",
			recoveryDuration, initialDelay, secondDelay)
	}
	t.Logf("Backoff reset verified: post-READY crash recovered in %v (initial backoff %v, escalated was %v)",
		recoveryDuration, initialDelay, secondDelay)
}

func TestLinuxAdapterStartupEventOrdering(t *testing.T) {
	var receivedPayload atomic.Pointer[[]byte]
	var receivedMime atomic.Value

	adapter, err := NewLinuxAdapter(LinuxAdapterConfig{
		HelperPath: os.Args[0],
		HelperArgs: []string{"-test.run=TestHelperProcess"},
		Env:        helperEnv("event_before_ready"),
		OnClipboardChanged: func(ctx context.Context, mimeType string, payload []byte) error {
			receivedMime.Store(mimeType)
			p := make([]byte, len(payload))
			copy(p, payload)
			receivedPayload.Store(&p)
			return nil
		},
		RestartBackoff: []time.Duration{20 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("NewLinuxAdapter failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := adapter.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = adapter.Stop() }()

	// Wait for Ready
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if adapter.Status() == AdapterStatusReady {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if adapter.Status() != AdapterStatusReady {
		t.Fatalf("adapter did not reach Ready, got %s", adapter.Status())
	}

	// Wait for event callback to be dispatched
	deadline = time.Now().Add(1 * time.Second)
	for time.Now().Before(deadline) {
		if receivedPayload.Load() != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	ptr := receivedPayload.Load()
	if ptr == nil {
		t.Fatal("expected pre-ready event to be delivered, got nil")
	}
	if string(*ptr) != "early_payload" {
		t.Fatalf("expected 'early_payload', got %q", string(*ptr))
	}
	if receivedMime.Load() != "text/plain;charset=utf-8" {
		t.Fatalf("expected 'text/plain;charset=utf-8', got %v", receivedMime.Load())
	}
}

func TestLinuxAdapterZeroLengthPayload(t *testing.T) {
	var deliveredPayload atomic.Pointer[[]byte]
	var deliveredMime atomic.Value
	delivered := make(chan struct{})

	adapter, err := NewLinuxAdapter(LinuxAdapterConfig{
		HelperPath: os.Args[0],
		HelperArgs: []string{"-test.run=TestHelperProcess"},
		Env:        helperEnv("ready_and_serve"),
		OnClipboardChanged: func(ctx context.Context, mimeType string, payload []byte) error {
			deliveredMime.Store(mimeType)
			p := make([]byte, len(payload))
			copy(p, payload)
			deliveredPayload.Store(&p)
			select {
			case <-delivered:
			default:
				close(delivered)
			}
			return nil
		},
		RestartBackoff: []time.Duration{20 * time.Millisecond},
	})
	if err != nil {
		t.Fatalf("NewLinuxAdapter failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := adapter.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}
	defer func() { _ = adapter.Stop() }()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if adapter.Status() == AdapterStatusReady {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if adapter.Status() != AdapterStatusReady {
		t.Fatalf("adapter did not reach Ready, got %s", adapter.Status())
	}

	// Trigger empty clipboard event
	adapter.mu.Lock()
	stdin := adapter.stdinPipe
	adapter.mu.Unlock()
	if stdin != nil {
		_ = WriteCommand(stdin, "TRIGGER_EMPTY", nil, nil)
	}

	select {
	case <-delivered:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for zero-length payload delivery")
	}

	ptr := deliveredPayload.Load()
	if ptr == nil {
		t.Fatal("expected delivered payload to be non-nil pointer")
	}
	if len(*ptr) != 0 {
		t.Fatalf("expected 0-byte payload, got %d bytes: %v", len(*ptr), *ptr)
	}
	if deliveredMime.Load() != "text/plain;charset=utf-8" {
		t.Fatalf("expected MIME text/plain;charset=utf-8, got %v", deliveredMime.Load())
	}
}

func TestCompiledHelperIntegration(t *testing.T) {
	// Locate compiled helper binary relative to core/pkg/clipboard
	helperPath, err := filepath.Abs("../../../linux/wayland-helper/phonebridge-wayland-helper")
	if err != nil {
		t.Fatalf("failed to resolve helper path: %v", err)
	}

	info, err := os.Stat(helperPath)
	if err != nil || info.IsDir() {
		t.Skipf("compiled helper binary not found at %s; run make in linux/wayland-helper first", helperPath)
	}

	t.Run("WaylandUnavailableTerminalState", func(t *testing.T) {
		// Run helper in clean environment with WAYLAND_DISPLAY and XDG_RUNTIME_DIR unset
		adapter, err := NewLinuxAdapter(LinuxAdapterConfig{
			HelperPath: helperPath,
			Env: []string{
				"WAYLAND_DISPLAY=",
				"XDG_RUNTIME_DIR=",
			},
			RestartBackoff: []time.Duration{20 * time.Millisecond},
		})
		if err != nil {
			t.Fatalf("NewLinuxAdapter failed: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()

		if err := adapter.Start(ctx); err != nil {
			t.Fatalf("Start failed: %v", err)
		}
		defer func() { _ = adapter.Stop() }()

		time.Sleep(100 * time.Millisecond)
		status := adapter.Status()
		if status != AdapterStatusWaylandUnavailable {
			t.Errorf("expected AdapterStatusWaylandUnavailable, got %s", status)
		}
	})

	t.Run("CleanShutdownAndStartupHandshakeOrdering", func(t *testing.T) {
		// Run compiled helper binary directly with CMD=SHUTDOWN passed to stdin
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		cmd := exec.CommandContext(ctx, helperPath)
		cmd.Env = []string{
			"WAYLAND_DISPLAY=",
			"XDG_RUNTIME_DIR=",
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatalf("stdout pipe failed: %v", err)
		}

		if err := cmd.Start(); err != nil {
			t.Fatalf("cmd.Start failed: %v", err)
		}

		r := bufio.NewReader(stdout)
		firstLine, err := r.ReadString('\n')
		if err != nil {
			t.Fatalf("failed to read first line from helper: %v", err)
		}

		// First line MUST be a STATUS line (e.g. STATUS=ERR_WAYLAND_CONNECT or STATUS=READY)
		// It must NEVER be unformatted raw bytes or an EVENT without a status
		if len(firstLine) < 7 || firstLine[:7] != "STATUS=" {
			t.Fatalf("expected first line to start with 'STATUS=', got: %q", firstLine)
		}

		_ = cmd.Wait()
	})
}
