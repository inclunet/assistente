package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandconfig"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	commandtool "assistente/internal/tools/command"
	"gorm.io/gorm"
)

func TestCommandAgentConfigScopeUsesCallerRuntimeWorkspace(t *testing.T) {
	c := &commandAgentCaller{
		principal: auth.LocalSessionPrincipal{UserID: "user-1", SessionID: "session-1"},
		product:   &commandProductRuntime{workspaceID: "workspace-from-runtime"},
	}
	scope, err := commandAgentConfigScope(c, string(CommandSettingsScopeWorkspace))
	if err != nil {
		t.Fatalf("commandAgentConfigScope: %v", err)
	}
	if scope.UserID != c.principal.UserID || scope.WorkspaceID == nil || *scope.WorkspaceID != c.product.workspaceID {
		t.Fatalf("escopo derivado do caller/runtime: %#v", scope)
	}
}

func TestDecodeAgentSettingsMutationEnvelopeIsAuthoritative(t *testing.T) {
	request := commandtool.Request{
		Action:  "layer_update",
		ID:      "layer-1",
		Scope:   "global",
		Payload: []byte(`{"scope":"workspace","operation":"layer_delete","id":"other","expectedRevision":3,"expectedFingerprint":"fp","layer":{"id":"layer-1","name":"name","enabled":true}}`),
	}
	if _, err := decodeAgentSettingsMutation(request); !errors.Is(err, commandexecution.ErrInvalidRequest) {
		t.Fatalf("payload envelope conflitante deveria falhar: %v", err)
	}

	request.Payload = []byte(`{"scope":"global","operation":"layer_update","id":"layer-1","expectedRevision":3,"expectedFingerprint":"fp","layer":{"id":"layer-1","name":"name","enabled":true}}`)
	decoded, err := decodeAgentSettingsMutation(request)
	if err != nil {
		t.Fatalf("payload envelope idêntica: %v", err)
	}
	if decoded.Scope != CommandSettingsScopeGlobal || decoded.Operation != request.Action || decoded.ID != request.ID || decoded.ExpectedRevision != 3 {
		t.Fatalf("contrato decodificado: %#v", decoded)
	}
}

func TestCommandAgentConfigRejectsMutationArgumentsOutsidePayload(t *testing.T) {
	request := commandtool.Request{
		Action:    "layer_create",
		Scope:     "global",
		Arguments: []byte(`{"expectedRevision":1}`),
		Payload:   []byte(`{"expectedRevision":1,"expectedFingerprint":"fp","layer":{"name":"x"}}`),
	}
	if _, err := decodeAgentSettingsMutation(request); !errors.Is(err, commandexecution.ErrInvalidRequest) {
		t.Fatalf("arguments paralelo deveria falhar: %v", err)
	}
}

