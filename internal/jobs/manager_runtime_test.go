package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"
)

func TestManagerPipelineStateControlsRuntimeWithoutOverwritingJobEnabled(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SavePipeline(userA, &Pipeline{Slug: "ops", Name: "Ops", Enabled: true}); err != nil {
		t.Fatalf("save pipeline: %v", err)
	}
	job := testRepositoryJob("sync-jira", "Sync Jira")
	job.Pipeline = "ops"
	job.Triggers = []Trigger{{Type: TriggerInterval, Every: "1h"}}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	job2 := testRepositoryJob("sync-github", "Sync GitHub")
	job2.Pipeline = "ops"
	job2.Triggers = []Trigger{{Type: TriggerInterval, Every: "1h"}}
	if err := repo.SaveJob(userA, job2); err != nil {
		t.Fatalf("save second job: %v", err)
	}
	var emitted []map[string]any

	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
		EmitEvent: func(event string, data any) {
			if event != "jobs:updated" {
				return
			}
			payload, ok := data.(map[string]any)
			if !ok {
				t.Fatalf("unexpected event payload: %#v", data)
			}
			emitted = append(emitted, payload)
		},
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(mgr.Stop)
	if got := mgr.scheduler.ScheduledJobs(); len(got) != 2 {
		t.Fatalf("scheduled jobs after start: got %#v, want 2 jobs", got)
	}

	if err := mgr.SavePipelineContext(userA, &Pipeline{Slug: "ops", Name: "Ops", Enabled: false}); err != nil {
		t.Fatalf("disable pipeline: %v", err)
	}
	if got := mgr.scheduler.ScheduledJobs(); len(got) != 0 {
		t.Fatalf("scheduled jobs after disabling pipeline: got %#v, want none", got)
	}
	gotJob, err := mgr.GetJobContext(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if !gotJob.Enabled {
		t.Fatal("pipeline state overwrote job-level enabled flag")
	}
	if gotJob.PipelineEnabled {
		t.Fatal("expected runtime pipeline state to be disabled")
	}
	infos, err := mgr.GetJobsContext(userA)
	if err != nil {
		t.Fatalf("get job infos: %v", err)
	}
	if len(infos) != 2 {
		t.Fatalf("get job infos: got %#v, want 2 jobs", infos)
	}
	for _, info := range infos {
		if info.EffectiveEnabled || info.PipelineEnabled {
			t.Fatalf("job info did not expose effective disabled pipeline state: %#v", infos)
		}
	}
	if len(emitted) != 1 {
		t.Fatalf("pipeline disable emitted redundant updates: %#v", emitted)
	}
	ids, ok := emitted[0]["ids"].([]string)
	if !ok || len(ids) != 2 {
		t.Fatalf("pipeline disable did not emit affected job ids: %#v", emitted)
	}

	if err := mgr.SavePipelineContext(userA, &Pipeline{Slug: "ops", Name: "Ops", Enabled: true}); err != nil {
		t.Fatalf("enable pipeline: %v", err)
	}
	if got := mgr.scheduler.ScheduledJobs(); len(got) != 2 {
		t.Fatalf("scheduled jobs after enabling pipeline: got %#v, want 2 jobs", got)
	}
	if len(emitted) != 2 {
		t.Fatalf("pipeline enable emitted redundant updates: %#v", emitted)
	}

	if err := mgr.SavePipelineContext(userA, &Pipeline{Slug: "ops", Name: "Ops", Enabled: false}); err != nil {
		t.Fatalf("disable pipeline again: %v", err)
	}
	if err := mgr.DeletePipelineContext(userA, "ops"); err != nil {
		t.Fatalf("delete pipeline: %v", err)
	}
	if got := mgr.scheduler.ScheduledJobs(); len(got) != 2 {
		t.Fatalf("scheduled jobs after deleting disabled pipeline: got %#v, want 2 jobs", got)
	}
	gotJob, err = mgr.GetJobContext(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job after delete: %v", err)
	}
	if gotJob.Pipeline != "" || !gotJob.PipelineEnabled {
		t.Fatalf("pipeline state after delete: pipeline=%q pipelineEnabled=%v", gotJob.Pipeline, gotJob.PipelineEnabled)
	}
	if len(emitted) != 4 {
		t.Fatalf("pipeline delete emitted unexpected updates: %#v", emitted)
	}
}

func TestManagerGetToolCatalogIncludesDiscoverableOptIn(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegisterOptIn(&fakeTool{
		name:   "hidden_context_tool",
		params: json.RawMessage(`{"type":"object"}`),
	})
	registry.MustRegisterDiscoverableOptIn(&fakeTool{
		name:   "job",
		params: json.RawMessage(`{"type":"object"}`),
	})

	mgr := NewManager(ManagerConfig{ToolRegistry: registry})
	catalog, err := mgr.GetToolCatalog()
	if err != nil {
		t.Fatalf("get catalog: %v", err)
	}

	var foundJob, foundHidden bool
	for _, entry := range catalog {
		switch entry.Name {
		case "job":
			foundJob = true
		case "hidden_context_tool":
			foundHidden = true
		}
	}
	if !foundJob {
		t.Fatalf("discoverable opt-in tool missing from catalog: %#v", catalog)
	}
	if foundHidden {
		t.Fatalf("non-discoverable opt-in tool leaked into catalog: %#v", catalog)
	}
}

func TestManagerGetToolCatalogDerivesMCPRegistryMetadata(t *testing.T) {
	registry := tools.NewRegistry()
	registry.MustRegister(&fakeTool{
		name:   "mcp_jira__issue__delete",
		params: json.RawMessage(`{"type":"object"}`),
	})
	registry.MustRegister(&fakeTool{
		name:   "mcp_native__filesystem",
		params: json.RawMessage(`{"type":"object"}`),
	})

	mgr := NewManager(ManagerConfig{ToolRegistry: registry})
	catalog, err := mgr.GetToolCatalog()
	if err != nil {
		t.Fatalf("get catalog: %v", err)
	}

	entries := map[string]CatalogEntry{}
	for _, entry := range catalog {
		entries[entry.Name] = entry
	}
	bridge := entries["mcp_jira__issue__delete"]
	if bridge.Source != "mcp" || bridge.Origin != tools.ToolOriginMCPBridge || bridge.Risk != "" {
		t.Fatalf("unexpected MCP bridge metadata: %#v", bridge)
	}
	native := entries["mcp_native__filesystem"]
	if native.Source != "mcp" || native.Origin != tools.ToolOriginMCPNative || native.Risk != "" {
		t.Fatalf("unexpected MCP native metadata: %#v", native)
	}
}

func TestManagerGetToolCatalogBackfillsEmptyMCPSchemaFromRegistry(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	userID := "user-a"
	serverID := "srv-1"
	toolName := "mcp_jira__issue__delete"
	liveSchema := json.RawMessage(`{"type":"object","properties":{"issue_key":{"type":"string"}}}`)
	if err := repo.db.Create(&database.MCPServer{
		UUIDModel: database.UUIDModel{ID: serverID},
		UserID:    userID,
		Slug:      "jira",
		Name:      "Jira",
		Transport: "stdio",
		Enabled:   true,
	}).Error; err != nil {
		t.Fatalf("seed MCP server: %v", err)
	}
	if err := repo.db.Create(&database.ToolCatalog{
		UUIDModel:          database.UUIDModel{ID: "tool-1"},
		MCPServerID:        &serverID,
		Name:               toolName,
		DisplayName:        "issue__delete",
		Origin:             tools.ToolOriginMCPBridge,
		Risk:               "network",
		Schema:             `{}`,
		AvailabilityStatus: tools.ToolAvailabilityAvailable,
	}).Error; err != nil {
		t.Fatalf("seed MCP tool: %v", err)
	}

	registry := tools.NewRegistry()
	registry.MustRegister(&fakeTool{name: toolName, params: liveSchema})
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ToolRegistry:    registry,
		ContextProvider: func() context.Context { return userA },
	})

	catalog, err := mgr.GetToolCatalog()
	if err != nil {
		t.Fatalf("get catalog: %v", err)
	}
	for _, entry := range catalog {
		if entry.Name == toolName {
			if string(entry.Schema) != string(liveSchema) {
				t.Fatalf("expected live schema backfill, got %s", entry.Schema)
			}
			return
		}
	}
	t.Fatalf("expected MCP tool in catalog: %#v", catalog)
}

