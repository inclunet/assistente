package app

import (
	"context"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/ossession"
)

func TestCommandOSBootstrapDoesNotExecuteWithVaultLockedOrRevokedSession(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		name := "vault_locked"
		if revoked {
			name = "session_revoked"
		}
		t.Run(name, func(t *testing.T) {
			a := readyCommandProduct(t)
			ctx := context.Background()
			before, err := a.GetLocalCommandKeyboardMap()
			if err != nil {
				t.Fatal(err)
			}
			if revoked {
				err = database.DB().Model(&database.Session{}).Where("id = ?", a.currentAuthUser.SessionID).Update("revoked_at", time.Now().UTC()).Error
			} else {
				err = a.commandHost.SetVaultUnlocked(ctx, false)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := a.commandHost.SetOSSessionState(ctx, true, false); err != nil {
				t.Fatal(err)
			}
			a.bootstrapCommandLifecycleAfterOSUnlock(ctx, a.commandHost)
			if _, err := a.GetLocalCommandKeyboardMap(); err == nil {
				t.Fatal("unlock liberou teclado sem autoridade")
			}
			if _, err := a.BeginLocalCommandUIKey(before.Generation, LocalCommandShortcut{Version: 1, Code: "KeyH", Modifiers: []string{"Alt"}}, false); err == nil {
				t.Fatal("unlock aceitou atalho sem autoridade")
			}
			var count int64
			if err := database.DB().Table("command_invocations").Count(&count).Error; err != nil || count != 0 {
				t.Fatalf("invocações=%d err=%v", count, err)
			}
		})
	}
}

func TestCommandOSObservationCallbackDoesNotWaitForBootstrapMutex(t *testing.T) {
	a := readyCommandProduct(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	callbackReturned := make(chan struct{})
	watch := func(ctx context.Context, observe func(ossession.State) error) error {
		if err := observe(ossession.State{Known: true}); err != nil {
			return err
		}
		if err := observe(ossession.State{Known: true, Locked: true}); err != nil {
			return err
		}
		close(callbackReturned)
		<-ctx.Done()
		return ctx.Err()
	}
	if err := a.lockCommandBootstrap(ctx); err != nil {
		t.Fatal(err)
	}
	defer a.unlockCommandBootstrap()
	result := make(chan error, 1)
	go func() { result <- a.observeCommandOSSession(ctx, watch) }()
	select {
	case <-callbackReturned:
		if _, err := a.GetLocalCommandKeyboardMap(); err == nil {
			t.Fatal("lock não invalidou mapa enquanto bootstrap ocupado")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("callback do monitor aguardou bootstrap")
	}
	cancel()
	select {
	case <-result:
	case <-time.After(3 * time.Second):
		t.Fatal("monitor não encerrou após cancelamento")
	}
}

func TestCommandOSSessionMonitorJoinsBootstrapWorkerOnShutdown(t *testing.T) {
	a := readyCommandProduct(t)
	a.ctx, a.cancel = context.WithCancel(context.Background())
	defer a.cancel()
	started := make(chan struct{})
	watch := func(ctx context.Context, observe func(ossession.State) error) error {
		if err := observe(ossession.State{Known: true}); err != nil {
			return err
		}
		close(started)
		<-ctx.Done()
		return ctx.Err()
	}
	a.authMu.Lock()
	a.startCommandOSSessionMonitorWithLocked(watch)
	a.authMu.Unlock()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("monitor não iniciou")
	}
	a.cancel()
	joined := make(chan struct{})
	go func() { a.bgWG.Wait(); close(joined) }()
	select {
	case <-joined:
	case <-time.After(3 * time.Second):
		t.Fatal("worker do monitor não encerrou")
	}
}
