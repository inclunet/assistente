package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandactivation"
	"assistente/internal/commandbindings"
	"assistente/internal/commandbridge"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandconfig"
	"assistente/internal/commandcontext"
	"assistente/internal/commanddecision"
	"assistente/internal/commandexecution"
	"assistente/internal/commandinstance"
	"assistente/internal/commandledger"
	"assistente/internal/commandruntime"
	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/questionnaire"
	"assistente/internal/workspace"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type appLifecyclePort struct {
	generation             commandruntime.Generation
	mu                     sync.Mutex
	app                    *App
	lockHeld               atomic.Bool
	deterministicLockProbe bool
	lockProbeWG            sync.WaitGroup
	failAuth               bool
	clearStarted           chan struct{}
	clearRelease           chan struct{}
	clearOnce              sync.Once
}

func (p *appLifecyclePort) checkLocks() {
	if p.app == nil {
		return
	}
	if !p.deterministicLockProbe {
		return
	}
	for _, lock := range []sync.Locker{&p.app.authMu, &p.app.authSessionMu} {
		done := make(chan struct{})
		p.lockProbeWG.Add(1)
		go func() {
			defer p.lockProbeWG.Done()
			lock.Lock()
			defer lock.Unlock()
			close(done)
		}()
		select {
		case <-done:
		case <-time.After(time.Second):
			p.lockHeld.Store(true)
		}
	}
}

