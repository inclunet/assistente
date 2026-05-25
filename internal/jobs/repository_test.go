package jobs

import (
	"context"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupJobsRepositoryTest(t *testing.T) (*DBRepository, context.Context, context.Context) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&database.User{},
		&database.MCPServer{},
		&database.ToolCatalog{},
		&database.Tag{},
		&database.TagAssignment{},
		&database.JobPipeline{},
		&database.Job{},
		&database.JobTrigger{},
		&database.JobRun{},
		&database.JobEvent{},
		&database.JobRunEvent{},
	); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	previous := database.DB()
	database.SetDB(db)
	t.Cleanup(func() {
		database.SetDB(previous)
	})
	if err := db.Create(&database.ToolCatalog{
		Name:               "test_tool",
		DisplayName:        "test_tool",
		Description:        "Test tool",
		Origin:             "builtin",
		AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatalf("seed tool catalog: %v", err)
	}
	return NewDBRepository(db), database.WithUserID(context.Background(), "user-a"), database.WithUserID(context.Background(), "user-b")
}

func TestDBRepositoryJobsAreScopedByUser(t *testing.T) {
	repo, userA, userB := setupJobsRepositoryTest(t)

	jobA := testRepositoryJob("daily-report", "A")
	if err := repo.SaveJob(userA, jobA); err != nil {
		t.Fatalf("save user A: %v", err)
	}
	jobB := testRepositoryJob("daily-report", "B")
	if err := repo.SaveJob(userB, jobB); err != nil {
		t.Fatalf("save user B: %v", err)
	}

	gotA, err := repo.GetJob(userA, "daily-report")
	if err != nil {
		t.Fatalf("get user A: %v", err)
	}
	gotB, err := repo.GetJob(userB, "daily-report")
	if err != nil {
		t.Fatalf("get user B: %v", err)
	}
	if gotA.Name != "A" || gotB.Name != "B" {
		t.Fatalf("escopo por usuário falhou: A=%q B=%q", gotA.Name, gotB.Name)
	}
}

