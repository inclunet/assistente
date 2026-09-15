package commandexecution

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandcontract"
	"assistente/internal/commanddecision"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"gorm.io/gorm"
)

type envelopePipelineFixture struct {
	t              *testing.T
	db             *gorm.DB
	sessions       *auth.SessionService
	epochs         *commandsecurity.EpochService
	store          *commandledger.Store
	service        *Service
	registry       *commandcatalog.Registry
	user           string
	session        string
	token          string
	now            time.Time
	keys           commandledger.FingerprintKeyProvider
	provider       *pipelineProvider
	startCalls     atomic.Int32
	resolveCalls   atomic.Int32
	authorizeCalls atomic.Int32
	lookupCalls    atomic.Int32
	start          func(context.Context, Invocation) (ExecutionHandle, error)
	resolve        func(EnvelopeCandidate) (EnvelopeResolution, error)
	authorize      func(int) error
	lookup         func(int) error
	awaitQueue     func(context.Context, commandcontract.Envelope) error
}

type pipelineProvider struct {
	fixture *envelopePipelineFixture
	calls   atomic.Int32
	panic   atomic.Bool
	mode    atomic.Int32
}

func (p *pipelineProvider) Snapshot(ctx context.Context, scope commandcontext.Scope, fact string) (commandcontext.OwnedSnapshot, error) {
	if err := ctx.Err(); err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	if p.panic.Load() {
		panic("provider fake panic")
	}
	if fact != "active" {
		return commandcontext.OwnedSnapshot{}, commandcontext.ErrProviderUnavailable
	}
	call := p.calls.Add(1)
	captured := p.fixture.now
	if p.mode.Load() == 1 && call == 1 {
		captured = captured.Add(-10 * time.Second)
	}
	owned, err := commandcontext.NewOwnedSnapshot(scope, commandcontext.Snapshot{Version: "context-v1", CapturedAt: captured})
	if err != nil {
		return commandcontext.OwnedSnapshot{}, err
	}
	return owned, nil
}

func newEnvelopePipelineFixture(t *testing.T) *envelopePipelineFixture {
	t.Helper()
	base := newExecutionFixture(t)
	if err := commanddecision.Migrate(context.Background(), base.db); err != nil {
		t.Fatalf("migrar receipts: %v", err)
	}
	keys := pipelineKeyProvider(t)
	registry, err := pipelineRegistry()
	if err != nil {
		t.Fatalf("criar catálogo completo: %v", err)
	}
	f := &envelopePipelineFixture{
		t: t, db: base.db, sessions: base.sessions, epochs: base.epochs, store: base.store,
		registry: registry, user: base.user.ID, session: base.pair.SessionID, token: base.pair.AccessToken,
		now: base.now, keys: keys,
	}
	f.provider = &pipelineProvider{fixture: f}
	f.start = func(_ context.Context, _ Invocation) (ExecutionHandle, error) {
		return pipelineCompletedHandle(commandledger.Succeeded), nil
	}
	f.resolve = func(c EnvelopeCandidate) (EnvelopeResolution, error) {
		f.resolveCalls.Add(1)
		return EnvelopeResolution{Mode: commandcontract.ResolutionExecute, CommandID: "pipe.write", Arguments: json.RawMessage(`{}`), BindingIDs: []string{"binding.pipeline"}}, nil
	}
	f.authorize = func(int) error { return nil }
	f.lookup = func(int) error { return nil }
	f.awaitQueue = func(context.Context, commandcontract.Envelope) error { return nil }
	bus, err := commandcontext.NewFactBus(map[string]commandcontext.ScopedProvider{"workspace": f.provider})
	if err != nil {
		t.Fatal(err)
	}
	start := func(ctx context.Context, invocation Invocation) (ExecutionHandle, error) {
		f.startCalls.Add(1)
		return f.start(ctx, invocation)
	}
	config := Config{
		Envelope: &EnvelopeConfig{
			Snapshot: f.snapshot,
			Resolve: func(_ context.Context, _ auth.LocalSessionPrincipal, c EnvelopeCandidate, _ commandcontract.Envelope) (EnvelopeResolution, error) {
				return f.resolve(c)
			},
			Authorize: func(_ context.Context, _ auth.LocalSessionPrincipal, _ commandcontract.Envelope, _ commandcatalog.Definition) error {
				call := int(f.authorizeCalls.Add(1))
				return f.authorize(call)
			},
			AuthorizeLookup: func(_ context.Context, _ auth.LocalSessionPrincipal, _ commandledger.FullRecord) error {
				call := int(f.lookupCalls.Add(1))
				return f.lookup(call)
			},
			Actor: func(context.Context, auth.LocalSessionPrincipal) (commandcontract.ActorType, string, error) {
				return commandcontract.ActorUser, f.user, nil
			},
			Context: bus, Decisions: nil, DecisionTTL: time.Minute,
			DecisionBody: func(commandcatalog.Definition, commandcontract.Envelope) (string, error) {
				return `{"version":1,"confirm":true}`, nil
			},
			AwaitQueue: func(ctx context.Context, e commandcontract.Envelope) error { return f.awaitQueue(ctx, e) },
		},
		Sessions: base.sessions, Epochs: base.epochs, Store: base.store, Registry: registry,
		RegistryVersion: "registry-v1", Handlers: pipelineHandlers(start), Source: commandcatalog.Palette,
		Keys: keys, KeyVersion: "v1", Now: func() time.Time { return f.now }, Retention: time.Hour,
		ExecutionTimeout: 2 * time.Second, FinalizationTimeout: 2 * time.Second,
	}
	f.service, err = NewComplete(config)
	if err != nil {
		t.Fatalf("criar serviço completo: %v", err)
	}
	return f
}

