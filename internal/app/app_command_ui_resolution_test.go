package app

import (
	"context"
	"encoding/json"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"github.com/google/uuid"
)

func paletteUICandidate(t *testing.T, selection string) commandexecution.EnvelopeCandidate {
	t.Helper()
	candidate, err := commandPaletteCandidate(
		uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String(), selection, json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("construir candidato UI da paleta: %v", err)
	}
	return candidate
}

func installPaletteUIDelta(t *testing.T, a *App, selection, reviewStatus, effect, commandID string) string {
	t.Helper()
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("produto de comandos ausente")
	}
	projection, err := commandProductProjection(p.registry, nil)
	if err != nil || len(projection.BuiltinLayers) < 1 {
		t.Fatalf("projeção builtin inválida: layers=%d err=%v", len(projection.BuiltinLayers), err)
	}
	paletteIndex := -1
	for index := range projection.BuiltinLayers {
		if projection.BuiltinLayers[index].ID == commandPaletteLayerID {
			paletteIndex = index
			break
		}
	}
	if paletteIndex < 0 {
		t.Fatalf("camada builtin de paleta ausente: %+v", projection.BuiltinLayers)
	}
	var defaultBinding *commandbindings.Default
	for index := range projection.BuiltinLayers[paletteIndex].Defaults {
		candidate := &projection.BuiltinLayers[paletteIndex].Defaults[index]
		if candidate.Candidate.CommandID == selection {
			defaultBinding = candidate
			break
		}
	}
	if defaultBinding == nil {
		t.Fatal("default de help.shortcuts.show não encontrado")
	}
	triggerSpec, err := json.Marshal(map[string]any{"version": 1, "selection": selection})
	if err != nil {
		t.Fatal(err)
	}
	row := commandconfig.Binding{
		ID: uuid.Must(uuid.NewV7()).String(), UserID: p.principal.UserID, LayerRefKind: "builtin", LayerRef: commandPaletteLayerID,
		TriggerType: string(commandcatalog.Palette), TriggerSpec: string(triggerSpec), Arguments: "{}", Condition: `{"version":1,"clauses":[]}`,
		Effect: effect, Enabled: true, Source: "user", ReviewStatus: reviewStatus, Presentation: `{"version":1}`,
	}
	if commandID != "" {
		row.CommandID = paletteStringPtr(commandID)
	}
	row.ReplacesDefaultID = paletteStringPtr(defaultBinding.Candidate.ID)
	row.ReplacesDefaultVersion = paletteStringPtr(defaultBinding.Version)
	row.ReplacesDefaultFingerprint = paletteStringPtr(defaultBinding.Fingerprint)
	if err := database.DB().Create(&row).Error; err != nil {
		t.Fatalf("inserir delta UI: %v", err)
	}
	advancePaletteConfigurationGeneration(t, a)
	return row.ID
}

