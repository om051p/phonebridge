// Command spiked — Spike 01 UDS+gRPC daemon (EXPERIMENTAL, throwaway).
//
// Minimal by design: no product features, no config file, no D-Bus. It exists
// to answer what the Flutter UI can expect from a Go daemon over a Unix domain
// socket: reachability, latency, lifecycle, and the trust boundary.
//
// Suggested systemd user-service shape (mirrors
// linux/packaging/systemd/phonebridge.service, which is a PLANNED stub):
//
//	ExecStart=%h/.local/bin/spiked --socket %t/phonebridge/spike01.sock
//	Environment=XDG_RUNTIME_DIR=%t
//
// Note the daemon must NOT fork/daemonise: systemd Type=simple expects the
// process to stay in the foreground, and it treats SIGTERM as the shutdown
// signal (handled here).
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/om051p/phonebridge/spikes/localipc-spike/internal/spikegrpc"
)

// version is overridable at build time with
// -ldflags "-X main.version=...".
var version = "0.0.0-spike01"

func defaultSocketPath() string {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "phonebridge", "spike01.sock")
}

func main() {
	socket := flag.String("socket", defaultSocketPath(), "Unix domain socket to bind")
	token := flag.String("token", "", "require `authorization: Bearer <token>` (empty disables the gate)")
	expectUID := flag.Int("expect-uid", -1, "only accept this peer uid (-1 = current euid)")
	maxEcho := flag.Int("max-echo-bytes", 64*1024, "max payload bytes echoed by Ping")
	maxRecv := flag.Int("max-recv-bytes", 4*1024*1024, "gRPC max receive message size")
	quiet := flag.Bool("quiet", false, "suppress the READY/STATS stdout lines")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	uid := uint32(os.Geteuid())
	if *expectUID >= 0 {
		uid = uint32(*expectUID)
	}

	srv := spikegrpc.New(spikegrpc.Config{
		SocketPath:   *socket,
		Token:        *token,
		ExpectUID:    uid,
		ExpectUIDSet: true,
		Version:      version,
		MaxEchoBytes: *maxEcho,
		MaxRecvBytes: *maxRecv,
		Announce:     !*quiet,
		Logf: func(format string, args ...any) {
			fmt.Fprintf(os.Stderr, "spiked: "+format+"\n", args...)
		},
	})

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := srv.Serve(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "spiked: %v\n", err)
		os.Exit(1)
	}
}
