package app

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"

	"assistente/internal/chat"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/events"
	"assistente/internal/messaging"
)

// ==================== helpers de teste ====================

// capturedEvent é um par evento/dado capturado pelo mockEmitter.
type capturedEvent struct {
	name string
	data any
}

// testEmitter é um events.Emitter que captura todos os eventos emitidos.
type testEmitter struct {
	mu     sync.Mutex
	events []capturedEvent
}

func (e *testEmitter) Emit(event string, data any) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.events = append(e.events, capturedEvent{event, data})
}

func (e *testEmitter) count() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.events)
}

func (e *testEmitter) find(eventName string) []capturedEvent {
	e.mu.Lock()
	defer e.mu.Unlock()
	var out []capturedEvent
	for _, ev := range e.events {
		if ev.name == eventName {
			out = append(out, ev)
		}
	}
	return out
}

var _ events.Emitter = (*testEmitter)(nil)

// mockMessageRepo é um MessageRepository in-memory para testes sem banco.
type mockMessageRepo struct {
	messages []database.ChatMessage
	summary  string
	upToID   string
	err      error
}

func (r *mockMessageRepo) CreateMessage(_ context.Context, opts database.MessageOptions) (*database.ChatMessage, error) {
	return nil, nil
}

func (r *mockMessageRepo) UpdateMessageContentAndReasoning(_ context.Context, _ string, _ string, _ string, _, _, _ int, _ string) error {
	return nil
}
func (r *mockMessageRepo) GetMessage(_ context.Context, messageID string) (*database.ChatMessage, error) {
	for i := range r.messages {
		if r.messages[i].ID == messageID {
			msg := r.messages[i]
			return &msg, nil
		}
	}
	return nil, nil
}
func (r *mockMessageRepo) GetMessages(_ context.Context, conversationID string, parentID *string) ([]database.ChatMessage, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.messages, nil
}

func (r *mockMessageRepo) GetMessagesByTurnID(_ context.Context, _ string, _ *string, _ string, _ int) ([]database.ChatMessage, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.messages, nil
}
func (r *mockMessageRepo) GetConversationSummary(_ context.Context, conversationID string) (string, string, error) {
	return r.summary, r.upToID, nil
}
func (r *mockMessageRepo) GetDetailedTokenStats(_ context.Context, conversationID string, summaryUpToMessageID string) (*database.DetailedTokenStats, error) {
	return &database.DetailedTokenStats{}, nil
}
func (r *mockMessageRepo) GetContextWindowUsage(_ context.Context, conversationID string, contextLimit int) (float64, int, error) {
	return 0, 0, nil
}
func (r *mockMessageRepo) GetRecentMessagesTokenCount(_ context.Context, conversationID string, messageLimit int) (int, error) {
	return 0, nil
}
func (r *mockMessageRepo) GetTurnTokenStats(_ context.Context, conversationID string, turnID string) (*database.TokenStats, error) {
	return &database.TokenStats{}, nil
}
func (r *mockMessageRepo) AddAssistantToolMessage(_ context.Context, conversationID, turnID string, content, toolCalls, reasoning, model string) (*database.ChatMessage, error) {
	return nil, nil
}
func (r *mockMessageRepo) AddToolResultMessage(_ context.Context, conversationID, turnID string, content, toolCallID string) (*database.ChatMessage, error) {
	return nil, nil
}
func (r *mockMessageRepo) SearchMessages(_ context.Context, query string, limit int) ([]database.MessageSearchResult, error) {
	return nil, nil
}

// newMinimalApp cria um *App com apenas os campos necessários para os testes deste arquivo.
func newMinimalApp() *App {
	notifier := messaging.NewResponseNotifier()
	return &App{
		ctx:              context.Background(),
		emitter:          &testEmitter{},
		responseNotifier: notifier,
		streamMgr:        chat.NewStreamingManager(notifier),
		currentUserID:    "test-user",
	}
}

// userMsg cria um ChatMessage com role=user.
func userMsg(id string, content string) database.ChatMessage {
	return database.ChatMessage{UUIDModel: database.UUIDModel{ID: id}, Role: "user", Content: content}
}

