package commandexecution

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandidentity"
	"assistente/internal/commandledger"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"
)

const (
	externalTestIssuer     = "https://commands.example.test"
	externalTestAdminScope = "assistente:identity:admin"
	externalTestScope      = "commands:read"
	externalTestRole       = "command-runner"
)

type externalClaimsVerifier struct {
	mu         sync.Mutex
	claims     map[string]*auth.ExternalClaims
	validates  atomic.Int32
	cached     atomic.Int32
	blockToken string
	entered    chan struct{}
	cancelled  chan struct{}
	release    chan struct{}
}

func (v *externalClaimsVerifier) Validate(ctx context.Context, token string) (*auth.ExternalClaims, error) {
	v.validates.Add(1)
	v.mu.Lock()
	block := token == v.blockToken
	entered, release := v.entered, v.release
	v.mu.Unlock()
	if block {
		select {
		case entered <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-ctx.Done():
			v.mu.Lock()
			cancelled := v.cancelled
			v.mu.Unlock()
			if cancelled != nil {
				select {
				case cancelled <- struct{}{}:
				default:
				}
			}
			return nil, ctx.Err()
		}
	}
	return v.claim(token)
}

func (v *externalClaimsVerifier) ValidateCached(_ context.Context, token string) (*auth.ExternalClaims, error) {
	v.cached.Add(1)
	return v.claim(token)
}

func (v *externalClaimsVerifier) claim(token string) (*auth.ExternalClaims, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	claims := v.claims[token]
	if claims == nil {
		return nil, errors.New("token de teste desconhecido")
	}
	copyClaims := *claims
	copyClaims.Roles = append([]string(nil), claims.Roles...)
	return &copyClaims, nil
}

func (v *externalClaimsVerifier) block(token string) (chan struct{}, chan struct{}) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.blockToken = token
	v.entered = make(chan struct{}, 1)
	v.cancelled = make(chan struct{}, 1)
	v.release = make(chan struct{})
	return v.entered, v.release
}

type externalHarness struct {
	fixture       *genericEnvelopeFixture
	base          *executionFixture
	verifier      *externalClaimsVerifier
	repository    *auth.ExternalIdentityRepository
	authenticator *auth.ExternalCommandAuthenticator
	admin         *auth.ExternalIdentityAdminService
	service       *ExternalService
	users         map[string]string
}

