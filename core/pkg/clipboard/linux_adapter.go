package clipboard

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Backend represents the desktop compositor clipboard integration backend.
type Backend string

const (
	BackendNone    Backend = "none"
	BackendMutter  Backend = "mutter"
	BackendWlroots Backend = "wlroots"
	BackendCustom  Backend = "custom"
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
	AdapterStatusNoBackend
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
	case AdapterStatusNoBackend:
		return "NO_BACKEND"
	default:
		return fmt.Sprintf("UNKNOWN(%d)", int(s))
	}
}

var (
	ErrHelperNotReady     = errors.New("clipboard: helper is not ready")
	ErrAdapterStopped     = errors.New("clipboard: Linux adapter is stopped")
	ErrCommandTimeout     = errors.New("clipboard: helper command timed out")
	ErrCosmicFlagRequired = errors.New("clipboard: COSMIC_DATA_CONTROL_ENABLED=1 is required in cosmic-comp")
	ErrNoDataControl      = errors.New("clipboard: zwlr_data_control_manager_v1 is not advertised by compositor")
	ErrWaylandUnavailable = errors.New("clipboard: Wayland display or socket is unavailable")
	ErrNoBackend          = errors.New("clipboard: no supported compositor clipboard backend found")
)

var defaultRestartBackoff = []time.Duration{
	500 * time.Millisecond,
	1 * time.Second,
	2 * time.Second,
	4 * time.Second,
	5 * time.Second,
}

// LinuxAdapterConfig configures the Linux clipboard platform adapter (DEC-023).
type LinuxAdapterConfig struct {
	// HelperPath is the absolute or relative path to the helper binary.
	// If empty, dynamic backend auto-detection probes between Mutter and Wayland/wlroots.
	HelperPath string

	// Backend optionally forces or hints the backend type (BackendMutter, BackendWlroots, etc.).
	Backend Backend

	// HelperArgs optionally specifies arguments passed to the helper process.
	HelperArgs []string

	// OnClipboardChanged is invoked when a new clipboard item is read from the compositor.
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

// LinuxAdapter supervises an isolated clipboard helper C subprocess and implements
// PlatformAdapter for Linux Wayland and GNOME/Mutter desktop environments (DEC-023).
type LinuxAdapter struct {
	cfg     LinuxAdapterConfig
	clock   Clock
	backoff []time.Duration
	status  atomic.Int32

	activeBackend  Backend
	resolvedHelper string

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
		if envHelper := os.Getenv("PHONEBRIDGE_CLIPBOARD_HELPER"); envHelper != "" {
			helperPath = envHelper
		} else if envWayland := os.Getenv("PHONEBRIDGE_WAYLAND_HELPER"); envWayland != "" {
			helperPath = envWayland
		}
	}
	cfg.HelperPath = helperPath

	activeBackend := cfg.Backend
	if activeBackend == "" && helperPath != "" {
		activeBackend = deriveBackend(helperPath)
	}

	a := &LinuxAdapter{
		cfg:           cfg,
		clock:         clock,
		backoff:       backoff,
		activeBackend: activeBackend,
		stopCh:        make(chan struct{}),
		respCh:        make(chan *IPCMessage, 1),
	}
	a.status.Store(int32(AdapterStatusStopped))

	return a, nil
}

// Backend returns the active desktop compositor clipboard mechanism.
func (a *LinuxAdapter) Backend() Backend {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.activeBackend == "" {
		return BackendNone
	}
	return a.activeBackend
}

// HelperPath returns the configured or auto-detected helper binary path.
func (a *LinuxAdapter) HelperPath() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cfg.HelperPath != "" {
		return a.cfg.HelperPath
	}
	return a.resolvedHelper
}

// Diagnostics returns operational metadata without any clipboard content.
func (a *LinuxAdapter) Diagnostics() map[string]string {
	return map[string]string{
		"backend":     string(a.Backend()),
		"helper_path": a.HelperPath(),
		"status":      a.Status().String(),
	}
}

