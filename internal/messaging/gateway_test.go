package messaging

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/channels"
	"assistente/internal/configdir"
	"assistente/internal/contacts"
	"assistente/internal/database"
	"assistente/internal/llm"
)

type fakeMessenger struct {
	name    string
	status  ConnectionStatus
	handler IncomingMessageHandler
	sentCh  chan OutgoingMessage
	sent    []OutgoingMessage
}

func (f *fakeMessenger) Name() string { return f.name }

func (f *fakeMessenger) Connect(ctx context.Context) error {
	f.status = StatusConnected
	return nil
}

func (f *fakeMessenger) Disconnect() error {
	f.status = StatusDisconnected
	return nil
}

func (f *fakeMessenger) Send(ctx context.Context, msg OutgoingMessage) error {
	f.sent = append(f.sent, msg)
	if f.sentCh != nil {
		f.sentCh <- msg
	}
	return nil
}

func (f *fakeMessenger) SetHandler(handler IncomingMessageHandler) { f.handler = handler }

func (f *fakeMessenger) Status() ConnectionStatus { return f.status }

func TestMain(m *testing.M) {
	tempDir, err := os.MkdirTemp("", "assistente-messaging-*")
	if err != nil {
		panic(err)
	}

	oldWd, _ := os.Getwd()
	_ = os.Setenv("HOME", tempDir)
	_ = os.Setenv("USERPROFILE", tempDir)
	_ = os.Chdir(tempDir)
	configdir.ResetForTests()

	_ = os.RemoveAll(filepath.Join(tempDir, ".assistente"))
	if err := database.Init(); err != nil {
		panic(err)
	}

	code := m.Run()

	_ = database.Close()
	_ = os.Chdir(oldWd)
	_ = os.RemoveAll(tempDir)
	os.Exit(code)
}

func resetState(t *testing.T) {
	db := database.DB()
	if db == nil {
		t.Fatalf("database não inicializado")
	}
	if err := db.Exec("DELETE FROM chat_messages").Error; err != nil {
		t.Fatalf("erro ao limpar chat_messages: %v", err)
	}
	if err := db.Exec("DELETE FROM conversations").Error; err != nil {
		t.Fatalf("erro ao limpar conversations: %v", err)
	}

	_ = channels.Delete("telegram")
	_ = channels.Delete("signal")
	_ = contacts.RemoveAll("telegram")
	_ = contacts.RemoveAll("signal")
}

func TestGateway_UnauthorizedContactDoesNotEmitEvent(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1, OwnerUserID: "test-owner"}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}

	notifier := NewResponseNotifier()

	var emitted []string
	emitEvent := func(event string, data any) {
		emitted = append(emitted, event)
	}

	gateway := NewGateway(notifier, func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
		t.Fatalf("sendMessage não deveria ser chamado para contato não autorizado")
		return "", nil
	}, emitEvent, nil, nil, nil)

	incoming := IncomingMessage{
		ID:      "msg-1",
		Channel: "telegram",
		From: Contact{
			ID:          "123",
			DisplayName: "Fulano",
			Username:    "user",
		},
		Text: "Oi",
	}

	gateway.handleIncoming(context.Background(), incoming)

	if len(emitted) != 0 {
		t.Fatalf("não esperava eventos, got=%v", emitted)
	}
}