func TestDecodeAgentSettingsMutationRejectsIrrelevantPayloadShapes(t *testing.T) {
	tests := []struct {
		name    string
		action  string
		layerID string
		payload string
	}{
		{name: "rule", action: "layer_create", payload: `{"expectedRevision":1,"expectedFingerprint":"fp","rule":{}}`},
		{name: "default", action: "layer_create", payload: `{"expectedRevision":1,"expectedFingerprint":"fp","default":{}}`},
		{name: "layer on disable", action: "layer_disable", payload: `{"expectedRevision":1,"expectedFingerprint":"fp","layer":{"name":"x"}}`},
		{name: "binding on delete", action: "binding_delete", payload: `{"expectedRevision":1,"expectedFingerprint":"fp","binding":{}}`},
		{name: "layer ref outside restore", action: "layer_update", payload: `{"expectedRevision":1,"expectedFingerprint":"fp","layerRefKind":"user","layer":{"id":"l","name":"x","enabled":true}}`},
		{name: "duplicate key", action: "layer_create", payload: `{"expectedRevision":1,"expectedRevision":2,"expectedFingerprint":"fp","layer":{"name":"x","enabled":true}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := decodeAgentSettingsMutation(commandtool.Request{Action: test.action, Scope: "global", LayerID: test.layerID, Payload: []byte(test.payload)})
			if !errors.Is(err, commandexecution.ErrInvalidRequest) {
				t.Fatalf("payload aceito: %v", err)
			}
		})
	}
	if err := validateAgentConfigEnvelope(commandtool.Request{Action: "layer_get", Scope: "global", ID: "layer", LayerID: "unexpected"}, false); !errors.Is(err, commandexecution.ErrInvalidRequest) {
		t.Fatalf("layer_id extraneous aceito em layer_get: %v", err)
	}
}

// Usa o ledger/conversation fixture compartilhado de app_command_agent_test.go;
// não duplica uma sessão desktop nem injeta invocation metadata manualmente.
func TestCommandAgentConfigReadUsesTrustedToolContext(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	ctx := commandAgentTestContext(t, a, commandtool.ConfigName)
	value, err := (commandAgentTools{app: a}).Config(ctx, commandtool.Request{
		Action: "layer_list", Scope: string(CommandSettingsScopeGlobal), Locale: "pt-BR",
	})
	if err != nil {
		t.Fatalf("command_config layer_list: %v", err)
	}
	view, ok := value.(CommandSettingsSnapshot)
	if !ok {
		t.Fatalf("tipo de leitura: %T", value)
	}
	if view.Scope != string(CommandSettingsScopeGlobal) || view.Fingerprint == "" || view.Revision < 1 {
		t.Fatalf("stamp de leitura incompleto: %#v", view)
	}
	if view.Bindings != nil || view.Commands != nil || view.Diagnostics != nil || view.Adjustments != nil {
		t.Fatalf("layer_list carregou projeção da UI: %+v", view)
	}
}

func agentConfigView(t *testing.T, backend commandAgentTools, ctx context.Context) CommandSettingsSnapshot {
	t.Helper()
	value, err := backend.Config(ctx, commandtool.Request{Action: "layer_list", Scope: string(CommandSettingsScopeGlobal), Locale: "pt-BR"})
	if err != nil {
		t.Fatalf("leitura do stamp: %v", err)
	}
	view, ok := value.(CommandSettingsSnapshot)
	if !ok || view.Revision < 1 || view.Fingerprint == "" {
		t.Fatalf("view sem stamp: %#v", value)
	}
	return view
}

func agentConfigApply(t *testing.T, a *App, backend commandAgentTools, ctx context.Context, decisions <-chan map[string]any, request CommandSettingsMutationRequest) CommandSettingsMutation {
	t.Helper()
	view := agentConfigView(t, backend, ctx)
	request.Locale = "pt-BR"
	request.Scope = CommandSettingsScopeGlobal
	request.ExpectedRevision = view.Revision
	request.ExpectedFingerprint = view.Fingerprint
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan struct {
		value any
		err   error
	}, 1)
	go func() {
		value, err := backend.Config(ctx, commandtool.Request{
			Action: request.Operation, ID: request.ID, Scope: string(request.Scope), Locale: request.Locale,
			LayerID: func() string {
				if request.Binding != nil {
					return request.Binding.LayerID
				}
				return ""
			}(), Payload: payload,
		})
		done <- struct {
			value any
			err   error
		}{value, err}
	}()
	decision := agentConfigDecision(t, decisions)
	if err := a.questionnaireMgr.Respond(decision["id"].(string), map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false); err != nil {
		t.Fatal(err)
	}
	var result struct {
		value any
		err   error
	}
	select {
	case result = <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("agent %s não encerrou após decisão", request.Operation)
	}
	if result.err != nil {
		t.Fatalf("agent %s: %v", request.Operation, result.err)
	}
	mutation, ok := result.value.(CommandSettingsMutation)
	if !ok || !mutation.Committed || !mutation.Published || mutation.ID == "" {
		t.Fatalf("agent %s resultado: %#v", request.Operation, result.value)
	}
	configuration, _, err := a.commandProduct.Load().host.UserConfiguration(ctx, a.currentUserID)
	if err != nil || configuration == nil {
		t.Fatalf("agent %s não publicou mapa: %v", request.Operation, err)
	}
	return mutation
}

func agentConfigDecision(t *testing.T, decisions <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case decision := <-decisions:
		if _, ok := decision["id"].(string); !ok {
			t.Fatalf("questionnaire sem id: %#v", decision)
		}
		return decision
	case <-time.After(5 * time.Second):
		t.Fatal("decision questionnaire não foi apresentado")
		return nil
	}
}

func TestCommandAgentConfigConfirmedCRUDPublishesRealState(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	backend := commandAgentTools{app: a}
	ctx := commandAgentTestContext(t, a, commandtool.ConfigName)

	layer := agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{
		Operation: "layer_create",
		Layer:     &CommandSettingsLayerInput{Name: "agent CRUD layer", Description: "agent", Enabled: true, ResolutionPriority: 4},
	})
	var storedLayer commandconfig.Layer
	if err := database.DB().First(&storedLayer, "id = ?", layer.ID).Error; err != nil || storedLayer.Name != "agent CRUD layer" {
		t.Fatalf("layer_create não persistiu: %+v %v", storedLayer, err)
	}

	agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{
		Operation: "layer_update", ID: layer.ID,
		Layer: &CommandSettingsLayerInput{ID: layer.ID, Name: "agent CRUD renamed", Description: "updated", Enabled: true, ResolutionPriority: 8},
	})
	if err := database.DB().First(&storedLayer, "id = ?", layer.ID).Error; err != nil || storedLayer.Name != "agent CRUD renamed" || storedLayer.ResolutionPriority != 8 {
		t.Fatalf("layer_update não persistiu: %+v %v", storedLayer, err)
	}
	for _, operation := range []string{"layer_disable", "layer_enable"} {
		agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{Operation: operation, ID: layer.ID})
		if err := database.DB().First(&storedLayer, "id = ?", layer.ID).Error; err != nil || storedLayer.Enabled != (operation == "layer_enable") {
			t.Fatalf("%s não persistiu enabled: %+v %v", operation, storedLayer, err)
		}
	}

	binding := agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{
		Operation: "binding_create",
		Binding:   &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: commandProductWorkspaceListID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyK","modifiers":["Control"]}`, Arguments: map[string]any{}, Effect: "execute", Enabled: true, ResolutionPriority: 1},
	})
	var storedBinding commandconfig.Binding
	if err := database.DB().First(&storedBinding, "id = ?", binding.ID).Error; err != nil || storedBinding.LayerRef != layer.ID || storedBinding.CommandID == nil || *storedBinding.CommandID != commandProductWorkspaceListID {
		t.Fatalf("binding_create não persistiu: %+v %v", storedBinding, err)
	}
	layerValue, err := backend.Config(ctx, commandtool.Request{Action: "layer_get", Scope: "global", ID: layer.ID})
	if err != nil {
		t.Fatal(err)
	}
	layerView, ok := layerValue.(CommandSettingsSnapshot)
	if !ok || len(layerView.Layers) != 1 || len(layerView.Bindings) != 1 || layerView.Layers[0].ID != layer.ID || layerView.Bindings[0].ID != binding.ID || layerView.Commands != nil {
		t.Fatalf("layer_get não foi escopado: %#v", layerValue)
	}
	bindingValue, err := backend.Config(ctx, commandtool.Request{Action: "binding_list", Scope: "global", LayerID: layer.ID})
	if err != nil {
		t.Fatal(err)
	}
	bindingView, ok := bindingValue.(CommandSettingsSnapshot)
	if !ok || len(bindingView.Bindings) != 1 || bindingView.Bindings[0].ID != binding.ID || bindingView.Layers != nil || bindingView.Rules != nil || bindingView.Commands != nil {
		t.Fatalf("binding_list não foi escopado: %#v", bindingValue)
	}

	agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{
		Operation: "binding_update", ID: binding.ID,
		Binding: &CommandSettingsBindingInput{ID: binding.ID, LayerID: layer.ID, CommandID: commandProductWorkspaceListID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyK","modifiers":["Control"]}`, Arguments: map[string]any{}, Effect: "execute", Enabled: true, ResolutionPriority: 3},
	})
	if err := database.DB().First(&storedBinding, "id = ?", binding.ID).Error; err != nil || storedBinding.ResolutionPriority != 3 {
		t.Fatalf("binding_update não persistiu: %+v %v", storedBinding, err)
	}
	for _, operation := range []string{"binding_disable", "binding_enable"} {
		agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{Operation: operation, ID: binding.ID})
		if err := database.DB().First(&storedBinding, "id = ?", binding.ID).Error; err != nil || storedBinding.Enabled != (operation == "binding_enable") {
			t.Fatalf("%s não persistiu enabled: %+v %v", operation, storedBinding, err)
		}
	}

	agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{Operation: "binding_delete", ID: binding.ID})
	if err := database.DB().First(&storedBinding, "id = ?", binding.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("binding_delete manteve registro: %+v %v", storedBinding, err)
	}
	agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{Operation: "layer_delete", ID: layer.ID})
	if err := database.DB().First(&storedLayer, "id = ?", layer.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("layer_delete manteve registro: %+v %v", storedLayer, err)
	}
}

