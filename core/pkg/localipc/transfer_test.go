package localipc

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"github.com/om051p/phonebridge/core/pkg/transfer"
)

// TestServer_TransferEndpoints exercises the DEC-024 file-transfer surface over
// the real UDS: the typed mapping is the contract the UI depends on, so it is
// asserted on the wire rather than by calling the converters directly.
func TestServer_TransferEndpoints(t *testing.T) {
	tmpDir := t.TempDir()
	sock := filepath.Join(tmpDir, "engine.sock")
	tok := filepath.Join(tmpDir, "token")
	tokVal := "test-bearer-token-transfer"

	mock := &mockOrchestrator{
		sendFileID: "transfer-abc123",
		transfers: []transfer.Info{
			{
				TransferID:       "transfer-abc123",
				Direction:        transfer.DirectionOutbound,
				State:            transfer.StateActive,
				PeerDeviceID:     "pixel-9",
				Filename:         "report.pdf",
				MimeType:         "application/pdf",
				SizeBytes:        4096,
				BytesTransferred: 1024,
				StartedAtMs:      1726000000000,
				ReasonCode:       transfer.ReasonNone,
			},
			{
				TransferID:   "transfer-def456",
				Direction:    transfer.DirectionInbound,
				State:        transfer.StateFailed,
				PeerDeviceID: "pixel-9",
				Filename:     "../etc/passwd",
				SizeBytes:    12,
				StartedAtMs:  1726000001000,
				FinishedAtMs: 1726000002000,
				ReasonCode:   transfer.ReasonUnsafeFilename,
				ErrorMessage: "filename is not a plain basename",
			},
		},
	}

	cfg := Config{
		SocketPath:    sock,
		TokenPath:     tok,
		Token:         tokVal,
		ServerVersion: "1.0.0",
		Orchestrator:  mock,
	}

	_, cancel, errCh := startTestServer(t, cfg)
	defer func() {
		cancel()
		<-errCh
	}()

	client, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()
	ctx := context.Background()

	t.Run("SendFile forwards the request and returns the transfer id", func(t *testing.T) {
		resp, err := client.SendFile(ctx, &phonebridgelocalipcv1.SendFileRequest{
			DeviceId:  "pixel-9",
			LocalPath: "/home/u/report.pdf",
			Filename:  "report.pdf",
		})
		if err != nil {
			t.Fatalf("SendFile: %v", err)
		}
		if resp.GetTransferId() != "transfer-abc123" {
			t.Fatalf("transfer_id = %q", resp.GetTransferId())
		}
		if resp.GetState() != phonebridgelocalipcv1.TransferState_TRANSFER_STATE_PENDING {
			t.Fatalf("state = %s", resp.GetState())
		}
		if resp.GetReasonCode() != phonebridgelocalipcv1.TransferReason_TRANSFER_REASON_NONE {
			t.Fatalf("reason = %s", resp.GetReasonCode())
		}
		if mock.lastSendFile != [3]string{"pixel-9", "/home/u/report.pdf", "report.pdf"} {
			t.Fatalf("orchestrator saw %v", mock.lastSendFile)
		}
	})

	t.Run("empty local_path is rejected before touching the orchestrator", func(t *testing.T) {
		_, err := client.SendFile(ctx, &phonebridgelocalipcv1.SendFileRequest{DeviceId: "pixel-9"})
		if status.Code(err) != codes.InvalidArgument {
			t.Fatalf("want InvalidArgument, got %v", err)
		}
	})

	t.Run("engine failure is a typed reason, not a gRPC status", func(t *testing.T) {
		mock.sendFileErr = transfer.NewFailure(phonebridgev1.Code_CODE_FILE_TOO_LARGE, transfer.ReasonTooLarge,
			"file of 32 bytes exceeds the 16-byte policy")
		defer func() { mock.sendFileErr = nil }()

		resp, err := client.SendFile(ctx, &phonebridgelocalipcv1.SendFileRequest{
			LocalPath: "/home/u/huge.bin",
		})
		if err != nil {
			t.Fatalf("SendFile: %v", err)
		}
		if resp.GetTransferId() != "" {
			t.Fatalf("transfer_id should be empty on failure, got %q", resp.GetTransferId())
		}
		if resp.GetReasonCode() != phonebridgelocalipcv1.TransferReason_TRANSFER_REASON_TOO_LARGE {
			t.Fatalf("reason = %s, want TOO_LARGE", resp.GetReasonCode())
		}
		if resp.GetErrorMessage() == "" {
			t.Fatal("error_message must carry the human-readable detail")
		}
	})

	t.Run("ListTransfers maps direction, state and reason", func(t *testing.T) {
		resp, err := client.ListTransfers(ctx, &phonebridgelocalipcv1.ListTransfersRequest{})
		if err != nil {
			t.Fatalf("ListTransfers: %v", err)
		}
		if len(resp.GetTransfers()) != 2 {
			t.Fatalf("got %d transfers, want 2", len(resp.GetTransfers()))
		}
		first := resp.GetTransfers()[0]
		if first.GetDirection() != phonebridgelocalipcv1.TransferDirection_TRANSFER_DIRECTION_OUTBOUND {
			t.Fatalf("direction = %s", first.GetDirection())
		}
		if first.GetState() != phonebridgelocalipcv1.TransferState_TRANSFER_STATE_ACTIVE {
			t.Fatalf("state = %s", first.GetState())
		}
		if first.GetBytesTransferred() != 1024 || first.GetSizeBytes() != 4096 {
			t.Fatalf("progress = %d/%d", first.GetBytesTransferred(), first.GetSizeBytes())
		}
		second := resp.GetTransfers()[1]
		if second.GetState() != phonebridgelocalipcv1.TransferState_TRANSFER_STATE_FAILED {
			t.Fatalf("state = %s", second.GetState())
		}
		if second.GetReasonCode() != phonebridgelocalipcv1.TransferReason_TRANSFER_REASON_UNSAFE_FILENAME {
			t.Fatalf("reason = %s", second.GetReasonCode())
		}
		if second.GetFinishedAtMs() == 0 {
			t.Fatal("finished_at_ms must be set for a terminal transfer")
		}
	})

	t.Run("CancelTransfer reports the outcome without a status error", func(t *testing.T) {
		resp, err := client.CancelTransfer(ctx, &phonebridgelocalipcv1.CancelTransferRequest{TransferId: "transfer-abc123"})
		if err != nil {
			t.Fatalf("CancelTransfer: %v", err)
		}
		if !resp.GetCancelled() {
			t.Fatalf("cancelled = false: %s", resp.GetErrorMessage())
		}
		if mock.lastCancelled != "transfer-abc123" {
			t.Fatalf("orchestrator saw cancel for %q", mock.lastCancelled)
		}

		mock.cancelErr = errString("transfer already finished")
		defer func() { mock.cancelErr = nil }()
		resp, err = client.CancelTransfer(ctx, &phonebridgelocalipcv1.CancelTransferRequest{TransferId: "transfer-def456"})
		if err != nil {
			t.Fatalf("CancelTransfer: %v", err)
		}
		if resp.GetCancelled() {
			t.Fatal("cancelled must be false when the engine refuses")
		}
		if resp.GetErrorMessage() != "transfer already finished" {
			t.Fatalf("error_message = %q", resp.GetErrorMessage())
		}

		if _, err := client.CancelTransfer(ctx, &phonebridgelocalipcv1.CancelTransferRequest{}); status.Code(err) != codes.InvalidArgument {
			t.Fatalf("empty transfer_id: want InvalidArgument, got %v", err)
		}
	})
}