// TestGateway_LegacyChannelWithoutOwnerRejectsMessage valida que mensagens de
// canais sem OwnerUserID (config pré-AEP-0052) são rejeitadas com log em vez
// de criar conversas órfãs invisíveis. Sem esse fail-closed, qualquer canal
// legado seguia recebendo mensagens silenciosamente — o usuário enxergava
// "tudo OK" na UI mas o conteúdo nunca aparecia. Blocker D do re-review do
// AEP-0052: o fix completo (migração de OwnerUserID em AdoptLegacyData) virá
// depois; até lá, falhar fechado é melhor que vazar para órfão.
func TestGateway_LegacyChannelWithoutOwnerRejectsMessage(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	called := 0
	notifier := NewResponseNotifier()
	defer notifier.Stop()

	var emittedEvents []string
	emitEvent := func(event string, data any) {
		emittedEvents = append(emittedEvents, event)
	}

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}
	gateway := NewGateway(notifier, func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
		called++
		return conversationID, nil
	}, emitEvent, nil, nil, nil)
	gateway.Register("telegram", fake)

	gateway.handleIncoming(context.Background(), IncomingMessage{
		ID:      "msg-legacy",
		Channel: "telegram",
		From:    Contact{ID: "123", DisplayName: "Fulano", Username: "user"},
		Text:    "Oi",
	})

	if called != 0 {
		t.Fatalf("sendMessage não deveria ser chamado para canal sem OwnerUserID, called=%d", called)
	}

	// M13: valida que callback NÃO foi registrado (antes era um silent
	// failure — o handler retornava sem cancelar nada porque também não
	// registrava. Hoje permanece sem callback, mas sem cobertura podia
	// regredir).
	if notifier.PendingCount() != 0 {
		t.Fatalf("canal legado registrou callback (pending=%d) — gateway deveria rejeitar antes do Register", notifier.PendingCount())
	}

	// M8: evento legacy_channel_dropped é emitido para o frontend.
	foundDropped := false
	for _, ev := range emittedEvents {
		if ev == "messaging:legacy_channel_dropped" {
			foundDropped = true
			break
		}
	}
	if !foundDropped {
		t.Fatalf("esperava evento messaging:legacy_channel_dropped, got=%v", emittedEvents)
	}

	// M8: aviso enviado ao remetente externo via fakeMessenger.
	select {
	case msg := <-fake.sentCh:
		if msg.ChatID != "123" {
			t.Fatalf("aviso enviado para ChatID errado: %q", msg.ChatID)
		}
		if msg.Text == "" {
			t.Fatalf("aviso de canal legado vazio")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout esperando aviso de canal legado para o remetente")
	}
}

// TestGateway_SendMessageErrorCancelsCallback cobre B7 do review da
// Fatia 2: quando sendMessage retorna erro, o callback registrado
// para a conversa deve ser cancelado imediatamente — antes ficava
// pendurado para sempre, virando leak crescente.
func TestGateway_SendMessageErrorCancelsCallback(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1, OwnerUserID: "test-owner"}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	notifier := NewResponseNotifier()
	defer notifier.Stop()

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}

	gateway := NewGateway(notifier, func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
		return conversationID, fmt.Errorf("falha simulada")
	}, nil, nil, nil, nil)
	gateway.Register("telegram", fake)

	gateway.handleIncoming(context.Background(), IncomingMessage{
		ID:      "msg-err",
		Channel: "telegram",
		From:    Contact{ID: "123", DisplayName: "Fulano", Username: "user"},
		Text:    "Oi",
	})

	// Drena o aviso enviado ao remetente para não bloquear o fakeMessenger.
	select {
	case <-fake.sentCh:
	case <-time.After(time.Second):
		t.Fatalf("aviso de erro não enviado ao remetente")
	}

	if notifier.PendingCount() != 0 {
		t.Fatalf("callback não cancelado após erro de sendMessage — leak (pending=%d)", notifier.PendingCount())
	}
}

// TestGateway_UnregisterCancelsPendingCallbacks cobre B7. Quando um
// canal é desregistrado (ex.: usuário desabilitou Telegram em
// settings), callbacks pendentes daquele canal não podem ficar
// pendurados — Unregister deve invocar CancelByChannel.
func TestGateway_UnregisterCancelsPendingCallbacks(t *testing.T) {
	resetState(t)

	notifier := NewResponseNotifier()
	defer notifier.Stop()

	gateway := NewGateway(notifier, func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
		return conversationID, nil
	}, nil, nil, nil, nil)
	fake := &fakeMessenger{name: "telegram", status: StatusConnected}
	gateway.Register("telegram", fake)

	notifier.Register("conv-1", ResponseCallback{Channel: "telegram", TraceID: "t1", Callback: func(string, string) {}})
	notifier.Register("conv-2", ResponseCallback{Channel: "telegram", TraceID: "t2", Callback: func(string, string) {}})
	notifier.Register("conv-3", ResponseCallback{Channel: "signal", TraceID: "s1", Callback: func(string, string) {}})

	if notifier.PendingCount() != 3 {
		t.Fatalf("expected 3 pending, got %d", notifier.PendingCount())
	}

	gateway.Unregister("telegram")

	if notifier.PendingCount() != 1 {
		t.Fatalf("expected 1 pending após Unregister(telegram), got %d", notifier.PendingCount())
	}
}

