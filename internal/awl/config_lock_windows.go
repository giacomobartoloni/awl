//go:build windows

package awl

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32DLL      = syscall.NewLazyDLL("kernel32.dll")
	lockFileExProc   = kernel32DLL.NewProc("LockFileEx")
	unlockFileExProc = kernel32DLL.NewProc("UnlockFileEx")
)

// flock uses a blocking, one-byte Windows file lock. The lock is released if
// the process terminates, and the handle remains open for the lock's lifetime.
func flock(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}

	var overlapped syscall.Overlapped
	locked, _, callErr := lockFileExProc.Call(
		file.Fd(), 0x2, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)),
	)
	if locked == 0 {
		file.Close()
		return nil, callErr
	}
	return func() {
		unlockFileExProc.Call(file.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
		file.Close()
	}, nil
}