func (a *LinuxAdapter) setActiveBackend(b Backend) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.activeBackend = b
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

		// Dynamic auto-detection when HelperPath is not statically fixed
		if a.cfg.HelperPath == "" && a.resolvedHelper == "" {
			det, err := DetectBackend(ctx, a.cfg.Env)
			if err != nil || det.HelperPath == "" {
				exitStatus := AdapterStatusNoBackend
				if errors.Is(err, ErrNoDataControl) {
					exitStatus = AdapterStatusNoDataControl
				} else if errors.Is(err, ErrCosmicFlagRequired) {
					exitStatus = AdapterStatusCosmicFlagRequired
				} else if errors.Is(err, ErrWaylandUnavailable) {
					exitStatus = AdapterStatusWaylandUnavailable
				}

				a.setStatus(exitStatus, err)

				if exitStatus == AdapterStatusCosmicFlagRequired {
					return
				}

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
					continue
				}
			}

			a.mu.Lock()
			a.resolvedHelper = det.HelperPath
			if a.activeBackend == "" || a.activeBackend == BackendNone {
				a.activeBackend = det.Backend
			}
			a.mu.Unlock()
		}

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
		} else if a.cfg.HelperPath == "" {
			// Clear resolvedHelper to re-detect on retry
			a.mu.Lock()
			a.resolvedHelper = ""
			a.activeBackend = BackendNone
			a.mu.Unlock()
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
	helperPath := a.HelperPath()
	if helperPath == "" {
		return AdapterStatusNoBackend, false, ErrNoBackend
	}

	cmd := exec.CommandContext(ctx, helperPath, a.cfg.HelperArgs...)
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
			if b := firstMsg.Params["backend"]; b != "" {
				a.setActiveBackend(Backend(b))
			} else if _, ok := firstMsg.Params["data_control"]; ok {
				a.setActiveBackend(BackendWlroots)
			}
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
		case "ERR_NO_MUTTER":
			exitStatus = AdapterStatusNoBackend
			err = fmt.Errorf("clipboard: mutter remote desktop error: %s", firstMsg.Params["detail"])
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

// deriveBackend infers backend type from the helper filename.
func deriveBackend(helperPath string) Backend {
	base := filepath.Base(helperPath)
	if strings.Contains(base, "mutter") {
		return BackendMutter
	}
	if strings.Contains(base, "wayland") || strings.Contains(base, "wlroots") {
		return BackendWlroots
	}
	return BackendCustom
}

// isGNOMEDesktop checks whether the desktop environment hints GNOME/Ubuntu.
func isGNOMEDesktop(env []string) bool {
	checkVar := func(key string) string {
		for _, e := range env {
			parts := strings.SplitN(e, "=", 2)
			if len(parts) == 2 && strings.EqualFold(parts[0], key) {
				return parts[1]
			}
		}
		return os.Getenv(key)
	}

	desktop := strings.ToLower(checkVar("XDG_CURRENT_DESKTOP"))
	session := strings.ToLower(checkVar("GDMSESSION"))
	desktopSession := strings.ToLower(checkVar("DESKTOP_SESSION"))

	for _, s := range []string{desktop, session, desktopSession} {
		if strings.Contains(s, "gnome") || strings.Contains(s, "ubuntu") {
			return true
		}
	}
	return false
}

// resolveHelperPath locates a helper binary across common repository and build locations.
func resolveHelperPath(name string) string {
	if filepath.IsAbs(name) {
		if _, err := os.Stat(name); err == nil {
			return name
		}
		return ""
	}

	// 1. Check relative to current executable
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			return p
		}
		for _, sub := range []string{"../linux/mutter-helper", "../linux/wayland-helper", "linux/mutter-helper", "linux/wayland-helper"} {
			subPath := filepath.Join(dir, sub, name)
			if _, err := os.Stat(subPath); err == nil {
				return subPath
			}
		}
	}

	// 2. Check current working directory and walk up parent directories
	cwd, err := os.Getwd()
	if err == nil {
		dir := cwd
		for i := 0; i < 6; i++ {
			for _, sub := range []string{"linux/mutter-helper", "linux/wayland-helper", ""} {
				candidate := filepath.Join(dir, sub, name)
				if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
					return candidate
				}
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}

	// 3. Check system PATH
	if p, err := exec.LookPath(name); err == nil {
		return p
	}

	return ""
}

