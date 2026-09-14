package commandexecution

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type executionFixture struct {
	db       *gorm.DB
	service  *Service
	store    *commandledger.Store
	sessions *auth.SessionService
	epochs   *commandsecurity.EpochService
	user     *database.User
	pair     *auth.TokenPair

	now      time.Time
	versions Versions

	snapshotCalls  atomic.Int32
	authorizeCalls atomic.Int32
	startCalls     atomic.Int32
	cancelCalls    atomic.Int32

	snapshotHook  func(int, Versions) (Versions, error)
	authorizeHook func(int) error
	startHook     func(context.Context, Invocation) (ExecutionHandle, error)
}

func newExecutionFixture(t *testing.T) *executionFixture {
	t.Helper()

	ctx := context.Background()
	now := time.Now().UTC()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "command-execution.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("abrir SQLite temporário: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("obter SQL DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(4)
	t.Cleanup(func() { _ = sqlDB.Close() })

	if err := commandledger.Migrate(ctx, db); err != nil {
		t.Fatalf("migrar ledger: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatalf("migrar autenticação: %v", err)
	}

	user := &database.User{
		Username:     "command-execution-fixture",
		PasswordHash: "hash não usado neste fixture",
		Role:         database.UserRoleUser,
		IsActive:     true,
	}
	if err := db.Create(user).Error; err != nil {
		t.Fatalf("criar usuário: %v", err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{
		Issuer:             "commandexecution-test",
		Audience:           "commandexecution-test-client",
		AccessTTL:          time.Hour,
		RefreshTTL:         time.Hour,
		RefreshTokenPepper: bytes.Repeat([]byte{0x21}, 32),
	})
	if err != nil {
		t.Fatalf("criar SessionService: %v", err)
	}
	pair, err := sessions.IssueSession(ctx, user, "command-execution-test")
	if err != nil {
		t.Fatalf("emitir sessão: %v", err)
	}

	gate := &commandsecurity.DispatchGate{}
	epochs, err := commandsecurity.NewEpochService(gate)
	if err != nil {
		t.Fatalf("criar EpochService: %v", err)
	}
	store, err := commandledger.New(db, func() time.Time { return now })
	if err != nil {
		t.Fatalf("criar store: %v", err)
	}

	key := bytes.Repeat([]byte{0x42}, 32)
	manager := credentials.NewManager(bytes.Repeat([]byte{0x43}, 32))
	if err := manager.RegisterInstanceSecret(
		"internal-auth:command-request-hmac:v1",
		base64.RawURLEncoding.EncodeToString(key),
	); err != nil {
		t.Fatalf("registrar chave de teste: %v", err)
	}
	keys, err := commandledger.NewCredentialKeyProvider(manager)
	if err != nil {
		t.Fatalf("criar key provider: %v", err)
	}

	registry, err := commandcatalog.New([]commandcatalog.Registration{
		testRegistration("workspace.read"),
		testRegistration("workspace.status"),
	})
	if err != nil {
		t.Fatalf("criar registry: %v", err)
	}

	f := &executionFixture{
		db:       db,
		store:    store,
		sessions: sessions,
		epochs:   epochs,
		user:     user,
		pair:     pair,
		now:      now,
		versions: Versions{Registry: "registry-v1", GlobalConfig: "global-v1", ActiveLayers: "layers-v1", Unlocked: true},
	}
	f.snapshotHook = func(_ int, versions Versions) (Versions, error) { return versions, nil }
	f.authorizeHook = func(int) error { return nil }
	f.startHook = func(_ context.Context, _ Invocation) (ExecutionHandle, error) {
		return f.completedHandle(commandledger.Succeeded), nil
	}

	start := func(ctx context.Context, invocation Invocation) (ExecutionHandle, error) {
		f.startCalls.Add(1)
		return f.startHook(ctx, invocation)
	}
	config := Config{
		Sessions:        sessions,
		Epochs:          epochs,
		Store:           store,
		Registry:        registry,
		RegistryVersion: "registry-v1",
		Handlers: map[string]Handler{
			"workspace.read":   {Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read}, Start: start},
			"workspace.status": {Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read}, Start: start},
		},
		Source:              commandcatalog.Palette,
		Snapshot:            f.snapshot,
		Authorize:           f.authorize,
		Keys:                keys,
		KeyVersion:          "v1",
		Now:                 func() time.Time { return f.now },
		Retention:           time.Hour,
		ExecutionTimeout:    2 * time.Second,
		FinalizationTimeout: 2 * time.Second,
	}
	f.service, err = New(config)
	if err != nil {
		t.Fatalf("criar CommandExecutionService: %v", err)
	}
	return f
}

func testRegistration(id string) commandcatalog.Registration {
	locales := map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: "Leitura de workspace", Description: "Lê o estado do workspace", Category: "Workspace"},
		"en":    {Name: "Workspace read", Description: "Reads workspace state", Category: "Workspace"},
		"es":    {Name: "Lectura del workspace", Description: "Lee el estado del workspace", Category: "Workspace"},
	}
	return commandcatalog.Registration{
		Definition: commandcatalog.Definition{
			ID:             id,
			Effect:         commandcatalog.Read,
			Decision:       commandcatalog.NoDecision,
			AllowedSources: []commandcatalog.Source{commandcatalog.Palette},
			Context:        commandcatalog.ContextPolicy{None: true},
			Presentation:   &commandcatalog.Presentation{Version: "metadata-v1", Locales: locales},
		},
		Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read},
	}
}

