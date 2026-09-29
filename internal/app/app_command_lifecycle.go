package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"time"

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
	// Uma montagem recusada não pode instalar parcialmente host/monitor/bridge
	// e impedir a tentativa seguinte. O worker novo começa frio e nenhuma porta
	// é chamada por NewMounted; publique tudo no mesmo lock usado pelo shutdown.
	a.commandLifecycleMount.Lock()
	defer a.commandLifecycleMount.Unlock()
	if a.commandLifecycleClosing {
		return commandruntime.ErrStopped
	}
	if a.commandLifecycle.Load() != nil {
		return errCommandLifecycleAlreadyConfigured
	}
	if bridge := a.commandBridge.Load(); bridge != nil && bridge != inputs.Bridge {
		return errCommandBridgeAlreadyConfigured
	}
	a.authMu.Lock()
	defer a.authMu.Unlock()
	if a.commandHost != nil && a.commandHost != inputs.Host {
		return commandexecution.ErrInvalidConfiguration
	}
	runtime, err := commandruntime.NewMounted(spec)
	if err != nil {
		return err
	}
	a.commandHost = inputs.Host
	a.commandBridge.Store(inputs.Bridge)
	a.commandLifecycle.Store(runtime)
	a.startCommandOSSessionMonitorLocked()
	return nil
}

func commandLifecycleRuntimeConfigEmpty(config commandruntime.Config) bool {
	return config.Authenticator == nil && config.Recovery == nil && config.Projector == nil &&
		config.Publisher == nil && config.Inputs == nil && config.Core == nil &&
		config.Generations == nil && config.Readiness == nil && config.CleanupTimeout == 0
}

