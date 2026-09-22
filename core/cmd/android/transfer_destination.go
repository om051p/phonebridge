//go:build android || jni

package main

// Platform destination (DEC-024): the Android side owns where a received file
// lands (MediaStore with IS_PENDING=1, or SAF), but the bytes are written by Go
// through the file descriptor the platform hands back, so nothing is copied
// twice and no per-write JNI call sits in the data path.
//
// This file is pure Go (no cgo) so the destination contract is host-testable
// under `go test -tags jni -race`: the Kotlin side is behind the TransferHost
// seam, exactly as the clipboard adapter is behind ClipboardHost.

import (
	"errors"
	"io"
	"os"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
	"github.com/om051p/phonebridge/core/pkg/transfer"
)

// TransferHost is the Kotlin-side storage host. Every method is flat because it
// crosses JNI: no Go pointers, no interfaces, no partial writes — the platform
// either accepts the whole buffer or reports how much it took.
type TransferHost interface {
	// BeginDownload creates the pending destination entry for a file and
	// returns its handle plus a writable file descriptor that the platform
	// already opened (MediaStore IS_PENDING row or a SAF document). An fd < 0
	// with a non-nil error means the platform refused the download.
	BeginDownload(filename, mimeType string, sizeBytes int64) (handle string, fd int, err error)
	// CommitDownload clears the pending state and returns the user-visible
	// display name (post collision rename). ok=false means the publish failed
	// and the entry must be discarded.
	CommitDownload(handle string) (displayName string, ok bool)
	// AbortDownload deletes the pending entry (and any bytes written to it).
	AbortDownload(handle string)
	// FreeSpaceBytes reports free space on the destination volume, or a
	// negative value when unknown (in which case the free-space policy is
	// skipped rather than guessed).
	FreeSpaceBytes() int64
	// OnOversizedFrame reports a frame that exceeded the protocol limit so the
	// UI can show that the peer misbehaved.
	OnOversizedFrame(size int)
}

// destinationFD is the minimum of *os.File's surface the committer needs, so a
// test can supply a pipe without pretending to be a real file.
type destinationFD interface {
	io.Writer
	Sync() error
	Close() error
}

// The destination never inspects content: integrity (size + SHA-256) is the
// engine's job, and verifying it twice would double the hashing cost for no
// extra safety.

// PlatformDestination implements transfer.Destination over TransferHost.
type PlatformDestination struct {
	host TransferHost
}

var _ transfer.Destination = (*PlatformDestination)(nil)

// NewPlatformDestination builds the Android destination.
func NewPlatformDestination(host TransferHost) *PlatformDestination {
	return &PlatformDestination{host: host}
}

// Begin asks the platform for a pending download and returns its committer.
func (d *PlatformDestination) Begin(meta transfer.Meta) (transfer.Committer, error) {
	if d.host == nil {
		return nil, transfer.NewFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, transfer.ReasonStorageFailed,
			"no storage host registered")
	}
	if err := d.checkFreeSpace(meta.SizeBytes); err != nil {
		return nil, err
	}

	handle, fd, err := d.host.BeginDownload(meta.Filename, meta.MimeType, int64(meta.SizeBytes))
	if err != nil {
		return nil, transfer.NewFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, transfer.ReasonStorageFailed,
			"cannot create the destination entry: %v", err)
	}
	if fd < 0 {
		return nil, transfer.NewFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, transfer.ReasonStorageFailed,
			"platform returned no writable descriptor for %q", meta.Filename)
	}

	// Dup the descriptor before wrapping it: the platform keeps ownership of the
	// original (it closes it when the entry is finalized) and the Go side must
	// not close a descriptor it does not own.
	dup, err := dupFD(fd)
	if err != nil {
		d.host.AbortDownload(handle)
		return nil, transfer.NewFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, transfer.ReasonStorageFailed,
			"cannot duplicate destination descriptor: %v", err)
	}

	file := os.NewFile(uintptr(dup), "phonebridge-transfer")
	if file == nil {
		_ = closeFD(dup)
		d.host.AbortDownload(handle)
		return nil, transfer.NewFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, transfer.ReasonStorageFailed,
			"invalid destination descriptor")
	}

	return &platformCommitter{
		host:      d.host,
		handle:    handle,
		file:      file,
		filename:  meta.Filename,
		bufferCap: transfer.DefaultChunkSize * 16,
	}, nil
}