// assistantMsg cria um ChatMessage com role=assistant.
func assistantMsg(id string, content string) database.ChatMessage {
	return database.ChatMessage{UUIDModel: database.UUIDModel{ID: id}, Role: "assistant", Content: content}
}

// toolMsg cria um ChatMessage com role=tool.
func toolMsg(id string, toolCallID string) database.ChatMessage {
	return database.ChatMessage{UUIDModel: database.UUIDModel{ID: id}, Role: "tool", ToolCallID: toolCallID, Content: "resultado"}
}

// assistantWithToolCalls cria um ChatMessage assistant com ToolCalls e sem conteúdo textual.
func assistantWithToolCalls(id string, toolCallID string) database.ChatMessage {
	toolCalls := []map[string]interface{}{
		{"id": toolCallID, "type": "function", "function": map[string]interface{}{"name": "test", "arguments": "{}"}},
	}
	b, _ := json.Marshal(toolCalls)
	return database.ChatMessage{UUIDModel: database.UUIDModel{ID: id}, Role: "assistant", Content: "", ToolCalls: string(b)}
}

// ==================== extractAudioFromMedia ====================

func TestExtractAudioFromMedia_SingleAudio(t *testing.T) {
	media := `[{"type":"audio/webm","data":"dGVzdGUK"}]`
	b64, mime := extractAudioFromMedia(media)
	if b64 != "dGVzdGUK" {
		t.Errorf("esperava data='dGVzdGUK', obteve %q", b64)
	}
	if mime != "audio/webm" {
		t.Errorf("esperava mime='audio/webm', obteve %q", mime)
	}
}

func TestExtractAudioFromMedia_ImageOnly_ReturnsEmpty(t *testing.T) {
	media := `[{"type":"image/png","data":"abc"}]`
	b64, mime := extractAudioFromMedia(media)
	if b64 != "" || mime != "" {
		t.Errorf("esperava strings vazias para imagem, obteve b64=%q mime=%q", b64, mime)
	}
}

func TestExtractAudioFromMedia_MultiplePartsFirstAudio(t *testing.T) {
	media := `[{"type":"image/png","data":"img"},{"type":"audio/mp3","data":"mp3data"}]`
	b64, mime := extractAudioFromMedia(media)
	if b64 != "mp3data" {
		t.Errorf("esperava mp3data, obteve %q", b64)
	}
	if mime != "audio/mp3" {
		t.Errorf("esperava audio/mp3, obteve %q", mime)
	}
}

func TestExtractAudioFromMedia_EmptyJSON_ReturnsEmpty(t *testing.T) {
	b64, mime := extractAudioFromMedia("[]")
	if b64 != "" || mime != "" {
		t.Errorf("esperava strings vazias para array vazio, obteve b64=%q mime=%q", b64, mime)
	}
}

func TestExtractAudioFromMedia_InvalidJSON_ReturnsEmpty(t *testing.T) {
	b64, mime := extractAudioFromMedia("não é json")
	if b64 != "" || mime != "" {
		t.Errorf("esperava strings vazias para JSON inválido, obteve b64=%q mime=%q", b64, mime)
	}
}

func TestExtractAudioFromMedia_AudioWithEmptyData_ReturnsEmpty(t *testing.T) {
	media := `[{"type":"audio/wav","data":""}]`
	b64, mime := extractAudioFromMedia(media)
	if b64 != "" || mime != "" {
		t.Errorf("esperava strings vazias para data vazio, obteve b64=%q mime=%q", b64, mime)
	}
}

// ==================== whisperFilename ====================

func TestWhisperFilename_KnownMappings(t *testing.T) {
	cases := []struct{ input, want string }{
		{"aac", "audio.m4a"},
		{"opus", "audio.ogg"},
		{"mp3", "audio.mp3"},
		{"wav", "audio.wav"},
		{"webm", "audio.webm"},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			got := chat.WhisperFilename(c.input)
			if got != c.want {
				t.Errorf("whisperFilename(%q) = %q, want %q", c.input, got, c.want)
			}
		})
	}
}

// ==================== recoverFromPanic ====================

