package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commanddeck"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/toolinvocations"
	commandtool "assistente/internal/tools/command"
	"assistente/internal/tools/invocationctx"
	"github.com/google/uuid"
)

type layerOriginConvergenceSetup struct {
	controlLayerID string
	ruleID         string
	shortcuts      map[string]LocalCommandShortcut
}

type convergencePhysicalDeckDriver struct{ opened chan *multiPhysicalDeckHandle }

func (d *convergencePhysicalDeckDriver) Enumerate(context.Context) ([]commanddeck.PhysicalDevice, error) {
	model := commanddeck.Model{ID: "fixture-convergence", Name: "Fixture convergence", Rows: 1, Columns: 3, KeyImageW: 72, KeyImageH: 72, SupportsHID: true}
	return []commanddeck.PhysicalDevice{{ID: "DECKA123456", Model: model}}, nil
}

func (d *convergencePhysicalDeckDriver) Open(ctx context.Context, device commanddeck.PhysicalDevice) (commanddeck.Handle, error) {
	return (&multiPhysicalDeckDriver{opened: d.opened}).Open(ctx, device)
}

func setupLayerOriginConvergence(t *testing.T, a *App, decisions <-chan map[string]any) layerOriginConvergenceSetup {
	t.Helper()
	control := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeGlobal,
		Operation: "layer_create",
		Layer:     &CommandSettingsLayerInput{Name: "Convergência de origens", Enabled: true},
	})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeGlobal,
		Operation: "rule_create",
		Rule:      &CommandSettingsRuleInput{LayerID: control.ID, Mode: "always", Lifecycle: "persistent", Enabled: true},
	})
	target := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeGlobal,
		Operation: "layer_create",
		Layer:     &CommandSettingsLayerInput{Name: "Alvo da convergência", Enabled: true},
	})
	rule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeGlobal,
		Operation: "rule_create",
		Rule:      &CommandSettingsRuleInput{LayerID: target.ID, Mode: "manual", Lifecycle: "persistent", Enabled: true},
	})

	shortcuts := map[string]LocalCommandShortcut{
		commandLayerActivateID: {Version: 1, Code: "KeyA", Modifiers: []string{"Control", "Shift"}},
		commandLayerToggleID:   {Version: 1, Code: "KeyT", Modifiers: []string{"Control", "Shift"}},
		commandLayerBackID:     {Version: 1, Code: "KeyB", Modifiers: []string{"Control", "Shift"}},
	}
	for _, action := range []string{commandLayerActivateID, commandLayerToggleID, commandLayerBackID} {
		args := map[string]any{"scope": "global", "rule_id": rule.ID, "duration_seconds": 0}
		if action == commandLayerBackID {
			args["rule_id"] = ""
		}
		paletteSpec, err := json.Marshal(map[string]any{"version": 1, "selection": action})
		if err != nil {
			t.Fatal(err)
		}
		settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
			Scope:     CommandSettingsScopeGlobal,
			Operation: "binding_create",
			Binding: &CommandSettingsBindingInput{
				LayerID: control.ID, CommandID: action, TriggerType: "palette", TriggerSpec: string(paletteSpec),
				Arguments: args, Effect: "execute", Enabled: true,
			},
		})

		keyboardSpec, err := json.Marshal(shortcuts[action])
		if err != nil {
			t.Fatal(err)
		}
		settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
			Scope:     CommandSettingsScopeGlobal,
			Operation: "binding_create",
			Binding: &CommandSettingsBindingInput{
				LayerID: control.ID, CommandID: action, TriggerType: "keyboard.local", TriggerSpec: string(keyboardSpec),
				Arguments: args, Effect: "execute", Enabled: true,
			},
		})

		key := map[string]int{commandLayerActivateID: 0, commandLayerToggleID: 1, commandLayerBackID: 2}[action]
		deckSpec, err := json.Marshal(map[string]any{"version": 1, "device": "DECKA123456", "key": key})
		if err != nil {
			t.Fatal(err)
		}
		settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
			Scope:     CommandSettingsScopeGlobal,
			Operation: "binding_create",
			Binding: &CommandSettingsBindingInput{
				LayerID: control.ID, CommandID: action, TriggerType: "streamdeck.key", TriggerSpec: string(deckSpec),
				Arguments: args, Effect: "execute", Enabled: true,
			},
		})
	}
	return layerOriginConvergenceSetup{controlLayerID: control.ID, ruleID: rule.ID, shortcuts: shortcuts}
}

