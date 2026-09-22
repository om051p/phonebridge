// Command ipcdrv is a temporary Local IPC driver for physical Phase 4 E2E
// validation. It is NOT part of the shipped product.
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/om051p/phonebridge/core/pkg/localipc"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
)

const (
	sockPath  = "/run/user/1000/phonebridge/engine.sock"
	tokenPath = "/run/user/1000/phonebridge/token"
)

func show(m proto.Message, err error) {
	if err != nil {
		fmt.Println("ERR:", err)
		os.Exit(1)
	}
	b, _ := prototext.MarshalOptions{Multiline: true}.Marshal(m)
	fmt.Println(string(b))
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("usage: ipcdrv health|session|devices|trusted|pair <device>|confirm <device> <true|false>|transfers|send <path> [device]|cancel <id>|events [seconds]")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, err := localipc.PollAndConnect(ctx, sockPath, tokenPath, 5*time.Second)
	if err != nil {
		fmt.Println("ERR dial:", err)
		os.Exit(1)
	}
	defer c.Close()
	rpcCtx, rpcCancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer rpcCancel()

	switch os.Args[1] {
	case "health":
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
	default:
		fmt.Println("unknown cmd")
		os.Exit(2)
	}
}
