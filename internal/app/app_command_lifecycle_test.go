package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandruntime"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
)

type appLifecyclePort struct {
	generation   commandruntime.Generation
	mu           sync.Mutex
	app          *App
	lockHeld     atomic.Bool
	failAuth     bool
	clearStarted chan struct{}
	clearRelease chan struct{}
	clearOnce    sync.Once
}

func (p *appLifecyclePort) checkLocks() {
	if p.app == nil {
		return
	}
	if p.app.authMu.TryLock() {
		p.app.authMu.Unlock()
	} else {
		p.lockHeld.Store(true)
	}
	if p.app.authSessionMu.TryLock() {
		p.app.authSessionMu.Unlock()
	} else {
		p.lockHeld.Store(true)
	}
}

func (p *appLifecyclePort) Authenticate(context.Context) error {
	p.checkLocks()
	if p.failAuth {
		return errors.New("autenticação de teste falhou")
	}
	return nil
}
func (p *appLifecyclePort) Recover(context.Context, commandruntime.Generation) error {
	p.checkLocks()
	return nil
}
func (p *appLifecyclePort) Project(_ context.Context, generation commandruntime.Generation) (commandruntime.Projection, error) {
	p.checkLocks()
	return commandruntime.Projection{Generation: generation, Entries: 1, Value: struct{}{}}, nil
}
func (p *appLifecyclePort) Publish(context.Context, commandruntime.Projection) error {
	p.checkLocks()
	return nil
}
func (p *appLifecyclePort) Clear(context.Context, commandruntime.Generation) error {
	p.checkLocks()
	if p.clearStarted != nil {
		p.clearOnce.Do(func() { close(p.clearStarted) })
		<-p.clearRelease
	}
	return nil
}
func (p *appLifecyclePort) SetEnabled(_ context.Context, generation commandruntime.Generation, enabled bool) error {
	p.checkLocks()
	if enabled {
		p.mu.Lock()
		p.generation = generation
		p.mu.Unlock()
	}
	return nil
}
func (p *appLifecyclePort) ValidateCurrent(context.Context, commandruntime.Generation, commandruntime.Boundary) error {
	p.checkLocks()
	return nil
}
func (p *appLifecyclePort) Authorize(context.Context, commandruntime.Generation, commandruntime.Boundary) error {
	p.checkLocks()
	return nil
}
func (p *appLifecyclePort) Commit(_ context.Context, _ commandruntime.Generation, commit func() error) error {
	p.checkLocks()
	return commit()
}
func (p *appLifecyclePort) Begin(context.Context) (commandruntime.Generation, error) {
	p.checkLocks()
	generation := commandruntime.Generation{Value: "trusted-generation"}
	p.mu.Lock()
	p.generation = generation
	p.mu.Unlock()
	return generation, nil
}
func (p *appLifecyclePort) Invalidate(context.Context, commandruntime.Generation, string) error {
	p.checkLocks()
	return nil
}
func (p *appLifecyclePort) PublishReadiness(context.Context, commandruntime.Snapshot) error {
	p.checkLocks()
	return nil
}

type appLifecycleReadinessPort func(context.Context, commandruntime.Snapshot) error

func (f appLifecycleReadinessPort) Publish(ctx context.Context, snapshot commandruntime.Snapshot) error {
	return f(ctx, snapshot)
}

func appLifecycleConfig(probe *appLifecyclePort) commandruntime.Config {
	return commandruntime.Config{
		Authenticator: probe,
		Recovery:      probe,
		Projector:     probe,
		Publisher:     probe,
		Inputs:        probe,
		Core:          probe,
		Generations:   probe,
		Readiness:     appLifecycleReadinessPort(probe.PublishReadiness),
	}
}

