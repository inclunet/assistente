package app

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
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

const dynamicProfileExpression = `{{ "dynamic-target" }}`

type appDynamicProfileCounterTool struct {
	calls atomic.Int32
}

func (t *appDynamicProfileCounterTool) Name() string { return jobprofilegrant.ToolSubagent }

func (*appDynamicProfileCounterTool) Description() string {
	return "contador real para job com profile dinâmico"
}

func (*appDynamicProfileCounterTool) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t *appDynamicProfileCounterTool) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	t.calls.Add(1)
	return tools.ToolResult{Content: `{"ok":true}`}, nil
}

func appDynamicProfileFixture(t *testing.T, expression string, grant bool) (*App, context.Context, *jobs.Job, string, *appDynamicProfileCounterTool) {
	t.Helper()
	a := commandJobPublicationApp(t)
	ctx := database.WithUserID(context.Background(), a.currentUserID)
	a.profileManager = profiles.NewManager()
	if err := a.profileManager.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	profile := profiles.DefaultProfile()
	profile.Name = "Dynamic Target"
	targetSlug, err := a.profileManager.Create(profile)
	if err != nil {
		t.Fatal(err)
	}
	if targetSlug != "dynamic-target" {
		t.Fatalf("profile slug = %q, want dynamic-target", targetSlug)
	}
	a.jobGrantStore = jobprofilegrant.NewStore(database.DB())
	a.profileAccess = profileaccess.NewService(a.profileManager, nil, nil, func(context.Context, *profiles.Profile) bool { return true }).WithJobGrants(a.jobGrantStore)
	tool := &appDynamicProfileCounterTool{}
	a.toolRegistry.MustRegister(tool)
	if err := database.DB().Create(&database.ToolCatalog{
		UUIDModel: database.UUIDModel{ID: uuid.NewString()}, Name: tool.Name(), DisplayName: tool.Name(),
		Description: tool.Description(), Origin: "builtin", Schema: string(tool.Parameters()), AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatal(err)
	}
	job := &jobs.Job{
		ID:          "app-command-dynamic-profile",
		Name:        "App command dynamic profile",
		Tool:        jobprofilegrant.ToolSubagent,
		Inputs:      map[string]any{"profile": expression},
		Triggers:    []jobs.Trigger{{Type: jobs.TriggerManual}},
		ErrorPolicy: jobs.ErrorPolicy{Strategy: jobs.ErrorStop},
	}
	if err := a.jobMgr.CreateJobContext(ctx, job); err != nil {
		t.Fatal(err)
	}
	job, err = jobs.NewDBRepository(database.DB()).GetJob(ctx, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	if grant {
		snapshot, err := a.jobGrantStore.AuthorizationSnapshot(ctx, job.DatabaseID, targetSlug)
		if err != nil {
			t.Fatal(err)
		}
		fingerprint := jobprofilegrant.Fingerprint(jobprofilegrant.ToolSubagent, expression)
		if snapshot.Config.Fingerprint != fingerprint {
			t.Fatalf("grant fingerprint = %s, want %s", snapshot.Config.Fingerprint, fingerprint)
		}
		if err := a.jobGrantStore.Grant(ctx, job.DatabaseID, targetSlug, fingerprint, "test", snapshot.Generation); err != nil {
			t.Fatal(err)
		}
	}
	if grant {
		job.Enabled = true
		if err := a.jobMgr.SaveJobContext(ctx, job); err != nil {
			t.Fatal(err)
		}
	} else if err := database.DB().Model(&database.Job{}).
		Where("id = ? AND user_id = ?", job.DatabaseID, a.currentUserID).
		Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	return a, ctx, job, targetSlug, tool
}

func appDynamicProfileHandler(t *testing.T, a *App, ctx context.Context, job *jobs.Job) commandexecution.Handler {
	t.Helper()
	definition := appCommandJobHandlerDefinition()
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Destructive, HasMutableTarget: true, Route: definition.HandlerRoute, Classification: commandcatalog.HandlerJob}
	handler, err := a.newCommandJobHandler(ctx, definition, contract, job.DatabaseID, definition.SensitivePaths)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func appDynamicProfileOutcome(t *testing.T, handler commandexecution.Handler, a *App, commandID string) commandexecution.Outcome {
	t.Helper()
	handle, err := handler.Start(database.WithUserID(context.Background(), a.currentUserID), appCommandJobInvocation(t, a, commandID))
	if err != nil {
		t.Fatal(err)
	}
	select {
	case outcome := <-handle.Done:
		return outcome
	case <-time.After(10 * time.Second):
		t.Fatal("job com profile dinâmico não encerrou")
		return commandexecution.Outcome{}
	}
}

func TestAppCommandJobDynamicProfileExpressionRunsGrantedTarget(t *testing.T) {
	a, ctx, job, targetSlug, tool := appDynamicProfileFixture(t, dynamicProfileExpression, true)
	handler := appDynamicProfileHandler(t, a, ctx, job)
	outcome := appDynamicProfileOutcome(t, handler, a, appCommandJobHandlerDefinition().ID)
	if outcome.Status != commandledger.Succeeded {
		t.Fatalf("outcome=%+v", outcome)
	}
	if got := tool.calls.Load(); got != 1 {
		t.Fatalf("tool calls=%d, want 1 for target %s", got, targetSlug)
	}
}

func TestAppCommandJobDynamicProfileExpressionRequiresGrant(t *testing.T) {
	a, ctx, job, _, tool := appDynamicProfileFixture(t, dynamicProfileExpression, false)
	definition := appCommandJobHandlerDefinition()
	contract := commandcatalog.HandlerContract{Effect: commandcatalog.Destructive, HasMutableTarget: true, Route: definition.HandlerRoute, Classification: commandcatalog.HandlerJob}
	if _, err := a.newCommandJobHandler(ctx, definition, contract, job.DatabaseID, definition.SensitivePaths); err == nil {
		t.Fatal("handler foi montado sem grant do profile resolvido")
	}
	if got := tool.calls.Load(); got != 0 {
		t.Fatalf("tool calls=%d sem grant", got)
	}
}

func TestAppCommandJobDynamicProfileRevocationRejectsOldHandlerWithoutRegrant(t *testing.T) {
	a, ctx, job, targetSlug, tool := appDynamicProfileFixture(t, dynamicProfileExpression, true)
	oldHandler := appDynamicProfileHandler(t, a, ctx, job)
	if err := a.jobGrantStore.Revoke(ctx, job.DatabaseID, targetSlug, "test-revoke"); err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Model(&database.Job{}).
		Where("id = ? AND user_id = ?", job.DatabaseID, a.currentUserID).
		Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	outcome := appDynamicProfileOutcome(t, oldHandler, a, appCommandJobHandlerDefinition().ID)
	if outcome.Status == commandledger.Succeeded {
		t.Fatalf("handler antigo aceitou grant revogado: %+v", outcome)
	}
	if got := tool.calls.Load(); got != 0 {
		t.Fatalf("tool alcançada após revogação: %d", got)
	}
}

func TestAppCommandJobDynamicProfileRegrantRejectsOldHandlerAndNewHandlerRuns(t *testing.T) {
	a, ctx, job, targetSlug, tool := appDynamicProfileFixture(t, dynamicProfileExpression, true)
	oldHandler := appDynamicProfileHandler(t, a, ctx, job)
	fingerprint := jobprofilegrant.Fingerprint(jobprofilegrant.ToolSubagent, dynamicProfileExpression)
	if err := a.jobGrantStore.Revoke(ctx, job.DatabaseID, targetSlug, "test-revoke"); err != nil {
		t.Fatal(err)
	}
	latest, err := a.jobGrantStore.AuthorizationSnapshot(ctx, job.DatabaseID, targetSlug)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.jobGrantStore.Grant(ctx, job.DatabaseID, targetSlug, fingerprint, "test-regrant", latest.Generation); err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Model(&database.Job{}).
		Where("id = ? AND user_id = ?", job.DatabaseID, a.currentUserID).
		Update("enabled", true).Error; err != nil {
		t.Fatal(err)
	}
	oldOutcome := appDynamicProfileOutcome(t, oldHandler, a, appCommandJobHandlerDefinition().ID)
	if oldOutcome.Status == commandledger.Succeeded {
		t.Fatalf("handler antigo aceitou regrant: %+v", oldOutcome)
	}
	if got := tool.calls.Load(); got != 0 {
		t.Fatalf("handler antigo alcançou tool após regrant: %d", got)
	}

	newHandler := appDynamicProfileHandler(t, a, ctx, job)
	newOutcome := appDynamicProfileOutcome(t, newHandler, a, appCommandJobHandlerDefinition().ID)
	if newOutcome.Status != commandledger.Succeeded {
		t.Fatalf("handler novo não executou após regrant: %+v", newOutcome)
	}
	if got := tool.calls.Load(); got != 1 {
		t.Fatalf("tool calls=%d, want 1 somente no handler novo", got)
	}
}
