package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"assistente/internal/auth"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/logging"
)

type externalCommandResponse struct {
	InvocationID  string          `json:"invocationId"`
	Status        string          `json:"status"`
	ResultSummary *string         `json:"resultSummary"`
	Result        json.RawMessage `json:"result,omitempty"`
}

func (s *Server) externalCommandService(w http.ResponseWriter, r *http.Request) (*commandexecution.ExternalService, bool) {
	w.Header().Set("Cache-Control", "no-store")
	if s.mode != "external" {
		http.NotFound(w, r)
		return nil, false
	}
	source := r.PathValue("source")
	switch source {
	case "palette", "ui", "chat":
	default:
		http.NotFound(w, r)
		return nil, false
	}
	service := s.externalCommands[source]
	if service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "execução externa indisponível"})
		return nil, false
	}
	return service, true
}

func externalCommandToken(w http.ResponseWriter, r *http.Request) (string, bool) {
	token, ok := strictBearerToken(r)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "credenciais inválidas"})
		return "", false
	}
	return token, true
}

func (s *Server) handleExternalCommandExecute(w http.ResponseWriter, r *http.Request) {
	service, ok := s.externalCommandService(w, r)
	if !ok {
		return
	}
	token, ok := externalCommandToken(w, r)
	if !ok {
		return
	}
	var candidate commandexecution.EnvelopeCandidate
	if !decodeExternalIdentity(w, r, &candidate) {
		return
	}
	record, result, err := service.ExecuteEnvelopeWithResult(r.Context(), token, candidate)
	if err != nil {
		writeExternalCommandError(r.Context(), w, err)
		return
	}
	writeJSON(w, http.StatusOK, externalCommandResponse{
		InvocationID:  record.InvocationID,
		Status:        string(record.Status),
		ResultSummary: record.ResultSummary,
		Result:        result,
	})
}

func (s *Server) handleExternalCommandLookup(w http.ResponseWriter, r *http.Request) {
	service, ok := s.externalCommandService(w, r)
	if !ok {
		return
	}
	token, ok := externalCommandToken(w, r)
	if !ok {
		return
	}
	record, err := service.GetEnvelopeInvocation(r.Context(), token, r.PathValue("id"))
	if err != nil {
		writeExternalCommandError(r.Context(), w, err)
		return
	}
	writeJSON(w, http.StatusOK, externalCommandResponse{
		InvocationID:  record.InvocationID,
		Status:        string(record.Status),
		ResultSummary: record.ResultSummary,
	})
}

func (s *Server) handleExternalIdentityRevoke(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if s.mode != "external" {
		http.NotFound(w, r)
		return
	}
	// A revogação é executada pelo adapter externo para atravessar o mesmo
	// EpochService usado pelos serviços por origem.
	service := s.externalCommands["palette"]
	if service == nil {
		service = s.externalCommands["ui"]
	}
	if service == nil {
		service = s.externalCommands["chat"]
	}
	if service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "revogação externa indisponível"})
		return
	}
	token, ok := externalCommandToken(w, r)
	if !ok {
		return
	}
	var request struct {
		Issuer  string `json:"issuer"`
		Subject string `json:"subject"`
	}
	if !decodeExternalIdentity(w, r, &request) {
		return
	}
	if err := service.RevokeIdentity(r.Context(), token, request.Issuer, request.Subject); err != nil {
		switch {
		case errors.Is(err, auth.ErrExternalAdministratorRequired), errors.Is(err, auth.ErrExternalAdminScopeRequired), errors.Is(err, auth.ErrExternalIdentityNotMapped), errors.Is(err, auth.ErrExternalIdentityRevoked):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "revogação externa recusada"})
		case errors.Is(err, auth.ErrExternalIdentityNotReady):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "revogação externa indisponível"})
		default:
			logging.Errorf(r.Context(), "httpapi.external-commands", "[httpapi] external_identity_revoke_failed")
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "falha na operação externa"})
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func writeExternalCommandError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, commandexecution.ErrInvalidRequest):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "solicitação inválida"})
	case errors.Is(err, commandledger.ErrConflict):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "invocação conflitante"})
	case errors.Is(err, commandledger.ErrNotFound), errors.Is(err, commandexecution.ErrDenied):
		// Ausência e falta de ownership são indistinguíveis.
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "invocação não encontrada"})
	case errors.Is(err, commandexecution.ErrStale):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "contexto de execução obsoleto"})
	default:
		// Auth, banco e executor podem incluir detalhes sensíveis; não os devolva.
		logging.Errorf(ctx, "httpapi.external-commands", "[httpapi] external_command_operation_failed")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "falha na operação externa"})
	}
}