func TestCommandAgentConfigStaleStampDoesNotPresentDecision(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	backend := commandAgentTools{app: a}
	ctx := commandAgentTestContext(t, a, commandtool.ConfigName)
	view := agentConfigView(t, backend, ctx)
	layer := commandconfig.Layer{ID: appCommandPortabilityUUID(t), UserID: a.currentUserID, Name: "concurrent agent writer", Enabled: true, Source: "user", CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := database.DB().Create(&layer).Error; err != nil {
		t.Fatal(err)
	}
	request := CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", ExpectedRevision: view.Revision, ExpectedFingerprint: view.Fingerprint, Layer: &CommandSettingsLayerInput{Name: "stale", Enabled: true}}
	payload, _ := json.Marshal(request)
	if _, err := backend.Config(ctx, commandtool.Request{Action: request.Operation, Scope: "global", Payload: payload}); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("stamp antigo aceito: %v", err)
	}
	select {
	case decision := <-decisions:
		t.Fatalf("stamp antigo abriu decisão: %#v", decision)
	default:
	}
}

func TestCommandAgentConfigRevokedSessionAfterPromptDoesNotWrite(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	backend := commandAgentTools{app: a}
	ctx := commandAgentTestContext(t, a, commandtool.ConfigName)
	view := agentConfigView(t, backend, ctx)
	request := CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", ExpectedRevision: view.Revision, ExpectedFingerprint: view.Fingerprint, Layer: &CommandSettingsLayerInput{Name: "revoked agent write", Enabled: true}}
	payload, _ := json.Marshal(request)
	done := make(chan struct {
		value any
		err   error
	}, 1)
	go func() {
		value, err := backend.Config(ctx, commandtool.Request{Action: request.Operation, Scope: "global", Payload: payload})
		done <- struct {
			value any
			err   error
		}{value, err}
	}()
	decision := agentConfigDecision(t, decisions)
	if err := database.DB().Model(&database.Session{}).Where("id = ?", a.commandProduct.Load().principal.SessionID).Update("revoked_at", time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}
	finishCommandDecision(t, a.questionnaireMgr, decision, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction}, false)
	var result struct {
		value any
		err   error
	}
	select {
	case result = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("mutação revogada não encerrou")
	}
	if result.err == nil || result.value != nil {
		t.Fatalf("sessão revogada aceitou mutação: %#v", result)
	}
	var count int64
	if err := database.DB().Model(&commandconfig.Layer{}).Where("name = ?", "revoked agent write").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("escrita após revogação: count=%d err=%v", count, err)
	}
}

