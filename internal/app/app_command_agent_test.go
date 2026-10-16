package app

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/questionnaire"
	"assistente/internal/toolinvocations"
	commandtool "assistente/internal/tools/command"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/wailsapi"
	"github.com/google/uuid"
)

func commandAgentTestContext(t *testing.T, a *App, toolName string) context.Context {
	t.Helper()
	db := database.DB()
	if err := db.AutoMigrate(&database.ToolCatalog{}, &database.ToolInvocation{}, &database.Conversation{}); err != nil {
		t.Fatal(err)
	}

	userID := a.currentUserID
	catalogID := uuid.Must(uuid.NewV7()).String()
	conversationID := uuid.Must(uuid.NewV7()).String()
	turnID := uuid.Must(uuid.NewV7()).String()
	invocationID := uuid.Must(uuid.NewV7()).String()
	var catalog database.ToolCatalog
	result := db.Where("name = ? AND origin = ?", toolName, "builtin").Find(&catalog)
	if result.Error != nil {
		t.Fatal(result.Error)
	}
	if result.RowsAffected == 0 {
		catalog = database.ToolCatalog{
			UUIDModel:          database.UUIDModel{ID: catalogID},
			Name:               toolName,
			DisplayName:        toolName,
			Description:        "tool de comando do teste",
			Origin:             "builtin",
			Schema:             `{"type":"object"}`,
			AvailabilityStatus: "available",
		}
		if err := db.Create(&catalog).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&database.Conversation{
		UUIDModel: database.UUIDModel{ID: conversationID},
		UserID:    userID,
		Title:     "conversa local do agente",
		Channel:   "",
		Kind:      "",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.ToolInvocation{
		UUIDModel:      database.UUIDModel{ID: invocationID},
		UserID:         userID,
		ToolCatalogID:  catalog.ID,
		OriginType:     toolinvocations.OriginChat,
		OriginID:       turnID,
		ConversationID: &conversationID,
		TurnID:         &turnID,
		Attempt:        1,
		Status:         toolinvocations.StatusRunning,
		DryRun:         false,
		QueuedAt:       time.Now().UTC(),
	}).Error; err != nil {
		t.Fatal(err)
	}

	if a.chatAPI == nil || !a.chatDesktopIngress {
		SetChatAPI(a, wailsapi.NewChat())
	}
	base, err := (commandChatSession{wailsSession{app: a}}).AuthenticatedContext()
	if err != nil {
		t.Fatal(err)
	}
	ctx := base
	ctx = invocationctx.With(ctx, invocationctx.InvocationContext{
		ConversationID: conversationID,
		TurnID:         turnID,
		ProfileSlug:    "lifecycle-profile",
		Source:         "chat",
	})
	return toolinvocations.WithCurrentInvocationID(ctx, invocationID)
}

func TestCommandAgentRequiresCurrentChatSessionStamp(t *testing.T) {
	a := readyCommandProduct(t)
	valid := commandAgentTestContext(t, a, commandtool.CatalogName)
	inv, ok := invocationctx.Get(valid)
	if !ok {
		t.Fatal("invocation context ausente")
	}
	invocationID := toolinvocations.CurrentInvocationID(valid)
	backend := commandAgentTools{app: a}

	withoutStamp := database.WithUserID(context.Background(), a.currentUserID)
	withoutStamp = invocationctx.With(withoutStamp, inv)
	withoutStamp = toolinvocations.WithCurrentInvocationID(withoutStamp, invocationID)
	if _, err := backend.Catalog(withoutStamp, commandtool.Request{Action: "list"}); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("contexto sem stamp aceito: %v", err)
	}

	wrongStamp := context.WithValue(valid, commandAgentSessionKey{}, auth.LocalSessionPrincipal{
		UserID:    a.currentUserID,
		SessionID: uuid.Must(uuid.NewV7()).String(),
	})
	if _, err := backend.Catalog(wrongStamp, commandtool.Request{Action: "list"}); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("stamp de sessão anterior aceito: %v", err)
	}
}

