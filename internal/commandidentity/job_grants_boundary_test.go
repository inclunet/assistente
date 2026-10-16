package commandidentity

import (
	"context"
	"testing"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/database"
	"assistente/internal/jobprofilegrant"
	"github.com/google/uuid"
)

func TestJobGrantBoundaryRevalidatesGenerationAndRuntimeOwner(t *testing.T) {
	db := commandIdentityDB(t)
	owner := commandIdentityUser(t, db)
	foreignID, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	foreign := &database.User{UUIDModel: database.UUIDModel{ID: foreignID.String()}, Username: "foreign-" + foreignID.String(), PasswordHash: "unused", Role: database.UserRoleUser, IsActive: true}
	if err := db.Create(foreign).Error; err != nil {
		t.Fatal(err)
	}
	if owner.ID == foreign.ID {
		t.Fatal("fixture criou owners iguais")
	}
	if err := db.AutoMigrate(&database.ToolCatalog{}, &database.Job{}, &database.JobProfileGrant{}, &database.JobProfileGrantEpoch{}, &database.ProfileGrantRevocationIntent{}); err != nil {
		t.Fatal(err)
	}
	catalog := database.ToolCatalog{Name: "subagent", DisplayName: "Subagent", Origin: "builtin", AvailabilityStatus: "available"}
	if err := db.Create(&catalog).Error; err != nil {
		t.Fatal(err)
	}
	job := database.Job{UserID: owner.ID, Slug: "boundary-job", Name: "Boundary", Enabled: true, ToolCatalogID: catalog.ID, ToolName: "subagent", Inputs: `{"profile":"specialist"}`}
	if err := db.Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	grantStore := jobprofilegrant.NewStore(db)
	ctx := database.WithUserID(context.Background(), owner.ID)
	fingerprint := jobprofilegrant.Fingerprint("subagent", "specialist")
	if err := grantStore.Grant(ctx, job.ID, "specialist", fingerprint, owner.ID, 0); err != nil {
		t.Fatal(err)
	}
	runtime := &jobRuntimeStub{job: TrustedJob{
		DatabaseID: job.ID, Slug: job.Slug, OwnerUserID: owner.ID, TargetProfileSlug: "specialist",
		JobDefinitionFingerprint: "definition-v1", DelegationFingerprint: fingerprint, GrantGeneration: 0, RunID: "run-boundary",
	}}
	service, err := New(Config{
		Epochs: &epochPortStub{}, JobRuntime: runtime, JobGrants: grantStore,
		AuthorizationRules: []AuthorizationRule{{CommandID: "maintenance.read", Actors: []commandcontract.ActorType{commandcontract.ActorAutomation}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := JobServiceRequest{Capability: JobServiceCapabilityForRuntime(), JobDatabaseID: job.ID, TargetProfileSlug: "specialist", RunID: "run-boundary", Source: commandcatalog.Event}
	identity, err := service.ResolveJobService(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if identity.Job == nil || identity.Job.OwnerUserID != owner.ID {
		t.Fatalf("owner não foi derivado do runtime: %+v", identity.Job)
	}
	authorization := AuthorizationRequest{Identity: identity, Definition: safeJobDefinition(), JobService: &request}

	// A projeção transportada não pode trocar o owner autoritativo.
	forged := identity
	forgedJob := *identity.Job
	forgedJob.OwnerUserID = foreign.ID
	forged.Job = &forgedJob
	authorization.Identity = forged
	if err := service.Authorize(context.Background(), authorization); err == nil {
		t.Fatal("owner forjado na projeção foi aceito")
	}
	authorization.Identity = identity

	// A revogação/reconcessão entre Resolve e Authorize invalida a geração
	// capturada; não há fallback para a geração atualmente persistida.
	if err := grantStore.Revoke(ctx, job.ID, "specialist", owner.ID); err != nil {
		t.Fatal(err)
	}
	if err := grantStore.Grant(ctx, job.ID, "specialist", fingerprint, owner.ID, 1); err != nil {
		t.Fatal(err)
	}
	if err := service.Authorize(context.Background(), authorization); err == nil {
		t.Fatal("grant substituído validou a identidade da geração antiga")
	}

	runtime.job.GrantGeneration = 1
	fresh, err := service.ResolveJobService(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	authorization.Identity = fresh
	if err := service.Authorize(context.Background(), authorization); err != nil {
		t.Fatalf("grant da geração atual foi recusado: %v", err)
	}

	// Mesmo com uma identidade já resolvida, trocar o owner na fonte real entre
	// Resolve e Authorize não pode herdar o owner do payload anterior.
	runtime.job.OwnerUserID = foreign.ID
	if err := service.Authorize(context.Background(), authorization); err == nil {
		t.Fatal("owner estrangeiro do runtime foi aceito")
	}
}
