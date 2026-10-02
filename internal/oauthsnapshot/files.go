// Package oauthsnapshot stores only opaque encrypted OAuth recovery envelopes.
package oauthsnapshot

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

var ErrStorage = errors.New("oauth_snapshot_storage")
var validID = regexp.MustCompile(`^[a-f0-9-]{36}$`)

const maxSize = 4 << 20

type Files struct{ root *os.Root }

// Open refuses symlinks/reparse points and restricts the directory before any
// ciphertext is written. os.Root confines every subsequent filesystem action.
func Open(path string) (*Files, error) {
	if !filepath.IsAbs(path) {
		return nil, ErrStorage
	}
	if err := os.MkdirAll(path, 0700); err != nil {
		return nil, ErrStorage
	}
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrStorage
		}
		if err := rejectReparse(current); err != nil {
			return nil, ErrStorage
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	f, err := openPrivateDirectory(path)
	if err != nil {
		return nil, ErrStorage
	}
	if err = protect(f, true); err != nil {
		_ = f.Close()
		return nil, ErrStorage
	}
	_ = f.Close()
	root, err := os.OpenRoot(path)
	if err != nil {
		return nil, ErrStorage
	}
	return &Files{root: root}, nil
}
func (f *Files) Close() { _ = f.root.Close() }

func (f *Files) Write(id string, encrypted []byte) error {
	return f.WriteGuarded(id, encrypted, func(publish func() error) error { return publish() })
}

func (f *Files) WriteGuarded(id string, encrypted []byte, guard func(func() error) error) error {
	if !validID.MatchString(id) || len(encrypted) == 0 || len(encrypted) > maxSize {
		return ErrStorage
	}
	tmp := uuid.NewString() + ".part"
	file, err := f.root.OpenFile(tmp, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return ErrStorage
	}
	defer func() { _ = file.Close(); _ = f.root.Remove(tmp) }()
	if err := protect(file, false); err != nil {
		return ErrStorage
	}
	if _, err := file.Write(encrypted); err != nil {
		return ErrStorage
	}
	if err := file.Sync(); err != nil {
		return ErrStorage
	}
	if err := file.Close(); err != nil {
		return ErrStorage
	}
	// Link publishes without replacing another snapshot, including another process.
	if err := guard(func() error { return f.root.Link(tmp, id+".oauth") }); err != nil {
		return ErrStorage
	}
	return f.syncDirectory()
}
func (f *Files) Read(id string) ([]byte, error) {
	if !validID.MatchString(id) {
		return nil, ErrStorage
	}
	info, err := f.root.Lstat(id + ".oauth")
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxSize {
		return nil, ErrStorage
	}
	file, err := f.root.OpenFile(id+".oauth", os.O_RDWR, 0)
	if err != nil {
		return nil, ErrStorage
	}
	defer func() { _ = file.Close() }()
	if err := protect(file, false); err != nil {
		return nil, ErrStorage
	}
	data, err := io.ReadAll(io.LimitReader(file, maxSize+1))
	if err != nil || len(data) > maxSize {
		return nil, ErrStorage
	}
	return data, nil
}
func (f *Files) List() ([]string, error) {
	dir, err := f.root.Open(".")
	if err != nil {
		return nil, ErrStorage
	}
	defer func() { _ = dir.Close() }()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, ErrStorage
	}
	ids := []string{}
	for _, entry := range entries {
		id := strings.TrimSuffix(entry.Name(), ".oauth")
		if id != entry.Name() && validID.MatchString(id) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}
func (f *Files) Delete(id string) error {
	return f.DeleteGuarded(id, func(remove func() error) error { return remove() })
}

func (f *Files) DeleteGuarded(id string, guard func(func() error) error) error {
	if !validID.MatchString(id) {
		return ErrStorage
	}
	if err := guard(func() error { return f.root.Remove(id + ".oauth") }); err != nil {
		return ErrStorage
	}
	return f.syncDirectory()
}
