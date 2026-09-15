package app

import (
	"context"
	"errors"
	"reflect"

	"assistente/internal/commandbridge"
	"assistente/internal/commandcontext"
	"assistente/internal/commandexecution"
	"assistente/internal/commandruntime"
	"assistente/internal/logging"
)

var errCommandLifecycleAlreadyConfigured = errors.New("ciclo de vida de comandos já configurado")

// ConfigureCommandLifecycle registra uma única montagem de ciclo de vida para
// o App. O principal deve fornecer adapters reais de autenticação, recovery,
// projeção, publicação, entradas, geração e readiness. A função não é método
// Wails e não inicia comandos por conta própria.
func ConfigureCommandLifecycle(a *App, config commandruntime.Config) error {
	return configureCommandLifecycleController(a, func() (*commandruntime.Controller, error) {
		return commandruntime.New(config)
	})
}

// ConfigureCommandLifecycleMountSpec registra a montagem final com manifesto
// explícito de dependências. Use esta entrada para I14: ela falha fechado
// quando catálogo/defaults/políticas/stores/presenter/providers/dispatcher ou
// adapters não foram realmente conectados pelo App.
func ConfigureCommandLifecycleMountSpec(a *App, spec commandruntime.MountSpec) error {
	return configureCommandLifecycleController(a, func() (*commandruntime.Controller, error) {
		return commandruntime.NewMounted(spec)
	})
}

// CommandLifecycleMountInputs são as dependências de produto que precisam ser
// conhecidas pelo App antes de aceitar o runtime final. A estrutura deliberadamente
// mistura os objetos concretos usados pelas fábricas existentes em vez de recriar
// stores, políticas ou dispatchers paralelos aqui.
type CommandLifecycleMountInputs struct {
	Runtime commandruntime.Config

	Execution commandexecution.Config
	Host      *commandexecution.HostState
	Bridge    *commandbridge.Bridge
	Facts     *commandcontext.FactBus
	Adapter   any
}

// ConfigureCommandLifecycleForApp monta o manifesto I14.2 a partir das
// dependências reais já preparadas pelo bootstrap confiável e só então instala
// o controller. Ele não registra comandos de produto nem chama Bootstrap.
func ConfigureCommandLifecycleForApp(a *App, inputs CommandLifecycleMountInputs) error {
	if commandLifecycleRuntimeConfigEmpty(inputs.Runtime) {
		runtimePorts, err := newAppCommandLifecycleRuntime(a, inputs)
		if err != nil {
			return err
		}
		inputs.Runtime = runtimePorts.config()
	}
	spec, err := a.commandLifecycleMountSpec(inputs)
	if err != nil {
		return err
	}
	if err := a.installCommandHost(inputs.Host); err != nil {
		return err
	}
	if err := configureCommandBridgeForLifecycle(a, inputs.Bridge); err != nil {
		return err
	}
	return ConfigureCommandLifecycleMountSpec(a, spec)
}

func configureCommandBridgeForLifecycle(a *App, bridge *commandbridge.Bridge) error {
	if a == nil || bridge == nil {
		return commandruntime.ErrMissingDependency
	}
	if current, ok := loadCommandBridge(a); ok {
		if current != bridge {
			return errCommandBridgeAlreadyConfigured
		}
		return nil
	}
	if err := ConfigureCommandBridge(a, bridge); err != nil {
		return err
	}
	return nil
}

func commandLifecycleRuntimeConfigEmpty(config commandruntime.Config) bool {
	return config.Authenticator == nil && config.Recovery == nil && config.Projector == nil &&
		config.Publisher == nil && config.Inputs == nil && config.Core == nil &&
		config.Generations == nil && config.Readiness == nil && config.CleanupTimeout == 0
}

