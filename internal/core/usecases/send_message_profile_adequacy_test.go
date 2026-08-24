package usecases

import (
	"context"
	"errors"
	"os"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/configdir"
	"assistente/internal/core/ports"
	"assistente/internal/llm"
	"assistente/internal/profileadequacy"
	"assistente/internal/profiles"
	"assistente/internal/questionnaire"
)

type adequacyMessageRepo struct {
	chat.MessageRepository
	messages []chat.Message
	err      error
}

func (r adequacyMessageRepo) GetMessages(context.Context, string, *string) ([]chat.Message, error) {
	return r.messages, r.err
}

type fixedProfileAdvisor struct {
	recommendation *profileadequacy.Recommendation
	err            error
	calls          int
}

func (a *fixedProfileAdvisor) Recommend(context.Context, profileadequacy.Request) (*profileadequacy.Recommendation, error) {
	a.calls++
	return a.recommendation, a.err
}

type fixedQuestionnaireRouter struct {
	response questionnaire.Response
	err      error
	calls    int
	payload  questionnaire.RequestPayload
}

type adequacyEmitter struct {
	event string
	data  any
}

func (e *adequacyEmitter) Emit(event string, data any) {
	e.event = event
	e.data = data
}

func (r *fixedQuestionnaireRouter) Ask(
	_ context.Context,
	_ questionnaire.Surface,
	payload questionnaire.RequestPayload,
) (questionnaire.Response, error) {
	r.calls++
	r.payload = payload
	return r.response, r.err
}

func TestEnsureAdequateProfileContinuePreservesCurrentProfile(t *testing.T) {
	advisor := &fixedProfileAdvisor{recommendation: adequacyRecommendation()}
	router := &fixedQuestionnaireRouter{response: questionnaire.Response{
		Answers: map[string]any{questionnaire.AnswerActionID: profileAdequacyContinueAction},
	}}
	switchCalls := 0
	uc := adequacyUseCase(advisor, router, nil, func(string, string, string) error {
		switchCalls++
		return nil
	})
	current := adequacyPreparedContext()

	got, err := uc.ensureAdequateProfile(t.Context(), adequacyRequest(), current)
	if err != nil {
		t.Fatal(err)
	}
	if got != current {
		t.Fatal("continuar deveria preservar o contexto e o profile atuais")
	}
	if advisor.calls != 1 || router.calls != 1 || switchCalls != 0 {
		t.Fatalf("calls advisor=%d router=%d switch=%d", advisor.calls, router.calls, switchCalls)
	}
	if router.payload.Kind != questionnaire.KindDecision ||
		len(router.payload.Actions) != 2 ||
		router.payload.Actions[0].ID != profileAdequacySwitchAction ||
		!router.payload.Actions[0].Primary ||
		router.payload.Actions[1].ID != profileAdequacyContinueAction {
		t.Fatalf("contrato de decisão inesperado: %#v", router.payload)
	}
}

func TestEnsureAdequateProfileCancelStopsBeforeSwitch(t *testing.T) {
	advisor := &fixedProfileAdvisor{recommendation: adequacyRecommendation()}
	router := &fixedQuestionnaireRouter{response: questionnaire.Response{Cancelled: true}}
	switchCalls := 0
	uc := adequacyUseCase(advisor, router, nil, func(string, string, string) error {
		switchCalls++
		return nil
	})

	current := adequacyPreparedContext()
	got, err := uc.ensureAdequateProfile(t.Context(), adequacyRequest(), current)
	if !errors.Is(err, errProfileAdequacyCancelled) || got != current {
		t.Fatalf("cancelamento deveria interromper: got=%#v err=%v", got, err)
	}
	if switchCalls != 0 {
		t.Fatalf("cancelamento não pode trocar profile: %d", switchCalls)
	}
}

func TestProfileAdequacyCancellationCompletesWithoutSendFailure(t *testing.T) {
	emitter := &adequacyEmitter{}
	uc := NewSendMessageUseCase(SendMessageConfig{Emitter: emitter})
	conversationID := uc.completeProfileAdequacyCancellation(adequacyRequest(), adequacyPreparedContext())

	if conversationID != "conversation-1" || emitter.event != "chat:done" {
		t.Fatalf("cancelamento = conversa %q evento %q", conversationID, emitter.event)
	}
	done, ok := emitter.data.(ports.DoneEvent)
	if !ok || done.Reason != "cancelled" || done.ErrorMessage != "" {
		t.Fatalf("evento terminal de cancelamento inesperado: %#v", emitter.data)
	}
}