func TestGateway_AuthorizedContact_TTSFallbackToText(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1, OwnerUserID: "test-owner"}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	notifier := NewResponseNotifier()

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}

	var sentConversationID string
	sendMessage := func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
		sentConversationID = conversationID
		if source != "telegram" {
			return "", fmt.Errorf("source inesperado: %s", source)
		}
		return conversationID, nil
	}

	gateway := NewGateway(
		notifier,
		sendMessage,
		nil,
		nil,
		func(ctx context.Context, text string, channel string, incomingIsAudio bool) ([]byte, error) {
			return nil, fmt.Errorf("tts indisponível")
		},
		nil,
	)
	gateway.Register("telegram", fake)

	incoming := IncomingMessage{
		ID:      "msg-2",
		Channel: "telegram",
		From: Contact{
			ID:          "123",
			DisplayName: "Fulano",
			Username:    "user",
		},
		Text: "Oi",
	}

	gateway.handleIncoming(context.Background(), incoming)
	if sentConversationID == "" {
		t.Fatalf("conversationID não foi criado")
	}

	conv, err := database.GetConversationInfoWithContext(database.WithUserID(context.Background(), "test-owner"), sentConversationID)
	if err != nil {
		t.Fatalf("erro ao buscar conversa: %v", err)
	}
	if conv.Channel != "telegram" || conv.ContactID != "123" {
		t.Fatalf("conversa não vinculada corretamente: channel=%s contact=%s", conv.Channel, conv.ContactID)
	}

	notifier.Notify(sentConversationID, "Resposta", "42")

	select {
	case sent := <-fake.sentCh:
		if sent.Text != "Resposta" {
			t.Fatalf("esperava texto de fallback, got=%q", sent.Text)
		}
		if len(sent.Attachments) != 0 {
			t.Fatalf("não esperava attachments no fallback")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout aguardando envio de mensagem")
	}
}

func TestGateway_AuthorizedContact_TTSSendsAudio(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1, OwnerUserID: "test-owner"}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	notifier := NewResponseNotifier()

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}

	var savedAudio struct {
		msgID string
		data  string
		mime  string
	}
	var sentConversationID string

	gateway := NewGateway(
		notifier,
		func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
			sentConversationID = conversationID
			return conversationID, nil
		},
		nil,
		nil,
		func(ctx context.Context, text string, channel string, incomingIsAudio bool) ([]byte, error) {
			return []byte("audio-bytes"), nil
		},
		func(_ context.Context, messageID string, audioBase64 string, mimeType string) error {
			savedAudio.msgID = messageID
			savedAudio.data = audioBase64
			savedAudio.mime = mimeType
			return nil
		},
	)
	gateway.Register("telegram", fake)

	incoming := IncomingMessage{
		ID:      "msg-3",
		Channel: "telegram",
		From: Contact{
			ID:          "123",
			DisplayName: "Fulano",
			Username:    "user",
		},
		Text: "Oi",
	}

	gateway.handleIncoming(context.Background(), incoming)
	if sentConversationID == "" {
		t.Fatalf("conversationID não foi criado")
	}

	notifier.Notify(sentConversationID, "Resposta", "99")

	select {
	case sent := <-fake.sentCh:
		if sent.Text != "" {
			t.Fatalf("esperava texto vazio quando envia áudio")
		}
		if len(sent.Attachments) != 1 {
			t.Fatalf("esperava 1 attachment, got=%d", len(sent.Attachments))
		}
		if sent.Attachments[0].MIMEType != "audio/mpeg" {
			t.Fatalf("mime incorreto: %s", sent.Attachments[0].MIMEType)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout aguardando envio de áudio")
	}

	if savedAudio.msgID == "" || savedAudio.mime != "audio/mpeg" || savedAudio.data == "" {
		t.Fatalf("áudio não foi salvo corretamente: %+v", savedAudio)
	}
}