func (p *appLifecyclePort) waitForLockProbes() {
	p.lockProbeWG.Wait()
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

func appLifecycleMountSpec(probe *appLifecyclePort) commandruntime.MountSpec {
	return commandruntime.MountSpec{Config: appLifecycleConfig(probe), Dependencies: []commandruntime.MountDependency{
		{Role: commandruntime.MountDependencyCatalog, Name: "catalogo-app", Instance: struct{}{}},
		{Role: commandruntime.MountDependencyDefaults, Name: "defaults-app", Instance: struct{}{}},
		{Role: commandruntime.MountDependencyPolicies, Name: "politicas-app", Instance: struct{}{}},
		{Role: commandruntime.MountDependencyStores, Name: "stores-app", Instance: struct{}{}},
		{Role: commandruntime.MountDependencyPresenter, Name: "presenter-app", Instance: struct{}{}},
		{Role: commandruntime.MountDependencyProviders, Name: "providers-app", Instance: struct{}{}},
		{Role: commandruntime.MountDependencyDispatcher, Name: "dispatcher-app", Instance: struct{}{}},
		{Role: commandruntime.MountDependencyAdapters, Name: "adapters-app", Instance: struct{}{}},
	}}
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

func TestAppCommandLifecycleMountSpecFailsClosedBeforeInstallingRuntime(t *testing.T) {
	app := &App{}
	probe := &appLifecyclePort{}
	spec := appLifecycleMountSpec(probe)
	spec.Dependencies = spec.Dependencies[:len(spec.Dependencies)-1]
	if err := ConfigureCommandLifecycleMountSpec(app, spec); !errors.Is(err, commandruntime.ErrMissingDependency) {
		t.Fatalf("montagem incompleta foi aceita/erro errado: %v", err)
	}
	if app.commandLifecycle.Load() != nil {
		t.Fatal("controller instalado apesar de manifesto incompleto")
	}
	if err := ConfigureCommandLifecycleMountSpec(app, appLifecycleMountSpec(probe)); err != nil {
		t.Fatalf("manifesto completo recusado: %v", err)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
}

type appLifecycleBridgePort struct{}

func (appLifecycleBridgePort) Dispatch(context.Context, commandbridge.Invocation) (commandbridge.InvocationAck, error) {
	return commandbridge.InvocationAck{Accepted: true}, nil
}

func (appLifecycleBridgePort) Cancel(context.Context, commandbridge.CancelRequest) error { return nil }

func appLifecycleProductMountFixture(t *testing.T) (*App, CommandLifecycleMountInputs) {
	t.Helper()
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open("file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "lifecycle-mount.db"))+"?_pragma=busy_timeout(100)"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(2)
	t.Cleanup(func() { _ = sqlDB.Close() })
	// Reproduz a política de concorrência de produção, mas preserva
	// auto_vacuum=NONE: os testes de manutenção exercitam a migração real
	// de bancos anteriores, não apenas a abertura de um banco novo.
	if err := db.Exec("PRAGMA journal_mode=WAL").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("PRAGMA synchronous=NORMAL").Error; err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatal(err)
	}
	if err := commandledger.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := commandconfig.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := commandactivation.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := commanddecision.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := commandinstance.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	previousDB := database.DB()
	database.SetDB(db)
	t.Cleanup(func() { database.SetDB(previousDB) })
	store, err := commandledger.New(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{RefreshTokenPepper: bytes.Repeat([]byte{7}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "lifecycle-mount", PasswordHash: "unused", IsActive: true, Role: database.UserRoleUser}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	session, err := sessions.IssueSession(ctx, &user, "lifecycle")
	if err != nil {
		t.Fatal(err)
	}
	workspaceManager := workspace.NewManager(filepath.Join(t.TempDir(), "assistente-home"))
	if err := workspaceManager.Initialize(filepath.Join(t.TempDir(), "workspace")); err != nil {
		t.Fatal(err)
	}
	if err := workspaceManager.SetProfile("lifecycle-profile"); err != nil {
		t.Fatal(err)
	}
	if err := workspaceManager.AddTab(workspace.Tab{ID: "tab-lifecycle", Type: workspace.TabTypeEditor, State: map[string]any{"version": float64(1)}}); err != nil {
		t.Fatal(err)
	}
	credMgr := credentials.NewManager(bytes.Repeat([]byte{8}, 32))
	if err := credMgr.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{8}, 32))); err != nil {
		t.Fatal(err)
	}
	app := &App{
		ctx:                   ctx,
		sessionSvc:            sessions,
		credMgr:               credMgr,
		questionnaireMgr:      questionnaire.NewManager(func(string, any) {}),
		workspaceMgr:          workspaceManager,
		commandStorageVersion: "v1",
		authKeyringDelete:     func() error { return nil },
	}
	app.setCurrentUserID(user.ID)
	app.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: session.SessionID, Role: user.Role})
	t.Cleanup(func() {
		_ = ShutdownCommandLifecycle(ctx, app)
		_ = app.drainCommandExecutors(ctx)
		_ = app.shutdownCommandBridgeIfConfigured(ctx)
	})
	epochs, err := app.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	state, err := commandexecution.NewHostState(epochs, commandProductRegistryVersion)
	if err != nil {
		t.Fatal(err)
	}
	bindings, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := state.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(context.Context) (auth.LocalSessionPrincipal, error) {
		return auth.LocalSessionPrincipal{UserID: user.ID, SessionID: session.SessionID}, nil
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return bindings, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	// A fixture desbloqueada precisa instalar explicitamente o HostState real;
	// a montagem produtiva não deve inferir vault desbloqueado quando cria um
	// host novo.
	app.commandHost = state
	locales := map[string]commandcatalog.LocalizedMetadata{}
	for _, locale := range []string{"pt-BR", "en", "es"} {
		locales[locale] = commandcatalog.LocalizedMetadata{Name: "Fixture", Description: "Fixture", Category: "Fixture"}
	}
	registry, err := commandcatalog.New([]commandcatalog.Registration{{Definition: commandcatalog.Definition{
		ID:             "fixture.read",
		Effect:         commandcatalog.Read,
		Decision:       commandcatalog.NoDecision,
		Context:        commandcatalog.ContextPolicy{None: true},
		AllowedSources: []commandcatalog.Source{commandcatalog.Palette},
		Presentation:   &commandcatalog.Presentation{Version: "1", Locales: locales},
	}, Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read}}})
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := commandbridge.New(commandbridge.Config{
		Port: appLifecycleBridgePort{},
		Capabilities: []commandbridge.Capability{{
			ID: uuid.Must(uuid.NewV7()).String(), CommandID: "fixture.read", Generation: 1, Source: commandbridge.SourcePalette,
			Owner: commandbridge.Owner{UserID: uuid.Must(uuid.NewV7()).String(), SessionID: uuid.Must(uuid.NewV7()).String(), WorkspaceID: "workspace-1"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	facts, err := commandcontext.NewFactBus(map[string]commandcontext.ScopedProvider{})
	if err != nil {
		t.Fatal(err)
	}
	runtimeProbe := &appLifecyclePort{}
	execution := commandexecution.Config{
		Envelope:        &commandexecution.EnvelopeConfig{},
		Sessions:        sessions,
		Epochs:          epochs,
		Store:           store,
		Registry:        registry,
		RegistryVersion: commandProductRegistryVersion,
		Source:          commandcatalog.Palette,
		Snapshot: func(context.Context, auth.LocalSessionPrincipal) (commandexecution.Versions, error) {
			return commandexecution.Versions{}, nil
		},
		Authorize:           func(context.Context, auth.LocalSessionPrincipal, string, commandcatalog.Source) error { return nil },
		KeyVersion:          "v1",
		Now:                 time.Now,
		Retention:           time.Hour,
		ExecutionTimeout:    time.Second,
		FinalizationTimeout: time.Second,
		Handlers: map[string]commandexecution.Handler{"fixture.read": {
			Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read},
			Start: func(context.Context, commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
				return commandexecution.ExecutionHandle{ID: "fixture", Done: make(chan commandexecution.Outcome, 1), Cancel: func() {}}, nil
			},
		}},
	}
	return app, CommandLifecycleMountInputs{
		Runtime:   appLifecycleConfig(runtimeProbe),
		Execution: execution,
		Host:      state,
		Bridge:    bridge,
		Facts:     facts,
		Adapter:   struct{ name string }{"palette-adapter"},
	}
}

func TestAppCommandLifecycleProductMountSpecUsesRealAppDependencies(t *testing.T) {
	app, inputs := appLifecycleProductMountFixture(t)
	if err := ConfigureCommandLifecycleForApp(app, inputs); err != nil {
		t.Fatalf("montagem de produto recusada: %v", err)
	}
	if app.commandHost != inputs.Host {
		t.Fatal("montagem não instalou HostState real no App")
	}
	if bridge, ok := loadCommandBridge(app); !ok || bridge != inputs.Bridge {
		t.Fatalf("montagem não instalou Bridge real: ok=%v bridge=%p want=%p", ok, bridge, inputs.Bridge)
	}
	snapshot, err := CommandLifecycleSnapshot(app)
	if err != nil || snapshot.State != commandruntime.StateCold {
		t.Fatalf("montagem instalou runtime em estado inesperado: %+v err=%v", snapshot, err)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleForAppBuildsDefaultRuntimeAndBootstraps(t *testing.T) {
	app, inputs := appLifecycleProductMountFixture(t)
	inputs.Runtime = commandruntime.Config{}
	if err := ConfigureCommandLifecycleForApp(app, inputs); err != nil {
		t.Fatalf("montagem com runtime padrão recusada: %v", err)
	}
	if snapshot, err := CommandLifecycleSnapshot(app); err != nil || snapshot.State != commandruntime.StateCold {
		t.Fatalf("runtime padrão não iniciou frio: %+v err=%v", snapshot, err)
	}
	if err := BootstrapCommandLifecycle(context.Background(), app); err != nil {
		t.Fatalf("bootstrap com portas reais falhou: %v", err)
	}
	snapshot, err := CommandLifecycleSnapshot(app)
	if err != nil || snapshot.State != commandruntime.StateReady || !snapshot.Published || snapshot.PublishedEntries != 1 {
		t.Fatalf("bootstrap não publicou readiness real: %+v err=%v", snapshot, err)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleProductMountSpecRejectsMissingProductDependencies(t *testing.T) {
	app, inputs := appLifecycleProductMountFixture(t)
	inputs.Adapter = (*appLifecycleBridgePort)(nil)
	if err := ConfigureCommandLifecycleForApp(app, inputs); !errors.Is(err, commandruntime.ErrMissingDependency) {
		t.Fatalf("adapter nil aceito/erro errado: %v", err)
	}
	if app.commandLifecycle.Load() != nil {
		t.Fatal("runtime instalado após dependência física nil")
	}

	app, inputs = appLifecycleProductMountFixture(t)
	app.questionnaireMgr = nil
	if err := ConfigureCommandLifecycleForApp(app, inputs); !errors.Is(err, commandruntime.ErrMissingDependency) {
		t.Fatalf("presenter ausente aceito/erro errado: %v", err)
	}

	app, inputs = appLifecycleProductMountFixture(t)
	otherApp := &App{authKeyringDelete: func() error { return nil }}
	otherEpochs, err := otherApp.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	inputs.Host, err = commandexecution.NewHostState(otherEpochs, "registry-v1")
	if err != nil {
		t.Fatal(err)
	}
	if err := ConfigureCommandLifecycleForApp(app, inputs); !errors.Is(err, commandruntime.ErrInvalidConfiguration) {
		t.Fatalf("HostState de outro epoch aceito/erro errado: %v", err)
	}

	app, inputs = appLifecycleProductMountFixture(t)
	otherBridge, err := commandbridge.New(commandbridge.Config{
		Port: appLifecycleBridgePort{},
		Capabilities: []commandbridge.Capability{{
			ID: uuid.Must(uuid.NewV7()).String(), CommandID: "fixture.read", Generation: 1, Source: commandbridge.SourcePalette,
			Owner: commandbridge.Owner{UserID: uuid.Must(uuid.NewV7()).String(), SessionID: uuid.Must(uuid.NewV7()).String(), WorkspaceID: "workspace-1"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := ConfigureCommandBridge(app, otherBridge); err != nil {
		t.Fatal(err)
	}
	if err := ConfigureCommandLifecycleForApp(app, inputs); !errors.Is(err, errCommandBridgeAlreadyConfigured) {
		t.Fatalf("bridge divergente aceita/erro errado: %v", err)
	}
}

func TestAppCommandLifecycleAfterAuthMountsProductBaseWhenMissing(t *testing.T) {
	app, _ := appLifecycleProductMountFixture(t)
	result := app.currentAuthUser
	if result == nil {
		t.Fatal("fixture sem auth user")
	}
	if err := app.commandHost.SetOSSessionState(context.Background(), false, true); err != nil {
		t.Fatal(err)
	}
	app.bootstrapCommandLifecycleAfterAuth(context.Background(), result, nil)
	if _, ok := loadCommandLifecycle(app); !ok {
		t.Fatal("pós-auth não montou lifecycle produtivo mínimo")
	}
	if app.commandHost == nil {
		t.Fatal("pós-auth não instalou HostState")
	}
	if _, ok := loadCommandBridge(app); !ok {
		t.Fatal("pós-auth não instalou Bridge")
	}
	// O monitor nativo ainda não comprovou a sessão nesta fixture. A recarga
	// negada deve permanecer fria, sem publicar a configuração antiga.
	if snapshot := app.commandLifecycle.Load().Snapshot(); snapshot.State != commandruntime.StateCold || snapshot.Published {
		t.Fatalf("recarga sem sessão do SO conhecida publicou runtime: %+v", snapshot)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleRebuildsEmptyProductThenBootstrapsReady(t *testing.T) {
	app, _ := appLifecycleProductMountFixture(t)
	if err := ensureCommandLifecycleMountedForCurrentUserForTest(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetOSSessionState(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	if err := app.rebuildCommandLifecycleEmptyConfiguration(context.Background()); err != nil {
		t.Fatalf("rebuild vazio falhou: %v", err)
	}
	if err := BootstrapCommandLifecycle(context.Background(), app); err != nil {
		t.Fatalf("bootstrap após rebuild falhou: %v", err)
	}
	snapshot, err := CommandLifecycleSnapshot(app)
	if err != nil || snapshot.State != commandruntime.StateReady || !snapshot.Published || snapshot.PublishedEntries != 151 {
		t.Fatalf("runtime não ficou ready após rebuild: %+v err=%v", snapshot, err)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleRebuildsPersistedLocalConfigurationAfterAuth(t *testing.T) {
	ctx := context.Background()
	app, _ := appLifecycleProductMountFixture(t)
	if err := ensureCommandLifecycleMountedForCurrentUserForTest(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	principal, err := app.currentCommandPrincipal()
	if err != nil {
		t.Fatal(err)
	}
	generation := commandconfig.Generation{ID: uuid.Must(uuid.NewV7()).String(), UserID: principal.UserID, Generation: 1, UpdatedAt: time.Now()}
	if err := database.DB().Create(&generation).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatalf("rebuild persistido falhou: %v", err)
	}
	configuration, activeLayers, err := app.commandHost.UserConfiguration(ctx, principal.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(activeLayers) != 0 {
		t.Fatalf("rebuild persistido restaurou claims indevidamente: %v", activeLayers)
	}
	selection, err := configuration.Resolve("palette:workspace.list", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Status != commandbindings.Selected || selection.CommandID != commandProductWorkspaceListID || len(selection.BindingIDs) != 1 || selection.BindingIDs[0] != "builtin.palette.workspace.list" {
		t.Fatalf("rebuild não publicou o default real da paleta: %+v", selection)
	}
	if err := ShutdownCommandLifecycle(ctx, app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleRestoresPersistentClaimsIntoActiveLayers(t *testing.T) {
	ctx := context.Background()
	app, _ := appLifecycleProductMountFixture(t)
	if err := ensureCommandLifecycleMountedForCurrentUserForTest(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	principal, err := app.currentCommandPrincipal()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := database.DB().Create(&commandconfig.Generation{ID: uuid.Must(uuid.NewV7()).String(), UserID: principal.UserID, Generation: 1, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	layer := commandconfig.Layer{ID: uuid.Must(uuid.NewV7()).String(), UserID: principal.UserID, Name: "persistente", Enabled: true, Source: "user", CreatedAt: now, UpdatedAt: now}
	if err := database.DB().Create(&layer).Error; err != nil {
		t.Fatal(err)
	}
	ruleID := uuid.Must(uuid.NewV7()).String()
	rule := commandactivation.Rule{
		ID: ruleID, UserID: principal.UserID, LayerRefKind: commandactivation.UserRef, LayerRef: layer.ID,
		RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Mode: commandactivation.ModeManual,
		Condition: "{}", Lifecycle: commandactivation.LifecyclePersistent, Enabled: true, Source: "user", ReviewStatus: "active",
	}
	if err := database.DB().Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	oldSession := uuid.Must(uuid.NewV7()).String()
	oldStack := "manual:old-stack"
	claimID := uuid.Must(uuid.NewV7()).String()
	claim := commandactivation.Claim{
		ActivationID: claimID, UserID: principal.UserID, LayerRefKind: commandactivation.UserRef, LayerRef: layer.ID,
		RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, AuthContextType: "local_session", AuthContextID: oldSession,
		AuthGeneration: "auth-old", SecurityGeneration: "security-old", SourceType: "manual", SourceInstanceID: &oldSession,
		State: commandactivation.StateActive, ManualStackKey: &oldStack, ActivatedAt: now.Add(-time.Hour), UpdatedAt: now.Add(-time.Hour),
	}
	if err := database.DB().Create(&claim).Error; err != nil {
		t.Fatal(err)
	}
	if err := app.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatalf("rebuild com restore falhou: %v", err)
	}
	configuration, activeLayers, err := app.commandHost.UserConfiguration(ctx, principal.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if len(activeLayers) != 1 || activeLayers[0] != layer.ID {
		t.Fatalf("camada persistente não foi ativada: %v", activeLayers)
	}
	if configuration == nil {
		t.Fatal("rebuild não materializou configuração produtiva")
	}
	var restored commandactivation.Claim
	if err := database.DB().Where("activation_id = ?", claimID).First(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if restored.AuthContextID != principal.SessionID || restored.SecurityGeneration == "security-old" || restored.ManualStackKey == nil || *restored.ManualStackKey == oldStack {
		t.Fatalf("claim não foi rebindada para a sessão atual: %+v", restored)
	}
	if err := ShutdownCommandLifecycle(ctx, app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleRejectsLoadedConfigurationAfterSessionChange(t *testing.T) {
	ctx := context.Background()
	app, _ := appLifecycleProductMountFixture(t)
	if err := ensureCommandLifecycleMountedForCurrentUserForTest(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	principal, err := app.currentCommandPrincipal()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Create(&commandconfig.Generation{ID: uuid.Must(uuid.NewV7()).String(), UserID: principal.UserID, Generation: 1, UpdatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatal(err)
	}
	store, err := commandconfig.New(database.DB())
	if err != nil {
		t.Fatal(err)
	}
	registry, _, err := app.commandProductCatalog()
	if err != nil {
		t.Fatal(err)
	}
	loaded, hasSnapshot, err := app.loadCommandLifecyclePersistedConfiguration(ctx, store, commandconfig.LocalReadProjection{
		Registry:           registry,
		NoArgumentCommands: nil,
		BuiltinLayers:      nil,
	})
	if err != nil || !hasSnapshot {
		t.Fatalf("load persistido falhou: has=%v err=%v", hasSnapshot, err)
	}
	staleSession := principal.SessionID
	app.authMu.Lock()
	app.currentAuthUser.SessionID = uuid.Must(uuid.NewV7()).String()
	app.authMu.Unlock()
	if err := loaded.publish(ctx); !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("publicação stale não foi rejeitada: %v", err)
	}
	app.authMu.RLock()
	currentSession := app.currentAuthUser.SessionID
	app.authMu.RUnlock()
	if currentSession == staleSession {
		t.Fatal("fixture não simulou troca de sessão")
	}
	if err := ShutdownCommandLifecycle(ctx, app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleResetForgetsPublishedHostConfiguration(t *testing.T) {
	ctx := context.Background()
	app, _ := appLifecycleProductMountFixture(t)
	if err := ensureCommandLifecycleMountedForCurrentUserForTest(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := app.rebuildCommandLifecycleEmptyConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	principal, err := app.currentCommandPrincipal()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.commandHost.UserConfiguration(ctx, principal.UserID); err != nil {
		t.Fatalf("fixture não publicou configuração inicial: %v", err)
	}
	if err := app.resetCommandLifecycleIfConfigured(ctx, "refresh"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.commandHost.UserConfiguration(ctx, principal.UserID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("reset não removeu configuração volátil do host: %v", err)
	}
	if err := ShutdownCommandLifecycle(ctx, app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleShutdownForgetsPublishedHostConfiguration(t *testing.T) {
	ctx := context.Background()
	app, _ := appLifecycleProductMountFixture(t)
	if err := ensureCommandLifecycleMountedForCurrentUserForTest(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := app.rebuildCommandLifecycleEmptyConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	principal, err := app.currentCommandPrincipal()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.commandHost.UserConfiguration(ctx, principal.UserID); err != nil {
		t.Fatalf("fixture não publicou configuração inicial: %v", err)
	}
	if err := app.shutdownCommandLifecycleIfConfigured(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, err := app.commandHost.UserConfiguration(ctx, principal.UserID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("shutdown não removeu configuração volátil do host: %v", err)
	}
	if app.commandLifecycle.Load() != nil {
		t.Fatal("shutdown não desmontou lifecycle")
	}
}

func TestAppCommandLifecycleShutdownDrainsBridgeAndBlocksRemount(t *testing.T) {
	ctx := context.Background()
	app, inputs := appLifecycleProductMountFixture(t)
	app.ctx, app.cancel = context.WithCancel(ctx)
	defer app.cancel()
	if err := ConfigureCommandLifecycleForApp(app, inputs); err != nil {
		t.Fatal(err)
	}
	app.authMu.Lock()
	app.commandStorageVersion = ""
	app.authMu.Unlock()
	if err := BootstrapCommandLifecycle(ctx, app); err != nil {
		t.Fatal(err)
	}

	app.Shutdown()

	if lifecycle, ok := loadCommandLifecycle(app); ok || lifecycle != nil {
		t.Fatal("shutdown integrado deixou lifecycle montado")
	}
	if bridge, ok := loadCommandBridge(app); ok || bridge != nil {
		t.Fatal("shutdown integrado deixou bridge montada")
	}
	core, err := app.commandSecurityService()
	if err != nil {
		t.Fatal(err)
	}
	if err := core.RegisterExecutorDrain(ctx, func(context.Context) error { return nil }); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatalf("shutdown integrado não fechou admissões de executores: %v", err)
	}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(&appLifecyclePort{})); !errors.Is(err, commandruntime.ErrStopped) {
		t.Fatalf("shutdown integrado permitiu remontar lifecycle: %v", err)
	}
	if err := ConfigureCommandBridge(app, inputs.Bridge); !errors.Is(err, commandbridge.ErrBridgeClosed) {
		t.Fatalf("shutdown integrado permitiu remontar bridge: %v", err)
	}
}

func TestAppCommandLifecycleRetriesAfterInitialNotReadyFailure(t *testing.T) {
	ctx := context.Background()
	app, _ := appLifecycleProductMountFixture(t)
	if err := ensureCommandLifecycleMountedForCurrentUserForTest(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetOSSessionState(ctx, false, false); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(ctx, app); err == nil {
		t.Fatalf("bootstrap sem ambiente pronto = %v", err)
	}
	if snapshot, err := CommandLifecycleSnapshot(app); err != nil || snapshot.State != commandruntime.StateFailed || snapshot.Published {
		t.Fatalf("falha inicial foi mascarada como pronta: %+v err=%v", snapshot, err)
	}
	if err := app.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := app.rebuildCommandLifecycleEmptyConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(ctx, app); err != nil {
		t.Fatalf("retry após ambiente pronto falhou: %v", err)
	}
	if snapshot, err := CommandLifecycleSnapshot(app); err != nil || snapshot.State != commandruntime.StateReady || !snapshot.Published {
		t.Fatalf("retry não publicou readiness: %+v err=%v", snapshot, err)
	}
	if err := ShutdownCommandLifecycle(ctx, app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleRuntimeRejectsStaleHostProjectionAtPublish(t *testing.T) {
	ctx := context.Background()
	app, inputs := appLifecycleProductMountFixture(t)
	runtimePort, err := newAppCommandLifecycleRuntime(app, inputs)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := runtimePort.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := runtimePort.Project(ctx, generation)
	if err != nil {
		t.Fatal(err)
	}
	if err := inputs.Host.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := runtimePort.Publish(ctx, projection); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("publish aceitou projeção stale do HostState: %v", err)
	}
	runtimePort.mu.Lock()
	_, published := runtimePort.published[generation.Value]
	runtimePort.mu.Unlock()
	if published {
		t.Fatal("projeção stale ficou publicada")
	}
}

func TestAppCommandLifecycleRuntimeRejectsStaleHostProjectionAtEnable(t *testing.T) {
	ctx := context.Background()
	app, inputs := appLifecycleProductMountFixture(t)
	runtimePort, err := newAppCommandLifecycleRuntime(app, inputs)
	if err != nil {
		t.Fatal(err)
	}
	generation, err := runtimePort.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := runtimePort.Project(ctx, generation)
	if err != nil {
		t.Fatal(err)
	}
	if err := runtimePort.Publish(ctx, projection); err != nil {
		t.Fatal(err)
	}
	if err := inputs.Host.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := runtimePort.SetEnabled(ctx, generation, true); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("enable aceitou projeção stale do HostState: %v", err)
	}
	runtimePort.mu.Lock()
	enabled := runtimePort.enabled
	runtimePort.mu.Unlock()
	if enabled == generation {
		t.Fatal("geração stale ficou habilitada")
	}
}

func TestAppCommandLifecycleRestartDoesNotInferLedgerRecoveryWithoutDrainProof(t *testing.T) {
	ctx := context.Background()
	app, _ := appLifecycleProductMountFixture(t)
	if err := ensureCommandLifecycleMountedForCurrentUserForTest(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	principal, err := app.currentCommandPrincipal()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := database.DB().Create(&commandconfig.Generation{ID: uuid.Must(uuid.NewV7()).String(), UserID: principal.UserID, Generation: 1, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	ledger, err := commandledger.New(database.DB(), func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	request := commandledger.LocalReadRequest{
		InvocationID:              uuid.Must(uuid.NewV7()).String(),
		Owner:                     commandledger.Owner{UserID: principal.UserID, AuthContextID: principal.SessionID},
		AuthGeneration:            "auth-before-restart",
		SecurityGeneration:        "security-before-restart",
		RegistryVersion:           commandProductRegistryVersion,
		GlobalConfigGeneration:    "global-before-restart",
		ActiveLayersGeneration:    "layers-before-restart",
		CommandID:                 commandProductWorkspaceListID,
		SourceType:                "palette",
		ArgumentsFingerprint:      "args",
		RequestFingerprintVersion: "v1",
		RequestFingerprint:        "request-before-restart",
		CorrelationID:             uuid.Must(uuid.NewV7()).String(),
		ReceivedAt:                now,
		ExpiresAt:                 now.Add(time.Hour),
	}
	if _, err := ledger.Reserve(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := app.rebuildCommandLifecyclePersistedConfiguration(ctx); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(ctx, app); !errors.Is(err, commandruntime.ErrNotReady) {
		t.Fatalf("restart sem prova de drain deveria falhar fechado: %v", err)
	}
	snapshot, err := CommandLifecycleSnapshot(app)
	if err != nil || snapshot.Published {
		t.Fatalf("restart sem drain publicou configuração indevidamente: %+v err=%v", snapshot, err)
	}
	record, err := ledger.Get(ctx, request.Owner, request.InvocationID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Status != commandledger.Evaluating {
		t.Fatalf("restart sem prova drenada reconciliou ledger indevidamente: %s", record.Status)
	}
	if err := ShutdownCommandLifecycle(ctx, app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleActiveLayerDerivationRejectsUnsafeClaims(t *testing.T) {
	now := time.Now().UTC()
	userID := uuid.Must(uuid.NewV7()).String()
	sessionID := uuid.Must(uuid.NewV7()).String()
	layerID := uuid.Must(uuid.NewV7()).String()
	ruleID := uuid.Must(uuid.NewV7()).String()
	stack := "manual:stack"
	base := commandconfig.Snapshot{
		Scope:  commandconfig.Scope{UserID: userID},
		Layers: []commandconfig.Layer{{ID: layerID, UserID: userID, Enabled: true}},
		ActivationRules: []commandactivation.Rule{{
			ID: ruleID, UserID: userID, LayerRefKind: commandactivation.UserRef, LayerRef: layerID,
			RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, Lifecycle: commandactivation.LifecyclePersistent,
			Enabled: true, ReviewStatus: "active",
		}},
	}
	claim := commandactivation.Claim{
		ActivationID: uuid.Must(uuid.NewV7()).String(), UserID: userID, LayerRefKind: commandactivation.UserRef, LayerRef: layerID,
		RuleRefKind: commandactivation.UserRef, RuleRef: ruleID, AuthContextType: "local_session", AuthContextID: sessionID,
		AuthGeneration: "auth", SecurityGeneration: "security", SourceType: "manual", State: commandactivation.StateActive,
		ManualStackKey: &stack, ActivatedAt: now.Add(-time.Hour), UpdatedAt: now,
	}
	principal := auth.LocalSessionPrincipal{UserID: userID, SessionID: sessionID}
	snapshot := base
	snapshot.ActivationClaims = []commandactivation.Claim{claim}
	if got := commandLifecycleActiveUserLayerIDs(snapshot, principal, now); len(got) != 1 || got[0] != layerID {
		t.Fatalf("claim válida não ativou camada: %v", got)
	}
	for name, mutate := range map[string]func(*commandconfig.Snapshot){
		"source": func(s *commandconfig.Snapshot) { s.ActivationClaims[0].SourceType = "context" },
		"session": func(s *commandconfig.Snapshot) {
			s.ActivationClaims[0].AuthContextID = uuid.Must(uuid.NewV7()).String()
		},
		"expired": func(s *commandconfig.Snapshot) {
			expired := now.Add(-time.Second)
			s.ActivationClaims[0].ExpiresAt = &expired
		},
		"lifecycle": func(s *commandconfig.Snapshot) {
			s.ActivationRules[0].Lifecycle = commandactivation.Lifecycle("unknown")
		},
		"disabled": func(s *commandconfig.Snapshot) { s.Layers[0].Enabled = false },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := base
			candidate.Layers = append([]commandconfig.Layer(nil), base.Layers...)
			candidate.ActivationRules = append([]commandactivation.Rule(nil), base.ActivationRules...)
			candidate.ActivationClaims = []commandactivation.Claim{claim}
			mutate(&candidate)
			if got := commandLifecycleActiveUserLayerIDs(candidate, principal, now); len(got) != 0 {
				t.Fatalf("claim insegura ativou camada: %v", got)
			}
		})
	}
}

func TestAppCommandLifecycleAfterUnlockBootstrapsCurrentSession(t *testing.T) {
	ctx := context.Background()
	app, _ := appLifecycleProductMountFixture(t)
	if err := ensureCommandLifecycleMountedForCurrentUserForTest(ctx, app); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetVaultUnlocked(ctx, false); err != nil {
		t.Fatal(err)
	}
	app.bootstrapCommandLifecycleAfterUnlock(ctx)
	if snapshot, err := CommandLifecycleSnapshot(app); err != nil || snapshot.State == commandruntime.StateReady || snapshot.Published {
		t.Fatalf("unlock falso publicou comandos: %+v err=%v", snapshot, err)
	}
	if err := app.commandHost.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	app.bootstrapCommandLifecycleAfterUnlock(ctx)
	snapshot, err := CommandLifecycleSnapshot(app)
	if err != nil || snapshot.State != commandruntime.StateReady || !snapshot.Published {
		t.Fatalf("unlock não relançou bootstrap: %+v err=%v", snapshot, err)
	}
	if err := ShutdownCommandLifecycle(ctx, app); err != nil {
		t.Fatal(err)
	}
}

func TestAppCommandLifecycleEmptyRebuildRejectsLoggedOutSession(t *testing.T) {
	app, _ := appLifecycleProductMountFixture(t)
	if err := ensureCommandLifecycleMountedForCurrentUserForTest(context.Background(), app); err != nil {
		t.Fatal(err)
	}
	if err := app.commandHost.SetOSSessionState(context.Background(), true, false); err != nil {
		t.Fatal(err)
	}
	app.setCurrentAuthUser(nil)
	if err := app.rebuildCommandLifecycleEmptyConfiguration(context.Background()); !errors.Is(err, commandexecution.ErrInvalidConfiguration) && !errors.Is(err, commandruntime.ErrNotReady) {
		t.Fatalf("rebuild deslogado aceito/erro errado: %v", err)
	}
	if err := ShutdownCommandLifecycle(context.Background(), app); err != nil {
		t.Fatal(err)
	}
}

func ensureCommandLifecycleMountedForCurrentUserForTest(ctx context.Context, app *App) error {
	if err := app.ensureCommandLifecycleMountedForCurrentUser(ctx); err != nil {
		return err
	}
	if app.commandHost == nil {
		return errors.New("HostState ausente")
	}
	return nil
}

func TestAppCommandLifecycleConfigureAllowsOnlyOneInstance(t *testing.T) {
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

func TestAppCommandLifecycleRejectedMountDoesNotRunCandidateCleanup(t *testing.T) {
	app := &App{}
	if err := ConfigureCommandLifecycle(app, appLifecycleConfig(&appLifecyclePort{})); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ShutdownCommandLifecycle(context.Background(), app) })
	probe := &appLifecyclePort{clearStarted: make(chan struct{}), clearRelease: make(chan struct{})}
	defer close(probe.clearRelease)
	result := make(chan error, 1)
	go func() { result <- ConfigureCommandLifecycle(app, appLifecycleConfig(probe)) }()
	select {
	case err := <-result:
		if !errors.Is(err, errCommandLifecycleAlreadyConfigured) {
			t.Fatalf("montagem repetida=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("montagem rejeitada aguardou cleanup de candidato nunca publicado")
	}
	select {
	case <-probe.clearStarted:
		t.Fatal("candidato rejeitado executou porta de cleanup")
	default:
	}
}

func TestAppCommandLifecycleShutdownClosesMountAdmissionEvenWhenCold(t *testing.T) {
	for _, mounted := range []bool{false, true} {
		app := &App{}
		if mounted {
			if err := ConfigureCommandLifecycle(app, appLifecycleConfig(&appLifecyclePort{})); err != nil {
				t.Fatal(err)
			}
		}
		if err := app.shutdownCommandLifecycleIfConfigured(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := ConfigureCommandLifecycle(app, appLifecycleConfig(&appLifecyclePort{})); !errors.Is(err, commandruntime.ErrStopped) {
			t.Fatalf("montagem após shutdown=%v", err)
		}
		if app.commandLifecycle.Load() != nil {
			t.Fatal("worker criado após shutdown")
		}
	}
}

func TestAppCommandLifecycleConcurrentMountAndShutdownLeavesNoWorker(t *testing.T) {
	for range 20 {
		app := &App{}
		start := make(chan struct{})
		result := make(chan error, 1)
		go func() { <-start; result <- ConfigureCommandLifecycle(app, appLifecycleConfig(&appLifecyclePort{})) }()
		close(start)
		if err := app.shutdownCommandLifecycleIfConfigured(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := <-result; err != nil && !errors.Is(err, commandruntime.ErrStopped) {
			t.Fatal(err)
		}
		if app.commandLifecycle.Load() != nil {
			t.Fatal("shutdown deixou montagem concorrente viva")
		}
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
	probe := &appLifecyclePort{app: app, deterministicLockProbe: true}
	defer probe.waitForLockProbes()
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
	probe := &appLifecyclePort{app: app, failAuth: true, deterministicLockProbe: true}
	defer probe.waitForLockProbes()
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

func TestAppCommandLifecycleLockProbeDetectsEachAuthLockWhenHeld(t *testing.T) {
	tests := []struct {
		name   string
		lock   func(*App)
		unlock func(*App)
	}{
		{
			name:   "auth",
			lock:   func(app *App) { app.authMu.Lock() },
			unlock: func(app *App) { app.authMu.Unlock() },
		},
		{
			name:   "auth-session",
			lock:   func(app *App) { app.authSessionMu.Lock() },
			unlock: func(app *App) { app.authSessionMu.Unlock() },
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := &App{}
			probe := &appLifecyclePort{app: app, deterministicLockProbe: true}
			test.lock(app)
			probe.checkLocks()
			if !probe.lockHeld.Load() {
				t.Fatal("sonda não detectou callback sob lock mantido pelo teste")
			}
			test.unlock(app)
			probe.waitForLockProbes()
		})
	}
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
}
