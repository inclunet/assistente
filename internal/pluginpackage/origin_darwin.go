package pluginpackage

import (
	"os"
	"runtime"
	"unsafe"

	"golang.org/x/sys/unix"
)

// F_GETPATH returns the spelling of the opened directory on the mounted volume.
func packageOrigin(dir *os.File, _ string) (string, error) {
	var path [unix.PathMax]byte
	_, _, errno := unix.Syscall(unix.SYS_FCNTL, dir.Fd(), unix.F_GETPATH, uintptr(unsafe.Pointer(&path[0])))
	runtime.KeepAlive(dir)
	if errno != 0 {
		return "", errno
	}
	return unix.ByteSliceToString(path[:]), nil
}
