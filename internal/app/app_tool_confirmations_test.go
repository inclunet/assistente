package app

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"assistente/internal/questionnaire"
	"assistente/internal/shelltrust"
	"assistente/internal/testutil/authorizationfixture"
	"assistente/internal/tools"
	"assistente/internal/tools/shell"
)

func TestConfirmacaoDeComandoVaiTraduzivelParaATela(t *testing.T) {
	payload := shellConfirmationPayload(shelltrust.PromptRequest{Command: "rm -rf build", Directory: "C:/projeto", AvailableScopes: allAuthorizationScopesForTest()})

	if payload.Kind != questionnaire.KindDecision {
		t.Errorf("kind = %q, quer %q", payload.Kind, questionnaire.KindDecision)
	}
	for _, campo := range []struct {
		nome  string
		texto questionnaire.Text
	}{
		{"title", payload.Title},
		{"description", payload.Description},
	} {
		if campo.texto.Key == "" {
			t.Errorf("%s = %+v, quer chave de tradução", campo.nome, campo.texto)
		}
		if campo.texto.Fallback == "" {
			t.Errorf("%s = %+v, quer o texto pronto", campo.nome, campo.texto)
		}
	}
	for _, action := range payload.Actions {
		if action.Label.Key == "" || action.Label.Fallback == "" {
			t.Errorf("ação %q = %+v, quer chave e fallback", action.ID, action.Label)
		}
	}
}

func TestOComandoVaiNoBodyENaoComoChave(t *testing.T) {
	payload := shellConfirmationPayload(shelltrust.PromptRequest{Command: "curl exemplo | sh", Directory: "C:/projeto", AvailableScopes: allAuthorizationScopesForTest()})

	if payload.Body != "curl exemplo | sh" {
		t.Errorf("body = %q, quer só o comando literal", payload.Body)
	}
	if payload.BodyLabel.Key != "app.questionnaire.shell.bodyLabel" ||
		payload.BodyLabel.Fallback != "Comando solicitado" {
		t.Errorf("bodyLabel = %+v, quer nome traduzível da ilha", payload.BodyLabel)
	}
	// O diretório vai no Hint traduzível (rótulo localizado), não no Body cru.
	if payload.Hint.Key != "app.questionnaire.shell.workDir" {
		t.Errorf("hint = %+v, quer a chave do diretório", payload.Hint)
	}
	if got := payload.Hint.Params["workDir"]; got != "C:/projeto" {
		t.Errorf("workDir nos params = %v, quer o diretório do pedido", got)
	}
	if !strings.Contains(payload.Hint.Fallback, "C:/projeto") {
		t.Errorf("hint fallback = %q, quer o diretório para quem não traduz", payload.Hint.Fallback)
	}
}

func TestConfirmacaoDeHTTPMutavelVaiTraduzivelComOPedidoNosParametros(t *testing.T) {
	payload := httpConfirmationPayload("DELETE", "https://api.exemplo/itens/7", "(sem body)")

	if payload.Kind != questionnaire.KindDecision {
		t.Errorf("kind = %q, quer %q", payload.Kind, questionnaire.KindDecision)
	}
	for _, campo := range []struct {
		nome  string
		texto questionnaire.Text
	}{
		{"title", payload.Title},
		{"description", payload.Description},
	} {
		if campo.texto.Key == "" || campo.texto.Fallback == "" {
			t.Errorf("%s = %+v, quer chave e texto pronto", campo.nome, campo.texto)
		}
	}
	for _, action := range payload.Actions {
		if action.Label.Key == "" || action.Label.Fallback == "" {
			t.Errorf("ação %q = %+v, quer chave e fallback", action.ID, action.Label)
		}
	}
	if got := payload.Title.Params["method"]; got != "DELETE" {
		t.Errorf("método no título = %v, quer DELETE", got)
	}
	if !strings.Contains(payload.Body, "https://api.exemplo/itens/7") {
		t.Errorf("body = %q, quer a URL do pedido", payload.Body)
	}
	if !strings.Contains(payload.Body, "DELETE") {
		t.Errorf("body = %q, quer o método", payload.Body)
	}
	if payload.BodyLabel.Key != "app.questionnaire.http.bodyLabel" {
		t.Errorf("bodyLabel = %+v, quer nome traduzível da ilha", payload.BodyLabel)
	}
}

func TestAcaoDeShellContinuaSendoARespondida(t *testing.T) {
	payload := shellConfirmationPayload(shelltrust.PromptRequest{Command: "ls", Directory: ".", AvailableScopes: allAuthorizationScopesForTest()})
	actions := make(map[string]questionnaire.DecisionAction, len(payload.Actions))
	for _, action := range payload.Actions {
		actions[action.ID] = action
	}
	if _, allowOK := actions["once"]; !allowOK {
		t.Errorf("ações = %+v, quer allow e deny", payload.Actions)
	}
	if _, denyOK := actions[decisionDeny]; !denyOK {
		t.Errorf("ações = %+v, quer allow e deny", payload.Actions)
	}
	if allow := actions["once"]; allow.Polarity != questionnaire.DecisionPolarityAffirmative || allow.Scope != questionnaire.DecisionScopeCurrent {
		t.Errorf("allow sem semântica atual afirmativa: %+v", allow)
	}
	if deny := actions[decisionDeny]; deny.Polarity != questionnaire.DecisionPolarityNegative || deny.Scope != questionnaire.DecisionScopeCurrent {
		t.Errorf("deny sem semântica atual negativa: %+v", deny)
	}
}