func TestAppCommandLifecycleBridgeRequiresMountedRuntimeAndRunsTransitions(t *testing.T) {
	app := &App{}
	if _, err := CommandLifecycleSnapshot(app); err == nil {
		t.Fatal("snapshot criou runtime sem bootstrap confiável")
	}
	probe := &appLifecyclePort{}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(probe)); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	ready, err := CommandLifecycleSnapshot(app)
	if err != nil || ready.State != commandruntime.StateReady || !ready.Published {
		t.Fatalf("bridge não publicou readiness: %+v, %v", ready, err)
	}
	if err := ResetCommandLifecycle(context.Background(), app, "logout"); err != nil {
		t.Fatal(err)
	}
	if snapshot, _ := CommandLifecycleSnapshot(app); snapshot.State != commandruntime.StateCold || snapshot.Published {
		t.Fatalf("bridge reteve estado após reset: %+v", snapshot)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	if _, err := CommandLifecycleSnapshot(app); err == nil {
		t.Fatal("runtime permaneceu montado após shutdown")
	}
}

func TestAppCommandLifecycleConfigureCASAllowsOnlyOneInstance(t *testing.T) {
	app := &App{}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			<-start
			results <- ConfigureCommandLifecycle(app, appLifecycleConfig(&appLifecyclePort{}))
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, errCommandLifecycleAlreadyConfigured) {
			t.Fatalf("erro inesperado no CAS de configuração: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("configurações vencedoras = %d, esperado 1", successes)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleShutdownCASRemovesSameInstance(t *testing.T) {
	app := &App{}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(&appLifecyclePort{})); err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			results <- ShutdownCommandLifecycle(context.Background(), app)
		}()
	}
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, commandruntime.ErrInvalidConfiguration) && !errors.Is(err, errCommandLifecycleAlreadyConfigured) {
			t.Fatalf("erro inesperado no CAS de shutdown: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("shutdowns vencedores = %d, esperado 1", successes)
	}
	if _, err := CommandLifecycleSnapshot(app); !errors.Is(err, commandruntime.ErrInvalidConfiguration) {
		t.Fatalf("instância permaneceu montada após shutdown: %v", err)
	}
}

func TestAppCommandLifecycleShutdownRetainsMountedWorkerUntilRetry(t *testing.T) {
	app := &App{}
	probe := &appLifecyclePort{app: app, clearStarted: make(chan struct{}), clearRelease: make(chan struct{})}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(probe)); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := ShutdownCommandLifecycle(stopCtx, app); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdown com cleanup bloqueado = %v", err)
	}
	select {
	case <-probe.clearStarted:
	case <-time.After(time.Second):
		t.Fatal("cleanup do shutdown não iniciou")
	}
	if app.commandLifecycle.Load() == nil {
		t.Fatal("controller foi desmontado antes da confirmação do worker")
	}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(&appLifecyclePort{})); !errors.Is(err, errCommandLifecycleAlreadyConfigured) {
		t.Fatalf("reconfiguração aceita com worker antigo vivo: %v", err)
	}
	close(probe.clearRelease)
	retryCtx, retryCancel := context.WithTimeout(context.Background(), time.Second)
	defer retryCancel()
	err := ShutdownCommandLifecycle(retryCtx, app)
	if err != nil && !errors.Is(err, commandruntime.ErrStopped) {
		t.Fatalf("shutdown após release = %v", err)
	}
	if app.commandLifecycle.Load() != nil {
		t.Fatal("controller não foi desmontado após confirmação do worker")
	}
}

func TestAppCommandLifecycleOperationsFailClosedWithoutConfiguration(t *testing.T) {
	app := &App{}
	if err := BootstrapCommandLifecycle(context.Background(), app); !errors.Is(err, commandruntime.ErrInvalidConfiguration) {
		t.Fatalf("bootstrap sem montagem foi aceito: %v", err)
	}
	if err := ResetCommandLifecycle(context.Background(), app, "logout"); !errors.Is(err, commandruntime.ErrInvalidConfiguration) {
		t.Fatalf("reset sem montagem foi aceito: %v", err)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); !errors.Is(err, commandruntime.ErrInvalidConfiguration) {
		t.Fatalf("shutdown sem montagem foi aceito: %v", err)
	}
}

