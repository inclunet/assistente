package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandexecution"
	"assistente/internal/commandsecurity"
	"assistente/internal/logging"

	"github.com/google/uuid"
)

// ExternalUIConnectionPort administra somente o grant de roteamento até a UI
// local. A política JWT por comando continua sendo aplicada pelo executor.
type ExternalUIConnectionPort interface {
	ConsumeInvitation(context.Context, string, auth.ExternalCommandPrincipal) (ExternalUIConnectionContext, error)
	ReadConnection(context.Context, auth.ExternalCommandPrincipal, string, string) (ExternalUIConnectionContext, error)
	RevokePrincipal(context.Context, string, string)
}

type ExternalUIConnectionContext struct {
	ConnectionID     string    `json:"connectionId"`
	Generation       string    `json:"generation"`
	TargetSnapshotID string    `json:"targetSnapshotId"`
	ContextVersion   string    `json:"contextVersion"`
	ExpiresAt        time.Time `json:"expiresAt,omitempty"`
}

var (
	ErrExternalUIConnectionUnavailable = errors.New("conexão de interface externa indisponível")
	ErrExternalUIConnectionDenied      = errors.New("conexão de interface externa recusada")
)

func (s *Server) externalUIService(w http.ResponseWriter, r *http.Request) (*commandexecution.ExternalService, bool) {
	w.Header().Set("Cache-Control", "no-store")
	if s.mode != "external" {
		http.NotFound(w, r)
		return nil, false
	}
	service := s.externalCommandServiceFor("ui")
	if service == nil || s.externalUIConnections == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "conexão externa indisponível"})
		return nil, false
	}
	return service, true
}

func (s *Server) handleExternalUIConnectionConsume(w http.ResponseWriter, r *http.Request) {
	service, ok := s.externalUIService(w, r)
	if !ok {
		return
	}
	token, ok := externalCommandToken(w, r)
	if !ok {
		return
	}
	var request struct {
		Invitation string `json:"invitation"`
	}
	if !decodeExternalIdentity(w, r, &request) {
		return
	}
	if strings.TrimSpace(request.Invitation) == "" || request.Invitation != strings.TrimSpace(request.Invitation) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "convite inválido"})
		return
	}
	var connection ExternalUIConnectionContext
	err := service.WithAuthenticatedPrincipal(r.Context(), token, func(ctx context.Context, principal auth.ExternalCommandPrincipal) error {
		var consumeErr error
		connection, consumeErr = s.externalUIConnections.ConsumeInvitation(ctx, request.Invitation, principal)
		return consumeErr
	})
	if err != nil {
		writeExternalUIConnectionError(r.Context(), w, err)
		return
	}
	writeJSON(w, http.StatusOK, connection)
}

func (s *Server) handleExternalUIConnectionContext(w http.ResponseWriter, r *http.Request) {
	service, ok := s.externalUIService(w, r)
	if !ok {
		return
	}
	token, ok := externalCommandToken(w, r)
	if !ok {
		return
	}
	connectionID, generation, valid := parseExternalUIContextRawQuery(r.URL.RawQuery)
	if !valid {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "conexão inválida"})
		return
	}
	var connection ExternalUIConnectionContext
	err := service.WithAuthenticatedPrincipal(r.Context(), token, func(ctx context.Context, principal auth.ExternalCommandPrincipal) error {
		var readErr error
		connection, readErr = s.externalUIConnections.ReadConnection(ctx, principal, connectionID, generation)
		return readErr
	})
	if err != nil {
		writeExternalUIConnectionError(r.Context(), w, err)
		return
	}
	writeJSON(w, http.StatusOK, connection)
}

func parseExternalUIContextRawQuery(raw string) (connectionID, generation string, ok bool) {
	if len(raw) > 128 {
		return "", "", false
	}
	query, err := url.ParseQuery(raw)
	if err != nil {
		return "", "", false
	}
	return parseExternalUIContextQuery(query)
}

func parseExternalUIContextQuery(query url.Values) (connectionID, generation string, ok bool) {
	if len(query) != 2 || len(query["connectionId"]) != 1 || len(query["generation"]) != 1 {
		return "", "", false
	}
	connectionID, generation = query.Get("connectionId"), query.Get("generation")
	parsedID, err := uuid.Parse(connectionID)
	if err != nil || parsedID.Version() != 7 || parsedID.Variant() != uuid.RFC4122 || parsedID.String() != connectionID {
		return "", "", false
	}
	parsedGeneration, err := strconv.ParseUint(generation, 10, 64)
	if err != nil || parsedGeneration == 0 || strconv.FormatUint(parsedGeneration, 10) != generation {
		return "", "", false
	}
	return connectionID, generation, true
}

func writeExternalUIConnectionError(ctx context.Context, w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, commandexecution.ErrDenied), errors.Is(err, ErrExternalUIConnectionDenied):
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "conexão não encontrada"})
	case errors.Is(err, commandexecution.ErrInvalidRequest):
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "solicitação inválida"})
	case errors.Is(err, commandexecution.ErrStale):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "contexto obsoleto"})
	case errors.Is(err, commandsecurity.ErrStaleEpoch):
		writeJSON(w, http.StatusConflict, map[string]string{"error": "identidade externa obsoleta"})
	case errors.Is(err, ErrExternalUIConnectionUnavailable):
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "conexão externa indisponível"})
	default:
		logging.Errorf(ctx, "httpapi.external-ui-connections", "[httpapi] external_ui_connection_operation_failed")
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "falha na operação externa"})
	}
}
