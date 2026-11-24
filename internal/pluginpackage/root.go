// Package pluginpackage reads untrusted plugin files relative to their package root.
package pluginpackage

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// ReadFile reads a regular file through an already opened package root.
// Package paths use slashes on every platform. The caller supplies the byte
// budget for this component, including when the file grows during the read.
func ReadFile(root *os.Root, name string, limit int64) ([]byte, error) {
	if limit < 0 || name == "." || !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	path, err := filepath.Localize(name)
	if err != nil {
		return nil, err
	}
	info, err := root.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fs.ErrInvalid
	}
	file, err := root.OpenFile(path, os.O_RDONLY|readFlags, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err = file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fs.ErrInvalid
	}
	if info.Size() > limit {
		return nil, ErrTooLarge
	}
	return readLimited(file, limit)
}

func readLimited(file io.Reader, limit int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) < limit {
		return data, nil
	}
	var extra [1]byte
	if n, err := io.ReadFull(file, extra[:]); n != 0 {
		return nil, ErrTooLarge
	} else if err != io.EOF {
		return nil, err
	}
	return data, nil
}

var ErrTooLarge = errors.New("plugin file exceeds its size limit")