func newExternalHarness(t *testing.T, mutateConfig func(*Config)) *externalHarness {
	t.Helper()
	f := newGenericEnvelopeFixture(t, commandcatalog.Palette, commandcontract.AuthExternalToken, commandcontract.ActorUser, "unused-auth-id", "unused-context-id")
	base := newExecutionFixture(t)
	verifier := &externalClaimsVerifier{claims: map[string]*auth.ExternalClaims{
		"token-a":          {Issuer: externalTestIssuer, Subject: "subject-a", Scope: externalTestScope, Roles: []string{externalTestRole}},
		"token-b":          {Issuer: externalTestIssuer, Subject: "subject-a", Scope: externalTestScope, Roles: []string{externalTestRole}},
		"token-other-user": {Issuer: externalTestIssuer, Subject: "subject-other", Scope: externalTestScope, Roles: []string{externalTestRole}},
		"token-no-scope":   {Issuer: externalTestIssuer, Subject: "subject-no-scope", Roles: []string{externalTestRole}},
		"token-no-role":    {Issuer: externalTestIssuer, Subject: "subject-no-role", Scope: externalTestScope},
		"admin-token":      {Issuer: externalTestIssuer, Subject: "administrator", Scope: externalTestAdminScope},
	}}
	if err := base.db.AutoMigrate(&auth.ExternalIdentityMapping{}); err != nil {
		t.Fatalf("migrar mappings externos de teste: %v", err)
	}
	repository := auth.NewExternalIdentityRepository(base.db)
	otherUser, err := auth.NewIdentityService(base.db).CreateLocalUser(context.Background(), auth.CreateUserParams{Username: "external-other-user", Password: "unused-password"})
	if err != nil {
		t.Fatalf("criar usuário local de teste: %v", err)
	}
	users := map[string]string{
		"subject-a":        base.user.ID,
		"subject-other":    otherUser.ID,
		"subject-no-scope": base.user.ID,
		"subject-no-role":  base.user.ID,
	}
	for subject, userID := range users {
		if _, err := repository.Create(context.Background(), auth.ExternalIdentityMappingParams{Issuer: externalTestIssuer, Subject: subject, UserID: userID}); err != nil {
			t.Fatalf("criar mapping %q: %v", subject, err)
		}
	}
	authenticator := auth.NewExternalCommandAuthenticator(verifier, repository)
	authenticator.SetReadiness(repository.CheckReadiness)
	admin, err := auth.NewExternalIdentityAdminService(verifier, repository, auth.ExternalIdentityAdminConfig{AdminScopes: []string{externalTestAdminScope}})
	if err != nil {
		t.Fatalf("criar serviço admin externo: %v", err)
	}
	config := f.service.config
	config.Store = base.store
	config.Epochs = base.epochs
	if mutateConfig != nil {
		mutateConfig(&config)
	}
	service, err := NewExternal(config, authenticator, admin, []commandidentity.AuthorizationRule{{
		CommandID: "generic.read", Actors: []commandcontract.ActorType{commandcontract.ActorUser},
		RequiredRoles: []string{externalTestRole}, RequiredScopes: []string{externalTestScope},
	}})
	if err != nil {
		t.Fatalf("criar ExternalService: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := service.Shutdown(ctx); err != nil {
			t.Errorf("encerrar ExternalService: %v", err)
		}
		if err := f.service.Shutdown(ctx); err != nil {
			t.Errorf("encerrar fixture de execução: %v", err)
		}
		if err := base.service.Shutdown(ctx); err != nil {
			t.Errorf("encerrar fixture SQLite: %v", err)
		}
	})
	return &externalHarness{fixture: f, base: base, verifier: verifier, repository: repository, authenticator: authenticator, admin: admin, service: service, users: users}
}

func externalCandidate() EnvelopeCandidate {
	return EnvelopeCandidate{InvocationID: genericUUID(), CorrelationID: genericUUID(), CommandID: "generic.read", Arguments: []byte(`{}`)}
}

func TestExternalServiceExecutesWithRealMappedIdentity(t *testing.T) {
	h := newExternalHarness(t, nil)
	candidate := externalCandidate()
	record, output, err := h.service.ExecuteEnvelopeWithResult(context.Background(), "token-a", candidate)
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("execução externa: status=%s err=%v", record.Status, err)
	}
	if len(output) == 0 || record.Ownership.UserID == nil || *record.Ownership.UserID != h.users["subject-a"] || record.Ownership.AuthContextType != commandcontract.AuthExternalToken {
		t.Fatalf("identidade/resultado não derivados do mapping: ownership=%+v output=%s", record.Ownership, output)
	}
	if record.Envelope.SessionID != nil || h.fixture.starts.Load() != 1 {
		t.Fatalf("sessão sintética ou execução incorreta: session=%v starts=%d", record.Envelope.SessionID, h.fixture.starts.Load())
	}
}

func TestExternalServiceReplacesTransportUserContextWithMappedOwner(t *testing.T) {
	observed := make(chan string, 1)
	h := newExternalHarness(t, func(config *Config) {
		handler := config.Handlers["generic.read"]
		handler.Start = func(ctx context.Context, _ Invocation) (ExecutionHandle, error) {
			userID, ok := database.UserIDFromContext(ctx)
			if !ok {
				observed <- "<missing>"
			} else {
				observed <- userID
			}
			return pipelineCompletedHandle(commandledger.Succeeded), nil
		}
		config.Handlers["generic.read"] = handler
	})
	transport := database.WithUserID(context.Background(), "forged-transport-owner")
	record, err := h.service.ExecuteEnvelope(transport, "token-a", externalCandidate())
	if err != nil || record.Status != commandledger.Succeeded {
		t.Fatalf("execução com owner de transporte forjado: status=%s err=%v", record.Status, err)
	}
	select {
	case got := <-observed:
		if got != h.users["subject-a"] {
			t.Fatalf("handler recebeu owner %q, want usuário mapeado %q", got, h.users["subject-a"])
		}
	case <-time.After(time.Second):
		t.Fatal("handler não observou contexto de usuário")
	}
}

