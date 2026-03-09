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
	if err := db.Exec("DELETE FROM chat_tabs").Error; err != nil {
		t.Fatalf("erro ao limpar chat_tabs: %v", err)
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

	notifier := NewResponseNotifier()

	var emitted []string
	emitEvent := func(event string, data any) {
		emitted = append(emitted, event)
	}

	gateway := NewGateway(notifier, func(conversationID uint, content, media string, params llm.ChatParams, source string) (uint, error) {
		t.Fatalf("sendMessage não deveria ser chamado para contato não autorizado")
		return 0, nil
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

func TestGateway_AuthorizedContact_TTSFallbackToText(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	notifier := NewResponseNotifier()

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}

	var sentConversationID uint
	sendMessage := func(conversationID uint, content, media string, params llm.ChatParams, source string) (uint, error) {
		sentConversationID = conversationID
		if source != "telegram" {
			return 0, fmt.Errorf("source inesperado: %s", source)
		}
		return conversationID, nil
	}

	gateway := NewGateway(
		notifier,
		sendMessage,
		nil,
		nil,
		func(text string, channel string, incomingIsAudio bool) ([]byte, error) {
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
	if sentConversationID == 0 {
		t.Fatalf("conversationID não foi criado")
	}

	conv, err := database.GetConversationInfo(sentConversationID)
	if err != nil {
		t.Fatalf("erro ao buscar conversa: %v", err)
	}
	if conv.Channel != "telegram" || conv.ContactID != "123" {
		t.Fatalf("conversa não vinculada corretamente: channel=%s contact=%s", conv.Channel, conv.ContactID)
	}

	notifier.Notify(sentConversationID, "Resposta", 42)

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

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	notifier := NewResponseNotifier()

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}

	var savedAudio struct {
		msgID uint
		data  string
		mime  string
	}
	var sentConversationID uint

	gateway := NewGateway(
		notifier,
		func(conversationID uint, content, media string, params llm.ChatParams, source string) (uint, error) {
			sentConversationID = conversationID
			return conversationID, nil
		},
		nil,
		nil,
		func(text string, channel string, incomingIsAudio bool) ([]byte, error) {
			return []byte("audio-bytes"), nil
		},
		func(messageID uint, audioBase64 string, mimeType string) error {
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
	if sentConversationID == 0 {
		t.Fatalf("conversationID não foi criado")
	}

	notifier.Notify(sentConversationID, "Resposta", 99)

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

	if savedAudio.msgID != 99 || savedAudio.mime != "audio/mpeg" || savedAudio.data == "" {
		t.Fatalf("áudio não foi salvo corretamente: %+v", savedAudio)
	}
}

func TestGateway_ContactLimitRejectsSilently(t *testing.T) {
	resetState(t)

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1}); err != nil {
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
	gateway := NewGateway(NewResponseNotifier(), func(conversationID uint, content, media string, params llm.ChatParams, source string) (uint, error) {
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

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	var capturedMedia string
	gateway := NewGateway(NewResponseNotifier(), func(conversationID uint, content, media string, params llm.ChatParams, source string) (uint, error) {
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

	if err := channels.Save("telegram", &channels.ChannelConfig{Enabled: true, MaxContacts: 1}); err != nil {
		t.Fatalf("erro ao salvar channel config: %v", err)
	}
	if err := contacts.Authorize("telegram", "123", "Fulano", "user", 1); err != nil {
		t.Fatalf("erro ao autorizar contato: %v", err)
	}

	fake := &fakeMessenger{name: "telegram", status: StatusConnected, sentCh: make(chan OutgoingMessage, 1)}

	gateway := NewGateway(NewResponseNotifier(), func(conversationID uint, content, media string, params llm.ChatParams, source string) (uint, error) {
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
