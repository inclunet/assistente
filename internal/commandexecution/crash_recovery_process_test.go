package commandexecution

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandcontract"
	"assistente/internal/commandinstance"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

const (
	c51CrashOptInEnv  = "ASSISTENTE_C51_PROCESS_CRASH_OPT_IN"
	c51CrashChildEnv  = "ASSISTENTE_C51_PROCESS_CRASH_CHILD"
	c51CrashDBEnv     = "ASSISTENTE_C51_PROCESS_CRASH_DB"
	c51CrashTokenEnv  = "ASSISTENTE_C51_PROCESS_CRASH_TOKEN"
	c51CrashInvokeEnv = "ASSISTENTE_C51_PROCESS_CRASH_INVOCATION"
	c51CrashCorrEnv   = "ASSISTENTE_C51_PROCESS_CRASH_CORRELATION"
	c51CrashEffects   = "c51_handler_effects"
	c51CrashCommandID = "workspace.read"
)

func c51CrashCandidate(invocationID, correlationID string) EnvelopeCandidate {
	return EnvelopeCandidate{InvocationID: invocationID, CorrelationID: correlationID,
		TriggerType: string(commandcontract.SourcePalette),
		TriggerSpec: json.RawMessage(`{"version":1,"selection":"workspace.read"}`), Arguments: json.RawMessage(`{}`)}
}

// This deliberately destructive integration test is opt-in. It starts the
// current go test executable, observes a committed handler effect, then kills
// only that child process to exercise the persisted instance/recovery protocol.
func TestC51CrashReplayAfterCommittedHandlerEffectOptIn(t *testing.T) {
	if os.Getenv(c51CrashOptInEnv) != "1" {
		t.Skip("C51 process-crash test requires explicit opt-in")
	}

	dbPath := filepath.Join(t.TempDir(), "c51-process-crash.sqlite")
	token := seedC51CrashDatabase(t, dbPath)
	request := c51CrashCandidate(newTestUUID(), newTestUUID())

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, "-test.run=^TestC51CrashReplayChildProcess$", "-test.timeout=60s")
	cmd.WaitDelay = 2 * time.Second
	cmd.Env = c51ChildEnvironment(os.Environ(), map[string]string{
		c51CrashOptInEnv: "1", c51CrashChildEnv: "1", c51CrashDBEnv: dbPath,
		c51CrashTokenEnv: token, c51CrashInvokeEnv: request.InvocationID, c51CrashCorrEnv: request.CorrelationID,
	})
	var childOutput c51SynchronizedBuffer
	cmd.Stdout, cmd.Stderr = &childOutput, &childOutput
	if err := cmd.Start(); err != nil {
		t.Fatalf("iniciar o binário go test filho: %v", err)
	}
	waited := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(waited)
	}()
	childRunning := true
	defer func() {
		if childRunning {
			_ = cmd.Process.Kill()
			select {
			case <-waited:
			case <-time.After(3 * time.Second):
			}
		}
	}()

	reader := openC51CrashDB(t, dbPath)
	defer func() {
		if reader != nil {
			closeC51CrashDB(t, reader)
		}
	}()
	waitForC51CommittedEffect(t, ctx, reader, request.InvocationID, waited, &childOutput)

	// The process lock is independent of the SQLite read connection. A second
	// core must fail closed while the child still owns the real file lock.
	probe, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	if err := probe.BindInstance(ctx, reader); !errors.Is(err, commandinstance.ErrBusy) {
		t.Fatalf("instância foi adquirida enquanto o filho continua vivo: %v", err)
	}

	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("matar somente cmd.Process do filho: %v", err)
	}
	select {
	case <-waited:
		childRunning = false
		if waitErr == nil {
			t.Fatal("filho terminou normalmente; esperava encerramento abrupto")
		}
	case <-ctx.Done():
		t.Fatalf("Wait não confirmou a queda do filho: %v; saída=%s", ctx.Err(), childOutput.String())
	}

	closeC51CrashDB(t, reader)
	reader = nil
	if got := c51CrashEffectCount(t, dbPath, request.InvocationID); got != 1 {
		t.Fatalf("efeitos confirmados antes/depois da queda=%d, want 1", got)
	}

	restarted := newC51CrashRuntime(t, dbPath, token)
	proof, err := restarted.epochs.RestartProof(ctx, restarted.db)
	if err != nil || !proof.Valid() {
		t.Fatalf("prova real de restart: valid=%v err=%v", proof.Valid(), err)
	}
	var securityGeneration string
	if err := restarted.db.Table("command_invocations").Select("security_generation").Where("invocation_id = ?", request.InvocationID).Scan(&securityGeneration).Error; err != nil || securityGeneration == "" || !proof.Includes(securityGeneration) {
		t.Fatalf("prova não cobre a geração da invocação interrompida: generation=%q valid=%v err=%v", securityGeneration, proof.Valid(), err)
	}
	recovery, err := commandledger.NewCoordinatorRecovery(restarted.store, commandsecurity.FromRestartProof(proof))
	if err != nil {
		t.Fatalf("montar adapter real de recovery: %v", err)
	}
	batch, err := recovery.Recover(ctx, 128)
	if err != nil || batch.Processed != 1 || batch.More {
		t.Fatalf("recovery da geração abandonada: %+v err=%v", batch, err)
	}
	assertC51CrashPairStatus(t, restarted.db, request.InvocationID, commandledger.OutcomeUnknown)
	assertC51CrashResultRedacted(t, restarted.db, request.InvocationID)
	queried, err := restarted.service.GetEnvelopeInvocation(ctx, token, request.InvocationID)
	if err != nil || queried.Status != commandledger.OutcomeUnknown {
		t.Fatalf("GetInvocation após recovery: %+v err=%v", queried, err)
	}
	secondRecovery, err := recovery.Recover(ctx, 128)
	if err != nil || secondRecovery.Processed != 0 || secondRecovery.More {
		t.Fatalf("segunda passagem de recovery não foi idempotente: %+v err=%v", secondRecovery, err)
	}

	if replay, err := restarted.service.ExecuteEnvelope(ctx, token, request); err != nil || replay.Status != commandledger.OutcomeUnknown {
		t.Fatalf("replay da mesma invocação após recovery: %+v err=%v", replay, err)
	}
	if replay, err := restarted.service.ExecuteEnvelope(ctx, token, request); err != nil || replay.Status != commandledger.OutcomeUnknown {
		t.Fatalf("replay repetido após recovery: %+v err=%v", replay, err)
	}
	if got := restarted.handlerStarts.Load(); got != 0 {
		t.Fatalf("replay reiniciou handler no processo novo: starts=%d", got)
	}
	if got := c51CrashEffectCount(t, dbPath, request.InvocationID); got != 1 {
		t.Fatalf("efeito observável foi duplicado no replay: count=%d", got)
	}
}