func pipelineKeyProvider(t *testing.T) commandledger.FingerprintKeyProvider {
	t.Helper()
	key := bytes.Repeat([]byte{0x51}, 32)
	manager := credentials.NewManager(bytes.Repeat([]byte{0x52}, 32))
	if err := manager.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(key)); err != nil {
		t.Fatal(err)
	}
	provider, err := commandledger.NewCredentialKeyProvider(manager)
	if err != nil {
		t.Fatal(err)
	}
	return provider
}

func pipelineHandlers(start func(context.Context, Invocation) (ExecutionHandle, error)) map[string]Handler {
	return map[string]Handler{
		"pipe.read":    {Contract: pipelineContract(commandcatalog.Read, false, "internal/pipe/read"), Start: start},
		"pipe.write":   {Contract: pipelineContract(commandcatalog.Write, true, "internal/pipe/write"), Start: start},
		"pipe.destroy": {Contract: pipelineContract(commandcatalog.Destructive, true, "internal/pipe/destroy"), Start: start},
	}
}

func pipelineContract(effect commandcatalog.Effect, mutable bool, route string) commandcatalog.HandlerContract {
	return commandcatalog.HandlerContract{Effect: effect, HasMutableTarget: mutable, Route: route, Classification: commandcatalog.HandlerInternal}
}

func pipelineRegistry() (*commandcatalog.Registry, error) {
	return commandcatalog.NewComplete([]commandcatalog.Registration{
		pipelineRegistration("pipe.read", commandcatalog.Read, commandcatalog.NoDecision, false, commandcatalog.ContextPolicy{None: true}, []commandcatalog.Source{commandcatalog.Palette}, "internal/pipe/read"),
		pipelineRegistration("pipe.write", commandcatalog.Write, commandcatalog.NoDecision, true, commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active", Mode: commandcatalog.MaxAge, MaxAgeMS: 5000}}}, []commandcatalog.Source{commandcatalog.Palette}, "internal/pipe/write"),
		pipelineRegistration("pipe.destroy", commandcatalog.Destructive, commandcatalog.Interactive, true, commandcatalog.ContextPolicy{Facts: []commandcatalog.ContextFact{{Provider: "workspace", Fact: "active", Mode: commandcatalog.ExactVersion}}}, []commandcatalog.Source{commandcatalog.Palette, commandcatalog.UI}, "internal/pipe/destroy"),
	})
}

func pipelineRegistration(id string, effect commandcatalog.Effect, decision commandcatalog.Decision, mutable bool, contextPolicy commandcatalog.ContextPolicy, sources []commandcatalog.Source, route string) commandcatalog.Registration {
	locales := map[string]commandcatalog.LocalizedMetadata{
		"pt-BR": {Name: id, Description: id, Category: "Test"},
		"en":    {Name: id, Description: id, Category: "Test"},
		"es":    {Name: id, Description: id, Category: "Test"},
	}
	argumentsSchema := &commandcatalog.Schema{Type: commandcatalog.SchemaObject}
	if id == "pipe.read" {
		argumentsSchema.Properties = map[string]commandcatalog.Schema{
			"secret": {Type: commandcatalog.SchemaString, Optional: true},
		}
	}
	return commandcatalog.Registration{
		Definition: commandcatalog.Definition{
			ID: id, Effect: effect, Decision: decision, HasMutableTarget: mutable, AllowedSources: sources,
			Context: contextPolicy, Presentation: &commandcatalog.Presentation{Version: "test-v1", Locales: locales},
			ArgumentsSchema: argumentsSchema, ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject},
			Risk: commandcatalog.RiskMedium, Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted},
			Scopes: []commandcatalog.Scope{commandcatalog.ScopeSession}, Availability: commandcatalog.Availability{Status: commandcatalog.Available},
			HandlerRoute: route, HandlerClassification: commandcatalog.HandlerInternal,
		},
		Handler: pipelineContract(effect, mutable, route),
	}
}