func TestAppCommandLifecycleHooksAreOptionalAndRespectAuthBoundary(t *testing.T) {
	legacy := &App{}
	if err := legacy.bootstrapCommandLifecycleAtStartup(context.Background()); err != nil {
		t.Fatalf("startup legado falhou sem montagem: %v", err)
	}
	if err := legacy.resetCommandLifecycleIfConfigured(context.Background(), "logout"); err != nil {
		t.Fatalf("reset legado falhou sem montagem: %v", err)
	}
	if err := legacy.shutdownCommandLifecycleIfConfigured(context.Background()); err != nil {
		t.Fatalf("shutdown legado falhou sem montagem: %v", err)
	}

	app := &App{}
	probe := &appLifecyclePort{app: app}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(probe)); err != nil {
		t.Fatal(err)
	}
	if err := app.bootstrapCommandLifecycleAtStartup(context.Background()); err != nil {
		t.Fatalf("startup pré-auth falhou: %v", err)
	}
	if snapshot := app.commandLifecycle.Load().Snapshot(); snapshot.State != commandruntime.StateCold {
		t.Fatalf("startup pré-auth habilitou runtime: %+v", snapshot)
	}

	app.authMu.Lock()
	app.currentUserID = "user-1"
	app.currentAuthUser = &AuthUser{UserID: "user-1", SessionID: "session-1"}
	app.authMu.Unlock()
	if err := app.bootstrapCommandLifecycleAtStartup(context.Background()); err != nil {
		t.Fatalf("startup autenticado falhou: %v", err)
	}
	if snapshot := app.commandLifecycle.Load().Snapshot(); snapshot.State != commandruntime.StateReady {
		t.Fatalf("startup autenticado não publicou readiness: %+v", snapshot)
	}
	if err := app.resetCommandLifecycleIfConfigured(context.Background(), "logout"); err != nil {
		t.Fatalf("reset autenticado falhou: %v", err)
	}
	if snapshot := app.commandLifecycle.Load().Snapshot(); snapshot.State != commandruntime.StateCold || snapshot.Published {
		t.Fatalf("reset não limpou publicação: %+v", snapshot)
	}
	if probe.lockHeld.Load() {
		t.Fatal("callback do lifecycle executou sob lock de autenticação")
	}

	app.Shutdown()
	if app.commandLifecycle.Load() != nil {
		t.Fatal("Shutdown do App não desmontou lifecycle configurado")
	}
}

func TestAppCommandLifecycleAuthResultSurvivesBootstrapFailure(t *testing.T) {
	app := &App{}
	probe := &appLifecyclePort{app: app, failAuth: true}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(probe)); err != nil {
		t.Fatal(err)
	}
	want := &AuthUser{UserID: "user-1", SessionID: "session-1"}
	result := want
	var authErr error
	app.bootstrapCommandLifecycleAfterAuth(context.Background(), result, authErr)
	if result != want || authErr != nil {
		t.Fatalf("falha do bootstrap alterou resultado da autenticação: result=%+v err=%v", result, authErr)
	}
	if snapshot := app.commandLifecycle.Load().Snapshot(); snapshot.State == commandruntime.StateReady || snapshot.Published {
		t.Fatalf("bootstrap falho publicou runtime: %+v", snapshot)
	}
	app.Shutdown()
}

func TestAppCommandLifecycleSuppressesDelayedStaleAuthResult(t *testing.T) {
	app := &App{}
	probe := &appLifecyclePort{app: app}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(probe)); err != nil {
		t.Fatal(err)
	}
	app.authMu.Lock()
	app.currentUserID = "user-1"
	app.currentAuthUser = &AuthUser{UserID: "user-1", SessionID: "session-1", Role: database.UserRoleUser}
	app.authMu.Unlock()
	result := &AuthUser{UserID: "user-1", SessionID: "session-1", Role: database.UserRoleUser}
	app.authMu.Lock()
	app.currentUserID = ""
	app.currentAuthUser = nil
	app.authMu.Unlock()
	app.bootstrapCommandLifecycleAfterAuth(context.Background(), result, nil)
	if snapshot := app.commandLifecycle.Load().Snapshot(); snapshot.State != commandruntime.StateCold || snapshot.Published {
		t.Fatalf("resultado de auth atrasado iniciou runtime: %+v", snapshot)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
}

