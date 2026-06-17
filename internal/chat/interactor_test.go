package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/contextprovider"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/skills"
	"assistente/internal/workspace"
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

func (s *spyEmitter) findSkillLoaded() *ports.SkillLoadedEvent {
	for _, e := range s.emitted {
		if e.name == "chat:skill_loaded" {
			if ev, ok := e.data.(ports.SkillLoadedEvent); ok {
				return &ev
			}
		}
	}
	return nil
}

var _ events.Emitter = (*spyEmitter)(nil)

// noopConvRepo is a minimal ConversationRepository for tests.
type noopConvRepo struct{}

func (noopConvRepo) GetConversationInfo(_ context.Context, _ string) (*Conversation, error) {
	return nil, nil
}
func (noopConvRepo) UpdateConversation(_ context.Context, _ string, _, _ string) error { return nil }
func (noopConvRepo) UpdateConversationChannel(_ context.Context, _ string, _, _ string) error {
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

type staticWorkspaceProvider struct {
	ws *workspace.Workspace
}

func (s staticWorkspaceProvider) Active() *workspace.Workspace {
	return s.ws
}

type staticSkillRuntimeManager struct {
	skills map[string]*skills.Skill
	files  []string
}

func (m staticSkillRuntimeManager) Get(slug string) (*skills.Skill, error) {
	if s, ok := m.skills[slug]; ok {
		return s, nil
	}
	return nil, gorm.ErrRecordNotFound
}

func (m staticSkillRuntimeManager) GetSkillFiles(slug string) ([]string, error) {
	return m.files, nil
}

func (m staticSkillRuntimeManager) GetAllSkillsFull() ([]skills.Skill, error) {
	result := make([]skills.Skill, 0, len(m.skills))
	for _, s := range m.skills {
		result = append(result, *s)
	}
	return result, nil
}

type capturingPromptBuilder struct {
	slashSkillContent string
	contextBlocks     []contextprovider.Block
}

func (b *capturingPromptBuilder) Build(messages []llm.Message, _ []string, _ bool, _ any, slashSkillContent string, _ string, _ ...string) []llm.Message {
	b.slashSkillContent = slashSkillContent
	return messages
}

func (b *capturingPromptBuilder) BuildWithContextBlocks(messages []llm.Message, _ []string, _ bool, _ bool, _ any, slashSkillContent string, _ string, blocks []contextprovider.Block) []llm.Message {
	b.slashSkillContent = slashSkillContent
	b.contextBlocks = append([]contextprovider.Block{}, blocks...)
	return messages
}

func (b *capturingPromptBuilder) BuildTemplateData(_ *profiles.Profile, _ llm.ChatParams, conversationID string) TemplateData {
	return TemplateData{ConversationID: conversationID}
}

func (r *retryMessageRepoStub) CreateMessage(_ context.Context, _ MessageOptions) (*Message, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) UpdateMessageContentAndReasoning(_ context.Context, _ string, _ string, _ string, _, _, _ int, _ string) error {
	return nil
}

func (r *retryMessageRepoStub) GetMessage(_ context.Context, messageID string) (*Message, error) {
	if r.getMessage != nil {
		return r.getMessage(messageID)
	}
	return nil, nil
}

func (r *retryMessageRepoStub) GetMessages(_ context.Context, _ string, _ *string) ([]Message, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) GetMessagesByTurnID(_ context.Context, _ string, _ *string, _ string, _ int) ([]Message, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) GetConversationSummary(_ context.Context, _ string) (string, string, error) {
	return "", "", nil
}

func (r *retryMessageRepoStub) GetDetailedTokenStats(_ context.Context, _ string, _ string) (*DetailedTokenStats, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) GetContextWindowUsage(_ context.Context, _ string, _ int) (float64, int, error) {
	return 0, 0, nil
}

func (r *retryMessageRepoStub) GetRecentMessagesTokenCount(_ context.Context, _ string, _ int) (int, error) {
	return 0, nil
}

func (r *retryMessageRepoStub) GetTurnTokenStats(_ context.Context, _ string, _ string) (*database.TokenStats, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) AddAssistantToolMessage(_ context.Context, _ string, _ string, _, _, _, _ string) (*Message, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) AddToolResultMessage(_ context.Context, _ string, _ string, _, _ string) (*Message, error) {
	return nil, nil
}

func (r *retryMessageRepoStub) SearchMessages(_ context.Context, _ string, _ int) ([]MessageSearchResult, error) {
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

	msg, err := interactor.GetRetryableUserMessage(context.Background(), "7", "42")
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

	msg, err := interactor.GetRetryableUserMessage(context.Background(), "7", "42")
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

// TestPrepareContext_ProfileSlugDoesNotInheritFromGlobalActiveProfile
// é o teste de regressão do bug "selecionei perfil Y mas o app usa X
// do perfil global". A versão antiga do interactor chamava
// `inheritProfileRoutingFields(panel, globalActive)` e qualquer campo
// de routing vazio em `panel` virava silenciosamente o do global,
// produzindo um Active profile híbrido — o perfil escolhido herdando
// provider/model de OUTRO perfil sem nenhum sinal pra UI ou pro user.
//
// O comportamento correto é: cada profile vive sozinho. Profiles
// legacy com campos vazios são normalizados na carga (`Manager.Get`)
// para `$default`, o sentinela explícito que `ResolveProfileDefaults`
// já sabe resolver para o provider default do user. NUNCA o valor
// concreto de OUTRO profile.
func TestPrepareContext_ProfileSlugDoesNotInheritFromGlobalActiveProfile(t *testing.T) {
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

	// Panel legacy: salvo no disco com campos de routing VAZIOS.
	// Manager.Get vai normalizar para `$default` na leitura — esse é o
	// valor que o interactor deve devolver, NÃO os concretos do global.
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

	// CONTRATO: o profile escolhido NÃO herda nada do global ativo.
	// Campos de routing legacy vazios viram `$default` (sentinela
	// resolvido por providers.Service.ResolveProfileDefaults), que é
	// o valor explícito de "use o default do user", não o valor
	// concreto de outro profile.
	if resp.ActiveProfile.Chat.LLMProvider != profiles.DefaultProviderSentinel {
		t.Fatalf("LLMProvider = %q, want %q (cross-profile leak detected)", resp.ActiveProfile.Chat.LLMProvider, profiles.DefaultProviderSentinel)
	}
	if resp.ActiveProfile.Chat.Model != profiles.DefaultProviderSentinel {
		t.Fatalf("Model = %q, want %q (cross-profile leak detected)", resp.ActiveProfile.Chat.Model, profiles.DefaultProviderSentinel)
	}
	if resp.ActiveProfile.Voice.Assistant.LLMProviderID != profiles.DefaultProviderSentinel {
		t.Fatalf("Voice.Assistant.LLMProviderID = %q, want %q (cross-profile leak detected)", resp.ActiveProfile.Voice.Assistant.LLMProviderID, profiles.DefaultProviderSentinel)
	}
	if resp.ActiveProfile.Input.LLMProviderID != profiles.DefaultProviderSentinel {
		t.Fatalf("Input.LLMProviderID = %q, want %q (cross-profile leak detected)", resp.ActiveProfile.Input.LLMProviderID, profiles.DefaultProviderSentinel)
	}

	// E os campos não-routing do panel preservados intactos.
	if resp.ActiveProfile.Chat.Temperature != 0.2 {
		t.Fatalf("Temperature = %v, want 0.2", resp.ActiveProfile.Chat.Temperature)
	}
	if resp.ActiveProfile.Name != "Perfil do Editor" {
		t.Fatalf("Name = %q, want %q", resp.ActiveProfile.Name, "Perfil do Editor")
	}
}

func TestPrepareContext_ResolvePerfilDoWorkspaceQuandoParamsNaoTrazemSlug(t *testing.T) {
	spy := &spyEmitter{}
	profileMgr := setupProfileTestEnv(t)

	active := profiles.DefaultProfile()
	active.Name = "Padrão"
	active.Active = true
	activeSlug, err := profileMgr.Create(active)
	if err != nil {
		t.Fatalf("create active profile: %v", err)
	}
	if err := profileMgr.SetActive(activeSlug); err != nil {
		t.Fatalf("set active profile: %v", err)
	}

	qwen := profiles.DefaultProfile()
	qwen.Name = "Qwen4"
	qwen.Active = false
	qwen.Chat.LLMProvider = "localai-provider"
	qwenSlug, err := profileMgr.Create(qwen)
	if err != nil {
		t.Fatalf("create qwen profile: %v", err)
	}

	inter := NewInteractor(InteractorConfig{
		Emitter:    spy,
		ConvRepo:   noopConvRepo{},
		ProfileMgr: profileMgr,
		Workspace: staticWorkspaceProvider{ws: &workspace.Workspace{
			ID:      "ws-1",
			Name:    "Workspace",
			Profile: activeSlug,
			Tabs: workspace.TabsState{
				Active: "tab-chat",
				Items: []workspace.Tab{
					{
						ID:             "tab-chat",
						Type:           workspace.TabTypeChat,
						ConversationID: "conv-1",
						ProfileOverride: map[string]any{
							"slug": qwenSlug,
						},
					},
				},
			},
		}},
	})

	resp, err := inter.PrepareContext(context.Background(), PrepareContextRequest{
		ConversationID: "conv-1",
		Source:         "wails",
		Params: ChatParams{
			SurfaceTabID: "tab-chat",
		},
	})
	if err != nil {
		t.Fatalf("PrepareContext: %v", err)
	}
	if resp == nil || resp.ActiveProfile == nil {
		t.Fatal("expected activeProfile in response")
	}
	if resp.Params.ProfileSlug != qwenSlug {
		t.Fatalf("ProfileSlug = %q, want %q", resp.Params.ProfileSlug, qwenSlug)
	}
	if resp.ActiveProfile.Name != "Qwen4" {
		t.Fatalf("active profile = %q, want Qwen4", resp.ActiveProfile.Name)
	}
	if resp.ActiveProfile.Chat.LLMProvider != "localai-provider" {
		t.Fatalf("LLMProvider = %q, want localai-provider", resp.ActiveProfile.Chat.LLMProvider)
	}
}

func TestPrepareMessagesEmitsSkillLoadedForOnDemandSkill(t *testing.T) {
	em := &spyEmitter{}
	skill := &skills.Skill{
		SkillMetadata: skills.SkillMetadata{Name: "helper", DisplayName: "Helper", Description: "Help"},
		Slug:          "helper",
		Content:       "help instructions",
	}
	baseSkill := &skills.Skill{
		SkillMetadata: skills.SkillMetadata{Name: "base", DisplayName: "Base", Description: "Base"},
		Slug:          "base",
		Content:       "base instructions",
	}
	interactor := NewInteractor(InteractorConfig{
		Emitter: em,
		SkillMgr: staticSkillRuntimeManager{
			skills: map[string]*skills.Skill{"base": baseSkill, "helper": skill},
		},
	})
	profile := &profiles.Profile{}
	profile.Chat.EnabledSkills = []string{"base", "helper"}

	result := interactor.PrepareMessages(context.Background(), PrepareMessagesRequest{
		Messages:       []llm.Message{{Role: "user", Content: "/helper now"}},
		UserContent:    "/helper now",
		ConversationID: "conv-1",
		TurnID:         "turn-1",
		ActiveProfile:  profile,
	})

	if result.Err != nil {
		t.Fatalf("PrepareMessages returned error: %v", result.Err)
	}
	loaded := em.findSkillLoaded()
	if loaded == nil {
		t.Fatal("expected chat:skill_loaded event")
	}
	if loaded.ConversationID != "conv-1" || loaded.TurnID != "turn-1" || loaded.Slug != "helper" || loaded.Mode != string(skills.SkillModeOnDemand) {
		t.Fatalf("unexpected skill_loaded event: %+v", loaded)
	}
	if result.InvokedSkillSlug != "helper" {
		t.Fatalf("expected invoked skill helper, got %q", result.InvokedSkillSlug)
	}
}

func TestPrepareMessagesDoesNotDuplicateBaseSkillOnSlashInvocation(t *testing.T) {
	em := &spyEmitter{}
	baseSkill := &skills.Skill{
		SkillMetadata: skills.SkillMetadata{Name: "base", DisplayName: "Base", Description: "Base"},
		Slug:          "base",
		Content:       "base instructions",
	}
	promptBuilder := &capturingPromptBuilder{}
	interactor := NewInteractor(InteractorConfig{
		Emitter:       em,
		PromptBuilder: promptBuilder,
		SkillMgr: staticSkillRuntimeManager{
			skills: map[string]*skills.Skill{"base": baseSkill},
		},
	})
	profile := &profiles.Profile{}
	profile.Chat.EnabledSkills = []string{"base"}

	result := interactor.PrepareMessages(context.Background(), PrepareMessagesRequest{
		Messages:       []llm.Message{{Role: "user", Content: "/base"}},
		UserContent:    "/base",
		ConversationID: "conv-1",
		ActiveProfile:  profile,
	})

	if result.Err != nil {
		t.Fatalf("PrepareMessages returned error: %v", result.Err)
	}
	if result.InvokedSkillSlug != "base" {
		t.Fatalf("expected invoked base skill, got %q", result.InvokedSkillSlug)
	}
	if promptBuilder.slashSkillContent != "" {
		t.Fatalf("base skill should not be appended again as slash content: %q", promptBuilder.slashSkillContent)
	}
	if em.findSkillLoaded() != nil {
		t.Fatal("base skill without arguments should not emit skill_loaded")
	}
}

func TestPrepareMessagesPreservesBaseSkillSlashArguments(t *testing.T) {
	baseSkill := &skills.Skill{
		SkillMetadata: skills.SkillMetadata{Name: "base", DisplayName: "Base", Description: "Base"},
		Slug:          "base",
		Content:       "base instructions with $ARGUMENTS",
	}
	promptBuilder := &capturingPromptBuilder{}
	interactor := NewInteractor(InteractorConfig{
		PromptBuilder: promptBuilder,
		SkillMgr: staticSkillRuntimeManager{
			skills: map[string]*skills.Skill{"base": baseSkill},
		},
	})
	profile := &profiles.Profile{}
	profile.Chat.EnabledSkills = []string{"base"}

	result := interactor.PrepareMessages(context.Background(), PrepareMessagesRequest{
		Messages:       []llm.Message{{Role: "user", Content: "/base revisar login"}},
		UserContent:    "/base revisar login",
		ConversationID: "conv-1",
		ActiveProfile:  profile,
	})

	if result.Err != nil {
		t.Fatalf("PrepareMessages returned error: %v", result.Err)
	}
	if !strings.Contains(promptBuilder.slashSkillContent, "<invoked_skill>") ||
		!strings.Contains(promptBuilder.slashSkillContent, "base instructions with revisar login") {
		t.Fatalf("base skill should be appended with processed arguments: %q", promptBuilder.slashSkillContent)
	}
	if strings.Contains(promptBuilder.slashSkillContent, "$ARGUMENTS") {
		t.Fatalf("base skill placeholders should be substituted: %q", promptBuilder.slashSkillContent)
	}
}

func TestPrepareMessagesInjectsLinkedTaskListsAsDynamicContext(t *testing.T) {
	promptBuilder := &capturingPromptBuilder{}
	interactor := NewInteractor(InteractorConfig{
		PromptBuilder: promptBuilder,
		LinkedTaskLists: func(_ context.Context, conversationID string) []TemplateTaskList {
			if conversationID != "conv-1" {
				t.Fatalf("conversationID = %q, want conv-1", conversationID)
			}
			return []TemplateTaskList{{
				ID:          "list-1",
				Title:       "Sprint",
				Description: "Current sprint",
				Tasks: []TemplateTask{{
					ID:         "task-1",
					Title:      "Fix login",
					Status:     "Doing",
					StatusIcon: ">",
				}},
			}}
		},
	})

	result := interactor.PrepareMessages(context.Background(), PrepareMessagesRequest{
		Messages:       []llm.Message{{Role: "user", Content: "status"}},
		UserContent:    "status",
		ConversationID: "conv-1",
		ActiveProfile:  &profiles.Profile{},
	})

	if result.Err != nil {
		t.Fatalf("PrepareMessages returned error: %v", result.Err)
	}
	if len(promptBuilder.contextBlocks) != 1 {
		t.Fatalf("expected one tasklist context block, got %#v", promptBuilder.contextBlocks)
	}
	block := promptBuilder.contextBlocks[0]
	if block.Provider != "tasklist" || block.Name != "linked_task_lists" {
		t.Fatalf("unexpected context block: %+v", block)
	}
	if !strings.Contains(block.Content, "<linked_task_lists>") ||
		!strings.Contains(block.Content, "Sprint (ID: list-1)") ||
		!strings.Contains(block.Content, "| > Doing | Fix login | task-1 |") {
		t.Fatalf("linked task list context missing expected content: %q", block.Content)
	}
}

func TestPrepareMessagesRejectsDisabledSkill(t *testing.T) {
	em := &spyEmitter{}
	skill := &skills.Skill{
		SkillMetadata: skills.SkillMetadata{Name: "disabled", DisplayName: "Disabled", Description: "Disabled"},
		Slug:          "disabled",
		Content:       "disabled instructions",
	}
	interactor := NewInteractor(InteractorConfig{
		Emitter: em,
		SkillMgr: staticSkillRuntimeManager{
			skills: map[string]*skills.Skill{"disabled": skill},
		},
	})
	profile := &profiles.Profile{}
	profile.Chat.EnabledSkills = []string{"other"}

	result := interactor.PrepareMessages(context.Background(), PrepareMessagesRequest{
		Messages:       []llm.Message{{Role: "user", Content: "/disabled"}},
		UserContent:    "/disabled",
		ConversationID: "conv-1",
		TurnID:         "turn-1",
		ActiveProfile:  profile,
	})

	if result.Err == nil {
		t.Fatal("expected disabled skill error")
	}
	if em.findError() == nil {
		t.Fatal("expected chat:error event")
	}
	if em.findSkillLoaded() != nil {
		t.Fatal("disabled skill must not emit skill_loaded")
	}
}