func TestRecoverFromPanic_EmitsStreamEventWithError(t *testing.T) {
	app := newMinimalApp()
	em := app.emitter.(*testEmitter)

	func() {
		defer app.recoverFromPanic("42", "TestSource")
		panic("algo explodiu")
	}()

	evs := em.find("chat:stream")
	if len(evs) == 0 {
		t.Fatal("esperava evento chat:stream após panic, nenhum emitido")
	}

	ev, ok := evs[0].data.(StreamEvent)
	if !ok {
		t.Fatalf("esperava StreamEvent, obteve %T", evs[0].data)
	}
	if !ev.Done {
		t.Error("StreamEvent.Done deveria ser true")
	}
	if ev.Error != ports.ChatErrorInternal {
		t.Errorf("esperava código de erro interno, obteve %q", ev.Error)
	}
	if ev.ConversationId != "42" {
		t.Errorf("esperava ConversationId=42, obteve %s", ev.ConversationId)
	}
}

func TestRecoverFromPanic_NoPanic_EmitsNothing(t *testing.T) {
	app := newMinimalApp()
	em := app.emitter.(*testEmitter)

	func() {
		defer app.recoverFromPanic("1", "TestSource")
	}()

	if em.count() != 0 {
		t.Errorf("esperava zero eventos sem panic, obteve %d", em.count())
	}
}

func TestRecoverFromPanic_NilEmitter_DoesNotDoublePanic(t *testing.T) {
	app := &App{
		emitter:   nil,
		streamMgr: chat.NewStreamingManager(nil),
	}
	// Não deve causar segundo panic
	func() {
		defer app.recoverFromPanic("99", "NilEmitterTest")
		panic("teste nil emitter")
	}()
}

// ==================== registerStreamingContext ====================

func TestRegisterStreamingContext_StoresEntry(t *testing.T) {
	app := newMinimalApp()
	_, cancel := context.WithCancel(context.Background())
	app.registerStreamingContext("10", cancel)

	app.streamMgr.Mu(func(m map[string]context.CancelFunc) {
		if _, ok := m["10"]; !ok {
			t.Error("esperava entry no map após register")
		}
	})
}

func TestRegisterStreamingContext_OverwriteCancelsPrevious(t *testing.T) {
	app := newMinimalApp()

	ctx1, cancel1 := context.WithCancel(context.Background())
	app.registerStreamingContext("10", cancel1)

	_, cancel2 := context.WithCancel(context.Background())
	app.registerStreamingContext("10", cancel2)

	select {
	case <-ctx1.Done():
		// correto — contexto anterior cancelado
	default:
		t.Error("context anterior deveria ter sido cancelado ao sobrescrever")
	}
}

func TestRegisterStreamingContext_MultipleConversations_AllStored(t *testing.T) {
	app := newMinimalApp()
	for i := 1; i <= 5; i++ {
		_, cancel := context.WithCancel(context.Background())
		app.registerStreamingContext(fmt.Sprintf("%d", i), cancel)
	}

	var n int
	app.streamMgr.Mu(func(m map[string]context.CancelFunc) { n = len(m) })

	if n != 5 {
		t.Errorf("esperava 5 entries, obteve %d", n)
	}
}

// ==================== unregisterStreamingContext ====================

func TestUnregisterStreamingContext_RemovesEntry(t *testing.T) {
	app := newMinimalApp()
	_, cancel := context.WithCancel(context.Background())
	app.registerStreamingContext("20", cancel)
	app.unregisterStreamingContext("20")

	app.streamMgr.Mu(func(m map[string]context.CancelFunc) {
		if _, ok := m["20"]; ok {
			t.Error("entry deveria ter sido removida")
		}
	})
}

func TestUnregisterStreamingContext_NonExistent_NoError(t *testing.T) {
	app := newMinimalApp()
	app.unregisterStreamingContext("999") // não deve panics ou erro
}

func TestUnregisterStreamingContext_OnlyTargetRemoved(t *testing.T) {
	app := newMinimalApp()
	_, c1 := context.WithCancel(context.Background())
	_, c2 := context.WithCancel(context.Background())
	app.registerStreamingContext("1", c1)
	app.registerStreamingContext("2", c2)
	app.unregisterStreamingContext("1")

	var has1, has2 bool
	app.streamMgr.Mu(func(m map[string]context.CancelFunc) {
		_, has1 = m["1"]
		_, has2 = m["2"]
	})

	if has1 {
		t.Error("conversa 1 deveria ter sido removida")
	}
	if !has2 {
		t.Error("conversa 2 não deveria ter sido removida")
	}
}