func assertLayerCatalogHasOneSharedDefinition(t *testing.T, a *App) {
	t.Helper()
	p := a.commandProduct.Load()
	if p == nil || p.registry == nil {
		t.Fatal("catálogo produtivo ausente")
	}
	for _, id := range []string{commandLayerActivateID, commandLayerToggleID, commandLayerBackID} {
		matches := 0
		for _, definition := range p.registry.List() {
			if definition.ID != id {
				continue
			}
			matches++
			if definition.HandlerRoute != "internal/layer/action" || definition.HandlerClassification != commandcatalog.HandlerBackend || definition.Effect != commandcatalog.Write {
				t.Fatalf("definição %s não usa o handler produtivo comum: %+v", id, definition)
			}
			for _, source := range []commandcatalog.Source{commandcatalog.Chat, commandcatalog.Palette, commandcatalog.KeyboardLocal, commandcatalog.StreamDeck} {
				if !definition.AllowsSource(source) {
					t.Fatalf("%s não permite origem %s no catálogo real", id, source)
				}
			}
		}
		if matches != 1 {
			t.Fatalf("%s aparece %d vezes no catálogo real", id, matches)
		}
	}
}

func layerActiveForOrigin(t *testing.T, userID, ruleID, instance string) int64 {
	t.Helper()
	var count int64
	if err := database.DB().Table("command_layer_activation_state").Where("user_id = ? AND rule_ref = ? AND source_type = ? AND source_instance_id = ? AND state = ?", userID, ruleID, "manual", instance, "active").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func waitLayerOriginState(t *testing.T, userID, ruleID, instance string, active bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if (layerActiveForOrigin(t, userID, ruleID, instance) == 1) == active {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("claim da origem %q não alcançou active=%v", instance, active)
}

func executePaletteLayerAction(t *testing.T, a *App, id string) {
	t.Helper()
	result, err := a.ExecutePaletteCommand(id, nil)
	if err != nil || result.Status != string(commandledger.Succeeded) {
		t.Fatalf("palette %s: %+v err=%v", id, result, err)
	}
}

func dispatchKeyboardLayerAction(t *testing.T, a *App, shortcut LocalCommandShortcut) {
	t.Helper()
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, binding := range view.Bindings {
		if binding.Shortcut.Code == shortcut.Code && binding.CommandID != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("atalho %s não foi publicado no mapa real", shortcut.Code)
	}
	result, err := a.DispatchLocalCommandKey(view.Generation, shortcut, "down", false)
	if err != nil || result == nil || result.Status != string(commandledger.Succeeded) {
		t.Fatalf("keyboard %s: %+v err=%v", shortcut.Code, result, err)
	}
}

func waitConvergenceDeckHandle(t *testing.T, opened <-chan *multiPhysicalDeckHandle, retired *multiPhysicalDeckHandle) *multiPhysicalDeckHandle {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case handle := <-opened:
			if handle != nil && handle.serial == "DECKA123456" && handle != retired {
				return handle
			}
		case <-deadline:
			t.Fatal("Deck A não publicou uma nova sessão após a mutação")
		}
	}
}

