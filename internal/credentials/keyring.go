package credentials

import (
	"encoding/base64"
	"errors"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "assistente"
	keyringUser    = "credential_dek"
)

// LoadDEKFromKeychain busca a DEK no keychain do SO.
func LoadDEKFromKeychain() ([]byte, error) {
	value, err := keyring.Get(keyringService, keyringUser)
	if err != nil {
		return nil, err
	}
	return base64.StdEncoding.DecodeString(value)
}

// SaveDEKToKeychain salva a DEK no keychain do SO.
func SaveDEKToKeychain(dek []byte) error {
	encoded := base64.StdEncoding.EncodeToString(dek)
	return keyring.Set(keyringService, keyringUser, encoded)
}

// IsKeychainNotFound indica se o erro é de item inexistente no keychain.
func IsKeychainNotFound(err error) bool {
	return errors.Is(err, keyring.ErrNotFound)
}
