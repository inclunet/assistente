package commandledger

import (
	"context"
	"encoding/base64"
	"strings"

	"assistente/internal/credentials"
)

// NewCredentialKeyProvider conecta o signer ao Manager já inicializado pelo
// host. Não inicializa cofre, não acessa keychain, não gera/persiste chaves e
// não faz fallback. O bootstrap deve carregar os segredos antes de usá-lo.
// O nome lógico command-request-hmac:vN é armazenado no namespace reservado
// internal-auth:command-request-hmac:vN como secret em base64url sem padding.
// Provisionamento, retenção e rotação permanecem responsabilidade do host.
func NewCredentialKeyProvider(manager *credentials.Manager) (FingerprintKeyProvider, error) {
	if manager == nil {
		return nil, ErrFingerprintKeyUnavailable
	}
	return func(ctx context.Context, name string) ([]byte, error) {
		if ctx == nil {
			return nil, ErrInvalidRequest
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		version, ok := strings.CutPrefix(name, "command-request-hmac:")
		if !ok || !fingerprintKeyVersion.MatchString(version) {
			return nil, ErrInvalidRequest
		}
		// A leitura bruta sem contexto de usuário lê apenas UserID vazio e por nome
		// exato. GetInstanceSecret tem fallback legado user-scoped, proibido aqui.
		auth, err := manager.GetConfigByPatternWithContext(context.Background(), "internal-auth:"+name)
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if err != nil || auth == nil || auth.Type != "secret" {
			return nil, ErrFingerprintKeyUnavailable
		}
		raw, err := base64.RawURLEncoding.Strict().DecodeString(auth.Token)
		if err != nil || len(raw) < 32 || base64.RawURLEncoding.EncodeToString(raw) != auth.Token {
			clear(raw)
			return nil, ErrFingerprintKeyUnavailable
		}
		return raw, nil
	}, nil
}
