package app

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandactivation"
	"assistente/internal/commandexecution"
	"assistente/internal/database"
	"gorm.io/gorm"
)

func TestCommandProjectionRejectsReadAcrossCompletedClaimTransition(t *testing.T) {
	a := preparePersistentRestoreBusyFixture(t)
	p := a.commandProduct.Load()
	if err := p.host.ForgetUserConfiguration(context.Background(), p.principal.UserID); err != nil {
		t.Fatal(err)
	}
	type projectionRead struct{}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), projectionRead{}, true), 5*time.Second)
	defer cancel()
	read, resume := make(chan struct{}), make(chan struct{})
	var once sync.Once
	db := database.DB()
	const hook = "test:projection_transition_read"
	if err := db.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(projectionRead{}) == true && tx.Statement.Table == "command_layer_activation_state" {
			once.Do(func() {
				close(read)
				select {
				case <-resume:
				case <-ctx.Done():
				}
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		done <- a.buildCommandLifecycleProjection(ctx, false, false)
	}()
	defer func() {
		cancel()
		select {
		case <-exited:
			_ = db.Callback().Query().Remove(hook)
		case <-time.After(2 * time.Second):
			t.Error("projeção não encerrou após cancelamento")
		}
	}()
	select {
	case <-read:
	case <-ctx.Done():
		t.Fatal("projeção não leu claims")
	}
	// A leitura capturou a claim com identidade antiga. Restore altera só a
	// identidade da claim persistente, sem alterar a configuração da camada.
	transitionErr := a.restoreCommandLifecyclePersistentClaims(context.Background())
	close(resume)
	buildErr := <-done
	if transitionErr != nil {
		t.Fatal(transitionErr)
	}
	if !errors.Is(buildErr, commandexecution.ErrStale) {
		t.Fatalf("projeção atravessada pela restauração foi publicada: %v", buildErr)
	}
	if _, err := p.host.Snapshot(context.Background(), p.principal); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("snapshot antigo publicado: %v", err)
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatalf("leitura nova não recuperou mapa: %v", err)
	}
}

func TestCommandProjectionCompletesPendingRestoreAfterSQLiteWriterReleased(t *testing.T) {
	a := preparePersistentRestoreBusyFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := startRestoreWithPersistentSQLiteWriterLock(t, a, ctx)
	defer run.cleanup()
	select {
	case <-run.busy:
	case <-time.After(2 * time.Second):
		t.Fatal("restore não encontrou writer SQLite")
	}
	cancel()
	if err := <-run.done; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento: %v", err)
	}
	if a.commandProduct.Load().claimTransition.Load() == nil {
		t.Fatal("restauração incompleta não foi preservada")
	}
	if err := a.reconcileCommandLifecycleClaims(context.Background(), false); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("expiração sobrepôs restauração pendente: %v", err)
	}
	run.release()
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(view.Bindings) == 0 {
		t.Fatalf("mapa não voltou após liberar SQLite: %v", err)
	}
	var claims []commandactivation.Claim
	if err := database.DB().Where("user_id = ? AND source_type = ? AND state = ?", a.currentUserID, "manual", commandactivation.StateActive).Find(&claims).Error; err != nil {
		t.Fatal(err)
	}
	if len(claims) != 1 || claims[0].AuthContextID != a.currentAuthUser.SessionID || a.commandProduct.Load().claimTransition.Load() != nil {
		t.Fatal("recuperação pulou a restauração da claim persistente")
	}
}

func TestCommandProjectionRecoversMissingPublicationAfterDatabaseFailure(t *testing.T) {
	a := commandJobPublicationApp(t)
	if err := a.jobMgr.CloseCommandMaintenance(context.Background()); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	ctx := context.Background()
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatal(err)
	}
	if err := p.host.ForgetUserConfiguration(ctx, p.principal.UserID); err != nil {
		t.Fatal(err)
	}
	type failureMarker struct{}
	failure := errors.New("transient database read failure")
	db := database.DB()
	const hook = "test:missing_projection_read_failure"
	if err := db.Callback().Query().Before("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Context.Value(failureMarker{}) == true {
			_ = tx.AddError(failure)
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Callback().Query().Remove(hook) })
	if err := p.refreshCommandJobProjection(context.WithValue(ctx, failureMarker{}, true)); !errors.Is(err, failure) {
		t.Fatalf("a tentativa não alcançou a reconstrução: %v", err)
	}
	if _, err := p.host.Snapshot(ctx, p.principal); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("falha de leitura publicou mapa: %v", err)
	}
	// A próxima chamada pública deve recuperar o mapa sem reiniciar o app,
	// autenticar novamente ou reaproveitar a configuração suspensa.
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil || len(view.Bindings) == 0 {
		t.Fatalf("mapa não recuperado após a falha: bindings=%d err=%v", len(view.Bindings), err)
	}
}