func TestGateway_ContactLimitRejectsSilently(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1, OwnerUserID: "test-owner"}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "111", "Contato 1", "user1", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	var emitted []string
	emitEvent := func(event string, data any) {
		emitted = append(emitted, event)
	}

	called := 0
	gateway := NewGateway(NewResponseNotifier(), func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
		called++
		return conversationID, nil
	}, emitEvent, nil, nil, nil)

	incoming := IncomingMessage{
		ID:      "msg-limit",
		Channel: "telegram",
		From: Contact{
			ID:          "222",
			DisplayName: "Contato 2",
			Username:    "user2",
		},
		Text: "Oi",
	}

	gateway.handleIncoming(context.Background(), incoming)

	if called != 0 {
		t.Fatalf("sendMessage não deveria ser chamado, called=%d", called)
	}
	if len(emitted) != 0 {
		t.Fatalf("não deveria emitir eventos, got=%v", emitted)
	}
}

func TestGateway_AttachmentsConvertedToMediaJSON(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1, OwnerUserID: "test-owner"}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	var capturedMedia string
	gateway := NewGateway(NewResponseNotifier(), func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
		capturedMedia = media
		return conversationID, nil
	}, nil, nil, nil, nil)

	incoming := IncomingMessage{
		ID:      "msg-media",
		Channel: "telegram",
		From: Contact{
			ID:          "123",
			DisplayName: "Fulano",
			Username:    "user",
		},
		Text: "Oi",
		Attachments: []Attachment{{
			Filename: "foto.png",
			MIMEType: "image/png",
			Data:     []byte("abc"),
			Size:     3,
		}},
	}

	gateway.handleIncoming(context.Background(), incoming)

	if capturedMedia == "" {
		t.Fatalf("media JSON não capturado")
	}

	var parts []map[string]any
	if err := json.Unmarshal([]byte(capturedMedia), &parts); err != nil {
		t.Fatalf("media JSON inválido: %v", err)
	}
	if len(parts) != 1 {
		t.Fatalf("esperava 1 item de mídia, got=%d", len(parts))
	}
	if parts[0]["type"] != "image/png" || parts[0]["name"] != "foto.png" {
		t.Fatalf("item de mídia incorreto: %+v", parts[0])
	}
	if parts[0]["data"] != base64.StdEncoding.EncodeToString([]byte("abc")) {
		t.Fatalf("base64 incorreto: %v", parts[0]["data"])
	}
}

func TestGateway_SendMessageErrorSendsToMessenger(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1, OwnerUserID: "test-owner"}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}

	gateway := NewGateway(NewResponseNotifier(), func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
		return conversationID, fmt.Errorf("falha de envio")
	}, nil, nil, nil, nil)
	gateway.Register("telegram", fake)

	incoming := IncomingMessage{
		ID:      "msg-error",
		Channel: "telegram",
		From: Contact{
			ID:          "123",
			DisplayName: "Fulano",
			Username:    "user",
		},
		Text: "Oi",
	}

	gateway.handleIncoming(context.Background(), incoming)

	select {
	case sent := <-fake.sentCh:
		if sent.Text == "" || sent.ChatID != "123" {
			t.Fatalf("mensagem de erro inválida: %+v", sent)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("timeout aguardando mensagem de erro")
	}
}

