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
	buf := make([]uint16, 512)
	for {
		n, err := windows.GetFinalPathNameByHandle(windows.Handle(dir.Fd()), &buf[0], uint32(len(buf)), 0)
		if err != nil {
			return "", err
		}
		if n >= uint32(len(buf)) {
			buf = make([]uint16, n+1)
			continue
		}
		origin := windows.UTF16ToString(buf[:n])
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
}