func (a *App) commandLifecycleMountSpec(inputs CommandLifecycleMountInputs) (commandruntime.MountSpec, error) {
	if a == nil {
		return commandruntime.MountSpec{}, fmt.Errorf("%w: app", commandruntime.ErrMissingDependency)
	}
	execution := inputs.Execution
	for _, dependency := range []struct {
		name  string
		value any
	}{
		{"host", inputs.Host}, {"bridge", inputs.Bridge}, {"context-fact-bus", inputs.Facts},
		{"adapter", inputs.Adapter}, {"registry", execution.Registry}, {"ledger-store", execution.Store},
		{"authorize", execution.Authorize}, {"envelope", execution.Envelope},
	} {
		if nilCommandMountDependency(dependency.value) {
			return commandruntime.MountSpec{}, fmt.Errorf("%w: %s", commandruntime.ErrMissingDependency, dependency.name)
		}
	}
	for _, requirement := range []struct {
		name  string
		valid bool
	}{
		{"handlers", len(execution.Handlers) > 0}, {"registry-version", execution.RegistryVersion != ""},
		{"retention", execution.Retention > 0}, {"execution-timeout", execution.ExecutionTimeout > 0},
		{"finalization-timeout", execution.FinalizationTimeout > 0},
	} {
		if !requirement.valid {
			return commandruntime.MountSpec{}, fmt.Errorf("%w: %s", commandruntime.ErrMissingDependency, requirement.name)
		}
	}
	a.authMu.RLock()
	presenter := (*commandDecisionPresenter)(nil)
	if a.questionnaireMgr != nil {
		presenter = &commandDecisionPresenter{manager: a.questionnaireMgr}
	}
	storageVersion := a.commandStorageVersion
	storageErr := a.commandStorageErr
	a.authMu.RUnlock()
	if presenter == nil {
		return commandruntime.MountSpec{}, fmt.Errorf("%w: decision-presenter", commandruntime.ErrMissingDependency)
	}
	if storageErr != nil || storageVersion == "" {
		return commandruntime.MountSpec{}, fmt.Errorf("%w: command-storage", commandruntime.ErrMissingDependency)
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
	// Chamadores sem resultado síncrono conservam o diagnóstico de cada etapa.
	_ = a.tryBootstrapCommandLifecycleAfterAuth(ctx, result, authErr)
}

func (a *App) tryBootstrapCommandLifecycleAfterAuth(ctx context.Context, result *AuthUser, authErr error) error {
	if a == nil {
		return nil
	}
	// Uma recarga explícita deve retirar o mapa antigo mesmo se seu contexto
	// já foi cancelado. O worker de observação usa aquisição cancelável abaixo.
	_ = a.lockCommandBootstrap(context.Background())
	defer a.unlockCommandBootstrap()
	return a.bootstrapCommandLifecycleAfterAuthLocked(ctx, result, authErr)
}

func (a *App) lockCommandBootstrap(ctx context.Context) error {
	a.commandBootstrapOnce.Do(func() { a.commandBootstrap = make(chan struct{}, 1) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case a.commandBootstrap <- struct{}{}:
		if err := ctx.Err(); err != nil {
			a.unlockCommandBootstrap()
			return err
		}
		return nil
	}
}

func (a *App) unlockCommandBootstrap() { <-a.commandBootstrap }

// Serializa a transição completa sem manter authSessionMu/DispatchGate durante
// as portas de bootstrap. O worker de SO usa seu contexto cancelável: parar o
// watcher não pode esperar uma aquisição que o próprio chamador impede.
func (a *App) lockCommandStartup(ctx context.Context) error {
	a.commandStartupOnce.Do(func() { a.commandStartup = make(chan struct{}, 1) })
	select {
	case <-ctx.Done():
		return ctx.Err()
	case a.commandStartup <- struct{}{}:
		if err := ctx.Err(); err != nil {
			a.unlockCommandStartup()
			return err
		}
		return nil
	}
}

func (a *App) unlockCommandStartup() { <-a.commandStartup }

func (a *App) bootstrapCommandLifecycleAfterOSUnlock(ctx context.Context, host *commandexecution.HostState) {
	if err := a.lockCommandStartup(ctx); err != nil {
		return
	}
	defer a.unlockCommandStartup()
	if err := a.lockCommandBootstrap(ctx); err != nil {
		return
	}
	var result *AuthUser
	var bootstrapOK bool
	defer func() {
		a.unlockCommandBootstrap()
		if bootstrapOK {
			if err := a.startPreparedJobsAfterCommands(ctx, result); err != nil {
				logging.Warnf(ctx, "app.commands", "jobs pendentes após desbloqueio do SO: %v", err)
			}
		}
	}()
	a.authMu.RLock()
	sessions, credentials := a.sessionSvc, a.credMgr
	if a.commandHost == host && a.currentAuthUser != nil && a.commandLifecycle.Load() != nil {
		copy := *a.currentAuthUser
		result = &copy
	}
	a.authMu.RUnlock()
	if ctx.Err() != nil || result == nil || sessions == nil {
		return
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil || principal.UserID != result.UserID || principal.SessionID != result.SessionID {
		return
	}
	current, err := sessions.RevalidateLocalSession(ctx, principal)
	if err != nil || current != principal || !a.commandPrincipalMatches(sessions, credentials, principal) {
		return
	}
	// Outro bootstrap pode ter incorporado esta observação enquanto o worker
	// aguardava a transição. Não retirar uma publicação ainda válida após Start.
	if snapshot, err := CommandLifecycleSnapshot(a); err == nil && snapshot.State == commandruntime.StateReady && snapshot.Published {
		if versions, err := host.Snapshot(ctx, principal); err == nil && versions.Unlocked {
			bootstrapOK = true
			return
		}
	}
	bootstrapOK = a.bootstrapCommandLifecycleAfterAuthLocked(ctx, result, nil) == nil
}

func (a *App) bootstrapCommandLifecycleAfterAuthLocked(ctx context.Context, result *AuthUser, authErr error) error {
	if authErr != nil || result == nil || !a.authResultStillCurrent(result) {
		return authErr
	}
	if _, ok := loadCommandLifecycle(a); !ok || a.commandProduct.Load() != nil {
		if err := a.ensureCommandLifecycleMountedForCurrentUser(ctx); err != nil {
			logging.Warnf(context.Background(), "app.app", "ciclo de vida de comandos não montado após autenticação: %v", err)
			return err
		}
	}
	// Retire a publicação anterior antes de tentar recarregar. Uma falha de
	// storage não pode deixar a geração anterior habilitada nem republicá-la.
	// O cancelamento da autenticação não cancela a limpeza de segurança.
	cleanupCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	resetErr := ResetCommandLifecycle(cleanupCtx, a, "configuration_reload")
	a.authMu.RLock()
	host := a.commandHost
	a.authMu.RUnlock()
	if host != nil {
		resetErr = errors.Join(resetErr, host.ForgetUserConfiguration(cleanupCtx, result.UserID))
	}
	cancel()
	if resetErr != nil {
		logging.Errorf(context.Background(), "app.app", "runtime de comandos não pôde ser desabilitado antes da recarga: %v", resetErr)
		return resetErr
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		logging.Warnf(context.Background(), "app.app", "configuração inicial de comandos indisponível após autenticação: %v", err)
		return err
	}
	if err := a.bootstrapCommandLifecycleIfConfigured(ctx); err != nil {
		logging.Errorf(context.Background(), "app.app", "ciclo de vida de comandos indisponível após autenticação: %v", err)
		return err
	}
	// A publicação da projeção antecede a habilitação do lifecycle. Avise
	// novamente quando a UI já pode ler o mapa, inclusive após a primeira
	// observação do SO ou um unlock tardio, sem depender de novo login.
	if a.emitter != nil {
		a.emitter.Emit("command:keyboard-map-changed", nil)
	}
	logging.Infof(ctx, "app.commands", "Configuração de comandos publicada após revalidação da sessão")
	return nil
}

// Não chamar sob authSessionMu: o bootstrap tem seu próprio mutex e os
// chamadores de mutação de perfil já usam a ordem authSessionMu -> bootstrap.
// Solte bootstrap antes de serializar Start com autenticação.
func (a *App) bootstrapCommandsAndStartPreparedJobs(ctx context.Context, user *AuthUser, authErr error) error {
	if err := a.lockCommandStartup(ctx); err != nil {
		return err
	}
	defer a.unlockCommandStartup()
	return a.bootstrapCommandsAndStartPreparedJobsInTransition(ctx, user, authErr)
}

// Login/refresh/retry já possuem commandStartup desde antes da preparação.
func (a *App) bootstrapCommandsAndStartPreparedJobsInTransition(ctx context.Context, user *AuthUser, authErr error) error {
	if err := a.tryBootstrapCommandLifecycleAfterAuth(ctx, user, authErr); err != nil {
		return err
	}
	return a.startPreparedJobsAfterCommands(ctx, user)
}

// Serializa com logout/troca de usuário. A preparação acontece sob o mesmo
// mutex; um bootstrap atrasado nunca inicia os jobs da próxima sessão.
func (a *App) startPreparedJobsAfterCommands(ctx context.Context, user *AuthUser) error {
	a.authSessionMu.Lock()
	defer a.authSessionMu.Unlock()
	pending := a.commandJobsPending
	if pending == nil || a.jobMgr == nil {
		return nil
	}
	if user == nil || pending.UserID != user.UserID || pending.SessionID != user.SessionID || !a.authResultStillCurrent(user) {
		return commandexecution.ErrStale
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	// Mesma ordem dos editores de perfil: authSessionMu -> bootstrap. Uma
	// atualização MCP entre publicação e Start termina antes de verificar
	// readiness; não deixa jobs pendentes por observar um estado intermediário.
	if err := a.lockCommandBootstrap(ctx); err != nil {
		return err
	}
	defer a.unlockCommandBootstrap()
	snapshot, err := CommandLifecycleSnapshot(a)
	if err != nil {
		return err
	}
	if snapshot.State != commandruntime.StateReady || !snapshot.Published {
		return commandexecution.ErrInvalidConfiguration
	}
	if err := a.jobMgr.Start(); err != nil {
		logging.Errorf(ctx, "app.commands", "jobs não iniciados após publicação dos comandos: %v", err)
		return err
	}
	a.commandJobsPending = nil
	return nil
}

func (a *App) bootstrapCommandLifecycleAfterUnlock(ctx context.Context) {
	if a == nil {
		return
	}
	a.authMu.RLock()
	var result *AuthUser
	if a.currentAuthUser != nil {
		copy := *a.currentAuthUser
		result = &copy
	}
	a.authMu.RUnlock()
	if err := a.bootstrapCommandsAndStartPreparedJobs(ctx, result, nil); err != nil {
		logging.Warnf(ctx, "app.commands", "runtime pendente após desbloqueio do cofre: %v", err)
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
	a.invalidateCommandProjectionRecovery()
	runtime, ok := loadCommandLifecycle(a)
	if !ok {
		return commandruntime.ErrInvalidConfiguration
	}
	return runtime.Reset(ctx, reason)
}

// resetCommandLifecycleIfConfigured é o hook de logout/troca de principal.
// Ausência de montagem preserva integralmente o fluxo legado.
func (a *App) resetCommandLifecycleIfConfigured(ctx context.Context, reason string) error {
	if a != nil {
		a.resetCommandHostSession(false)
	}
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
	a.resetCommandHostSession(false)
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
	a.resetCommandHostSession(false)
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
