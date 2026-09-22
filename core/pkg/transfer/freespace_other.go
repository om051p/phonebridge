//go:build !unix

package transfer

import "errors"

// freeSpace is unavailable on this platform; the receiver treats "cannot report
// free space" as "do not block delivery" and still fails typed if the disk
// actually fills up.
func freeSpace(path string) (uint64, error) {
	return 0, errors.New("free space is not supported on this platform")
}
