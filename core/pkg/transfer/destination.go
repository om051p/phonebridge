package transfer

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// partialDirName is the staging directory created inside the destination so a
// verified file can be promoted with a same-filesystem atomic rename while an
// unverified partial is never visible where the user looks (DEC-024).
const partialDirName = ".phonebridge-partial"

// writeBufferBytes is the receiver's buffered write size: one megabyte bounds
// memory while keeping syscalls rare (DEC-024).
const writeBufferBytes = 1 << 20

// Meta describes an inbound transfer to a Destination.
type Meta struct {
	TransferID string
	Filename   string
	MimeType   string
	SizeBytes  uint64
}

// Committer is one destination writer. Write appends received bytes; Commit
// promotes the verified file and returns the stored basename; Abort discards
// every trace of it. Neither Commit nor Abort may be called after the other.
type Committer interface {
	io.Writer
	Commit() (savedName string, err error)
	Abort() error
}

// Destination creates the writer for an inbound file. The Linux/plain-file
// implementation is FileDestination; Android supplies a MediaStore-pending
// implementation behind the same seam (DEC-024), which is why the engine never
// talks about paths.
type Destination interface {
	Begin(meta Meta) (Committer, error)
}

// FileDestination stores received files in Dir and stages partials in a hidden
// subdirectory of Dir.
type FileDestination struct {
	dir             string
	partialDir      string
	freeSpaceMargin uint64
	now             func() time.Time
}

// FileDestinationConfig configures NewFileDestination.
type FileDestinationConfig struct {
	// Dir is the destination directory. Empty means DefaultDownloadDir().
	Dir string
	// FreeSpaceMargin is how much space beyond the file size must be free
	// before an offer is accepted (0 means DefaultFreeSpaceMargin).
	FreeSpaceMargin uint64
	// Now is the clock seam.
	Now func() time.Time
}

// NewFileDestination resolves and creates the destination directories.
func NewFileDestination(cfg FileDestinationConfig) (*FileDestination, error) {
	dir := cfg.Dir
	if dir == "" {
		dir = DefaultDownloadDir()
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "resolve destination %q: %v", dir, err)
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "create destination %q: %v", abs, err)
	}
	partial := filepath.Join(abs, partialDirName)
	if err := os.MkdirAll(partial, 0o700); err != nil {
		return nil, newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "create staging directory %q: %v", partial, err)
	}
	margin := cfg.FreeSpaceMargin
	if margin == 0 {
		margin = DefaultFreeSpaceMargin
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &FileDestination{dir: abs, partialDir: partial, freeSpaceMargin: margin, now: now}, nil
}

// Dir returns the resolved destination directory.
func (d *FileDestination) Dir() string { return d.dir }

// PartialDir returns the staging directory.
func (d *FileDestination) PartialDir() string { return d.partialDir }

// SweepPartial removes every staged partial. Phase 4 has no resume, so nothing
// in the staging directory is ever worth keeping across a restart; the daemon
// calls this at startup.
func (d *FileDestination) SweepPartial() error {
	entries, err := os.ReadDir(d.partialDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "read staging directory: %v", err)
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(d.partialDir, e.Name())); err != nil {
			return newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "remove staged partial %q: %v", e.Name(), err)
		}
	}
	return nil
}

// Begin creates the staged partial for one inbound transfer.
func (d *FileDestination) Begin(meta Meta) (Committer, error) {
	name, err := SanitizeFilename(meta.Filename)
	if err != nil {
		return nil, err
	}
	if err := d.checkFreeSpace(meta.SizeBytes); err != nil {
		return nil, err
	}

	path := filepath.Join(d.partialDir, meta.TransferID+".part")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return nil, newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "create staging file: %v", err)
	}
	return &fileCommitter{
		d:    d,
		meta: meta,
		name: name,
		path: path,
		file: f,
		buf:  bufio.NewWriterSize(f, writeBufferBytes),
	}, nil
}

func (d *FileDestination) checkFreeSpace(size uint64) error {
	free, err := freeSpace(d.dir)
	if err != nil {
		// A filesystem that cannot report free space must not block delivery;
		// the write path still fails typed if the disk actually fills.
		return nil
	}
	if free < size+d.freeSpaceMargin {
		return newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed,
			"insufficient free space: %d bytes free, %d required", free, size+d.freeSpaceMargin)
	}
	return nil
}

