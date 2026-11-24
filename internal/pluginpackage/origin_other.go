//go:build !darwin && !windows

package pluginpackage

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func packageOrigin(dir *os.File, absolute string) (string, error) {
	// On Linux the descriptor link also resolves case aliases on casefold mounts.
	if runtime.GOOS == "linux" {
		path, err := os.Readlink(fmt.Sprintf("/proc/self/fd/%d", dir.Fd()))
		runtime.KeepAlive(dir)
		if err == nil {
			return path, nil
		}
	}
	return filepath.EvalSymlinks(absolute)
}