func (f *envelopePipelineFixture) snapshot(ctx context.Context, _ auth.LocalSessionPrincipal, c EnvelopeCandidate) (commandcontract.Envelope, error) {
	if err := ctx.Err(); err != nil {
		return commandcontract.Envelope{}, err
	}
	return commandcontract.Envelope{
		RegistryVersion: "registry-v1", GlobalConfigGeneration: pipelineStringPtr("global-v1"), ActiveLayersGeneration: pipelineStringPtr("layers-v1"),
		CorrelationID: c.CorrelationID, WorkspaceID: c.WorkspaceID,
	}, nil
}

func (f *envelopePipelineFixture) candidate(id, command string, args string) EnvelopeCandidate {
	return EnvelopeCandidate{InvocationID: id, CorrelationID: newTestUUID(), CommandID: command, Arguments: json.RawMessage(args)}
}

func (f *envelopePipelineFixture) trigger(id string) EnvelopeCandidate {
	return EnvelopeCandidate{InvocationID: id, CorrelationID: newTestUUID(), TriggerType: string(commandcontract.SourcePalette), TriggerSpec: json.RawMessage(`{"version":1,"selection":"pipe.write"}`), Arguments: json.RawMessage(`{}`)}
}

func pipelineStringPtr(value string) *string { return &value }

func pipelineCompletedHandle(status commandledger.Status) ExecutionHandle {
	done := make(chan Outcome, 1)
	done <- Outcome{Status: status, Result: json.RawMessage(`{}`)}
	return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() {}}
}

type pipelineDecisionPresenter struct {
	action string
	calls  atomic.Int32
}

func (p *pipelineDecisionPresenter) Present(_ context.Context, request commanddecision.Request) (commanddecision.Response, error) {
	p.calls.Add(1)
	return commanddecision.Response{DecisionID: request.DecisionID, ActionID: p.action}, nil
}

func TestEnvelopePipelineDiretoReadEWrite(t *testing.T) {
	for _, command := range []string{"pipe.read", "pipe.write"} {
		t.Run(command, func(t *testing.T) {
			f := newEnvelopePipelineFixture(t)
			id := newTestUUID()
			record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(id, command, `{}`))
			if err != nil || record.Status != commandledger.Succeeded {
				t.Fatalf("execução direta: status=%s err=%v", record.Status, err)
			}
			if f.startCalls.Load() != 1 {
				t.Fatalf("Start chamado %d vezes", f.startCalls.Load())
			}
		})
	}
}

func TestEnvelopePipelineTriggerResolveESuppress(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	f.resolve = func(EnvelopeCandidate) (EnvelopeResolution, error) {
		f.resolveCalls.Add(1)
		return EnvelopeResolution{Mode: commandcontract.ResolutionSuppress, BindingIDs: []string{"binding.suppressed"}}, nil
	}
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.trigger(newTestUUID()))
	if err != nil || record.Status != commandledger.Suppressed {
		t.Fatalf("suppress: status=%s err=%v", record.Status, err)
	}
	if f.resolveCalls.Load() != 2 || f.startCalls.Load() != 0 {
		t.Fatalf("resolver/start incorretos: resolve=%d start=%d", f.resolveCalls.Load(), f.startCalls.Load())
	}
}

