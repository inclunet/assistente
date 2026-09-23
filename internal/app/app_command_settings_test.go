package app

import (
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/database"
)

func TestCommandSettingsCRUDPersisteSomenteEstadoSeguro(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	mutate := func(call func() (CommandSettingsMutation, error)) CommandSettingsMutation {
		done := settingsSecurityStart(t, a, call)
		appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
		outcome := settingsSecurityFinish(t, done)
		if outcome.err != nil || !outcome.result.Committed || !outcome.result.Published || outcome.result.ID == "" {
			t.Fatalf("mutação não persistiu/publicou: %+v", outcome)
		}
		return outcome.result
	}
	layerID := mutate(func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{Name: "Atalhos de teste", Description: "camada CRUD", Enabled: true})
	}).ID

	snapshot, err := a.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	var layer CommandSettingsLayer
	for _, item := range snapshot.Layers {
		if item.ID == layerID {
			layer = item
		}
	}
	if layer.ID == "" {
		t.Fatalf("camada criada não apareceu no snapshot: %s", layerID)
	}
	mutate(func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{ID: layerID, Name: "Atalhos editados", Description: layer.Description, Enabled: true})
	})
	mutate(func() (CommandSettingsMutation, error) {
		return a.SaveCommandLayer(CommandLayerEdit{ID: layerID, Name: "Atalhos editados", Description: layer.Description, Enabled: false})
	})
	bindingID := mutate(func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{LayerID: layerID, CommandID: commandProductWorkspaceListID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"workspace.list"}`, Enabled: true})
	}).ID
	mutate(func() (CommandSettingsMutation, error) {
		return a.SaveCommandBinding(CommandBindingEdit{ID: bindingID, LayerID: layerID, CommandID: commandProductWorkspaceListID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"workspace.list"}`, Enabled: false})
	})
	mutate(func() (CommandSettingsMutation, error) { return a.DeleteCommandBinding(bindingID) })

	final, err := a.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range final.Layers {
		if item.ID == layerID && (item.Name != "Atalhos editados" || item.Enabled) {
			t.Fatalf("estado final da camada incorreto: %+v", item)
		}
	}
	for _, item := range final.Bindings {
		if item.ID == bindingID {
			t.Fatalf("binding excluído ainda aparece: %+v", item)
		}
		var trigger any
		if err := json.Unmarshal([]byte(item.TriggerSpec), &trigger); err != nil {
			t.Fatalf("trigger inválido: %v", err)
		}
		if commandSettingsTestHasTokenField(trigger) {
			t.Fatal("snapshot expôs argumento sensível")
		}
	}
}

// Command IDs may contain "tokens"; credential fields must never be projected.
func commandSettingsTestHasTokenField(value any) bool {
	switch node := value.(type) {
	case map[string]any:
		for key, child := range node {
			if strings.Contains(strings.ToLower(key), "token") || commandSettingsTestHasTokenField(child) {
				return true
			}
		}
	case []any:
		for _, child := range node {
			if commandSettingsTestHasTokenField(child) {
				return true
			}
		}
	}
	return false
}

func TestCommandSettingsTokenFieldCheckDistinguishesCommandID(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		sensitive bool
	}{
		{`{"version":1,"selection":"chat.tokens.open"}`, false},
		{`{"version":1,"token":"secret"}`, true},
		{`{"arguments":{"token":"secret"}}`, true},
		{`{"arguments":[{"token":"secret"}]}`, true},
		{`{"arguments":{"access_token":"secret"}}`, true},
		{`{"arguments":[{"RefreshToken":"secret"}]}`, true},
	} {
		var value any
		if err := json.Unmarshal([]byte(tc.raw), &value); err != nil {
			t.Fatal(err)
		}
		if commandSettingsTestHasTokenField(value) != tc.sensitive {
			t.Fatalf("incorrect sensitive-field detection: %s", tc.raw)
		}
	}
}

