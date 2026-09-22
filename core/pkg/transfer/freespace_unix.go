//go:build unix

package transfer

import "syscall"

// freeSpace reports the bytes available to this user on the filesystem holding
// path. It is used for the receiver's free-space preflight (DEC-024) and lives in
// a build-tagged file because syscall.Statfs is Unix/Android only.
func freeSpace(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return uint64(st.Bavail) * uint64(st.Bsize), nil
}