func TestManagerSaveJobBeforeStartDoesNotScheduleInterval(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})

	job := testRepositoryJob("prestart", "Prestart")
	job.Triggers = []Trigger{{Type: TriggerInterval, Every: "1h"}}
	if err := mgr.SaveJobContext(userA, job); err != nil {
		t.Fatalf("save job before start: %v", err)
	}

	if got := mgr.scheduler.ScheduledJobs(); len(got) != 0 {
		t.Fatalf("job saved before manager start scheduled triggers: %#v", got)
	}
	mgr.Stop()
}

func TestManagerExecuteJobSkipsAutomaticMCPRunWhenToolUnavailable(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ToolRegistry:    tools.NewRegistry(),
		ContextProvider: func() context.Context { return userA },
	})
	job := testRepositoryJob("mcp-job", "MCP Job")
	job.Triggers = []Trigger{{Type: TriggerInterval, Every: "1m"}}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	job.Tool = "mcp_jira__create_issue"
	mgr.registry.Set(job)

	mgr.executeJob(context.Background(), job, &TriggerContext{Type: TriggerInterval, Every: "1m"})
	runs, err := repo.GetRuns(userA, "mcp-job", 1)
	if err != nil {
		t.Fatalf("get runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected skipped run to be persisted, got %#v", runs)
	}
	if runs[0].Status != "skipped" || runs[0].Replayable {
		t.Fatalf("unexpected skipped run: %#v", runs[0])
	}
	if !strings.Contains(runs[0].Error, "not available") {
		t.Fatalf("skipped run did not record reason: %#v", runs[0])
	}
}

