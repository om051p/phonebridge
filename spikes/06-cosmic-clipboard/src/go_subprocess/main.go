package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

type SubprocessClipboardWatcher struct {
	cmd     *exec.Cmd
	eventCh chan string
	stopCh  chan struct{}
}

func StartSubprocessWatcher(probePath string) (*SubprocessClipboardWatcher, error) {
	cmd := exec.Command(probePath, "--mode", "listen", "--no-read")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	w := &SubprocessClipboardWatcher{
		cmd:     cmd,
		eventCh: make(chan string, 16),
		stopCh:  make(chan struct{}),
	}

	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, "EVENT=") {
				w.eventCh <- line
			}
		}
		close(w.eventCh)
	}()

	return w, nil
}

func (w *SubprocessClipboardWatcher) Stop() {
	if w.cmd != nil && w.cmd.Process != nil {
		_ = w.cmd.Process.Kill()
		_ = w.cmd.Wait()
	}
}

func main() {
	fmt.Println("=== SUBPROCESS HELPER GO CLIPBOARD PROBE ===")
	probePath := "./06-cosmic-clipboard/bin/data_control_probe"

	t0 := time.Now()
	watcher, err := StartSubprocessWatcher(probePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to start subprocess helper: %v\n", err)
		os.Exit(1)
	}
	defer watcher.Stop()
	spawnDuration := time.Since(t0)

	fmt.Printf("Subprocess helper spawned in %v (PID %d)\n", spawnDuration, watcher.cmd.Process.Pid)

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	fmt.Printf("Go Heap Alloc: %d KB, TotalAlloc: %d KB\n", mem.Alloc/1024, mem.TotalAlloc/1024)

	timeout := time.After(3 * time.Second)
	eventsReceived := 0

loop:
	for {
		select {
		case ev, ok := <-watcher.eventCh:
			if !ok {
				break loop
			}
			eventsReceived++
			fmt.Printf("Go Channel Received Event #%d from helper: %s\n", eventsReceived, ev)
			if eventsReceived >= 2 {
				break loop
			}
		case <-timeout:
			fmt.Printf("Timeout reached. Total events received: %d\n", eventsReceived)
			break loop
		}
	}

	runtime.ReadMemStats(&mem)
	fmt.Printf("Final Go Heap Alloc: %d KB\n", mem.Alloc/1024)
	fmt.Println("STATUS=subprocess_probe_success")
}
