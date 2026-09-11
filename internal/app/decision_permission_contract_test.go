package app

import (
	"testing"

	"assistente/internal/fstrust"
	"assistente/internal/nettrust"
	"assistente/internal/questionnaire"
)

// Fase 4 do AEP-0091: permissões críticas não podem voltar ao híbrido
// rádio + Confirmar/Negar. O contrato é kind=decision com Actions.
func assertDecisionOnly(t *testing.T, nome string, payload questionnaire.RequestPayload) {
	t.Helper()
	assertDecisionOnlyShape(t, nome, payload)
	hasAffirmativeCurrent := false
	hasNegativeCurrent := false
	for _, action := range payload.Actions {
		hasAffirmativeCurrent = hasAffirmativeCurrent ||
			(action.Polarity == questionnaire.DecisionPolarityAffirmative &&
				action.Scope == questionnaire.DecisionScopeCurrent)
		hasNegativeCurrent = hasNegativeCurrent ||
			(action.Polarity == questionnaire.DecisionPolarityNegative &&
				action.Scope == questionnaire.DecisionScopeCurrent)
	}
	if !hasAffirmativeCurrent || !hasNegativeCurrent {
		t.Errorf("%s: decisão precisa expor par semântico atual afirmativo/negativo", nome)
	}
}

func assertDecisionOnlyShape(t *testing.T, nome string, payload questionnaire.RequestPayload) {
	t.Helper()
	if payload.Kind != questionnaire.KindDecision {
		t.Errorf("%s: kind = %q, quer %q", nome, payload.Kind, questionnaire.KindDecision)
	}
	if len(payload.Actions) == 0 {
		t.Errorf("%s: actions vazias; decisão precisa de botões", nome)
	}
	for _, q := range payload.Questions {
		// Permissão crítica = só botões (+ readonly_code no body, se houver).
		// text/long_text/boolean/single_choice reabririam o híbrido.
		if q.Type != "readonly_code" {
			t.Errorf("%s: pergunta %q tipo %q — permissão só admite readonly_code ou nenhuma", nome, q.ID, q.Type)
		}
	}
	if payload.SubmitLabel.Key != "" || payload.SubmitLabel.Fallback != "" {
		t.Errorf("%s: SubmitLabel = %+v; DecisionDialog não usa Confirmar", nome, payload.SubmitLabel)
	}
	if payload.CancelLabel.Key != "" || payload.CancelLabel.Fallback != "" {
		t.Errorf("%s: CancelLabel = %+v; a negativa é uma Action, não CancelLabel", nome, payload.CancelLabel)
	}
}

func TestPermissoesCriticasUsamSoDecisionDialog(t *testing.T) {
	assertDecisionOnly(t, "shell", shellConfirmationPayload("echo ok", "C:/tmp"))
	assertDecisionOnly(t, "http", httpConfirmationPayload("POST", "https://exemplo/api", `{"a":1}`))
	// Mesmo builder que PromptNetworkAuthorization envia ao questionnaire —
	// montar o payload à mão deixaria o prompter livre para regressar.
	assertDecisionOnly(t, "rede", networkConfirmationPayload(nettrust.PromptRequest{
		Host:     "interno.local",
		Category: "private",
		Reason:   "teste",
	}))
	// Filesystem tem alternativas de path/pasta que não cabem sem colisão na
	// família universal; o builder possui teste específico para seus slots.
	payloadFS := pathConfirmationPayload(fstrust.PromptRequest{
		Path: "C:/tmp/contrato.txt", Operation: "write",
	})
	assertDecisionOnlyShape(t, "filesystem", payloadFS)
}
