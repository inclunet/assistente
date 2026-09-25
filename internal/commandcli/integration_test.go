package commandcli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Prova o protocolo sobre o executor e ledger reais. O comando de fixture não
// é registrado no produto e não amplia AllowedSources de comandos visuais.
func TestCLIRealExecutorReplayOwnershipAndRedactedStatus(t *testing.T) {
	ctx := context.Background()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "cli.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := commandledger.Migrate(ctx, db); err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatal(err)
	}
	user := database.User{Username: "cli-fixture", PasswordHash: "unused", Role: database.UserRoleUser, IsActive: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{RefreshTokenPepper: bytes.Repeat([]byte{11}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	pair, err := sessions.IssueSession(ctx, &user, "cli-fixture")
	if err != nil {
		t.Fatal(err)
	}
	principal := auth.LocalSessionPrincipal{UserID: user.ID, SessionID: pair.SessionID}
	epochs, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	store, err := commandledger.New(db, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentials.NewManager(bytes.Repeat([]byte{12}, 32))
	if err := manager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{13}, 32))); err != nil {
		t.Fatal(err)
	}
	keys, err := commandledger.NewCredentialKeyProvider(manager)
	if err != nil {
		t.Fatal(err)
	}
	d := registryDefinition(t)
	d.Risk = commandcatalog.RiskLow
	d.Scopes = []commandcatalog.Scope{commandcatalog.ScopeSession}
	d.Persistence = commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceNever, Result: commandcatalog.PersistenceNever, Audit: commandcatalog.PersistenceRedacted}
	d.ResultSchema = &commandcatalog.Schema{Type: commandcatalog.SchemaObject, Properties: map[string]commandcatalog.Schema{"answer": {Type: commandcatalog.SchemaInteger}}}
	contract := commandcatalog.HandlerContract{Effect: d.Effect, Route: d.HandlerRoute, Classification: d.HandlerClassification}
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{Definition: d, Handler: contract}})
	if err != nil {
		t.Fatal(err)
	}
	bus, err := commandcontext.NewFactBus(map[string]commandcontext.ScopedProvider{})
	if err != nil {
		t.Fatal(err)
	}
	authenticate := func(check context.Context) error {
		_, err := sessions.RevalidateLocalSession(check, principal)
		return err
	}
	var calls atomic.Int32
	executor, err := commandexecution.NewComplete(commandexecution.Config{
		Epochs: epochs, Store: store, Registry: registry, RegistryVersion: "cli-test-v1", Source: commandcatalog.CLI,
		Keys: keys, KeyVersion: "v1", Now: time.Now, Retention: time.Hour, ExecutionTimeout: time.Second, FinalizationTimeout: time.Second,
		Handlers: map[string]commandexecution.Handler{d.ID: {Contract: contract, Start: func(context.Context, commandexecution.Invocation) (commandexecution.ExecutionHandle, error) {
			calls.Add(1)
			done := make(chan commandexecution.Outcome, 1)
			done <- commandexecution.Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`{"answer":42}`)}
			return commandexecution.ExecutionHandle{ID: uuid.Must(uuid.NewV7()).String(), Done: done, Cancel: func() {}}, nil
		}}},
		Envelope: &commandexecution.EnvelopeConfig{Context: bus, Identity: &commandexecution.EnvelopeIdentityPorts{
			Authenticate: func(check context.Context, token string) (commandexecution.EnvelopeAuthenticatedIdentity, error) {
				if token != "" {
					return commandexecution.EnvelopeAuthenticatedIdentity{}, commandexecution.ErrDenied
				}
				if err := authenticate(check); err != nil {
					return commandexecution.EnvelopeAuthenticatedIdentity{}, err
				}
				userID, sessionID := principal.UserID, principal.SessionID
				return commandexecution.EnvelopeAuthenticatedIdentity{Ownership: commandledger.FullOwnership{UserID: &userID, AuthContextType: commandcontract.AuthLocalSession, AuthContextID: sessionID, ActorType: commandcontract.ActorUser, ActorID: userID}, ContextPrincipal: commandsecurity.ContextPrincipal{UserID: userID, Type: string(commandcontract.AuthLocalSession), ID: sessionID}, WireSessionID: &sessionID}, nil
			},
			Snapshot: func(context.Context, commandledger.FullOwnership, commandexecution.EnvelopeCandidate) (commandcontract.Envelope, error) {
				global, layers := "global-v1", "layers-v1"
				return commandcontract.Envelope{RegistryVersion: "cli-test-v1", GlobalConfigGeneration: &global, ActiveLayersGeneration: &layers}, nil
			},
			Resolve: func(context.Context, commandledger.FullOwnership, commandexecution.EnvelopeCandidate, commandcontract.Envelope) (commandexecution.EnvelopeResolution, error) {
				return commandexecution.EnvelopeResolution{}, commandexecution.ErrDenied
			},
			Authorize: func(check context.Context, _ commandledger.FullOwnership, _ commandcontract.Envelope, definition commandcatalog.Definition) error {
				if UnavailableReason(definition) != "" {
					return commandexecution.ErrDenied
				}
				return authenticate(check)
			},
			AuthorizeLookup: func(check context.Context, _ commandledger.FullOwnership, record commandledger.FullRecord) error {
				if record.SourceType == nil || *record.SourceType != commandcontract.SourceCLI {
					return commandexecution.ErrDenied
				}
				return authenticate(check)
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = executor.Shutdown(context.Background()) })
	cli, err := New(Config{Registry: registry, Executor: executor, Authenticate: authenticate})
	if err != nil {
		t.Fatal(err)
	}
	result, err := cli.Execute(ctx, Request{CommandID: d.ID, Arguments: json.RawMessage(`{"limit":1}`)})
	if err != nil || result.Status != string(commandledger.Succeeded) || string(result.Output) != `{"answer":42}` || calls.Load() != 1 {
		t.Fatalf("execute: %+v %v calls=%d", result, err, calls.Load())
	}
	status, err := cli.Status(ctx, result.RequestID)
	if err != nil || status.Status != string(commandledger.Succeeded) || len(status.Output) != 0 {
		t.Fatalf("status: %+v %v", status, err)
	}
	retry, err := cli.Retry(ctx, Request{CommandID: d.ID, Arguments: json.RawMessage(`{ "limit": 1 }`), RequestID: result.RequestID})
	if err != nil || retry.Status != string(commandledger.Succeeded) || len(retry.Output) != 0 || calls.Load() != 1 {
		t.Fatalf("replay: %+v %v calls=%d", retry, err, calls.Load())
	}
	_, err = cli.Retry(ctx, Request{CommandID: d.ID, Arguments: json.RawMessage(`{"limit":2}`), RequestID: result.RequestID})
	if !errors.Is(err, commandledger.ErrConflict) || calls.Load() != 1 {
		t.Fatalf("changed args: %v calls=%d", err, calls.Load())
	}
	_, err = cli.Retry(ctx, Request{CommandID: "other.command", Arguments: json.RawMessage(`{}`), RequestID: result.RequestID})
	if !errors.Is(err, ErrCommandMismatch) {
		t.Fatalf("changed command: %v", err)
	}
	_, err = cli.Retry(ctx, Request{CommandID: d.ID, Arguments: json.RawMessage(`{}`), RequestID: uuid.Must(uuid.NewV7()).String()})
	if !errors.Is(err, commandledger.ErrNotFound) || calls.Load() != 1 {
		t.Fatalf("unknown retry: %v calls=%d", err, calls.Load())
	}
	second, err := sessions.IssueSession(ctx, &user, "different-session")
	if err != nil {
		t.Fatal(err)
	}
	principal.SessionID = second.SessionID
	if _, err = cli.Status(ctx, result.RequestID); !errors.Is(err, commandledger.ErrNotFound) {
		t.Fatalf("foreign session status: %v", err)
	}
	if _, err = cli.Retry(ctx, Request{CommandID: d.ID, Arguments: json.RawMessage(`{"limit":1}`), RequestID: result.RequestID}); !errors.Is(err, commandledger.ErrNotFound) || calls.Load() != 1 {
		t.Fatalf("foreign session retry: %v calls=%d", err, calls.Load())
	}
}
