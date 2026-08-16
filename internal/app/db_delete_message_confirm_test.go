package app

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/questionnaire"
)

// appComResposta devolve um App cujo questionário responde sempre o mesmo.
func appComResposta(t *testing.T, answers map[string]any, cancelled bool) *App {
	t.Helper()

	var mgr *questionnaire.Manager
	mgr = questionnaire.NewManager(func(_ string, data any) {
		dataMap, ok := data.(map[string]any)
		if !ok {
			t.Errorf("payload do evento = %T, quer map", data)
			return
		}
		id, _ := dataMap["id"].(string)
		go func() {
			if err := mgr.Respond(id, answers, cancelled); err != nil {
				t.Errorf("Respond: %v", err)
			}
		}()
	})

	return &App{ctx: context.Background(), questionnaireMgr: mgr}
}

func TestExclusaoDeMensagemSeguePelaAcaoExcluir(t *testing.T) {
	a := appComResposta(t, map[string]any{questionnaire.AnswerActionID: "delete"}, false)

	if err := a.confirmDeleteMessageQuestionnaire(); err != nil {
		t.Fatalf("confirmDeleteMessageQuestionnaire = %v, quer nil", err)
	}
}

func TestExclusaoDeMensagemParaNaAcaoCancelar(t *testing.T) {
	a := appComResposta(t, map[string]any{questionnaire.AnswerActionID: "cancel"}, false)

	err := a.confirmDeleteMessageQuestionnaire()
	if err == nil {
		t.Fatal("confirmDeleteMessageQuestionnaire = nil, quer recusa")
	}
	if !strings.Contains(err.Error(), "cancelada pelo usuário") {
		t.Errorf("erro = %q, quer falar em cancelamento do usuário", err)
	}
}

// Resposta sem actionId é contrato quebrado, não decisão de quem usa: a
// exclusão continua barrada, mas o erro precisa dizer o que de fato houve para
// o defeito não passar por "o usuário desistiu".
func TestExclusaoDeMensagemDistingueRespostaInvalidaDeCancelamento(t *testing.T) {
	a := appComResposta(t, map[string]any{}, false)

	err := a.confirmDeleteMessageQuestionnaire()
	if err == nil {
		t.Fatal("confirmDeleteMessageQuestionnaire = nil, quer erro")
	}
	if strings.Contains(err.Error(), "cancelada pelo usuário") {
		t.Errorf("erro = %q, quer distinguir resposta inválida de cancelamento", err)
	}
	if !strings.Contains(err.Error(), questionnaire.AnswerActionID) {
		t.Errorf("erro = %q, quer nomear o campo ausente", err)
	}
}
