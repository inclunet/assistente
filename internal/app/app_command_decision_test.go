package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/commanddecision"
	"assistente/internal/questionnaire"
)

func commandDecisionRequest(id string, expiresIn time.Duration) commanddecision.Request {
	return commanddecision.Request{
		DecisionID:         id,
		MutationID:         "mutation-1",
		UserID:             "user-1",
		SessionID:          "session-1",
		Fingerprint:        "fingerprint-1",
		AuthGeneration:     "auth-generation-1",
		SecurityGeneration: "security-generation-1",
		ExpiresAt:          time.Now().Add(expiresIn),
		Body:               "trigger: Ctrl+Shift+K\ncommand: {{trusted-body}}",
	}
}

func newCommandDecisionManager(t *testing.T) (*questionnaire.Manager, <-chan map[string]any) {
	t.Helper()
	events := make(chan map[string]any, 4)
	var manager *questionnaire.Manager
	manager = questionnaire.NewManager(func(event string, data any) {
		if event != questionnaire.EventQuestionnaire {
			return
		}
		payload, ok := data.(map[string]any)
		if !ok {
			t.Errorf("payload do evento = %T, want map[string]any", data)
			return
		}
		events <- payload
	})
	return manager, events
}

func receiveCommandDecisionEvent(t *testing.T, events <-chan map[string]any) map[string]any {
	t.Helper()
	select {
	case payload := <-events:
		return payload
	case <-time.After(5 * time.Second):
		t.Fatal("o diálogo de decisão não foi emitido")
		return nil
	}
}

func finishCommandDecision(t *testing.T, manager *questionnaire.Manager, payload map[string]any, answers map[string]any, cancelled bool) {
	t.Helper()
	uiID, ok := payload["id"].(string)
	if !ok || uiID == "" {
		t.Fatalf("id de correlação da UI inválido: %#v", payload["id"])
	}
	if err := manager.Respond(uiID, answers, cancelled); err != nil {
		t.Fatalf("Respond: %v", err)
	}
}

func presentCommandDecision(t *testing.T, presenter *commandDecisionPresenter, manager *questionnaire.Manager, events <-chan map[string]any, req commanddecision.Request, answers map[string]any, cancelled bool) (commanddecision.Response, error, map[string]any) {
	t.Helper()
	type outcome struct {
		response commanddecision.Response
		err      error
	}
	resultCh := make(chan outcome, 1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		response, err := presenter.Present(ctx, req)
		resultCh <- outcome{response: response, err: err}
	}()
	payload := receiveCommandDecisionEvent(t, events)
	finishCommandDecision(t, manager, payload, answers, cancelled)
	var got outcome
	select {
	case got = <-resultCh:
	case <-time.After(5 * time.Second):
		t.Fatal("Present não concluiu em 5s")
	}
	return got.response, got.err, payload
}

func TestCommandDecisionPresenterBuildsAccessibleDecisionPayload(t *testing.T) {
	manager, events := newCommandDecisionManager(t)
	presenter := &commandDecisionPresenter{manager: manager}
	req := commandDecisionRequest("backend-decision-1", 5*time.Second)

	response, err, payload := presentCommandDecision(t, presenter, manager, events, req,
		map[string]any{
			questionnaire.AnswerActionID: "attacker-injected-action-id",
			"decisionId":                 "attacker-injected-decision-id",
		}, false)
	if err == nil {
		t.Fatal("ação desconhecida deveria falhar")
	}
	if !errors.Is(err, commanddecision.ErrInvalid) {
		t.Fatalf("erro = %v, want commanddecision.ErrInvalid", err)
	}

	if payload["kind"] != questionnaire.KindDecision {
		t.Fatalf("kind = %#v, want %q", payload["kind"], questionnaire.KindDecision)
	}
	if payload["body"] != req.Body {
		t.Fatalf("body = %#v, want conteúdo cru integral", payload["body"])
	}
	if payload["allowCancel"] != true {
		t.Fatalf("allowCancel = %#v, want true", payload["allowCancel"])
	}
	if payload["severity"] != questionnaire.DecisionSeverityPermission {
		t.Fatalf("severity = %#v, want permission", payload["severity"])
	}
	if payload["id"] == req.DecisionID {
		t.Fatalf("id da UI não pode ser o decision ID backend: %#v", payload["id"])
	}

	title := payload["title"].(questionnaire.Text)
	description := payload["description"].(questionnaire.Text)
	bodyLabel := payload["bodyLabel"].(questionnaire.Text)
	if title.Key != "app.questionnaire.commandBinding.title" || description.Key != "app.questionnaire.commandBinding.description" || bodyLabel.Key != "app.questionnaire.commandBinding.bodyLabel" {
		t.Fatalf("chaves de tradução = title=%#v description=%#v bodyLabel=%#v", title, description, bodyLabel)
	}
	actions := payload["actions"].([]questionnaire.DecisionAction)
	if len(actions) != 2 {
		t.Fatalf("ações = %#v, want apply e deny", actions)
	}
	if actions[0].ID != commanddecision.ApplyAction || !actions[0].Primary || actions[0].Polarity != questionnaire.DecisionPolarityAffirmative || actions[0].Scope != questionnaire.DecisionScopePersistent {
		t.Fatalf("ação apply = %#v", actions[0])
	}
	if actions[1].ID != commanddecision.DenyAction || actions[1].Polarity != questionnaire.DecisionPolarityNegative || actions[1].Scope != questionnaire.DecisionScopeCurrent {
		t.Fatalf("ação deny = %#v", actions[1])
	}
	if questions, ok := payload["questions"].([]questionnaire.Question); !ok || len(questions) != 0 {
		t.Fatalf("questions = %#v, want vazio", payload["questions"])
	}
	if submitLabel, ok := payload["submitLabel"].(questionnaire.Text); !ok || !submitLabel.IsZero() {
		t.Fatalf("submitLabel = %#v, want zero", payload["submitLabel"])
	}
	if response != (commanddecision.Response{}) {
		t.Fatalf("resposta de ação desconhecida = %#v, want zero", response)
	}
}