func countLayerInvocations(t *testing.T, userID, source, commandID, status string) int64 {
	t.Helper()
	var count int64
	query := database.DB().Table("command_invocations").Where("user_id = ? AND source_type = ? AND command_id = ?", userID, source, commandID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if err := query.Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func waitLayerSucceededInvocations(t *testing.T, userID, source, commandID string, want int64) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if countLayerInvocations(t, userID, source, commandID, string(commandledger.Succeeded)) >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("execuções %s/%s não chegaram a succeeded=%d", source, commandID, want)
}

func TestCommandLayerActionsConvergePersistAndIsolateOrigins(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	setup := setupLayerOriginConvergence(t, a, decisions)
	assertLayerCatalogHasOneSharedDefinition(t, a)

	chatCtx := commandAgentTestContext(t, a, commandtool.CatalogName)
	chatInvocation, ok := invocationctx.Get(chatCtx)
	if !ok {
		t.Fatal("contexto de chat sem conversa")
	}
	chatRequest := commandtool.Request{
		Action: "execute", CommandID: commandLayerActivateID, Locale: "pt-BR",
		Arguments: json.RawMessage(`{"scope":"global","rule_id":"` + setup.ruleID + `","duration_seconds":0}`),
	}
	chatResult := commandAgentExecuteWithDecision(t, a, decisions, chatCtx, chatRequest, "apply")
	if chatResult.Status != string(commandledger.Succeeded) {
		t.Fatalf("chat layer.activate: %+v", chatResult)
	}
	waitLayerOriginState(t, a.currentUserID, setup.ruleID, chatInvocation.ConversationID, true)

	// A pilha pertence à origem, não ao comando textual: o ciclo da palette
	// não pode encerrar a claim criada pelo chat.
	for _, step := range []struct {
		id     string
		active bool
	}{
		{commandLayerActivateID, true}, {commandLayerToggleID, false}, {commandLayerActivateID, true}, {commandLayerBackID, false},
	} {
		executePaletteLayerAction(t, a, step.id)
		waitLayerOriginState(t, a.currentUserID, setup.ruleID, "palette", step.active)
	}
	waitLayerOriginState(t, a.currentUserID, setup.ruleID, chatInvocation.ConversationID, true)

	for _, step := range []struct {
		id     string
		active bool
	}{
		{commandLayerActivateID, true}, {commandLayerToggleID, false}, {commandLayerActivateID, true}, {commandLayerBackID, false},
	} {
		dispatchKeyboardLayerAction(t, a, setup.shortcuts[step.id])
		waitLayerOriginState(t, a.currentUserID, setup.ruleID, "local-keyboard", step.active)
	}
	waitLayerOriginState(t, a.currentUserID, setup.ruleID, chatInvocation.ConversationID, true)

	p := a.commandProduct.Load()
	bindings, _, err := p.deckMap(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for key, id := range map[int]string{0: commandLayerActivateID, 1: commandLayerToggleID, 2: commandLayerBackID} {
		if binding, ok := bindings["DECKA123456"][key]; !ok || binding.commandID != id {
			t.Fatalf("mapa Deck sem %s na tecla %d: %#v", id, key, bindings["DECKA123456"])
		}
	}

	driver := &convergencePhysicalDeckDriver{opened: make(chan *multiPhysicalDeckHandle, 16)}
	deckCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.startDeck(deckCtx, driver)
	handles := map[string]*multiPhysicalDeckHandle{"DECKA123456": waitConvergenceDeckHandle(t, driver.opened, nil)}
	for _, step := range []struct {
		key    int
		active bool
	}{
		{0, true}, {1, false}, {0, true}, {2, false},
	} {
		handles["DECKA123456"].events <- commanddeck.PhysicalKeyEvent{Index: step.key, Down: true}
		waitLayerOriginState(t, a.currentUserID, setup.ruleID, "DECKA123456", step.active)
		current := handles["DECKA123456"]
		handles["DECKA123456"] = waitConvergenceDeckHandle(t, driver.opened, current)
		handles["DECKA123456"].events <- commanddeck.PhysicalKeyEvent{Index: step.key, Down: false}
	}
	waitLayerOriginState(t, a.currentUserID, setup.ruleID, chatInvocation.ConversationID, true)

	for _, expected := range []struct {
		source, commandID string
		count             int64
	}{
		{"streamdeck.key", commandLayerActivateID, 2},
		{"streamdeck.key", commandLayerToggleID, 1},
		{"streamdeck.key", commandLayerBackID, 1},
	} {
		waitLayerSucceededInvocations(t, a.currentUserID, expected.source, expected.commandID, expected.count)
	}

	for _, expected := range []struct {
		source, commandID string
		count             int64
	}{
		{"chat", commandLayerActivateID, 1},
		{"palette", commandLayerActivateID, 2},
		{"palette", commandLayerToggleID, 1},
		{"palette", commandLayerBackID, 1},
		{"keyboard.local", commandLayerActivateID, 2},
		{"keyboard.local", commandLayerToggleID, 1},
		{"keyboard.local", commandLayerBackID, 1},
		{"streamdeck.key", commandLayerActivateID, 2},
		{"streamdeck.key", commandLayerToggleID, 1},
		{"streamdeck.key", commandLayerBackID, 1},
	} {
		got := countLayerInvocations(t, a.currentUserID, expected.source, expected.commandID, "")
		if got != expected.count {
			t.Fatalf("quantidade de ingressos inesperada para %s/%s: got=%d want=%d", expected.source, expected.commandID, got, expected.count)
		}
		if succeeded := countLayerInvocations(t, a.currentUserID, expected.source, expected.commandID, string(commandledger.Succeeded)); succeeded != expected.count {
			t.Fatalf("execuções concluídas inesperadas para %s/%s: got=%d want=%d", expected.source, expected.commandID, succeeded, expected.count)
		}
	}
}

func TestCommandLayerActionsRejectForeignOwnerAndRevokedSession(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	setup := setupLayerOriginConvergence(t, a, decisions)
	ctx := commandAgentTestContext(t, a, commandtool.CatalogName)
	inv, ok := invocationctx.Get(ctx)
	if !ok {
		t.Fatal("contexto de agente ausente")
	}
	request := commandtool.Request{
		Action: "execute", CommandID: commandLayerActivateID, Locale: "pt-BR",
		Arguments: json.RawMessage(`{"scope":"global","rule_id":"` + setup.ruleID + `","duration_seconds":0}`),
	}
	foreign := database.WithUserID(ctx, uuid.NewString())
	if _, err := (commandAgentTools{app: a}).Catalog(foreign, request); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("owner estrangeiro aceito: %v", err)
	}
	if layerActiveForOrigin(t, a.currentUserID, setup.ruleID, inv.ConversationID) != 0 {
		t.Fatal("owner estrangeiro criou claim")
	}

	if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", a.commandProduct.Load().principal.SessionID).Error; err != nil {
		t.Fatal(err)
	}
	chatSucceededBefore := countLayerInvocations(t, a.currentUserID, "chat", commandLayerActivateID, string(commandledger.Succeeded))
	if _, err := (commandAgentTools{app: a}).Catalog(ctx, request); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("agente com sessão revogada aceito: %v", err)
	}
	if got := countLayerInvocations(t, a.currentUserID, "chat", commandLayerActivateID, string(commandledger.Succeeded)); got != chatSucceededBefore {
		t.Fatalf("sessão revogada criou execução succeeded de chat: antes=%d depois=%d", chatSucceededBefore, got)
	}
	if layerActiveForOrigin(t, a.currentUserID, setup.ruleID, inv.ConversationID) != 0 {
		t.Fatal("sessão revogada criou claim do chat")
	}
	result, err := a.ExecutePaletteCommand(commandLayerActivateID, nil)
	if err == nil && result.Status == string(commandledger.Succeeded) {
		t.Fatalf("sessão revogada executou layer.activate: %+v", result)
	}
	if layerActiveForOrigin(t, a.currentUserID, setup.ruleID, "palette") != 0 {
		t.Fatal("sessão revogada criou claim da palette")
	}
}

