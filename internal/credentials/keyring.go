package credentials

import (
	"encoding/base64"
	"errors"

	"github.com/zalando/go-keyring"
)

const (
	keyringService          = "assistente"
	keyringUser             = "credential_dek"
	authRefreshTokenKeyUser = "auth_refresh_token"
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

func LoadAuthRefreshTokenFromKeychain() (string, error) {
	return keyring.Get(keyringService, authRefreshTokenKeyUser)
}

func SaveAuthRefreshTokenToKeychain(refreshToken string) error {
	return keyring.Set(keyringService, authRefreshTokenKeyUser, refreshToken)
}

func DeleteAuthRefreshTokenFromKeychain() error {
	return keyring.Delete(keyringService, authRefreshTokenKeyUser)
}

// IsKeychainNotFound indica se o erro é de item inexistente no keychain.
func IsKeychainNotFound(err error) bool {
	return errors.Is(err, keyring.ErrNotFound)
}

// KeyringEntry representa uma entrada do keyring do SO.
type KeyringEntry struct {
	Target string
	User   string
}