func TestEnsureAdequateProfileSwitchesTabAndPreparesSuggestedProfile(t *testing.T) {
	advisor := &fixedProfileAdvisor{recommendation: adequacyRecommendation()}
	router := &fixedQuestionnaireRouter{response: questionnaire.Response{
		Answers: map[string]any{questionnaire.AnswerActionID: profileAdequacySwitchAction},
	}}
	var switchedTab, switchedProfile string
	uc := adequacyUseCase(advisor, router, nil, func(tabID, conversationID, profileSlug string) error {
		if conversationID != "conversation-1" {
			t.Fatalf("conversationID = %q", conversationID)
		}
		switchedTab, switchedProfile = tabID, profileSlug
		return nil
	})
	uc.chatInteractor = chat.NewInteractor(chat.InteractorConfig{
		Repo:       adequacyMessageRepo{},
		ProfileMgr: isolatedProfileManager(t),
	})
	request := adequacyRequest()
	request.UserContent = ""

	got, err := uc.ensureAdequateProfile(t.Context(), request, adequacyPreparedContext())
	if err != nil {
		t.Fatal(err)
	}
	if switchedTab != "tab-1" || switchedProfile != "programacao" {
		t.Fatalf("troca = tab %q profile %q", switchedTab, switchedProfile)
	}
	if got == nil || got.Params.ProfileSlug != "programacao" || got.ActiveProfile == nil {
		t.Fatalf("contexto não foi refeito com profile sugerido: %#v", got)
	}
}

func isolatedProfileManager(t *testing.T) *profiles.Manager {
	t.Helper()
	tempDir := t.TempDir()
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	configdir.ResetForTests()
	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
		configdir.ResetForTests()
	})

	manager := profiles.NewManager()
	profile := profiles.DefaultProfile()
	profile.Name = "Programação"
	profile.Active = false
	slug, err := manager.Create(profile)
	if err != nil {
		t.Fatal(err)
	}
	if slug != "programacao" {
		t.Fatalf("slug do profile de teste = %q", slug)
	}
	return manager
}

func TestEnsureAdequateProfileSkipsConversationWithMessages(t *testing.T) {
	advisor := &fixedProfileAdvisor{recommendation: adequacyRecommendation()}
	router := &fixedQuestionnaireRouter{}
	uc := adequacyUseCase(advisor, router, []chat.Message{{Role: "user"}}, func(string, string, string) error {
		return errors.New("não deveria trocar")
	})
	current := adequacyPreparedContext()

	got, err := uc.ensureAdequateProfile(t.Context(), adequacyRequest(), current)
	if err != nil {
		t.Fatal(err)
	}
	if got != current || advisor.calls != 0 || router.calls != 0 {
		t.Fatalf("conversa não vazia não deveria abrir preflight: advisor=%d router=%d", advisor.calls, router.calls)
	}
}

func TestEnsureAdequateProfileSkipsRetryAndNonDesktopSource(t *testing.T) {
	for _, req := range []SendMessageRequest{
		func() SendMessageRequest {
			request := adequacyRequest()
			request.RetryMessageID = "message"
			return request
		}(),
		func() SendMessageRequest {
			request := adequacyRequest()
			request.Source = "telegram"
			return request
		}(),
	} {
		advisor := &fixedProfileAdvisor{recommendation: adequacyRecommendation()}
		router := &fixedQuestionnaireRouter{}
		uc := adequacyUseCase(advisor, router, nil, func(string, string, string) error { return nil })
		current := adequacyPreparedContext()

		got, err := uc.ensureAdequateProfile(t.Context(), req, current)
		if err != nil {
			t.Fatal(err)
		}
		if got != current || advisor.calls != 0 || router.calls != 0 {
			t.Fatalf("request %#v não deveria abrir preflight", req)
		}
	}
}

func TestConversationLockRespectsCancellationWhileWaiting(t *testing.T) {
	uc := NewSendMessageUseCase(SendMessageConfig{})
	unlock, err := uc.lockConversation(t.Context(), "conversation-1")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if secondUnlock, lockErr := uc.lockConversation(ctx, "conversation-1"); !errors.Is(lockErr, context.Canceled) || secondUnlock != nil {
		t.Fatalf("lock cancelado deveria devolver função nil e context.Canceled; função nil=%t err=%v", secondUnlock == nil, lockErr)
	}
	unlock()
}

func adequacyUseCase(
	advisor *fixedProfileAdvisor,
	router *fixedQuestionnaireRouter,
	messages []chat.Message,
	switchProfile func(string, string, string) error,
) *SendMessageUseCase {
	interactor := chat.NewInteractor(chat.InteractorConfig{
		Repo:       adequacyMessageRepo{messages: messages},
		ProfileMgr: profiles.NewManager(),
	})
	return NewSendMessageUseCase(SendMessageConfig{
		ChatInteractor:      interactor,
		ProfileAdvisor:      advisor,
		QuestionnaireRouter: router,
		SwitchTabProfile:    switchProfile,
	})
}

func adequacyPreparedContext() *chat.PrepareContextResponse {
	return &chat.PrepareContextResponse{
		ActiveProfile: &profiles.Profile{Name: "Padrão"},
		Params: llm.ChatParams{
			ProfileSlug:  "padrao",
			SurfaceTabID: "tab-1",
		},
		UserContent: "rode os testes",
	}
}

func adequacyRequest() SendMessageRequest {
	return SendMessageRequest{
		ConversationID: "conversation-1",
		UserContent:    "rode os testes",
		Source:         "wails",
		Params:         llm.ChatParams{SurfaceTabID: "tab-1"},
	}
}

func adequacyRecommendation() *profileadequacy.Recommendation {
	return &profileadequacy.Recommendation{
		CurrentSlug:   "padrao",
		CurrentName:   "Padrão",
		SuggestedSlug: "programacao",
		SuggestedName: "Programação",
		RequiredTools: []string{"run_command"},
	}
}
