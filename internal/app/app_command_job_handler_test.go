package app

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
	"assistente/internal/jobprofilegrant"
	"assistente/internal/jobs"
	"assistente/internal/profileaccess"
	"assistente/internal/profiles"
	"assistente/internal/tools"
	"github.com/google/uuid"
)

type appCommandJobHandlerTool struct{ calls atomic.Int32 }

func (t *appCommandJobHandlerTool) Name() string { return "app.command_handler_test_tool" }
func (t *appCommandJobHandlerTool) Description() string {
	return "tool controlada do teste de handler App"
}
func (t *appCommandJobHandlerTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}
func (t *appCommandJobHandlerTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	t.calls.Add(1)
	return tools.ToolResult{Content: `{"ok":true}`}, nil
}

func TestAppCommandJobHandlerUsesRealOwnerSessionAndFixedTarget(t *testing.T) {
	a := commandJobPublicationApp(t)
	ctx := database.WithUserID(context.Background(), a.currentUserID)
	tool := &appCommandJobHandlerTool{}
	a.toolRegistry.MustRegister(tool)
	toolID := uuid.Must(uuid.NewV7()).String()
	if err := database.DB().Create(&database.ToolCatalog{UUIDModel: database.UUIDModel{ID: toolID}, Name: tool.Name(), DisplayName: tool.Name(), Description: tool.Description(), Origin: "builtin", Schema: string(tool.Parameters()), AvailabilityStatus: "available"}).Error; err != nil {
		t.Fatal(err)
	}
	job := &jobs.Job{ID: "app-command-handler-regular", Name: "App command handler regular", Tool: tool.Name(), Inputs: map[string]any{}, Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}}, ErrorPolicy: jobs.ErrorPolicy{Strategy: jobs.ErrorStop}}
	if err := a.jobMgr.CreateJobContext(ctx, job); err != nil {
		t.Fatal(err)
	}
	loadedJob, err := jobs.NewDBRepository(database.DB()).GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	job = loadedJob
	definition := appCommandJobHandlerDefinition()
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Destructive, HasMutableTarget: true, Route: definition.HandlerRoute, Classification: commandcatalog.HandlerJob}
	handler, err := a.newCommandJobHandler(ctx, definition, contract, job.DatabaseID, definition.SensitivePaths)
	if err != nil {
		t.Fatal(err)
	}
	invocation := appCommandJobInvocation(t, a, definition.ID)
	handle, err := handler.Start(ctx, invocation)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	select {
	case outcome := <-handle.Done:
		if outcome.Status != commandledger.Succeeded {
			t.Fatalf("outcome=%+v", outcome)
		}
		var metadata map[string]string
		if err := json.Unmarshal(outcome.Result, &metadata); err != nil || metadata["job_id"] != job.DatabaseID || metadata["run_id"] == "" || metadata["status"] != jobs.RunStatusCompleted {
			t.Fatalf("metadata-only inválido: %s", outcome.Result)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("handler App excedeu o prazo")
	}
	if tool.calls.Load() != 1 {
		t.Fatalf("tool calls=%d", tool.calls.Load())
	}

	foreignCtx := database.WithUserID(context.Background(), uuid.Must(uuid.NewV7()).String())
	if _, err := a.newCommandJobHandler(foreignCtx, definition, contract, job.DatabaseID, commandcatalog.SensitivePaths{}); err == nil {
		t.Fatal("owner estrangeiro aceito pela fábrica")
	}

	if err := database.DB().Model(&database.Session{}).Where("id = ?", a.currentAuthUser.SessionID).Update("revoked_at", time.Now().UTC()).Error; err != nil {
		t.Fatal(err)
	}
	staleHandle, err := handler.Start(ctx, appCommandJobInvocation(t, a, definition.ID))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-staleHandle.Done:
		if outcome.Status == commandledger.Succeeded {
			t.Fatal("sessão revogada executou o job")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("sessão revogada não foi recusada no prazo")
	}
	if tool.calls.Load() != 1 {
		t.Fatalf("tool executou após revogação: %d", tool.calls.Load())
	}
}

