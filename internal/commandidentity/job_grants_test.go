package commandidentity

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/database"
	"assistente/internal/jobprofilegrant"
)

type exactJobGrantFixture struct{}

func (exactJobGrantFixture) HasValidGeneration(ctx context.Context, job, target, fingerprint string, generation uint64) (bool, error) {
	owner, err := database.RequireUserID(ctx)
	return owner == "user" && job == "job" && target == "profile" && fingerprint == "delegation" && generation == 4, err
}

func TestJobIdentityRevalidatesRealGrantGeneration(t *testing.T) {
	db := commandIdentityDB(t)
	user := commandIdentityUser(t, db)
	if err := db.AutoMigrate(&database.ToolCatalog{}, &database.Job{}, &database.JobProfileGrant{}, &database.JobProfileGrantEpoch{}, &database.ProfileGrantRevocationIntent{}); err != nil {
		t.Fatal(err)
	}
	catalog := database.ToolCatalog{Name: "subagent", DisplayName: "Subagent", Origin: "builtin", AvailabilityStatus: "available"}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	job := database.Job{UserID: user.ID, Slug: "exact-job", Name: "Exact", Enabled: true, ToolCatalogID: catalog.ID, ToolName: "subagent", Inputs: `{"profile":"specialist"}`}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}
	grants := jobprofilegrant.NewStore(db)
	ctx := database.WithUserID(context.Background(), user.ID)
	fingerprint := jobprofilegrant.Fingerprint("subagent", "specialist")
	if err := grants.Grant(ctx, job.ID, "specialist", fingerprint, user.ID, 0); err != nil {
		t.Fatal(err)
	}
	runtime := &jobRuntimeStub{job: TrustedJob{DatabaseID: job.ID, Slug: job.Slug, OwnerUserID: user.ID, TargetProfileSlug: "specialist", JobDefinitionFingerprint: "definition-v1", DelegationFingerprint: fingerprint, GrantGeneration: 0, RunID: "run-exact"}}
	service, err := New(Config{Epochs: &epochPortStub{}, JobRuntime: runtime, JobGrants: grants, AuthorizationRules: []AuthorizationRule{{CommandID: "maintenance.read", Actors: []commandcontract.ActorType{commandcontract.ActorAutomation}}}})
	if err != nil {
		t.Fatal(err)
	}
	request := JobServiceRequest{Capability: JobServiceCapabilityForRuntime(), JobDatabaseID: job.ID, TargetProfileSlug: "specialist", RunID: "run-exact", Source: commandcatalog.Event}
	// A revalidação sob gate não pode adquirir o writer nem fazer manutenção.
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := db.Exec("PRAGMA query_only = ON").Error; err != nil {
		t.Fatal(err)
	}
	identity, err := service.ResolveJobService(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	authorization := AuthorizationRequest{Identity: identity, Definition: safeJobDefinition(), JobService: &request}
	if err := service.Authorize(context.Background(), authorization); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("PRAGMA query_only = OFF").Error; err != nil {
		t.Fatal(err)
	}
	if err := grants.Revoke(ctx, job.ID, "specialist", user.ID); err != nil {
		t.Fatal(err)
	}
	if err := grants.Grant(ctx, job.ID, "specialist", fingerprint, user.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := service.Authorize(context.Background(), authorization); err == nil {
		t.Fatal("regrant validou geração antiga")
	}
	runtime.job.GrantGeneration = 1
	fresh, err := service.ResolveJobService(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	authorization.Identity = fresh
	if err := service.Authorize(context.Background(), authorization); err != nil {
		t.Fatal(err)
	}
	// Uma intenção de exclusão de profile já suspende o grant, antes da purga.
	if err := grants.BeginProfileRevocation(ctx, "specialist", "profile-v1", user.ID); err != nil {
		t.Fatal(err)
	}
	if err := service.Authorize(context.Background(), authorization); err == nil {
		t.Fatal("intenção de revogação foi ignorada")
	}
}

func TestJobIdentityRequiresGrantStoreAndExactOwner(t *testing.T) {
	runtime := &jobRuntimeStub{job: TrustedJob{DatabaseID: "job", Slug: "daily", OwnerUserID: "foreign", TargetProfileSlug: "profile", JobDefinitionFingerprint: "definition", DelegationFingerprint: "delegation", GrantGeneration: 4, RunID: "run"}}
	if _, err := New(Config{Epochs: &epochPortStub{}, JobRuntime: runtime}); !errors.Is(err, ErrJobExecutionDenied) {
		t.Fatalf("store ausente: %v", err)
	}
	service, err := New(Config{Epochs: &epochPortStub{}, JobRuntime: runtime, JobGrants: exactJobGrantFixture{}})
	if err != nil {
		t.Fatal(err)
	}
	request := JobServiceRequest{Capability: JobServiceCapabilityForRuntime(), JobDatabaseID: "job", TargetProfileSlug: "profile", RunID: "run", Source: commandcatalog.Event}
	if _, err := service.ResolveJobService(database.WithUserID(context.Background(), "user"), request); !errors.Is(err, ErrJobExecutionDenied) {
		t.Fatalf("owner do ctx mascarou owner real: %v", err)
	}
}
