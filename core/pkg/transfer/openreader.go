package transfer

import (
	"errors"
	"io/fs"
	"os"

	"github.com/om051p/phonebridge/core/pkg/protocol/phonebridgev1"
)

// openRegularReadonly opens a local source file for sending.
//
// DEC-024: the sender is the local user's own file, but the engine still refuses
// symlinks and non-regular files instead of following whatever a path points at
// when the transfer starts. The unix build uses O_NOFOLLOW (open_unix.go); the
// portable fallback in open_other.go checks the mode after opening.
func openRegularReadonly(path string) (*os.File, fs.FileInfo, error) {
	f, info, err := openReadonlyPlatform(path)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			return nil, nil, newFailure(phonebridgev1.Code_CODE_NOT_FOUND, ReasonStorageFailed, "file %q does not exist", path)
		case errors.Is(err, os.ErrPermission):
			return nil, nil, newFailure(phonebridgev1.Code_CODE_PERMISSION_DENIED, ReasonStorageFailed, "file %q is not readable", path)
		default:
			return nil, nil, newFailure(phonebridgev1.Code_CODE_STORAGE_FAILED, ReasonStorageFailed, "open %q: %v", path, err)
		}
	}
	if !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, newFailure(phonebridgev1.Code_CODE_INVALID_ARGUMENT, ReasonStorageFailed, "%q is not a regular file", path)
	}
	return f, info, nil
}