func TestCommandAgentCLIIngressWithoutRegisteredGUIHasNoAgentStamp(t *testing.T) {
	a := readyCommandProduct(t)
	valid := commandAgentTestContext(t, a, commandtool.CatalogName)
	inv, ok := invocationctx.Get(valid)
	if !ok {
		t.Fatal("invocation context ausente")
	}
	invocationID := toolinvocations.CurrentInvocationID(valid)
	SetChatAPI(a, nil)
	cliCtx, err := (commandChatSession{wailsSession{app: a}}).AuthenticatedContext()
	if err != nil {
		t.Fatal(err)
	}
	if _, stamped := cliCtx.Value(commandAgentSessionKey{}).(auth.LocalSessionPrincipal); stamped {
		t.Fatal("ingresso sem GUI recebeu stamp de sessão")
	}
	cliCtx = invocationctx.With(cliCtx, inv)
	cliCtx = toolinvocations.WithCurrentInvocationID(cliCtx, invocationID)
	if _, err := (commandAgentTools{app: a}).Catalog(cliCtx, commandtool.Request{Action: "list"}); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("ingresso CLI sem GUI recebeu autoridade de agente: %v", err)
	}
}

func TestCommandAgentCatalogListsAndDescribesRealCatalog(t *testing.T) {
	a := readyCommandProduct(t)
	ctx := commandAgentTestContext(t, a, commandtool.CatalogName)
	tool := commandtool.NewCatalog(commandAgentTools{app: a})

	list, err := tool.Execute(ctx, json.RawMessage(`{"action":"list","locale":"en"}`))
	if err != nil || list.IsError || !list.Structured {
		t.Fatalf("list: %+v %v", list, err)
	}
	var listed struct {
		Commands []struct {
			ID string `json:"id"`
		} `json:"commands"`
	}
	if err := json.Unmarshal([]byte(list.Content), &listed); err != nil {
		t.Fatalf("list JSON: %v (%s)", err, list.Content)
	}
	found := false
	for _, command := range listed.Commands {
		if command.ID == commandProductWorkspaceListID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("workspace.list ausente do catálogo: %s", list.Content)
	}

	describe, err := tool.Execute(ctx, json.RawMessage(`{"action":"describe","command_id":"workspace.list","locale":"en"}`))
	if err != nil || describe.IsError || !describe.Structured {
		t.Fatalf("describe: %+v %v", describe, err)
	}
	var described struct {
		ID                 string `json:"id"`
		ExecutableFromChat bool   `json:"executable_from_chat"`
	}
	if err := json.Unmarshal([]byte(describe.Content), &described); err != nil {
		t.Fatalf("describe JSON: %v (%s)", err, describe.Content)
	}
	if described.ID != commandProductWorkspaceListID || !described.ExecutableFromChat {
		t.Fatalf("describe inesperado: %s", describe.Content)
	}
}

func TestCommandAgentExecuteRecordsAgentAndReplaysByCallerInvocation(t *testing.T) {
	a := readyCommandProduct(t)
	ctx := commandAgentTestContext(t, a, commandtool.CatalogName)
	backend := commandAgentTools{app: a}
	request := commandtool.Request{Action: "execute", CommandID: commandProductWorkspaceListID, Arguments: json.RawMessage(`{}`)}

	firstValue, err := backend.Catalog(ctx, request)
	if err != nil {
		t.Fatalf("first execute: %v", err)
	}
	first := commandAgentExecutionJSON(t, firstValue)
	if first.InvocationID != toolinvocations.CurrentInvocationID(ctx) || first.Status != string(commandledger.Succeeded) || first.CommandID != commandProductWorkspaceListID {
		t.Fatalf("first result: %+v", first)
	}

	var audit struct {
		ActorType string `gorm:"column:actor_type"`
		ActorID   string `gorm:"column:actor_id"`
		Status    string `gorm:"column:status"`
		CommandID string `gorm:"column:command_id"`
	}
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", first.InvocationID).Take(&audit).Error; err != nil {
		t.Fatal(err)
	}
	if audit.ActorType != "agent" || audit.ActorID != toolinvocations.CurrentInvocationID(ctx) || audit.Status != string(commandledger.Succeeded) || audit.CommandID != commandProductWorkspaceListID {
		t.Fatalf("ledger de agente inesperado: %+v", audit)
	}

	secondValue, err := backend.Catalog(ctx, request)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	second := commandAgentExecutionJSON(t, secondValue)
	if second.InvocationID != first.InvocationID || second.Status != first.Status {
		t.Fatalf("replay criou resultado diferente: first=%+v second=%+v", first, second)
	}
	var count int64
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", first.InvocationID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("ledger replay count=%d err=%v", count, err)
	}

	_, err = backend.Catalog(ctx, commandtool.Request{Action: "execute", CommandID: commandProductWorkspaceListID, Arguments: json.RawMessage(`{"changed":true}`)})
	if !errors.Is(err, commandledger.ErrConflict) {
		t.Fatalf("argumentos alterados: err=%v, want %v", err, commandledger.ErrConflict)
	}
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", first.InvocationID).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("ledger após conflito count=%d err=%v", count, err)
	}
}

