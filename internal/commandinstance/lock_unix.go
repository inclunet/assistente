//go:build linux || darwin

package commandinstance

import (
	"fmt"
	"os"
	"syscall"
)

func acquirePlatformLock(file *os.File) (string, func() error, error) {
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if err == syscall.EWOULDBLOCK || err == syscall.EAGAIN {
			return "", nil, fmt.Errorf("%w: %v", ErrBusy, err)
		}
		return "", nil, err
	}
	info, err := file.Stat()
	if err != nil {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		return "", nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		return "", nil, ErrUnsupported
	}
	identity := fmt.Sprintf("unix:%d:%d", stat.Dev, stat.Ino)
	return identity, func() error {
		// Closing the descriptor releases flock even if Close reports an
		// ambiguous OS-level error; do not retry unlock on a closed fd.
		return file.Close()
	}, nil
}
