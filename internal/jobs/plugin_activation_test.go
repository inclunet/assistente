package jobs

import (
	"context"
	"errors"
	"gorm.io/gorm"
	"testing"

	"assistente/internal/database"
	"assistente/internal/tools"
)

func TestRevokedPluginJobCannotPublishEvents(t *testing.T) {
	repo, owner, _ := setupJobsRepositoryTest(t)
	if err := repo.db.AutoMigrate(&database.PluginInstallation{}); err != nil {
		t.Fatal(err)
	}
	bus := NewEventBus()
	events := make(chan string, 4)
	for _, name := range []string{"plugin.success", "plugin.failure"} {
		bus.Subscribe(name, "consumer", func(_ context.Context, event string, _ map[string]any) { events <- event })
	}
	executor := mustNewJobExecutor(t, ExecutorConfig{Repository: repo, EventBus: bus, CircuitBreaker: NewCircuitBreaker()})
	job := &Job{ID: "removed", DatabaseID: "past-job", PluginInstallationID: "removed-installation", PluginDigest: "old", Tool: "test_tool", Events: EventsConfig{OnSuccess: "plugin.success", OnFailure: "plugin.failure"}}
	for _, mock := range []bool{false, true} {
		job.DryRun.Enabled = mock
		run := executor.Execute(owner, job, &TriggerContext{Type: TriggerManual})
		if run.Status != RunStatusFailed || run.Error != ErrPluginJobInactive.Error() || len(run.RunEvents) != 0 {
			t.Fatalf("job revogado iniciou execução: %+v", run)
		}
		executor.emitSuccess(owner, job, run, &TriggerContext{})
		executor.emitFailure(owner, job, run, &TriggerContext{})
	}
	bus.Close()
	select {
	case event := <-events:
		t.Fatalf("evento indevido: %s", event)
	default:
	}
}

func TestCatalogTestRevalidatesManagedJob(t *testing.T) {
	repo, owner, _ := setupJobsRepositoryTest(t)
	if err := repo.db.AutoMigrate(&database.PluginInstallation{}); err != nil {
		t.Fatal(err)
	}
	plugin := database.PluginInstallation{UserID: "user-a", Origin: "local", Name: "example", Digest: "v1"}
	if err := repo.db.Create(&plugin).Error; err != nil {
		t.Fatal(err)
	}
	job := &Job{ID: "plugin-example", Name: "Example", Tool: "test_tool", Triggers: []Trigger{{Type: TriggerManual}}}
	if err := repo.db.Transaction(func(tx *gorm.DB) error {
		return CreateDisabledPluginJobTx(owner, tx, job, plugin.ID, plugin.Digest, "jobs/example")
	}); err != nil {
		t.Fatal(err)
	}
	m := mustNewManager(t, ManagerConfig{Repository: repo, ContextProvider: func() context.Context { return owner }})
	observed, err := repo.GetJob(owner, job.ID)
	if err != nil {
		t.Fatal(err)
	}
	provenance, err := m.executor.commandRunProvenance(observed, &TriggerContext{}, &RunLog{})
	if err != nil || provenance["plugin_installation_id"] != plugin.ID {
		t.Fatalf("origem do histórico: %v, %v", provenance, err)
	}
	req := TestToolRequest{JobID: job.ID, PluginInstallationID: plugin.ID, ToolName: job.Tool, AllowUnsafe: true}
	if _, err := m.TestToolDryRunContext(owner, req); !errors.Is(err, ErrPluginJobInactive) {
		t.Fatalf("teste de job desativado: %v", err)
	}
	observed.Enabled = true
	if err := repo.SaveJob(owner, observed); err != nil {
		t.Fatal(err)
	}
	ctx, err := m.executor.pluginJobContext(owner, observed)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Model(&plugin).Update("disabled", true).Error; err != nil {
		t.Fatal(err)
	}
	ec, _ := tools.GetExecutionContext(ctx)
	if err := ec.CheckCommandActivation(ctx); !errors.Is(err, ErrPluginJobInactive) {
		t.Fatalf("revogação no despacho: %v", err)
	}
	if _, err := m.TestToolDryRunContext(owner, req); !errors.Is(err, ErrPluginJobInactive) {
		t.Fatalf("replay autorizado após revogação: %v", err)
	}
	if err := repo.DeleteJob(owner, observed.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveJob(owner, observed); !errors.Is(err, ErrPluginJobInactive) {
		t.Fatalf("editor recriou job excluído: %v", err)
	}
}