func TestCommandAgentLayerActionDecisionOriginAndReplay(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeGlobal,
		Operation: "layer_create",
		Layer:     &CommandSettingsLayerInput{Name: "Camada do agente", Enabled: true},
	})
	rule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope:     CommandSettingsScopeGlobal,
		Operation: "rule_create",
		Rule:      &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "manual", Lifecycle: "persistent", Enabled: true},
	})
	request := commandtool.Request{
		Action:    "execute",
		CommandID: commandLayerActivateID,
		Locale:    "pt-BR",
		Arguments: json.RawMessage(`{"scope":"global","rule_id":"` + rule.ID + `","duration_seconds":0}`),
	}
	ctx := commandAgentTestContext(t, a, commandtool.CatalogName)
	first := commandAgentExecuteWithDecision(t, a, decisions, ctx, request, commanddecision.ApplyAction)
	if first.Status != string(commandledger.Succeeded) || first.InvocationID != toolinvocations.CurrentInvocationID(ctx) {
		t.Fatalf("ação de camada aceita: %+v", first)
	}

	var receipt struct {
		Status string `gorm:"column:status"`
	}
	if err := database.DB().Table("command_decision_receipts").Select("status").Where("subject_id = ?", first.InvocationID).Take(&receipt).Error; err != nil {
		t.Fatal(err)
	}
	if receipt.Status != commanddecision.Consumed {
		t.Fatalf("recibo da decisão aceita = %q, want %q", receipt.Status, commanddecision.Consumed)
	}
	var ledger struct {
		SourceType              string `gorm:"column:source_type"`
		AuthorizationDecisionID string `gorm:"column:authorization_decision_id"`
	}
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", first.InvocationID).Take(&ledger).Error; err != nil {
		t.Fatal(err)
	}
	if ledger.SourceType != "chat" || ledger.AuthorizationDecisionID == "" {
		t.Fatalf("ledger chat/decision incompleto: %+v", ledger)
	}
	var claim struct {
		SourceType       string `gorm:"column:source_type"`
		SourceInstanceID string `gorm:"column:source_instance_id"`
	}
	if err := database.DB().Table("command_layer_activation_state").Where("user_id = ? AND rule_ref = ?", a.currentUserID, rule.ID).Take(&claim).Error; err != nil {
		t.Fatal(err)
	}
	inv, ok := invocationctx.Get(ctx)
	if !ok || claim.SourceType != "manual" || claim.SourceInstanceID != inv.ConversationID {
		t.Fatalf("origem da claim de camada: claim=%+v inv=%+v", claim, inv)
	}

	secondValue, err := (commandAgentTools{app: a}).Catalog(ctx, request)
	if err != nil {
		t.Fatalf("replay da camada: %v", err)
	}
	second := commandAgentExecutionJSON(t, secondValue)
	if second.InvocationID != first.InvocationID || second.Status != first.Status {
		t.Fatalf("replay da camada divergente: first=%+v second=%+v", first, second)
	}
	select {
	case unexpected := <-decisions:
		t.Fatalf("replay abriu decisão nova: %+v", unexpected)
	default:
	}
	var claims int64
	if err := database.DB().Table("command_layer_activation_state").Where("user_id = ? AND rule_ref = ? AND state = ?", a.currentUserID, rule.ID, "active").Count(&claims).Error; err != nil || claims != 1 {
		t.Fatalf("claims após replay=%d err=%v", claims, err)
	}

	deniedCtx := commandAgentTestContext(t, a, commandtool.CatalogName)
	denied := commandAgentExecuteWithDecision(t, a, decisions, deniedCtx, request, commanddecision.DenyAction)
	if denied.Status != string(commandledger.Denied) {
		t.Fatalf("ação de camada rejeitada: %+v", denied)
	}
	var deniedReceipt struct {
		Status string `gorm:"column:status"`
	}
	if err := database.DB().Table("command_decision_receipts").Select("status").Where("subject_id = ?", denied.InvocationID).Take(&deniedReceipt).Error; err != nil {
		t.Fatal(err)
	}
	if deniedReceipt.Status != commanddecision.Denied {
		t.Fatalf("recibo da decisão rejeitada = %q, want %q", deniedReceipt.Status, commanddecision.Denied)
	}
}

