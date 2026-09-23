package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"github.com/google/uuid"
)

const (
	commandLayerActivateID = "layer.activate"
	commandLayerToggleID   = "layer.toggle"
	commandLayerBackID     = "layer.back"
)

type commandLayerActionArguments struct {
	Scope           string `json:"scope"`
	RuleID          string `json:"rule_id"`
	DurationSeconds int    `json:"duration_seconds"`
}

func isCommandLayerAction(commandID string) bool {
	switch commandID {
	case commandLayerActivateID, commandLayerToggleID, commandLayerBackID:
		return true
	default:
		return false
	}
}

func commandLayerActionRegistrations() []commandcatalog.Registration {
	return []commandcatalog.Registration{
		commandLayerActionRegistration(commandLayerActivateID, "Ativar camada", "Ativa uma camada manual", commandLayerActionSchema(false), commandLayerResultSchema()),
		commandLayerActionRegistration(commandLayerToggleID, "Alternar camada", "Alterna uma camada manual", commandLayerActionSchema(false), commandLayerResultSchema()),
		commandLayerActionRegistration(commandLayerBackID, "Voltar camada", "Encerra a reivindicação manual mais recente", commandLayerActionSchema(true), commandLayerResultSchema()),
	}
}

func commandLayerActionRegistration(id, name, description string, arguments, result *commandcatalog.Schema) commandcatalog.Registration {
	locales := map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: name, Description: description, Category: "Camadas"},
		"en":    {Name: map[string]string{commandLayerActivateID: "Activate layer", commandLayerToggleID: "Toggle layer", commandLayerBackID: "Back layer"}[id], Description: map[string]string{commandLayerActivateID: "Activates a manual layer", commandLayerToggleID: "Toggles a manual layer", commandLayerBackID: "Ends the latest manual claim"}[id], Category: "Layers"},
		"es":    {Name: map[string]string{commandLayerActivateID: "Activar capa", commandLayerToggleID: "Alternar capa", commandLayerBackID: "Volver de capa"}[id], Description: map[string]string{commandLayerActivateID: "Activa una capa manual", commandLayerToggleID: "Alterna una capa manual", commandLayerBackID: "Finaliza la última reclamación manual"}[id], Category: "Capas"},
	}
	definition := commandcatalog.Definition{
		ID: id, Effect: commandcatalog.Write, Decision: commandcatalog.NoDecision,
		MutatesEffectiveCapability: true, HasMutableTarget: true,
		AllowedSources:  []commandcatalog.Source{commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck, commandcatalog.Chat},
		Context:         commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}},
		Presentation:    &commandcatalog.Presentation{Version: "layer-action-v1", Locales: locales},
		ArgumentsSchema: arguments, ResultSchema: result, Risk: commandcatalog.RiskMedium,
		Persistence:  commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceRedacted},
		Scopes:       []commandcatalog.Scope{commandcatalog.ScopeSession, commandcatalog.ScopeWorkspace, commandcatalog.ScopeGlobal},
		Availability: commandcatalog.Availability{Status: commandcatalog.Available},
		HandlerRoute: "internal/layer/action", HandlerClassification: commandcatalog.HandlerBackend,
	}
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Write, HasMutableTarget: true, MutatesEffectiveCapability: true, Route: definition.HandlerRoute, Classification: commandcatalog.HandlerBackend}
	return commandcatalog.Registration{Definition: definition, Handler: contract}
}

func commandLayerActionSchema(back bool) *commandcatalog.Schema {
	properties := map[string]commandcatalog.Schema{
		"scope":            {Type: commandcatalog.SchemaString, Enum: []any{"global", "workspace"}},
		"rule_id":          {Type: commandcatalog.SchemaString},
		"duration_seconds": {Type: commandcatalog.SchemaInteger, Minimum: floatPtr(0), Maximum: floatPtr(86400)},
	}
	if back {
		properties["rule_id"] = commandcatalog.Schema{Type: commandcatalog.SchemaString}
	}
	return &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: properties, Required: []string{"scope", "rule_id", "duration_seconds"}}
}

func commandLayerResultSchema() *commandcatalog.Schema {
	return &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{
		"committed": {Type: commandcatalog.SchemaBoolean}, "published": {Type: commandcatalog.SchemaBoolean}, "id": {Type: commandcatalog.SchemaString},
	}, Required: []string{"committed", "published", "id"}}
}

func floatPtr(value float64) *float64 { return &value }

