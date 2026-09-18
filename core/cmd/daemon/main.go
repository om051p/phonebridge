// Package main — phonebridge-daemon production entrypoint.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/engine"
	"github.com/om051p/phonebridge/core/pkg/localipc"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
)

var version = "0.1.0"

func main() {
	var (
		showVersion = flag.Bool("version", false, "Print daemon version and exit")
		showV       = flag.Bool("V", false, "Print daemon version and exit (shorthand)")
		socketPath  = flag.String("socket", "", "UDS socket path (default: $XDG_RUNTIME_DIR/phonebridge/engine.sock)")
		tokenPath   = flag.String("token-file", "", "Bearer token file path (default: $XDG_RUNTIME_DIR/phonebridge/token)")
	)
	flag.Parse()

	if *showVersion || *showV {
		fmt.Println(version)
		return
	}

	cfg := localipc.Config{
		SocketPath:    *socketPath,
		TokenPath:     *tokenPath,
		ServerVersion: version,
		Logf:          log.Printf,
	}

	srv, err := localipc.NewServer(cfg)
	if err != nil {
		log.Fatalf("failed to configure localipc server: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Initialize cryptographic identity and trust store
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "Linux Host"
	}
	identity, err := crypto.LoadOrGenerateIdentity(crypto.DefaultIdentityPath(), hostname, "linux")
	if err != nil {
		log.Fatalf("failed to initialize device identity: %v", err)
	}
	trustStore, err := crypto.NewTrustStore(crypto.DefaultTrustStorePath())
	if err != nil {
		log.Fatalf("failed to initialize trust store: %v", err)
	}

	// Initialize mDNS discovery
	discCfg := discovery.Config{
		DeviceID:        identity.DeviceID,
		DeviceName:      identity.DisplayName,
		IncludeLoopback: true,
	}
	disc, err := discovery.NewDiscovery(discCfg)
	if err != nil {
		log.Printf("warning: discovery initialization failed: %v", err)
	} else {
		go func() {
			if err := disc.Start(ctx); err != nil {
				log.Printf("discovery exited: %v", err)
			}
		}()
		defer disc.Close()
	}

	// Initialize session coordinator
	sessionCfg := engine.DefaultSessionConfig()
	sessionCfg.Identity = identity
	sessionCfg.TrustStore = trustStore
	mgr := engine.NewSessionManager(sessionCfg, disc, nil, func(evt engine.SessionEvent) {
		srv.BroadcastSessionEvent(&phonebridgelocalipcv1.SessionEvent{
			SessionId:    evt.SessionID,
			State:        localipc.ToProtoSessionState(evt.State),
			Reason:       evt.Reason,
			ErrorMessage: evt.ErrorMessage,
		})
	})
	mgr.SetIdentity(identity)
	mgr.SetTrustStore(trustStore)
	srv.SetOrchestrator(mgr)

	activeCfg := srv.Config()
	log.Printf("Starting phonebridge-daemon %s (pid=%d, uid=%d, device_id=%s)", version, os.Getpid(), os.Geteuid(), identity.DeviceID)
	log.Printf("Local IPC UDS: %s", activeCfg.SocketPath)
	log.Printf("Token file: %s", activeCfg.TokenPath)
	log.Printf("Identity file: %s", crypto.DefaultIdentityPath())
	log.Printf("Trust store: %s", crypto.DefaultTrustStorePath())

	if err := srv.Serve(ctx); err != nil {
		log.Fatalf("daemon exited with error: %v", err)
	}
	log.Printf("phonebridge-daemon stopped cleanly")
}
