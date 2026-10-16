package app

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandruntime"
)

func TestCommandLifecycleRejectedMountDoesNotInstallPartialDependencies(t *testing.T) {
	app, inputs := appLifecycleProductMountFixture(t)
	// Este teste cobre a montagem que cria o host; a fixture produtiva padrão
	// já entrega um HostState desbloqueado instalado.
	app.commandHost = nil
	invalid := inputs
	invalid.Runtime.Recovery = nil
	if err := ConfigureCommandLifecycleForApp(app, invalid); !errors.Is(err, commandruntime.ErrInvalidConfiguration) {
		t.Fatalf("montagem inválida: %v", err)
	}
	if app.commandHost != nil || app.commandBridge.Load() != nil || app.commandLifecycle.Load() != nil {
		t.Fatal("montagem rejeitada publicou dependências parciais")
	}
	if err := ConfigureCommandLifecycleForApp(app, inputs); err != nil {
		t.Fatalf("nova tentativa válida bloqueada pela tentativa anterior: %v", err)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
}

func TestCommandLifecycleBridgeConflictDoesNotInstallHost(t *testing.T) {
	app, inputs := appLifecycleProductMountFixture(t)
	// O conflito deve ser avaliado antes de instalar um host novo.
	app.commandHost = nil
	_, other := appLifecycleProductMountFixture(t)
	if err := ConfigureCommandBridge(app, other.Bridge); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureCommandLifecycleForApp(app, inputs); !errors.Is(err, errCommandBridgeAlreadyConfigured) {
		t.Fatalf("conflito de ponte: %v", err)
	}
	if app.commandHost != nil || app.commandLifecycle.Load() != nil || app.commandBridge.Load() != other.Bridge {
		t.Fatal("conflito alterou montagem existente ou instalou host parcial")
	}
	if err := app.shutdownCommandBridgeIfConfigured(context.Background()); err != nil {
		t.Fatal(err)
	}
}
