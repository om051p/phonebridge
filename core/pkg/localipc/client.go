// Package localipc provides client helpers for phonebridge.localipc.v1 over UDS.
package localipc

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgelocalipcv1"
)

// Client wraps phonebridgelocalipcv1.LocalEngineServiceClient with token injection.
type Client struct {
	phonebridgelocalipcv1.LocalEngineServiceClient
	conn  *grpc.ClientConn
	token string
}

// ReadTokenFile reads the bearer token from the specified file, stripping trailing whitespace.
func ReadTokenFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read token file %s: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

// Dial connects to the local daemon over the specified Unix domain socket path.
func Dial(ctx context.Context, socketPath string, token string) (*Client, error) {
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, "unix", socketPath)
	}

	unaryInterceptor := func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if token != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}

	streamInterceptor := func(ctx context.Context, desc *grpc.StreamDesc, cc *grpc.ClientConn, method string, streamer grpc.Streamer, opts ...grpc.CallOption) (grpc.ClientStream, error) {
		if token != "" {
			ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+token)
		}
		return streamer(ctx, desc, cc, method, opts...)
	}

	conn, err := grpc.DialContext(ctx, "passthrough:///unix",
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(unaryInterceptor),
		grpc.WithStreamInterceptor(streamInterceptor),
	)
	if err != nil {
		return nil, fmt.Errorf("dial localipc %s: %w", socketPath, err)
	}

	return &Client{
		LocalEngineServiceClient: phonebridgelocalipcv1.NewLocalEngineServiceClient(conn),
		conn:                     conn,
		token:                    token,
	}, nil
}

// Close closes the underlying gRPC connection.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// PollAndConnect polls for the token file and connects to the socket with exponential backoff.
func PollAndConnect(ctx context.Context, socketPath, tokenPath string, timeout time.Duration) (*Client, error) {
	deadline := time.Now().Add(timeout)
	backoff := 25 * time.Millisecond

	for {
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timeout waiting for daemon on %s", socketPath)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		token, err := ReadTokenFile(tokenPath)
		if err == nil && token != "" {
			client, err := Dial(ctx, socketPath, token)
			if err == nil {
				// Verify reachability via Health
				hCtx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
				resp, err := client.Health(hCtx, &phonebridgelocalipcv1.HealthRequest{})
				cancel()
				if err == nil && resp.GetReady() {
					return client, nil
				}
				client.Close()
			}
		}

		time.Sleep(backoff)
		if backoff < 200*time.Millisecond {
			backoff *= 2
		}
	}
}
