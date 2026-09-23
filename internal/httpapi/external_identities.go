package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"assistente/internal/auth"
)

// Cadastro administrativo não publica readiness nem altera a identidade usada
// pelas APIs existentes. O cutover exige uma migração explícita posterior.
func (s *Server) externalIdentityToken(w http.ResponseWriter, r *http.Request) (string, bool) {
	if s.mode != "external" {
		http.NotFound(w, r)
		return "", false
	}
	if s.externalIdentityAdmin == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "cadastro externo indisponível"})
		return "", false
	}
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "credenciais inválidas"})
		return "", false
	}
	return parts[1], true
}

func decodeExternalIdentity(w http.ResponseWriter, r *http.Request, dst any) bool {
	defer r.Body.Close()
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8192))
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "solicitação inválida"})
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "solicitação inválida"})
		return false
	}
	object := bytes.TrimSpace(raw)
	if len(object) == 0 || object[0] != '{' {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "solicitação inválida"})
		return false
	}
	fields := json.NewDecoder(bytes.NewReader(object))
	fields.DisallowUnknownFields()
	if err := fields.Decode(dst); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "solicitação inválida"})
		return false
	}
	return true
}

func (s *Server) handleExternalIdentityBootstrap(w http.ResponseWriter, r *http.Request) {
	token, ok := s.externalIdentityToken(w, r)
	if !ok {
		return
	}
	// Não há userId/issuer/subject fornecido pelo cliente no primeiro vínculo.
	var request struct{}
	if !decodeExternalIdentity(w, r, &request) {
		return
	}
	mapping, err := s.externalIdentityAdmin.BootstrapLegacy(r.Context(), token)
	s.writeExternalIdentity(w, mapping, err)
}

func (s *Server) handleExternalIdentityCreate(w http.ResponseWriter, r *http.Request) {
	token, ok := s.externalIdentityToken(w, r)
	if !ok {
		return
	}
	var request struct {
		Issuer  string `json:"issuer"`
		Subject string `json:"subject"`
		UserID  string `json:"userId"`
	}
	if !decodeExternalIdentity(w, r, &request) {
		return
	}
	mapping, err := s.externalIdentityAdmin.CreateMapped(r.Context(), token, auth.ExternalIdentityMappingParams{
		Issuer: request.Issuer, Subject: request.Subject, UserID: request.UserID,
	})
	s.writeExternalIdentity(w, mapping, err)
}

func (s *Server) writeExternalIdentity(w http.ResponseWriter, mapping *auth.ExternalIdentityMapping, err error) {
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		status := http.StatusForbidden
		if errors.Is(err, auth.ErrExternalIdentityAlreadyMapped) {
			status = http.StatusConflict
		} else if errors.Is(err, auth.ErrExternalIdentityNotReady) {
			status = http.StatusServiceUnavailable
		}
		// Não enviar SQL, JWT ou a existência de contas locais em mensagens.
		writeJSON(w, status, map[string]string{"error": "cadastro de identidade externa recusado"})
		return
	}
	writeJSON(w, http.StatusCreated, mapping)
}