func TestCommandAgentConfigD10MutableActionMapIsClosed(t *testing.T) {
	actions := []string{
		"layer_create", "layer_update", "layer_delete", "layer_enable", "layer_disable", "layer_restore",
		"binding_create", "binding_update", "binding_delete", "binding_enable", "binding_disable", "binding_restore",
		"config_import",
	}
	for _, action := range actions {
		mutates, known := commandtool.ConfigActionMutates(action)
		if !known || !mutates {
			t.Errorf("ação mutável D10 não mapeada: %q known=%v mutates=%v", action, known, mutates)
		}
	}
	if _, known := commandtool.ConfigActionMutates("config_restore"); known {
		t.Error("config_restore não é verbo público de command_config")
	}
}

func TestCommandAgentConfigExplicitDenyDoesNotWrite(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	backend := commandAgentTools{app: a}
	ctx := commandAgentTestContext(t, a, commandtool.ConfigName)
	view := agentConfigView(t, backend, ctx)
	request := CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", ExpectedRevision: view.Revision, ExpectedFingerprint: view.Fingerprint, Layer: &CommandSettingsLayerInput{Name: "denied agent write", Enabled: true}}
	payload, _ := json.Marshal(request)
	done := make(chan struct {
		value any
		err   error
	}, 1)
	go func() {
		value, err := backend.Config(ctx, commandtool.Request{Action: request.Operation, Scope: "global", Payload: payload})
		done <- struct {
			value any
			err   error
		}{value, err}
	}()
	decision := agentConfigDecision(t, decisions)
	if err := a.questionnaireMgr.Respond(decision["id"].(string), map[string]any{questionnaire.AnswerActionID: commanddecision.DenyAction}, false); err != nil {
		t.Fatal(err)
	}
	var result struct {
		value any
		err   error
	}
	select {
	case result = <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("negação explícita não encerrou")
	}
	if result.err == nil || result.value != nil {
		t.Fatalf("negação aceitou mutação: %#v", result)
	}
	var count int64
	if err := database.DB().Model(&commandconfig.Layer{}).Where("name = ?", "denied agent write").Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("negação escreveu layer: count=%d err=%v", count, err)
	}
}

