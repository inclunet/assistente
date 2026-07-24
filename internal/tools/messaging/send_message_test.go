package messaging

import (
	"context"
	"encoding/json"
	"testing"

	msgpkg "assistente/internal/messaging"
)

type fakeMessenger struct {
	name     string
	lastSent msgpkg.OutgoingMessage
	sendErr  error
}

func (f *fakeMessenger) Name() string                                     { return f.name }
func (f *fakeMessenger) Connect(ctx context.Context) error                { return nil }
func (f *fakeMessenger) Disconnect() error                                { return nil }
func (f *fakeMessenger) SetHandler(handler msgpkg.IncomingMessageHandler) {}
func (f *fakeMessenger) Status() msgpkg.ConnectionStatus                  { return msgpkg.StatusConnected }
func (f *fakeMessenger) Send(ctx context.Context, msg msgpkg.OutgoingMessage) error {
	f.lastSent = msg
	return f.sendErr
}

func TestSendMessageTool_ValidateArgs(t *testing.T) {
	tool := NewSendMessageTool(nil)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"channel":"telegram"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result for missing fields")
	}
}

func TestSendMessageTool_GatewayMissing(t *testing.T) {
	tool := NewSendMessageTool(nil)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"channel":"telegram","to":"123","message":"hi"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result when gateway is nil")
	}
}

func TestSendMessageTool_ChannelNotFound(t *testing.T) {
	gateway := msgpkg.NewGateway(nil, nil, nil, nil, nil, nil)
	tool := NewSendMessageTool(gateway)

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"channel":"telegram","to":"123","message":"hi"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result when channel not registered")
	}
}

func TestSendMessageTool_Success(t *testing.T) {
	gateway := msgpkg.NewGateway(nil, nil, nil, nil, nil, nil)
	messenger := &fakeMessenger{name: "telegram"}
	gateway.Register("telegram", messenger)

	tool := NewSendMessageTool(gateway)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"channel":"telegram","to":"123","message":"hi"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", result.Content)
	}
	if messenger.lastSent.ChatID != "123" || messenger.lastSent.Text != "hi" {
		t.Fatalf("unexpected message sent: %+v", messenger.lastSent)
	}
}

func TestSendMessageTool_ParametersIncludeSlack(t *testing.T) {
	tool := NewSendMessageTool(nil)
	raw := tool.Parameters()
	if !json.Valid(raw) {
		t.Fatalf("parameters JSON inválido")
	}

	var schema struct {
		Properties struct {
			Channel struct {
				Enum []string `json:"enum"`
			} `json:"channel"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}

	want := map[string]bool{"slack": true, "telegram": true, "signal": true}
	got := map[string]bool{}
	for _, v := range schema.Properties.Channel.Enum {
		got[v] = true
	}
	for channel := range want {
		if !got[channel] {
			t.Fatalf("enum de channel deveria incluir %q; got=%v", channel, schema.Properties.Channel.Enum)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("enum de channel inesperado: got=%v want=%v", schema.Properties.Channel.Enum, []string{"slack", "telegram", "signal"})
	}
}

func TestSendMessageTool_SendFailure(t *testing.T) {
	gateway := msgpkg.NewGateway(nil, nil, nil, nil, nil, nil)
	messenger := &fakeMessenger{name: "signal", sendErr: context.DeadlineExceeded}
	gateway.Register("signal", messenger)

	tool := NewSendMessageTool(gateway)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"channel":"signal","to":"+5511999999999","message":"hi"}`))
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !result.IsError {
		t.Fatalf("expected error result when send fails")
	}
}