// ==================== CancelStreamingForConversation ====================

func TestCancelStreaming_CancelsContextAndRemovesEntry(t *testing.T) {
	app := newMinimalApp()
	ctx, cancel := context.WithCancel(context.Background())
	app.registerStreamingContext("30", cancel)
	app.CancelStreamingForConversation("30")

	select {
	case <-ctx.Done():
	default:
		t.Error("context deveria estar cancelado")
	}

	var ok bool
	app.streamMgr.Mu(func(m map[string]context.CancelFunc) { _, ok = m["30"] })
	if ok {
		t.Error("entry deveria ter sido removida do map")
	}
}

func TestCancelStreaming_NotifiesResponseNotifier(t *testing.T) {
	// responseNotifier é concreto (*messaging.ResponseNotifier), então verificamos
	// o comportamento via PendingCount: registrar um callback e cancelar deve zerar o pending.
	app := newMinimalApp()
	notifier := app.responseNotifier

	// Registra callback para conversa 30
	notifier.Register("30", messaging.ResponseCallback{
		Channel:  "test",
		Callback: func(_ string, _ string) {},
	})
	if notifier.PendingCount() != 1 {
		t.Fatalf("esperava 1 pending antes do cancel, obteve %d", notifier.PendingCount())
	}

	_, cancel := context.WithCancel(context.Background())
	app.registerStreamingContext("30", cancel)
	app.CancelStreamingForConversation("30")

	if notifier.PendingCount() != 0 {
		t.Errorf("esperava 0 pending após cancel, obteve %d", notifier.PendingCount())
	}
}

func TestCancelStreaming_NonExistent_DoesNotNotify(t *testing.T) {
	// Cancela conversa que nunca teve streaming registrado — PendingCount deve permanecer 0.
	app := newMinimalApp()
	app.CancelStreamingForConversation("999")
	if app.responseNotifier.PendingCount() != 0 {
		t.Errorf("pendingCount deveria ser 0, obteve %d", app.responseNotifier.PendingCount())
	}
}

func TestCancelStreaming_Idempotent_DoesNotPanic(t *testing.T) {
	app := newMinimalApp()
	_, cancel := context.WithCancel(context.Background())
	app.registerStreamingContext("40", cancel)
	app.CancelStreamingForConversation("40")
	app.CancelStreamingForConversation("40") // segunda chamada não deve panicar
}

func TestCancelStreaming_NilNotifier_DoesNotPanic(t *testing.T) {
	app := newMinimalApp()
	app.responseNotifier = nil
	_, cancel := context.WithCancel(context.Background())
	app.registerStreamingContext("5", cancel)
	app.CancelStreamingForConversation("5")
}

// ==================== loadConversationHistory ====================