func TestCommandAgentConfigBindingRestoreBuiltinOverride(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	backend := commandAgentTools{app: a}
	ctx := commandAgentTestContext(t, a, commandtool.ConfigName)
	value, err := backend.Config(ctx, commandtool.Request{Action: "binding_list", Scope: "global"})
	if err != nil {
		t.Fatal(err)
	}
	view, ok := value.(CommandSettingsSnapshot)
	if !ok {
		t.Fatalf("binding_list: %T", value)
	}
	var target CommandSettingsBinding
	for _, row := range view.Bindings {
		if row.DefaultID == "builtin.palette.workspace.list" {
			target = row
			break
		}
	}
	if target.DefaultID == "" || target.CurrentDefaultVersion == "" || target.CurrentDefaultFingerprint == "" {
		t.Fatalf("default para restore ausente: %+v", target)
	}
	override := agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{Operation: "binding_create", Binding: settingsDefaultOverrideInput(target)})
	var row commandconfig.Binding
	if err := database.DB().First(&row, "id = ?", override.ID).Error; err != nil || row.ReplacesDefaultID == nil {
		t.Fatalf("override não persistiu: %+v %v", row, err)
	}
	agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{Operation: "binding_restore", ID: override.ID})
	if err := database.DB().Where("id = ?", override.ID).First(&row).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("binding_restore manteve override: %+v %v", row, err)
	}
}

func TestCommandAgentConfigLayerRestoreUserRemovesBindings(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	backend := commandAgentTools{app: a}
	ctx := commandAgentTestContext(t, a, commandtool.ConfigName)
	layer := agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "restore user layer", Enabled: true}})
	binding := agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{Operation: "binding_create", Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: commandProductWorkspaceListID, TriggerType: "keyboard.local", TriggerSpec: `{"version":1,"code":"KeyR","modifiers":["Control"]}`, Effect: "execute", Enabled: true}})
	agentConfigApply(t, a, backend, ctx, decisions, CommandSettingsMutationRequest{Operation: "layer_restore", ID: layer.ID, LayerRefKind: "user"})
	var storedLayer commandconfig.Layer
	if err := database.DB().First(&storedLayer, "id = ?", layer.ID).Error; err != nil {
		t.Fatalf("layer_restore removeu a layer user: %v", err)
	}
	var storedBinding commandconfig.Binding
	if err := database.DB().First(&storedBinding, "id = ?", binding.ID).Error; !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("layer_restore manteve binding: %+v %v", storedBinding, err)
	}
}
