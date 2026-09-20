// Package main — phonebridge-daemon production entrypoint.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	pion "github.com/pion/webrtc/v4"

	"github.com/om051p/phonebridge/core/pkg/clipboard"
	"github.com/om051p/phonebridge/core/pkg/crypto"
	"github.com/om051p/phonebridge/core/pkg/discovery"
	"github.com/om051p/phonebridge/core/pkg/engine"
	"github.com/om051p/phonebridge/core/pkg/localipc"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
)

var version = "0.1.0"

func main() {
	var (
		showVersion   = flag.Bool("version", false, "Print daemon version and exit")
		showV         = flag.Bool("V", false, "Print daemon version and exit (shorthand)")
		socketPath    = flag.String("socket", "", "UDS socket path (default: $XDG_RUNTIME_DIR/phonebridge/engine.sock)")
		tokenPath     = flag.String("token-file", "", "Bearer token file path (default: $XDG_RUNTIME_DIR/phonebridge/token)")
		signalingPort = flag.Int("signaling-port", engine.DefaultSignalingPort, "TCP port for LAN signaling server (0 for ephemeral)")
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

	// Initialize session coordinator
	sessionCfg := engine.DefaultSessionConfig()
	sessionCfg.Identity = identity
	sessionCfg.TrustStore = trustStore
	mgr := engine.NewSessionManager(sessionCfg, nil, nil, func(evt engine.SessionEvent) {
		srv.BroadcastSessionEvent(&phonebridgelocalipcv1.SessionEvent{
			SessionId:    evt.SessionID,
			State:        localipc.ToProtoSessionState(evt.State),
			Reason:       evt.Reason,
			ReasonCode:   localipc.ToProtoSessionReason(evt.ReasonCode),
			ErrorMessage: evt.ErrorMessage,
		})
	})
	mgr.SetIdentity(identity)
	mgr.SetTrustStore(trustStore)

	// Initialize LAN signaling server with inbound session handlers (DEC-022)
	portToUse := *signalingPort
	if envPort := os.Getenv("PHONEBRIDGE_SIGNALING_PORT"); envPort != "" {
		if p, err := strconv.Atoi(envPort); err == nil {
			portToUse = p
		}
	}

	sigSrv := engine.NewSignalingServer(engine.SignalingServerConfig{
		Port:       portToUse,
		Identity:   identity,
		TrustStore: trustStore,
		OfferHandler: func(req engine.NegotiationRequest) (engine.NegotiationResponse, error) {
			return mgr.HandleInboundOffer(req)
		},
		AnswerHandler: func(answer pion.SessionDescription) error {
			return mgr.HandleInboundAnswer(answer)
		},
		StopHandler: func(reason string, code engine.Code) error {
			return mgr.HandleInboundStop(reason, code)
		},
	})
	if err := sigSrv.Start(ctx); err != nil {
		log.Printf("warning: LAN signaling server start failed on port %d: %v", portToUse, err)
	} else {
		defer sigSrv.Close()
		log.Printf("LAN signaling server listening on port %d", sigSrv.Port())
	}

	// Initialize mDNS discovery (advertising port and browsing LAN)
	discCfg := discovery.Config{
		DeviceID:        identity.DeviceID,
		DeviceName:      identity.DisplayName,
		Port:            uint16(sigSrv.Port()),
		IncludeLoopback: true,
		Version:         "1",
		Capabilities:    []string{"SCREEN", "CLIPBOARD"},
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
		mgr.SetDiscovery(disc)
	}

	// Initialize clipboard subsystem (DEC-023)
	var clipboardAdapter *clipboard.LinuxAdapter
	var clipboardEngine *clipboard.Engine

	helperPath := os.Getenv("PHONEBRIDGE_WAYLAND_HELPER")
	if helperPath == "" {
		candidates := []string{
			"phonebridge-wayland-helper",
			"/usr/local/bin/phonebridge-wayland-helper",
			"/usr/bin/phonebridge-wayland-helper",
		}
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				helperPath = c
				break
			}
		}
	}

	adapterCfg := clipboard.LinuxAdapterConfig{
		HelperPath: helperPath,
		OnClipboardChanged: func(c context.Context, mimeType string, payload []byte) error {
			if clipboardEngine != nil {
				nowMs := uint64(time.Now().UnixMilli())
				_, err := clipboardEngine.OnLocalCopy(c, mimeType, payload, nowMs)
				return err
			}
			return nil
		},
		OnOversizedPayload: func(size int) {
			log.Printf("clipboard: oversized payload ignored (%d bytes > 768 KiB)", size)
		},
		OnStatusChanged: func(status clipboard.AdapterStatus, err error) {
			log.Printf("clipboard adapter status: %s (err: %v)", status, err)
			stateStr := "STOPPED"
			switch status {
			case clipboard.AdapterStatusReady:
				stateStr = "AMBIENT_ACTIVE"
			case clipboard.AdapterStatusCosmicFlagRequired:
				stateStr = "COSMIC_FLAG_REQUIRED"
			case clipboard.AdapterStatusNoDataControl:
				stateStr = "NO_DATA_CONTROL"
			case clipboard.AdapterStatusWaylandUnavailable:
				stateStr = "WAYLAND_UNAVAILABLE"
			case clipboard.AdapterStatusCrashed:
				stateStr = "UNAVAILABLE"
			}
			isConn := false
			remotePeer := ""
			if clipboardEngine != nil {
				isConn = clipboardEngine.HasTransport()
				remotePeer = clipboardEngine.RemotePeerID()
			}
			srv.BroadcastClipboardEvent(&phonebridgelocalipcv1.ClipboardStatusEvent{
				State:          stateStr,
				IsConnected:    isConn,
				AdapterStatus:  status.String(),
				RemotePeerId:   remotePeer,
				MaxPayloadSize: clipboard.MaxPayloadSize,
			})
		},
	}

	adapter, err := clipboard.NewLinuxAdapter(adapterCfg)
	if err != nil {
		log.Printf("warning: clipboard adapter initialization failed: %v", err)
	} else {
		clipboardAdapter = adapter
		mgr.SetClipboardAdapter(clipboardAdapter)
		engineCfg := clipboard.EngineConfig{
			Role:        clipboard.RoleDesktop,
			LocalPeerID: identity.DeviceID,
			Platform:    clipboardAdapter,
		}
		eng, err := clipboard.NewEngine(engineCfg)
		if err != nil {
			log.Printf("warning: clipboard engine initialization failed: %v", err)
		} else {
			clipboardEngine = eng
			mgr.SetClipboardEngine(clipboardEngine)
			if err := clipboardAdapter.Start(ctx); err != nil {
				log.Printf("warning: clipboard adapter start failed: %v", err)
			}
			defer clipboardAdapter.Stop()
		}
	}

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