func TestDBRepositorySaveJobNormalizesPipelineTriggersAndTags(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("sync-jira", "Sync Jira")
	job.Pipeline = "Ops"
	job.Tags = []string{"Ops", "jira", "ops"}
	job.Triggers = []Trigger{{Type: TriggerInterval, Every: "1h"}}

	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	if job.Pipeline != "ops" {
		t.Fatalf("pipeline normalizada no objeto salvo: got %q, want ops", job.Pipeline)
	}
	if len(job.Tags) != 2 || job.Tags[0] != "ops" || job.Tags[1] != "jira" {
		t.Fatalf("tags normalizadas no objeto salvo: %#v", job.Tags)
	}
	got, err := repo.GetJob(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Pipeline != "ops" {
		t.Fatalf("pipeline: got %q, want ops", got.Pipeline)
	}
	if len(got.Triggers) != 2 {
		t.Fatalf("triggers: got %d, want interval + manual", len(got.Triggers))
	}
	if len(got.Tags) != 2 || got.Tags[0] != "jira" || got.Tags[1] != "ops" {
		t.Fatalf("tags normalizadas: %#v", got.Tags)
	}
	listed, err := repo.ListJobs(userA, JobFilter{})
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	if len(listed) != 1 || len(listed[0].Tags) != 2 || listed[0].Tags[0] != "jira" || listed[0].Tags[1] != "ops" {
		t.Fatalf("tags normalizadas na listagem: %#v", listed)
	}
}

func TestDBRepositoryKeepsJobEnabledSeparateFromPipelineState(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SavePipeline(userA, &Pipeline{Slug: "ops", Name: "Ops", Enabled: false}); err != nil {
		t.Fatalf("save pipeline: %v", err)
	}
	job := testRepositoryJob("sync-jira", "Sync Jira")
	job.Pipeline = "ops"
	job.Enabled = true
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	got, err := repo.GetJob(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if !got.Enabled {
		t.Fatal("job-level enabled flag was folded into disabled pipeline state")
	}
	if got.PipelineEnabled {
		t.Fatal("expected disabled pipeline runtime state to be preserved separately")
	}
}

func TestDBRepositoryRequiresUser(t *testing.T) {
	repo, _, _ := setupJobsRepositoryTest(t)
	err := repo.SaveJob(context.Background(), testRepositoryJob("x", "X"))
	if err != database.ErrUserScopeRequired {
		t.Fatalf("SaveJob sem usuário: got %v, want ErrUserScopeRequired", err)
	}
}

func TestDBRepositorySaveJobCreatesUnresolvedMCPToolForImportedServer(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	userID := "user-a"
	if err := repo.db.Create(&database.MCPServer{
		UserID: userID,
		Slug:   "jira",
		Name:   "Jira",
	}).Error; err != nil {
		t.Fatalf("seed mcp server: %v", err)
	}
	job := testRepositoryJob("sync-jira", "Sync Jira")
	job.Tool = "mcp_jira__create_issue"
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job with unresolved mcp tool: %v", err)
	}
	var row database.ToolCatalog
	if err := repo.db.Where("name = ?", "mcp_jira__create_issue").First(&row).Error; err != nil {
		t.Fatalf("expected placeholder tool catalog row: %v", err)
	}
	if row.AvailabilityStatus != "unavailable" || row.MCPServerID == nil {
		t.Fatalf("unexpected placeholder row: %#v", row)
	}
	var jobRow database.Job
	if err := repo.db.Where("slug = ?", "sync-jira").First(&jobRow).Error; err != nil {
		t.Fatalf("load job row: %v", err)
	}
	if jobRow.ToolName != "mcp_jira__create_issue" {
		t.Fatalf("job tool_name = %q, want mcp_jira__create_issue", jobRow.ToolName)
	}
	catalog, err := repo.ListToolCatalog(userA)
	if err != nil {
		t.Fatalf("list tool catalog: %v", err)
	}
	found := false
	for _, entry := range catalog {
		if entry.Name == "mcp_jira__create_issue" && entry.Source == "mcp" {
			if entry.AvailabilityStatus != "unavailable" {
				t.Fatalf("placeholder availability = %q, want unavailable", entry.AvailabilityStatus)
			}
			found = true
		}
	}
	if !found {
		t.Fatalf("persistent MCP placeholder hidden from job catalog: %#v", catalog)
	}
}

func TestDBRepositorySaveJobPrefersAttachedAvailableMCPTool(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	userID := "user-a"
	if err := repo.db.Create(&database.MCPServer{
		UserID: userID,
		Slug:   "jira",
		Name:   "Jira",
	}).Error; err != nil {
		t.Fatalf("seed mcp server: %v", err)
	}
	var server database.MCPServer
	if err := repo.db.Where("user_id = ? AND slug = ?", userID, "jira").First(&server).Error; err != nil {
		t.Fatalf("load server: %v", err)
	}
	detached := database.ToolCatalog{
		UserID:             &userID,
		Name:               "mcp_jira__create_issue",
		DisplayName:        "old",
		Origin:             "mcp_bridge",
		AvailabilityStatus: "unavailable",
	}
	if err := repo.db.Create(&detached).Error; err != nil {
		t.Fatalf("seed detached tool: %v", err)
	}
	attached := database.ToolCatalog{
		UserID:             &userID,
		MCPServerID:        &server.ID,
		Name:               "mcp_jira__create_issue",
		DisplayName:        "new",
		Origin:             "mcp_bridge",
		AvailabilityStatus: "available",
	}
	if err := repo.db.Create(&attached).Error; err != nil {
		t.Fatalf("seed attached tool: %v", err)
	}
	job := testRepositoryJob("sync-jira", "Sync Jira")
	job.Tool = "mcp_jira__create_issue"
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	var row database.Job
	if err := repo.db.Where("slug = ?", "sync-jira").First(&row).Error; err != nil {
		t.Fatalf("load job: %v", err)
	}
	if row.ToolCatalogID != attached.ID {
		t.Fatalf("job picked stale tool catalog id %s, want attached %s", row.ToolCatalogID, attached.ID)
	}
}

func TestDBRepositorySaveJobPreservesCreatedByOnUpdate(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("sync-jira", "Sync Jira")
	job.Metadata.CreatedBy = "seed-import"
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save initial: %v", err)
	}

	update := testRepositoryJob("sync-jira", "Sync Jira Updated")
	update.Metadata.CreatedBy = "malicious-update"
	if err := repo.SaveJob(userA, update); err != nil {
		t.Fatalf("save update: %v", err)
	}
	got, err := repo.GetJob(userA, "sync-jira")
	if err != nil {
		t.Fatalf("get job: %v", err)
	}
	if got.Metadata.CreatedBy != "seed-import" {
		t.Fatalf("created_by: got %q, want seed-import", got.Metadata.CreatedBy)
	}
}

