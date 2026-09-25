package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/questionnaire"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestCommandDecisionAdapterPersistsAndConsumesExactBackendReceipt(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "receipt.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	ctx := context.Background()
	if err := commanddecision.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE decision_effect_fixture (id TEXT PRIMARY KEY)").Error; err != nil {
		t.Fatal(err)
	}
	id := func() string { return uuid.Must(uuid.NewV7()).String() }
	request := commanddecision.Request{DecisionID: id(), MutationID: id(), UserID: id(), SessionID: id(), Fingerprint: "fixture-only-fingerprint", AuthGeneration: "auth:1", SecurityGeneration: "security:1", ExpiresAt: time.Now().Add(time.Minute), Body: `{"enabled":{"before":true,"after":false}}`}
	var manager *questionnaire.Manager
	var emitted bool
	manager = questionnaire.NewManager(func(event string, data any) {
		if event != questionnaire.EventQuestionnaire {
			return
		}
		emitted = true
		payload := data.(map[string]any)
		if payload["body"] != request.Body || payload["kind"] != questionnaire.KindDecision {
			t.Fatal("payload divergente")
		}
		uiID := payload["id"].(string)
		if uiID == request.DecisionID {
			t.Fatal("ID backend usado como ID curto da UI")
		}
		if err := manager.Respond(uiID, map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction, "decision_id": id()}, false); err != nil {
			t.Fatal(err)
		}
	})
	store, err := commanddecision.New(db, &commandDecisionPresenter{manager: manager}, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.Decide(ctx, request)
	if err != nil || state != commanddecision.Accepted || !emitted {
		t.Fatal(state, err)
	}
	if err := store.Consume(ctx, request, func(tx *gorm.DB) error {
		return tx.Exec("INSERT INTO decision_effect_fixture(id) VALUES (?)", request.MutationID).Error
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.Consume(ctx, request, func(*gorm.DB) error { t.Fatal("receipt executou duas vezes"); return nil }); !errors.Is(err, commanddecision.ErrStale) {
		t.Fatal(err)
	}
	var count int64
	if err := db.Table("decision_effect_fixture").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatal("efeito incorreto", count)
	}
}
