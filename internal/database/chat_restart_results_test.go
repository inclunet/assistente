package database

import (
	"strings"
	"testing"
	"time"
)

func TestRestartContinuationUsesSelectedLedgerAndRetryBreaksChain(t *testing.T) {
	db, repo, ctx, first := executionSchemaFixture(t)
	if _, err := repo.EnsureResponse(ctx, "conv", "question", first); err != nil {
		t.Fatal(err)
	}
	catalog := ToolCatalog{Name: "write_file", DisplayName: "Write", Origin: "builtin"}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	for index, status := range []string{"succeeded", "running"} {
		if err := db.Table("tool_invocations").Create(map[string]any{
			"id": status, "user_id": "user", "tool_catalog_id": catalog.ID,
			"origin_type": "chat", "origin_id": "question", "conversation_id": "conv", "turn_id": "question",
			"status": status, "queued_at": time.Unix(int64(index), 0), "chat_execution_id": first,
			"output": `{"content":"arquivo salvo","metadata":{"privado":"` + strings.Repeat("x", 5000) + `"}}`, "result_availability": "available",
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := repo.Finish(ctx, "conv", "question", first, TurnInterrupted, "restart"); err != nil {
		t.Fatal(err)
	}
	next, err := repo.Begin(ctx, "conv", "question", "", TurnModeContinue)
	if err != nil {
		t.Fatal(err)
	}
	rows := next.RestartResults
	if len(rows) != 2 || rows[0].Status != "running" || rows[1].ID != "succeeded" || !strings.Contains(rows[1].Output, "arquivo salvo") || strings.Contains(rows[1].Output, "privado") {
		t.Fatalf("evidência %v", rows)
	}
	if err := repo.Finish(ctx, "conv", "question", next.ExecutionID, TurnInterrupted, "cancelled"); err != nil {
		t.Fatal(err)
	}
	projected, err := repo.GetManyWithPresentation(ctx, "conv", []string{"question"})
	if err != nil || !projected["question"].RestartInChain {
		t.Fatalf("a segunda tentativa perdeu a base interrompida: %v, %v", projected, err)
	}
	retry, err := repo.Begin(ctx, "conv", "question", "", TurnModeRetry)
	if err != nil || len(retry.RestartResults) != 0 {
		t.Fatalf("retry: %v, %v", retry, err)
	}
	if _, err := repo.EnsureResponse(ctx, "conv", "question", retry.ExecutionID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Finish(ctx, "conv", "question", retry.ExecutionID, TurnInterrupted, "restart"); err != nil {
		t.Fatal(err)
	}
	continued, err := repo.Begin(ctx, "conv", "question", "", TurnModeContinue)
	if err != nil || continued.RestartResults == nil || len(continued.RestartResults) != 0 {
		t.Fatalf("recuperou ação substituída: %v, %v", continued, err)
	}
}
