// Package localipc provides the production Unix Domain Socket + gRPC local IPC transport
// ratified by DEC-018 for the same-user, same-machine boundary between Flutter UI and Go daemon.
//
//go:build linux

package localipc

import (
	"context"
	"fmt"
	"net"
	"syscall"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// PeerInfo is the kernel-reported identity of the process on the far end of an
// accepted Unix domain socket (SO_PEERCRED).
type PeerInfo struct {
	UID uint32
	GID uint32
	PID int32
}

// AuthType implements credentials.AuthInfo.
func (p *PeerInfo) AuthType() string { return "unix-peercred" }

// String renders the identity for logs. Token is never logged.
func (p *PeerInfo) String() string {
	return fmt.Sprintf("uid=%d gid=%d pid=%d", p.UID, p.GID, p.PID)
}

// insecureAuthInfo is the client-side AuthInfo for the local cleartext UDS.
type insecureAuthInfo struct{}

func (insecureAuthInfo) AuthType() string { return "insecure" }

// UnixPeerCreds is a TransportCredentials that captures SO_PEERCRED on the server
// side while running over a local Unix domain socket.
type UnixPeerCreds struct{}

// ClientHandshake implements credentials.TransportCredentials.
func (UnixPeerCreds) ClientHandshake(_ context.Context, _ string, rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return rawConn, insecureAuthInfo{}, nil
}

// ServerHandshake implements credentials.TransportCredentials.
func (UnixPeerCreds) ServerHandshake(rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	info, err := PeerCred(rawConn)
	if err != nil {
		return nil, nil, err
	}
	return rawConn, info, nil
}

// Info implements credentials.TransportCredentials.
func (UnixPeerCreds) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{
		SecurityProtocol: "insecure",
		SecurityVersion:  "unix-peercred",
	}
}

// Clone implements credentials.TransportCredentials.
func (c UnixPeerCreds) Clone() credentials.TransportCredentials { return c }

// OverrideServerName implements credentials.TransportCredentials (no-op).
func (UnixPeerCreds) OverrideServerName(string) error { return nil }

// PeerCred reads SO_PEERCRED from an accepted Unix socket connection.
func PeerCred(conn net.Conn) (*PeerInfo, error) {
	sc, ok := conn.(syscall.Conn)
	if !ok {
		return nil, fmt.Errorf("peercred: %T does not implement syscall.Conn", conn)
	}
	raw, err := sc.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("peercred: SyscallConn: %w", err)
	}
	var (
		ucred *syscall.Ucred
		cerr  error
	)
	if err := raw.Control(func(fd uintptr) {
		ucred, cerr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return nil, fmt.Errorf("peercred: control: %w", err)
	}
	if cerr != nil {
		return nil, fmt.Errorf("peercred: getsockopt(SO_PEERCRED): %w", cerr)
	}
	if ucred == nil {
		return nil, fmt.Errorf("peercred: kernel returned no credentials")
	}
	return &PeerInfo{UID: ucred.Uid, GID: ucred.Gid, PID: ucred.Pid}, nil
}

// PeerInfoFromContext returns the peer identity attached by UnixPeerCreds.
func PeerInfoFromContext(ctx context.Context) (*PeerInfo, bool) {
	p, ok := peer.FromContext(ctx)
	if !ok || p.AuthInfo == nil {
		return nil, false
	}
	info, ok := p.AuthInfo.(*PeerInfo)
	return info, ok
}