func TestLoadConversationHistory_SimpleTextMessages(t *testing.T) {
	app := newMinimalApp()
	app.msgRepo = &mockMessageRepo{
		messages: []database.ChatMessage{
			userMsg("1", "olá"),
			assistantMsg("2", "oi, como posso ajudar?"),
		},
	}

	msgs, summary, err := app.loadConversationHistory("1", nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if summary != "" {
		t.Errorf("esperava summary vazio, obteve %q", summary)
	}
	if len(msgs) != 2 {
		t.Errorf("esperava 2 mensagens, obteve %d", len(msgs))
	}
	if msgs[0].Role != "user" {
		t.Errorf("primeira mensagem deveria ser user, obteve %q", msgs[0].Role)
	}
	if msgs[1].Role != "assistant" {
		t.Errorf("segunda mensagem deveria ser assistant, obteve %q", msgs[1].Role)
	}
}

func TestLoadConversationHistory_SkipsToolMessages(t *testing.T) {
	app := newMinimalApp()
	app.msgRepo = &mockMessageRepo{
		messages: []database.ChatMessage{
			userMsg("1", "busque algo"),
			assistantWithToolCalls("2", "call_abc"),
			toolMsg("3", "call_abc"),
			assistantMsg("4", "encontrei o resultado"),
		},
	}

	msgs, _, err := app.loadConversationHistory("1", nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	for _, m := range msgs {
		if m.Role == "tool" {
			t.Error("mensagem com role=tool não deveria estar no histórico para o LLM")
		}
	}
}

func TestLoadConversationHistory_SkipsEmptyAssistantWithToolCalls(t *testing.T) {
	app := newMinimalApp()
	app.msgRepo = &mockMessageRepo{
		messages: []database.ChatMessage{
			userMsg("1", "faça algo"),
			assistantWithToolCalls("2", "call_xyz"), // assistant vazio com tool_calls
			toolMsg("3", "call_xyz"),
			assistantMsg("4", "feito!"),
		},
	}

	msgs, _, err := app.loadConversationHistory("1", nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}

	for _, m := range msgs {
		if m.Role == "assistant" && m.Content == "" {
			t.Error("assistant vazio com tool_calls não deveria aparecer no histórico")
		}
	}
}

func TestLoadConversationHistory_RollingContext_ExcludesOldMessages(t *testing.T) {
	app := newMinimalApp()
	app.msgRepo = &mockMessageRepo{
		// upToID=2 significa que msgs ID<=2 já foram resumidas
		summary: "resumo das mensagens antigas",
		upToID:  "2",
		messages: []database.ChatMessage{
			userMsg("1", "antiga 1"),
			assistantMsg("2", "antiga 2"),
			userMsg("3", "nova 1"),
			assistantMsg("4", "nova 2"),
		},
	}

	msgs, summary, err := app.loadConversationHistory("1", nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if summary != "resumo das mensagens antigas" {
		t.Errorf("esperava summary, obteve %q", summary)
	}
	// Apenas as mensagens com ID > upToID (3 e 4) devem estar no contexto
	oldIDs := map[string]bool{"1": true, "2": true}
	for _, m := range msgs {
		// Não há acesso direto ao ID via Message (type alias), portanto verificamos
		// o conteúdo: as mensagens antigas têm conteúdo "antiga 1" e "antiga 2".
		if content, ok := m.Content.(string); ok {
			if content == "antiga 1" || content == "antiga 2" {
				t.Errorf("mensagem já resumida não deveria estar no contexto: %q", content)
			}
		}
		_ = oldIDs
	}
	if len(msgs) != 2 {
		t.Errorf("esperava 2 mensagens pós-resumo, obteve %d", len(msgs))
	}
}

func TestLoadConversationHistory_RepoError_ReturnsError(t *testing.T) {
	app := newMinimalApp()
	app.msgRepo = &mockMessageRepo{
		err: fmt.Errorf("banco indisponível"),
	}

	_, _, err := app.loadConversationHistory("1", nil)
	if err == nil {
		t.Fatal("esperava erro quando o repositório falha")
	}
}

func TestLoadConversationHistory_EmptyConversation_ReturnsEmpty(t *testing.T) {
	app := newMinimalApp()
	app.msgRepo = &mockMessageRepo{messages: []database.ChatMessage{}}

	msgs, _, err := app.loadConversationHistory("1", nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(msgs) != 0 {
		t.Errorf("esperava slice vazio, obteve %d mensagens", len(msgs))
	}
}

func TestLoadConversationHistory_StartsWithAssistant_AssistantTrimmed(t *testing.T) {
	// HistoryLoader garante que a primeira mensagem é sempre user
	app := newMinimalApp()
	app.msgRepo = &mockMessageRepo{
		messages: []database.ChatMessage{
			assistantMsg("1", "mensagem inicial do assistant"),
			userMsg("2", "oi"),
		},
	}

	msgs, _, err := app.loadConversationHistory("1", nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(msgs) == 0 {
		t.Fatal("esperava pelo menos uma mensagem")
	}
	if msgs[0].Role != "user" {
		t.Errorf("primeira mensagem deveria ser user após trim, obteve %q", msgs[0].Role)
	}
}

func TestLoadConversationHistory_MultimodalImageMessage(t *testing.T) {
	media := `[{"type":"image/png","data":"iVBORw0KGgo=","name":"foto.png"}]`
	app := newMinimalApp()
	app.msgRepo = &mockMessageRepo{
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: "1"}, Role: "user", Content: "o que tem na imagem?", Media: media},
		},
	}

	msgs, _, err := app.loadConversationHistory("1", nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("esperava 1 mensagem, obteve %d", len(msgs))
	}
	// Conteúdo deve ser multimodal ([]interface{})
	parts, ok := msgs[0].Content.([]interface{})
	if !ok {
		t.Fatalf("esperava conteúdo multimodal ([]interface{}), obteve %T", msgs[0].Content)
	}
	// Deve ter ao menos um part de imagem
	hasImage := false
	for _, p := range parts {
		m, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if m["type"] == "image_url" {
			hasImage = true
		}
	}
	if !hasImage {
		t.Error("esperava part de image_url para imagem PNG")
	}
}

func TestLoadConversationHistory_AudioWAV_SendsAsInputAudio(t *testing.T) {
	media := `[{"type":"audio/wav","data":"UklGRg==","name":"rec.wav"}]`
	app := newMinimalApp()
	app.msgRepo = &mockMessageRepo{
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: "1"}, Role: "user", Content: "", Media: media},
		},
	}

	msgs, _, err := app.loadConversationHistory("1", nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("esperava 1 mensagem, obteve %d", len(msgs))
	}
	parts, ok := msgs[0].Content.([]interface{})
	if !ok {
		t.Fatalf("esperava []interface{}, obteve %T", msgs[0].Content)
	}
	hasAudio := false
	for _, p := range parts {
		m, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if m["type"] == "input_audio" {
			hasAudio = true
		}
	}
	if !hasAudio {
		t.Error("áudio WAV suportado deveria gerar part input_audio")
	}
}

