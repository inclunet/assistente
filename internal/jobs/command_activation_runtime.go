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

type commandRuntimeEntry struct {
	identity commandjobactivation.RuntimeIdentity
	watchCtx context.Context
}

type commandRuntimeIdentityProvenance struct {
	Generation         string `json:"generation"`
	UserID             string `json:"user_id"`
	AuthContextType    string `json:"auth_context_type"`
	AuthContextID      string `json:"auth_context_id"`
	AuthGeneration     string `json:"auth_generation"`
	SecurityGeneration string `json:"security_generation"`
}

// CommandRuntimeIdentityProvenance serializa o fragmento reservado de
// proveniência do runtime de comandos. Quando configurado, o executor gera a
// Generation privada e sobrescreve essa chave; ela nunca é aceita do payload.
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

// CommandRuntimeIdentityFromFact lê apenas a identidade estrutural persistida.
// Ela não prova runtime vivo; use Manager.CommandRuntimeIdentity para isso.
func CommandRuntimeIdentityFromFact(ctx context.Context, tx *gorm.DB, fact commandjobevents.Fact) (commandjobactivation.RuntimeIdentity, error) {
	if ctx == nil || tx == nil || strings.TrimSpace(fact.UserID) == "" || strings.TrimSpace(fact.JobDatabaseID) == "" || strings.TrimSpace(fact.RunID) == "" {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return commandjobactivation.RuntimeIdentity{}, err
	}
	if _, ok := runtimeStatusForFactState(fact.State); !ok {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	return CommandPersistedIdentityFromFact(ctx, tx, fact)
}

// CommandPersistedIdentityFromFact valida a identidade estrutural persistida
// para qualquer estado conhecido, inclusive terminal. Não prova que o run
// continua vivo; essa prova pertence a Manager.CommandRuntimeIdentity.
func CommandPersistedIdentityFromFact(ctx context.Context, tx *gorm.DB, fact commandjobevents.Fact) (commandjobactivation.RuntimeIdentity, error) {
	if ctx == nil || tx == nil || strings.TrimSpace(fact.UserID) == "" || strings.TrimSpace(fact.JobDatabaseID) == "" || strings.TrimSpace(fact.RunID) == "" {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	if err := ctx.Err(); err != nil {
		return commandjobactivation.RuntimeIdentity{}, err
	}
	if _, ok := runtimeStatusForFactState(fact.State); !ok {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	var row database.JobRun
	if err := tx.WithContext(ctx).
		Where("id = ? AND user_id = ? AND job_id = ? AND root_origin_type = ? AND root_origin_id = ?",
			fact.RunID, fact.UserID, fact.JobDatabaseID, fact.RootOriginType, fact.RootOriginID).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
		}
		return commandjobactivation.RuntimeIdentity{}, err
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
	case commandjobevents.StateCompleted:
		return RunStatusCompleted, true
	case commandjobevents.StateFailed:
		return RunStatusFailed, true
	case commandjobevents.StateSkipped:
		return RunStatusSkipped, true
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

// CommandRuntimeIdentity prova, simultaneamente, a linha persistida e a
// entrada viva deste Manager. O watchCtx é consultado sob o mutex próprio do
// tracker e nunca é usado como contexto da execução da tool.
func (m *Manager) CommandRuntimeIdentity(ctx context.Context, tx *gorm.DB, fact commandjobevents.Fact) (commandjobactivation.RuntimeIdentity, error) {
	if m == nil {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	identity, err := CommandPersistedIdentityFromFact(ctx, tx, fact)
	if err != nil {
		return commandjobactivation.RuntimeIdentity{}, err
	}
	var row database.JobRun
	if err := tx.WithContext(ctx).
		Select("status").
		Where("id = ? AND user_id = ? AND job_id = ? AND root_origin_type = ? AND root_origin_id = ?",
			fact.RunID, fact.UserID, fact.JobDatabaseID, fact.RootOriginType, fact.RootOriginID).
		Take(&row).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
		}
		return commandjobactivation.RuntimeIdentity{}, err
	}
	if row.Status == RunStatusCompleted || row.Status == RunStatusFailed || row.Status == RunStatusSkipped {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	m.commandRuntimeMu.Lock()
	defer m.commandRuntimeMu.Unlock()
	entry, ok := m.commandRuntime[fact.RunID]
	if !ok || entry.watchCtx == nil || entry.watchCtx.Err() != nil || entry.identity != identity {
		return commandjobactivation.RuntimeIdentity{}, ErrCommandMaintenanceUnavailable
	}
	return identity, nil
}

func (m *Manager) enableCommandRuntimeTracking() {
	if m == nil || m.cfg.CommandRuntimeIdentity == nil {
		return
	}
	m.commandRuntimeMu.Lock()
	defer m.commandRuntimeMu.Unlock()
	if m.commandRuntime == nil {
		m.commandRuntime = make(map[string]commandRuntimeEntry)
	}
	m.commandRuntimeAccepting = true
}

func (m *Manager) invalidateCommandRuntimeTracking() {
	if m == nil {
		return
	}
	m.commandRuntimeMu.Lock()
	defer m.commandRuntimeMu.Unlock()
	m.commandRuntimeAccepting = false
	m.commandRuntimeToken++
	clear(m.commandRuntime)
}

func (m *Manager) commandRuntimeTokenValue() uint64 {
	if m == nil {
		return 0
	}
	m.commandRuntimeMu.Lock()
	defer m.commandRuntimeMu.Unlock()
	return m.commandRuntimeToken
}

func (m *Manager) registerCommandRuntime(runID string, identity commandjobactivation.RuntimeIdentity, watchCtx context.Context, token uint64) bool {
	if m == nil || runID == "" || watchCtx == nil {
		return false
	}
	m.commandRuntimeMu.Lock()
	defer m.commandRuntimeMu.Unlock()
	if !m.commandRuntimeAccepting || token != m.commandRuntimeToken || watchCtx.Err() != nil {
		return false
	}
	if m.commandRuntime == nil {
		m.commandRuntime = make(map[string]commandRuntimeEntry)
	}
	m.commandRuntime[runID] = commandRuntimeEntry{identity: identity, watchCtx: watchCtx}
	return true
}

func (m *Manager) unregisterCommandRuntime(runID string) {
	if m == nil {
		return
	}
	m.commandRuntimeMu.Lock()
	delete(m.commandRuntime, runID)
	m.commandRuntimeMu.Unlock()
}