func TestValidateCommandBindingEditAceitaSomenteContratoEstrito(t *testing.T) {
	valid := CommandBindingEdit{LayerID: "layer", CommandID: "command.id", TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"command.id"}`}
	if err := validateCommandBindingEdit(valid); err != nil {
		t.Fatalf("pedido válido recusado: %v", err)
	}
	for name, request := range map[string]CommandBindingEdit{
		"origem não permitida": validWithTrigger(valid, "keyboard.global"),
		"json inválido":        validWithTrigger(valid, "{"),
		"espaço externo":       validWithTrigger(valid, ` {"version":1,"selection":"command.id"}`),
		"camada ausente":       func() CommandBindingEdit { v := valid; v.LayerID = ""; return v }(),
		"comando ausente":      func() CommandBindingEdit { v := valid; v.CommandID = ""; return v }(),
	} {
		if err := validateCommandBindingEdit(request); err == nil {
			t.Errorf("%s foi aceito", name)
		}
	}
}

func TestValidateCommandBindingEditAceitaTriggerStreamDeckVersionado(t *testing.T) {
	req := CommandBindingEdit{
		LayerID: "layer", CommandID: "command.id", TriggerType: "streamdeck.key",
		TriggerSpec: `{"version":1,"device":"SERIAL","key":0}`,
	}
	if err := validateCommandBindingEdit(req); err != nil {
		t.Fatalf("binding Stream Deck válido recusado: %v", err)
	}
}

func TestCommandSettingsBindingReadOnlyNaoExponeCamposComplexos(t *testing.T) {
	row := commandconfig.Binding{Condition: `{"version":1,"clauses":[{"field":"private","op":"eq","value":"secret"}]}`, Arguments: `{"token":"secret"}`, ReviewStatus: "active", Effect: "execute", Presentation: commandSettingsDefaultPresentation}
	if !commandSettingsBindingReadOnly(row) {
		t.Fatal("binding com condição/argumentos não foi marcado read-only")
	}
	row.Condition = commandSettingsDefaultCondition
	row.Arguments = commandSettingsDefaultArguments
	if commandSettingsBindingReadOnly(row) {
		t.Fatal("binding vazio foi marcado read-only")
	}
}

func TestCommandSettingsTriggerSpecPreservaGramaticaReal(t *testing.T) {
	keyboard, err := commandSettingsTriggerSpec(nil, "keyboard.local:Control+Shift+KeyK")
	if err != nil || keyboard != `{"code":"KeyK","modifiers":["Control","Shift"],"version":1}` {
		t.Fatalf("keyboard não foi normalizado para documento estrito: %q (%v)", keyboard, err)
	}
	palette, err := commandSettingsTriggerSpec(nil, "palette:workspace.list")
	if err != nil || palette != `{"selection":"workspace.list","version":1}` {
		t.Fatalf("palette não foi normalizada para documento estrito: %q (%v)", palette, err)
	}
}

func TestCommandSettingsPublicProjectionFailsClosedForInvalidDocuments(t *testing.T) {
	if got := commandSettingsPublicArguments(`{"unterminated"`); got != nil {
		t.Fatalf("argumentos inválidos foram ampliados para um objeto válido: %#v", got)
	}
	if got := commandSettingsPublicCondition(`{"version":2,"clauses":[]`); got.Version != 0 || got.Clauses != nil {
		t.Fatalf("condição inválida foi normalizada como vazia válida: %+v", got)
	}
	if got := commandSettingsPublicCondition(`{}`); got.Version != 1 || got.Clauses == nil || len(got.Clauses) != 0 {
		t.Fatalf("condição vazia canônica não foi preservada: %+v", got)
	}
}

func TestCommandSettingsRuleInputCanonicalizesTypedEmptyCondition(t *testing.T) {
	rule, err := commandSettingsRuleInput(&CommandSettingsRuleInput{LayerID: "layer", Mode: "manual", Lifecycle: "persistent", Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{}}}, "rule")
	if err != nil || rule.Condition != "{}" {
		t.Fatalf("condição manual vazia não foi canonicalizada: rule=%+v err=%v", rule, err)
	}
	for _, condition := range []*CommandSettingsCondition{{}, {Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Op: "eq", Value: "chat"}}}} {
		if _, err := commandSettingsRuleInput(&CommandSettingsRuleInput{LayerID: "layer", Mode: "manual", Lifecycle: "persistent", Condition: condition}, "rule"); err == nil {
			t.Fatalf("manual rule accepted invalid or ignored restriction: %+v", condition)
		}
	}
}

func validWithTrigger(request CommandBindingEdit, triggerType string) CommandBindingEdit {
	request.TriggerType = triggerType
	return request
}

func TestCommandSettingsSnapshotConsolidaSupressaoEReview(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	done := settingsSecurityStart(t, a, func() (CommandSettingsMutation, error) {
		return a.SetDefaultCommandSuppressed("builtin.palette.workspace.list", true)
	})
	appCommandImportWailsRespond(t, a, decisions, commanddecision.ApplyAction)
	if outcome := settingsSecurityFinish(t, done); outcome.err != nil || !outcome.result.Committed {
		t.Fatalf("supressão não persistiu: %+v", outcome)
	}
	snapshot, err := a.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	var found []CommandSettingsBinding
	for _, row := range snapshot.Bindings {
		if row.DefaultID == "builtin.palette.workspace.list" {
			found = append(found, row)
		}
	}
	if len(found) != 1 || found[0].CommandID != "" || !found[0].Customized || found[0].ReadOnly || !found[0].Suppressed || found[0].PersistedEnabled != true || found[0].Enabled || found[0].ReviewStatus != "active" {
		t.Fatalf("linha consolidada de supressão inválida: %+v", found)
	}
	overrideID := findSettingsBindingID(t, a, "builtin.palette.workspace.list")
	if err := database.DB().Model(&commandconfig.Binding{}).Where("id = ?", overrideID).Update("review_status", "needs_review").Error; err != nil {
		t.Fatal(err)
	}
	snapshot, err = a.GetCommandSettings("pt-BR")
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range snapshot.Bindings {
		if row.DefaultID == "builtin.palette.workspace.list" && row.ReviewStatus != "needs_review" {
			t.Fatalf("review não refletido: %+v", row)
		}
	}
}

func findSettingsBindingID(t *testing.T, a *App, defaultID string) string {
	t.Helper()
	p, err := a.authenticatedCommandProduct()
	if err != nil {
		t.Fatal(err)
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := store.Load(a.commandBridgeContext(), commandconfig.Scope{UserID: p.principal.UserID})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range snapshot.Bindings {
		if row.ReplacesDefaultID != nil && *row.ReplacesDefaultID == defaultID {
			return row.ID
		}
	}
	t.Fatalf("override não encontrado")
	return ""
}
