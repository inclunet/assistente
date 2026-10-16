package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestAppShutdownPreservesDependenciesWhenExecutorDrainFails(t *testing.T) {
	a := &App{}
	core, err := a.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	deactivated := false
	a.cancel = func() { deactivated = true }
	failure := errors.New("executor still finalizing")
	if err := core.RegisterExecutorDrain(context.Background(), func(context.Context) error { return failure }); err != nil {
		t.Fatal(err)
	}
	a.Shutdown()
	if deactivated {
		t.Fatal("App destruiu dependência antes da drenagem")
	}
	if err := core.RegisterExecutorDrain(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatalf("novo executor após shutdown=%v", err)
	}
}

func TestAppDrainRecoversPendingInvocationsFromDrainedGenerations(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "commands.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := commandledger.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := commanddecision.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previousDB) })

	app := &App{commandStorageVersion: "test"}
	core, err := app.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	userID := uuid.Must(uuid.NewV7()).String()
	sessionID := uuid.Must(uuid.NewV7()).String()
	epoch, err := core.Capture(ctx, userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	store, err := commandledger.New(db, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	request := commandledger.LocalReadRequest{
		InvocationID:              uuid.Must(uuid.NewV7()).String(),
		Owner:                     commandledger.Owner{UserID: userID, AuthContextID: sessionID},
		AuthGeneration:            epoch.AuthGeneration,
		SecurityGeneration:        epoch.SecurityGeneration,
		RegistryVersion:           "registry-v1",
		GlobalConfigGeneration:    "global-v1",
		ActiveLayersGeneration:    "layers-v1",
		CommandID:                 "workspace.read",
		SourceType:                "palette",
		ArgumentsFingerprint:      "args",
		RequestFingerprintVersion: "v1",
		RequestFingerprint:        "request",
		CorrelationID:             uuid.Must(uuid.NewV7()).String(),
		ReceivedAt:                now,
		ExpiresAt:                 now.Add(time.Hour),
	}
	if _, err := store.Reserve(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := app.drainCommandExecutors(ctx); err != nil {
		t.Fatal(err)
	}
	record, err := store.Get(ctx, request.Owner, request.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != commandledger.OutcomeUnknown {
		t.Fatalf("pendência drenada não foi reconciliada: %s", record.Status)
	}
}

func TestAppDrainRecoversPendingDecisionReceiptsFromDrainedGenerations(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "commands.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := commanddecision.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := commandledger.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previousDB) })

	app := &App{commandStorageVersion: "test"}
	core, err := app.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	userID := uuid.Must(uuid.NewV7()).String()
	sessionID := uuid.Must(uuid.NewV7()).String()
	epoch, err := core.Capture(ctx, userID, sessionID)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Millisecond)
	decisionID := uuid.Must(uuid.NewV7()).String()
	if err := db.Table("command_decision_receipts").Create(map[string]any{
		"decision_id":         decisionID,
		"subject_id":          uuid.Must(uuid.NewV7()).String(),
		"user_id":             userID,
		"auth_context_id":     sessionID,
		"request_fingerprint": "request",
		"auth_generation":     epoch.AuthGeneration,
		"security_generation": epoch.SecurityGeneration,
		"expires_at":          now.Add(time.Hour).UnixMilli(),
		"status":              commanddecision.Pending,
		"auth_context_type":   "local_session",
		"subject_type":        "invocation",
		"allowed_action_ids":  `["apply","deny"]`,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("command_decision_receipt_events").Create(map[string]any{
		"id":          uuid.Must(uuid.NewV7()).String(),
		"decision_id": decisionID,
		"state":       commanddecision.Pending,
		"occurred_ms": now.UnixMilli(),
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.drainCommandExecutors(ctx); err != nil {
		t.Fatal(err)
	}
	var state string
	if err := db.Table("command_decision_receipts").Where("decision_id = ?", decisionID).Select("status").Scan(&state).Error; err != nil {
		t.Fatal(err)
	}
	if state != commanddecision.Cancelled {
		t.Fatalf("receipt drenado não foi cancelado: %s", state)
	}
}
