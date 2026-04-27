package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/profiles"
	"gorm.io/gorm"
)

// spyEmitter captures emitted events for assertions.
type spyEmitter struct {
	emitted []emittedEvent
}

type emittedEvent struct {
	name string
	data any
}

func (s *spyEmitter) Emit(event string, data any) {
	s.emitted = append(s.emitted, emittedEvent{name: event, data: data})
}

func (s *spyEmitter) findError() *ports.ErrorEvent {
	for _, e := range s.emitted {
		if e.name == "chat:error" {
			if ev, ok := e.data.(ports.ErrorEvent); ok {
				return &ev
			}
		}
	}
	return nil
}

var _ events.Emitter = (*spyEmitter)(nil)

// noopConvRepo is a minimal ConversationRepository for tests.
type noopConvRepo struct{}

func (noopConvRepo) GetConversationInfo(_ string) (*Conversation, error) { return nil, nil }
func (noopConvRepo) UpdateConversation(_ string, _, _ string) error      { return nil }
func (noopConvRepo) UpdateConversationChannel(_ string, _, _ string) error {
	return nil
}

func newTestInteractor(em events.Emitter) *Interactor {
	return NewInteractor(InteractorConfig{
		Emitter:  em,
		ConvRepo: noopConvRepo{},
	})
}

type retryMessageRepoStub struct {
	getMessage func(messageID string) (*database.ChatMessage, error)
}

func (r *retryMessageRepoStub) CreateMessage(_ MessageOptions) (*Message, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) GetMessage(messageID string) (*Message, error) {
	if r.getMessage != nil {
		return r.getMessage(messageID)
	}
	return nil, nil
}

func (r *retryMessageRepoStub) GetMessages(_ string, _ *string) ([]Message, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) GetConversationSummary(_ string) (string, string, error) {
	return "", "", nil
}

func (r *retryMessageRepoStub) GetDetailedTokenStats(_ string, _ string) (*DetailedTokenStats, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) GetContextWindowUsage(_ string, _ int) (float64, int, error) {
	return 0, 0, nil
}

func (r *retryMessageRepoStub) GetRecentMessagesTokenCount(_ string, _ int) (int, error) {
	return 0, nil
}

func (r *retryMessageRepoStub) GetTurnTokenStats(_ string, _ string) (*database.TokenStats, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) AddAssistantToolMessage(_ string, _ string, _, _, _, _ string) (*Message, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) AddToolResultMessage(_ string, _ string, _, _ string) (*Message, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) SearchMessages(_ string, _ int) ([]MessageSearchResult, error) {
	return nil, nil
}

func setupProfileTestEnv(t *testing.T) *profiles.Manager {
	t.Helper()

	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)

	return profiles.NewManager()
}

func TestPrepareContext_RejectsContentExceedingMaxSize(t *testing.T) {
	spy := &spyEmitter{}
	inter := newTestInteractor(spy)

	bigContent := strings.Repeat("x", MaxMessageContentSize+1)

	_, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: "1",
		UserContent:    bigContent,
	})

	if err == nil {
		t.Fatal("expected error for content exceeding max size")
	}
	if !strings.Contains(err.Error(), "Mensagem muito grande") {
		t.Errorf("unexpected error: %q", err.Error())
	}

	ev := spy.findError()
	if ev == nil {
		t.Fatal("expected chat:error event to be emitted")
	}
	if ev.ConversationID != "1" {
		t.Errorf("expected conversationId=1, got %s", ev.ConversationID)
	}
}

func TestPrepareContext_AcceptsContentAtExactMaxSize(t *testing.T) {
	spy := &spyEmitter{}
	inter := newTestInteractor(spy)

	exactContent := strings.Repeat("x", MaxMessageContentSize)

	// Should fail after size check (at provider check), not AT the size check
	_, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: "1",
		UserContent:    exactContent,
	})

	// Should NOT fail with "Mensagem muito grande"
	if err != nil && strings.Contains(err.Error(), "Mensagem muito grande") {
		t.Errorf("content at exact max size should not be rejected: %q", err.Error())
	}
}

func TestPrepareContext_RejectsMediaExceedingMaxSize(t *testing.T) {
	spy := &spyEmitter{}
	inter := newTestInteractor(spy)

	bigMedia := strings.Repeat("x", MaxMediaSize+1)

	_, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: "1",
		UserContent:    "hello",
		UserMedia:      bigMedia,
	})

	if err == nil {
		t.Fatal("expected error for media exceeding max size")
	}
	if !strings.Contains(err.Error(), "Mídia muito grande") {
		t.Errorf("unexpected error: %q", err.Error())
	}

	ev := spy.findError()
	if ev == nil {
		t.Fatal("expected chat:error event to be emitted")
	}
}