func TestDBRepositorySaveJobRejectsNilJob(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SaveJob(userA, nil); err == nil {
		t.Fatal("expected nil job to return an error")
	}
}

func TestDBRepositoryLogRunMatchesEventTriggerByExpression(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("event-job", "Event Job")
	job.Triggers = []Trigger{
		{Type: TriggerEvent, Listen: "deploy"},
		{Type: TriggerEvent, Listen: "deploy-extra"},
	}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{
		RunID:     "run-event",
		JobID:     "event-job",
		Status:    "completed",
		Trigger:   TriggerInfo{Type: TriggerEvent, Event: "deploy"},
		StartedAt: time.Now(),
	}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run: %v", err)
	}
	var row database.JobRun
	if err := repo.db.Preload("Trigger").First(&row, "id = ?", "run-event").Error; err != nil {
		t.Fatalf("load run row: %v", err)
	}
	if row.Trigger == nil || row.Trigger.Expression != "deploy" {
		t.Fatalf("trigger expression: %#v", row.Trigger)
	}
}

func TestDBRepositoryTriggerExpressionFollowsTriggerType(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("event-job", "Event Job")
	job.Triggers = []Trigger{{Type: TriggerEvent, Expression: "wrong-cron", Listen: "deploy"}}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	var row database.JobTrigger
	if err := repo.db.Where("type = ?", string(TriggerEvent)).First(&row).Error; err != nil {
		t.Fatalf("load trigger: %v", err)
	}
	if row.Expression != "deploy" {
		t.Fatalf("expression = %q, want deploy", row.Expression)
	}
	if strings.Contains(row.Config, "wrong-cron") {
		t.Fatalf("trigger config kept stale expression field: %s", row.Config)
	}
	rl := &RunLog{
		RunID:     "run-event-extra-expression",
		JobID:     "event-job",
		Status:    "completed",
		Trigger:   TriggerInfo{Type: TriggerEvent, Event: "deploy"},
		StartedAt: time.Now(),
	}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run: %v", err)
	}
}

func TestDBRepositoryLogRunDisambiguatesEventTriggerByWhen(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("deploy-listeners", "Deploy Listeners")
	job.Triggers = []Trigger{
		{Type: TriggerEvent, Listen: "deploy", When: `{{ eq .event.env "prod" }}`},
		{Type: TriggerEvent, Listen: "deploy", When: `{{ eq .event.env "staging" }}`},
	}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{
		RunID:     "run-staging",
		JobID:     "deploy-listeners",
		Status:    "completed",
		Trigger:   TriggerInfo{Type: TriggerEvent, Event: "deploy", When: `{{ eq .event.env "staging" }}`},
		StartedAt: time.Now(),
	}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run: %v", err)
	}

	var row database.JobRun
	if err := repo.db.Preload("Trigger").First(&row, "id = ?", "run-staging").Error; err != nil {
		t.Fatalf("load run row: %v", err)
	}
	if row.Trigger == nil || !strings.Contains(row.Trigger.Config, "staging") {
		t.Fatalf("run was linked to wrong trigger: %#v", row.Trigger)
	}
}

