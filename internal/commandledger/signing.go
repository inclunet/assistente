package commandledger

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

var ErrFingerprintKeyUnavailable = errors.New("chave de fingerprint indisponível")
var fingerprintKeyVersion = regexp.MustCompile(`^v[1-9][0-9]*$`)

// FingerprintKeyProvider é injetado pelo host, nunca por adapter/cliente. Deve
// fornecer bytes estáveis durante a chamada; SignLocalRead não altera a chave.
// O host mantém versões antigas pelo horizonte dos ledgers. Não há fallback.
type FingerprintKeyProvider func(context.Context, string) ([]byte, error)

// SignLocalRead calcula a projeção fechada read/none sem argumentos de D2.1.
// O serviço confiável já deve ter derivado identidade/origem, validado catálogo
// e contexto. Isto não autentica nem autoriza e não está ligado ao secret manager.
// version vem da configuração para primeira tentativa ou do ledger escopado
// para retry, nunca do payload. Fingerprints de ingresso são proibidos.
// Não é canonicalizador JSON genérico: só strings, constantes 1, {} e [].
func SignLocalRead(ctx context.Context, req LocalReadRequest, version string, keys FingerprintKeyProvider) (LocalReadRequest, error) {
	if ctx == nil || keys == nil || !fingerprintKeyVersion.MatchString(version) {
		return LocalReadRequest{}, ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return LocalReadRequest{}, err
	}
	if req.ArgumentsFingerprint != "" || req.RequestFingerprint != "" || req.RequestFingerprintVersion != "" {
		return LocalReadRequest{}, ErrInvalidRequest
	}
	candidate := req
	candidate.ArgumentsFingerprint = "pending"
	candidate.RequestFingerprint = "pending"
	candidate.RequestFingerprintVersion = version
	if err := validateRequest(candidate); err != nil {
		return LocalReadRequest{}, err
	}
	payload, err := canonicalLocalRead(req)
	if err != nil {
		return LocalReadRequest{}, err
	}
	material, err := keys(ctx, "command-request-hmac:"+version)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return LocalReadRequest{}, ctxErr
	}
	// Não propaga erro arbitrário do provider: ele pode conter material sensível.
	if err != nil || len(material) < 32 {
		return LocalReadRequest{}, ErrFingerprintKeyUnavailable
	}
	key := append([]byte(nil), material...)
	defer clear(key)
	digest := func(data []byte) string {
		mac := hmac.New(sha256.New, key)
		_, _ = mac.Write(data)
		return hex.EncodeToString(mac.Sum(nil))
	}
	candidate.RequestFingerprint = digest(payload)
	candidate.ArgumentsFingerprint = digest([]byte("{}"))
	return candidate, nil
}

// Chaves são fixas e ASCII: ordenação de bytes coincide com UTF-16 de JCS.
// Campos opcionais não suportados são omitidos, nunca representados por null.
// auth_generation e timestamps não participam da identidade semântica.
func canonicalLocalRead(req LocalReadRequest) ([]byte, error) {
	fields := map[string]string{
		"invocation_id": req.InvocationID, "command_id": req.CommandID,
		"user_id": req.Owner.UserID, "actor_type": "user", "actor_id": req.Owner.UserID,
		"auth_context_type": "local_session", "auth_context_id": req.Owner.AuthContextID,
		"session_id": req.Owner.AuthContextID, "security_generation": req.SecurityGeneration,
		"source_type": req.SourceType, "registry_version": req.RegistryVersion,
		"global_config_generation": req.GlobalConfigGeneration, "active_layers_generation": req.ActiveLayersGeneration,
		"correlation_id": req.CorrelationID, "effect": "read", "decision": "none", "context_policy": "none",
	}
	encoded := map[string]string{"version": "1", "arguments": "{}", "binding_ids": "[]"}
	for key, value := range fields {
		quoted, err := canonicalString(value)
		if err != nil {
			return nil, err
		}
		encoded[key] = quoted
	}
	names := make([]string, 0, len(encoded))
	for name := range encoded {
		names = append(names, name)
	}
	sort.Strings(names)
	var out strings.Builder
	out.WriteByte('{')
	for i, name := range names {
		if i > 0 {
			out.WriteByte(',')
		}
		out.WriteByte('"')
		out.WriteString(name)
		out.WriteString(`":`)
		out.WriteString(encoded[name])
	}
	out.WriteByte('}')
	return []byte(out.String()), nil
}

// RFC8785 §3.2.2.2: não escapa HTML/U+2028 nem normaliza Unicode.
func canonicalString(value string) (string, error) {
	if !utf8.ValidString(value) {
		return "", ErrInvalidRequest
	}
	const digits = "0123456789abcdef"
	var out strings.Builder
	out.WriteByte('"')
	for _, r := range value {
		switch r {
		case '"', '\\':
			out.WriteByte('\\')
			out.WriteRune(r)
		case '\b':
			out.WriteString(`\b`)
		case '\t':
			out.WriteString(`\t`)
		case '\n':
			out.WriteString(`\n`)
		case '\f':
			out.WriteString(`\f`)
		case '\r':
			out.WriteString(`\r`)
		default:
			if r < 32 {
				out.WriteString(`\u00`)
				out.WriteByte(digits[byte(r)>>4])
				out.WriteByte(digits[byte(r)&15])
			} else {
				out.WriteRune(r)
			}
		}
	}
	out.WriteByte('"')
	return out.String(), nil
}