func TestCommandDecisionPresenterAcceptDenyCancelAndIgnoresInjectedDecisionID(t *testing.T) {
	tests := []struct {
		name      string
		answers   map[string]any
		cancelled bool
		want      commanddecision.Response
		wantError error
	}{
		{name: "apply", answers: map[string]any{questionnaire.AnswerActionID: commanddecision.ApplyAction, "decisionId": "wrong"}, want: commanddecision.Response{DecisionID: "backend-apply", ActionID: commanddecision.ApplyAction}},
		{name: "deny", answers: map[string]any{questionnaire.AnswerActionID: commanddecision.DenyAction, "decisionId": "wrong"}, want: commanddecision.Response{DecisionID: "backend-deny", ActionID: commanddecision.DenyAction}},
		{name: "esc", answers: map[string]any{"decisionId": "wrong"}, cancelled: true, want: commanddecision.Response{DecisionID: "backend-esc", Cancelled: true}},
		{name: "unknown", answers: map[string]any{questionnaire.AnswerActionID: " APPLY "}, wantError: commanddecision.ErrInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, events := newCommandDecisionManager(t)
			presenter := &commandDecisionPresenter{manager: manager}
			req := commandDecisionRequest("backend-"+tt.name, 5*time.Second)
			response, err, _ := presentCommandDecision(t, presenter, manager, events, req, tt.answers, tt.cancelled)
			if tt.wantError != nil {
				if !errors.Is(err, tt.wantError) {
					t.Fatalf("erro = %v, want %v", err, tt.wantError)
				}
				return
			}
			if err != nil {
				t.Fatalf("Present: %v", err)
			}
			if response != tt.want {
				t.Fatalf("resposta = %#v, want %#v", response, tt.want)
			}
		})
	}
}

func TestCommandDecisionPresenterExpiryCoversQuestionnaireQueue(t *testing.T) {
	manager, events := newCommandDecisionManager(t)
	presenter := &commandDecisionPresenter{manager: manager}
	firstCtx, cancelFirst := context.WithCancel(context.Background())
	defer cancelFirst()
	firstResult := make(chan error, 1)
	go func() {
		_, err := presenter.Present(firstCtx, commandDecisionRequest("first", 5*time.Second))
		firstResult <- err
	}()
	firstPayload := receiveCommandDecisionEvent(t, events)

	secondResult := make(chan error, 1)
	go func() {
		_, err := presenter.Present(context.Background(), commandDecisionRequest("second", 40*time.Millisecond))
		secondResult <- err
	}()
	select {
	case err := <-secondResult:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("erro da fila = %v, want deadline exceeded", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Present ficou aguardando além da expiração durante a fila")
	}
	select {
	case extra := <-events:
		t.Fatalf("segundo diálogo foi emitido após expirar na fila: %#v", extra)
	default:
	}

	finishCommandDecision(t, manager, firstPayload, nil, true)
	select {
	case err := <-firstResult:
		if err != nil {
			t.Fatalf("primeira decisão cancelada: %v", err)
		}
	case <-time.After(5 * time.Second):
		cancelFirst()
		t.Fatal("primeira apresentação não concluiu em 5s")
	}
}

func TestCommandDecisionPresenterGuardsAndPropagatesManagerError(t *testing.T) {
	req := commandDecisionRequest("guarded", 5*time.Second)
	if _, err := (*commandDecisionPresenter)(nil).Present(context.Background(), req); !errors.Is(err, commanddecision.ErrInvalid) {
		t.Fatalf("receiver nil = %v, want ErrInvalid", err)
	}
	if _, err := (&commandDecisionPresenter{}).Present(context.Background(), req); !errors.Is(err, commanddecision.ErrInvalid) {
		t.Fatalf("manager nil = %v, want ErrInvalid", err)
	}
	manager, events := newCommandDecisionManager(t)
	presenter := &commandDecisionPresenter{manager: manager}
	if _, err := presenter.Present(nil, req); !errors.Is(err, commanddecision.ErrInvalid) {
		t.Fatalf("context nil = %v, want ErrInvalid", err)
	}
	if _, err := presenter.Present(context.Background(), commandDecisionRequest("expired", -time.Second)); !errors.Is(err, commanddecision.ErrInvalid) {
		t.Fatalf("request expirado = %v, want ErrInvalid", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := presenter.Present(cancelled, req)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("erro do manager = %v, want context.Canceled", err)
	}
	select {
	case payload := <-events:
		t.Fatalf("contexto já cancelado emitiu diálogo: %#v", payload)
	default:
	}
}
