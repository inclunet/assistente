package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"assistente/internal/commandjobactivation"
	"assistente/internal/commandjobevents"
	"assistente/internal/database"
	"gorm.io/gorm"
)

const commandRuntimeIdentityProvenanceKey = "command_runtime_identity"

var ErrInvalidCommandRuntimeIdentity = errors.New("identidade de runtime de comando inválida")

type commandRuntimeIdentityProvenance struct {
	Generation         string `json:"generation"`
	UserID             string `json:"user_id"`
	AuthContextType    string `json:"auth_context_type"`
	AuthContextID      string `json:"auth_context_id"`
	AuthGeneration     string `json:"auth_generation"`
	SecurityGeneration string `json:"security_generation"`
}

// CommandRuntimeIdentityProvenance retorna o fragmento de proveniência que um
// adapter autenticado deve anexar ao TriggerContext de jobs iniciados pelo
// runtime de comandos. O executor preserva esse fragmento sem payload.
func CommandRuntimeIdentityProvenance(identity commandjobactivation.RuntimeIdentity) (map[string]any, error) {
	proof := commandRuntimeIdentityProvenance{
		Generation: identity.Generation, UserID: identity.UserID, AuthContextType: identity.AuthContextType,
		AuthContextID: identity.AuthContextID, AuthGeneration: identity.AuthGeneration, SecurityGeneration: identity.SecurityGeneration,
	}
	if !validCommandRuntimeIdentity(proof, identity.UserID) {
		return nil, ErrInvalidCommandRuntimeIdentity
	}
	return map[string]any{commandRuntimeIdentityProvenanceKey: proof}, nil
}

// CommandRuntimeIdentityFromFact prova que a ocorrência de job ainda pertence
// a um run autenticado não terminal. A autoridade vem da linha persistida de
// job_runs e da proveniência estrutural gravada pelo runtime de comandos; o
// payload da outbox/fact não consegue suprir campos ausentes.
func CommandRuntimeIdentityFromFact(ctx context.Context, tx *gorm.DB, fact commandjobevents.Fact) (commandjobactivation.RuntimeIdentity, error) {
	if ctx == nil || tx == nil || strings.TrimSpace(fact.UserID) == "" || strings.TrimSpace(fact.JobDatabaseID) == "" || strings.TrimSpace(fact.RunID) == "" {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return commandjobactivation.RuntimeIdentity{}, err
	}
	wantStatus, ok := runtimeStatusForFactState(fact.State)
	if !ok {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	var row database.JobRun
	if err := tx.WithContext(ctx).
		Where("id = ? AND user_id = ? AND job_id = ? AND root_origin_type = ? AND root_origin_id = ? AND status = ?",
			fact.RunID, fact.UserID, fact.JobDatabaseID, fact.RootOriginType, fact.RootOriginID, wantStatus).
		Take(&row).Error; err != nil {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	if strings.TrimSpace(row.Provenance) == "" {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	var provenance map[string]json.RawMessage
	if err := json.Unmarshal([]byte(row.Provenance), &provenance); err != nil {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	raw, ok := provenance[commandRuntimeIdentityProvenanceKey]
	if !ok || len(raw) == 0 {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	var identity commandRuntimeIdentityProvenance
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&identity); err != nil {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	if !validCommandRuntimeIdentity(identity, fact.UserID) {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	return commandjobactivation.RuntimeIdentity{
		Generation: identity.Generation, UserID: identity.UserID, AuthContextType: identity.AuthContextType,
		AuthContextID: identity.AuthContextID, AuthGeneration: identity.AuthGeneration, SecurityGeneration: identity.SecurityGeneration,
	}, nil
}

func runtimeStatusForFactState(state string) (string, bool) {
	switch state {
	case commandjobevents.StateQueued:
		return RunStatusQueued, true
	case commandjobevents.StateStarted:
		return RunStatusRunning, true
	case commandjobevents.StateRetryScheduled:
		return RunStatusRetrying, true
	default:
		return "", false
	}
}

func validCommandRuntimeIdentity(identity commandRuntimeIdentityProvenance, userID string) bool {
	values := []string{identity.Generation, identity.UserID, identity.AuthContextType, identity.AuthContextID, identity.AuthGeneration, identity.SecurityGeneration}
	for _, value := range values {
		if value == "" || strings.TrimSpace(value) != value || strings.ContainsRune(value, '\x00') {
			return false
		}
	}
	return identity.UserID == userID && (identity.AuthContextType == "local_session" || identity.AuthContextType == "system")
}
