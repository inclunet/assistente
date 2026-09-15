package database

import (
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestCommandCutoverPreservesQueuedRunAndProvenance(t *testing.T) {
	user, _, catalog := setupToolLedgerMigrationTest(t)
	db := DB()
	job := Job{UUIDModel: UUIDModel{ID: "cutover-job"}, UserID: user.ID, Slug: "cutover-job", Name: "Cutover", Enabled: true, ToolCatalogID: catalog.ID, ToolName: "search"}
	trigger := JobTrigger{UUIDModel: UUIDModel{ID: "cutover-trigger"}, UserID: user.ID, JobID: job.ID, Type: "manual", Enabled: true}
	queued := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	run := JobRun{UUIDModel: UUIDModel{ID: "run_opaque_cutover"}, UserID: user.ID, JobID: job.ID, TriggerID: trigger.ID, Status: "queued", QueuedAt: queued, RootOriginType: "manual", RootOriginID: "origin-exact", Provenance: `{"_chain_id":"preserve-exact"}`}
	for _, record := range []any{&job, &trigger, &run} {
		if err := db.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Model(&run).Update("started_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error { return rebuildOperationalJobRuns(tx) }); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		var got JobRun
		if err := db.First(&got, "id = ?", run.ID).Error; err != nil {
			t.Fatal(err)
		}
		if !got.QueuedAt.Equal(queued) || !got.StartedAt.IsZero() || got.RootOriginType != run.RootOriginType || got.RootOriginID != run.RootOriginID || got.Provenance != run.Provenance {
			t.Fatalf("cutover perdeu fatos operacionais: %+v", got)
		}
		if err := db.AutoMigrate(&JobRun{}); err != nil {
			t.Fatal(err)
		}
	}
}