func TestPrepareContext_RejectsConversationIDZero(t *testing.T) {
	spy := &spyEmitter{}
	inter := newTestInteractor(spy)

	_, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: "",
		UserContent:    "hello",
	})

	if err == nil {
		t.Fatal("expected error for conversationID=0")
	}
	if !strings.Contains(err.Error(), "conversationID") {
		t.Errorf("unexpected error: %q", err.Error())
	}

	ev := spy.findError()
	if ev == nil {
		t.Fatal("expected chat:error event to be emitted")
	}
	if ev.ConversationID != "" {
		t.Errorf("expected conversationId empty, got %s", ev.ConversationID)
	}
}

func TestGetRetryableUserMessage_ReturnsDomainErrorWhenMessageNotFound(t *testing.T) {
	interactor := NewInteractor(InteractorConfig{
		Repo: &retryMessageRepoStub{
			getMessage: func(_ string) (*database.ChatMessage, error) {
				return nil, gorm.ErrRecordNotFound
			},
		},
	})

	msg, err := interactor.GetRetryableUserMessage("7", "42")
	if msg != nil {
		t.Fatalf("expected nil message, got %+v", msg)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "mensagem não encontrada" {
		t.Fatalf("expected domain not found error, got %v", err)
	}
}

func TestGetRetryableUserMessage_ReturnsErrorWhenRepositoryIsUnavailable(t *testing.T) {
	interactor := NewInteractor(InteractorConfig{})

	msg, err := interactor.GetRetryableUserMessage("7", "42")
	if msg != nil {
		t.Fatalf("expected nil message, got %+v", msg)
	}
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "repositório de mensagens indisponível" {
		t.Fatalf("expected repository unavailable error, got %v", err)
	}
}

func TestPrepareContext_ProfileSlugInheritsProviderAndModelFromActiveProfile(t *testing.T) {
	spy := &spyEmitter{}
	profileMgr := setupProfileTestEnv(t)

	active := profiles.DefaultProfile()
	active.Name = "Padrão Ativo"
	active.Active = true
	active.Chat.LLMProvider = "provider-global"
	active.Chat.Model = "model-global"
	active.Voice.Assistant.LLMProviderID = "voice-global"
	active.Input.LLMProviderID = "stt-global"
	activeSlug, err := profileMgr.Create(active)
	if err != nil {
		t.Fatalf("create active profile: %v", err)
	}
	if err := profileMgr.SetActive(activeSlug); err != nil {
		t.Fatalf("set active profile: %v", err)
	}

	panel := profiles.DefaultProfile()
	panel.Name = "Perfil do Editor"
	panel.Active = false
	panel.Chat.LLMProvider = ""
	panel.Chat.Model = ""
	panel.Voice.Assistant.LLMProviderID = ""
	panel.Input.LLMProviderID = ""
	panel.Chat.Temperature = 0.2
	panelSlug := "perfil-editor-legado"
	panelData, err := json.Marshal(panel)
	if err != nil {
		t.Fatalf("marshal panel profile: %v", err)
	}
	if err := configdir.NewResolver("profiles").Create(panelSlug+".json", panelData); err != nil {
		t.Fatalf("write legacy panel profile: %v", err)
	}

	inter := NewInteractor(InteractorConfig{
		Emitter:     spy,
		ConvRepo:    noopConvRepo{},
		ProfileMgr:  profileMgr,
		ProviderSvc: nil,
	})

	resp, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: "1",
		UserContent:    "oi",
		Params: ChatParams{
			ProfileSlug: panelSlug,
		},
	})
	if err != nil {
		t.Fatalf("PrepareContext: %v", err)
	}
	if resp == nil || resp.ActiveProfile == nil {
		t.Fatal("expected activeProfile in response")
	}
	if resp.ActiveProfile.Chat.LLMProvider != "provider-global" {
		t.Fatalf("LLMProvider = %q, want provider-global", resp.ActiveProfile.Chat.LLMProvider)
	}
	if resp.ActiveProfile.Chat.Model != "model-global" {
		t.Fatalf("Model = %q, want model-global", resp.ActiveProfile.Chat.Model)
	}
	if resp.ActiveProfile.Voice.Assistant.LLMProviderID != "voice-global" {
		t.Fatalf("Voice.Assistant.LLMProviderID = %q, want voice-global", resp.ActiveProfile.Voice.Assistant.LLMProviderID)
	}
	if resp.ActiveProfile.Input.LLMProviderID != "stt-global" {
		t.Fatalf("Input.LLMProviderID = %q, want stt-global", resp.ActiveProfile.Input.LLMProviderID)
	}
	if resp.ActiveProfile.Chat.Temperature != 0.2 {
		t.Fatalf("Temperature = %v, want 0.2", resp.ActiveProfile.Chat.Temperature)
	}
}
