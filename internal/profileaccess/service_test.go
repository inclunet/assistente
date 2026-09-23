package profileaccess

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/eventctx"
	"assistente/internal/jobprofilegrant"
	"assistente/internal/profiles"
	"assistente/internal/questionnaire"
)

type fakeProfileStore struct {
	infos  []profiles.ProfileInfo
	bySlug map[string]*profiles.Profile
}

func (f fakeProfileStore) List() ([]profiles.ProfileInfo, error) {
	return f.infos, nil
}

func (f fakeProfileStore) Get(slug string) (*profiles.Profile, error) {
	profile := f.bySlug[slug]
	if profile == nil {
		return nil, errors.New("não encontrado")
	}
	return profile, nil
}

func (f fakeProfileStore) Update(slug string, profile *profiles.Profile) error {
	f.bySlug[slug] = profile
	return nil
}

type fakeAsker struct {
	calls   int
	payload questionnaire.RequestPayload
	resp    questionnaire.Response
	err     error
	onAsk   func()
}

type fakeJobGrants struct {
	configs    []jobprofilegrant.DelegationConfig
	valid      bool
	granted    int
	revoked    int
	generation uint64
	revokeErr  error
	beginErr   error
	currentErr error
	validErr   error
	begun      int
	canceled   int
	notified   int
	beginHook  func()
	revokeHook func()
}

func (f *fakeJobGrants) AuthorizationSnapshot(ctx context.Context, jobID, _ string) (jobprofilegrant.AuthorizationSnapshot, error) {
	config, err := f.CurrentDelegation(ctx, jobID)
	return jobprofilegrant.AuthorizationSnapshot{Config: config, Generation: f.generation}, err
}

func (f *fakeJobGrants) CurrentDelegation(_ context.Context, _ string) (jobprofilegrant.DelegationConfig, error) {
	if f.currentErr != nil {
		return jobprofilegrant.DelegationConfig{}, f.currentErr
	}
	if len(f.configs) == 0 {
		return jobprofilegrant.DelegationConfig{}, errors.New("sem configuração")
	}
	config := f.configs[0]
	if len(f.configs) > 1 {
		f.configs = f.configs[1:]
	}
	return config, nil
}
func (f *fakeJobGrants) HasValid(context.Context, string, string, string) (bool, error) {
	return f.valid, f.validErr
}
func (f *fakeJobGrants) ListValid(context.Context, string) ([]jobprofilegrant.Grant, jobprofilegrant.DelegationConfig, error) {
	config, err := f.CurrentDelegation(context.Background(), "")
	return nil, config, err
}
func (f *fakeJobGrants) Grant(_ context.Context, _, _, _, _ string, expectedGeneration uint64) error {
	if expectedGeneration != f.generation {
		return jobprofilegrant.ErrGrantGenerationChanged
	}
	f.granted++
	return nil
}
func (f *fakeJobGrants) Revoke(context.Context, string, string, string) error {
	f.generation++
	f.revoked++
	return nil
}
func (f *fakeJobGrants) BeginProfileRevocation(context.Context, string, string, string) error {
	f.begun++
	if f.beginHook != nil {
		f.beginHook()
	}
	return f.beginErr
}
func (f *fakeJobGrants) CancelProfileRevocation(context.Context, string) error {
	f.canceled++
	return nil
}
func (f *fakeJobGrants) RevokeProfileGlobal(context.Context, string, string) error {
	f.revoked++
	return f.revokeErr
}
func (f *fakeJobGrants) RevokeProfileGlobalDeferred(context.Context, string, string) (func(), error) {
	f.revoked++
	if f.revokeErr != nil {
		if f.revokeHook != nil {
			f.revokeHook()
		}
		return nil, f.revokeErr
	}
	return func() { f.notified++ }, nil
}

func (f *fakeAsker) Ask(_ context.Context, _ questionnaire.Surface, payload questionnaire.RequestPayload) (questionnaire.Response, error) {
	f.calls++
	f.payload = payload
	if f.onAsk != nil {
		f.onAsk()
	}
	return f.resp, f.err
}

func profileStoreFixture() fakeProfileStore {
	return fakeProfileStore{
		infos: []profiles.ProfileInfo{
			{Slug: "geral", Name: "Geral", Description: "Tarefas gerais"},
			{Slug: "custom", Name: "Custom", Description: "Análise especializada", Source: "home"},
		},
		bySlug: map[string]*profiles.Profile{
			"geral":  {Name: "Geral"},
			"custom": {Name: "Custom"},
		},
	}
}

