package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/credentials"
)

func TestGetAuthStatusWaitsForStartupBeforeReadingServices(t *testing.T) {
	db := setupAuthAppTestDB(t)
	// :memory: pertence à conexão; duas consultas não podem abrir bancos distintos.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	a := NewApp()
	results := make(chan error, 2)
	for range 2 {
		go func() { _, err := a.GetAuthStatus(); results <- err }()
	}
	select {
	case err := <-results:
		t.Fatalf("status respondeu antes do startup: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	// Estes campos ainda não existiam quando os bindings foram chamados.
	a.ctx = context.Background()
	a.credStore = credentials.NewDBStore()
	a.identitySvc = auth.NewIdentityService(db)
	a.vaultSvc = auth.NewVaultService(a.credStore, nil)
	a.startupResult.finish(nil)
	for range 2 {
		select {
		case err := <-results:
			if err != nil {
				t.Fatalf("status não retomou automaticamente: %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("status continuou esperando após o startup")
		}
	}
}

func TestStartupFailureReleasesAuthStatusWaiters(t *testing.T) {
	previous := InitDatabase
	t.Cleanup(func() { InitDatabase = previous })
	entered, release := make(chan struct{}), make(chan struct{})
	want := errors.New("banco indisponível no teste")
	InitDatabase = func() error { close(entered); <-release; return want }
	a := NewApp()
	startupDone := make(chan error, 1)
	go func() { startupDone <- a.StartupWithAdapters(context.Background(), nil, nil, nil) }()
	<-entered
	statusDone := make(chan error, 1)
	go func() { _, err := a.GetAuthStatus(); statusDone <- err }()
	select {
	case err := <-statusDone:
		t.Fatalf("status respondeu durante startup: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	if err := <-startupDone; !errors.Is(err, want) {
		t.Fatalf("startup: %v", err)
	}
	select {
	case err := <-statusDone:
		if !errors.Is(err, want) {
			t.Fatalf("falha real foi perdida: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("status não recebeu falha do startup")
	}
	// Nova tentativa devolve a mesma falha, sem esperar um segundo startup.
	if _, err := a.GetAuthStatus(); !errors.Is(err, want) {
		t.Fatalf("retry: %v", err)
	}
}

func TestStartupReadinessCancellationWinsOverLateCompletion(t *testing.T) {
	a := NewApp()
	done := make(chan error, 1)
	go func() { _, err := a.GetAuthStatus(); done <- err }()
	a.Shutdown()
	a.startupResult.finish(nil)
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelamento perdido: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("status não foi cancelado")
	}
}
