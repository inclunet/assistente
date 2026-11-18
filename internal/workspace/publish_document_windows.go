package workspace

import (
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
)

func publishNewWorkspaceDocument(source, target string) error {
	from, err := workspaceWindowsPath(source)
	if err != nil {
		return err
	}
	to, err := workspaceWindowsPath(target)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return &os.LinkError{Op: "publish", Old: source, New: target, Err: err}
	}
	return nil
}

func workspaceWindowsPath(path string) (*uint16, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(absolute, `\\?\`) {
		if strings.HasPrefix(absolute, `\\`) {
			absolute = `\\?\UNC\` + strings.TrimPrefix(absolute, `\\`)
		} else {
			absolute = `\\?\` + absolute
		}
	}
	return windows.UTF16PtrFromString(absolute)
}