func TestExternalServiceIsolatesTokenReplayAndLookupOwnership(t *testing.T) {
	h := newExternalHarness(t, nil)
	candidate := externalCandidate()
	first, err := h.service.ExecuteEnvelope(context.Background(), "token-a", candidate)
	if err != nil || first.Status != commandledger.Succeeded {
		t.Fatalf("execução inicial: status=%s err=%v", first.Status, err)
	}
	foreign, err := h.service.GetEnvelopeInvocation(context.Background(), "token-b", candidate.InvocationID)
	if !errors.Is(err, commandledger.ErrNotFound) || foreign.Status != "" || foreign.Ownership.UserID != nil {
		t.Fatalf("lookup fora do escopo deveria ser indistinguível de ausência: record=%+v err=%v", foreign, err)
	}
	if _, err := h.service.ExecuteEnvelope(context.Background(), "token-b", candidate); !errors.Is(err, commandledger.ErrConflict) {
		t.Fatalf("mesma invocation_id em outro token deveria conflitar sem revelar o registro: %v", err)
	}
	if h.fixture.starts.Load() != 1 {
		t.Fatalf("conflito cross-token iniciou handler: starts=%d", h.fixture.starts.Load())
	}
	secondCandidate := externalCandidate()
	second, err := h.service.ExecuteEnvelope(context.Background(), "token-b", secondCandidate)
	if err != nil || second.Status != commandledger.Succeeded || first.Ownership.AuthContextID == second.Ownership.AuthContextID || h.fixture.starts.Load() != 2 {
		t.Fatalf("token distinto não teve escopo isolado: status=%s ctx1=%q ctx2=%q starts=%d err=%v", second.Status, first.Ownership.AuthContextID, second.Ownership.AuthContextID, h.fixture.starts.Load(), err)
	}
	replayOtherToken, err := h.service.ExecuteEnvelope(context.Background(), "token-b", secondCandidate)
	if err != nil || replayOtherToken.Status != commandledger.Succeeded || h.fixture.starts.Load() != 2 {
		t.Fatalf("replay do segundo token executou novamente: status=%s err=%v starts=%d", replayOtherToken.Status, err, h.fixture.starts.Load())
	}
	replay, err := h.service.ExecuteEnvelope(context.Background(), "token-a", candidate)
	if err != nil || replay.Status != commandledger.Succeeded || h.fixture.starts.Load() != 2 {
		t.Fatalf("replay do mesmo token repetiu handler: status=%s err=%v starts=%d", replay.Status, err, h.fixture.starts.Load())
	}
	otherUserRecord, err := h.service.GetEnvelopeInvocation(context.Background(), "token-other-user", candidate.InvocationID)
	if !errors.Is(err, commandledger.ErrNotFound) || otherUserRecord.Status != "" || otherUserRecord.Ownership.UserID != nil {
		t.Fatalf("lookup de outro usuário não ocultou registro: record=%+v err=%v", otherUserRecord, err)
	}
}

func TestExternalServiceDeniesMissingScopeOrRoleBeforeHandler(t *testing.T) {
	h := newExternalHarness(t, nil)
	for _, token := range []string{"token-no-scope", "token-no-role"} {
		t.Run(token, func(t *testing.T) {
			candidate := externalCandidate()
			record, output, err := h.service.ExecuteEnvelopeWithResult(context.Background(), token, candidate)
			if err != nil || record.Status != commandledger.Denied || output != nil {
				t.Fatalf("claim ausente deveria produzir terminal Denied sem output: status=%s output=%s err=%v", record.Status, output, err)
			}
			deniedOwner := record.Ownership
			stored, err := h.service.executor.config.Store.GetEnvelopeByID(context.Background(), deniedOwner, candidate.InvocationID)
			if err != nil || stored.Status != commandledger.Denied {
				t.Fatalf("terminal Denied não foi persistido no ledger: status=%s err=%v", stored.Status, err)
			}
			throughService, err := h.service.GetEnvelopeInvocation(context.Background(), token, candidate.InvocationID)
			if !errors.Is(err, ErrDenied) || throughService.Status != "" {
				t.Fatalf("lookup externo sem autorização da operação não negou: record=%+v err=%v", throughService, err)
			}
		})
	}
	if got := h.fixture.starts.Load(); got != 0 {
		t.Fatalf("handler começou antes da autorização: starts=%d", got)
	}
}

