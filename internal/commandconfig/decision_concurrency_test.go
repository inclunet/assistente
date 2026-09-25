package commandconfig

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandsecurity"
	"gorm.io/gorm"
)

type concurrentDecisionPresenter struct{}

func (concurrentDecisionPresenter) Present(_ context.Context, r commanddecision.Request) (commanddecision.Response, error) {
	return commanddecision.Response{DecisionID: r.DecisionID, ActionID: commanddecision.ApplyAction}, nil
}

func TestConfirmedBindingCompetingProposalsCommitOnce(t *testing.T) {
	f, binding := writeTestBaseline(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := commanddecision.Migrate(ctx, f.db); err != nil {
		t.Fatal(err)
	}
	receipts, err := commanddecision.New(f.db, concurrentDecisionPresenter{}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	epoch := commandsecurity.EpochSnapshot{UserID: f.scope.UserID, SessionID: storeTestUUID7(t), AuthGeneration: "auth:fixture", SecurityGeneration: "security:fixture"}
	keys := func(context.Context, string) ([]byte, error) { return []byte("fixture-key-only-0123456789abcdef"), nil }
	var proposals [2]*ConfirmedBindingEnabledChange
	for i := range proposals {
		change, err := f.store.PrepareBindingEnabled(ctx, f.scope, binding.ID, false, f.options)
		if err != nil {
			t.Fatal(err)
		}
		proposals[i], err = f.store.ConfirmBindingEnabled(ctx, change, epoch, receipts, "v1", keys, time.Now().Add(time.Minute), func(Binding, Binding) (string, error) { return "fixture", nil })
		if err != nil {
			t.Fatal(err)
		}
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, proposal := range proposals {
		go func() { <-start; results <- f.store.CommitConfirmedBindingEnabled(ctx, proposal, epoch) }()
	}
	close(start)
	wins := 0
	for range proposals {
		select {
		case err := <-results:
			if err == nil {
				wins++
				continue
			}
			if !errors.Is(err, ErrStale) && !strings.Contains(err.Error(), "SQLITE_BUSY") && !strings.Contains(err.Error(), "SQLITE_LOCKED") {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("escritas concorrentes não terminaram", ctx.Err())
		}
	}
	if wins != 1 {
		t.Fatal("quantidade de commits", wins)
	}
	if writeTestBindingRow(t, f.db, binding.ID).Enabled {
		t.Fatal("binding não mudou")
	}
	generation := f.snapshot.Generations[0]
	requireGeneration(t, writeTestGenerationRow(t, f.db, generation.ID), generation.ID, generation.Generation+1)
	var count int64
	if err := f.db.Table("command_decision_receipts").Where("status = ?", commanddecision.Consumed).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("consumos", count)
	}
	if countRows(t, f.db, "command_config_mutations") != 1 {
		t.Fatal("auditoria duplicada ou ausente")
	}
}

func TestConfirmedBindingExpiryAfterSQLWriteRollsBackEverything(t *testing.T) {
	f, binding := writeTestBaseline(t)
	ctx := context.Background()
	now := time.Now()
	deadline := now.Add(time.Minute)
	if err := commanddecision.Migrate(ctx, f.db); err != nil {
		t.Fatal(err)
	}
	receipts, err := commanddecision.New(f.db, concurrentDecisionPresenter{}, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	epoch := commandsecurity.EpochSnapshot{UserID: f.scope.UserID, SessionID: storeTestUUID7(t), AuthGeneration: "auth:fixture", SecurityGeneration: "security:fixture"}
	change, err := f.store.PrepareBindingEnabled(ctx, f.scope, binding.ID, false, f.options)
	if err != nil {
		t.Fatal(err)
	}
	confirmed, err := f.store.ConfirmBindingEnabled(ctx, change, epoch, receipts, "v1", func(context.Context, string) ([]byte, error) { return []byte("fixture-key-only-0123456789abcdef"), nil }, deadline, func(Binding, Binding) (string, error) { return "fixture", nil })
	if err != nil {
		t.Fatal(err)
	}
	// Avança relógio precisamente após UPDATE do binding, antes do commit.
	if err := f.db.Callback().Update().After("gorm:update").Register("fixture:expire_binding_decision", func(tx *gorm.DB) {
		if tx.Statement.Table == "command_bindings" {
			now = deadline.Add(time.Millisecond)
		}
	}); err != nil {
		t.Fatal(err)
	}
	err = f.store.CommitConfirmedBindingEnabled(ctx, confirmed, epoch)
	if !errors.Is(err, commanddecision.ErrStale) {
		t.Fatal("prazo ignorado", err)
	}
	if !writeTestBindingRow(t, f.db, binding.ID).Enabled {
		t.Fatal("binding não revertido")
	}
	generation := f.snapshot.Generations[0]
	requireGeneration(t, writeTestGenerationRow(t, f.db, generation.ID), generation.ID, generation.Generation)
	var status string
	if err := f.db.Table("command_decision_receipts").Select("status").Where("decision_id = ?", confirmed.request.DecisionID).Scan(&status).Error; err != nil {
		t.Fatal(err)
	}
	if status != commanddecision.Accepted {
		t.Fatal("receipt não revertida", status)
	}
	var count int64
	if err := f.db.Table("command_decision_receipt_events").Where("state = ?", commanddecision.Consumed).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("consumo auditado apesar do rollback")
	}
	if countRows(t, f.db, "command_config_mutations") != 0 {
		t.Fatal("auditoria de alteração sobreviveu ao rollback")
	}
}