func TestLoadConversationHistory_AudioWithTranscription_SkipsAudioPart(t *testing.T) {
	// Se a mensagem já tem Content (transcrição), o áudio não deve ser re-adicionado
	media := `[{"type":"audio/wav","data":"UklGRg=="}]`
	app := newMinimalApp()
	app.msgRepo = &mockMessageRepo{
		messages: []database.ChatMessage{
			{UUIDModel: database.UUIDModel{ID: "1"}, Role: "user", Content: "o usuário disse: olá", Media: media},
		},
	}

	msgs, _, err := app.loadConversationHistory("1", nil)
	if err != nil {
		t.Fatalf("erro inesperado: %v", err)
	}
	parts, ok := msgs[0].Content.([]interface{})
	if !ok {
		t.Fatalf("esperava []interface{}, obteve %T", msgs[0].Content)
	}
	audioParts := 0
	for _, p := range parts {
		m, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		if m["type"] == "input_audio" {
			audioParts++
		}
	}
	if audioParts > 0 {
		t.Error("mensagem com transcrição existente não deveria re-enviar áudio como input_audio")
	}
}

// ==================== concorrência ====================

func TestStreamingContext_ConcurrentRegisterUnregister(t *testing.T) {
	app := newMinimalApp()
	var wg sync.WaitGroup

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, cancel := context.WithCancel(context.Background())
			app.registerStreamingContext(id, cancel)
			app.unregisterStreamingContext(id)
		}(fmt.Sprintf("%d", i))
	}

	wg.Wait()

	var n int
	app.streamMgr.Mu(func(m map[string]context.CancelFunc) { n = len(m) })
	if n != 0 {
		t.Errorf("esperava map vazio após todos os unregisters, obteve %d entries", n)
	}
}

func TestCancelStreaming_ConcurrentCancels_DoNotPanic(t *testing.T) {
	app := newMinimalApp()

	for i := 0; i < 20; i++ {
		_, cancel := context.WithCancel(context.Background())
		app.registerStreamingContext(fmt.Sprintf("%d", i), cancel)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			app.CancelStreamingForConversation(id)
		}(fmt.Sprintf("%d", i))
	}
	wg.Wait()

	var n2 int
	app.streamMgr.Mu(func(m map[string]context.CancelFunc) { n2 = len(m) })
	if n2 != 0 {
		t.Errorf("esperava map vazio após cancelamentos, obteve %d entries", n2)
	}
}
