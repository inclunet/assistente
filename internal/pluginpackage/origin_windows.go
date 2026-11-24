package pluginpackage

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/windows"
)

// Query the opened directory so DOS, extended and device paths share one origin.
func packageOrigin(dir *os.File, _ string) (string, error) {
	defer runtime.KeepAlive(dir)
	origin, err := finalPackagePath(windows.Handle(dir.Fd()), queryPackagePath)
	if err != nil {
		return "", err
	}
	// Preserve the extended spelling for paths that need it. Prefer the
	// ordinary spelling for display only if it reopens the same directory.
	ordinary := strings.TrimPrefix(origin, `\\?\`)
	if strings.HasPrefix(ordinary, `UNC\`) {
		ordinary = `\\` + ordinary[4:]
		volume := filepath.VolumeName(ordinary)
		ordinary = strings.ToLower(volume) + ordinary[len(volume):]
	}
	if filepath.IsAbs(ordinary) {
		current, err := dir.Stat()
		if err != nil {
			return "", err
		}
		if candidate, err := os.Stat(ordinary); err == nil && os.SameFile(current, candidate) {
			return ordinary, nil
		}
	}
	return origin, nil
}

const volumeNameGUID = 1 // VOLUME_NAME_GUID in GetFinalPathNameByHandleW.

func finalPackagePath(handle windows.Handle, query func(windows.Handle, uint32) (string, error)) (string, error) {
	path, err := query(handle, 0) // VOLUME_NAME_DOS
	if err == windows.ERROR_PATH_NOT_FOUND {
		// A volume without a DOS name can still be opened by its GUID.
		return query(handle, volumeNameGUID)
	}
	return path, err
}

func queryPackagePath(handle windows.Handle, flags uint32) (string, error) {
	buf := make([]uint16, 512)
	for {
		n, err := windows.GetFinalPathNameByHandle(handle, &buf[0], uint32(len(buf)), flags)
		if err != nil {
			return "", err
		}
		if n >= uint32(len(buf)) {
			buf = make([]uint16, n+1)
			continue
		}
		return windows.UTF16ToString(buf[:n]), nil
	}
}