func TestDBRepositorySaveJobKeepsDistinctTriggersWithSameExpression(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("deploy-listeners", "Deploy Listeners")
	job.Triggers = []Trigger{
		{Type: TriggerEvent, Listen: "deploy", When: `{{ eq .event.env "prod" }}`},
		{Type: TriggerEvent, Listen: "deploy", When: `{{ eq .event.env "staging" }}`},
	}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}

	triggers, err := repo.ListTriggers(userA, "deploy-listeners")
	if err != nil {
		t.Fatalf("list triggers: %v", err)
	}
	eventTriggers := 0
	seen := map[string]bool{}
	for _, trigger := range triggers {
		if trigger.Type != TriggerEvent {
			continue
		}
		eventTriggers++
		seen[trigger.When] = true
	}
	if eventTriggers != 2 || !seen[`{{ eq .event.env "prod" }}`] || !seen[`{{ eq .event.env "staging" }}`] {
		t.Fatalf("distinct event triggers were not preserved: %#v", triggers)
	}
}

func TestDBRepositoryJobTriggerIdentityIsUniqueInDatabase(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("unique-trigger-job", "Unique Trigger Job")
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	var existing database.JobTrigger
	if err := repo.db.Where("type = ?", string(TriggerManual)).First(&existing).Error; err != nil {
		t.Fatalf("load trigger: %v", err)
	}
	duplicate := existing
	duplicate.ID = ""
	if err := repo.db.Create(&duplicate).Error; err == nil {
		t.Fatal("expected duplicate trigger identity to violate unique index")
	}
}

func TestDBRepositoryLogRunRejectsDuplicateRunID(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("duplicate-run", "Duplicate Run")
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{RunID: "run-duplicate", JobID: "duplicate-run", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: time.Now()}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log initial run: %v", err)
	}
	duplicate := *rl
	duplicate.Status = "failed"
	if err := repo.LogRun(userA, &duplicate); err == nil {
		t.Fatal("expected duplicate run id to fail instead of overwriting")
	}
	got, err := repo.GetRun(userA, "duplicate-run", "run-duplicate")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != "completed" {
		t.Fatalf("duplicate run overwrote original status: got %q", got.Status)
	}
}

func TestDBRepositoryLogRunPreservesEmptyRuntimeJSONValues(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("empty-runtime-json", "Empty Runtime JSON")
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{
		RunID:          "run-empty-json",
		JobID:          "empty-runtime-json",
		Status:         "completed",
		Trigger:        TriggerInfo{Type: TriggerManual},
		StartedAt:      time.Now(),
		ResolvedInputs: map[string]any{},
		Output:         map[string]any{},
		EventsEmitted:  []string{},
	}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run: %v", err)
	}

	got, err := repo.GetRun(userA, "empty-runtime-json", "run-empty-json")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.ResolvedInputs == nil || len(got.ResolvedInputs) != 0 {
		t.Fatalf("resolved inputs not preserved as empty object: %#v", got.ResolvedInputs)
	}
	if got.Output == nil || len(got.Output) != 0 {
		t.Fatalf("output not preserved as empty object: %#v", got.Output)
	}
	if got.EventsEmitted == nil || len(got.EventsEmitted) != 0 {
		t.Fatalf("events emitted not preserved as empty array: %#v", got.EventsEmitted)
	}
}