func (f *executionFixture) snapshot(ctx context.Context, _ auth.LocalSessionPrincipal) (Versions, error) {
	if err := ctx.Err(); err != nil {
		return Versions{}, err
	}
	call := int(f.snapshotCalls.Add(1))
	return f.snapshotHook(call, f.versions)
}

func (f *executionFixture) authorize(ctx context.Context, _ auth.LocalSessionPrincipal, _ string, _ commandcatalog.Source) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	call := int(f.authorizeCalls.Add(1))
	return f.authorizeHook(call)
}

func (f *executionFixture) completedHandle(status commandledger.Status) ExecutionHandle {
	done := make(chan Outcome, 1)
	done <- Outcome{Status: status}
	return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() { f.cancelCalls.Add(1) }}
}

func (f *executionFixture) request(t *testing.T, commandID string) Request {
	t.Helper()
	return Request{InvocationID: newTestUUID(), CorrelationID: newTestUUID(), CommandID: commandID}
}

func (f *executionFixture) record(t *testing.T, request Request) commandledger.Record {
	t.Helper()
	record, err := f.store.Get(context.Background(), commandledger.Owner{UserID: f.user.ID, AuthContextID: f.pair.SessionID}, request.InvocationID)
	if err != nil {
		t.Fatalf("lerdar registro: %v", err)
	}
	return record
}

func newTestUUID() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id.String()
}

func TestServiceExecutesReadNoneAndReplaysWithoutRepeatingStart(t *testing.T) {
	f := newExecutionFixture(t)
	definition, ok := f.registryDefinition("workspace.read")
	if !ok || definition.Effect != commandcatalog.Read || !definition.Context.None || len(definition.Context.Facts) != 0 || definition.Presentation == nil || len(definition.Presentation.Locales) != 3 {
		t.Fatalf("registry do fixture não representa read/none com 3 locales: %#v", definition)
	}

	request := f.request(t, "workspace.read")
	var started Invocation
	f.startHook = func(_ context.Context, invocation Invocation) (ExecutionHandle, error) {
		started = invocation
		return f.completedHandle(commandledger.Succeeded), nil
	}

	first, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
	if err != nil || first.Status != commandledger.Succeeded {
		t.Fatalf("primeira execução: record=%+v err=%v", first, err)
	}
	second, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
	if err != nil || second.ID != first.ID || second.Status != commandledger.Succeeded {
		t.Fatalf("replay: record=%+v err=%v", second, err)
	}
	if got := f.startCalls.Load(); got != 1 {
		t.Fatalf("Start calls = %d, want 1", got)
	}
	if got := f.snapshotCalls.Load(); got != 5 {
		t.Fatalf("Snapshot calls = %d, want 5 (capture, queue, running, replay capture, replay check)", got)
	}
	if got := f.authorizeCalls.Load(); got != 3 {
		t.Fatalf("Authorize calls = %d, want 3", got)
	}
	if started.Principal.UserID != f.user.ID || started.Principal.SessionID != f.pair.SessionID || started.Source != commandcatalog.Palette || started.CommandID != request.CommandID {
		t.Fatalf("invocation entregue ao handler não deriva do contexto real: %#v", started)
	}
}