// TestGateway_ChannelOwnerScopesConversation valida o fix do Blocker 2 do
// review do AEP-0052: o config do canal carrega OwnerUserID (preenchido por
// App.SaveChannelConfig com o userID autenticado), e o gateway propaga esse
// valor via WithUserID antes de criar/buscar a conversa. Sem isso, mensagens
// recebidas criariam conversas órfãs (user_id="") visíveis a qualquer caller.
func TestGateway_ChannelOwnerScopesConversation(t *testing.T) {
	resetState(t)

	const ownerID = "user-ana"
	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled:     true,
		MaxContacts: 1,
		OwnerUserID: ownerID,
	}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	var sentConversationID string
	var sendCtxUserID string
	var sendCtxHasUserID bool
	gateway := NewGateway(NewResponseNotifier(), func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
		sentConversationID = conversationID
		sendCtxUserID, sendCtxHasUserID = database.UserIDFromContext(ctx)
		return conversationID, nil
	}, nil, nil, nil, nil)

	gateway.handleIncoming(context.Background(), IncomingMessage{
		ID:      "msg-owner",
		Channel: "telegram",
		From:    Contact{ID: "123", DisplayName: "Fulano", Username: "user"},
		Text:    "Oi",
	})

	if sentConversationID == "" {
		t.Fatalf("conversationID não foi criado")
	}

	if !sendCtxHasUserID || sendCtxUserID != ownerID {
		t.Fatalf("SendMessageFunc recebeu ctx sem OwnerUserID (got userID=%q, has=%v) — gateway falhou em propagar AEP-0052",
			sendCtxUserID, sendCtxHasUserID)
	}

	conv, err := database.GetConversationInfoWithContext(database.WithUserID(context.Background(), ownerID), sentConversationID)
	if err != nil {
		t.Fatalf("erro ao buscar conversa com ctx do owner: %v", err)
	}
	if conv.UserID != ownerID {
		t.Fatalf("conversa criada com user_id=%q, esperava %q", conv.UserID, ownerID)
	}

	// Ctx de outro usuário não deve enxergar a conversa.
	if _, err := database.GetConversationInfoWithContext(database.WithUserID(context.Background(), "user-leo"), sentConversationID); err == nil {
		t.Fatalf("conversa do canal vazou para outro usuário")
	}
}

func TestGateway_TTSNotApplicable_FallsBackToText(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1, OwnerUserID: "test-owner"}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	notifier := NewResponseNotifier()
	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}

	var sentConversationID string
	gateway := NewGateway(
		notifier,
		func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
			sentConversationID = conversationID
			return conversationID, nil
		},
		nil,
		nil,
		func(ctx context.Context, text string, channel string, incomingIsAudio bool) ([]byte, error) {
			// Simula perfil que decide não gerar TTS (retorna nil) — Cancel é chamado,
			// desbloqueando Wait imediatamente com fallback para texto
			return nil, nil
		},
		nil,
	)
	gateway.Register("telegram", fake)

	incoming := IncomingMessage{
		ID:      "msg-timeout",
		Channel: "telegram",
		From:    Contact{ID: "123", DisplayName: "Fulano", Username: "user"},
		Text:    "Oi",
	}

	gateway.handleIncoming(context.Background(), incoming)
	if sentConversationID == "" {
		t.Fatalf("conversationID não foi criado")
	}

	notifier.Notify(sentConversationID, "Resposta texto", "50")

	select {
	case sent := <-fake.sentCh:
		if sent.Text != "Resposta texto" {
			t.Fatalf("esperava texto fallback, got=%q", sent.Text)
		}
		if len(sent.Attachments) != 0 {
			t.Fatalf("não esperava attachments no timeout/cancel, got=%d", len(sent.Attachments))
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timeout aguardando envio de mensagem")
	}
}

func TestGateway_MaxHistoryOverridesContextMessages(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{
		Enabled: true, MaxContacts: 1, OwnerUserID: "test-owner", MaxHistory: 17,
	}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	notifier := NewResponseNotifier()
	defer notifier.Stop()

	var gotParams llm.ChatParams
	gateway := NewGateway(notifier, func(ctx context.Context, conversationID string, content, media string, params llm.ChatParams, source string) (string, error) {
		gotParams = params
		return conversationID, nil
	}, nil, nil, nil, nil)
	fake := &fakeMessenger{name: "telegram", status: StatusConnected}
	gateway.Register("telegram", fake)

	gateway.handleIncoming(context.Background(), IncomingMessage{
		ID: "msg-hist", Channel: "telegram",
		From: Contact{ID: "123", DisplayName: "Fulano", Username: "user"},
		Text: "Oi",
	})

	if gotParams.MaxContextMessages != 17 {
		t.Fatalf("MaxContextMessages = %d, want 17 (max_history do canal)", gotParams.MaxContextMessages)
	}
}
