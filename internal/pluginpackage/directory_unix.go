//go:build unix

package pluginpackage

import (
	"fmt"
	"os"
	"syscall"
)

func directoryID(file *os.File) (string, error) {
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	raw, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", fmt.Errorf("unsupported directory identity")
	}
	return fmt.Sprintf("%x:%x", raw.Dev, raw.Ino), nil
}