func TestAppLoginPreservesSessionWhenCommandLifecycleBootstrapFails(t *testing.T) {
	t.Chdir(t.TempDir())
	db := setupAuthAppTestDB(t)
	password := "correct horse battery staple"
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		t.Fatal(err)
	}
	user := &database.User{
		Username:     "login-lifecycle",
		PasswordHash: passwordHash,
		Role:         database.UserRoleUser,
		IsActive:     true,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	signer, err := auth.NewTokenSigner()
	if err != nil {
		t.Fatal(err)
	}
	sessionSvc, err := auth.NewSessionService(db, auth.SessionConfig{Signer: signer})
	if err != nil {
		t.Fatal(err)
	}
	credStore := credentials.NewDBStore()
	credMgr := credentials.NewManagerWithStore([]byte("test-key-exactly-32-bytes-long!!"), credStore, true)
	app := &App{
		ctx:         context.Background(),
		credMgr:     credMgr,
		credStore:   credStore,
		vaultSvc:    auth.NewVaultService(credStore, nil),
		identitySvc: auth.NewIdentityService(db),
		sessionSvc:  sessionSvc,
		llmRegistry: llm.NewProviderRegistry(),
		authKeyringSave: func(string) error {
			return nil
		},
		authKeyringDelete: func() error {
			return nil
		},
	}
	probe := &appLifecyclePort{app: app, failAuth: true}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(probe)); err != nil {
		t.Fatal(err)
	}

	result, err := app.Login(LoginRequest{Username: user.Username, Password: password})
	if err != nil {
		t.Fatalf("login válido foi convertido em falha pelo bootstrap: %v", err)
	}
	if result == nil || result.UserID != user.ID {
		t.Fatalf("login não retornou sessão válida: %+v", result)
	}
	app.authMu.RLock()
	currentUserID := app.currentUserID
	currentAuthUser := app.currentAuthUser
	app.authMu.RUnlock()
	if currentUserID != user.ID || currentAuthUser == nil {
		t.Fatalf("sessão não permaneceu ativa após falha opcional: id=%q auth=%+v", currentUserID, currentAuthUser)
	}
	if snapshot := app.commandLifecycle.Load().Snapshot(); snapshot.State == commandruntime.StateReady || snapshot.Published {
		t.Fatalf("lifecycle falho publicou runtime: %+v", snapshot)
	}
	if probe.lockHeld.Load() {
		t.Fatal("callback do lifecycle executou sob lock de autenticação")
	}
	app.Shutdown()
}

func TestAppCommandLifecycleHooksSerializeConcurrentResetAndShutdown(t *testing.T) {
	app := &App{}
	probe := &appLifecyclePort{app: app}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(probe)); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 4)
	var wait sync.WaitGroup
	wait.Add(4)
	for range 4 {
		go func() {
			defer wait.Done()
			<-start
			results <- app.resetCommandLifecycleIfConfigured(context.Background(), "logout")
		}()
	}
	close(start)
	wait.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("reset concorrente falhou: %v", err)
		}
	}

	shutdownResults := make(chan error, 2)
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			shutdownResults <- app.shutdownCommandLifecycleIfConfigured(context.Background())
		}()
	}
	wait.Wait()
	close(shutdownResults)
	for err := range shutdownResults {
		if err != nil && !errors.Is(err, errCommandLifecycleAlreadyConfigured) {
			t.Fatalf("shutdown concorrente falhou: %v", err)
		}
	}
	if app.commandLifecycle.Load() != nil {
		t.Fatal("shutdown concorrente deixou lifecycle montado")
	}
	if probe.lockHeld.Load() {
		t.Fatal("callback concorrente executou sob lock de autenticação")
	}
}
