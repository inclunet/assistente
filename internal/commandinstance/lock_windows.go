//go:build windows

package commandinstance

import (
	"fmt"
	"golang.org/x/sys/windows"
	"os"
)

const lockOffset uint64 = 0x7ffffffffffff000

func acquirePlatformLock(file *os.File) (string, func() error, error) {
	handle := windows.Handle(file.Fd())
	offset := windows.Overlapped{Offset: uint32(lockOffset & 0xffffffff), OffsetHigh: uint32(lockOffset >> 32)}
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err := windows.LockFileEx(handle, flags, 0, 1, 0, &offset); err != nil {
		if err == windows.ERROR_LOCK_VIOLATION || err == windows.ERROR_IO_PENDING {
			return "", nil, fmt.Errorf("%w: %v", ErrBusy, err)
		}
		return "", nil, err
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
		_ = windows.UnlockFileEx(handle, 0, 1, 0, &offset)
		return "", nil, err
	}
	// FileIndexHigh/Low is the stable file identity on Windows. A zero pair
	// cannot identify the locked file uniquely, so fail closed after Close
	// releases the byte-range lock.
	if info.FileIndexHigh == 0 && info.FileIndexLow == 0 {
		_ = file.Close()
		return "", nil, ErrUnsupported
	}
	identity := fmt.Sprintf("windows:%d:%d:%d", info.VolumeSerialNumber, info.FileIndexHigh, info.FileIndexLow)
	return identity, func() error {
		// Closing the handle releases the byte-range lock; do not retry
		// UnlockFileEx after an ambiguous Close result.
		return file.Close()
	}, nil
}
