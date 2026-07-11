package filesystem

import (
	"context"
	"strings"
	"testing"

	"assistente/internal/questionnaire"
)

// rejectReasonFakeRequester devolve uma resposta pré-configurada com answers,
// registrando o payload recebido.
type rejectReasonFakeRequester struct {
	calls    []questionnaire.RequestPayload
	response questionnaire.Response
}

func (f *rejectReasonFakeRequester) RequestQuestionnaire(_ context.Context, payload questionnaire.RequestPayload) (questionnaire.Response, error) {
	f.calls = append(f.calls, payload)
	return f.response, nil
}

func TestConfirmEditWithDiff_PayloadIncludesRejectReason(t *testing.T) {
	quest := &rejectReasonFakeRequester{response: questionnaire.Response{}}

	ok, result := confirmEditWithDiff(context.Background(), quest, "Título", "Descrição", "antes", "depois")
	if !ok || result.IsError {
		t.Fatalf("aprovação deve retornar ok sem erro: ok=%v result=%+v", ok, result)
	}

	if len(quest.calls) != 1 {
		t.Fatalf("questionário deve ser solicitado 1 vez, foi %d", len(quest.calls))
	}
	rr := quest.calls[0].RejectReason
	if rr == nil {
		t.Fatal("payload deve incluir RejectReason")
	}
	autoFocusByID := map[string]bool{}
	for _, q := range quest.calls[0].Questions {
		autoFocusByID[q.ID] = q.AutoFocus
	}
	if focus, ok := autoFocusByID["before"]; !ok {
		t.Error("payload deve incluir a pergunta 'before'")
	} else if focus {
		t.Error("bloco 'Antes' não deve ter AutoFocus")
	}
	if focus, ok := autoFocusByID["after"]; !ok {
		t.Error("payload deve incluir a pergunta 'after'")
	} else if !focus {
		t.Error("bloco 'Depois' deve ter AutoFocus para receber o foco inicial do diálogo")
	}
	if rr.ID != "reject_reason" {
		t.Errorf("RejectReason.ID deve ser 'reject_reason', obtido %q", rr.ID)
	}
	if rr.Label == "" {
		t.Error("RejectReason.Label não pode ser vazio")
	}
	if rr.MaxLen != rejectReasonMaxLen {
		t.Errorf("RejectReason.MaxLen deve espelhar rejectReasonMaxLen (%d), obtido %d", rejectReasonMaxLen, rr.MaxLen)
	}
}

func TestConfirmEditWithDiff_RejectedWithReason(t *testing.T) {
	quest := &rejectReasonFakeRequester{response: questionnaire.Response{
		Cancelled: true,
		Answers:   map[string]any{"reject_reason": "  Prefiro manter o parágrafo original.  "},
	}}

	ok, result := confirmEditWithDiff(context.Background(), quest, "Título", "Descrição", "antes", "depois")
	if ok {
		t.Fatal("rejeição deve retornar ok=false")
	}
	if !result.IsError {
		t.Fatal("rejeição deve produzir ToolResult com IsError=true")
	}
	if !strings.Contains(result.Content, "Alteração rejeitada pelo usuário") {
		t.Errorf("mensagem deve manter o prefixo de rejeição: %s", result.Content)
	}
	if !strings.Contains(result.Content, "Motivo informado: Prefiro manter o parágrafo original.") {
		t.Errorf("mensagem deve conter o motivo com espaços aparados: %s", result.Content)
	}
}

func TestConfirmEditWithDiff_RejectedWithoutReason(t *testing.T) {
	cases := map[string]map[string]any{
		"sem answers":          nil,
		"answers vazio":        {},
		"motivo em branco":     {"reject_reason": "   \n  "},
		"motivo de outro tipo": {"reject_reason": 42},
	}

	for name, answers := range cases {
		t.Run(name, func(t *testing.T) {
			quest := &rejectReasonFakeRequester{response: questionnaire.Response{
				Cancelled: true,
				Answers:   answers,
			}}

			ok, result := confirmEditWithDiff(context.Background(), quest, "Título", "Descrição", "antes", "depois")
			if ok {
				t.Fatal("rejeição deve retornar ok=false")
			}
			if result.Content != "Alteração rejeitada pelo usuário" {
				t.Errorf("sem motivo, mensagem deve ser a padrão: %q", result.Content)
			}
			if !result.IsError {
				t.Error("rejeição deve produzir IsError=true")
			}
		})
	}
}

func TestExtractRejectReason_TruncatesLongReason(t *testing.T) {
	long := strings.Repeat("é", rejectReasonMaxLen+500)
	got := extractRejectReason(map[string]any{"reject_reason": long})

	runes := []rune(got)
	if len(runes) != rejectReasonMaxLen+1 { // +1 pela elipse
		t.Fatalf("motivo truncado deve ter %d runes (com elipse), tem %d", rejectReasonMaxLen+1, len(runes))
	}
	if runes[len(runes)-1] != '…' {
		t.Error("motivo truncado deve terminar com elipse")
	}
	for _, r := range runes[:len(runes)-1] {
		if r != 'é' {
			t.Fatalf("truncamento quebrou rune UTF-8: encontrou %q", r)
		}
	}
}

func TestExtractRejectReason_PreservesLineBreaks(t *testing.T) {
	got := extractRejectReason(map[string]any{"reject_reason": "linha 1\nlinha 2"})
	if got != "linha 1\nlinha 2" {
		t.Errorf("quebras de linha internas devem ser preservadas: %q", got)
	}
}