// checkFreeSpace refuses an offer that cannot fit, using the platform's own
// answer. An unknown free-space value (negative) skips the check: refusing a
// download because the platform did not answer would be worse than letting the
// write fail with a typed storage error later.
func (d *PlatformDestination) checkFreeSpace(size uint64) error {
	if d.host == nil {
		return nil
	}
	free := d.host.FreeSpaceBytes()
	if free < 0 {
		return nil
	}
	if uint64(free) < size+transfer.DefaultFreeSpaceMargin {
		return transfer.NewFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, transfer.ReasonStorageFailed,
			"not enough free space: %d bytes available, need %d", free, size+transfer.DefaultFreeSpaceMargin)
	}
	return nil
}

// platformCommitter buffers writes so the data path is one syscall per ~1 MiB
// rather than one per chunk, then hands the file to the platform on Commit.
type platformCommitter struct {
	host     TransferHost
	handle   string
	file     destinationFD
	filename string

	// bufferCap bounds memory: a received file is never held in memory, only
	// this much at a time.
	bufferCap int

	written uint64

	buf      []byte
	finished bool
}

var _ transfer.Committer = (*platformCommitter)(nil)

// Write appends received bytes, flushing in bounded pieces so memory stays
// proportional to bufferCap instead of the file size.
func (c *platformCommitter) Write(p []byte) (int, error) {
	if c.finished {
		return 0, errors.New("transfer: destination already finalized")
	}
	c.buf = append(c.buf, p...)
	if len(c.buf) >= c.bufferCap {
		if err := c.flush(); err != nil {
			return 0, err
		}
	}
	c.written += uint64(len(p))
	return len(p), nil
}

func (c *platformCommitter) flush() error {
	if len(c.buf) == 0 {
		return nil
	}
	n, err := c.file.Write(c.buf)
	if err != nil {
		c.buf = c.buf[n:]
		return transfer.NewFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, transfer.ReasonStorageFailed,
			"write to destination failed: %v", err)
	}
	c.buf = c.buf[:0]
	return nil
}

// Commit flushes, closes the descriptor, and asks the platform to publish the
// file (IS_PENDING=0). A publish failure discards the entry rather than leaving
// a partial file where the user can see it.
func (c *platformCommitter) Commit() (string, error) {
	if c.finished {
		return "", errors.New("transfer: destination already finalized")
	}
	c.finished = true

	if err := c.flush(); err != nil {
		c.discard()
		return "", err
	}
	if err := c.file.Sync(); err != nil {
		c.discard()
		return "", transfer.NewFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, transfer.ReasonStorageFailed,
			"sync destination failed: %v", err)
	}
	if err := c.file.Close(); err != nil {
		c.discard()
		return "", transfer.NewFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, transfer.ReasonStorageFailed,
			"close destination failed: %v", err)
	}

	name, ok := c.host.CommitDownload(c.handle)
	if !ok {
		// The entry may be half-published; delete it and report the failure so
		// the sender learns nothing was stored.
		c.host.AbortDownload(c.handle)
		return "", transfer.NewFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, transfer.ReasonStorageFailed,
			"platform could not publish the received file")
	}
	if name == "" {
		name = c.filename
	}
	return name, nil
}

// Abort discards everything: the descriptor and the pending entry.
func (c *platformCommitter) Abort() error {
	if c.finished {
		return nil
	}
	c.finished = true
	c.discard()
	return nil
}

func (c *platformCommitter) discard() {
	c.buf = nil
	if c.file != nil {
		_ = c.file.Close()
	}
	if c.host != nil && c.handle != "" {
		c.host.AbortDownload(c.handle)
	}
}
