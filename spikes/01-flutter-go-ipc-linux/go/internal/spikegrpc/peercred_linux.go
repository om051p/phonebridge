// Package spikegrpc — Spike 01 (EXPERIMENTAL, throwaway).
//
// Peer-credential extraction for the UDS trust boundary: SO_PEERCRED on an
// accepted AF_UNIX socket is kernel-provided evidence of which process is on
// the other end. Socket file permissions stop *other users* from connecting;
// SO_PEERCRED additionally lets the daemon verify the peer instead of trusting
// the filesystem check alone.
//
// Linux-only by design (SO_PEERCRED is a Linux/BSD feature and the spike's
// target is Wayland/COSMIC Linux).
//
//go:build linux

package spikegrpc

import (
	"context"
	"fmt"
	"net"
	"syscall"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

// PeerInfo is the kernel-reported identity of the process on the far end of an
// accepted Unix domain socket.
type PeerInfo struct {
	UID uint32
	GID uint32
	PID int32
}

// AuthType implements credentials.AuthInfo.
func (p *PeerInfo) AuthType() string { return "unix-peercred" }

// String renders the identity for logs. The token is never part of it.
func (p *PeerInfo) String() string {
	return fmt.Sprintf("uid=%d gid=%d pid=%d", p.UID, p.GID, p.PID)
}

// insecureAuthInfo is the client-side AuthInfo for the spike's cleartext UDS.
type insecureAuthInfo struct{}

func (insecureAuthInfo) AuthType() string { return "insecure" }

// UnixPeerCreds is a TransportCredentials that performs no crypto (the socket
// never leaves the machine and is filesystem-protected) but captures
// SO_PEERCRED on the server side. Keeping it in the credentials layer means the
// peer identity is available on *every* RPC — unary and streaming — without the
// client being able to spoof or omit it.
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
