package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandconfig"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func preparePersistentRestoreBusyFixture(t *testing.T) *App {
	t.Helper()
	a, _ := settingsSecurityFixture(t)
	db := database.DB()
	assertCommandBootstrapTemporaryDatabase(t, db)
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatalf("habilitar WAL: %v", err)
	}
	if err := db.Exec("PRAGMA busy_timeout=1").Error; err != nil {
		t.Fatalf("configurar busy_timeout: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter pool SQLite: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)

	ctx := context.Background()
	configStore, err := commandconfig.New(db)
	if err != nil {
		t.Fatalf("criar store de configuração: %v", err)
	}
	if err := configStore.EnsureScope(ctx, commandconfig.Scope{UserID: a.currentUserID}); err != nil {
		t.Fatalf("EnsureScope: %v", err)
	}
	now := time.Now().UTC()
	layerID := uuid.Must(uuid.NewV7()).String()
	ruleID := uuid.Must(uuid.NewV7()).String()
	if err := db.Create(&commandconfig.Layer{
		ID: layerID, UserID: a.currentUserID, Name: "busy restore layer", Enabled: true,
		Source: "user", CreatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("inserir camada persistente: %v", err)
	}
	if err := db.Create(&commandactivation.Rule{
		ID: ruleID, UserID: a.currentUserID, LayerRefKind: commandactivation.UserRef,
		LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: ruleID,
		Mode: commandactivation.ModeManual, Condition: "{}", Lifecycle: commandactivation.LifecyclePersistent,
		Enabled: true, Source: "user", ReviewStatus: "active",
	}).Error; err != nil {
		t.Fatalf("inserir regra persistente: %v", err)
	}
	stack := "manual:busy-restore"
	if err := db.Create(&commandactivation.Claim{
		ActivationID: uuid.Must(uuid.NewV7()).String(), LayerRefKind: commandactivation.UserRef,
		LayerRef: layerID, RuleRefKind: commandactivation.UserRef, RuleRef: ruleID,
		UserID: a.currentUserID, AuthContextType: "local_session", AuthContextID: "old-session",
		AuthGeneration: "old-auth", SecurityGeneration: "old-security", SourceType: "manual",
		State: commandactivation.StateActive, ManualStackKey: &stack, ActivatedAt: now, UpdatedAt: now,
	}).Error; err != nil {
		t.Fatalf("inserir claim persistente: %v", err)
	}

	return a
}

type restoreBusyRun struct {
	done    <-chan error
	busy    <-chan struct{}
	release func()
	cleanup func()
}

func startRestoreWithPersistentSQLiteWriterLock(t *testing.T, a *App, ctx context.Context) restoreBusyRun {
	t.Helper()
	db := database.DB()
	assertCommandBootstrapTemporaryDatabase(t, db)
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatalf("habilitar WAL: %v", err)
	}
	pool, err := db.DB()
	if err != nil {
		t.Fatalf("obter pool SQLite: %v", err)
	}
	pool.SetMaxOpenConns(4)
	pool.SetMaxIdleConns(4)
	writer, err := pool.Conn(ctx)
	if err != nil {
		t.Fatalf("obter conexão writer: %v", err)
	}
	if _, err := writer.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		_ = writer.Close()
		t.Fatalf("adquirir lock writer SQLite: %v", err)
	}

	busy := make(chan struct{})
	var once sync.Once
	observe := func(tx *gorm.DB) {
		if database.IsSQLiteBusyError(tx.Error) {
			once.Do(func() { close(busy) })
		}
	}
	const hook = "test:command_bootstrap_restore_busy"
	if err := db.Callback().Raw().After("gorm:raw").Register(hook, observe); err != nil {
		_, _ = writer.ExecContext(context.Background(), "ROLLBACK")
		_ = writer.Close()
		t.Fatalf("registrar observador raw: %v", err)
	}
	if err := db.Callback().Update().After("gorm:update").Register(hook, observe); err != nil {
		_ = db.Callback().Raw().Remove(hook)
		_, _ = writer.ExecContext(context.Background(), "ROLLBACK")
		_ = writer.Close()
		t.Fatalf("registrar observador update: %v", err)
	}

	workerCtx, cancelWorker := context.WithCancel(ctx)
	done := make(chan error, 1)
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		done <- a.restoreCommandLifecyclePersistentClaims(workerCtx)
	}()
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() {
			_, _ = writer.ExecContext(context.Background(), "ROLLBACK")
			_ = writer.Close()
		})
	}
	cleanup := func() {
		cancelWorker()
		release()
		select {
		case <-exited:
		case <-time.After(2 * time.Second):
			t.Error("restore não encerrou durante cleanup")
		}
		_ = db.Callback().Raw().Remove(hook)
		_ = db.Callback().Update().Remove(hook)
	}
	return restoreBusyRun{
		done:    done,
		busy:    busy,
		release: release,
		cleanup: cleanup,
	}
}