func TestDBRepositoryLogRunRedactsSensitiveInputs(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("redact-run", "Redact Run")
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{
		RunID:     "run-redact",
		JobID:     "redact-run",
		Status:    "completed",
		Trigger:   TriggerInfo{Type: TriggerManual},
		StartedAt: time.Now(),
		ResolvedInputs: map[string]any{
			"api_key":    "secret-value",
			"apiKey":     "camel-secret",
			"privateKey": "private-secret",
			"accessKey":  "access-secret",
			"cookie":     "cookie-secret",
			"session_id": "session-secret",
			"jwt":        "jwt-secret",
			"query":      "public",
		},
		Output:        map[string]any{},
		EventsEmitted: []string{},
	}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run: %v", err)
	}

	got, err := repo.GetRun(userA, "redact-run", "run-redact")
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.ResolvedInputs["api_key"] != redactedValue {
		t.Fatalf("api_key not redacted: %#v", got.ResolvedInputs)
	}
	if got.ResolvedInputs["apiKey"] != redactedValue ||
		got.ResolvedInputs["privateKey"] != redactedValue ||
		got.ResolvedInputs["accessKey"] != redactedValue ||
		got.ResolvedInputs["cookie"] != redactedValue ||
		got.ResolvedInputs["session_id"] != redactedValue ||
		got.ResolvedInputs["jwt"] != redactedValue {
		t.Fatalf("camelCase sensitive keys not redacted: %#v", got.ResolvedInputs)
	}
	if got.ResolvedInputs["query"] != "public" {
		t.Fatalf("public input was changed: %#v", got.ResolvedInputs)
	}
	if got.Replayable {
		t.Fatalf("run with redacted inputs should not be replayable: %#v", got)
	}
}

func TestDBRepositoryLogRunPersistsRunEvents(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("run-events", "Run Events")
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{
		RunID:     "run-with-events",
		JobID:     "run-events",
		Status:    "failed",
		Trigger:   TriggerInfo{Type: TriggerManual},
		StartedAt: time.Now(),
		RunEvents: []RunEvent{
			{Type: "triggered", Message: "started"},
			{Type: "failed", Message: "failed"},
		},
	}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run: %v", err)
	}
	events, err := repo.GetRunEvents(userA, "run-with-events")
	if err != nil {
		t.Fatalf("get run events: %v", err)
	}
	if len(events) != 2 || events[0].Type != "triggered" || events[1].Type != "failed" {
		t.Fatalf("unexpected run events: %#v", events)
	}
	timeline, err := repo.ListEvents(userA, EventFilter{})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(timeline) != 2 || timeline[0].JobID != "run-events" || timeline[1].JobID != "run-events" {
		t.Fatalf("run events not exposed in public timeline: %#v", timeline)
	}
}

func TestDBRepositoryLogRunEventAssignsMissingSequence(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SaveJob(userA, testRepositoryJob("sequenced-events", "Sequenced Events")); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{RunID: "run-sequenced", JobID: "sequenced-events", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: time.Now()}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run: %v", err)
	}
	first := &RunEvent{RunID: "run-sequenced", Type: "note", Message: "first"}
	second := &RunEvent{RunID: "run-sequenced", Type: "note", Message: "second"}
	if err := repo.LogRunEvent(userA, first); err != nil {
		t.Fatalf("log first run event: %v", err)
	}
	if err := repo.LogRunEvent(userA, second); err != nil {
		t.Fatalf("log second run event: %v", err)
	}
	events, err := repo.GetRunEvents(userA, "run-sequenced")
	if err != nil {
		t.Fatalf("get run events: %v", err)
	}
	if len(events) != 2 || events[0].Sequence != 1 || events[1].Sequence != 2 {
		t.Fatalf("sequences not assigned monotonically: %#v", events)
	}
	if first.Sequence != 1 || second.Sequence != 2 {
		t.Fatalf("entry sequences not reflected back: first=%d second=%d", first.Sequence, second.Sequence)
	}
}

func TestDBRepositoryLogRunEventRequiresScopedRun(t *testing.T) {
	repo, userA, userB := setupJobsRepositoryTest(t)
	if err := repo.SaveJob(userA, testRepositoryJob("user-a-job", "User A Job")); err != nil {
		t.Fatalf("save user A job: %v", err)
	}
	rl := &RunLog{RunID: "run-user-a", JobID: "user-a-job", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: time.Now()}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run: %v", err)
	}
	err := repo.LogRunEvent(userB, &RunEvent{RunID: "run-user-a", Type: "triggered", Message: "cross user"})
	if err == nil {
		t.Fatal("expected cross-user run event insertion to fail")
	}
	events, err := repo.GetRunEvents(userA, "run-user-a")
	if err != nil {
		t.Fatalf("get run events: %v", err)
	}
	if len(events) != 0 {
		t.Fatalf("cross-user run event was inserted: %#v", events)
	}
}

