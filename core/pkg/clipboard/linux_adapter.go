package clipboard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// AdapterStatus represents the operational state of the Linux Wayland clipboard adapter.
type AdapterStatus int

const (
	AdapterStatusStopped AdapterStatus = iota
	AdapterStatusStarting
	AdapterStatusReady
	AdapterStatusWaylandUnavailable
	AdapterStatusCosmicFlagRequired
	AdapterStatusNoDataControl
	AdapterStatusCompositorDisconnected
	AdapterStatusCrashed
)

func (s AdapterStatus) String() string {
	switch s {
	case AdapterStatusStopped:
		return "STOPPED"
	case AdapterStatusStarting:
		return "STARTING"
	case AdapterStatusReady:
		return "READY"
	case AdapterStatusWaylandUnavailable:
		return "WAYLAND_UNAVAILABLE"
	case AdapterStatusCosmicFlagRequired:
		return "COSMIC_FLAG_REQUIRED"
	case AdapterStatusNoDataControl:
		return "NO_DATA_CONTROL"
	case AdapterStatusCompositorDisconnected:
		return "COMPOSITOR_DISCONNECTED"
	case AdapterStatusCrashed:
		return "CRASHED"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(s))
	}
}

var (
	ErrHelperNotReady     = errors.New("clipboard: Wayland helper is not ready")
	ErrAdapterStopped     = errors.New("clipboard: Linux adapter is stopped")
	ErrCommandTimeout     = errors.New("clipboard: helper command timed out")
	ErrCosmicFlagRequired = errors.New("clipboard: COSMIC_DATA_CONTROL_ENABLED=1 is required in cosmic-comp")
	ErrNoDataControl      = errors.New("clipboard: zwlr_data_control_manager_v1 is not advertised by compositor")
	ErrWaylandUnavailable = errors.New("clipboard: Wayland display or socket is unavailable")
)

var defaultRestartBackoff = []time.Duration{
	500 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
	5 * time.Second,
}

// LinuxAdapterConfig configures the Linux Wayland clipboard platform adapter.
type LinuxAdapterConfig struct {
	// HelperPath is the absolute or relative path to the phonebridge-wayland-helper binary.
	HelperPath string

	// HelperArgs optionally specifies arguments passed to the helper process.
	HelperArgs []string

	// OnClipboardChanged is invoked when a new clipboard item is read from Wayland.
	// Typically forwards to Engine.OnLocalCopy or Engine.OnLocalClipboard.
	OnClipboardChanged func(ctx context.Context, mimeType string, payload []byte) error

	// OnOversizedPayload is invoked when an offer exceeds the 768 KiB ceiling.
	OnOversizedPayload func(size int)

	// OnStatusChanged notifies when adapter status changes.
	OnStatusChanged func(status AdapterStatus, err error)

	// RestartBackoff overrides the default backoff sequence (500ms, 1s, 2s, 4s, 5s).
	RestartBackoff []time.Duration

	// Env specifies additional environment variables for the helper process.
	Env []string

	// Clock provides time operations. If nil, real system time is used.
	Clock Clock
}

// LinuxAdapter supervises the phonebridge-wayland-helper C subprocess and implements
// PlatformAdapter for Wayland/COSMIC environments (DEC-023).
type LinuxAdapter struct {
	cfg     LinuxAdapterConfig
	clock   Clock
	backoff []time.Duration
	status  atomic.Int32

	mu        sync.Mutex
	writeMu   sync.Mutex
	cmd       *exec.Cmd
	stdinPipe io.WriteCloser
	stopCh    chan struct{}
	stopped   bool

	// Response channel for synchronizing CMD -> STATUS=OK/ERROR responses
	respMu sync.Mutex
	respCh chan *IPCMessage

	// WaitGroup tracking supervisor goroutines
	wg sync.WaitGroup
}

// NewLinuxAdapter constructs a LinuxAdapter with the given configuration.
func NewLinuxAdapter(cfg LinuxAdapterConfig) (*LinuxAdapter, error) {
	clock := cfg.Clock
	if clock == nil {
		clock = realClock{}
	}

	backoff := cfg.RestartBackoff
	if len(backoff) == 0 {
		backoff = defaultRestartBackoff
	}

	helperPath := cfg.HelperPath
	if helperPath == "" {
		helperPath = "phonebridge-wayland-helper"
	}
	cfg.HelperPath = helperPath

	a := &LinuxAdapter{
		cfg:     cfg,
		clock:   clock,
		backoff: backoff,
		stopCh:  make(chan struct{}),
		respCh:  make(chan *IPCMessage, 1),
	}
	a.status.Store(int32(AdapterStatusStopped))

	return a, nil
}

// Status returns the current operational status of the adapter.
func (a *LinuxAdapter) Status() AdapterStatus {
	return AdapterStatus(a.status.Load())
}

func (a *LinuxAdapter) setStatus(status AdapterStatus, err error) {
	old := AdapterStatus(a.status.Swap(int32(status)))
	if old != status && a.cfg.OnStatusChanged != nil {
		a.cfg.OnStatusChanged(status, err)
	}
}

