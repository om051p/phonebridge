//go:build !unix

package transfer

import (
	"io/fs"
	"os"
)

// openReadonlyPlatform is the portable fallback: open, then refuse anything that
// is not a regular file (openRegularReadonly performs the mode check). The unix
// build additionally passes O_NOFOLLOW.
func openReadonlyPlatform(path string) (*os.File, fs.FileInfo, error) {
	f, err := os.Open(path)
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
