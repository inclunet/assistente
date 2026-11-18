package workspace

import (
	"golang.org/x/sys/unix"
	"os"
)

func publishNewWorkspaceDocument(source, target string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE); err != nil {
		return &os.LinkError{Op: "publish", Old: source, New: target, Err: err}
	}
	return nil
}
