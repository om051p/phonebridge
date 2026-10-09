// Command ipcdrv is a Local IPC diagnostic and verification tool for PhoneBridge.
// It is intended for testing, health probing, and developer automation.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/om051p/phonebridge/core/pkg/localipc"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

var (
	version   = "0.1.0"
	commit    = "dev"
	buildDate = "unknown"
)

func resolveSocketPath() string {
	if s := os.Getenv("PHONEBRIDGE_SOCK"); s != "" {
		return s
	}
	return localipc.DefaultSocketPath()
}

func resolveTokenPath() string {
	if t := os.Getenv("PHONEBRIDGE_TOKEN"); t != "" {
		return t
	}
	return localipc.DefaultTokenPath()
}

func show(m proto.Message, err error) {
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	b, _ := prototext.MarshalOptions{Multiline: true}.Marshal(m)
	fmt.Println(string(b))
}

func main() {
	if len(os.Args) < 2 || os.Args[1] == "help" || os.Args[1] == "--help" || os.Args[1] == "-h" {
		fmt.Printf("PhoneBridge Local IPC Diagnostic Tool (ipcdrv %s %s)\n", version, commit)
		fmt.Println("usage: ipcdrv ping|health|session|devices|trusted|pair <device>|confirm <device> <true|false>|transfers|send <path> [device]|cancel <id>|notifications|clipboard|clipboard-pull|events [seconds]|input-tap|input-longpress|input-swipe|input-action|input-text|input-raw")
		os.Exit(2)
	}
	if os.Args[1] == "version" || os.Args[1] == "--version" || os.Args[1] == "-v" {
		fmt.Printf("ipcdrv %s (%s, %s)\n", version, commit, buildDate)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	sock := resolveSocketPath()
	tok := resolveTokenPath()
	c, err := localipc.PollAndConnect(ctx, sock, tok, 5*time.Second)
	if err != nil {
		fmt.Println("ERR dial:", err)
		os.Exit(1)
	}
	defer c.Close()
	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer rpcCancel()

	switch os.Args[1] {
	case "health", "ping":
		r, err := c.Health(rpcCtx, &phonebridgelocalipcv1.HealthRequest{})
		show(r, err)
	case "session":
		r, err := c.GetSessionState(rpcCtx, &phonebridgelocalipcv1.GetSessionStateRequest{})
		show(r, err)
	case "devices":
		r, err := c.ListDevices(rpcCtx, &phonebridgelocalipcv1.ListDevicesRequest{})
		show(r, err)
	case "trusted":
		r, err := c.ListTrustedDevices(rpcCtx, &phonebridgelocalipcv1.ListTrustedDevicesRequest{})
		show(r, err)
	case "pair":
		if len(os.Args) < 3 {
			fmt.Println("pair needs a device id")
			os.Exit(2)
		}
		r, err := c.PairDevice(rpcCtx, &phonebridgelocalipcv1.PairDeviceRequest{DeviceId: os.Args[2]})
		show(r, err)
	case "confirm":
		if len(os.Args) < 4 {
			fmt.Println("confirm needs a device id and true|false")
			os.Exit(2)
		}
		confirmed := os.Args[3] == "true"
		r, err := c.ConfirmPairing(rpcCtx, &phonebridgelocalipcv1.ConfirmPairingRequest{
			DeviceId:      os.Args[2],
			UserConfirmed: confirmed,
		})
		show(r, err)
	case "start":
		if len(os.Args) < 3 {
			fmt.Println("start needs a device id")
			os.Exit(2)
		}
		r, err := c.StartSession(rpcCtx, &phonebridgelocalipcv1.StartSessionRequest{DeviceId: os.Args[2]})
		show(r, err)
	case "stop":
		r, err := c.StopSession(rpcCtx, &phonebridgelocalipcv1.StopSessionRequest{})
		show(r, err)
	case "transfers":
		r, err := c.ListTransfers(rpcCtx, &phonebridgelocalipcv1.ListTransfersRequest{})
		show(r, err)
	case "send":
		if len(os.Args) < 3 {
			fmt.Println("send needs a path")
			os.Exit(2)
		}
		req := &phonebridgelocalipcv1.SendFileRequest{LocalPath: os.Args[2]}
		if len(os.Args) > 3 {
			req.DeviceId = os.Args[3]
		}
		r, err := c.SendFile(rpcCtx, req)
		show(r, err)
	case "cancel":
		if len(os.Args) < 3 {
			fmt.Println("cancel needs a transfer id")
			os.Exit(2)
		}
		r, err := c.CancelTransfer(rpcCtx, &phonebridgelocalipcv1.CancelTransferRequest{TransferId: os.Args[2]})
		show(r, err)
	case "notifications":
		r, err := c.ListNotifications(rpcCtx, &phonebridgelocalipcv1.ListNotificationsRequest{})
		show(r, err)
	case "dismiss":
		if len(os.Args) < 3 {
			fmt.Println("dismiss needs a notification key")
			os.Exit(2)
		}
		r, err := c.DismissNotification(rpcCtx, &phonebridgelocalipcv1.DismissNotificationRequest{Key: os.Args[2]})
		show(r, err)
	case "clipboard":
		r, err := c.GetClipboardStatus(rpcCtx, &phonebridgelocalipcv1.GetClipboardStatusRequest{})
		show(r, err)
	case "clipboard-pull":
		r, err := c.TriggerClipboardPull(rpcCtx, &phonebridgelocalipcv1.TriggerClipboardPullRequest{})
		show(r, err)
	case "events":
		secs := 30
		if len(os.Args) > 2 {
			fmt.Sscanf(os.Args[2], "%d", &secs)
		}
		stream, err := c.StreamEvents(rpcCtx, &phonebridgelocalipcv1.StreamEventsRequest{})
		if err != nil {
			fmt.Println("ERR stream:", err)
			os.Exit(1)
		}
		deadline := time.After(time.Duration(secs) * time.Second)
		for {
			ev, err := stream.Recv()
			if err != nil {
				fmt.Println("stream ended:", err)
				return
			}
			b, _ := prototext.Marshal(ev)
			fmt.Printf("EVENT %s\n", b)
			select {
			case <-deadline:
				return
			default:
			}
		}
	case "input-tap":
		if len(os.Args) < 4 {
			fmt.Println("usage: ipcdrv input-tap <normX> <normY> [sessionId]")
			os.Exit(2)
		}
		var x, y float64
		fmt.Sscanf(os.Args[2], "%f", &x)
		fmt.Sscanf(os.Args[3], "%f", &y)
		sessID := ""
		if len(os.Args) > 4 {
			sessID = os.Args[4]
		} else {
			st, err := c.GetSessionState(rpcCtx, &phonebridgelocalipcv1.GetSessionStateRequest{})
			if err != nil {
				fmt.Println("ERR get session:", err)
				os.Exit(1)
			}
			sessID = st.SessionId
		}
		_, err = c.SendInput(rpcCtx, &phonebridgelocalipcv1.SendInputRequest{
			SessionId: sessID,
			Frame: &phonebridgev1.InputFrame{
				TimestampMs: uint64(time.Now().UnixMilli()),
				Event: &phonebridgev1.InputFrame_Touch{
					Touch: &phonebridgev1.TouchEvent{
						Action:      phonebridgev1.TouchEvent_ACTION_DOWN,
						PointerId:   0,
						NormalizedX: float32(x),
						NormalizedY: float32(y),
					},
				},
			},
		})
		if err != nil {
			fmt.Println("ERR down:", err)
			os.Exit(1)
		}
		time.Sleep(50 * time.Millisecond)
		r, err := c.SendInput(rpcCtx, &phonebridgelocalipcv1.SendInputRequest{
			SessionId: sessID,
			Frame: &phonebridgev1.InputFrame{
				TimestampMs: uint64(time.Now().UnixMilli()),
				Event: &phonebridgev1.InputFrame_Touch{
					Touch: &phonebridgev1.TouchEvent{
						Action:      phonebridgev1.TouchEvent_ACTION_UP,
						PointerId:   0,
						NormalizedX: float32(x),
						NormalizedY: float32(y),
					},
				},
			},
		})
		show(r, err)
	case "input-longpress":
		if len(os.Args) < 4 {
			fmt.Println("usage: ipcdrv input-longpress <normX> <normY> [sessionId]")
			os.Exit(2)
		}
		var x, y float64
		fmt.Sscanf(os.Args[2], "%f", &x)
		fmt.Sscanf(os.Args[3], "%f", &y)
		sessID := ""
		if len(os.Args) > 4 {
			sessID = os.Args[4]
		} else {
			st, err := c.GetSessionState(rpcCtx, &phonebridgelocalipcv1.GetSessionStateRequest{})
			if err != nil {
				fmt.Println("ERR get session:", err)
				os.Exit(1)
			}
			sessID = st.SessionId
		}
		_, err = c.SendInput(rpcCtx, &phonebridgelocalipcv1.SendInputRequest{
			SessionId: sessID,
			Frame: &phonebridgev1.InputFrame{
				TimestampMs: uint64(time.Now().UnixMilli()),
				Event: &phonebridgev1.InputFrame_Touch{
					Touch: &phonebridgev1.TouchEvent{
						Action:      phonebridgev1.TouchEvent_ACTION_DOWN,
						PointerId:   0,
						NormalizedX: float32(x),
						NormalizedY: float32(y),
					},
				},
			},
		})
		if err != nil {
			fmt.Println("ERR down:", err)
			os.Exit(1)
		}
		time.Sleep(550 * time.Millisecond)
		r, err := c.SendInput(rpcCtx, &phonebridgelocalipcv1.SendInputRequest{
			SessionId: sessID,
			Frame: &phonebridgev1.InputFrame{
				TimestampMs: uint64(time.Now().UnixMilli()),
				Event: &phonebridgev1.InputFrame_Touch{
					Touch: &phonebridgev1.TouchEvent{
						Action:      phonebridgev1.TouchEvent_ACTION_UP,
						PointerId:   0,
						NormalizedX: float32(x),
						NormalizedY: float32(y),
					},
				},
			},
		})
		show(r, err)
	case "input-swipe":
		if len(os.Args) < 6 {
			fmt.Println("usage: ipcdrv input-swipe <x1> <y1> <x2> <y2> [sessionId]")
			os.Exit(2)
		}
		var x1, y1, x2, y2 float64
		fmt.Sscanf(os.Args[2], "%f", &x1)
		fmt.Sscanf(os.Args[3], "%f", &y1)
		fmt.Sscanf(os.Args[4], "%f", &x2)
		fmt.Sscanf(os.Args[5], "%f", &y2)
		sessID := ""
		if len(os.Args) > 6 {
			sessID = os.Args[6]
		} else {
			st, err := c.GetSessionState(rpcCtx, &phonebridgelocalipcv1.GetSessionStateRequest{})
			if err != nil {
				fmt.Println("ERR get session:", err)
				os.Exit(1)
			}
			sessID = st.SessionId
		}
		_, err = c.SendInput(rpcCtx, &phonebridgelocalipcv1.SendInputRequest{
			SessionId: sessID,
			Frame: &phonebridgev1.InputFrame{
				TimestampMs: uint64(time.Now().UnixMilli()),
				Event: &phonebridgev1.InputFrame_Touch{
					Touch: &phonebridgev1.TouchEvent{
						Action:      phonebridgev1.TouchEvent_ACTION_DOWN,
						PointerId:   0,
						NormalizedX: float32(x1),
						NormalizedY: float32(y1),
					},
				},
			},
		})
		if err != nil {
			fmt.Println("ERR down:", err)
			os.Exit(1)
		}
		// Interpolate steps
		steps := 5
		for i := 1; i <= steps; i++ {
			t := float64(i) / float64(steps)
			curX := x1 + (x2-x1)*t
			curY := y1 + (y2-y1)*t
			time.Sleep(30 * time.Millisecond)
			c.SendInput(rpcCtx, &phonebridgelocalipcv1.SendInputRequest{
				SessionId: sessID,
				Frame: &phonebridgev1.InputFrame{
					TimestampMs: uint64(time.Now().UnixMilli()),
					Event: &phonebridgev1.InputFrame_Touch{
						Touch: &phonebridgev1.TouchEvent{
							Action:      phonebridgev1.TouchEvent_ACTION_MOVE,
							PointerId:   0,
							NormalizedX: float32(curX),
							NormalizedY: float32(curY),
						},
					},
				},
			})
		}
		time.Sleep(30 * time.Millisecond)
		r, err := c.SendInput(rpcCtx, &phonebridgelocalipcv1.SendInputRequest{
			SessionId: sessID,
			Frame: &phonebridgev1.InputFrame{
				TimestampMs: uint64(time.Now().UnixMilli()),
				Event: &phonebridgev1.InputFrame_Touch{
					Touch: &phonebridgev1.TouchEvent{
						Action:      phonebridgev1.TouchEvent_ACTION_UP,
						PointerId:   0,
						NormalizedX: float32(x2),
						NormalizedY: float32(y2),
					},
				},
			},
		})
		show(r, err)
	case "input-action":
		if len(os.Args) < 3 {
			fmt.Println("usage: ipcdrv input-action <back|home|recents|notifications|quick_settings> [sessionId]")
			os.Exit(2)
		}
		var actType phonebridgev1.GlobalActionEvent_Type
		switch os.Args[2] {
		case "back":
			actType = phonebridgev1.GlobalActionEvent_TYPE_GLOBAL_ACTION_BACK
		case "home":
			actType = phonebridgev1.GlobalActionEvent_TYPE_GLOBAL_ACTION_HOME
		case "recents":
			actType = phonebridgev1.GlobalActionEvent_TYPE_GLOBAL_ACTION_RECENTS
		case "notifications":
			actType = phonebridgev1.GlobalActionEvent_TYPE_GLOBAL_ACTION_NOTIFICATIONS
		case "quick_settings":
			actType = phonebridgev1.GlobalActionEvent_TYPE_GLOBAL_ACTION_QUICK_SETTINGS
		default:
			fmt.Println("unknown action:", os.Args[2])
			os.Exit(2)
		}
		sessID := ""
		if len(os.Args) > 3 {
			sessID = os.Args[3]
		} else {
			st, err := c.GetSessionState(rpcCtx, &phonebridgelocalipcv1.GetSessionStateRequest{})
			if err != nil {
				fmt.Println("ERR get session:", err)
				os.Exit(1)
			}
			sessID = st.SessionId
		}
		r, err := c.SendInput(rpcCtx, &phonebridgelocalipcv1.SendInputRequest{
			SessionId: sessID,
			Frame: &phonebridgev1.InputFrame{
				TimestampMs: uint64(time.Now().UnixMilli()),
				Event: &phonebridgev1.InputFrame_Action{
					Action: &phonebridgev1.GlobalActionEvent{
						Type: actType,
					},
				},
			},
		})
		show(r, err)
	case "input-text":
		if len(os.Args) < 3 {
			fmt.Println("usage: ipcdrv input-text <text> [sessionId]")
			os.Exit(2)
		}
		text := os.Args[2]
		sessID := ""
		if len(os.Args) > 3 {
			sessID = os.Args[3]
		} else {
			st, err := c.GetSessionState(rpcCtx, &phonebridgelocalipcv1.GetSessionStateRequest{})
			if err != nil {
				fmt.Println("ERR get session:", err)
				os.Exit(1)
			}
			sessID = st.SessionId
		}
		r, err := c.SendInput(rpcCtx, &phonebridgelocalipcv1.SendInputRequest{
			SessionId: sessID,
			Frame: &phonebridgev1.InputFrame{
				TimestampMs: uint64(time.Now().UnixMilli()),
				Event: &phonebridgev1.InputFrame_Text{
					Text: &phonebridgev1.TextEvent{
						Text: text,
					},
				},
			},
		})
		show(r, err)
	case "input-raw":
		// Raw coordinate test (allows out-of-bounds, e.g. -0.5 or 1.5)
		if len(os.Args) < 4 {
			fmt.Println("usage: ipcdrv input-raw <normX> <normY> [sessionId]")
			os.Exit(2)
		}
		var x, y float64
		fmt.Sscanf(os.Args[2], "%f", &x)
		fmt.Sscanf(os.Args[3], "%f", &y)
		sessID := ""
		if len(os.Args) > 4 {
			sessID = os.Args[4]
		} else {
			st, err := c.GetSessionState(rpcCtx, &phonebridgelocalipcv1.GetSessionStateRequest{})
			if err != nil {
				fmt.Println("ERR get session:", err)
				os.Exit(1)
			}
			sessID = st.SessionId
		}
		r, err := c.SendInput(rpcCtx, &phonebridgelocalipcv1.SendInputRequest{
			SessionId: sessID,
			Frame: &phonebridgev1.InputFrame{
				TimestampMs: uint64(time.Now().UnixMilli()),
				Event: &phonebridgev1.InputFrame_Touch{
					Touch: &phonebridgev1.TouchEvent{
						Action:      phonebridgev1.TouchEvent_ACTION_DOWN,
						PointerId:   0,
						NormalizedX: float32(x),
						NormalizedY: float32(y),
					},
				},
			},
		})
		show(r, err)
	default:
		fmt.Println("unknown cmd")
		os.Exit(2)
	}
}
