package wailsapi

import (
	"assistente/controllers"
	"assistente/internal/database"
	"assistente/internal/jobprofilegrant"
	"assistente/internal/jobs"
	"assistente/internal/profileaccess"
	"assistente/internal/profiles"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestJobsNotWired(t *testing.T) {
	t.Parallel()
	api := NewJobs()
	if _, err := api.GetJobs(); !errors.Is(err, ErrJobsNotWired) {
		t.Fatalf("got %v", err)
	}
}

func TestJobsUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	dir := filepath.Dir(thisFile)
	for _, name := range []string{"jobs.go", "jobs_dryrun.go"} {
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		body := string(src)
		if strings.Contains(body, "requireAuthenticatedContext(") {
			t.Fatalf("%s não deve chamar requireAuthenticatedContext(; use WithUser", name)
		}
	}
	src, err := os.ReadFile(filepath.Join(dir, "jobs.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "WithUser(") {
		t.Fatal("jobs.go deve chamar WithUser(")
	}
}

func TestJobGrantCoversLiteralAndDynamicProfiles(t *testing.T) {
	state := profileaccess.JobGrantState{
		Grants: []jobprofilegrant.Grant{{TargetProfileSlug: "especialista"}},
	}
	if !jobGrantCovers(state, map[string]any{"profile": "especialista"}) {
		t.Fatal("grant literal exato deveria permitir habilitação")
	}
	if jobGrantCovers(state, map[string]any{"profile": "outro"}) {
		t.Fatal("grant de outro target não deveria permitir habilitação")
	}
	if !jobGrantCovers(state, map[string]any{"profile": "{{ event.profile }}"}) {
		t.Fatal("template com ao menos um target autorizado deveria permitir habilitação")
	}
	if jobGrantCovers(profileaccess.JobGrantState{}, map[string]any{"profile": "{{ event.profile }}"}) {
		t.Fatal("template sem targets autorizados não deveria permitir habilitação")
	}
}

func TestCanonicalSaveJobIDMatchesPersistenceNormalization(t *testing.T) {
	if got := canonicalSaveJobID("Meu_Job"); got != "meu-job" {
		t.Fatalf("ID canônico = %q, esperado meu-job", got)
	}
}

type jobsGrantTestProfiles struct{}

func (jobsGrantTestProfiles) List() ([]profiles.ProfileInfo, error) { return nil, nil }
func (jobsGrantTestProfiles) Get(slug string) (*profiles.Profile, error) {
	return &profiles.Profile{Name: slug}, nil
}

type jobsGrantTestStore struct {
	config  jobprofilegrant.DelegationConfig
	grants  []jobprofilegrant.Grant
	listErr error
}

func (s *jobsGrantTestStore) CurrentDelegation(context.Context, string) (jobprofilegrant.DelegationConfig, error) {
	return s.config, nil
}
func (s *jobsGrantTestStore) AuthorizationSnapshot(context.Context, string, string) (jobprofilegrant.AuthorizationSnapshot, error) {
	return jobprofilegrant.AuthorizationSnapshot{Config: s.config}, nil
}
func (s *jobsGrantTestStore) HasValid(context.Context, string, string, string) (bool, error) {
	return len(s.grants) > 0, nil
}
func (s *jobsGrantTestStore) ListValid(context.Context, string) ([]jobprofilegrant.Grant, jobprofilegrant.DelegationConfig, error) {
	return s.grants, s.config, s.listErr
}
func (*jobsGrantTestStore) Grant(context.Context, string, string, string, string, uint64) error {
	return nil
}
func (*jobsGrantTestStore) Revoke(context.Context, string, string, string) error { return nil }
func (*jobsGrantTestStore) BeginProfileRevocation(context.Context, string, string, string) error {
	return nil
}
func (*jobsGrantTestStore) CancelProfileRevocation(context.Context, string) error { return nil }
func (*jobsGrantTestStore) RevokeProfileGlobal(context.Context, string, string) error {
	return nil
}

func setupJobsGrantBehaviorTest(t *testing.T) (*Jobs, *jobs.Manager, *gorm.DB, context.Context, *jobsGrantTestStore) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if sqlDB, dbErr := db.DB(); dbErr == nil {
		sqlDB.SetMaxOpenConns(1)
	}
	if err := db.AutoMigrate(
		&database.User{}, &database.ToolCatalog{}, &database.Tag{}, &database.TagAssignment{},
		&database.JobPipeline{}, &database.Job{}, &database.JobProfileGrant{},
		&database.JobProfileGrantEpoch{}, &database.ProfileGrantRevocationIntent{},
		&database.JobTrigger{}, &database.JobRun{},
		&database.JobEvent{}, &database.JobRunEvent{},
	); err != nil {
		t.Fatal(err)
	}
	userID := "user-jobs-grants"
	ctx := database.WithUserID(context.Background(), userID)
	if err := db.Create(&database.User{UUIDModel: database.UUIDModel{ID: userID}, Username: userID}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&database.ToolCatalog{
		UUIDModel: database.UUIDModel{ID: "tool-subagent"}, Name: "subagent",
		DisplayName: "Subagent", Origin: "builtin", AvailabilityStatus: "available",
	}).Error; err != nil {
		t.Fatal(err)
	}
	manager := jobs.NewManager(jobs.ManagerConfig{
		Repository: jobs.NewDBRepository(db), ContextProvider: func() context.Context { return ctx },
	})
	grantStore := &jobsGrantTestStore{}
	access := profileaccess.NewService(jobsGrantTestProfiles{}, nil, nil, nil).WithJobGrants(grantStore)
	api := NewJobs()
	AttachJobs(
		api, stubSession{ctx: ctx},
		controllers.NewJobsController(controllers.JobsControllerConfig{JobMgr: manager}),
		nil, nil, access,
	)
	return api, manager, db, ctx, grantStore
}

func grantBehaviorJobJSON(t *testing.T, id, expression string, enabled bool) string {
	t.Helper()
	payload, err := json.Marshal(jobs.Job{
		ID: id, Name: id, Tool: "subagent", Enabled: enabled,
		Inputs:   map[string]any{"profile": expression, "prompt": "x"},
		Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(payload)
}

func TestJobsSaveJobFailsClosedWithoutGrantAndAllowsExactGrant(t *testing.T) {
	api, manager, db, ctx, state := setupJobsGrantBehaviorTest(t)
	expression := "pesquisa"
	fingerprint := jobprofilegrant.Fingerprint("subagent", expression)
	state.config = jobprofilegrant.DelegationConfig{JobSlug: "literal", Tool: "subagent", ProfileExpression: expression, Fingerprint: fingerprint}

	result, err := api.SaveJob(grantBehaviorJobJSON(t, "literal", expression, true))
	if err != nil {
		t.Fatal(err)
	}
	saved, err := manager.GetJobContext(ctx, "literal")
	if err != nil {
		t.Fatal(err)
	}
	if !result.AuthorizationRequired || saved.Enabled {
		t.Fatalf("save literal sem grant não falhou fechado: result=%#v enabled=%v", result, saved.Enabled)
	}

	store := jobprofilegrant.NewStore(db)
	snapshot, err := store.AuthorizationSnapshot(ctx, "literal", expression)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Grant(ctx, snapshot.Config.JobID, expression, fingerprint, "teste", snapshot.Generation); err != nil {
		t.Fatal(err)
	}
	state.config = snapshot.Config
	state.grants = []jobprofilegrant.Grant{{TargetProfileSlug: expression}}
	result, err = api.SaveJob(grantBehaviorJobJSON(t, "literal", expression, true))
	if err != nil {
		t.Fatal(err)
	}
	saved, _ = manager.GetJobContext(ctx, "literal")
	if result.AuthorizationRequired || !saved.Enabled {
		t.Fatalf("save literal autorizado foi bloqueado: result=%#v enabled=%v", result, saved.Enabled)
	}
	for _, expectedState := range []error{
		jobprofilegrant.ErrNotSubagentJob,
		jobprofilegrant.ErrProfileExpressionRequired,
	} {
		state.listErr = expectedState
		result, err = api.SaveJob(grantBehaviorJobJSON(t, "literal", "programacao", true))
		if err != nil {
			t.Fatalf("transição válida foi bloqueada por %v: %v", expectedState, err)
		}
		saved, _ = manager.GetJobContext(ctx, "literal")
		if !result.AuthorizationRequired || saved.Enabled {
			t.Fatalf("transição sem grant não falhou fechado: result=%#v enabled=%v", result, saved.Enabled)
		}
	}
	state.listErr = nil

	state.config = jobprofilegrant.DelegationConfig{
		JobSlug: "dinamico", Tool: "subagent",
		ProfileExpression: "{{ .event.profile }}",
		Fingerprint:       jobprofilegrant.Fingerprint("subagent", "{{ .event.profile }}"),
	}
	state.grants = nil
	result, err = api.SaveJob(grantBehaviorJobJSON(t, "dinamico", "{{ .event.profile }}", true))
	if err != nil {
		t.Fatal(err)
	}
	if !result.AuthorizationRequired || !result.DynamicProfile {
		t.Fatalf("template sem grant não foi sinalizado: %#v", result)
	}
	if err := api.ToggleJob("dinamico", true); !errors.Is(err, profileaccess.ErrAuthorizationNotGranted) {
		t.Fatalf("toggle dinâmico sem grant deveria falhar fechado: %v", err)
	}
}

func TestJobsToggleJobChecksGrantBeforeChangingState(t *testing.T) {
	api, manager, db, ctx, state := setupJobsGrantBehaviorTest(t)
	expression := "pesquisa"
	fingerprint := jobprofilegrant.Fingerprint("subagent", expression)
	createCandidate := &jobs.Job{
		ID: "create-sem-grant", Name: "create-sem-grant", Tool: "subagent", Enabled: true,
		Inputs:   map[string]any{"profile": expression, "prompt": "x"},
		Triggers: []jobs.Trigger{{Type: jobs.TriggerManual}},
	}
	if err := manager.CreateJobContext(ctx, createCandidate); !errors.Is(err, jobprofilegrant.ErrAuthorizationNotGranted) {
		t.Fatalf("create compartilhado confirmou ativação recusada: %v", err)
	}
	created, _ := manager.GetJobContext(ctx, "create-sem-grant")
	if created == nil || created.Enabled {
		t.Fatalf("create recusado não reconciliou estado persistido: %#v", created)
	}
	state.config = jobprofilegrant.DelegationConfig{
		JobSlug: "toggle", Tool: "subagent", ProfileExpression: expression, Fingerprint: fingerprint,
	}
	if _, err := api.SaveJob(grantBehaviorJobJSON(t, "toggle", expression, false)); err != nil {
		t.Fatal(err)
	}
	if err := manager.ToggleJobContext(ctx, "toggle", true); !errors.Is(err, jobprofilegrant.ErrAuthorizationNotGranted) {
		t.Fatalf("manager compartilhado confirmou ativação recusada: %v", err)
	}
	if err := api.ToggleJob("toggle", true); !errors.Is(err, profileaccess.ErrAuthorizationNotGranted) {
		t.Fatalf("toggle sem grant deveria falhar fechado: %v", err)
	}
	saved, _ := manager.GetJobContext(ctx, "toggle")
	if saved.Enabled {
		t.Fatal("toggle sem grant alterou o job")
	}
	storeErr := errors.New("SQLite indisponível")
	state.listErr = storeErr
	if err := api.ToggleJob("toggle", true); !errors.Is(err, storeErr) {
		t.Fatalf("toggle ocultou erro operacional do store: %v", err)
	}
	if _, err := api.SaveJob(grantBehaviorJobJSON(t, "toggle", expression, true)); !errors.Is(err, storeErr) {
		t.Fatalf("save ocultou erro operacional do store: %v", err)
	}
	state.listErr = nil

	store := jobprofilegrant.NewStore(db)
	snapshot, err := store.AuthorizationSnapshot(ctx, "toggle", expression)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Grant(ctx, snapshot.Config.JobID, expression, fingerprint, "teste", snapshot.Generation); err != nil {
		t.Fatal(err)
	}
	state.config = snapshot.Config
	state.grants = []jobprofilegrant.Grant{{TargetProfileSlug: expression}}
	if err := api.ToggleJob("toggle", true); err != nil {
		t.Fatal(err)
	}
	saved, _ = manager.GetJobContext(ctx, "toggle")
	if !saved.Enabled {
		t.Fatal("toggle com grant exato não habilitou o job")
	}

	state.config.Fingerprint = "fingerprint-divergente"
	if err := api.ToggleJob("toggle", true); !errors.Is(err, profileaccess.ErrAuthorizationNotGranted) {
		t.Fatalf("fingerprint divergente deveria falhar fechado: %v", err)
	}
}
