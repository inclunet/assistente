package commandidentity

import (
	"context"
	"errors"
	"strings"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandsecurity"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type epochPortStub struct{ captures, mutations int }

func (p *epochPortStub) CaptureContextAuthenticated(_ context.Context, authenticate func() (ContextPrincipal, error)) (Epoch, error) {
	p.captures++
	if _, err := authenticate(); err != nil {
		return Epoch{}, err
	}
	return Epoch{AuthGeneration: "auth-generation", SecurityGeneration: "security-generation"}, nil
}
func (p *epochPortStub) MutateContext(_ context.Context, _ ContextPrincipal, action func() error) error {
	p.mutations++
	return action()
}
func (p *epochPortStub) MutateContextGroup(ctx context.Context, principal ContextPrincipal, action func() error) error {
	return p.MutateContext(ctx, principal, action)
}

type cachedClaimsStub struct {
	claims          *auth.ExternalClaims
	network, cached int
}

func (v *cachedClaimsStub) Validate(_ context.Context, _ string) (*auth.ExternalClaims, error) {
	v.network++
	return v.claims, nil
}
func (v *cachedClaimsStub) ValidateCached(_ context.Context, _ string) (*auth.ExternalClaims, error) {
	v.cached++
	return v.claims, nil
}

func commandIdentityDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&database.User{}, &auth.ExternalIdentityMapping{}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func commandIdentityUser(t *testing.T, db *gorm.DB) *database.User {
	t.Helper()
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	user := &database.User{UUIDModel: database.UUIDModel{ID: id.String()}, Username: "mapped", PasswordHash: "unused", Role: database.UserRoleUser, IsActive: true}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	return user
}

func safeCommandDefinition() commandcatalog.Definition {
	return commandcatalog.Definition{ID: "maintenance.read", Effect: commandcatalog.Read, Decision: commandcatalog.NoDecision, Context: commandcatalog.ContextPolicy{None: true}, AllowedSources: []commandcatalog.Source{commandcatalog.Palette, commandcatalog.System}, HandlerClassification: commandcatalog.HandlerInternal}
}

func TestAuthorizeRelêOrigemEIgnoraRoleForjado(t *testing.T) {
	db := commandIdentityDB(t)
	user := commandIdentityUser(t, db)
	repo := auth.NewExternalIdentityRepository(db)
	claims := &auth.ExternalClaims{Issuer: "issuer", Subject: "subject", Scope: "commands:read"}
	verifier := &cachedClaimsStub{claims: claims}
	external := auth.NewExternalCommandAuthenticator(verifier, repo)
	// Adoção explícita do middleware: a tabela presente não habilita o caminho
	// externo sozinha.
	external.SetReadiness(repo.CheckReadiness)
	if _, err := repo.Create(context.Background(), auth.ExternalIdentityMappingParams{Issuer: "issuer", Subject: "subject", UserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	epochs := &epochPortStub{}
	service, err := New(Config{External: external, Epochs: epochs, AuthorizationRules: []AuthorizationRule{{CommandID: "maintenance.read", Actors: []commandcontract.ActorType{commandcontract.ActorUser}, RequiredScopes: []string{"commands:read"}}}})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := service.ResolveExternalToken(context.Background(), ExternalTokenRequest{AccessToken: "synthetic-token", Source: commandcatalog.Palette})
	if err != nil {
		t.Fatal(err)
	}
	identity.Role = database.UserRoleAdmin
	if err := service.Authorize(context.Background(), AuthorizationRequest{Identity: identity, Definition: safeCommandDefinition(), ExternalToken: &ExternalTokenRequest{AccessToken: "synthetic-token", Source: commandcatalog.Palette}}); !errors.Is(err, ErrCommandNotAuthorized) {
		t.Fatalf("role forjado não deveria autorizar: %v", err)
	}
	if epochs.captures != 1 {
		t.Fatalf("Authorize chamou Capture recursivamente: %d", epochs.captures)
	}
	if verifier.network != 1 || verifier.cached != 2 {
		t.Fatalf("revalidação externa inesperada: network=%d cached=%d", verifier.network, verifier.cached)
	}
}

func TestExternalCaptureERevogacaoUsamMesmoEpochReal(t *testing.T) {
	ctx := context.Background()
	db := commandIdentityDB(t)
	user := commandIdentityUser(t, db)
	repo := auth.NewExternalIdentityRepository(db)
	if _, err := repo.Create(ctx, auth.ExternalIdentityMappingParams{Issuer: "issuer", Subject: "subject", UserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	verifier := &cachedClaimsStub{claims: &auth.ExternalClaims{
		Issuer: "issuer", Subject: "subject", Scope: "commands:read identity:admin",
	}}
	external := auth.NewExternalCommandAuthenticator(verifier, repo)
	external.SetReadiness(repo.CheckReadiness)
	admin, err := auth.NewExternalIdentityAdminService(verifier, repo, auth.ExternalIdentityAdminConfig{AdminScopes: []string{"identity:admin"}})
	if err != nil {
		t.Fatal(err)
	}
	core, err := commandsecurity.NewEpochService(&commandsecurity.DispatchGate{})
	if err != nil {
		t.Fatal(err)
	}
	epochs, err := NewCoreEpochs(core)
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(Config{External: external, ExternalAdmin: admin, Epochs: epochs})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := service.ResolveExternalToken(ctx, ExternalTokenRequest{AccessToken: "external-token", Source: commandcatalog.Palette})
	if err != nil {
		t.Fatal(err)
	}
	wantContextID := auth.ExternalTokenContextID("issuer", "subject", "external-token")
	if identity.AuthContextID != wantContextID || strings.ContainsRune(identity.AuthContextID, '\x00') {
		t.Fatalf("contexto externo não usa framing JSON: %q", identity.AuthContextID)
	}
	snapshot, err := core.CaptureContextAuthenticated(ctx, func(context.Context) (commandsecurity.ContextPrincipal, error) {
		return commandsecurity.ContextPrincipal{UserID: user.ID, Type: string(commandcontract.AuthExternalToken), ID: wantContextID, GroupID: auth.ExternalIdentityContextID("issuer", "subject")}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	watch, release, err := core.WatchEpoch(ctx, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if err := service.RevokeExternal(ctx, "admin-token", "issuer", "subject"); err != nil {
		t.Fatalf("revogação externa: %v", err)
	}
	if watch.Err() == nil {
		t.Fatal("revogação não cancelou watch do EpochService real")
	}
}

func TestSystemCapabilityRestritaESemOwner(t *testing.T) {
	epochs := &epochPortStub{}
	service, err := New(Config{Epochs: epochs, AuthorizationRules: []AuthorizationRule{{CommandID: "maintenance.read", Actors: []commandcontract.ActorType{commandcontract.ActorAutomation}, AllowSystem: true}}})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := service.ResolveSystem(context.Background(), SystemRequest{Capability: SystemCapabilityForBootstrap()})
	if err != nil {
		t.Fatal(err)
	}
	if identity.UserID != "" || identity.SessionID != "" {
		t.Fatalf("system recebeu owner: %+v", identity)
	}
	if err := service.Authorize(context.Background(), AuthorizationRequest{Identity: identity, Definition: safeCommandDefinition(), System: &SystemRequest{Capability: SystemCapabilityForBootstrap()}}); err != nil {
		t.Fatalf("system seguro deveria autorizar: %v", err)
	}
	if epochs.captures != 1 {
		t.Fatalf("Authorize system recapturou epoch: %d", epochs.captures)
	}
	unsafe := safeCommandDefinition()
	unsafe.HandlerClassification = commandcatalog.HandlerBackend
	if err := service.Authorize(context.Background(), AuthorizationRequest{Identity: identity, Definition: unsafe, System: &SystemRequest{Capability: SystemCapabilityForBootstrap()}}); !errors.Is(err, ErrCommandNotAuthorized) {
		t.Fatalf("system backend deveria ser negado: %v", err)
	}
}

func TestJobServiceExigeCapabilityERuntimeAutenticado(t *testing.T) {
	epochs := &epochPortStub{}
	runtime := &jobRuntimeStub{job: TrustedJob{DatabaseID: "job", Slug: "daily", OwnerUserID: "user", TargetProfileSlug: "profile", JobDefinitionFingerprint: "fingerprint", DelegationFingerprint: "delegation", GrantGeneration: 4, RunID: "legacy-run/2021-04-17"}}
	service, err := New(Config{Epochs: epochs, JobRuntime: runtime, JobGrants: exactJobGrantFixture{}, AuthorizationRules: []AuthorizationRule{{CommandID: "maintenance.read", Actors: []commandcontract.ActorType{commandcontract.ActorAutomation}}}})
	if err != nil {
		t.Fatal(err)
	}
	request := JobServiceRequest{Capability: JobServiceCapabilityForRuntime(), JobDatabaseID: "job", TargetProfileSlug: "profile", RunID: "legacy-run/2021-04-17", Source: commandcatalog.Event}
	identity, err := service.ResolveJobService(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.Authorize(context.Background(), AuthorizationRequest{Identity: identity, Definition: safeJobDefinition(), JobService: &request}); err != nil {
		t.Fatalf("job deveria autorizar: %v", err)
	}
	if runtime.revalidations != 1 {
		t.Fatalf("runtime não foi relido no gate final: %d", runtime.revalidations)
	}
	if runtime.lastRequest.RunID != request.RunID || runtime.lastRequest.JobDatabaseID != request.JobDatabaseID || runtime.lastRequest.TargetProfileSlug != request.TargetProfileSlug {
		t.Fatalf("lookup não preservou referências opacas: %+v", runtime.lastRequest)
	}
	request.Capability = nil
	if _, err := service.ResolveJobService(context.Background(), request); !errors.Is(err, ErrJobExecutionDenied) {
		t.Fatalf("job sem capability deveria falhar: %v", err)
	}
}

func TestRevogacaoExternaPreparaForaEDesabilitaNoMesmoEpoch(t *testing.T) {
	db := commandIdentityDB(t)
	user := commandIdentityUser(t, db)
	repo := auth.NewExternalIdentityRepository(db)
	if _, err := repo.Create(context.Background(), auth.ExternalIdentityMappingParams{Issuer: "issuer", Subject: "subject", UserID: user.ID}); err != nil {
		t.Fatal(err)
	}
	verifier := &cachedClaimsStub{claims: &auth.ExternalClaims{Scope: "identity:admin"}}
	admin, err := auth.NewExternalIdentityAdminService(verifier, repo, auth.ExternalIdentityAdminConfig{AdminScopes: []string{"identity:admin"}})
	if err != nil {
		t.Fatal(err)
	}
	epochs := &epochPortStub{}
	service, err := New(Config{Epochs: epochs, ExternalAdmin: admin})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.RevokeExternal(context.Background(), "admin-token", "issuer", "subject"); err != nil {
		t.Fatalf("revogação externa: %v", err)
	}
	if epochs.mutations != 1 {
		t.Fatalf("revogação não usou MutateContext: %d", epochs.mutations)
	}
	if _, err := repo.Resolve(context.Background(), "issuer", "subject"); !errors.Is(err, auth.ErrExternalIdentityRevoked) {
		t.Fatalf("vínculo revogado ainda resolvido: %v", err)
	}
}

func safeJobDefinition() commandcatalog.Definition {
	d := safeCommandDefinition()
	d.AllowedSources = []commandcatalog.Source{commandcatalog.Event}
	return d
}

type jobRuntimeStub struct {
	job           TrustedJob
	lastRequest   JobServiceRequest
	revalidations int
}

func (r *jobRuntimeStub) ResolveCommandJob(_ context.Context, _ JobServiceCapability, request JobServiceRequest) (TrustedJob, error) {
	r.lastRequest = request
	return r.job, nil
}
func (r *jobRuntimeStub) RevalidateCommandJob(_ context.Context, _ JobServiceCapability, job TrustedJob) error {
	r.revalidations++
	if job != r.job {
		return ErrJobExecutionDenied
	}
	return nil
}