func advancePaletteConfigurationGeneration(t *testing.T, a *App) {
	t.Helper()
	p := a.commandProduct.Load()
	var generation commandconfig.Generation
	if err := database.DB().Where("user_id = ? AND workspace_id IS NULL", p.principal.UserID).First(&generation).Error; err != nil {
		t.Fatalf("ler geração persistida: %v", err)
	}
	if err := database.DB().Model(&commandconfig.Generation{}).Where("id = ?", generation.ID).Update("generation", generation.Generation+1).Error; err != nil {
		t.Fatalf("avançar geração persistida: %v", err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(context.Background()); err != nil {
		t.Fatalf("rebuild da configuração persistida: %v", err)
	}
}

func removePaletteUIDelta(t *testing.T, a *App, bindingID string) {
	t.Helper()
	if err := database.DB().Delete(&commandconfig.Binding{}, "id = ?", bindingID).Error; err != nil {
		t.Fatalf("remover delta UI: %v", err)
	}
	advancePaletteConfigurationGeneration(t, a)
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestUIResolutionDefaultHelpShortcutsBeginTakeCompleteGetResult(t *testing.T) {
	a := readyCommandProduct(t)
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil || !containsString(view.LocalPaletteCommands, commandProductShortcutsShowID) {
		t.Fatalf("help.shortcuts.show não foi publicado como local_ui: %+v err=%v", view, err)
	}
	if _, err := a.BeginUICommand(commandProductShortcutsShowID); err == nil {
		t.Fatal("help.shortcuts.show entrou no handoff auditado")
	}
}

func TestUIResolutionPersistedSuppressHasNoHandoffOrInvocation(t *testing.T) {
	a := readyCommandProduct(t)
	installPaletteUIDelta(t, a, commandProductShortcutsShowID, "active", "suppress", "")
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if containsString(view.LocalPaletteCommands, commandProductShortcutsShowID) {
		t.Fatal("suppress persistido publicou comando local")
	}
	if _, err := a.BeginUICommand(commandProductShortcutsShowID); err == nil {
		t.Fatal("suppress persistido liberou handoff UI")
	}
}

func TestUIResolutionSuppressReplaySurvivesDeltaRemoval(t *testing.T) {
	a := readyCommandProduct(t)
	deltaID := installPaletteUIDelta(t, a, commandProductShortcutsShowID, "active", "suppress", "")
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil || containsString(view.LocalPaletteCommands, commandProductShortcutsShowID) {
		t.Fatalf("suppress não foi aplicado: %+v err=%v", view, err)
	}
	removePaletteUIDelta(t, a, deltaID)
	view, err = a.GetLocalCommandKeyboardMap()
	if err != nil || !containsString(view.LocalPaletteCommands, commandProductShortcutsShowID) {
		t.Fatalf("remoção do suppress não restaurou o default local: %+v err=%v", view, err)
	}
}

func TestUIResolutionNeedsReviewRefusesWithoutHandoff(t *testing.T) {
	a := readyCommandProduct(t)
	installPaletteUIDelta(t, a, commandProductShortcutsShowID, "needs_review", "suppress", "")
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil || containsString(view.LocalPaletteCommands, commandProductShortcutsShowID) {
		t.Fatalf("needs_review publicou comando local: %+v err=%v", view, err)
	}
	if _, err := a.BeginUICommand(commandProductShortcutsShowID); err == nil {
		t.Fatal("needs_review liberou handoff UI")
	}
}

func TestExecutePaletteUIAlwaysRefusesWithoutReservation(t *testing.T) {
	a := readyCommandProduct(t)
	result, err := a.ExecutePaletteCommand(commandProductShortcutsShowID, json.RawMessage(`{}`))
	if err == nil || result.Status == string(commandledger.Succeeded) {
		t.Fatalf("ExecutePaletteCommand executou UI sem reserva: result=%+v err=%v", result, err)
	}
}

func TestUIResolutionRejectsDeltaRedirectingUIToBackend(t *testing.T) {
	a := readyCommandProduct(t)
	installPaletteUIDelta(t, a, commandProductShortcutsShowID, "active", "execute", commandProductWorkspaceListID)
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil || containsString(view.LocalPaletteCommands, commandProductShortcutsShowID) {
		t.Fatalf("delta UI→backend publicou comando local: %+v err=%v", view, err)
	}
	if _, err := a.BeginUICommand(commandProductShortcutsShowID); err == nil {
		t.Fatal("delta UI→backend liberou handoff")
	}
}

func TestLocalPaletteCommandsDoNotResurrectSuppressedSelectionThroughRemap(t *testing.T) {
	a := readyCommandProduct(t)
	installPaletteUIDelta(t, a, commandProductShortcutsShowID, "active", "suppress", "")
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range view.LocalPaletteCommands {
		if id == commandProductShortcutsShowID {
			t.Fatal("seleção A suprimida apareceu na paleta local")
		}
	}
	installPaletteUIDelta(t, a, commandProductWorkspaceListID, "active", "execute", commandProductShortcutsShowID)
	view, err = a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range view.LocalPaletteCommands {
		if id == commandProductShortcutsShowID {
			t.Fatal("remap B→A ressuscitou a seleção A suprimida")
		}
	}
}

func TestCommandChatPickerLocalPaletteSuppressionAndRemapStayWithoutHandoff(t *testing.T) {
	a := readyCommandProduct(t)
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil || !containsString(view.LocalPaletteCommands, commandChatModelOpenID) {
		t.Fatalf("picker não foi publicado na paleta local: %+v err=%v", view, err)
	}
	installPaletteUIDelta(t, a, commandChatModelOpenID, "active", "suppress", "")
	view, err = a.GetLocalCommandKeyboardMap()
	if err != nil || containsString(view.LocalPaletteCommands, commandChatModelOpenID) {
		t.Fatalf("supressão do picker não foi aplicada: %+v err=%v", view, err)
	}
	installPaletteUIDelta(t, a, commandProductWorkspaceListID, "active", "execute", commandChatModelOpenID)
	view, err = a.GetLocalCommandKeyboardMap()
	if err != nil || containsString(view.LocalPaletteCommands, commandChatModelOpenID) {
		t.Fatalf("remap não ressuscitou picker suprimido: %+v err=%v", view, err)
	}
	if _, err := a.BeginUICommand(commandChatModelOpenID); err == nil {
		t.Fatal("picker local aceitou handoff auditado")
	}
}