func TestEnvelopePipelineDuplicateDifferentArgsAndCatalogChange(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	id := newTestUUID()
	first := f.candidate(id, "pipe.read", `{}`)
	got, err := f.service.ExecuteEnvelope(context.Background(), f.token, first)
	if err != nil || got.Status != commandledger.Succeeded {
		t.Fatalf("primeira execução: %s %v", got.Status, err)
	}
	f.service.config.RegistryVersion = "registry-v2"
	f.service.config.Registry = nil
	second, err := f.service.ExecuteEnvelope(context.Background(), f.token, first)
	if err != nil || second.Status != commandledger.Succeeded || f.startCalls.Load() != 1 {
		t.Fatalf("reentrega não independente do catálogo: %s %v starts=%d", second.Status, err, f.startCalls.Load())
	}
	different := first
	different.Arguments = json.RawMessage(`{"different":true}`)
	if _, err := f.service.ExecuteEnvelope(context.Background(), f.token, different); !errors.Is(err, commandledger.ErrConflict) {
		t.Fatalf("payload conflitante aceito: %v", err)
	}
	f.service.config.Registry = f.registry
	read, err := f.service.GetEnvelopeInvocation(context.Background(), f.token, id)
	if err != nil || read.Status != commandledger.Succeeded {
		t.Fatalf("Get independente da versão: %s %v", read.Status, err)
	}
}

func TestEnvelopePipelineQueuedConfigMutationNaoChamaStart(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	queued := make(chan struct{})
	release := make(chan struct{})
	f.awaitQueue = func(ctx context.Context, _ commandcontract.Envelope) error {
		close(queued)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	result := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	go func() {
		record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.read", `{}`))
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record, err}
	}()
	<-queued
	if err := f.epochs.MutateSession(context.Background(), f.user, f.session, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	close(release)
	got := <-result
	if got.err != nil || got.record.Status != commandledger.CancelledStale || f.startCalls.Load() != 0 {
		t.Fatalf("mutação enfileirada iniciou/retornou errado: status=%s err=%v starts=%d", got.record.Status, got.err, f.startCalls.Load())
	}
}

func TestEnvelopePipelineProviderTTLAndPanicFailClosed(t *testing.T) {
	t.Run("ttl", func(t *testing.T) {
		f := newEnvelopePipelineFixture(t)
		f.provider.mode.Store(1)
		record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.write", `{}`))
		if err != nil || record.Status != commandledger.CancelledStale || f.startCalls.Load() != 0 {
			t.Fatalf("TTL não falhou fechado: status=%s err=%v starts=%d", record.Status, err, f.startCalls.Load())
		}
	})
	t.Run("panic", func(t *testing.T) {
		f := newEnvelopePipelineFixture(t)
		f.provider.panic.Store(true)
		record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.write", `{}`))
		if err != nil || record.Status != commandledger.Denied {
			t.Fatalf("panic do provider não falhou fechado: status=%s err=%v", record.Status, err)
		}
		if f.startCalls.Load() != 0 {
			t.Fatal("provider em panic alcançou Start")
		}
	})
}

func TestEnvelopePipelineStartPanicPersisteOutcomeUnknown(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	f.start = func(context.Context, Invocation) (ExecutionHandle, error) { panic("handler panic") }
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.read", `{}`))
	if !errors.Is(err, ErrExecution) || record.Status != commandledger.OutcomeUnknown {
		t.Fatalf("panic do Start não virou outcome_unknown: status=%s err=%v", record.Status, err)
	}
}

func TestEnvelopePipelineMissingReceiptAndHeadlessAreDenied(t *testing.T) {
	t.Run("receipt ausente", func(t *testing.T) {
		f := newEnvelopePipelineFixture(t)
		record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.destroy", `{}`))
		if err != nil || record.Status != commandledger.Denied || f.startCalls.Load() != 0 {
			t.Fatalf("receipt ausente não negou: status=%s err=%v starts=%d", record.Status, err, f.startCalls.Load())
		}
	})
	t.Run("headless", func(t *testing.T) {
		f := newEnvelopePipelineFixture(t)
		f.service.config.Source = commandcatalog.CLI
		record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.destroy", `{}`))
		if err != nil || record.Status != commandledger.Denied || f.startCalls.Load() != 0 {
			t.Fatalf("headless destrutivo não negou: status=%s err=%v starts=%d", record.Status, err, f.startCalls.Load())
		}
	})
}

func TestEnvelopePipelineReceiptSuccessUsesInvocationSubject(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	presenter := &pipelineDecisionPresenter{action: commanddecision.ApplyAction}
	decisions, err := commanddecision.New(f.db, presenter, func() time.Time { return f.now })
	if err != nil {
		t.Fatal(err)
	}
	f.service.config.Envelope.Decisions = decisions
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.destroy", `{}`))
	if err != nil || record.Status != commandledger.Succeeded || presenter.calls.Load() != 1 {
		t.Fatalf("receipt não autorizou execução: status=%s err=%v presentations=%d", record.Status, err, presenter.calls.Load())
	}
	var subject string
	if err := f.db.Table("command_decision_receipts").Select("subject_type").Where("subject_id = ?", record.InvocationID).Scan(&subject).Error; err != nil {
		t.Fatal(err)
	}
	if subject != "invocation" {
		t.Fatalf("subject_type inesperado: %q", subject)
	}
}

func TestEnvelopePipelineCancellationCancelsHandleAndPersistsUnknown(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	started := make(chan struct{})
	var cancelled atomic.Int32
	f.start = func(_ context.Context, _ Invocation) (ExecutionHandle, error) {
		close(started)
		done := make(chan Outcome)
		return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() { cancelled.Add(1) }}, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	go func() {
		record, err := f.service.ExecuteEnvelope(ctx, f.token, f.candidate(newTestUUID(), "pipe.read", `{}`))
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record, err}
	}()
	<-started
	cancel()
	got := <-result
	if got.err != nil || got.record.Status != commandledger.OutcomeUnknown || cancelled.Load() != 1 {
		t.Fatalf("cancelamento não foi concluído como unknown: status=%s err=%v cancels=%d", got.record.Status, got.err, cancelled.Load())
	}
}

