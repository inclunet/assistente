package app

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func paletteResolutionCandidate(t *testing.T) commandexecution.EnvelopeCandidate {
	t.Helper()
	candidate, err := commandPaletteCandidate(
		uuid.Must(uuid.NewV7()).String(), uuid.Must(uuid.NewV7()).String(),
		commandProductWorkspaceListID, json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatalf("construir candidato de paleta: %v", err)
	}
	return candidate
}

func installPaletteDelta(t *testing.T, a *App, reviewStatus, condition string) string {
	t.Helper()
	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("produto de comandos ausente")
	}
	projection, err := commandProductProjection(p.registry, nil)
	if err != nil || len(projection.BuiltinLayers) != 2 {
		t.Fatalf("projeção builtin inválida: layers=%d err=%v", len(projection.BuiltinLayers), err)
	}
	var paletteLayer commandconfig.BuiltinLayer
	for _, layer := range projection.BuiltinLayers {
		if layer.ID == commandPaletteLayerID {
			paletteLayer = layer
		}
	}
	if len(paletteLayer.Defaults) == 0 {
		t.Fatalf("defaults da paleta ausentes: %+v", projection.BuiltinLayers)
	}
	var defaultBinding commandbindings.Default
	for _, binding := range paletteLayer.Defaults {
		if binding.Candidate.CommandID == commandProductWorkspaceListID {
			defaultBinding = binding
			break
		}
	}
	if defaultBinding.Candidate.ID == "" {
		t.Fatal("default de workspace.list ausente")
	}
	rowID := uuid.Must(uuid.NewV7()).String()
	row := commandconfig.Binding{
		ID: rowID, UserID: p.principal.UserID, LayerRefKind: "builtin", LayerRef: "application.palette",
		TriggerType: string(commandcatalog.Palette), TriggerSpec: `{"version":1,"selection":"workspace.list"}`,
		Arguments: "{}", Condition: condition, Effect: "suppress", Enabled: true, Source: "user", ReviewStatus: reviewStatus,
		Presentation: `{"version":1}`,
	}
	row.ReplacesDefaultID = paletteStringPtr(defaultBinding.Candidate.ID)
	row.ReplacesDefaultVersion = paletteStringPtr(defaultBinding.Version)
	row.ReplacesDefaultFingerprint = paletteStringPtr(defaultBinding.Fingerprint)
	if err := database.DB().Create(&row).Error; err != nil {
		t.Fatalf("inserir delta persistido: %v", err)
	}
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
	return rowID
}

func paletteStringPtr(value string) *string { return &value }

func countPaletteRows(t *testing.T, table, invocationID string) int64 {
	t.Helper()
	var count int64
	if err := database.DB().Table(table).Where("invocation_id = ?", invocationID).Count(&count).Error; err != nil {
		t.Fatalf("contar %s: %v", table, err)
	}
	return count
}