func TestExternalServiceRevocationCancelsQueuedInvocation(t *testing.T) {
	queued := make(chan struct{})
	h := newExternalHarness(t, func(config *Config) {
		config.Envelope.AwaitQueue = func(ctx context.Context, _ commandcontract.Envelope) error {
			close(queued)
			<-ctx.Done()
			return ctx.Err()
		}
	})
	result := make(chan struct {
		record commandledger.FullRecord
		err    error
	}, 1)
	go func() {
		record, err := h.service.ExecuteEnvelope(context.Background(), "token-a", externalCandidate())
		result <- struct {
			record commandledger.FullRecord
			err    error
		}{record: record, err: err}
	}()
	select {
	case <-queued:
	case <-time.After(2 * time.Second):
		t.Fatal("execução não chegou à fila")
	}
	if err := h.service.RevokeIdentity(context.Background(), "admin-token", externalTestIssuer, "subject-a"); err != nil {
		t.Fatalf("revogar identidade: %v", err)
	}
	select {
	case got := <-result:
		if got.err != nil || got.record.Status != commandledger.CancelledStale || h.fixture.starts.Load() != 0 {
			t.Fatalf("revogação não cancelou antes do handler: status=%s err=%v starts=%d", got.record.Status, got.err, h.fixture.starts.Load())
		}
	case <-time.After(2 * time.Second):
		t.Fatal("execução não encerrou após revogação")
	}
}

func TestExternalServiceValidatesColdTokenOutsideDispatchGate(t *testing.T) {
	h := newExternalHarness(t, nil)
	entered, release := h.verifier.block("token-a")
	result := make(chan error, 1)
	go func() {
		_, err := h.service.ExecuteEnvelope(context.Background(), "token-a", externalCandidate())
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		close(release)
		t.Fatal("validação completa não foi iniciada")
	}
	mutated := make(chan error, 1)
	go func() {
		mutated <- h.base.epochs.MutateContext(context.Background(), commandsecurity.ContextPrincipal{
			UserID: h.users["subject-a"], Type: string(commandcontract.AuthExternalToken), ID: "unrelated-token-context",
		}, func() error { return nil })
	}()
	select {
	case err := <-mutated:
		if err != nil {
			close(release)
			t.Fatalf("mutação concorrente: %v", err)
		}
	case <-time.After(time.Second):
		close(release)
		t.Fatal("validação completa reteve o DispatchGate")
	}
	close(release)
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("execução com token frio: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("execução com token frio não terminou")
	}
	if h.verifier.validates.Load() != 1 || h.verifier.cached.Load() == 0 {
		t.Fatalf("fluxo cold/cache inesperado: Validate=%d ValidateCached=%d", h.verifier.validates.Load(), h.verifier.cached.Load())
	}
	if _, err := h.service.ExecuteEnvelope(context.Background(), "token-a", externalCandidate()); err != nil {
		t.Fatalf("execução warm: %v", err)
	}
	if h.verifier.validates.Load() != 2 || h.verifier.cached.Load() < 2 {
		t.Fatalf("revalidação por request incompleta: Validate=%d ValidateCached=%d", h.verifier.validates.Load(), h.verifier.cached.Load())
	}
}

