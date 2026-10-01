package oauthflow

import (
	"errors"
	"github.com/google/uuid"
	"os"
	"path/filepath"
	"strings"
)

// HostID is non-secret installation metadata, independent of users and logout.
func HostID() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return hostIDAt(filepath.Join(root, "assistente"))
}
func readHostID(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(string(data))
	if _, err = uuid.Parse(strings.TrimPrefix(value, "urn:uuid:")); err != nil {
		return "", errors.New("oauth_host_id_invalid")
	}
	return value, nil
}
func hostIDAt(dir string) (string, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "oauth-host-id")
	if value, err := readHostID(path); err == nil {
		return value, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".oauth-host-id-*")
	if err != nil {
		return "", err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	value := "urn:uuid:" + uuid.NewString()
	_, writeErr := f.WriteString(value)
	syncErr := f.Sync()
	closeErr := f.Close()
	if writeErr != nil {
		return "", writeErr
	}
	if syncErr != nil {
		return "", syncErr
	}
	if closeErr != nil {
		return "", closeErr
	}
	// A hard link publishes the fully written inode atomically without replacing
	// a winner from another process. Crashes before publication leave only a temp.
	if err = os.Link(f.Name(), path); err != nil && !os.IsExist(err) {
		return "", err
	}
	return readHostID(path)
}