func TestDBRepositorySaveJobNamesUnknownTool(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("unknown-tool", "Unknown Tool")
	job.Tool = "not_a_real_tool"
	err := repo.SaveJob(userA, job)
	if err == nil || !strings.Contains(err.Error(), "not_a_real_tool") {
		t.Fatalf("expected missing tool error to include tool name, got %v", err)
	}
}

func TestDBRepositoryListEventsReturnsJobSlugAndFiltersByJob(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	jobA := testRepositoryJob("job-a", "Job A")
	jobB := testRepositoryJob("job-b", "Job B")
	if err := repo.SaveJob(userA, jobA); err != nil {
		t.Fatalf("save job A: %v", err)
	}
	if err := repo.SaveJob(userA, jobB); err != nil {
		t.Fatalf("save job B: %v", err)
	}
	now := time.Now()
	if err := repo.LogEvent(userA, &EventEntry{JobID: "job-a", Timestamp: now, Type: "emitted", Event: "done", Message: "A"}); err != nil {
		t.Fatalf("log event A: %v", err)
	}
	if err := repo.LogEvent(userA, &EventEntry{JobID: "job-b", Timestamp: now.Add(time.Second), Type: "emitted", Event: "done", Message: "B"}); err != nil {
		t.Fatalf("log event B: %v", err)
	}

	got, err := repo.ListEvents(userA, EventFilter{JobID: "job-a"})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("events count: got %d, want 1", len(got))
	}
	if got[0].JobID != "job-a" || got[0].Message != "A" {
		t.Fatalf("unexpected event: %#v", got[0])
	}
}

