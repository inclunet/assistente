package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"

	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandexecution"
)

// commandDeckPersistentState reads only immutable metadata from the exact
// configuration snapshot published with activeIDs and versions. Empty means
// the target or its effective state cannot be proven for this snapshot.
func (p *commandProductRuntime) commandDeckPersistentState(ctx context.Context, resolved commandbindings.Result, activeIDs []string, versions commandexecution.Versions) string {
	if p == nil || ctx == nil || p.host == nil || (resolved.CommandID != commandLayerActivateID && resolved.CommandID != commandLayerToggleID) || !versions.Unlocked {
		return ""
	}
	var input commandLayerActionArguments
	decoder := json.NewDecoder(bytes.NewBufferString(resolved.ArgumentsKey))
	decoder.DisallowUnknownFields()
	if strings.TrimSpace(resolved.ArgumentsKey) == "" || decoder.Decode(&input) != nil || decoder.Decode(&struct{}{}) != io.EOF ||
		(input.Scope != string(CommandSettingsScopeGlobal) && input.Scope != string(CommandSettingsScopeWorkspace)) || strings.TrimSpace(input.RuleID) == "" || strings.TrimSpace(input.RuleID) != input.RuleID {
		return ""
	}
	if !p.dependenciesMatch(p.app) || p.app.commandProduct.Load() != p {
		return ""
	}
	configuration, publishedActive, current, err := p.host.ResolutionSnapshot(ctx, p.principal)
	if err != nil || configuration == nil || current != versions || !current.Unlocked || !slices.Equal(publishedActive, activeIDs) {
		return ""
	}
	target, ok := configuration.LayerPresentationTarget(input.RuleID)
	if !ok || target.LayerID == "" || target.Scope != input.Scope || !target.RuleEnabled || target.RuleSource != "user" || target.ReviewStatus != "active" ||
		(target.RuleMode != string(commandactivation.ModeManual) && target.RuleMode != string(commandactivation.ModeToggle)) || target.RuleCondition != `{}` {
		return ""
	}
	state := commandDeckEffectiveLayerState(target, publishedActive)
	if !p.commandDeckPresentationSnapshotCurrent(ctx, configuration, activeIDs, versions) {
		return ""
	}
	return state
}

func commandDeckEffectiveLayerState(target commandbindings.LayerPresentationState, activeIDs []string) string {
	if !target.LayerEnabled {
		return "off"
	}
	if target.AlwaysActive || slices.Contains(activeIDs, target.LayerID) {
		return "on"
	}
	if target.Contextual {
		return ""
	}
	return "off"
}

func (p *commandProductRuntime) commandDeckPresentationSnapshotCurrent(ctx context.Context, expectedConfiguration *commandbindings.Configuration, activeIDs []string, versions commandexecution.Versions) bool {
	if p == nil || ctx == nil || ctx.Err() != nil || p.host == nil || !p.dependenciesMatch(p.app) || p.app.commandProduct.Load() != p {
		return false
	}
	configuration, currentActive, current, err := p.host.ResolutionSnapshot(ctx, p.principal)
	return err == nil && configuration == expectedConfiguration && current == versions && current.Unlocked && slices.Equal(currentActive, activeIDs)
}
