package app

import (
	"testing"

	"assistente/internal/fstrust"
	"assistente/internal/questionnaire"
)

func TestPathConfirmationPayloadIsDecision(t *testing.T) {
	payload := pathConfirmationPayload(fstrust.PromptRequest{AvailableScopes: allAuthorizationScopesForTest(),
		Path:      "C:/tmp/a.txt",
		Operation: "read",
	})
	if payload.Kind != questionnaire.KindDecision {
		t.Fatalf("kind=%q want decision", payload.Kind)
	}
	if payload.BodyLabel.Key != "app.questionnaire.fstrust.bodyLabel" ||
		payload.BodyLabel.Fallback != "Caminho e detalhes solicitados" {
		t.Fatalf("bodyLabel=%+v; esperava nome traduzível da ilha de path", payload.BodyLabel)
	}
	if len(payload.Actions) < 3 {
		t.Fatalf("actions=%d; esperava escopos file+dir+deny", len(payload.Actions))
	}
	last := payload.Actions[len(payload.Actions)-1]
	if last.ID != "deny" {
		t.Fatalf("última ação=%q want deny", last.ID)
	}
	if !payload.Actions[0].Primary {
		t.Fatal("a primeira permissão segura deve ser primária antes das recusas")
	}
	expectedShortcuts := map[string]struct {
		polarity questionnaire.DecisionPolarity
		scope    questionnaire.DecisionScope
	}{
		"once":           {questionnaire.DecisionPolarityAffirmative, questionnaire.DecisionScopeCurrent},
		"session":        {questionnaire.DecisionPolarityAffirmative, questionnaire.DecisionScopeConversation},
		"workspace":      {questionnaire.DecisionPolarityAffirmative, questionnaire.DecisionScopePersistent},
		"deny-session":   {questionnaire.DecisionPolarityNegative, questionnaire.DecisionScopeConversation},
		"deny-workspace": {questionnaire.DecisionPolarityNegative, questionnaire.DecisionScopePersistent},
		"deny":           {questionnaire.DecisionPolarityNegative, questionnaire.DecisionScopeCurrent},
	}
	seenDenyWorkspace := false
	for _, action := range payload.Actions {
		if want, ok := expectedShortcuts[action.ID]; ok {
			if action.Polarity != want.polarity || action.Scope != want.scope {
				t.Errorf("%s = polarity %q scope %q, quer %q/%q", action.ID, action.Polarity, action.Scope, want.polarity, want.scope)
			}
		}
		// Pasta e escopos profile/global coexistem com os slots universais:
		// mantê-los sem metadata evita colisão; Alt+mnemônico continua ativo.
		if action.ID == "dir-once" || action.ID == "profile" || action.ID == "global" ||
			action.ID == "deny-profile" || action.ID == "deny-global" {
			if action.Scope != "" {
				t.Errorf("%s não deve ocupar chord universal: %+v", action.ID, action)
			}
		}
		if action.ID == "deny-workspace" {
			seenDenyWorkspace = true
		}
	}
	if !seenDenyWorkspace {
		t.Fatal("faltou ação de negar e lembrar no workspace")
	}
}
