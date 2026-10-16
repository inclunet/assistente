//go:build !windows

package oauthsnapshot

import (
	"os"
	"syscall"
)

func (f *Files) syncDirectory() error {
	dir, err := f.root.Open(".")
	if err != nil {
		return ErrStorage
	}
	defer func() { _ = dir.Close() }()
	if err := dir.Sync(); err != nil {
		return ErrStorage
	}
	return nil
}

func openPrivateDirectory(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_DIRECTORY, 0)
}
func checkDirectoryHandle(*os.File) error { return nil }
func rejectReparse(string) error          { return nil }
func protect(file *os.File, directory bool) error {
	mode := os.FileMode(0600)
	if directory {
		mode = 0700
	}
	if err := file.Chmod(mode); err != nil {
		return err
	}
	info, err := file.Stat()
	if err != nil || info.Mode().Perm() != mode {
		return ErrStorage
	}
	return nil
}