// TestC51CrashReplayChildProcess is selected only by the parent subprocess.
// The service's 25-second execution deadline would finalize the invocation if
// the parent did not intervene; the parent must kill this process after the
// committed handler effect is visible and before that deadline.
func TestC51CrashReplayChildProcess(t *testing.T) {
	if os.Getenv(c51CrashOptInEnv) != "1" || os.Getenv(c51CrashChildEnv) != "1" {
		t.Skip("helper subprocess interno")
	}
	dbPath, token := os.Getenv(c51CrashDBEnv), os.Getenv(c51CrashTokenEnv)
	request := c51CrashCandidate(os.Getenv(c51CrashInvokeEnv), os.Getenv(c51CrashCorrEnv))
	if dbPath == "" || token == "" || !validID(request.InvocationID) || !validID(request.CorrelationID) {
		t.Fatal("entrada do helper C51 inválida")
	}
	runtime := newC51CrashRuntime(t, dbPath, token)
	runtime.blockAfterCommit = true
	_, _ = runtime.service.ExecuteEnvelope(context.Background(), token, request)
	t.Fatal("handler bloqueado retornou inesperadamente; o pai deveria encerrar este filho")
}

type c51CrashRuntime struct {
	db               *gorm.DB
	epochs           *commandsecurity.EpochService
	store            *commandledger.Store
	service          *Service
	blockAfterCommit bool
	handlerStarts    atomic.Int32
}