func TestAppCommandJobHandlerRejectsMissingAndStaleProfileGrant(t *testing.T) {
	a := commandJobPublicationApp(t)
	ctx := database.WithUserID(context.Background(), a.currentUserID)
	a.profileManager = profiles.NewManager()
	if err := a.profileManager.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	profile := profiles.DefaultProfile()
	profile.Name = "Handler Grant Target"
	targetSlug, err := a.profileManager.Create(profile)
	if err != nil {
		t.Fatal(err)
	}
	a.jobGrantStore = jobprofilegrant.NewStore(database.DB())
	a.profileAccess = profileaccess.NewService(a.profileManager, nil, nil, func(context.Context, *profiles.Profile) bool { return true }).WithJobGrants(a.jobGrantStore)
	toolID := uuid.Must(uuid.NewV7()).String()
	if err := database.DB().Create(&database.ToolCatalog{UUIDModel: database.UUIDModel{ID: toolID}, Name: jobprofilegrant.ToolSubagent, DisplayName: jobprofilegrant.ToolSubagent, Description: "subagent de teste", Origin: "builtin", Schema: `{"type":"object"}`, AvailabilityStatus: "available"}).Error; err != nil {
		t.Fatal(err)
	}

	job := &jobs.Job{ID: "app-command-handler-subagent", Name: "App command handler subagent", Tool: jobprofilegrant.ToolSubagent, Inputs: map[string]any{"profile": targetSlug}, Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}}, ErrorPolicy: jobs.ErrorPolicy{Strategy: jobs.ErrorStop}}
	if err := a.jobMgr.CreateJobContext(ctx, job); err != nil {
		t.Fatal(err)
	}
	job, err = jobs.NewDBRepository(database.DB()).GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	definition := appCommandJobHandlerDefinition()
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Destructive, HasMutableTarget: true, Route: definition.HandlerRoute, Classification: commandcatalog.HandlerJob}
	if _, err := a.newCommandJobHandler(ctx, definition, contract, job.DatabaseID, commandcatalog.SensitivePaths{}); err == nil {
		t.Fatal("grant ausente permitiu montar handler")
	}

	grantStore := a.jobGrantStore
	fingerprint := jobprofilegrant.Fingerprint(jobprofilegrant.ToolSubagent, targetSlug)
	snapshot, err := grantStore.AuthorizationSnapshot(ctx, job.DatabaseID, targetSlug)
	if err != nil {
		t.Fatal(err)
	}
	if err := grantStore.Grant(ctx, job.DatabaseID, targetSlug, fingerprint, "test", snapshot.Generation); err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Model(&database.Job{}).Where("id = ? AND user_id = ?", job.DatabaseID, a.currentUserID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	job, err = jobs.NewDBRepository(database.DB()).GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	originalFingerprint, err := jobs.DefinitionFingerprint(job)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := a.newCommandJobHandler(ctx, definition, contract, job.DatabaseID, commandcatalog.SensitivePaths{})
	if err != nil {
		t.Fatal(err)
	}
	if err := grantStore.Revoke(ctx, job.DatabaseID, targetSlug, "test-revoke"); err != nil {
		t.Fatal(err)
	}
	latest, err := grantStore.AuthorizationSnapshot(ctx, job.DatabaseID, targetSlug)
	if err != nil {
		t.Fatal(err)
	}
	if err := grantStore.Grant(ctx, job.DatabaseID, targetSlug, fingerprint, "test-regrant", latest.Generation); err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Model(&database.Job{}).Where("id = ? AND user_id = ?", job.DatabaseID, a.currentUserID).Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	job, err = jobs.NewDBRepository(database.DB()).GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := jobs.DefinitionFingerprint(job); err != nil || got != originalFingerprint {
		t.Fatalf("fingerprint do alvo mudou após grant: got=%s want=%s err=%v", got, originalFingerprint, err)
	}

	handle, err := handler.Start(ctx, appCommandJobInvocation(t, a, definition.ID))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-handle.Done:
		if outcome.Status == commandledger.Succeeded {
			t.Fatal("regrant posterior revalidou handler antigo")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("grant stale não foi recusado no prazo")
	}
	var runs int64
	if err := database.DB().Model(&database.JobRun{}).Where("user_id = ? AND job_id = ?", a.currentUserID, job.DatabaseID).Count(&runs).Error; err != nil {
		t.Fatal(err)
	}
	if runs != 0 {
		t.Fatalf("grant stale criou JobRun: %d", runs)
	}
}

