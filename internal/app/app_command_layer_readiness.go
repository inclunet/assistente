package app

import (
	"context"
	"encoding/json"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/database"
)

// As ações de camada não têm argumentos padrão: a paleta precisa de um
// binding efetivo, configurado pelo usuário. Readiness não autoriza a execução;
// o executor volta a resolver e revalidar a regra antes do efeito.
func commandLayerPaletteReady(ctx context.Context, p *commandProductRuntime, definition commandcatalog.Definition) bool {
	if p == nil || p.app == nil || p.host == nil || !isCommandLayerAction(definition.ID) ||
		definition.Effect != commandcatalog.Write || definition.Decision != commandcatalog.NoDecision ||
		!definition.HasMutableTarget || !definition.MutatesEffectiveCapability ||
		definition.HandlerClassification != commandcatalog.HandlerBackend || !definition.AllowsSource(commandcatalog.Palette) ||
		definition.Context.None || len(definition.Context.Facts) != 1 ||
		definition.Context.Facts[0].Provider != "workspace" || definition.Context.Facts[0].Fact != "active_tab" ||
		definition.Context.Facts[0].Mode != commandcatalog.ExactVersion {
		return false
	}
	configuration, _, versions, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil || !versions.Unlocked {
		return false
	}
	identity := "palette:" + definition.ID
	required := configuration.RequiredFacts(identity)
	origin, err := p.commandOriginFacts(ctx, commandcatalog.Palette, required)
	if err != nil && len(required) > 0 {
		// Readiness is presentation only. Use the canonical workspace for the
		// closed visual palette ingress; the UI still has to supply its lease.
		snapshot, snapshotErr := p.workspaceMgr.CommandSnapshot()
		if snapshotErr != nil || snapshot.WorkspaceID != p.workspaceID {
			return false
		}
		proof := &localCommandKeyboardContextProof{snapshot: snapshot, observed: LocalCommandKeyboardContext{Profile: localKeyboardEffectiveProfile(snapshot)}}
		origin, err = workspaceVisualCommandFacts(proof, required)
	}
	if err != nil {
		return false
	}
	selected, err := configuration.Resolve(identity, origin.facts, nil)
	if err != nil || selected.Status != commandbindings.Selected || selected.CommandID != definition.ID || selected.ExecutionScopeKey != "global" {
		return false
	}
	arguments, err := definition.ValidateArguments([]byte(selected.ArgumentsKey))
	if err != nil {
		return false
	}
	var input commandLayerActionArguments
	if err := json.Unmarshal(arguments, &input); err != nil {
		return false
	}
	scope, err := p.app.commandSettingsScope(p, input.Scope)
	if err != nil {
		return false
	}
	if definition.ID == commandLayerBackID {
		return input.RuleID == "" && input.DurationSeconds == 0
	}
	action := "pin"
	if definition.ID == commandLayerToggleID {
		action = "toggle"
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		return false
	}
	snapshot, err := store.Load(ctx, scope)
	if err != nil || store.CheckCurrent(ctx, snapshot) != nil {
		return false
	}
	_, _, err = commandSettingsManualActionTarget(snapshot, scope, p.principal, input.RuleID, action, input.DurationSeconds)
	return err == nil
}
