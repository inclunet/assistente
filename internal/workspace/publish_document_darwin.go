package workspace

import (
	"golang.org/x/sys/unix"
	"os"
)

func publishNewWorkspaceDocument(source, target string) error {
	if err := unix.RenamexNp(source, target, unix.RENAME_EXCL); err != nil {
		return &os.LinkError{Op: "publish", Old: source, New: target, Err: err}
	}
	return nil
}
