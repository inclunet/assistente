package commandexecution

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontext"
	"assistente/internal/commandcontract"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
)

func TestCompleteGenericIdentityExternalJobAndSystem(t *testing.T) {
	for _, tc := range []struct {
		name      string
		auth      commandcontract.AuthContextType
		actor     commandcontract.ActorType
		source    commandcatalog.Source
		authID    string
		contextID string
	}{
		{name: "external", auth: commandcontract.AuthExternalToken, actor: commandcontract.ActorUser, source: commandcatalog.Palette, authID: "external:issuer:subject", contextID: "issuer:subject"},
		{name: "job", auth: commandcontract.AuthJobService, actor: commandcontract.ActorAutomation, source: commandcatalog.Palette, authID: "job-service:job-1:run-1", contextID: "job-1"},
		{name: "system", auth: commandcontract.AuthSystem, actor: commandcontract.ActorAutomation, source: commandcatalog.System, authID: "system:fixed", contextID: "system"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGenericEnvelopeFixture(t, tc.source, tc.auth, tc.actor, tc.authID, tc.contextID)
			got, err := f.service.ExecuteEnvelope(context.Background(), "opaque-token", EnvelopeCandidate{InvocationID: genericUUID(), CorrelationID: genericUUID(), CommandID: "generic.read", Arguments: json.RawMessage(`{}`)})
			if err != nil || got.Status != commandledger.Succeeded {
				t.Fatalf("execução genérica: status=%s err=%v", got.Status, err)
			}
			if got.Ownership.AuthContextType != tc.auth || got.Ownership.AuthContextID != tc.authID || got.Ownership.ActorType != tc.actor {
				t.Fatalf("ownership incorreta: %+v", got.Ownership)
			}
			if got.Envelope.SessionID != nil {
				t.Fatalf("wire session inesperada: %+v", got.Envelope.SessionID)
			}
			if f.starts.Load() != 1 || f.authorizes.Load() < 2 {
				t.Fatalf("portas não usadas: start=%d authorize=%d", f.starts.Load(), f.authorizes.Load())
			}
		})
	}
}

func TestCompleteGenericIdentityRejectsInvalidAndPhysicalExternal(t *testing.T) {
	t.Run("owner inválido", func(t *testing.T) {
		f := newGenericEnvelopeFixture(t, commandcatalog.Palette, commandcontract.AuthExternalToken, commandcontract.ActorUser, "external:bad", "issuer:subject")
		f.service.config.Envelope.Identity.Authenticate = func(context.Context, string) (EnvelopeAuthenticatedIdentity, error) {
			return EnvelopeAuthenticatedIdentity{Ownership: commandledger.FullOwnership{UserID: stringPtr("not-a-user"), AuthContextType: commandcontract.AuthExternalToken, AuthContextID: "external:bad", ActorType: commandcontract.ActorUser, ActorID: "not-a-user"}, ContextPrincipal: commandsecurity.ContextPrincipal{UserID: "not-a-user", Type: string(commandcontract.AuthExternalToken), ID: "issuer:subject"}}, nil
		}
		_, err := f.service.ExecuteEnvelope(context.Background(), "opaque-token", EnvelopeCandidate{InvocationID: genericUUID(), CorrelationID: genericUUID(), CommandID: "generic.read", Arguments: json.RawMessage(`{}`)})
		if !errors.Is(err, ErrDenied) || f.starts.Load() != 0 {
			t.Fatalf("owner inválido aceito: err=%v starts=%d", err, f.starts.Load())
		}
	})

	t.Run("external físico", func(t *testing.T) {
		f := newGenericEnvelopeFixture(t, commandcatalog.KeyboardLocal, commandcontract.AuthExternalToken, commandcontract.ActorUser, "external:physical", "issuer:subject")
		_, err := f.service.ExecuteEnvelope(context.Background(), "opaque-token", EnvelopeCandidate{InvocationID: genericUUID(), CorrelationID: genericUUID(), CommandID: "generic.read", Arguments: json.RawMessage(`{}`)})
		if !errors.Is(err, ErrDenied) || f.starts.Load() != 0 {
			t.Fatalf("external físico aceito: err=%v starts=%d", err, f.starts.Load())
		}
	})
}