// Start launches the supervisor loop in the background.
func (a *LinuxAdapter) Start(ctx context.Context) error {
	a.mu.Lock()
	if !a.stopped && a.cmd != nil {
		a.mu.Unlock()
		return nil // already running
	}
	a.stopped = false
	a.stopCh = make(chan struct{})
	a.mu.Unlock()

	a.wg.Add(1)
	go a.supervisorLoop(ctx)
	return nil
}

// Stop cleanly terminates the helper process and halts the supervisor loop.
func (a *LinuxAdapter) Stop() error {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return nil
	}
	a.stopped = true
	close(a.stopCh)

	// Send SHUTDOWN command if helper is running
	_ = a.sendShutdownLocked()

	cmd := a.cmd
	a.mu.Unlock()

	// Wait for supervisor goroutine to exit
	waitDone := make(chan struct{})
	go func() {
		a.wg.Wait()
		close(waitDone)
	}()

	select {
	case <-waitDone:
	case <-time.After(1 * time.Second):
		if cmd != nil && cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		<-waitDone
	}

	a.setStatus(AdapterStatusStopped, nil)
	return nil
}

// WriteClipboard implements PlatformAdapter. It writes the given clipboard item
// to the Wayland selection via the helper subprocess.
func (a *LinuxAdapter) WriteClipboard(ctx context.Context, item *Item) error {
	if item == nil {
		return ErrMalformedUpdate
	}

	if len(item.Payload) > MaxPayloadSize {
		if a.cfg.OnOversizedPayload != nil {
			a.cfg.OnOversizedPayload(len(item.Payload))
		}
		return &OversizedPayloadError{Size: len(item.Payload), MaxSize: MaxPayloadSize}
	}

	if a.Status() != AdapterStatusReady {
		return ErrHelperNotReady
	}

	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	a.mu.Lock()
	stdin := a.stdinPipe
	a.mu.Unlock()

	if stdin == nil {
		return ErrHelperNotReady
	}

	// Prepare response channel
	a.respMu.Lock()
	// Drain any stale response
	select {
	case <-a.respCh:
	default:
	}
	a.respMu.Unlock()

	params := map[string]string{
		"mime": item.MimeType,
	}

	if err := WriteCommand(stdin, CmdSetSelection, params, item.Payload); err != nil {
		return fmt.Errorf("clipboard: failed to send SET_SELECTION: %w", err)
	}

	// Wait for response
	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp, ok := <-a.respCh:
		if !ok || resp == nil {
			return ErrHelperNotReady
		}
		if resp.Type == "STATUS" && resp.Name == StatusOk {
			return nil
		}
		if resp.Type == "STATUS" && resp.Name == StatusError {
			return fmt.Errorf("clipboard: helper rejected SET_SELECTION: %s", resp.Params["detail"])
		}
		return nil
	case <-time.After(2 * time.Second):
		return ErrCommandTimeout
	}
}

// ClearClipboard clears the Wayland selection.
func (a *LinuxAdapter) ClearClipboard(ctx context.Context) error {
	if a.Status() != AdapterStatusReady {
		return ErrHelperNotReady
	}

	a.writeMu.Lock()
	defer a.writeMu.Unlock()

	a.mu.Lock()
	stdin := a.stdinPipe
	a.mu.Unlock()

	if stdin == nil {
		return ErrHelperNotReady
	}

	a.respMu.Lock()
	select {
	case <-a.respCh:
	default:
	}
	a.respMu.Unlock()

	if err := WriteCommand(stdin, CmdClearSelection, nil, nil); err != nil {
		return fmt.Errorf("clipboard: failed to send CLEAR_SELECTION: %w", err)
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case resp, ok := <-a.respCh:
		if !ok || resp == nil {
			return ErrHelperNotReady
		}
		if resp.Type == "STATUS" && resp.Name == StatusOk {
			return nil
		}
		return nil
	case <-time.After(2 * time.Second):
		return ErrCommandTimeout
	}
}

func (a *LinuxAdapter) sendShutdownLocked() error {
	if a.stdinPipe != nil {
		_ = WriteCommand(a.stdinPipe, CmdShutdown, nil, nil)
		_ = a.stdinPipe.Close()
		a.stdinPipe = nil
	}
	return nil
}

// supervisorLoop continuously runs the helper process with bounded backoff restart.
func (a *LinuxAdapter) supervisorLoop(ctx context.Context) {
	defer a.wg.Done()

	backoffIdx := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		default:
		}

		a.setStatus(AdapterStatusStarting, nil)

		exitStatus, wasReady, err := a.runHelperInstance(ctx)

		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		default:
		}

		// Handle terminal environment states (do not spin-restart indefinitely)
		if exitStatus == AdapterStatusCosmicFlagRequired ||
			exitStatus == AdapterStatusNoDataControl ||
			exitStatus == AdapterStatusWaylandUnavailable {
			a.setStatus(exitStatus, err)
			return
		}

		// Reset failure backoff if the helper had successfully reached READY
		if wasReady {
			backoffIdx = 0
		}

		// Crashed or disconnected: apply bounded backoff restart
		a.setStatus(exitStatus, err)

		delay := a.backoff[backoffIdx]
		if backoffIdx < len(a.backoff)-1 {
			backoffIdx++
		}

		select {
		case <-ctx.Done():
			return
		case <-a.stopCh:
			return
		case <-time.After(delay):
		}
	}
}