func (a *App) commandLifecycleMountSpec(inputs CommandLifecycleMountInputs) (commandruntime.MountSpec, error) {
	if a == nil || inputs.Host == nil || inputs.Bridge == nil || inputs.Facts == nil || nilCommandMountDependency(inputs.Adapter) {
		return commandruntime.MountSpec{}, commandruntime.ErrMissingDependency
	}
	execution := inputs.Execution
	if execution.Registry == nil || len(execution.Handlers) == 0 || execution.Store == nil || execution.Authorize == nil || execution.Envelope == nil {
		return commandruntime.MountSpec{}, commandruntime.ErrMissingDependency
	}
	if execution.RegistryVersion == "" || execution.Retention <= 0 || execution.ExecutionTimeout <= 0 || execution.FinalizationTimeout <= 0 {
		return commandruntime.MountSpec{}, commandruntime.ErrMissingDependency
	}
	a.authMu.RLock()
	presenter := (*commandDecisionPresenter)(nil)
	if a.questionnaireMgr != nil {
		presenter = &commandDecisionPresenter{manager: a.questionnaireMgr}
	}
	storageVersion := a.commandStorageVersion
	storageErr := a.commandStorageErr
	a.authMu.RUnlock()
	if presenter == nil || storageErr != nil || storageVersion == "" {
		return commandruntime.MountSpec{}, commandruntime.ErrMissingDependency
	}
	if inputs.Host.Epochs() != execution.Epochs {
		return commandruntime.MountSpec{}, commandruntime.ErrInvalidConfiguration
	}
	return commandruntime.MountSpec{
		Config: inputs.Runtime,
		Dependencies: []commandruntime.MountDependency{
			{Role: commandruntime.MountDependencyCatalog, Name: execution.RegistryVersion, Instance: execution.Registry},
			{Role: commandruntime.MountDependencyDefaults, Name: "execution-envelope", Instance: execution.Envelope},
			{Role: commandruntime.MountDependencyPolicies, Name: "execution-authorize", Instance: execution.Authorize},
			{Role: commandruntime.MountDependencyStores, Name: storageVersion, Instance: execution.Store},
			{Role: commandruntime.MountDependencyPresenter, Name: "decision-presenter", Instance: presenter},
			{Role: commandruntime.MountDependencyProviders, Name: "context-fact-bus", Instance: inputs.Facts},
			{Role: commandruntime.MountDependencyDispatcher, Name: "command-bridge", Instance: inputs.Bridge},
			{Role: commandruntime.MountDependencyAdapters, Name: string(execution.Source), Instance: inputs.Adapter},
		},
	}, nil
}

func nilCommandMountDependency(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

func configureCommandLifecycleController(a *App, build func() (*commandruntime.Controller, error)) error {
	if a == nil {
		return commandruntime.ErrInvalidConfiguration
	}
	// Não construir um worker que já sabemos que perderá a montagem: parar
	// esse candidato exigiria callbacks externos mesmo sem nunca ter sido usado.
	// New apenas valida/cria o worker frio; nenhuma porta roda sob este mutex.
	a.commandLifecycleMount.Lock()
	defer a.commandLifecycleMount.Unlock()
	if a.commandLifecycle.Load() != nil {
		return errCommandLifecycleAlreadyConfigured
	}
	if a.commandLifecycleClosing {
		return commandruntime.ErrStopped
	}
	runtime, err := build()
	if err != nil {
		return err
	}
	a.commandLifecycle.Store(runtime)
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
	if _, ok := loadCommandLifecycle(a); !ok {
		if err := a.ensureCommandLifecycleMountedForCurrentUser(ctx); err != nil {
			logging.Warnf(context.Background(), "app.app", "ciclo de vida de comandos não montado após autenticação: %v", err)
			return
		}
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		logging.Warnf(context.Background(), "app.app", "configuração inicial de comandos indisponível após autenticação: %v", err)
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
	if a == nil || ctx == nil {
		return commandruntime.ErrInvalidConfiguration
	}
	// Fechamento terminal compete com a montagem no mesmo mutex curto.
	// Não manter esse lock durante Stop/WaitStopped ou portas externas.
	a.commandLifecycleMount.Lock()
	a.commandLifecycleClosing = true
	runtime := a.commandLifecycle.Load()
	a.commandLifecycleMount.Unlock()
	if runtime == nil {
		return commandruntime.ErrInvalidConfiguration
	}
	return a.shutdownMountedCommandLifecycle(ctx, runtime)
}

func (a *App) shutdownMountedCommandLifecycle(ctx context.Context, runtime *commandruntime.Controller) error {
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
	if a == nil || ctx == nil {
		return commandruntime.ErrInvalidConfiguration
	}
	a.commandLifecycleMount.Lock()
	a.commandLifecycleClosing = true
	runtime := a.commandLifecycle.Load()
	a.commandLifecycleMount.Unlock()
	if runtime == nil {
		return nil
	}
	return a.shutdownMountedCommandLifecycle(ctx, runtime)
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