func TestNewCompleteCopiesGenericPortsAtPublication(t *testing.T) {
	f := newGenericEnvelopeFixture(t, commandcatalog.Palette, commandcontract.AuthExternalToken, commandcontract.ActorUser, "external:copy", "issuer:copy")
	f.identity.Authorize = func(context.Context, commandledger.FullOwnership, commandcontract.Envelope, commandcatalog.Definition) error {
		return ErrDenied
	}
	got, err := f.service.ExecuteEnvelope(context.Background(), "opaque-token", EnvelopeCandidate{InvocationID: genericUUID(), CorrelationID: genericUUID(), CommandID: "generic.read", Arguments: json.RawMessage(`{}`)})
	if err != nil || got.Status != commandledger.Succeeded {
		t.Fatalf("mutação do grupo original afetou serviço publicado: status=%s err=%v", got.Status, err)
	}
}

func TestGenericIdentityCallbacksCannotMutateOwnershipOrLookupRecord(t *testing.T) {
	f := newGenericEnvelopeFixture(t, commandcatalog.Palette, commandcontract.AuthExternalToken, commandcontract.ActorUser, "external:alias", "issuer:alias")
	f.service.config.Envelope.Identity.Snapshot = func(_ context.Context, owner commandledger.FullOwnership, _ EnvelopeCandidate) (commandcontract.Envelope, error) {
		*owner.UserID = "mutated-by-snapshot"
		return commandcontract.Envelope{RegistryVersion: "registry-v1", GlobalConfigGeneration: stringPtr("global-v1"), ActiveLayersGeneration: stringPtr("layers-v1")}, nil
	}
	f.service.config.Envelope.Identity.Resolve = func(_ context.Context, owner commandledger.FullOwnership, _ EnvelopeCandidate, _ commandcontract.Envelope) (EnvelopeResolution, error) {
		*owner.UserID = "mutated-by-resolve"
		return EnvelopeResolution{Mode: commandcontract.ResolutionSuppress, BindingIDs: []string{"binding.suppressed"}}, nil
	}
	f.service.config.Envelope.Identity.Authorize = func(_ context.Context, owner commandledger.FullOwnership, _ commandcontract.Envelope, _ commandcatalog.Definition) error {
		*owner.UserID = "mutated-by-authorize"
		return nil
	}
	candidate := EnvelopeCandidate{InvocationID: genericUUID(), CorrelationID: genericUUID(), CommandID: "generic.read", Arguments: json.RawMessage(`{}`)}
	record, err := f.service.ExecuteEnvelope(context.Background(), "opaque-token", candidate)
	if err != nil || record.Status != commandledger.Succeeded || record.Ownership.UserID == nil || *record.Ownership.UserID != f.contextPrincipal.UserID {
		t.Fatalf("ownership escapou dos callbacks: status=%s owner=%v err=%v", record.Status, record.Ownership.UserID, err)
	}
	identity, _, resolveErr := f.service.authenticateEnvelope(context.Background(), "opaque-token")
	if resolveErr != nil {
		t.Fatal(resolveErr)
	}
	if _, resolveErr := f.service.resolveEnvelope(context.Background(), identity, candidate, record.Envelope); resolveErr != nil || identity.Ownership.UserID == nil || *identity.Ownership.UserID != f.contextPrincipal.UserID {
		t.Fatalf("ownership escapou do resolver: owner=%v err=%v", identity.Ownership.UserID, resolveErr)
	}

	f.service.config.Envelope.Identity.AuthorizeLookup = func(_ context.Context, owner commandledger.FullOwnership, lookup commandledger.FullRecord) error {
		*owner.UserID = "mutated-by-lookup-owner"
		if lookup.Ownership.UserID != nil {
			*lookup.Ownership.UserID = "mutated-by-lookup-record"
		}
		if lookup.Envelope.UserID != nil {
			*lookup.Envelope.UserID = "mutated-by-lookup-envelope"
		}
		if lookup.Envelope.Arguments != nil {
			*lookup.Envelope.Arguments = json.RawMessage(`{"leak":true}`)
		}
		if lookup.ResultSummary != nil {
			*lookup.ResultSummary = "mutated-by-lookup-result"
		}
		return nil
	}
	got, err := f.service.GetEnvelopeInvocation(context.Background(), "opaque-token", candidate.InvocationID)
	if err != nil || got.Ownership.UserID == nil || *got.Ownership.UserID != f.contextPrincipal.UserID || got.Envelope.UserID == nil || *got.Envelope.UserID != f.contextPrincipal.UserID || got.Envelope.Arguments == nil || string(*got.Envelope.Arguments) == `{"leak":true}` {
		t.Fatalf("registro escapou mutação do lookup: owner=%v envelope=%v args=%v err=%v", got.Ownership.UserID, got.Envelope.UserID, got.Envelope.Arguments, err)
	}
	marker := EnvelopeCandidate{InvocationID: genericUUID(), CorrelationID: genericUUID(), TriggerType: string(commandcontract.SourcePalette), TriggerSpec: json.RawMessage(`{"version":1}`), Arguments: json.RawMessage(`{}`)}
	marked, err := f.service.ExecuteEnvelope(context.Background(), "opaque-token", marker)
	if err != nil || marked.Status != commandledger.Suppressed {
		t.Fatalf("marcador suppressed: status=%s err=%v", marked.Status, err)
	}
	markerRecord, err := f.service.GetEnvelopeInvocation(context.Background(), "opaque-token", marker.InvocationID)
	if err != nil || markerRecord.Status != commandledger.Suppressed {
		t.Fatalf("lookup do marcador suppressed: status=%s err=%v", markerRecord.Status, err)
	}
}