func TestEnvelopePipelineInputFingerprintDoesNotExposeArguments(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	secret := "do-not-persist"
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.read", `{"secret":"do-not-persist"}`))
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("argumento válido não executou: status=%s err=%v", record.Status, err)
	}
	if record.Envelope.Arguments == nil || bytes.Contains(*record.Envelope.Arguments, []byte(secret)) || bytes.Contains([]byte(record.ArgumentsSummary), []byte(secret)) || bytes.Contains([]byte(record.InputFingerprint), []byte(secret)) {
		t.Fatal("argumento bruto apareceu na projeção persistida ou no fingerprint")
	}
	var summary string
	if err := f.db.Table("command_invocations").Select("arguments_summary").Where("invocation_id = ?", record.InvocationID).Scan(&summary).Error; err != nil {
		t.Fatal(err)
	}
	if bytes.Contains([]byte(summary), []byte(secret)) {
		t.Fatal("argumento bruto apareceu no banco")
	}
}

func TestEnvelopePipelineResultUsesExactEmptyJSON(t *testing.T) {
	f := newEnvelopePipelineFixture(t)
	var seen atomic.Int32
	f.start = func(_ context.Context, _ Invocation) (ExecutionHandle, error) {
		seen.Add(1)
		done := make(chan Outcome, 1)
		done <- Outcome{Status: commandledger.Succeeded, Result: json.RawMessage(`{}`)}
		return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() {}}, nil
	}
	record, err := f.service.ExecuteEnvelope(context.Background(), f.token, f.candidate(newTestUUID(), "pipe.read", `{}`))
	if err != nil || record.Status != commandledger.Succeeded || seen.Load() != 1 {
		t.Fatalf("resultado vazio válido não persistiu: status=%s err=%v", record.Status, err)
	}
}
