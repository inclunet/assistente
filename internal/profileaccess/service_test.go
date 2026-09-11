package profileaccess

import (
	"context"
	"errors"
	"testing"

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
}

func (f *fakeJobGrants) AuthorizationSnapshot(ctx context.Context, jobID, _ string) (jobprofilegrant.AuthorizationSnapshot, error) {
	config, err := f.CurrentDelegation(ctx, jobID)
	return jobprofilegrant.AuthorizationSnapshot{Config: config, Generation: f.generation}, err
}

func (f *fakeJobGrants) CurrentDelegation(_ context.Context, _ string) (jobprofilegrant.DelegationConfig, error) {
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
	return f.valid, nil
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
func (f *fakeJobGrants) RevokeProfileGlobal(context.Context, string, string) error {
	f.revoked++
	return nil
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