func commandAgentExecuteWithDecision(t *testing.T, a *App, decisions <-chan map[string]any, ctx context.Context, request commandtool.Request, action string) commandAgentExecutionResult {
	t.Helper()
	done := make(chan struct {
		value any
		err   error
	}, 1)
	go func() {
		value, err := (commandAgentTools{app: a}).Catalog(ctx, request)
		done <- struct {
			value any
			err   error
		}{value: value, err: err}
	}()
	var decision map[string]any
	select {
	case decision = <-decisions:
	case result := <-done:
		t.Fatalf("execução terminou antes da decisão: value=%v err=%v", result.value, result.err)
	case <-time.After(5 * time.Second):
		t.Fatal("ação do agente não apresentou decisão")
	}
	finishCommandDecision(t, a.questionnaireMgr, decision, map[string]any{questionnaire.AnswerActionID: action}, false)
	result := <-done
	if result.err != nil {
		t.Fatalf("execução após decisão: %v", result.err)
	}
	return commandAgentExecutionJSON(t, result.value)
}

type commandAgentExecutionResult struct {
	InvocationID string          `json:"invocationId"`
	Status       string          `json:"status"`
	CommandID    string          `json:"command_id"`
	Result       json.RawMessage `json:"result"`
}

func commandAgentExecutionJSON(t *testing.T, value any) commandAgentExecutionResult {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result commandAgentExecutionResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("execution JSON: %v (%s)", err, encoded)
	}
	return result
}

func TestCommandAgentDeniesInvalidCallers(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *App, context.Context)
	}{
		{name: "missing user and invocation context", mutate: func(t *testing.T, _ *App, _ context.Context) {}},
		{name: "foreign user", mutate: func(t *testing.T, _ *App, ctx context.Context) {
			_ = ctx
		}},
		{name: "wrong tool name", mutate: func(t *testing.T, _ *App, _ context.Context) {}},
		{name: "dry run", mutate: func(t *testing.T, _ *App, ctx context.Context) {
			id := toolinvocations.CurrentInvocationID(ctx)
			if err := database.DB().Model(&database.ToolInvocation{}).Where("id = ?", id).Update("dry_run", true).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "external channel", mutate: func(t *testing.T, ctxApp *App, ctx context.Context) {
			_ = ctxApp
			inv, ok := invocationctx.Get(ctx)
			if !ok {
				t.Fatal("invocation context ausente")
			}
			if err := database.DB().Model(&database.Conversation{}).Where("id = ?", inv.ConversationID).Update("channel", "telegram").Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "stopped invocation", mutate: func(t *testing.T, _ *App, ctx context.Context) {
			id := toolinvocations.CurrentInvocationID(ctx)
			if err := database.DB().Model(&database.ToolInvocation{}).Where("id = ?", id).Update("status", toolinvocations.StatusSucceeded).Error; err != nil {
				t.Fatal(err)
			}
		}},
		{name: "role outside product policy", mutate: func(t *testing.T, a *App, _ context.Context) {
			if err := database.DB().Model(&database.User{}).Where("id = ?", a.currentUserID).Update("role", "viewer").Error; err != nil {
				t.Fatal(err)
			}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			a := readyCommandProduct(t)
			toolName := commandtool.CatalogName
			if tc.name == "wrong tool name" {
				toolName = commandtool.ConfigName
			}
			ctx := commandAgentTestContext(t, a, toolName)
			tc.mutate(t, a, ctx)
			switch tc.name {
			case "missing user and invocation context":
				ctx = context.Background()
			case "foreign user":
				ctx = database.WithUserID(ctx, uuid.Must(uuid.NewV7()).String())
			}
			_, err := (commandAgentTools{app: a}).Catalog(ctx, commandtool.Request{Action: "list"})
			if !errors.Is(err, commandexecution.ErrDenied) {
				t.Fatalf("caller aceito: %v", err)
			}
		})
	}
}