func TestCommandProjectionMissingPublicationDoesNotBypassVaultOrCancellation(t *testing.T) {
	for _, locked := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancelled", true: "vault_locked"}[locked], func(t *testing.T) {
			a := readyCommandProduct(t)
			p := a.commandProduct.Load()
			ctx := context.Background()
			if err := p.host.ForgetUserConfiguration(ctx, p.principal.UserID); err != nil {
				t.Fatal(err)
			}
			if locked {
				if err := p.host.SetVaultUnlocked(ctx, false); err != nil {
					t.Fatal(err)
				}
			} else {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if err := p.refreshCommandJobProjection(ctx); err == nil {
				t.Fatal("recuperação ignorou bloqueio/cancelamento")
			}
			if _, err := p.host.Snapshot(context.Background(), p.principal); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
				t.Fatalf("recuperação publicou mapa indevido: %v", err)
			}
		})
	}
}

func TestCommandProjectionRecoveryCannotCrossOSLockUnlock(t *testing.T) {
	a := preparePersistentRestoreBusyFixture(t)
	p := a.commandProduct.Load()
	if err := p.host.ForgetUserConfiguration(context.Background(), p.principal.UserID); err != nil {
		t.Fatal(err)
	}
	db := database.DB()
	const hook = "test:recovery_os_transition"
	var once sync.Once
	transitioned := false
	var transitionErr error
	if err := db.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "command_layer_activation_state" {
			once.Do(func() {
				transitioned = true
				transitionErr = p.host.SetOSSessionState(context.Background(), true, true)
				if transitionErr == nil {
					transitionErr = p.host.SetOSSessionState(context.Background(), true, false)
				}
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Callback().Query().Remove(hook) }()
	if err := p.refreshCommandJobProjection(context.Background()); err == nil {
		t.Fatal("recuperação atravessou mudança de segurança do SO")
	}
	if transitionErr != nil {
		t.Fatal(transitionErr)
	}
	if !transitioned {
		t.Fatal("recuperação não alcançou a transição de SO injetada")
	}
	if _, err := p.host.Snapshot(context.Background(), p.principal); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("mapa foi republicado antes do bootstrap de unlock: %v", err)
	}
}

func TestCommandProjectionSettingsAuthorityCannotReplacePublishedSecurityProof(t *testing.T) {
	a, binding := persistentGlobalVoiceExecutionFixture(t)
	p := a.commandProduct.Load()
	ctx := context.Background()
	if err := p.host.SetOSSessionState(ctx, true, true); err != nil {
		t.Fatal(err)
	}
	if err := p.host.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	// A leitura das configurações pode capturar a autoridade atual, mas isso
	// não equivale à publicação de uma configuração pelo bootstrap de unlock.
	if _, _, err := p.commandManualClaimAuthority(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.prepareGlobalOccurrence(ctx, binding, func() bool { return true }); err == nil {
		t.Fatal("leitura renovou indevidamente prova da publicação anterior")
	}
	a.bootstrapCommandLifecycleAfterOSUnlock(ctx, a.commandHost)
	if _, err := p.prepareGlobalOccurrence(ctx, binding, func() bool { return true }); err != nil {
		t.Fatalf("bootstrap não liberou nova ocorrência: %v", err)
	}
}

func TestCommandProjectionDeliberateResetRequiresAuthoritativePublication(t *testing.T) {
	a := preparePersistentRestoreBusyFixture(t)
	p := a.commandProduct.Load()
	p.persistedConfigMu.RLock()
	store, snapshot, epoch, revision := p.persistedConfigStore, p.persistedConfigSnapshot, p.persistedConfigEpoch, p.projectionResetRevision.Load()
	p.persistedConfigMu.RUnlock()
	a.resetCommandHostSession(false)
	// Nem o término tardio de uma publicação anterior pode reativar a prova.
	p.rememberPersistedCommandConfiguration(store, snapshot, epoch, revision)
	if err := p.refreshCommandJobProjection(context.Background()); err == nil {
		t.Fatal("reset deliberado foi interpretado como falha transitória")
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err == nil {
		t.Fatal("endpoint republicou antes do bootstrap autoritativo")
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatalf("publicação autoritativa não recuperou mapa: %v", err)
	}
}

func TestCommandProjectionResetCancelsRecoveryAlreadyReading(t *testing.T) {
	a := preparePersistentRestoreBusyFixture(t)
	p := a.commandProduct.Load()
	if err := p.host.ForgetUserConfiguration(context.Background(), p.principal.UserID); err != nil {
		t.Fatal(err)
	}
	db := database.DB()
	const hook = "test:recovery_deliberate_reset"
	var once sync.Once
	reset := false
	if err := db.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
		if tx.Statement.Table == "command_layer_activation_state" {
			once.Do(func() {
				reset = true
				a.resetCommandHostSession(false)
			})
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Callback().Query().Remove(hook) }()
	if err := p.refreshCommandJobProjection(context.Background()); err == nil {
		t.Fatal("recuperação em andamento republicou depois do reset")
	}
	if !reset {
		t.Fatal("recuperação não alcançou o reset injetado")
	}
	if _, err := p.host.Snapshot(context.Background(), p.principal); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("reset deixou mapa republicado: %v", err)
	}
}

func TestCommandProjectionResetRejectsNormalAndJobReads(t *testing.T) {
	for _, job := range []bool{false, true} {
		t.Run(map[bool]string{false: "normal", true: "job"}[job], func(t *testing.T) {
			a := preparePersistentRestoreBusyFixture(t)
			db := database.DB()
			const hook = "test:ordinary_projection_reset"
			var once sync.Once
			reset := false
			if err := db.Callback().Query().After("gorm:query").Register(hook, func(tx *gorm.DB) {
				if tx.Statement.Table == "command_layer_activation_state" {
					once.Do(func() {
						reset = true
						a.resetCommandHostSession(false)
					})
				}
			}); err != nil {
				t.Fatal(err)
			}
			defer func() { _ = db.Callback().Query().Remove(hook) }()
			err := a.rebuildCommandLifecycleProjectionMode(context.Background(), false, job)
			if !reset || !errors.Is(err, commandexecution.ErrStale) {
				t.Fatalf("projeção atravessou reset: reset=%v err=%v", reset, err)
			}
			p := a.commandProduct.Load()
			if _, err := p.host.Snapshot(context.Background(), p.principal); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
				t.Fatalf("reset deixou publicação anterior utilizável: %v", err)
			}
		})
	}
}

func TestCommandProjectionResetSupersedesFailedClaimTransition(t *testing.T) {
	a := preparePersistentRestoreBusyFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	run := startRestoreWithPersistentSQLiteWriterLock(t, a, ctx)
	defer run.cleanup()
	select {
	case <-run.busy:
	case <-time.After(2 * time.Second):
		t.Fatal("restore não encontrou writer")
	}
	cancel()
	if err := <-run.done; !errors.Is(err, context.Canceled) {
		t.Fatalf("restore interrompido: %v", err)
	}
	p := a.commandProduct.Load()
	if p.currentCommandClaimTransition() == nil {
		t.Fatal("transição pendente não registrada")
	}
	previous := p.currentCommandClaimTransition()
	a.resetCommandHostSession(false)
	run.release()
	if err := a.reconcileCommandLifecycleClaimsAtReset(context.Background(), previous.restore, previous.workspaceSwitch, p, previous.resetRevision); !errors.Is(err, commandexecution.ErrStale) {
		t.Fatalf("operação capturada antes do reset foi reaplicada: %v", err)
	}
	if err := a.reconcileCommandLifecycleClaimsForTransition(context.Background(), false, true); err != nil {
		t.Fatalf("reset autoritativo ficou preso na operação antiga: %v", err)
	}
	if err := a.rebuildCommandLifecycleProjection(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
		t.Fatalf("mapa não voltou após nova transição: %v", err)
	}
}
