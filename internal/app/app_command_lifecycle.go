package app

import (
	"context"
	"errors"

	"assistente/internal/commandruntime"
)

var errCommandLifecycleAlreadyConfigured = errors.New("ciclo de vida de comandos já configurado")

// ConfigureCommandLifecycle registra uma única montagem de ciclo de vida para
// o App. O principal deve fornecer adapters reais de autenticação, recovery,
// projeção, publicação, entradas, geração e readiness. A função não é método
// Wails e não inicia comandos por conta própria.
func ConfigureCommandLifecycle(a *App, config commandruntime.Config) error {
	if a == nil {
		return commandruntime.ErrInvalidConfiguration
	}
	runtime, err := commandruntime.New(config)
	if err != nil {
		return err
	}
	if !a.commandLifecycle.CompareAndSwap(nil, runtime) {
		_ = runtime.Stop(context.Background())
		return errCommandLifecycleAlreadyConfigured
	}
	return nil
}

// BootstrapCommandLifecycle executa a cadeia autenticar → recuperar →
// projetar → publicar → habilitar entradas. É serializado pelo runtime e só
// retorna sucesso depois de readiness publicada para uma projeção não vazia.
// O principal deve chamá-lo fora de authMu, authSessionMu e do DispatchGate;
// os adapters também não podem readquirir esses locks a partir de callbacks.
func BootstrapCommandLifecycle(ctx context.Context, a *App) error {
	runtime, ok := loadCommandLifecycle(a)
	if !ok {
		return commandruntime.ErrInvalidConfiguration
	}
	return runtime.Bootstrap(ctx)
}

// ResetCommandLifecycle cancela a sessão em memória e invalida sua geração.
// Persistência não é apagada e nenhum mapa vazio é anunciado como pronto.
// Deve ser chamado pelo principal fora de authMu, authSessionMu e do
// DispatchGate, após a autenticação/transição que motivou o reset terminar.
func ResetCommandLifecycle(ctx context.Context, a *App, reason string) error {
	runtime, ok := loadCommandLifecycle(a)
	if !ok {
		return commandruntime.ErrInvalidConfiguration
	}
	return runtime.Reset(ctx, reason)
}

// ShutdownCommandLifecycle deve ser chamado pelo principal no hook de
// shutdown antes de destruir seus adapters. A ordem interna desabilita
// entradas, limpa publicação em memória e invalida a geração sem manter lock
// do App durante qualquer porta externa.
func ShutdownCommandLifecycle(ctx context.Context, a *App) error {
	runtime, ok := loadCommandLifecycle(a)
	if !ok {
		return commandruntime.ErrInvalidConfiguration
	}
	// Remove somente a instância observada. Isso evita apagar uma montagem
	// concorrente e encerra o runtime destacado sem manter registry global.
	if !a.commandLifecycle.CompareAndSwap(runtime, nil) {
		return errCommandLifecycleAlreadyConfigured
	}
	return runtime.Stop(ctx)
}

func CommandLifecycleSnapshot(a *App) (commandruntime.Snapshot, error) {
	runtime, ok := loadCommandLifecycle(a)
	if !ok {
		return commandruntime.Snapshot{}, commandruntime.ErrInvalidConfiguration
	}
	return runtime.Snapshot(), nil
}

func loadCommandLifecycle(a *App) (*commandruntime.Controller, bool) {
	if a == nil {
		return nil, false
	}
	runtime := a.commandLifecycle.Load()
	return runtime, runtime != nil
}