func (a *App) startCommandLayerAction(ctx context.Context, invocation commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
	if a == nil || ctx == nil || invocation.Envelope == nil {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrDenied
	}
	arguments := json.RawMessage(`{}`)
	if invocation.Envelope.Arguments != nil {
		arguments = append(arguments[:0], (*invocation.Envelope.Arguments)...)
	}
	var input commandLayerActionArguments
	if err := json.Unmarshal(arguments, &input); err != nil {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrInvalidRequest
	}
	if strings.TrimSpace(input.Scope) == "" || (input.Scope != string(CommandSettingsScopeGlobal) && input.Scope != string(CommandSettingsScopeWorkspace)) {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrInvalidRequest
	}
	if invocation.CommandID == commandLayerBackID {
		if input.RuleID != "" || input.DurationSeconds != 0 {
			return commandexecution.ExecutionHandle{}, commandexecution.ErrInvalidRequest
		}
	} else if input.RuleID == "" {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrInvalidRequest
	}
	origin, err := commandLayerOrigin(invocation)
	if err != nil {
		return commandexecution.ExecutionHandle{}, err
	}
	action := map[string]string{commandLayerActivateID: "pin", commandLayerToggleID: "toggle", commandLayerBackID: "back"}[invocation.CommandID]
	if action == "" || !isCommandLayerAction(invocation.CommandID) {
		return commandexecution.ExecutionHandle{}, commandexecution.ErrInvalidRequest
	}
	id, err := uuid.NewV7()
	if err != nil {
		return commandexecution.ExecutionHandle{}, err
	}
	ownership, err := commandexecution.NewCommitOwnership(35 * time.Second)
	if err != nil {
		return commandexecution.ExecutionHandle{}, err
	}
	done := make(chan commandexecution.Outcome, 1)
	handoff := newCommandLayerMutationHandoff(ctx)
	var cancelOnce sync.Once
	go func() {
		defer handoff.Dispose()
		result, callErr := a.applyCommandLayerActionWithOrigin(handoff.Context(), invocation.Principal, input.Scope, input.RuleID, action, input.DurationSeconds, origin, ownership, handoff)
		if callErr != nil {
			if ctx.Err() != nil && !result.Committed {
				done <- commandexecution.Outcome{Status: commandledger.Cancelled}
				return
			}
			done <- commandexecution.Outcome{Status: commandledger.Failed}
			return
		}
		if !result.Committed {
			done <- commandexecution.Outcome{Status: commandledger.Failed}
			return
		}
		if !result.Published {
			done <- commandexecution.Outcome{Status: commandledger.OutcomeUnknown}
			return
		}
		// Resultado interno validado pelo executor; a fachada não o expõe
		// como efeito visual, e a política do catálogo não o persiste.
		payload, marshalErr := json.Marshal(struct {
			Committed bool   `json:"committed"`
			Published bool   `json:"published"`
			ID        string `json:"id"`
		}{Committed: result.Committed, Published: result.Published, ID: result.ID})
		if marshalErr != nil {
			done <- commandexecution.Outcome{Status: commandledger.Failed}
			return
		}
		done <- commandexecution.Outcome{Status: commandledger.Succeeded, Result: payload}
	}()
	return commandexecution.ExecutionHandle{ID: id.String(), Done: done, Cancel: func() { cancelOnce.Do(handoff.Cancel) }, CommitOwnership: ownership}, nil
}

func commandLayerOrigin(invocation commandexecution.Invocation) (commandactivation.Origin, error) {
	if invocation.Principal.SessionID == "" {
		return commandactivation.Origin{}, commandexecution.ErrDenied
	}
	var device string
	if invocation.Envelope != nil && invocation.Envelope.SourceInstanceID != nil {
		device = strings.TrimSpace(*invocation.Envelope.SourceInstanceID)
	}
	switch invocation.Source {
	case commandcatalog.Chat:
		if invocation.Envelope == nil || invocation.Envelope.ActorType != "agent" || invocation.Envelope.AuthorizationDecisionID == nil || invocation.Envelope.ConversationID == nil || strings.TrimSpace(*invocation.Envelope.AuthorizationDecisionID) == "" || strings.TrimSpace(*invocation.Envelope.ConversationID) == "" {
			return commandactivation.Origin{}, commandexecution.ErrDenied
		}
		device = *invocation.Envelope.ConversationID
	case commandcatalog.Palette:
		if device == "" {
			device = "palette"
		}
	case commandcatalog.KeyboardLocal:
		// SourceInstanceID is a keyboard-map generation, not a physical
		// identity. The local keyboard is one stable device per session;
		// the session already participates in ManualStackKey.
		device = "local-keyboard"
	case commandcatalog.StreamDeck:
		// O envelope já foi vinculado à ocorrência física pelo adapter.
		// A instância da conexão muda ao reconectar; a pilha pertence ao
		// dispositivo, compartilhada entre suas teclas.
		if invocation.Envelope == nil || invocation.Envelope.TriggerSpec == nil {
			return commandactivation.Origin{}, commandexecution.ErrDenied
		}
		identity, normalizeErr := (commandconfig.StreamDeckTriggerPort{}).Normalize(context.Background(), *invocation.Envelope.TriggerSpec)
		if normalizeErr != nil {
			return commandactivation.Origin{}, commandexecution.ErrDenied
		}
		spec, parseErr := commandconfig.ParseStreamDeckTriggerIdentity(identity)
		if parseErr != nil || strings.TrimSpace(spec.Device) == "" {
			return commandactivation.Origin{}, commandexecution.ErrDenied
		}
		device = spec.Device
	default:
		return commandactivation.Origin{}, commandexecution.ErrDenied
	}
	return commandactivation.Origin{Type: string(invocation.Source), SessionID: invocation.Principal.SessionID, DeviceID: device}, nil
}
