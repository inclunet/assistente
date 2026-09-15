package app

import (
	"context"
	"errors"

	"assistente/internal/commandruntime"
	"assistente/internal/logging"
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

// bootstrapCommandLifecycleAtStartup conecta o hook real de startup sem
// transformar a ausência de montagem em falha. Startup ocorre antes de Login
// na instalação normal; portanto, sem uma sessão autenticada, o runtime
// permanece frio e nenhuma entrada é habilitada.
func (a *App) bootstrapCommandLifecycleAtStartup(ctx context.Context) error {
	if _, ok := loadCommandLifecycle(a); !ok {
		return nil
	}
	a.authMu.RLock()
	authenticated := a.currentUserID != "" && a.currentAuthUser != nil
	a.authMu.RUnlock()
	if !authenticated {
		return nil
	}
	return BootstrapCommandLifecycle(ctx, a)
}

// bootstrapCommandLifecycleIfConfigured é usado após uma transição de auth
// bem-sucedida. O chamador deve invocá-lo fora de authSessionMu/authMu e do
// DispatchGate; o controller e as portas confiáveis fazem a revalidação final.
func (a *App) bootstrapCommandLifecycleIfConfigured(ctx context.Context) error {
	if _, ok := loadCommandLifecycle(a); !ok {
		return nil
	}
	return BootstrapCommandLifecycle(ctx, a)
}

// bootstrapCommandLifecycleAfterAuth tenta publicar a nova geração depois de
// uma autenticação já confirmada. Falha do runtime não desfaz a sessão local:
// a transição de autenticação permanece válida e o controller fica fail-closed
// (sem publicação/entradas prontas), preservando o contrato legado do App.
func (a *App) bootstrapCommandLifecycleAfterAuth(ctx context.Context, result *AuthUser, authErr error) {
	if authErr != nil || result == nil || !a.authResultStillCurrent(result) {
		return
	}
	if err := a.bootstrapCommandLifecycleIfConfigured(ctx); err != nil {
		logging.Errorf(context.Background(), "app.app", "ciclo de vida de comandos indisponível após autenticação: %v", err)
	}
}

func (a *App) authResultStillCurrent(result *AuthUser) bool {
	if a == nil || result == nil {
		return false
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	return a.currentUserID == result.UserID && a.currentAuthUser != nil &&
		a.currentAuthUser.UserID == result.UserID &&
		a.currentAuthUser.SessionID == result.SessionID &&
		a.currentAuthUser.Role == result.Role
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

// resetCommandLifecycleIfConfigured é o hook de logout/troca de principal.
// Ausência de montagem preserva integralmente o fluxo legado.
func (a *App) resetCommandLifecycleIfConfigured(ctx context.Context, reason string) error {
	if _, ok := loadCommandLifecycle(a); !ok {
		return nil
	}
	return ResetCommandLifecycle(ctx, a, reason)
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
	// Mantém a instância observada montada até o worker terminar. Timeout não
	// libera dependências nem permite instalar outro worker sobre elas.
	stopErr := runtime.Stop(ctx)
	if err := runtime.WaitStopped(ctx); err != nil {
		if stopErr != nil {
			return errors.Join(stopErr, err)
		}
		return err
	}
	if !a.commandLifecycle.CompareAndSwap(runtime, nil) {
		return errCommandLifecycleAlreadyConfigured
	}
	if errors.Is(stopErr, commandruntime.ErrStopped) {
		// Outra chamada pode ter recebido a resposta do único opStop. O join
		// acima já comprovou o encerramento; preserve eventual erro de cleanup.
		if detail := runtime.Snapshot().LastError; detail != "" {
			return errors.New(detail)
		}
		return nil
	}
	return stopErr
}

// shutdownCommandLifecycleIfConfigured é o hook de shutdown do App. A
// remoção CAS do ponteiro torna chamadas repetidas seguras e evita que uma
// montagem concorrente seja encerrada por engano.
func (a *App) shutdownCommandLifecycleIfConfigured(ctx context.Context) error {
	if _, ok := loadCommandLifecycle(a); !ok {
		return nil
	}
	return ShutdownCommandLifecycle(ctx, a)
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
