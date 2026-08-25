package profileaccess

import (
	"context"
	"errors"
	"testing"

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
}

func (f *fakeAsker) Ask(_ context.Context, _ questionnaire.Surface, payload questionnaire.RequestPayload) (questionnaire.Response, error) {
	f.calls++
	f.payload = payload
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
