package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"

	"assistente/internal/credentials"
)

// LoadOrCreateTokenSigner carrega o signer Ed25519 persistido em
// credentials.InstanceSecretJWTSigningKey ou cria + persiste um novo.
//
// SECURITY (B3 do review da Fatia 1): o signer é estável enquanto o
// instance secret existir. Não há rotação automática hoje — toda chave
// gerada vive até alguém apagar manualmente o secret (reset de vault).
// Como access tokens duram 15min e o refresh path emite novos sem
// depender da chave antiga, uma troca de chave invalida access tokens
// em voo (aceitável) mas mantém sessões abertas via refresh. Multi-key
// rotation pode ser introduzida sem schema change estendendo o vetor
// retornado por TokenSigner.JWKSet com chaves "old" — fora de escopo
// para o PR atual.
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

// refreshTokenPepperLength é o tamanho em bytes do pepper persistido
// para HMAC dos refresh tokens. 32 bytes = 256 bits, o output natural
// de SHA-256 e o tamanho de chave recomendado por NIST FIPS 198-1.
const refreshTokenPepperLength = 32

// LoadOrCreateRefreshTokenPepper carrega ou cria o pepper usado pelo
// SessionService para HMAC-SHA256 dos refresh tokens persistidos
// (B2 do review da Fatia 1). Retorna nil se credMgr == nil — o caller
// (testes) decide se quer modo legacy SHA-256 puro nesse caso.
func LoadOrCreateRefreshTokenPepper(credMgr *credentials.Manager) ([]byte, error) {
	if credMgr == nil {
		return nil, nil
	}

	encoded, ok, err := credMgr.GetInstanceSecret(credentials.InstanceSecretRefreshTokenPepper)
	if err != nil {
		return nil, err
	}
	if ok {
		raw, err := base64.RawURLEncoding.DecodeString(encoded)
		if err != nil {
			return nil, fmt.Errorf("pepper de refresh token inválido: %w", err)
		}
		return raw, nil
	}

	pepper := make([]byte, refreshTokenPepperLength)
	if _, err := rand.Read(pepper); err != nil {
		return nil, err
	}
	encoded = base64.RawURLEncoding.EncodeToString(pepper)
	if err := credMgr.RegisterInstanceSecret(credentials.InstanceSecretRefreshTokenPepper, encoded); err != nil {
		return nil, fmt.Errorf("persistir pepper de refresh token: %w", err)
	}
	return pepper, nil
}
