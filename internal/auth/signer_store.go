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
//
// TODO(aep-0052/key-rotation): rotação multi-key com grace period.
// Cenário NÃO coberto hoje: rotação de emergência (chave vazada,
// suspeita de comprometimento). Procedimento atual:
//
//  1. Apagar manualmente `instance-jwt-signing-key` no cofre
//     (`asst data export --only-credentials` → editar → import) ou
//     resetar o vault via UI.
//  2. App gera nova chave automaticamente no próximo boot.
//  3. JWKS expõe só a nova chave; access tokens em voo (≤15min)
//     param de validar; clientes externos refazem fetch de JWKS.
//
// Janela de impacto: até 15min para access tokens em voo cessarem.
// Sessões abertas seguem via refresh (que emite novos access tokens
// com a nova chave). Aceitável para single-instance alpha; revisar
// antes de qualquer claim "enterprise multi-tenant" — multi-key com
// grace period vira requisito quando JWKS for consumido por
// integrações que não tolerem 15min de erro 401.
//
// Documentação operacional: docs/operations/key-rotation.md.
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