func TestDBRepositoryListEventsFiltersByDateRange(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("job-a", "Job A")
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatalf("save job: %v", err)
	}
	day := time.Date(2026, 5, 13, 10, 0, 0, 0, time.UTC)
	if err := repo.LogEvent(userA, &EventEntry{JobID: "job-a", Timestamp: day.Add(-time.Hour), Type: "emitted", Message: "old"}); err != nil {
		t.Fatalf("log old event: %v", err)
	}
	if err := repo.LogEvent(userA, &EventEntry{JobID: "job-a", Timestamp: day.Add(time.Hour), Type: "emitted", Message: "inside"}); err != nil {
		t.Fatalf("log inside event: %v", err)
	}
	if err := repo.LogEvent(userA, &EventEntry{JobID: "job-a", Timestamp: day.Add(25 * time.Hour), Type: "emitted", Message: "new"}); err != nil {
		t.Fatalf("log new event: %v", err)
	}

	got, err := repo.ListEvents(userA, EventFilter{StartAt: day, EndAt: day.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(got) != 1 || got[0].Message != "inside" {
		t.Fatalf("unexpected events: %#v", got)
	}
}

func TestDBRepositoryGetLastRunsBatch(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SaveJob(userA, testRepositoryJob("job-a", "Job A")); err != nil {
		t.Fatalf("save job A: %v", err)
	}
	if err := repo.SaveJob(userA, testRepositoryJob("job-b", "Job B")); err != nil {
		t.Fatalf("save job B: %v", err)
	}
	start := time.Now()
	runs := []RunLog{
		{RunID: "run-a-old", JobID: "job-a", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: start},
		{RunID: "run-a-new", JobID: "job-a", Status: "failed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: start.Add(time.Hour)},
		{RunID: "run-b", JobID: "job-b", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: start.Add(2 * time.Hour)},
	}
	for i := range runs {
		if err := repo.LogRun(userA, &runs[i]); err != nil {
			t.Fatalf("log run %s: %v", runs[i].RunID, err)
		}
	}

	got, err := repo.GetLastRuns(userA, []string{"job-a", "job-b"})
	if err != nil {
		t.Fatalf("get last runs: %v", err)
	}
	if got["job-a"] == nil || got["job-a"].RunID != "run-a-new" {
		t.Fatalf("job-a last run: %#v", got["job-a"])
	}
	if got["job-b"] == nil || got["job-b"].RunID != "run-b" {
		t.Fatalf("job-b last run: %#v", got["job-b"])
	}
}

func TestDBRepositoryListRunsFilterByStatus(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SaveJob(userA, testRepositoryJob("filter-status", "Filter Status")); err != nil {
		t.Fatalf("save job: %v", err)
	}
	base := time.Now()
	runs := []RunLog{
		{RunID: "r-completed", JobID: "filter-status", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: base},
		{RunID: "r-failed", JobID: "filter-status", Status: "failed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: base.Add(time.Minute)},
		{RunID: "r-skipped", JobID: "filter-status", Status: "skipped", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: base.Add(2 * time.Minute)},
	}
	for i := range runs {
		if err := repo.LogRun(userA, &runs[i]); err != nil {
			t.Fatalf("log run %s: %v", runs[i].RunID, err)
		}
	}

	got, err := repo.ListRuns(userA, "filter-status", RunFilter{Status: []string{"failed"}})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(got) != 1 || got[0].RunID != "r-failed" {
		t.Fatalf("status filter: %#v", got)
	}

	got, err = repo.ListRuns(userA, "filter-status", RunFilter{Status: []string{"failed", "skipped"}})
	if err != nil {
		t.Fatalf("list runs (multi): %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("multi status filter count: got %d want 2", len(got))
	}
}

func TestDBRepositoryListRunsFilterByDateRange(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SaveJob(userA, testRepositoryJob("filter-range", "Filter Range")); err != nil {
		t.Fatalf("save job: %v", err)
	}
	day := time.Date(2026, 5, 10, 12, 0, 0, 0, time.UTC)
	runs := []RunLog{
		{RunID: "r-before", JobID: "filter-range", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: day.Add(-2 * time.Hour)},
		{RunID: "r-inside", JobID: "filter-range", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: day.Add(time.Hour)},
		{RunID: "r-after", JobID: "filter-range", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: day.Add(25 * time.Hour)},
	}
	for i := range runs {
		if err := repo.LogRun(userA, &runs[i]); err != nil {
			t.Fatalf("log run %s: %v", runs[i].RunID, err)
		}
	}

	got, err := repo.ListRuns(userA, "filter-range", RunFilter{StartedAfter: day, StartedBefore: day.Add(24 * time.Hour)})
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(got) != 1 || got[0].RunID != "r-inside" {
		t.Fatalf("date range filter: %#v", got)
	}
}

func TestDBRepositoryListRunsExcludesDryRunByDefault(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SaveJob(userA, testRepositoryJob("filter-dryrun", "Filter DryRun")); err != nil {
		t.Fatalf("save job: %v", err)
	}
	base := time.Now()
	runs := []RunLog{
		{RunID: "r-real", JobID: "filter-dryrun", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: base},
		{RunID: "r-dry", JobID: "filter-dryrun", Status: "completed", Trigger: TriggerInfo{Type: TriggerManual}, StartedAt: base.Add(time.Minute), IsDryRun: true},
	}
	for i := range runs {
		if err := repo.LogRun(userA, &runs[i]); err != nil {
			t.Fatalf("log run %s: %v", runs[i].RunID, err)
		}
	}

	got, err := repo.ListRuns(userA, "filter-dryrun", RunFilter{})
	if err != nil {
		t.Fatalf("list runs default: %v", err)
	}
	if len(got) != 1 || got[0].RunID != "r-real" {
		t.Fatalf("default should exclude dry-runs: %#v", got)
	}

	got, err = repo.ListRuns(userA, "filter-dryrun", RunFilter{IncludeDryRun: true})
	if err != nil {
		t.Fatalf("list runs include: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("include_dry_run: got %d want 2", len(got))
	}
}

func TestDBRepositoryGetRunDetailHydratesEvents(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SaveJob(userA, testRepositoryJob("detail-job", "Detail Job")); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{
		RunID:     "run-detail",
		JobID:     "detail-job",
		Status:    "failed",
		Trigger:   TriggerInfo{Type: TriggerManual},
		StartedAt: time.Now(),
		RunEvents: []RunEvent{
			{Type: "triggered", Message: "started"},
			{Type: "failed", Message: "boom"},
		},
	}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run: %v", err)
	}
	if err := repo.LogEvent(userA, &EventEntry{JobID: "detail-job", RunID: "run-detail", Timestamp: time.Now(), Type: "emitted", Event: "fail", Message: "domain fail"}); err != nil {
		t.Fatalf("log event: %v", err)
	}
	if err := repo.LogEvent(userA, &EventEntry{JobID: "detail-job", Timestamp: time.Now(), Type: "emitted", Event: "unrelated", Message: "no run"}); err != nil {
		t.Fatalf("log unrelated event: %v", err)
	}

	detail, err := repo.GetRunDetail(userA, "detail-job", "run-detail")
	if err != nil {
		t.Fatalf("get run detail: %v", err)
	}
	if detail.RunID != "run-detail" || detail.Status != "failed" {
		t.Fatalf("unexpected run log in detail: %#v", detail.RunLog)
	}
	if len(detail.RunEvents) != 2 || detail.RunEvents[0].Sequence != 1 || detail.RunEvents[1].Sequence != 2 {
		t.Fatalf("run events not hydrated ordered: %#v", detail.RunEvents)
	}
	if len(detail.DomainEvents) == 0 {
		t.Fatalf("domain events empty")
	}
	for _, ev := range detail.DomainEvents {
		if ev.RunID != "" && ev.RunID != "run-detail" {
			t.Fatalf("domain events not scoped to run: %#v", ev)
		}
	}
}

func TestDBRepositoryListEventsFilterByRunID(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	if err := repo.SaveJob(userA, testRepositoryJob("evt-runid", "Evt RunID")); err != nil {
		t.Fatalf("save job: %v", err)
	}
	rl := &RunLog{
		RunID:     "run-x",
		JobID:     "evt-runid",
		Status:    "completed",
		Trigger:   TriggerInfo{Type: TriggerManual},
		StartedAt: time.Now(),
		RunEvents: []RunEvent{{Type: "triggered", Message: "x"}},
	}
	if err := repo.LogRun(userA, rl); err != nil {
		t.Fatalf("log run x: %v", err)
	}
	rl2 := &RunLog{
		RunID:     "run-y",
		JobID:     "evt-runid",
		Status:    "completed",
		Trigger:   TriggerInfo{Type: TriggerManual},
		StartedAt: time.Now().Add(time.Second),
		RunEvents: []RunEvent{{Type: "triggered", Message: "y"}},
	}
	if err := repo.LogRun(userA, rl2); err != nil {
		t.Fatalf("log run y: %v", err)
	}
	if err := repo.LogEvent(userA, &EventEntry{JobID: "evt-runid", RunID: "run-x", Timestamp: time.Now(), Type: "emitted", Event: "x-event", Message: "domain x"}); err != nil {
		t.Fatalf("log domain x: %v", err)
	}
	if err := repo.LogEvent(userA, &EventEntry{JobID: "evt-runid", RunID: "run-y", Timestamp: time.Now(), Type: "emitted", Event: "y-event", Message: "domain y"}); err != nil {
		t.Fatalf("log domain y: %v", err)
	}

	got, err := repo.ListEvents(userA, EventFilter{RunID: "run-x"})
	if err != nil {
		t.Fatalf("list events run-x: %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("expected events for run-x")
	}
	for _, ev := range got {
		if ev.RunID != "run-x" {
			t.Fatalf("run-id filter leaked event from other run: %#v", ev)
		}
	}
}

func testRepositoryJob(id, name string) *Job {
	return &Job{
		ID:      id,
		Name:    name,
		Enabled: true,
		Tool:    "test_tool",
		Triggers: []Trigger{
			{Type: TriggerManual},
		},
	}
}
