package commandledger

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const maxConfigurationDocumentBytes = 256 * 1024

// ConfigurationMutationRequest é o snapshot privado de uma mutação de
// configuração global. Os documentos não são uma projeção pública nem são
// reserializados pelo signer: seus bytes completos participam da assinatura.
type ConfigurationMutationRequest struct {
	MutationID, UserID, SessionID, AuthGeneration, SecurityGeneration, GenerationID string
	Generation                                                                      int64
	BeforeDocument, AfterDocument                                                   string
}

// SignConfigurationMutation assina uma proposta de troca de enabled de um
// binding global. O host deve fornecer a chave já existente; este caminho não
// cria chaves, inicializa store ou consulta armazenamento.
func SignConfigurationMutation(ctx context.Context, req ConfigurationMutationRequest, version string, keys FingerprintKeyProvider) (string, error) {
	if ctx == nil || keys == nil || !fingerprintKeyVersion.MatchString(version) {
		return "", ErrInvalidRequest
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := validateConfigurationMutationRequest(req); err != nil {
		return "", err
	}
	payload, err := canonicalConfigurationMutation(req)
	if err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	material, err := keys(ctx, "command-request-hmac:"+version)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	// Não propaga o erro arbitrário do provider: ele pode revelar segredo,
	// caminho de armazenamento ou detalhes internos do host.
	if err != nil || len(material) < 32 {
		return "", ErrFingerprintKeyUnavailable
	}
	key := append([]byte(nil), material...)
	defer clear(key)

	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write(payload)
	return version + ":" + hex.EncodeToString(mac.Sum(nil)), nil
}

func validateConfigurationMutationRequest(req ConfigurationMutationRequest) error {
	for _, id := range []string{req.MutationID, req.UserID, req.SessionID, req.GenerationID} {
		if !validUUID(id) {
			return ErrInvalidRequest
		}
	}
	for _, generation := range []string{req.AuthGeneration, req.SecurityGeneration} {
		if !validConfigurationGeneration(generation) {
			return ErrInvalidRequest
		}
	}
	if req.Generation < 1 || !validOpaqueConfigurationDocument(req.BeforeDocument) || !validOpaqueConfigurationDocument(req.AfterDocument) {
		return ErrInvalidRequest
	}
	return nil
}

func validConfigurationGeneration(value string) bool {
	return utf8.ValidString(value) && value != "" && strings.TrimSpace(value) == value && len(value) <= 256
}

func validOpaqueConfigurationDocument(value string) bool {
	if len(value) > maxConfigurationDocumentBytes || !utf8.ValidString(value) || !json.Valid([]byte(value)) {
		return false
	}
	trimmed := strings.TrimSpace(value)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// canonicalConfigurationMutation produz somente o documento fechado desta
// operação. Todos os valores são strings canônicas; generation é decimal para
// preservar integralmente qualquer int64, e os documentos internos ficam
// opacos, sem canonização JSON adicional.
func canonicalConfigurationMutation(req ConfigurationMutationRequest) ([]byte, error) {
	if err := validateConfigurationMutationRequest(req); err != nil {
		return nil, err
	}
	fields := map[string]string{
		"action":              "binding_enabled",
		"after_document":      req.AfterDocument,
		"auth_context_type":   "local_session",
		"auth_generation":     req.AuthGeneration,
		"before_document":     req.BeforeDocument,
		"domain":              "config",
		"generation":          strconv.FormatInt(req.Generation, 10),
		"generation_id":       req.GenerationID,
		"mutation_id":         req.MutationID,
		"scope":               "global",
		"security_generation": req.SecurityGeneration,
		"session_id":          req.SessionID,
		"user_id":             req.UserID,
	}

	encoded := make(map[string]string, len(fields))
	for name, value := range fields {
		quoted, err := canonicalString(value)
		if err != nil {
			return nil, err
		}
		encoded[name] = quoted
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
