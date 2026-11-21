package app

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"assistente/controllers"
	"assistente/internal/database"
	"assistente/internal/jobs"
	"assistente/internal/messaging"
	"assistente/internal/questionnaire"
	"assistente/internal/shelltrust"
	"assistente/internal/testutil/authorizationfixture"
	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/tools/shell"
	"assistente/internal/wailsapi"
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
	const source = `example {{ secret "token" }}`
	for _, surface := range []questionnaire.Surface{
		questionnaire.DesktopSurface("chat"),
		questionnaire.ChannelSurface("chat", "telegram", "contact"),
	} {
		t.Run(string(surface.Kind), func(t *testing.T) {
			var visible, body string
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
				body, _ = message["body"].(string)
				if err := desktop.Respond(message["id"].(string), map[string]any{questionnaire.AnswerActionID: "deny"}, false); err != nil {
					t.Fatal(err)
				}
			})
			gateway := messaging.NewGateway(nil, nil, nil, nil, nil, nil)
			gateway.Register("telegram", &consentMessenger{send: func(msg messaging.OutgoingMessage) error {
				visible = msg.Text
				body = msg.Text
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
			ctx = invocationctx.WithShellConsentTemplate(ctx, source, "example "+secret)
			allowed, err = authorizer.Authorize(ctx, "example "+secret, secret+"-diretorio")
			if allowed || err != nil || strings.Contains(visible, secret) || !strings.Contains(body, source) || !strings.Contains(visible, "começa com example") || !strings.Contains(visible, "única execução") {
				t.Fatal("a superfície não exibiu exclusivamente o template de origem")
			}
			allowed, err = authorizer.Authorize(ctx, "example diferente", secret+"-diretorio")
			if allowed || err != nil || strings.Contains(body, source) || strings.Contains(visible, secret) {
				t.Fatal("template público herdado indevidamente por outra operação")
			}
			const vaultProgram = `{{ secret "programa" }}`
			ctx = invocationctx.WithProtectedShellCommand(authorizationfixture.Context(), "[redacted]", "")
			ctx = invocationctx.WithShellConsentTemplate(ctx, vaultProgram, secret)
			allowed, err = authorizer.Authorize(ctx, secret, secret+"-diretorio")
			if allowed || err != nil || strings.Contains(visible, secret) || strings.Contains(body, vaultProgram) || !strings.Contains(visible, "Os detalhes ficam ocultos") || !strings.Contains(visible, "única execução") {
				t.Fatal("a superfície deve usar a confirmação genérica quando o programa vem do cofre")
			}
		})
	}
}

type consentTestSecretStore struct{}

func (consentTestSecretStore) GetSecret(context.Context, string) (string, error) {
	return "sintetico-confidencial", nil
}

func TestManualCatalogShellConsentHasNoConversation(t *testing.T) {
	db := setupAuthAppTestDB(t)
	user := database.User{Username: "teste-catalogo", PasswordHash: "unused", Role: database.UserRoleUser, IsActive: true}
	if err := db.Create(&user).Error; err != nil {
		t.Fatal(err)
	}
	app := &App{currentUserID: user.ID}
	ctx, err := app.requireAuthenticatedContext()
	if err != nil {
		t.Fatal(err)
	}
	conv, err := database.CreateConversationWithContext(ctx, "catalogo", "")
	if err != nil {
		t.Fatal(err)
	}
	askedWithoutConversation := false
	router := questionnaire.NewRouter(nil, nil)
	auth := shelltrust.Authorizer{Prompt: func(ctx context.Context, req shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
		inv, _ := invocationctx.Get(ctx)
		surface := resolveProfileAccessSurface(ctx, inv.Source, inv.ConversationID, database.GetConversationInfoWithContext)
		askedWithoutConversation = surface.Kind == questionnaire.SurfaceNone && req.PublicTemplate != ""
		return askShellConfirmation(ctx, router, surface, req)
	}}
	reg := tools.NewRegistry()
	reg.MustRegister(shell.NewRunCommand(nil, auth.Authorize, nil, t.TempDir()))
	ledger := toolinvocations.NewService(toolinvocations.NewDBRepository(db), tools.NewExecutor(reg, tools.DefaultExecutorConfig()))
	m := jobs.NewManager(jobs.ManagerConfig{ToolRegistry: reg, ToolInvocations: ledger, SecretStore: consentTestSecretStore{}})
	api := wailsapi.NewJobs()
	wailsapi.AttachJobs(api, wailsSession{app: app}, controllers.NewJobsController(controllers.JobsControllerConfig{JobMgr: m}), nil, nil, nil)
	// Nem uma conversa existente, nem campos livres do formulário conferem
	// a um teste do catálogo a proveniência de um turno real.
	req, err := json.Marshal(map[string]any{"tool_name": "run_command", "inputs": map[string]any{"command": `example {{ secret "token" }}`}, "allow_unsafe": true, "conversation_id": conv.ID, "event_data": map[string]any{"conversationId": conv.ID}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := api.TestToolDryRun(string(req))
	if err != nil || result == nil || result.Success || !askedWithoutConversation || strings.Contains(result.Error, "sintetico-confidencial") {
		t.Fatal("o teste manual deveria terminar sem despacho, sem interlocutor")
	}
}