func newC51CrashRuntime(t *testing.T, dbPath, token string) *c51CrashRuntime {
	t.Helper()
	db := openC51CrashDB(t, dbPath)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.Exec("PRAGMA synchronous = FULL").Error; err != nil {
		t.Fatal(err)
	}
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	if err := epochs.BindInstance(context.Background(), db); err != nil {
		t.Fatalf("bind real process instance: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := epochs.CloseAndDrain(ctx); err != nil {
			t.Errorf("drain do core C51: %v", err)
			return
		}
		if err := epochs.ReleaseInstance(ctx); err != nil {
			t.Errorf("release da lease C51: %v", err)
		}
	})
	store, err := commandledger.New(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	sessionConfig, err := c51SessionConfig()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(db, sessionConfig)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentials.NewManager(bytes.Repeat([]byte{0x43}, 32))
	if err := manager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", "QkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkJCQkI"); err != nil {
		t.Fatal(err)
	}
	keys, err := commandledger.NewCredentialKeyProvider(manager)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{
		pipelineRegistration(c51CrashCommandID, commandcatalog.Read, commandcatalog.NoDecision, false,
			commandcatalog.ContextPolicy{None: true}, []commandcatalog.Source{commandcatalog.Palette}, "internal/c51/read"),
	})
	if err != nil {
		t.Fatal(err)
	}
	bus, err := commandcontext.NewFactBus(nil)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &c51CrashRuntime{db: db, epochs: epochs, store: store}
	start := func(ctx context.Context, invocation Invocation) (ExecutionHandle, error) {
		done := make(chan Outcome, 1)
		handle := ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() {}}
		go func() {
			runtime.handlerStarts.Add(1)
			if err := db.WithContext(ctx).Exec("INSERT INTO "+c51CrashEffects+" (invocation_id) VALUES (?)", invocation.ID).Error; err != nil {
				done <- Outcome{Status: commandledger.Failed, Result: json.RawMessage(`{}`)}
				return
			}
			if runtime.blockAfterCommit {
				// Keep Done pending after the committed marker. The parent observes
				// running in the ledger/audit and kills this process before timeout.
				return
			}
			done <- Outcome{Status: commandledger.Succeeded}
		}()
		return handle, nil
	}
	service, err := NewComplete(Config{
		Envelope: &EnvelopeConfig{
			Snapshot: func(ctx context.Context, _ auth.LocalSessionPrincipal, candidate EnvelopeCandidate) (commandcontract.Envelope, error) {
				if err := ctx.Err(); err != nil {
					return commandcontract.Envelope{}, err
				}
				return commandcontract.Envelope{RegistryVersion: "c51-registry-v1", GlobalConfigGeneration: c51StringPtr("c51-global-v1"),
					ActiveLayersGeneration: c51StringPtr("c51-layers-v1"), CorrelationID: candidate.CorrelationID}, nil
			},
			Resolve: func(ctx context.Context, _ auth.LocalSessionPrincipal, _ EnvelopeCandidate, _ commandcontract.Envelope) (EnvelopeResolution, error) {
				if err := ctx.Err(); err != nil {
					return EnvelopeResolution{}, err
				}
				return EnvelopeResolution{Mode: commandcontract.ResolutionExecute, CommandID: c51CrashCommandID,
					Arguments: json.RawMessage(`{}`), BindingIDs: []string{}}, nil
			},
			Authorize: func(ctx context.Context, _ auth.LocalSessionPrincipal, _ commandcontract.Envelope, _ commandcatalog.Definition) error {
				return ctx.Err()
			},
			AuthorizeLookup: func(ctx context.Context, _ auth.LocalSessionPrincipal, _ commandledger.FullRecord) error {
				return ctx.Err()
			},
			Actor: func(ctx context.Context, principal auth.LocalSessionPrincipal) (commandcontract.ActorType, string, error) {
				return commandcontract.ActorUser, principal.UserID, ctx.Err()
			},
			Context: bus,
		},
		Sessions: sessions, Epochs: epochs, Store: store, Registry: registry, RegistryVersion: "c51-registry-v1",
		Handlers: map[string]Handler{c51CrashCommandID: {Contract: pipelineContract(commandcatalog.Read, false, "internal/c51/read"), Start: start}},
		Source:   commandcatalog.Palette,
		Keys:     keys, KeyVersion: "v1", Now: time.Now, Retention: time.Hour,
		ExecutionTimeout: 25 * time.Second, FinalizationTimeout: 3 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime.service = service
	return runtime
}

func c51StringPtr(value string) *string { return &value }

func seedC51CrashDatabase(t *testing.T, path string) string {
	t.Helper()
	db := openC51CrashDB(t, path)
	if err := commandledger.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := commandinstance.Migrate(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE " + c51CrashEffects + " (id INTEGER PRIMARY KEY AUTOINCREMENT, invocation_id TEXT NOT NULL)").Error; err != nil {
		t.Fatal(err)
	}
	user := &database.User{Username: "c51-process-crash", PasswordHash: "unused", Role: database.UserRoleUser, IsActive: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	sessionConfig, err := c51SessionConfig()
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(db, sessionConfig)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := sessions.IssueSession(context.Background(), user, "c51-child-restart-test")
	if err != nil {
		t.Fatal(err)
	}
	closeC51CrashDB(t, db)
	return pair.AccessToken
}

func c51SessionConfig() (auth.SessionConfig, error) {
	seed := bytes.Repeat([]byte{0x24}, ed25519.SeedSize)
	signer, err := auth.NewTokenSignerFromPrivateKey(ed25519.NewKeyFromSeed(seed))
	if err != nil {
		return auth.SessionConfig{}, err
	}
	return auth.SessionConfig{Issuer: "c51-crash-test", Audience: "c51-crash-test-client", AccessTTL: time.Hour,
		RefreshTTL: time.Hour, Signer: signer, RefreshTokenPepper: bytes.Repeat([]byte{0x21}, 32)}, nil
}

func openC51CrashDB(t *testing.T, path string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("PRAGMA busy_timeout = 5000").Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func closeC51CrashDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
}

func c51ChildEnvironment(current []string, values map[string]string) []string {
	result := make([]string, 0, len(current)+len(values))
	for _, entry := range current {
		name, _, _ := strings.Cut(entry, "=")
		if _, replace := values[name]; !replace {
			result = append(result, entry)
		}
	}
	for name, value := range values {
		result = append(result, name+"="+value)
	}
	return result
}

type c51SynchronizedBuffer struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *c51SynchronizedBuffer) Write(value []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(value)
}

func (b *c51SynchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.String()
}

func waitForC51CommittedEffect(t *testing.T, ctx context.Context, db *gorm.DB, invocationID string, waited <-chan struct{}, childOutput fmt.Stringer) {
	t.Helper()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		var effects, ledgerRunning, auditRunning int64
		if err := db.Table(c51CrashEffects).Where("invocation_id = ?", invocationID).Count(&effects).Error; err == nil && effects == 1 {
			if err := db.Table("command_idempotency_keys").Where("invocation_id = ? AND status = ?", invocationID, commandledger.Running).Count(&ledgerRunning).Error; err != nil || ledgerRunning != 1 {
				t.Fatalf("efeito confirmado sem ledger Running: count=%d err=%v", ledgerRunning, err)
			}
			if err := db.Table("command_invocations").Where("invocation_id = ? AND status = ?", invocationID, commandledger.Running).Count(&auditRunning).Error; err != nil || auditRunning != 1 {
				t.Fatalf("efeito confirmado sem auditoria Running: count=%d err=%v", auditRunning, err)
			}
			return
		}
		select {
		case <-waited:
			t.Fatalf("filho terminou antes do commit observável; saída=%s", childOutput.String())
		case <-ctx.Done():
			t.Fatalf("timeout esperando commit do handler: %v; saída=%s", ctx.Err(), childOutput.String())
		case <-ticker.C:
		}
	}
}

func c51CrashEffectCount(t *testing.T, path, invocationID string) int64 {
	t.Helper()
	db := openC51CrashDB(t, path)
	defer closeC51CrashDB(t, db)
	var count int64
	if err := db.Table(c51CrashEffects).Where("invocation_id = ?", invocationID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func assertC51CrashPairStatus(t *testing.T, db *gorm.DB, invocationID string, expected commandledger.Status) {
	t.Helper()
	for _, table := range []string{"command_idempotency_keys", "command_invocations"} {
		var status string
		if err := db.Table(table).Select("status").Where("invocation_id = ?", invocationID).Scan(&status).Error; err != nil || commandledger.Status(status) != expected {
			t.Fatalf("%s status=%q, want %q; err=%v", table, status, expected, err)
		}
	}
}

func assertC51CrashResultRedacted(t *testing.T, db *gorm.DB, invocationID string) {
	t.Helper()
	for _, table := range []string{"command_idempotency_keys", "command_invocations"} {
		var summary string
		if err := db.Table(table).Select("result_summary").Where("invocation_id = ?", invocationID).Scan(&summary).Error; err != nil || summary != "{}" {
			t.Fatalf("%s result_summary=%q, want redacted empty object; err=%v", table, summary, err)
		}
	}
}