func TestExternalServiceShutdownCancelsBlockedWarmAuthentication(t *testing.T) {
	h := newExternalHarness(t, nil)
	entered, _ := h.verifier.block("token-a")
	result := make(chan error, 1)
	go func() {
		_, err := h.service.ExecuteEnvelope(context.Background(), "token-a", externalCandidate())
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("validação completa não foi iniciada")
	}
	shutdown := make(chan error, 1)
	go func() { shutdown <- h.service.Shutdown(context.Background()) }()
	select {
	case <-h.verifier.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown não cancelou a validação bloqueada")
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("request cancelado pelo shutdown retornou sucesso")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request não encerrou após cancelamento")
	}
	select {
	case err := <-shutdown:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown não drenou autenticação em andamento")
	}
	if got := h.verifier.cached.Load(); got != 0 {
		t.Fatalf("autenticação cached começou depois do cancelamento: %d", got)
	}
	if got := h.fixture.starts.Load(); got != 0 {
		t.Fatalf("handler começou após shutdown: %d", got)
	}
}

func TestExternalServiceShutdownCancelsBlockedAdminAuthentication(t *testing.T) {
	h := newExternalHarness(t, nil)
	entered, _ := h.verifier.block("admin-token")
	result := make(chan error, 1)
	go func() {
		result <- h.service.RevokeIdentity(context.Background(), "admin-token", externalTestIssuer, "subject-a")
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("autenticação admin não foi iniciada")
	}
	shutdown := make(chan error, 1)
	go func() { shutdown <- h.service.Shutdown(context.Background()) }()
	select {
	case <-h.verifier.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown não cancelou autenticação admin bloqueada")
	}
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("revogação cancelada pelo shutdown retornou sucesso")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("revogação não encerrou após cancelamento")
	}
	select {
	case err := <-shutdown:
		if err != nil {
			t.Fatalf("Shutdown: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Shutdown não drenou autenticação admin")
	}
	if _, err := h.repository.Resolve(context.Background(), externalTestIssuer, "subject-a"); err != nil {
		t.Fatalf("revogação persistiu apesar do cancelamento prévio: %v", err)
	}
	if got := h.fixture.starts.Load(); got != 0 {
		t.Fatalf("handler começou durante revogação admin: %d", got)
	}
}

func TestNewExternalRejectsPhysicalSources(t *testing.T) {
	for _, source := range []commandcatalog.Source{commandcatalog.KeyboardLocal, commandcatalog.KeyboardGlobal, commandcatalog.StreamDeck} {
		t.Run(string(source), func(t *testing.T) {
			h := newExternalHarness(t, nil)
			config := h.fixture.service.config
			config.Source = source
			_, err := NewExternal(config, h.authenticator, h.admin, []commandidentity.AuthorizationRule{{
				CommandID: "generic.read", Actors: []commandcontract.ActorType{commandcontract.ActorUser},
				RequiredRoles: []string{externalTestRole}, RequiredScopes: []string{externalTestScope},
			}})
			if !errors.Is(err, ErrInvalidConfiguration) {
				t.Fatalf("origem física deveria ser rejeitada no construtor: %s err=%v", source, err)
			}
		})
	}
}

func TestNewExternalWithoutReadinessFailsClosedPerRequest(t *testing.T) {
	h := newExternalHarness(t, nil)
	unready := auth.NewExternalCommandAuthenticator(h.verifier, h.repository)
	config := h.fixture.service.config
	config.Store = h.base.store
	config.Epochs = h.base.epochs
	service, err := NewExternal(config, unready, h.admin, []commandidentity.AuthorizationRule{{
		CommandID: "generic.read", Actors: []commandcontract.ActorType{commandcontract.ActorUser},
		RequiredRoles: []string{externalTestRole}, RequiredScopes: []string{externalTestScope},
	}})
	if err != nil {
		t.Fatalf("construtor não deve publicar readiness implicitamente: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := service.Shutdown(ctx); err != nil {
			t.Errorf("encerrar serviço sem readiness: %v", err)
		}
	})
	if _, err := service.ExecuteEnvelope(context.Background(), "token-a", externalCandidate()); !errors.Is(err, ErrDenied) {
		t.Fatalf("request sem readiness não falhou fechado: %v", err)
	}
	if got := h.fixture.starts.Load(); got != 0 {
		t.Fatalf("handler começou sem readiness: %d", got)
	}
}