func appCommandJobInvocation(t *testing.T, a *App, commandID string) commandexecution.Invocation {
	t.Helper()
	ctx := database.WithUserID(context.Background(), a.currentUserID)
	epoch, err := a.commandEpochs.Capture(ctx, a.currentUserID, a.currentAuthUser.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	decisionID := uuid.Must(uuid.NewV7()).String()
	invocationID := uuid.Must(uuid.NewV7()).String()
	userID := a.currentUserID
	source := commandcontract.SourcePalette
	provenance := json.RawMessage(`{"_chain_id":"` + invocationID + `","_chain_history":[],"command_chain_history":[{"command_id":"` + commandID + `","invocation_id":"` + invocationID + `","layer_refs":[]}]}`)
	return commandexecution.Invocation{ID: invocationID, CorrelationID: invocationID, CommandID: commandID, Principal: auth.LocalSessionPrincipal{UserID: userID, SessionID: a.currentAuthUser.SessionID}, Source: commandcatalog.Palette, Envelope: &commandcontract.Envelope{Version: 1, InvocationID: invocationID, CommandID: &commandID, UserID: &userID, AuthContextType: commandcontract.AuthLocalSession, AuthContextID: a.currentAuthUser.SessionID, AuthGeneration: epoch.AuthGeneration, SecurityGeneration: epoch.SecurityGeneration, AuthorizationDecisionID: &decisionID, ActorType: commandcontract.ActorUser, ActorID: userID, SourceType: &source, BindingIDs: []string{}, RegistryVersion: commandProductRegistryVersion, Provenance: &provenance, CorrelationID: invocationID, ReceivedAt: time.Now().UTC()}}
}

func appCommandJobHandlerDefinition() commandcatalog.Definition {
	locales := map[string]commandcatalog.LocalizedMetadata{"pt-BR": {Name: "Delegar job", Description: "Executa um job", Category: "Testes"}, "en": {Name: "Delegate job", Description: "Run a job", Category: "Tests"}, "es": {Name: "Delegar job", Description: "Ejecuta un job", Category: "Pruebas"}}
	result := &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{"job_id": {Type: commandcatalog.SchemaString}, "run_id": {Type: commandcatalog.SchemaString}, "status": {Type: commandcatalog.SchemaString}}, Required: []string{"job_id", "run_id", "status"}}
	return commandcatalog.Definition{ID: "app.command_handler_test", Effect: commandcatalog.Destructive, Decision: commandcatalog.Interactive, HasMutableTarget: true, AllowedSources: []commandcatalog.Source{commandcatalog.Palette}, Context: commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active_tab", Mode: commandcatalog.ExactVersion}}}, Presentation: &commandcatalog.Presentation{Version: "1", Locales: locales}, ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, ResultSchema: result, Risk: commandcatalog.RiskHigh, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted}, Scopes: []commandcatalog.Scope{commandcatalog.ScopeWorkspace}, Availability: commandcatalog.Availability{Status: commandcatalog.Available}, HandlerRoute: "app/test-command-handler", HandlerClassification: commandcatalog.HandlerJob}
}