func realProfileManager(t *testing.T) *profiles.Manager {
	t.Helper()
	tempDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("HOME", tempDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("USERPROFILE", tempDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	configdir.ResetForTests()
	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("USERPROFILE", oldUserProfile)
		configdir.ResetForTests()
	})
	manager := profiles.NewManager()
	if err := manager.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	return manager
}

func preparedProfileMutation(t *testing.T, manager *profiles.Manager, operation, slug string, profile *profiles.Profile) (*profiles.CommandMutation, string) {
	t.Helper()
	fingerprint, err := manager.CommandMutationSnapshot(slug)
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := manager.PrepareCommandMutation(operation, slug, profile, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	return mutation, fingerprint
}

func deletionProfileFixture(t *testing.T, name string) (*profiles.Manager, string) {
	t.Helper()
	manager := realProfileManager(t)
	anchor := profiles.DefaultProfile()
	anchor.Name = "Âncora " + name
	anchor.Active = true
	if _, err := manager.Create(anchor); err != nil {
		t.Fatal(err)
	}
	profile := profiles.DefaultProfile()
	profile.Name = name
	profile.Active = false
	slug, err := manager.Create(profile)
	if err != nil {
		t.Fatal(err)
	}
	return manager, slug
}

func profileFilePath(t *testing.T, manager *profiles.Manager, slug string) string {
	t.Helper()
	for _, root := range manager.GetSearchPaths() {
		path := filepath.Join(root, slug+".json")
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	t.Fatalf("arquivo do profile %q não encontrado nas raízes %#v", slug, manager.GetSearchPaths())
	return ""
}

func TestListIncludesCustomDescriptionsAndCurrentProfile(t *testing.T) {
	service := NewService(profileStoreFixture(), nil, nil, func(_ context.Context, profile *profiles.Profile) bool {
		return profile.Name != "Custom"
	})

	items, err := service.List(t.Context(), "custom")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("profiles = %#v", items)
	}
	if !items[1].Current || items[1].Description != "Análise especializada" || items[1].Available {
		t.Fatalf("profile custom inesperado: %#v", items[1])
	}
}

func TestAuthorizeCrossProfileUsesDecisionDialog(t *testing.T) {
	asker := &fakeAsker{resp: questionnaire.Response{
		Answers: map[string]any{questionnaire.AnswerActionID: ActionAllow},
	}}
	service := NewService(
		profileStoreFixture(),
		asker,
		func(context.Context, string, string) questionnaire.Surface {
			return questionnaire.DesktopSurface("conversation-1")
		},
		func(context.Context, *profiles.Profile) bool { return true },
	)

	allowed, err := service.Authorize(t.Context(), AuthorizationRequest{
		Source:         "wails",
		ConversationID: "conversation-1",
		CurrentSlug:    "geral",
		TargetSlug:     "custom",
		TaskTitle:      "Analise os dados",
		Background:     true,
	})
	if err != nil || !allowed {
		t.Fatalf("allowed=%v err=%v", allowed, err)
	}
	if asker.calls != 1 || asker.payload.Kind != questionnaire.KindDecision {
		t.Fatalf("payload de decisão não emitido: %#v", asker.payload)
	}
	if asker.payload.Description.Key != "app.questionnaire.subagentProfile.descriptionBackground" {
		t.Fatalf("modo background não foi descrito: %#v", asker.payload.Description)
	}
	if asker.payload.Body != "Analise os dados" || len(asker.payload.Actions) != 2 ||
		asker.payload.Actions[0].ID != ActionAllow || asker.payload.Actions[1].ID != ActionDeny {
		t.Fatalf("payload inesperado: %#v", asker.payload)
	}
}

func TestAuthorizeSameProfileDoesNotAsk(t *testing.T) {
	asker := &fakeAsker{}
	service := NewService(profileStoreFixture(), asker, nil, nil)
	allowed, err := service.Authorize(t.Context(), AuthorizationRequest{
		CurrentSlug: "geral",
		TargetSlug:  "geral",
	})
	if err != nil || !allowed || asker.calls != 0 {
		t.Fatalf("same-profile deveria passar sem diálogo: allowed=%v calls=%d err=%v", allowed, asker.calls, err)
	}
}

func TestAuthorizeFailsClosedWithoutInterlocutor(t *testing.T) {
	service := NewService(
		profileStoreFixture(),
		&fakeAsker{},
		func(context.Context, string, string) questionnaire.Surface {
			return questionnaire.NoSurface("conversation-1")
		},
		nil,
	)
	allowed, err := service.Authorize(t.Context(), AuthorizationRequest{
		CurrentSlug:    "geral",
		TargetSlug:     "custom",
		ConversationID: "conversation-1",
	})
	if allowed || !errors.Is(err, questionnaire.ErrNoInterlocutor) {
		t.Fatalf("esperava fail-closed: allowed=%v err=%v", allowed, err)
	}
}

func TestAuthorizeJobUsesExactGrantWithoutSurface(t *testing.T) {
	grants := &fakeJobGrants{valid: true, configs: []jobprofilegrant.DelegationConfig{{
		JobID: "job-db", Fingerprint: "fingerprint",
	}}}
	asker := &fakeAsker{}
	service := NewService(profileStoreFixture(), asker, func(context.Context, string, string) questionnaire.Surface {
		t.Fatal("SurfaceResolver não pode ser chamado para job")
		return questionnaire.Surface{}
	}, func(context.Context, *profiles.Profile) bool { return true }).WithJobGrants(grants)
	ctx := database.WithUserID(context.Background(), "user-a")
	ctx = eventctx.With(ctx, eventctx.Provenance{Source: "job", SourceJobID: "job-db"})
	allowed, err := service.Authorize(ctx, AuthorizationRequest{TargetSlug: "custom"})
	if err != nil || !allowed || asker.calls != 0 {
		t.Fatalf("grant válido deveria liberar sem diálogo: allowed=%v calls=%d err=%v", allowed, asker.calls, err)
	}
}

func TestAuthorizeJobWithoutGrantFailsBeforeSurface(t *testing.T) {
	grants := &fakeJobGrants{configs: []jobprofilegrant.DelegationConfig{{JobID: "job-db", Fingerprint: "fp"}}}
	asker := &fakeAsker{}
	service := NewService(profileStoreFixture(), asker, func(context.Context, string, string) questionnaire.Surface {
		t.Fatal("SurfaceResolver não pode ser chamado para job sem grant")
		return questionnaire.Surface{}
	}, nil).WithJobGrants(grants)
	ctx := database.WithUserID(context.Background(), "user-a")
	ctx = eventctx.With(ctx, eventctx.Provenance{Source: "job", SourceJobID: "job-db"})
	allowed, err := service.Authorize(ctx, AuthorizationRequest{TargetSlug: "custom"})
	if allowed || !errors.Is(err, ErrAuthorizationNotGranted) || asker.calls != 0 {
		t.Fatalf("esperava fail-closed sem diálogo: allowed=%v calls=%d err=%v", allowed, asker.calls, err)
	}
}

func TestAuthorizeJobPreservesCancellationAndOperationalGrantErrors(t *testing.T) {
	ctx := database.WithUserID(context.Background(), "user-a")
	ctx = eventctx.With(ctx, eventctx.Provenance{Source: "job", SourceJobID: "job-db"})
	grants := &fakeJobGrants{currentErr: context.Canceled}
	service := NewService(profileStoreFixture(), &fakeAsker{}, nil, nil).WithJobGrants(grants)
	if _, err := service.Authorize(ctx, AuthorizationRequest{TargetSlug: "custom"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelamento foi convertido em autorização negada: %v", err)
	}

	storeErr := errors.New("SQLite indisponível")
	grants.currentErr = storeErr
	if _, err := service.Authorize(ctx, AuthorizationRequest{TargetSlug: "custom"}); !errors.Is(err, ErrGrantStoreUnavailable) || errors.Is(err, ErrAuthorizationNotGranted) {
		t.Fatalf("erro operacional foi classificado incorretamente: %v", err)
	}

	grants.currentErr = nil
	grants.configs = []jobprofilegrant.DelegationConfig{{JobID: "job-db", Fingerprint: "fp"}}
	grants.validErr = context.DeadlineExceeded
	if _, err := service.Authorize(ctx, AuthorizationRequest{TargetSlug: "custom"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("deadline de HasValid foi perdida: %v", err)
	}
}

func TestAuthorizeJobTargetPersistsOnlyDesktopApproval(t *testing.T) {
	config := jobprofilegrant.DelegationConfig{JobID: "job-db", JobName: "Resumo diário", ProfileExpression: "custom", Fingerprint: "fp"}
	grants := &fakeJobGrants{configs: []jobprofilegrant.DelegationConfig{config, config}}
	asker := &fakeAsker{resp: questionnaire.Response{Answers: map[string]any{questionnaire.AnswerActionID: ActionAllow}}}
	service := NewService(profileStoreFixture(), asker, nil, nil).WithJobGrants(grants)
	allowed, err := service.AuthorizeJobTarget(context.Background(), questionnaire.DesktopSurface(""), "job-db", "custom")
	if err != nil || !allowed || grants.granted != 1 {
		t.Fatalf("aprovação desktop não persistiu: allowed=%v grants=%d err=%v", allowed, grants.granted, err)
	}

	grants.granted = 0
	if allowed, err := service.AuthorizeJobTarget(context.Background(), questionnaire.ChannelSurface("c", "telegram", "u"), "job-db", "custom"); allowed || !errors.Is(err, questionnaire.ErrNoInterlocutor) || grants.granted != 0 {
		t.Fatalf("canal não pode conceder: allowed=%v grants=%d err=%v", allowed, grants.granted, err)
	}
}

func TestJobGrantStateExposesPublicSlug(t *testing.T) {
	grants := &fakeJobGrants{configs: []jobprofilegrant.DelegationConfig{{
		JobID: "uuid-interno", JobSlug: "job-publico", JobName: "Job",
		ProfileExpression: "custom", Fingerprint: "fp",
	}}}
	service := NewService(profileStoreFixture(), nil, nil, nil).WithJobGrants(grants)
	state, err := service.JobGrantState(context.Background(), "job-publico")
	if err != nil {
		t.Fatal(err)
	}
	if state.JobID != "job-publico" || state.JobSlug != "job-publico" {
		t.Fatalf("DTO expôs identidade interna: %#v", state)
	}
}

func TestAuthorizeJobTargetRevalidatesTOCTOUAndDenial(t *testing.T) {
	before := jobprofilegrant.DelegationConfig{JobID: "job-db", JobName: "Job", ProfileExpression: "custom", Fingerprint: "before"}
	after := before
	after.Fingerprint = "after"
	grants := &fakeJobGrants{configs: []jobprofilegrant.DelegationConfig{before, after}}
	asker := &fakeAsker{resp: questionnaire.Response{Answers: map[string]any{questionnaire.AnswerActionID: ActionAllow}}}
	service := NewService(profileStoreFixture(), asker, nil, nil).WithJobGrants(grants)
	if allowed, err := service.AuthorizeJobTarget(context.Background(), questionnaire.DesktopSurface(""), "job-db", "custom"); allowed || err == nil || grants.granted != 0 {
		t.Fatalf("mudança concorrente deveria impedir grant: allowed=%v grants=%d err=%v", allowed, grants.granted, err)
	}

	grants.configs = []jobprofilegrant.DelegationConfig{before}
	asker.resp = questionnaire.Response{Answers: map[string]any{questionnaire.AnswerActionID: ActionDeny}}
	if allowed, err := service.AuthorizeJobTarget(context.Background(), questionnaire.DesktopSurface(""), "job-db", "custom"); err != nil || allowed || grants.granted != 0 {
		t.Fatalf("recusa não pode conceder: allowed=%v grants=%d err=%v", allowed, grants.granted, err)
	}
}

func TestAuthorizeJobTargetDoesNotUndoRevocationDuringDialog(t *testing.T) {
	store := profileStoreFixture()
	config := jobprofilegrant.DelegationConfig{
		JobID: "job-db", JobName: "Job", Tool: "subagent",
		ProfileExpression: "{{ .event.profile }}", Fingerprint: "fp",
	}
	grants := &fakeJobGrants{configs: []jobprofilegrant.DelegationConfig{config, config}}
	asker := &fakeAsker{
		resp: questionnaire.Response{Answers: map[string]any{questionnaire.AnswerActionID: ActionAllow}},
		onAsk: func() {
			_ = grants.Revoke(context.Background(), "job-db", "custom", "desktop")
		},
	}
	service := NewService(&store, asker, nil, nil).WithJobGrants(grants)
	allowed, err := service.AuthorizeJobTarget(
		context.Background(), questionnaire.DesktopSurface(""), "job-db", "custom",
	)
	if allowed || !errors.Is(err, jobprofilegrant.ErrGrantGenerationChanged) || grants.granted != 0 {
		t.Fatalf("revogação concorrente deveria vencer: allowed=%v grants=%d err=%v", allowed, grants.granted, err)
	}
}

func TestAuthorizeJobTargetRevalidatesSessionAfterDialog(t *testing.T) {
	store := profileStoreFixture()
	config := jobprofilegrant.DelegationConfig{
		JobID: "job-db", JobName: "Job", ProfileExpression: "custom", Fingerprint: "fp",
	}
	grants := &fakeJobGrants{configs: []jobprofilegrant.DelegationConfig{config, config}}
	sessionCurrent := true
	asker := &fakeAsker{
		resp: questionnaire.Response{Answers: map[string]any{questionnaire.AnswerActionID: ActionAllow}},
		onAsk: func() {
			sessionCurrent = false
		},
	}
	service := NewService(store, asker, nil, nil).
		WithJobGrants(grants).
		WithSessionValidator(func(context.Context) error {
			if !sessionCurrent {
				return errors.New("sessão mudou")
			}
			return nil
		})
	allowed, err := service.AuthorizeJobTarget(
		context.Background(), questionnaire.DesktopSurface(""), "job-db", "custom",
	)
	if allowed || err == nil || grants.granted != 0 {
		t.Fatalf("troca de sessão deveria invalidar decisão: allowed=%v grants=%d err=%v", allowed, grants.granted, err)
	}
}

func TestAuthorizeJobTargetRejectsDifferentLiteralBeforeDialog(t *testing.T) {
	config := jobprofilegrant.DelegationConfig{
		JobID: "job-db", JobName: "Job", ProfileExpression: "geral", Fingerprint: "fp",
	}
	grants := &fakeJobGrants{configs: []jobprofilegrant.DelegationConfig{config}}
	asker := &fakeAsker{}
	service := NewService(profileStoreFixture(), asker, nil, nil).WithJobGrants(grants)
	allowed, err := service.AuthorizeJobTarget(context.Background(), questionnaire.DesktopSurface(""), "job-db", "custom")
	if allowed || err == nil || asker.calls != 0 || grants.granted != 0 {
		t.Fatalf("target diferente do literal deveria falhar antes do diálogo: allowed=%v calls=%d grants=%d err=%v",
			allowed, asker.calls, grants.granted, err)
	}
}

func TestDeleteProfileRevokesGloballyWithoutUserContext(t *testing.T) {
	grants := &fakeJobGrants{}
	service := NewService(profileStoreFixture(), nil, nil, nil).WithJobGrants(grants)
	deleted := false
	err := service.DeleteProfile(context.Background(), "custom", func() error {
		deleted = true
		return nil
	})
	if err != nil || !deleted || grants.revoked != 1 {
		t.Fatalf("exclusão global deveria revogar e apagar sem sessão: deleted=%v revoked=%d err=%v",
			deleted, grants.revoked, err)
	}
}

func TestDeleteProfileFailureDoesNotRevokeGrants(t *testing.T) {
	grants := &fakeJobGrants{}
	service := NewService(profileStoreFixture(), nil, nil, nil).WithJobGrants(grants)
	deleteErr := errors.New("falha de I/O")
	err := service.DeleteProfile(context.Background(), "custom", func() error {
		return deleteErr
	})
	if !errors.Is(err, deleteErr) || grants.revoked != 0 {
		t.Fatalf("exclusão falha não pode revogar grants: revoked=%d err=%v", grants.revoked, err)
	}
}

func TestDeleteProfileOutcomeUnknownKeepsRevocationIntent(t *testing.T) {
	grants := &fakeJobGrants{}
	service := NewService(profileStoreFixture(), nil, nil, nil).WithJobGrants(grants)
	err := service.DeleteProfile(context.Background(), "custom", func() error {
		return profiles.ErrCommandMutationOutcomeUnknown
	})
	if !errors.Is(err, profiles.ErrCommandMutationOutcomeUnknown) {
		t.Fatalf("erro de outcome desconhecido perdido: %v", err)
	}
	if grants.canceled != 0 {
		t.Fatalf("intenção incerta foi cancelada: %d", grants.canceled)
	}
}

func TestCommitProfileMutationDeleteCoordinatesSuccessAndPublishesAfterCommit(t *testing.T) {
	manager, slug := deletionProfileFixture(t, "Coordenado")
	grants := &fakeJobGrants{}
	service := NewService(manager, nil, nil, nil).WithJobGrants(grants)
	mutation, _ := preparedProfileMutation(t, manager, profiles.CommandMutationDelete, slug, nil)

	got, err := service.CommitProfileMutation(context.Background(), mutation)
	if err != nil || got != slug {
		t.Fatalf("commit = %q, err=%v", got, err)
	}
	if _, err := manager.Get(slug); err == nil {
		t.Fatal("profile excluído ainda está presente")
	}
	if grants.begun != 1 || grants.revoked != 1 || grants.canceled != 0 || grants.notified != 1 {
		t.Fatalf("coordenação de grants inesperada: begun=%d revoked=%d canceled=%d notified=%d", grants.begun, grants.revoked, grants.canceled, grants.notified)
	}
}

func TestCommitProfileMutationRejectsStaleCASAndABAWithoutGrantEffects(t *testing.T) {
	manager, slug := deletionProfileFixture(t, "CAS profile")
	profile := profiles.DefaultProfile()
	profile.Name = "CAS profile"
	profile.Active = false
	current, err := manager.Get(slug)
	if err != nil {
		t.Fatal(err)
	}
	profile = current
	grants := &fakeJobGrants{}
	service := NewService(manager, nil, nil, nil).WithJobGrants(grants)
	mutation, _ := preparedProfileMutation(t, manager, profiles.CommandMutationDelete, slug, nil)
	changed := *profile
	changed.Description = "mudança concorrente"
	if err := manager.Update(slug, &changed); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitProfileMutation(context.Background(), mutation); !errors.Is(err, profiles.ErrStaleCommandMutation) {
		t.Fatalf("CAS não recusou mutação obsoleta: %v", err)
	}
	if grants.begun != 0 || grants.revoked != 0 || grants.canceled != 0 {
		t.Fatalf("CAS obsoleto produziu efeitos de grant: %#v", grants)
	}

	original := changed
	original.Description = profile.Description
	if err := manager.Update(slug, &original); err != nil {
		t.Fatal(err)
	}
	abaMutation, _ := preparedProfileMutation(t, manager, profiles.CommandMutationDelete, slug, nil)
	aba := original
	aba.Description = "ABA intermediário"
	if err := manager.Update(slug, &aba); err != nil {
		t.Fatal(err)
	}
	if err := manager.Update(slug, &original); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitProfileMutation(context.Background(), abaMutation); !errors.Is(err, profiles.ErrStaleCommandMutation) {
		t.Fatalf("ABA não foi recusado: %v", err)
	}
}

func TestCommitProfileMutationRejectsActiveProfileBeforeGrantIntent(t *testing.T) {
	manager, slug := deletionProfileFixture(t, "Ativo coordenado")
	grants := &fakeJobGrants{}
	service := NewService(manager, nil, nil, nil).WithJobGrants(grants)
	mutation, _ := preparedProfileMutation(t, manager, profiles.CommandMutationDelete, slug, nil)
	if err := manager.SetActive(slug); err != nil {
		t.Fatal(err)
	}
	if _, err := service.CommitProfileMutation(context.Background(), mutation); !errors.Is(err, profiles.ErrStaleCommandMutation) {
		t.Fatalf("mutação deveria ser recusada pelo CAS após ativação concorrente: %v", err)
	}
	if grants.begun != 0 || grants.revoked != 0 {
		t.Fatalf("recusa de preparação produziu efeitos: %#v", grants)
	}
}

func TestCommitProfileMutationGrantFailureRollsBackAndCancelsIntent(t *testing.T) {
	manager, slug := deletionProfileFixture(t, "Falha grant")
	revokeErr := errors.New("SQLite indisponível")
	grants := &fakeJobGrants{revokeErr: revokeErr}
	service := NewService(manager, nil, nil, nil).WithJobGrants(grants)
	mutation, _ := preparedProfileMutation(t, manager, profiles.CommandMutationDelete, slug, nil)

	if _, err := service.CommitProfileMutation(context.Background(), mutation); !errors.Is(err, profiles.ErrCommandMutationRolledBack) || !errors.Is(err, revokeErr) {
		t.Fatalf("falha de grant não reportou rollback: %v", err)
	}
	if restored, err := manager.Get(slug); err != nil || restored == nil {
		t.Fatalf("rollback não restaurou profile: profile=%#v err=%v", restored, err)
	}
	if grants.begun != 1 || grants.revoked != 1 || grants.canceled != 1 || grants.notified != 0 {
		t.Fatalf("efeitos após falha de grant inesperados: %#v", grants)
	}
}

func TestCommitProfileMutationInitialIntentFailureKeepsUnknownOutcome(t *testing.T) {
	manager, slug := deletionProfileFixture(t, "Falha intenção")
	beginErr := errors.New("falha ao gravar intenção")
	grants := &fakeJobGrants{beginErr: beginErr}
	service := NewService(manager, nil, nil, nil).WithJobGrants(grants)
	mutation, _ := preparedProfileMutation(t, manager, profiles.CommandMutationDelete, slug, nil)

	if _, err := service.CommitProfileMutation(context.Background(), mutation); !errors.Is(err, beginErr) {
		t.Fatalf("falha inicial não foi propagada: %v", err)
	}
	if grants.canceled != 0 || grants.revoked != 0 {
		t.Fatalf("falha inicial cancelou ou revogou indevidamente: %#v", grants)
	}
	if _, err := manager.Get(slug); err != nil {
		t.Fatalf("arquivo foi alterado apesar da falha inicial: %v", err)
	}
}

func TestCommitProfileMutationJournalFailureKeepsIntentWithoutNotification(t *testing.T) {
	manager, slug := deletionProfileFixture(t, "Falha journal")
	profilePath := profileFilePath(t, manager, slug)
	journalPath := filepath.Join(filepath.Dir(profilePath), ".profile-mutation.journal")
	grants := &fakeJobGrants{}
	grants.beginHook = func() {
		if err := os.Mkdir(journalPath, 0755); err != nil {
			t.Fatalf("bloquear journal: %v", err)
		}
	}
	service := NewService(manager, nil, nil, nil).WithJobGrants(grants)
	mutation, _ := preparedProfileMutation(t, manager, profiles.CommandMutationDelete, slug, nil)

	if _, err := service.CommitProfileMutation(context.Background(), mutation); !errors.Is(err, profiles.ErrCommandMutationOutcomeUnknown) {
		t.Fatalf("falha de journal não foi incerta: %v", err)
	}
	if grants.canceled != 0 || grants.revoked != 0 || grants.notified != 0 {
		t.Fatalf("falha de journal publicou/cancelou indevidamente: %#v", grants)
	}
	if _, err := os.Stat(profilePath); err != nil {
		t.Fatalf("arquivo do profile mudou apesar da falha de journal: %v", err)
	}
	if info, err := os.Stat(journalPath); err != nil || !info.IsDir() {
		t.Fatalf("sentinela do journal não permaneceu: info=%#v err=%v", info, err)
	}
}

func TestCommitProfileMutationRollbackFailureKeepsIntent(t *testing.T) {
	manager, slug := deletionProfileFixture(t, "Falha rollback")
	profilePath := profileFilePath(t, manager, slug)
	revokeErr := errors.New("falha de revogação")
	grants := &fakeJobGrants{revokeErr: revokeErr}
	grants.revokeHook = func() {
		if err := os.WriteFile(profilePath, []byte("estado externo"), 0644); err != nil {
			t.Fatalf("bloquear rollback: %v", err)
		}
	}
	service := NewService(manager, nil, nil, nil).WithJobGrants(grants)
	mutation, _ := preparedProfileMutation(t, manager, profiles.CommandMutationDelete, slug, nil)

	if _, err := service.CommitProfileMutation(context.Background(), mutation); !errors.Is(err, profiles.ErrCommandMutationOutcomeUnknown) {
		t.Fatalf("falha de rollback não foi incerta: %v", err)
	}
	if grants.canceled != 0 || grants.notified != 0 {
		t.Fatalf("falha de rollback cancelou/publicou indevidamente: %#v", grants)
	}
	if data, err := os.ReadFile(profilePath); err != nil || string(data) != "estado externo" {
		t.Fatalf("estado externo foi sobrescrito: data=%q err=%v", data, err)
	}
}

func TestCommitProfileMutationRejectsNilAndCanceledContextBeforeEffects(t *testing.T) {
	manager, slug := deletionProfileFixture(t, "Contexto")
	grants := &fakeJobGrants{}
	service := NewService(manager, nil, nil, nil).WithJobGrants(grants)
	mutation, _ := preparedProfileMutation(t, manager, profiles.CommandMutationDelete, slug, nil)
	if _, err := service.CommitProfileMutation(nil, mutation); err == nil {
		t.Fatal("contexto nulo deveria ser recusado")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.CommitProfileMutation(ctx, mutation); !errors.Is(err, context.Canceled) {
		t.Fatalf("contexto cancelado não foi recusado antes dos efeitos: %v", err)
	}
	if grants.begun != 0 || grants.revoked != 0 || grants.canceled != 0 {
		t.Fatalf("contexto cancelado produziu efeitos: %#v", grants)
	}
	if _, err := manager.Get(slug); err != nil {
		t.Fatalf("profile mudou com contexto cancelado: %v", err)
	}
}

func TestAuthorizeJobTargetInvalidatedByCommittedProfileEpoch(t *testing.T) {
	manager, slug := deletionProfileFixture(t, "Epoch")
	updated, err := manager.Get(slug)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := manager.CommandMutationSnapshot(slug)
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := manager.PrepareCommandMutation(profiles.CommandMutationUpdate, slug, updated, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	config := jobprofilegrant.DelegationConfig{JobID: "job-db", JobName: "Job", ProfileExpression: slug, Fingerprint: "fp"}
	grants := &fakeJobGrants{configs: []jobprofilegrant.DelegationConfig{config, config, config}}
	asker := &fakeAsker{resp: questionnaire.Response{Answers: map[string]any{questionnaire.AnswerActionID: ActionAllow}}}
	service := NewService(manager, asker, nil, nil).WithJobGrants(grants)
	asker.onAsk = func() {
		if _, err := service.CommitProfileMutation(context.Background(), mutation); err != nil {
			t.Errorf("update concorrente: %v", err)
		}
	}
	allowed, err := service.AuthorizeJobTarget(context.Background(), questionnaire.DesktopSurface(""), "job-db", slug)
	if allowed || err == nil || grants.granted != 0 {
		t.Fatalf("epoch do profile não invalidou autorização pendente: allowed=%v granted=%d err=%v", allowed, grants.granted, err)
	}
}

func TestDeleteProfileRestoresFileWhenGrantRevocationFails(t *testing.T) {
	store := profileStoreFixture()
	revokeErr := errors.New("SQLite indisponível")
	grants := &fakeJobGrants{revokeErr: revokeErr}
	service := NewService(store, nil, nil, nil).WithJobGrants(grants)
	err := service.DeleteProfile(context.Background(), "custom", func() error {
		delete(store.bySlug, "custom")
		return nil
	})
	if !errors.Is(err, revokeErr) || grants.revoked != 1 {
		t.Fatalf("falha de revogação deveria ser propagada: revoked=%d err=%v", grants.revoked, err)
	}
	if restored, getErr := store.Get("custom"); getErr != nil || restored == nil {
		t.Fatalf("profile deveria ser restaurado após falha no SQLite: profile=%#v err=%v", restored, getErr)
	}
}

func TestAuthorizeJobTargetRejectsProfileRemovedAndRecreatedDuringDialog(t *testing.T) {
	store := profileStoreFixture()
	config := jobprofilegrant.DelegationConfig{
		JobID: "job-db", JobName: "Job", ProfileExpression: "custom", Fingerprint: "fp",
	}
	grants := &fakeJobGrants{configs: []jobprofilegrant.DelegationConfig{config}}
	var service *Service
	asker := &fakeAsker{resp: questionnaire.Response{
		Answers: map[string]any{questionnaire.AnswerActionID: ActionAllow},
	}}
	service = NewService(store, asker, nil, nil).WithJobGrants(grants)
	asker.onAsk = func() {
		if err := service.DeleteProfile(context.Background(), "custom", func() error {
			delete(store.bySlug, "custom")
			store.bySlug["custom"] = &profiles.Profile{Name: "Custom recriado"}
			return nil
		}); err != nil {
			t.Errorf("delete profile: %v", err)
		}
	}
	allowed, err := service.AuthorizeJobTarget(context.Background(), questionnaire.DesktopSurface(""), "job-db", "custom")
	if allowed || err == nil || grants.granted != 0 {
		t.Fatalf("profile recriado durante diálogo não pode receber grant: allowed=%v grants=%d err=%v",
			allowed, grants.granted, err)
	}
}

func TestAuthorizeJobTargetSharesLockWithProfileUpdates(t *testing.T) {
	store := profileStoreFixture()
	config := jobprofilegrant.DelegationConfig{
		JobID: "job-db", JobName: "Job", ProfileExpression: "custom", Fingerprint: "fp",
	}
	grants := &fakeJobGrants{configs: []jobprofilegrant.DelegationConfig{config, config}}
	var service *Service
	asker := &fakeAsker{resp: questionnaire.Response{
		Answers: map[string]any{questionnaire.AnswerActionID: ActionAllow},
	}}
	service = NewService(store, asker, nil, nil).WithJobGrants(grants)
	asker.onAsk = func() {
		if err := service.MutateProfiles(func() error {
			store.bySlug["custom"].Description = "configuração alterada"
			return nil
		}); err != nil {
			t.Errorf("update profile: %v", err)
		}
	}
	allowed, err := service.AuthorizeJobTarget(
		context.Background(), questionnaire.DesktopSurface(""), "job-db", "custom",
	)
	if allowed || err == nil || grants.granted != 0 {
		t.Fatalf("profile editado durante diálogo não pode receber grant: allowed=%v grants=%d err=%v",
			allowed, grants.granted, err)
	}
}
