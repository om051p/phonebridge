//go:build unix

package transfer

import (
	"io/fs"
	"os"
	"syscall"
)

// openReadonlyPlatform opens path read-only without following a final symlink
// (DEC-024: a source path must not resolve to something else mid-transfer).
func openReadonlyPlatform(path string) (*os.File, fs.FileInfo, error) {
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, info, nil
}
