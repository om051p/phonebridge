// Package localipc provides cross-platform stubs for non-Linux platforms.
//
//go:build !linux

package localipc

import (
	"context"
	"errors"
	"net"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
)

type PeerInfo struct {
	UID uint32
	GID uint32
	PID int32
}

func (p *PeerInfo) AuthType() string { return "unix-peercred-stub" }
func (p *PeerInfo) String() string   { return "peercred-unsupported" }

type UnixPeerCreds struct{}

func (UnixPeerCreds) ClientHandshake(_ context.Context, _ string, rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return rawConn, nil, nil
}

func (UnixPeerCreds) ServerHandshake(rawConn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return rawConn, &PeerInfo{UID: 0, GID: 0, PID: 0}, nil
}

func (UnixPeerCreds) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "insecure", SecurityVersion: "stub"}
}

func (c UnixPeerCreds) Clone() credentials.TransportCredentials { return c }
func (UnixPeerCreds) OverrideServerName(string) error           { return nil }

func PeerCred(conn net.Conn) (*PeerInfo, error) {
	return nil, errors.New("SO_PEERCRED is only supported on Linux")
}

func PeerInfoFromContext(ctx context.Context) (*PeerInfo, bool) {
	p, ok := peer.FromContext(ctx)
	if !ok || p.AuthInfo == nil {
		return nil, false
	}
	info, ok := p.AuthInfo.(*PeerInfo)
	return info, ok
}