// TestServer_BroadcastTransferEvent proves the pushed transfer transition is
// delivered on the same StreamEvents stream as session and clipboard events, so
// the UI needs no second subscription.
func TestServer_BroadcastTransferEvent(t *testing.T) {
	tmpDir := t.TempDir()
	sock := filepath.Join(tmpDir, "engine.sock")
	tok := filepath.Join(tmpDir, "token")
	tokVal := "test-bearer-token-transfer-events"

	cfg := Config{
		SocketPath:    sock,
		TokenPath:     tok,
		Token:         tokVal,
		ServerVersion: "1.0.0",
		Orchestrator:  &mockOrchestrator{},
	}

	s, cancel, errCh := startTestServer(t, cfg)
	defer func() {
		cancel()
		<-errCh
	}()

	client, err := Dial(context.Background(), sock, tokVal)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer client.Close()

	ctx, streamCancel := context.WithCancel(context.Background())
	defer streamCancel()
	stream, err := client.StreamEvents(ctx, &phonebridgelocalipcv1.StreamEventsRequest{})
	if err != nil {
		t.Fatalf("StreamEvents: %v", err)
	}

	// Push one transfer event and require it to arrive as a TransferEvent with
	// the same identity: no interleaving with other event types is allowed to
	// consume or reshape it.
	want := &phonebridgelocalipcv1.TransferEvent{
		Transfer: ToProtoTransferInfo(transfer.Info{
			TransferID:   "transfer-live-1",
			Direction:    transfer.DirectionInbound,
			State:        transfer.StateActive,
			PeerDeviceID: "pixel-9",
			Filename:     "photo.jpg",
			SizeBytes:    8192,
			StartedAtMs:  1726000000000,
			ReasonCode:   transfer.ReasonNone,
		}),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		// The subscriber registers asynchronously inside the stream handler, so
		// give it a moment before the first broadcast rather than racing it.
		time.Sleep(100 * time.Millisecond)
		s.BroadcastTransferEvent(want)
	}()

	for {
		resp, err := stream.Recv()
		if err != nil {
			t.Fatalf("stream recv: %v", err)
		}
		if resp.GetTransferEvent() == nil {
			continue
		}
		got := resp.GetTransferEvent().GetTransfer()
		if got.GetTransferId() != "transfer-live-1" {
			t.Fatalf("transfer_id = %q", got.GetTransferId())
		}
		if got.GetState() != phonebridgelocalipcv1.TransferState_TRANSFER_STATE_ACTIVE {
			t.Fatalf("state = %s", got.GetState())
		}
		if got.GetDirection() != phonebridgelocalipcv1.TransferDirection_TRANSFER_DIRECTION_INBOUND {
			t.Fatalf("direction = %s", got.GetDirection())
		}
		if got.GetPeerDeviceId() != "pixel-9" || got.GetFilename() != "photo.jpg" {
			t.Fatalf("attribution lost: %v", got)
		}
		<-done
		return
	}
}

// errString is a tiny error type so the test can assert the daemon passes a
// non-transfer error message through verbatim.
type errString string

func (e errString) Error() string { return string(e) }