// runHelperInstance runs a single instance of the helper process and monitors its output.
func (a *LinuxAdapter) runHelperInstance(ctx context.Context) (AdapterStatus, bool, error) {
	cmd := exec.CommandContext(ctx, a.cfg.HelperPath, a.cfg.HelperArgs...)
	if len(a.cfg.Env) > 0 {
		cmd.Env = append(os.Environ(), a.cfg.Env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return AdapterStatusCrashed, false, fmt.Errorf("stdin pipe failed: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return AdapterStatusCrashed, false, fmt.Errorf("stdout pipe failed: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return AdapterStatusCrashed, false, fmt.Errorf("stderr pipe failed: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return AdapterStatusCrashed, false, fmt.Errorf("helper start failed: %w", err)
	}

	a.mu.Lock()
	a.cmd = cmd
	a.stdinPipe = stdin
	a.mu.Unlock()

	// Drain stderr in background (Zero Logging: discard or internal debug)
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			// Intentionally silent or diagnostic; never log clipboard contents
		}
	}()

	stdoutReader := bufio.NewReader(stdout)

	// Initial handshake: read status message, buffering any preceding events
	var initialEvents []*IPCMessage
	var firstMsg *IPCMessage
	for {
		msg, readErr := ReadIPCMessage(stdoutReader)
		if readErr != nil {
			_ = cmd.Wait()
			return AdapterStatusCrashed, false, fmt.Errorf("failed to read initial helper status: %w", readErr)
		}
		if msg.Type == "STATUS" {
			firstMsg = msg
			break
		}
		if msg.Type == "EVENT" {
			initialEvents = append(initialEvents, msg)
		}
	}

	var exitStatus AdapterStatus = AdapterStatusCrashed
	var wasReady bool

	if firstMsg.Type == "STATUS" {
		switch firstMsg.Name {
		case StatusReady:
			a.setStatus(AdapterStatusReady, nil)
			wasReady = true
			for _, evt := range initialEvents {
				a.handleEvent(ctx, evt)
			}
			exitStatus = a.processStdoutStream(ctx, stdoutReader)
		case StatusErrCosmicFlagRequired:
			exitStatus = AdapterStatusCosmicFlagRequired
			err = ErrCosmicFlagRequired
		case StatusErrNoDataControl:
			exitStatus = AdapterStatusNoDataControl
			err = ErrNoDataControl
		case StatusErrWaylandConnect:
			exitStatus = AdapterStatusWaylandUnavailable
			err = ErrWaylandUnavailable
		case StatusErrNoSeat:
			exitStatus = AdapterStatusWaylandUnavailable
			err = errors.New("clipboard: wl_seat not found")
		default:
			exitStatus = AdapterStatusCrashed
			err = fmt.Errorf("clipboard: unexpected initial status: %s", firstMsg.Name)
		}
	}

	_ = cmd.Wait()

	a.mu.Lock()
	a.cmd = nil
	a.stdinPipe = nil
	a.mu.Unlock()

	return exitStatus, wasReady, err
}

// processStdoutStream reads events and command responses until EOF or error.
func (a *LinuxAdapter) processStdoutStream(ctx context.Context, r *bufio.Reader) AdapterStatus {
	for {
		msg, err := ReadIPCMessage(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return AdapterStatusCrashed
			}
			return AdapterStatusCrashed
		}

		switch msg.Type {
		case "STATUS":
			if msg.Name == StatusErrCompositorDisconn {
				return AdapterStatusCompositorDisconnected
			}
			// Command response (OK / ERROR)
			a.respMu.Lock()
			select {
			case a.respCh <- msg:
			default:
			}
			a.respMu.Unlock()

		case "EVENT":
			a.handleEvent(ctx, msg)
		}
	}
}

func (a *LinuxAdapter) handleEvent(ctx context.Context, msg *IPCMessage) {
	switch msg.Name {
	case EventReadData:
		mime := msg.Params["mime"]
		if a.cfg.OnClipboardChanged != nil && msg.Payload != nil {
			// Invoke outside locks
			_ = a.cfg.OnClipboardChanged(ctx, mime, msg.Payload)
		}
	case EventReadOversized:
		sizeStr := msg.Params["size"]
		if size, err := strconv.Atoi(sizeStr); err == nil && a.cfg.OnOversizedPayload != nil {
			a.cfg.OnOversizedPayload(size)
		}
	case EventSelectionCleared:
		// Optional cleared notification
	case EventSourceCancelled:
		// Selection replaced by external client
	case EventUnsupportedOffer:
		// Offered MIME is non-text
	}
}