func TestCommandLayerChatToggleAndBackPersistAndReplay(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	setup := setupLayerOriginConvergence(t, a, decisions)
	ctx := commandAgentTestContext(t, a, commandtool.CatalogName)
	inv, ok := invocationctx.Get(ctx)
	if !ok {
		t.Fatal("contexto de chat ausente")
	}
	var initial database.ToolInvocation
	if err := database.DB().Where("id = ?", toolinvocations.CurrentInvocationID(ctx)).Take(&initial).Error; err != nil {
		t.Fatal(err)
	}
	for index, step := range []struct {
		commandID string
		active    bool
	}{
		{commandLayerActivateID, true},
		{commandLayerToggleID, false},
		{commandLayerToggleID, true},
		{commandLayerBackID, false},
	} {
		// Cada chamada tem sua própria invocação, mas pertence à mesma conversa.
		// Reutilizar o ID anterior aqui exercitaria replay, não a próxima ação.
		if index != 0 {
			next := database.ToolInvocation{
				UUIDModel: database.UUIDModel{ID: uuid.Must(uuid.NewV7()).String()},
				UserID:    initial.UserID, ToolCatalogID: initial.ToolCatalogID,
				OriginType: initial.OriginType, OriginID: initial.OriginID,
				ConversationID: initial.ConversationID, TurnID: initial.TurnID,
				Attempt: 1, Status: toolinvocations.StatusRunning, QueuedAt: time.Now().UTC(),
			}
			if err := database.DB().Create(&next).Error; err != nil {
				t.Fatal(err)
			}
			ctx = toolinvocations.WithCurrentInvocationID(ctx, next.ID)
		}
		args := map[string]any{"scope": "global", "rule_id": setup.ruleID, "duration_seconds": 0}
		if step.commandID == commandLayerBackID {
			args["rule_id"] = ""
		}
		encoded, err := json.Marshal(args)
		if err != nil {
			t.Fatal(err)
		}
		request := commandtool.Request{Action: "execute", CommandID: step.commandID, Locale: "pt-BR", Arguments: encoded}
		first := commandAgentExecuteWithDecision(t, a, decisions, ctx, request, "apply")
		if first.Status != string(commandledger.Succeeded) {
			t.Fatalf("chat %s: %+v", step.commandID, first)
		}
		waitLayerOriginState(t, a.currentUserID, setup.ruleID, inv.ConversationID, step.active)
		value, err := (commandAgentTools{app: a}).Catalog(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		replay := commandAgentExecutionJSON(t, value)
		if replay.InvocationID != first.InvocationID || replay.Status != first.Status {
			t.Fatalf("replay divergente de %s: first=%+v replay=%+v", step.commandID, first, replay)
		}
		waitLayerOriginState(t, a.currentUserID, setup.ruleID, inv.ConversationID, step.active)
		select {
		case decision := <-decisions:
			t.Fatalf("replay apresentou nova decisão: %+v", decision)
		default:
		}
	}
	for id, count := range map[string]int64{commandLayerActivateID: 1, commandLayerToggleID: 2, commandLayerBackID: 1} {
		if got := countLayerInvocations(t, a.currentUserID, "chat", id, ""); got != count {
			t.Fatalf("chat %s criou %d invocações, esperado %d", id, got, count)
		}
	}
}
