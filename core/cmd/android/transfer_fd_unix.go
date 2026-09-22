//go:build (android || jni) && unix

package main

import "syscall"

// dupFD duplicates a file descriptor the platform opened for us. Go must own
// the exact descriptor it closes: the platform keeps ownership of the original
// and closes it when the pending entry is finalized.
func dupFD(fd int) (int, error) {
	return syscall.Dup(fd)
}

// closeFD closes a descriptor that has not been wrapped by os.NewFile.
func closeFD(fd int) error {
	return syscall.Close(fd)
}
