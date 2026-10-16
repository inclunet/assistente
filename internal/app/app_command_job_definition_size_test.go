package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/database"
	"assistente/internal/jobs"
	"github.com/google/uuid"
)

func TestCommandProjectionSurvivesLargePersistedJobDefinitions(t *testing.T) {
	a := commandMaintenanceAppFixture(t)
	owner := database.WithUserID(context.Background(), a.currentUserID)
	repo := jobs.NewDBRepository(database.DB())
	tool := &globalJobTestTool{}
	a.toolRegistry.MustRegister(tool)
	if err := database.DB().Create(&database.ToolCatalog{
		UUIDModel: database.UUIDModel{ID: uuid.Must(uuid.NewV7()).String()},
		Name:      tool.Name(), DisplayName: tool.Name(), Description: tool.Description(),
		Origin: "builtin", Schema: string(tool.Parameters()), AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		slug   string
		size   int
		hotkey bool
	}{
		{"large-event-job", 277289, false},
		{"large-hotkey-job", 277289, true},
		{"invalid-hotkey-job", 1024 * 1024, true},
	} {
		job := &jobs.Job{
			ID: item.slug, Name: item.slug, Enabled: true, Tool: "test.command_global_job",
			Inputs:   map[string]any{},
			Output:   jobs.OutputConfig{Schema: json.RawMessage(`{"description":"` + strings.Repeat("x", item.size) + `"}`)},
			Triggers: []jobs.Trigger{{Type: jobs.TriggerEvent, Listen: "example.ready"}},
		}
		if item.hotkey {
			job.Triggers = []jobs.Trigger{{Type: jobs.TriggerHotkey, Keys: "Ctrl+Alt+G"}}
		}
		if err := repo.SaveJob(owner, job); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(context.Background()); err != nil {
		t.Fatalf("job bloqueou publicação dos comandos: %v", err)
	}
	bindings, err := a.commandGlobalBindings(context.Background())
	if err != nil || len(bindings) != 1 {
		t.Fatalf("hotkey de job grande deve permanecer disponível: %d %v", len(bindings), err)
	}
	// Executa um comando independente pelo ledger/handoff reais após rebuild.
	reservation := beginAuditedTabCreate(t, a)
	handoff := takeAuditedTabCreate(t, a, reservation.Ticket)
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatal(err)
	}
	if result := getUIResultEventually(t, a, reservation.Ticket); result.Status != "succeeded" {
		t.Fatalf("comando independente bloqueado: %+v", result)
	}
}