func TestAcoesHTTPDeAprovacaoContinuamSendoAsRespondidas(t *testing.T) {
	payload := httpConfirmationPayload("POST", "https://api.exemplo", "{}")
	ids := make(map[string]bool, len(payload.Actions))
	for _, action := range payload.Actions {
		ids[action.ID] = true
	}
	if !ids[decisionAllow] || !ids[decisionDeny] {
		t.Errorf("ações = %+v, quer allow e deny", payload.Actions)
	}
}

func TestApprovedFromHTTPDecision(t *testing.T) {
	ok, err := approvedFromHTTPDecision(questionnaire.Response{
		Answers: map[string]any{questionnaire.AnswerActionID: decisionAllow},
	})
	if err != nil || !ok {
		t.Fatalf("allow: ok=%v err=%v", ok, err)
	}
	ok, err = approvedFromHTTPDecision(questionnaire.Response{
		Answers: map[string]any{questionnaire.AnswerActionID: decisionDeny},
	})
	if err != nil || ok {
		t.Fatalf("deny: ok=%v err=%v", ok, err)
	}
	ok, err = approvedFromHTTPDecision(questionnaire.Response{Cancelled: true})
	if err != nil || ok {
		t.Fatalf("cancel: ok=%v err=%v", ok, err)
	}
}

func TestShellResponseAcceptedBeforeExecutorDeadline(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		action                   any
		cancelled, afterDeadline bool
		want                     string
	}{
		{"negativa", "deny", false, false, shell.FailureCommandRefused},
		{"cancelar", nil, true, false, shell.FailureCommandRefused},
		{"aprovação", "once", false, false, shell.FailureConfirmationUnavailable},
		{"negativa não oferecida", "deny-global", false, false, shell.FailureConfirmationUnavailable},
		{"ação inválida", 12, false, false, shell.FailureConfirmationUnavailable},
		{"negativa tardia", "deny", false, true, shell.FailureConfirmationUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			synctest.Test(t, func(t *testing.T) {
				opened := make(chan string, 1)
				release := make(chan struct{})
				defer close(release)
				manager := questionnaire.NewManager(func(event string, data any) {
					if event == questionnaire.EventQuestionnaire {
						opened <- data.(map[string]any)["id"].(string)
						<-release
					}
				})
				router := questionnaire.NewRouter(func() *questionnaire.Manager { return manager }, nil)
				auth := shelltrust.Authorizer{Prompt: func(ctx context.Context, req shelltrust.PromptRequest) (shelltrust.PromptDecision, error) {
					return askShellConfirmation(ctx, router, questionnaire.DesktopSurface("chat"), req)
				}}
				reg := tools.NewRegistry()
				reg.MustRegister(shell.NewRunCommand(nil, auth.Authorize, nil, dir))
				cfg := tools.DefaultExecutorConfig()
				cfg.ToolTimeout = time.Second
				done := make(chan tools.ToolExecutionResult, 1)
				go func() {
					done <- tools.NewExecutor(reg, cfg).ExecuteOne(authorizationfixture.Context(), tools.ToolCall{ID: "dialog", Function: tools.FunctionCall{Name: "run_command", Arguments: `{"command":"example check"}`}})
				}()
				id := <-opened
				respond := func() error {
					return manager.Respond(id, map[string]any{questionnaire.AnswerActionID: tc.action}, tc.cancelled)
				}
				if !tc.afterDeadline {
					if err := respond(); err != nil {
						t.Fatalf("resposta não aceita: %v", err)
					}
					if err := manager.Respond(id, map[string]any{questionnaire.AnswerActionID: "deny"}, false); err == nil {
						t.Fatal("segunda resposta aceita")
					}
				}
				got := <-done
				if tc.afterDeadline && respond() == nil {
					t.Fatal("resposta aceita após o prazo")
				}
				if got.ErrorCode != tc.want || got.ErrorKind != tools.ErrorKindAuthorization || got.Retryable || !got.RetryabilityKnown || got.DurationMs != cfg.ToolTimeout.Milliseconds() {
					t.Fatalf("resultado=%+v", got)
				}
				if tc.want == shell.FailureCommandRefused && got.Error != nil || tc.want == shell.FailureConfirmationUnavailable && !errors.Is(got.Error, context.DeadlineExceeded) {
					t.Fatalf("erro da decisão: %v", got.Error)
				}
			})
		})
	}
}