func TestCommandBootstrapRestoreCancelsDuringPersistentSQLiteWriterLock(t *testing.T) {
	a := preparePersistentRestoreBusyFixture(t)

	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatalf("fixture sem mapa operacional: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	run := startRestoreWithPersistentSQLiteWriterLock(t, a, ctx)
	defer run.cleanup()
	select {
	case <-run.busy:
	case <-time.After(2 * time.Second):
		t.Fatal("restore não encontrou lock SQLite real")
	}
	cancel()

	select {
	case err := <-run.done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("restore sob lock ignorou cancelamento: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("restore sob lock não encerrou após cancelamento")
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err == nil {
		t.Fatal("cancelamento deixou mapa operacional publicado")
	}
}

func TestCommandBootstrapRestorePersistentSQLiteWriterLockDoesNotPublishMap(t *testing.T) {
	a := preparePersistentRestoreBusyFixture(t)

	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatalf("fixture sem mapa operacional: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	run := startRestoreWithPersistentSQLiteWriterLock(t, a, ctx)
	defer run.cleanup()
	select {
	case <-run.busy:
	case <-ctx.Done():
		t.Fatal("restore não encontrou lock SQLite real")
	}
	err := <-run.done
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock persistente não respeitou deadline: %v", err)
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err == nil {
		t.Fatal("lock persistente deixou mapa operacional publicado")
	}
}

func TestCommandBootstrapRestoreSQLiteBusyReleasesGateBeforeOSLockDeniesNextAttempt(t *testing.T) {
	a := preparePersistentRestoreBusyFixture(t)
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatalf("fixture sem mapa operacional: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	run := startRestoreWithPersistentSQLiteWriterLock(t, a, ctx)
	defer run.cleanup()
	select {
	case <-run.busy:
	case <-ctx.Done():
		t.Fatal("restore não encontrou lock SQLite real")
	}

	// O retry aguarda fora do gate de segurança: a observação de lock do SO
	// deve concluir enquanto a tentativa SQL está ocupada.
	osStateDone := make(chan error, 1)
	go func() { osStateDone <- a.commandHost.SetOSSessionState(ctx, true, true) }()
	select {
	case err := <-osStateDone:
		if err != nil {
			t.Fatalf("bloqueio do SO durante retry: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("SetOSSessionState ficou bloqueado pelo retry SQLite")
	}

	run.release()
	select {
	case err := <-run.done:
		if !errors.Is(err, commandexecution.ErrDenied) {
			t.Fatalf("próxima tentativa após lock do SO não foi negada: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("restore não encerrou após lock do SO")
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err == nil {
		t.Fatal("lock do SO deixou mapa operacional publicado")
	}
}

func TestCommandBootstrapRestoreSQLiteBusyRejectsNextAttemptAfterIdentityChange(t *testing.T) {
	a := preparePersistentRestoreBusyFixture(t)
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatalf("fixture sem mapa operacional: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	run := startRestoreWithPersistentSQLiteWriterLock(t, a, ctx)
	defer run.cleanup()
	select {
	case <-run.busy:
	case <-ctx.Done():
		t.Fatal("restore não encontrou lock SQLite real")
	}

	a.setCurrentAuthUser(nil)
	run.release()
	select {
	case err := <-run.done:
		if !errors.Is(err, commandexecution.ErrDenied) {
			t.Fatalf("próxima tentativa após troca de identidade não foi negada: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("restore não encerrou após troca de identidade")
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err == nil {
		t.Fatal("troca de identidade deixou mapa operacional publicado")
	}
}