func TestCompleteGenericIdentityRevocationCancelsQueuedRun(t *testing.T) {
	f := newGenericEnvelopeFixture(t, commandcatalog.Palette, commandcontract.AuthExternalToken, commandcontract.ActorUser, "external:revoke", "issuer:revoke")
	queued := make(chan struct{})
	release := make(chan struct{})
	f.service.config.Envelope.AwaitQueue = func(ctx context.Context, _ commandcontract.Envelope) error {
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
		record, err := f.service.ExecuteEnvelope(context.Background(), "opaque-token", EnvelopeCandidate{InvocationID: genericUUID(), CorrelationID: genericUUID(), CommandID: "generic.read", Arguments: json.RawMessage(`{}`)})
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record, err}
	}()
	<-queued
	if err := f.service.config.Epochs.MutateContext(context.Background(), f.contextPrincipal, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	close(release)
	got := <-result
	if got.err != nil || got.record.Status != commandledger.CancelledStale || f.starts.Load() != 0 {
		t.Fatalf("revogação genérica não cancelou: status=%s err=%v starts=%d", got.record.Status, got.err, f.starts.Load())
	}
}

func TestCompleteGenericChainUsesLayerRefsAndRechecksExactList(t *testing.T) {
	t.Run("append preservado", func(t *testing.T) {
		f := newGenericEnvelopeFixture(t, commandcatalog.Palette, commandcontract.AuthExternalToken, commandcontract.ActorUser, "external:chain", "issuer:chain")
		provenance := json.RawMessage(`{"version":1,"_chain_id":"chain-1","command_chain_history":[]}`)
		f.service.config.Envelope.Identity.Snapshot = func(context.Context, commandledger.FullOwnership, EnvelopeCandidate) (commandcontract.Envelope, error) {
			return commandcontract.Envelope{RegistryVersion: "registry-v1", GlobalConfigGeneration: stringPtr("global-v1"), ActiveLayersGeneration: stringPtr("layers-v1"), Provenance: &provenance}, nil
		}
		f.service.config.Envelope.Identity.Resolve = func(context.Context, commandledger.FullOwnership, EnvelopeCandidate, commandcontract.Envelope) (EnvelopeResolution, error) {
			return EnvelopeResolution{Mode: commandcontract.ResolutionExecute, CommandID: "generic.read", Arguments: json.RawMessage(`{}`), BindingIDs: []string{"same-binding"}, LayerRefs: []string{"layer.a"}}, nil
		}
		var observed *json.RawMessage
		handler := f.service.config.Handlers["generic.read"]
		handler.Start = func(_ context.Context, invocation Invocation) (ExecutionHandle, error) {
			observed = cloneRawMessage(invocation.Envelope.Provenance)
			return pipelineCompletedHandle(commandledger.Succeeded), nil
		}
		f.service.config.Handlers["generic.read"] = handler
		got, err := f.service.ExecuteEnvelope(context.Background(), "opaque-token", EnvelopeCandidate{InvocationID: genericUUID(), CorrelationID: genericUUID(), TriggerType: string(commandcontract.SourcePalette), TriggerSpec: json.RawMessage(`{"version":1}`), Arguments: json.RawMessage(`{}`)})
		if err != nil || got.Status != commandledger.Succeeded {
			t.Fatalf("cadeia: status=%s err=%v", got.Status, err)
		}
		if observed == nil || !bytes.Contains(*observed, []byte(`"layer_refs":["layer.a"]`)) {
			provenanceText := "<nil>"
			if observed != nil {
				provenanceText = string(*observed)
			}
			t.Fatalf("layer_refs não anexado à provenance: %s", provenanceText)
		}
	})

	t.Run("recheck não usa binding ids", func(t *testing.T) {
		f := newGenericEnvelopeFixture(t, commandcatalog.Palette, commandcontract.AuthExternalToken, commandcontract.ActorUser, "external:chain-recheck", "issuer:chain-recheck")
		provenance := json.RawMessage(`{"version":1,"_chain_id":"chain-2","command_chain_history":[]}`)
		f.service.config.Envelope.Identity.Snapshot = func(context.Context, commandledger.FullOwnership, EnvelopeCandidate) (commandcontract.Envelope, error) {
			return commandcontract.Envelope{RegistryVersion: "registry-v1", GlobalConfigGeneration: stringPtr("global-v1"), ActiveLayersGeneration: stringPtr("layers-v1"), Provenance: &provenance}, nil
		}
		var calls atomic.Int32
		f.service.config.Envelope.Identity.Resolve = func(context.Context, commandledger.FullOwnership, EnvelopeCandidate, commandcontract.Envelope) (EnvelopeResolution, error) {
			refs := []string{"layer.a"}
			if calls.Add(1) > 1 {
				refs = []string{"layer.b"}
			}
			return EnvelopeResolution{Mode: commandcontract.ResolutionExecute, CommandID: "generic.read", Arguments: json.RawMessage(`{}`), BindingIDs: []string{"same-binding"}, LayerRefs: refs}, nil
		}
		got, err := f.service.ExecuteEnvelope(context.Background(), "opaque-token", EnvelopeCandidate{InvocationID: genericUUID(), CorrelationID: genericUUID(), TriggerType: string(commandcontract.SourcePalette), TriggerSpec: json.RawMessage(`{"version":1}`), Arguments: json.RawMessage(`{}`)})
		if err != nil || got.Status != commandledger.CancelledStale || f.starts.Load() != 0 {
			t.Fatalf("layer refs divergentes aceitas: status=%s err=%v starts=%d", got.Status, err, f.starts.Load())
		}
	})
}

