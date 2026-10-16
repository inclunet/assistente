package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/database"
)

func TestCommandProjectionWaitCancellationAndRecovery(t *testing.T) {
	p := &commandProductRuntime{}
	release, err := p.acquireCommandProjection(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if release != nil {
			release()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	called := false
	err = p.withCommandProjection(ctx, func(context.Context) error { called = true; return nil })
	if !errors.Is(err, context.DeadlineExceeded) || called {
		t.Fatalf("espera não cancelada: %v called=%v", err, called)
	}
	release()
	release = nil
	if err := p.withCommandProjection(context.Background(), func(context.Context) error { called = true; return nil }); err != nil || !called {
		t.Fatalf("projeção não recuperou: %v", err)
	}
}

func TestCommandReadBoundedUnderProjectionAndPoolContention(t *testing.T) {
	for _, resource := range []string{"projection", "security_gate", "connection_pool"} {
		for _, endpoint := range []string{"keyboard", "settings", "rebuild"} {
			if endpoint == "rebuild" && resource != "security_gate" {
				continue
			}
			t.Run(resource+"/"+endpoint, func(t *testing.T) {
				a := readyCommandProduct(t)
				p := a.commandProduct.Load()
				var release func()
				switch resource {
				case "projection":
					var err error
					release, err = p.acquireCommandProjection(context.Background())
					if err != nil {
						t.Fatal(err)
					}
					if err := p.host.ForgetUserConfiguration(context.Background(), p.principal.UserID); err != nil {
						release()
						t.Fatal(err)
					}
				case "security_gate":
					entered, resume, exited := make(chan struct{}), make(chan struct{}), make(chan error, 1)
					go func() {
						exited <- a.commandGate.WithMutation(context.Background(), func() error {
							close(entered)
							<-resume
							return nil
						})
					}()
					<-entered
					release = func() {
						close(resume)
						if err := <-exited; err != nil {
							t.Error(err)
						}
					}
				default:
					db, err := database.DB().DB()
					if err != nil {
						t.Fatal(err)
					}
					maxOpen := db.Stats().MaxOpenConnections
					db.SetMaxOpenConns(1)
					conn, err := db.Conn(context.Background())
					if err != nil {
						db.SetMaxOpenConns(maxOpen)
						t.Fatal(err)
					}
					release = func() { _ = conn.Close(); db.SetMaxOpenConns(maxOpen) }
				}
				defer func() {
					if release != nil {
						release()
					}
				}()
				load := func() error {
					if endpoint == "rebuild" {
						return a.rebuildCommandLifecycleProjection(context.Background(), false)
					}
					if endpoint == "keyboard" {
						_, err := a.GetLocalCommandKeyboardMap()
						return err
					}
					_, err := a.GetCommandSettings("pt-BR")
					return err
				}
				done := make(chan error, 1)
				go func() { done <- load() }()
				select {
				case err := <-done:
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatalf("esperado deadline sem liberar recurso: %v", err)
					}
				case <-time.After(commandReadTimeout + 2*time.Second):
					release()
					release = nil
					<-done
					t.Fatal("leitura ficou presa além do orçamento")
				}
				release()
				release = nil
				if err := load(); err != nil {
					t.Fatalf("leitura não recuperou após liberar recurso: %v", err)
				}
				p.keyboardMu.Lock()
				defer p.keyboardMu.Unlock()
				if endpoint == "keyboard" && (p.keyboardMap == nil || p.keyboardMap.ctx.Err() != nil) {
					t.Fatal("orçamento de leitura cancelou mapa retido")
				}
			})
		}
	}
}