// TestEventTriggeredRunSurvivesPublisherContextCancellation reproduz o bug #164:
// um run disparado por evento herdava o ctx do publicador, que é cancelado logo
// após o publicador retornar (o fan-out do EventBus é assíncrono). A tool MCP
// downstream morria com "context canceled" em poucos ms. Com o desacoplamento do
// lifetime (context.WithoutCancel no subscriber), o run downstream deve completar.
func TestEventTriggeredRunSurvivesPublisherContextCancellation(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	registry := tools.NewRegistry()
	blocking := &fakeBlockingTool{
		name:     "test_tool",
		delay:    120 * time.Millisecond,
		response: `{"ok":true}`,
	}
	registry.MustRegister(blocking)

	job := testRepositoryJob("downstream", "Downstream")
	job.Tool = "test_tool"
	job.Triggers = []Trigger{{Type: TriggerEvent, Listen: "upstream.success"}}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ToolRegistry:    registry,
		ContextProvider: func() context.Context { return userA },
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(mgr.Stop)

	// Simula o publicador: um ctx de escopo de requisição (com user_id) que é
	// cancelado imediatamente após publicar, como acontece com a tool `job` ou
	// um interval em hot reload.
	pubCtx, cancel := context.WithCancel(userA)
	mgr.eventBus.Publish(pubCtx, "upstream.success", map[string]any{"id": "evt-1"})
	cancel()

	// Aguarda o run downstream ser persistido com status terminal.
	var run *RunLog
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		runs, err := repo.GetRuns(userA, "downstream", 1)
		if err != nil {
			t.Fatalf("get runs: %v", err)
		}
		if len(runs) == 1 && runs[0].Status != "" {
			run = &runs[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	if run == nil {
		t.Fatal("downstream run was never persisted")
	}
	if blocking.wasCanceled() {
		t.Fatalf("tool received a canceled context (run inherited publisher lifetime): %#v", run)
	}
	if !blocking.didFinish() {
		t.Fatal("tool did not run to completion")
	}
	if run.Status != "completed" {
		t.Fatalf("downstream run status = %q (error: %q), want completed", run.Status, run.Error)
	}
}

func TestManagerGetJobContextReturnsCopy(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("sync-jira", "Sync Jira")
	job.Inputs = map[string]any{"query": "original"}
	job.Tags = []string{"ops"}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	if err := mgr.Start(); err != nil {
		t.Fatalf("start manager: %v", err)
	}
	t.Cleanup(mgr.Stop)

	got, err := mgr.GetJobContext(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	got.Tool = "missing_tool"
	got.Inputs["query"] = "mutated"
	got.Tags[0] = "mutated"
	got.Triggers[0].Type = TriggerCron

	again, err := mgr.GetJobContext(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job again: %v", err)
	}
	if again.Tool != "test_tool" {
		t.Fatalf("registry job was mutated through returned pointer: got %q", again.Tool)
	}
	if again.Inputs["query"] == "mutated" {
		t.Fatalf("registry inputs map was mutated through returned pointer: %#v", again.Inputs)
	}
	if again.Tags[0] == "mutated" {
		t.Fatalf("registry tags slice was mutated through returned pointer: %#v", again.Tags)
	}
	if again.Triggers[0].Type == TriggerCron {
		t.Fatalf("registry triggers slice was mutated through returned pointer: %#v", again.Triggers)
	}
}

func TestManagerStopResetsCircuitBreakerState(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	mgr.circuitBreaker.SetMaxRunsPerHour(1)
	mgr.circuitBreaker.RecordRun("shared-job-id")
	if err := mgr.circuitBreaker.CheckRateLimit("shared-job-id", 0); err == nil {
		t.Fatal("expected recorded run to trip rate limit before reset")
	}

	mgr.Stop()
	if err := mgr.circuitBreaker.CheckRateLimit("shared-job-id", 0); err != nil {
		t.Fatalf("rate limit state leaked after stop: %v", err)
	}
}

func TestManagerContextFromUsesManagerUserScope(t *testing.T) {
	repo, userA, userB := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})

	scoped := mgr.contextFrom(userB)
	got, ok := database.UserIDFromContext(scoped)
	if !ok {
		t.Fatal("expected scoped context to keep a user")
	}
	want, _ := database.UserIDFromContext(userA)
	if got != want {
		t.Fatalf("contextFrom did not bind to manager user: got %q, want %q", got, want)
	}
}

func TestManagerGetJobEventsReturnsFullDay(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	day := time.Date(2026, 5, 13, 12, 0, 0, 0, time.Local)
	for i := 0; i < 505; i++ {
		entry := &EventEntry{
			ID:        fmt.Sprintf("event-%03d", i),
			Timestamp: day.Add(time.Duration(i) * time.Second),
			Type:      "info",
			Message:   "event",
		}
		if err := repo.LogEvent(userA, entry); err != nil {
			t.Fatalf("log event %d: %v", i, err)
		}
	}

	events, err := mgr.GetJobEvents("2026-05-13")
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 505 {
		t.Fatalf("GetJobEvents truncated full day: got %d, want 505", len(events))
	}
}

func TestManagerGetJobEventsEmptyDateDefaultsToToday(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := NewManager(ManagerConfig{
		Repository:      repo,
		ContextProvider: func() context.Context { return userA },
	})
	today := time.Now().In(time.Local)
	entries := []*EventEntry{
		{ID: "today-event", Timestamp: today, Type: "info", Message: "today"},
		{ID: "old-event", Timestamp: today.AddDate(0, 0, -1), Type: "info", Message: "old"},
	}
	for _, entry := range entries {
		if err := repo.LogEvent(userA, entry); err != nil {
			t.Fatalf("log event %s: %v", entry.ID, err)
		}
	}

	events, err := mgr.GetJobEvents("")
	if err != nil {
		t.Fatalf("get events: %v", err)
	}
	if len(events) != 1 || events[0].ID != "today-event" {
		t.Fatalf("empty date should return only today's events, got %#v", events)
	}
}