func (f *executionFixture) registryDefinition(id string) (commandcatalog.Definition, bool) {
	return f.service.config.Registry.Lookup(id)
}

func TestServiceConflictsOnReusedInvocationIDWithDifferentCommandOrCorrelation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*executionFixture, Request) Request
	}{
		{name: "command_id", mutate: func(f *executionFixture, request Request) Request {
			request.CommandID = "workspace.status"
			return request
		}},
		{name: "correlation_id", mutate: func(f *executionFixture, request Request) Request {
			request.CorrelationID = newTestUUID()
			return request
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newExecutionFixture(t)
			request := f.request(t, "workspace.read")
			first, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
			if err != nil || first.Status != commandledger.Succeeded {
				t.Fatalf("execução inicial: %+v %v", first, err)
			}

			_, err = f.service.Execute(context.Background(), f.pair.AccessToken, tc.mutate(f, request))
			if !errors.Is(err, commandledger.ErrConflict) {
				t.Fatalf("conflito = %v, want %v", err, commandledger.ErrConflict)
			}
			if got := f.startCalls.Load(); got != 1 {
				t.Fatalf("Start calls = %d, want 1", got)
			}
			if got := f.record(t, request).Status; got != commandledger.Succeeded {
				t.Fatalf("registro original foi alterado para %q", got)
			}
		})
	}
}

func TestServiceGetInvocationQueriesWithoutStartingOrReserving(t *testing.T) {
	f := newExecutionFixture(t)
	request := f.request(t, "workspace.read")
	first, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
	if err != nil || first.Status != commandledger.Succeeded {
		t.Fatalf("execução inicial: record=%+v err=%v", first, err)
	}

	queried, err := f.service.GetInvocation(context.Background(), f.pair.AccessToken, request)
	if err != nil || queried.ID != first.ID || queried.Status != commandledger.Succeeded {
		t.Fatalf("consulta existente: record=%+v err=%v", queried, err)
	}
	if got := f.startCalls.Load(); got != 1 {
		t.Fatalf("GetInvocation chamou Start: %d", got)
	}

	secondPair, err := f.sessions.IssueSession(context.Background(), f.user, "segunda-sessao")
	if err != nil {
		t.Fatalf("emitir segunda sessão: %v", err)
	}
	if _, err := f.service.GetInvocation(context.Background(), secondPair.AccessToken, request); !errors.Is(err, commandledger.ErrNotFound) {
		t.Fatalf("consulta cross-session = %v, want %v", err, commandledger.ErrNotFound)
	}
	if got := f.startCalls.Load(); got != 1 {
		t.Fatalf("consulta cross-session chamou Start: %d", got)
	}

	f.authorizeHook = func(int) error { return errors.New("política revogada") }
	if _, err := f.service.GetInvocation(context.Background(), f.pair.AccessToken, request); !errors.Is(err, ErrDenied) {
		t.Fatalf("consulta após revogação = %v, want %v", err, ErrDenied)
	}
	if got := f.record(t, request).Status; got != commandledger.Succeeded {
		t.Fatalf("consulta negada alterou registro: %q", got)
	}

	f.authorizeHook = func(int) error { return nil }
	missing := f.request(t, "workspace.read")
	if _, err := f.service.GetInvocation(context.Background(), f.pair.AccessToken, missing); !errors.Is(err, commandledger.ErrNotFound) {
		t.Fatalf("consulta inexistente = %v, want %v", err, commandledger.ErrNotFound)
	}
	var ledgers int64
	if err := f.db.Table("command_idempotency_keys").Count(&ledgers).Error; err != nil {
		t.Fatal(err)
	}
	if ledgers != 1 {
		t.Fatalf("consulta inexistente criou ledger: %d", ledgers)
	}
}