type genericEnvelopeFixture struct {
	service          *Service
	identity         *EnvelopeIdentityPorts
	contextPrincipal commandsecurity.ContextPrincipal
	starts           atomic.Int32
	authorizes       atomic.Int32
}

func newGenericEnvelopeFixture(t *testing.T, source commandcatalog.Source, authType commandcontract.AuthContextType, actor commandcontract.ActorType, authID, contextID string) *genericEnvelopeFixture {
	t.Helper()
	base := newExecutionFixture(t)
	registry, err := commandcatalog.NewComplete([]commandcatalog.Registration{{
		Definition: commandcatalog.Definition{
			ID: "generic.read", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision, AllowedSources: []commandcatalog.Source{source},
			Context: commandcatalog.ContextPolicy{None: true}, Presentation: &commandcatalog.Presentation{Version: "test-v1", Locales: map[string]commandcatalog.LocalizedMetadata{"pt-BR": {Name: "generic.read", Description: "generic.read", Category: "test"}, "en": {Name: "generic.read", Description: "generic.read", Category: "test"}, "es": {Name: "generic.read", Description: "generic.read", Category: "test"}}},
			ArgumentsSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, ResultSchema: &commandcatalog.Schema{Type: commandcatalog.SchemaObject}, Risk: commandcatalog.RiskLow,
			Persistence: commandcatalog.PersistencePolicy{Arguments: commandcatalog.PersistenceRedacted, Result: commandcatalog.PersistenceSummary, Audit: commandcatalog.PersistenceRedacted}, Scopes: []commandcatalog.Scope{commandcatalog.ScopeSession}, Availability: commandcatalog.Availability{Status: commandcatalog.Available}, HandlerRoute: "internal/generic/read", HandlerClassification: commandcatalog.HandlerInternal,
		},
		Handler: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "internal/generic/read", Classification: commandcatalog.HandlerInternal},
	}})
	if err != nil {
		t.Fatal(err)
	}
	keys := pipelineKeyProvider(t)
	bus, err := commandcontextForGeneric()
	if err != nil {
		t.Fatal(err)
	}
	f := &genericEnvelopeFixture{}
	user := base.user.ID
	jobID := genericUUID()
	contextUser := user
	if authType == commandcontract.AuthSystem {
		contextUser = ""
	}
	f.contextPrincipal = commandsecurity.ContextPrincipal{UserID: contextUser, Type: string(authType), ID: contextID}
	f.identity = &EnvelopeIdentityPorts{}
	f.identity.Authenticate = func(context.Context, string) (EnvelopeAuthenticatedIdentity, error) {
		var userID *string
		callbackContextUser := user
		if authType != commandcontract.AuthSystem {
			userID = &user
		} else {
			callbackContextUser = ""
		}
		return EnvelopeAuthenticatedIdentity{Ownership: commandledger.FullOwnership{UserID: userID, AuthContextType: authType, AuthContextID: authID, ActorType: actor, ActorID: func() string {
			if actor == commandcontract.ActorUser {
				return user
			}
			return "generic-actor"
		}()}, ContextPrincipal: commandsecurity.ContextPrincipal{UserID: callbackContextUser, Type: string(authType), ID: contextID}}, nil
	}
	f.identity.Snapshot = func(context.Context, commandledger.FullOwnership, EnvelopeCandidate) (commandcontract.Envelope, error) {
		e := commandcontract.Envelope{RegistryVersion: "registry-v1"}
		if authType != commandcontract.AuthSystem {
			e.GlobalConfigGeneration = stringPtr("global-v1")
			e.ActiveLayersGeneration = stringPtr("layers-v1")
		}
		if authType == commandcontract.AuthJobService {
			jobSlug, jobFingerprint, runID := "generic-job", "job-definition-v1", "run-1"
			e.JobID, e.JobSlug, e.JobDefinitionFingerprint, e.RunID = &jobID, &jobSlug, &jobFingerprint, &runID
		}
		return e, nil
	}
	f.identity.Resolve = func(context.Context, commandledger.FullOwnership, EnvelopeCandidate, commandcontract.Envelope) (EnvelopeResolution, error) {
		return EnvelopeResolution{}, errors.New("não deveria resolver comando direto")
	}
	f.identity.Authorize = func(context.Context, commandledger.FullOwnership, commandcontract.Envelope, commandcatalog.Definition) error {
		f.authorizes.Add(1)
		return nil
	}
	f.identity.AuthorizeLookup = func(context.Context, commandledger.FullOwnership, commandledger.FullRecord) error {
		f.authorizes.Add(1)
		return nil
	}
	config := Config{Envelope: &EnvelopeConfig{Identity: f.identity, Context: bus}, Epochs: base.epochs, Store: base.store, Registry: registry, RegistryVersion: "registry-v1", Handlers: map[string]Handler{"generic.read": {Contract: commandcatalog.HandlerContract{Effect: commandcatalog.Read, Route: "internal/generic/read", Classification: commandcatalog.HandlerInternal}, Start: func(context.Context, Invocation) (ExecutionHandle, error) {
		f.starts.Add(1)
		return pipelineCompletedHandle(commandledger.Succeeded), nil
	}}}, Source: source, Keys: keys, KeyVersion: "v1", Now: func() time.Time { return base.now }, Retention: time.Hour, ExecutionTimeout: 2 * time.Second, FinalizationTimeout: 2 * time.Second}
	f.service, err = NewComplete(config)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func commandcontextForGeneric() (*commandcontext.FactBus, error) {
	return commandcontext.NewFactBus(nil)
}

func genericUUID() string { return newTestUUID() }

func stringPtr(value string) *string { return &value }
