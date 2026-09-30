package oauthflow

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// HostID is non-secret installation metadata, independent of users and logout.
func HostID() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(root, "assistente")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "oauth-host-id")
	if data, err := os.ReadFile(path); err == nil {
		value := strings.TrimSpace(string(data))
		if _, err = uuid.Parse(strings.TrimPrefix(value, "urn:uuid:")); err != nil {
			return "", errors.New("oauth_host_id_invalid")
		}
		return value, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if os.IsExist(err) {
		return HostID()
	}
	if err != nil {
		return "", err
	}
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
	return value, nil
}