// ProbeResult holds the output of a helper probe check.
type ProbeResult struct {
	Backend        Backend
	HelperPath     string
	CompositorName string
	Status         string
	ErrorDetail    string
}

// ProbeHelper invokes `<helperPath> probe` and parses the initial status response.
func ProbeHelper(ctx context.Context, helperPath string, env []string) (*ProbeResult, error) {
	if helperPath == "" {
		return nil, ErrNoBackend
	}

	probeCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, helperPath, "probe")
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("probe stdout pipe failed: %w", err)
	}
	defer stdout.Close()

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("probe start failed: %w", err)
	}

	r := bufio.NewReader(stdout)
	msg, readErr := ReadIPCMessage(r)
	_ = cmd.Wait()

	if readErr != nil {
		return nil, fmt.Errorf("probe read failed: %w", readErr)
	}

	if msg.Type != "STATUS" {
		return nil, fmt.Errorf("probe unexpected response type: %s", msg.Type)
	}

	res := &ProbeResult{
		HelperPath:     helperPath,
		Status:         msg.Name,
		CompositorName: msg.Params["compositor"],
		ErrorDetail:    msg.Params["detail"],
	}

	if b := msg.Params["backend"]; b != "" {
		res.Backend = Backend(b)
	} else if _, ok := msg.Params["data_control"]; ok {
		res.Backend = BackendWlroots
	} else {
		res.Backend = deriveBackend(helperPath)
	}

	switch msg.Name {
	case StatusReady:
		return res, nil
	case StatusErrCosmicFlagRequired:
		return res, ErrCosmicFlagRequired
	case StatusErrNoDataControl:
		return res, ErrNoDataControl
	case StatusErrWaylandConnect:
		return res, ErrWaylandUnavailable
	default:
		return res, fmt.Errorf("probe failed: %s (%s)", msg.Name, msg.Params["detail"])
	}
}

// DetectBackend discovers the appropriate helper binary by probing the live environment.
func DetectBackend(ctx context.Context, env []string) (*ProbeResult, error) {
	// 1. Explicit environment override
	envOverride := os.Getenv("PHONEBRIDGE_CLIPBOARD_HELPER")
	if envOverride == "" {
		envOverride = os.Getenv("PHONEBRIDGE_WAYLAND_HELPER")
	}
	if envOverride != "" {
		resolved := envOverride
		if !filepath.IsAbs(resolved) {
			if r := resolveHelperPath(resolved); r != "" {
				resolved = r
			}
		}
		res, err := ProbeHelper(ctx, resolved, env)
		if err != nil {
			return res, fmt.Errorf("explicit helper override %q failed probe: %w", envOverride, err)
		}
		return res, nil
	}

	// 2. Identify candidate helpers
	mutterPath := resolveHelperPath("phonebridge-mutter-helper")
	waylandPath := resolveHelperPath("phonebridge-wayland-helper")

	type candidate struct {
		path    string
		backend Backend
	}

	var candidates []candidate
	if isGNOMEDesktop(env) {
		if mutterPath != "" {
			candidates = append(candidates, candidate{path: mutterPath, backend: BackendMutter})
		}
		if waylandPath != "" {
			candidates = append(candidates, candidate{path: waylandPath, backend: BackendWlroots})
		}
	} else {
		if waylandPath != "" {
			candidates = append(candidates, candidate{path: waylandPath, backend: BackendWlroots})
		}
		if mutterPath != "" {
			candidates = append(candidates, candidate{path: mutterPath, backend: BackendMutter})
		}
	}

	if len(candidates) == 0 {
		return nil, ErrNoBackend
	}

	var lastErr error
	var bestErr error
	for _, c := range candidates {
		res, err := ProbeHelper(ctx, c.path, env)
		if err == nil && res.Status == StatusReady {
			if res.Backend == "" || res.Backend == BackendNone {
				res.Backend = c.backend
			}
			return res, nil
		}
		lastErr = err
		if errors.Is(err, ErrNoDataControl) || errors.Is(err, ErrCosmicFlagRequired) {
			bestErr = err
		}
	}

	if bestErr != nil {
		return nil, bestErr
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrNoBackend
}