func removePaletteDeltaAndRebuild(t *testing.T, a *App, bindingID string) {
	t.Helper()
	p := a.commandProduct.Load()
	if err := database.DB().Delete(&commandconfig.Binding{}, "id = ?", bindingID).Error; err != nil {
		t.Fatalf("remover delta persistido: %v", err)
	}
	var generation commandconfig.Generation
	if err := database.DB().Where("user_id = ? AND workspace_id IS NULL", p.principal.UserID).First(&generation).Error; err != nil {
		t.Fatalf("ler geração após remoção: %v", err)
	}
	if err := database.DB().Model(&commandconfig.Generation{}).Where("id = ?", generation.ID).Update("generation", generation.Generation+1).Error; err != nil {
		t.Fatalf("avançar geração após remoção: %v", err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(context.Background()); err != nil {
		t.Fatalf("rebuild após remoção: %v", err)
	}
}

func TestCommandProductResolutionPersistsSuppressAndKeepsDefaultReplay(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	candidate := paletteResolutionCandidate(t)
	first, err := p.execute(context.Background(), candidate)
	if err != nil || first.Status != commandledger.Succeeded || first.Envelope.CommandID == nil || *first.Envelope.CommandID != commandProductWorkspaceListID {
		t.Fatalf("default não executou workspace.list: status=%q command=%v err=%v", first.Status, first.Envelope.CommandID, err)
	}
	if len(first.Envelope.BindingIDs) != 1 || first.Envelope.BindingIDs[0] != "builtin.palette.workspace.list" {
		t.Fatalf("binding_ids do default inesperados: %v", first.Envelope.BindingIDs)
	}
	replay, err := p.execute(context.Background(), candidate)
	if err != nil || replay.ID != first.ID || replay.Status != commandledger.Succeeded {
		t.Fatalf("replay do trigger alterou o resultado: first=%+v replay=%+v err=%v", first, replay, err)
	}

	deltaID := installPaletteDelta(t, a, "active", `{"version":1,"clauses":[]}`)
	result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
	if err != nil || result.Status != string(commandledger.Suppressed) || result.Output != nil {
		t.Fatalf("delta suppress não foi aplicado: result=%+v err=%v", result, err)
	}
	if countPaletteRows(t, "command_invocations", result.InvocationID) != 0 || countPaletteRows(t, "command_idempotency_keys", result.InvocationID) != 1 {
		t.Fatalf("persistência de suppress incorreta para %s", result.InvocationID)
	}
	stored, err := a.GetPaletteInvocation(result.InvocationID)
	if err != nil || stored.Status != string(commandledger.Suppressed) || stored.Output != nil {
		t.Fatalf("lookup do suppress incorreto: result=%+v err=%v", stored, err)
	}
	if err := database.DB().Table("command_idempotency_keys").Where("invocation_id = ?", result.InvocationID).Update("source_type", "keyboard.local").Error; err != nil {
		t.Fatalf("alterar origem do marker para o teste de isolamento: %v", err)
	}
	if _, err := a.GetPaletteInvocation(result.InvocationID); err == nil {
		t.Fatal("lookup Palette expôs marker de outra origem")
	}
	var binding commandconfig.Binding
	if err := database.DB().First(&binding, "id = ?", deltaID).Error; err != nil || binding.ReviewStatus != "active" {
		t.Fatalf("delta não preservado: binding=%+v err=%v", binding, err)
	}
}

func TestCommandProductResolutionNeedsReviewAndUIConditionDoNotFallback(t *testing.T) {
	t.Run("needs_review", func(t *testing.T) {
		a := readyCommandProduct(t)
		installPaletteDelta(t, a, "needs_review", `{"version":1,"clauses":[]}`)
		result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
		if err == nil && result.Status == string(commandledger.Succeeded) || result.Output != nil {
			t.Fatalf("needs_review caiu no default: result=%+v err=%v", result, err)
		}
	})

	t.Run("condição de UI sem provider", func(t *testing.T) {
		a := readyCommandProduct(t)
		installPaletteDelta(t, a, "active", `{"version":1,"clauses":[{"field":"app.focused","op":"eq","value":true}]}`)
		result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
		if err == nil && result.Status == string(commandledger.Succeeded) || result.Output != nil {
			t.Fatalf("condição de UI sem provider caiu no default: result=%+v err=%v", result, err)
		}
	})
}

func TestCommandProductResolutionBridgeDoesNotBypassSuppress(t *testing.T) {
	a := readyCommandProduct(t)
	installPaletteDelta(t, a, "active", `{"version":1,"clauses":[]}`)
	p := a.commandProduct.Load()
	invocation := commandProductBridgeInvocation(p)
	ack, err := a.CommandBridgeInvoke(invocation, p.owner())
	if err != nil || !ack.Accepted || ack.InvocationID != invocation.InvocationID {
		t.Fatalf("bridge suppress não aceito: ack=%+v err=%v", ack, err)
	}
	waitCommandProductWorker(t, p)
	var statuses []string
	if err := database.DB().Table("command_idempotency_keys").Where("invocation_id = ?", invocation.InvocationID).Pluck("status", &statuses).Error; err != nil {
		t.Fatalf("ler status do bridge: %v", err)
	}
	if len(statuses) != 1 || statuses[0] != string(commandledger.Suppressed) {
		t.Fatalf("bridge bypassou suppress: statuses=%v", statuses)
	}
	if countPaletteRows(t, "command_invocations", invocation.InvocationID) != 0 {
		t.Fatal("bridge suppress criou command_invocations")
	}
}

func TestCommandProductResolutionReplaySurvivesDeltaRemoval(t *testing.T) {
	a := readyCommandProduct(t)
	deltaID := installPaletteDelta(t, a, "active", `{"version":1,"clauses":[]}`)
	p := a.commandProduct.Load()
	candidate := paletteResolutionCandidate(t)
	first, err := p.execute(context.Background(), candidate)
	if err != nil || first.Status != commandledger.Suppressed {
		t.Fatalf("primeiro trigger não foi suppressed: %+v err=%v", first, err)
	}
	removePaletteDeltaAndRebuild(t, a, deltaID)

	replay, err := p.execute(context.Background(), candidate)
	if err != nil || replay.ID != first.ID || replay.Status != commandledger.Suppressed {
		t.Fatalf("replay mudou após rebuild: first=%+v replay=%+v err=%v", first, replay, err)
	}
	newResult, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
	if err != nil || newResult.Status != string(commandledger.Succeeded) || newResult.Output == nil {
		t.Fatalf("nova invocação não voltou ao default: result=%+v err=%v", newResult, err)
	}
}

func TestCommandProductResolutionPersistsRejectedStaleAfterAuthenticatedOwnerChanges(t *testing.T) {
	a := readyCommandProduct(t)
	deltaID := installPaletteDelta(t, a, "active", `{"version":1,"clauses":[]}`)
	p := a.commandProduct.Load()
	originalID := a.currentUserID
	originalUser := *a.currentAuthUser
	var sessionQueries atomic.Int32
	var ownerChanged atomic.Bool
	hook := "test:command_product_resolution_owner_change"
	if err := database.DB().Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		queryShape := strings.ToLower(tx.Statement.Table + " " + tx.Statement.SQL.String())
		if !strings.Contains(queryShape, "sessions") || sessionQueries.Add(1) != 3 {
			return
		}
		a.authMu.Lock()
		a.currentUserID = uuid.Must(uuid.NewV7()).String()
		a.currentAuthUser = &AuthUser{UserID: a.currentUserID, SessionID: uuid.Must(uuid.NewV7()).String()}
		a.authMu.Unlock()
		ownerChanged.Store(true)
	}); err != nil {
		t.Fatal(err)
	}
	restoreOwner := func() {
		a.authMu.Lock()
		a.currentUserID = originalID
		a.currentAuthUser = &originalUser
		a.authMu.Unlock()
	}
	callbackRemoved := false
	defer func() {
		if !callbackRemoved {
			_ = database.DB().Callback().Query().Remove(hook)
		}
		restoreOwner()
	}()

	invocation := commandProductBridgeInvocation(p)
	ack, err := a.CommandBridgeInvoke(invocation, p.owner())
	if err != nil || !ack.Accepted {
		t.Fatalf("ingresso bridge para stale: ack=%+v err=%v", ack, err)
	}
	waitCommandProductWorker(t, p)
	if sessionQueries.Load() != 3 || !ownerChanged.Load() {
		t.Fatalf("barreira não atingiu a terceira autenticação: queries=%d changed=%v", sessionQueries.Load(), ownerChanged.Load())
	}
	restoreOwner()
	if err := database.DB().Callback().Query().Remove(hook); err != nil {
		t.Fatal(err)
	}
	callbackRemoved = true
	result, err := a.GetPaletteInvocation(invocation.InvocationID)
	if err != nil || result.Status != string(commandledger.RejectedStale) || result.Output != nil {
		t.Fatalf("owner alterado não persistiu rejected_stale: result=%+v err=%v queries=%d changed=%v", result, err, sessionQueries.Load(), ownerChanged.Load())
	}
	if countPaletteRows(t, "command_invocations", invocation.InvocationID) != 0 || countPaletteRows(t, "command_idempotency_keys", invocation.InvocationID) != 1 {
		t.Fatalf("rejected_stale criou auditoria indevida para %s", invocation.InvocationID)
	}
	removePaletteDeltaAndRebuild(t, a, deltaID)

	replay := invocation
	ack, err = a.CommandBridgeInvoke(replay, p.owner())
	if err != nil || !ack.Accepted {
		t.Fatalf("replay bridge do stale não aceito: ack=%+v err=%v", ack, err)
	}
	waitCommandProductWorker(t, p)
	stored, err := a.GetPaletteInvocation(invocation.InvocationID)
	if err != nil || stored.Status != string(commandledger.RejectedStale) || countPaletteRows(t, "command_invocations", invocation.InvocationID) != 0 || countPaletteRows(t, "command_idempotency_keys", invocation.InvocationID) != 1 {
		t.Fatalf("replay stale reabriu execução: stored=%+v err=%v", stored, err)
	}
	candidate, err := commandPaletteCandidate(invocation.InvocationID, invocation.InvocationID, commandProductWorkspaceListID, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	direct, err := p.execute(context.Background(), candidate)
	if err != nil || direct.Status != commandledger.RejectedStale {
		t.Fatalf("replay direto não expôs marker stale: direct=%+v err=%v", direct, err)
	}
	fresh, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
	if err != nil || fresh.Status != string(commandledger.Succeeded) || fresh.Output == nil || fresh.InvocationID == invocation.InvocationID {
		t.Fatalf("nova invocação após remoção não executou: fresh=%+v err=%v", fresh, err)
	}
}

func TestCommandProductResolutionDeniesLockAndRevokedSession(t *testing.T) {
	t.Run("lock", func(t *testing.T) {
		a := readyCommandProduct(t)
		if err := a.commandHost.SetVaultUnlocked(context.Background(), false); err != nil {
			t.Fatal(err)
		}
		result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
		if err == nil && result.Status == string(commandledger.Succeeded) || result.Output != nil {
			t.Fatalf("lock permitiu execução: result=%+v err=%v", result, err)
		}
	})

	t.Run("sessão revogada", func(t *testing.T) {
		a := readyCommandProduct(t)
		p := a.commandProduct.Load()
		if err := database.DB().Exec("UPDATE sessions SET revoked_at = CURRENT_TIMESTAMP WHERE id = ?", p.principal.SessionID).Error; err != nil {
			t.Fatal(err)
		}
		result, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
		if err == nil && result.Status == string(commandledger.Succeeded) || result.Output != nil {
			t.Fatalf("sessão revogada permitiu execução: result=%+v err=%v", result, err)
		}
	})
}