// fileCommitter stages one file and promotes it by rename after verification.
type fileCommitter struct {
	d    *FileDestination
	meta Meta
	name string
	path string
	file *os.File
	buf  *bufio.Writer
	done bool
}

func (c *fileCommitter) Write(p []byte) (int, error) {
	n, err := c.buf.Write(p)
	if err != nil {
		return n, newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "write staged file: %v", err)
	}
	return n, nil
}

// Commit flushes, fsyncs and renames the verified file into place. The rename is
// atomic because the staging directory lives on the destination filesystem.
func (c *fileCommitter) Commit() (string, error) {
	if c.done {
		return "", newFailure(phonebridgev1.Code_CODE_INTERNAL, ReasonUnspecified, "commit after the committer finished")
	}
	c.done = true

	if err := c.buf.Flush(); err != nil {
		_ = c.discard()
		return "", newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "flush staged file: %v", err)
	}
	if err := c.file.Sync(); err != nil {
		_ = c.discard()
		return "", newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "sync staged file: %v", err)
	}
	if err := c.file.Close(); err != nil {
		_ = c.discard()
		return "", newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "close staged file: %v", err)
	}

	final, err := c.d.uniquePath(c.name)
	if err != nil {
		_ = c.discard()
		return "", err
	}
	if err := os.Rename(c.path, final); err != nil {
		_ = c.discard()
		return "", newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "promote staged file: %v", err)
	}
	return filepath.Base(final), nil
}

// Abort removes the staged partial. It is safe on an already-aborted committer.
func (c *fileCommitter) Abort() error {
	if c.done {
		return nil
	}
	c.done = true
	return c.discard()
}

func (c *fileCommitter) discard() error {
	var firstErr error
	if c.buf != nil {
		_ = c.buf.Flush() // best effort: the file is being deleted
	}
	if c.file != nil {
		if err := c.file.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := os.Remove(c.path); err != nil && !errors.Is(err, os.ErrNotExist) && firstErr == nil {
		firstErr = err
	}
	if firstErr != nil {
		return newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "discard staged file: %v", firstErr)
	}
	return nil
}

// uniquePath returns a destination path for name, appending " (n)" before the
// extension until it does not exist. The sender never learns the absolute path
// (only the basename travels back in FileResult.saved_name).
func (d *FileDestination) uniquePath(name string) (string, error) {
	candidate := filepath.Join(d.dir, name)
	if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
		return candidate, nil
	}
	ext := filepath.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	for n := 1; n <= 9999; n++ {
		candidate = filepath.Join(d.dir, fmt.Sprintf("%s (%d)%s", stem, n, ext))
		if _, err := os.Lstat(candidate); errors.Is(err, os.ErrNotExist) {
			return candidate, nil
		}
	}
	return "", newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "no free destination name for %q", name)
}

// DefaultDownloadDir resolves the user's Downloads directory without requiring
// root or a portal: $XDG_DOWNLOAD_DIR, then ~/.config/user-dirs.dirs, then
// ~/Downloads, then a temp fallback (DEC-024).
func DefaultDownloadDir() string {
	if v := strings.TrimSpace(os.Getenv("XDG_DOWNLOAD_DIR")); v != "" {
		return v
	}
	if v := userDirsDownload(); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, "Downloads")
	}
	return filepath.Join(os.TempDir(), "phonebridge-downloads")
}

// userDirsDownload parses XDG_DOWNLOAD_DIR out of user-dirs.dirs, expanding
// $HOME / ${HOME} as the spec's shell-style value requires.
func userDirsDownload() string {
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return ""
		}
		base = filepath.Join(home, ".config")
	}
	data, err := os.ReadFile(filepath.Join(base, "user-dirs.dirs"))
	if err != nil {
		return ""
	}
	home, _ := os.UserHomeDir()
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "XDG_DOWNLOAD_DIR=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "XDG_DOWNLOAD_DIR="))
		value = strings.Trim(value, `"`)
		value = strings.ReplaceAll(value, "${HOME}", home)
		value = strings.ReplaceAll(value, "$HOME", home)
		if value == "" {
			continue
		}
		if strings.HasPrefix(value, "$") {
			// An unresolvable variable is a path that may not exist; keep the
			// literal value only when it is absolute, otherwise fall through.
			if !filepath.IsAbs(value) {
				continue
			}
		}
		return value
	}
	return ""
}
