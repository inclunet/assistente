package auth

import (
	"fmt"

	"assistente/internal/credentials"
)

func LoadOrCreateTokenSigner(credMgr *credentials.Manager) (*TokenSigner, error) {
	if credMgr == nil {
		return NewTokenSigner()
	}

	encoded, ok, err := credMgr.GetInstanceSecret(credentials.InstanceSecretJWTSigningKey)
	if err != nil {
		return nil, err
	}
	if ok {
		return TokenSignerFromEncodedPrivateKey(encoded)
	}

	signer, err := NewTokenSigner()
	if err != nil {
		return nil, err
	}
	exported, err := signer.ExportPrivateKey()
	if err != nil {
		return nil, err
	}
	if err := credMgr.RegisterInstanceSecret(credentials.InstanceSecretJWTSigningKey, exported); err != nil {
		return nil, fmt.Errorf("persistir chave JWT: %w", err)
	}
	return signer, nil
}
