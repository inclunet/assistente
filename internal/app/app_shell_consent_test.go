package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/internal/messaging"
	"assistente/internal/questionnaire"
	"assistente/internal/shelltrust"
	"assistente/internal/testutil/authorizationfixture"
	"assistente/internal/tools/invocationctx"
)

type consentMessenger struct {
	messaging.Messenger
	send func(messaging.OutgoingMessage) error
}

func (*consentMessenger) SetHandler(messaging.IncomingMessageHandler) {}
func (m *consentMessenger) Send(_ context.Context, msg messaging.OutgoingMessage) error {
	return m.send(msg)
}

func TestProtectedShellConsentOnDesktopAndChannel(t *testing.T) {
	const secret = "marcador-privado-consentimento"
	for _, surface := range []questionnaire.Surface{
		questionnaire.DesktopSurface("chat"),
		questionnaire.ChannelSurface("chat", "telegram", "contact"),
	} {
		t.Run(string(surface.Kind), func(t *testing.T) {
			var visible string
			var desktop *questionnaire.Manager
			desktop = questionnaire.NewManager(func(event string, data any) {
				if event != questionnaire.EventQuestionnaire {
					return
				}
				message := data.(map[string]any)
				value, err := json.Marshal(message)
				if err != nil {
					t.Fatal(err)
				}
				visible = string(value)
				if err := desktop.Respond(message["id"].(string), map[string]any{questionnaire.AnswerActionID: "deny"}, false); err != nil {
					t.Fatal(err)
				}
			})
			gateway := messaging.NewGateway(nil, nil, nil, nil, nil, nil)
			gateway.Register("telegram", &consentMessenger{send: func(msg messaging.OutgoingMessage) error {
				visible = msg.Text
				if gateway.ChannelQuestions().TryAnswer(t.Context(), "chat", "contact", "2") != messaging.AnswerDelivered {
					t.Fatal("resposta de canal não entregue")
				}
				return nil
			}})
			router := questionnaire.NewRouter(func() *questionnaire.Manager { return desktop }, func() questionnaire.ChannelAsker { return gateway.ChannelQuestions() })
			ctx := invocationctx.WithProtectedShellCommand(authorizationfixture.Context(), "[redacted]", "example")
			authorizer := shelltrust.Authorizer{Prompt: func(ctx context.Context, req shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
				return askShellConfirmation(ctx, router, surface, req)
			}}
			allowed, err := authorizer.Authorize(ctx, "example "+secret, secret+"-diretorio")
			if allowed || err != nil || strings.Contains(visible, secret) || !strings.Contains(visible, "começa com example") || !strings.Contains(visible, "única execução") {
				t.Fatal("pergunta ou decisão incorreta na superfície")
			}
		})
	}
}
