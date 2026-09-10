package app

import (
	"testing"

	"assistente/internal/nettrust"
	"assistente/internal/questionnaire"
)

// scopeFromActionID resolve só pelo id estável do DecisionDialog — o rótulo
// traduzido nunca volta ao backend (AEP-0085 / AEP-0091).
func TestScopeFromActionID(t *testing.T) {
	cases := []struct {
		name     string
		actionID string
		want     nettrust.Scope
		ok       bool
	}{
		{"once", "once", nettrust.ScopeOnce, true},
		{"session", "session", nettrust.ScopeSession, true},
		{"workspace", "workspace", nettrust.ScopeWorkspace, true},
		{"profile", "profile", nettrust.ScopeProfile, true},
		{"global", "global", nettrust.ScopeGlobal, true},
		{"com espaços", "  workspace  ", nettrust.ScopeWorkspace, true},
		{"legado rádio+rótulo", "session — Durante esta conversa", "", false},
		{"só rótulo humano", "Durante esta conversa", "", false},
		{"desconhecido", "banana", "", false},
		{"vazio", "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := scopeFromActionID(tc.actionID)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("scopeFromActionID(%q) = (%q, %v), want (%q, %v)", tc.actionID, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// Toda ação de escopo apresentada ao usuário deve ser parseável de volta.
func TestNetworkDecisionActionsRoundTrip(t *testing.T) {
	for _, action := range networkDecisionActions() {
		if action.ID == decisionDeny {
			continue
		}
		got, ok := scopeFromActionID(action.ID)
		if !ok || string(got) != action.ID {
			t.Errorf("round-trip falhou para %q: (%q, %v)", action.ID, got, ok)
		}
	}
}

func TestNetworkDecisionActionsDeclaramAtalhosSemanticosSemColisao(t *testing.T) {
	actions := networkDecisionActions()
	byID := make(map[string]questionnaire.DecisionAction, len(actions))
	for _, action := range actions {
		byID[action.ID] = action
	}

	cases := []struct {
		id       string
		polarity questionnaire.DecisionPolarity
		scope    questionnaire.DecisionScope
	}{
		{"once", questionnaire.DecisionPolarityAffirmative, questionnaire.DecisionScopeCurrent},
		{"session", questionnaire.DecisionPolarityAffirmative, questionnaire.DecisionScopeConversation},
		{"global", questionnaire.DecisionPolarityAffirmative, questionnaire.DecisionScopeGlobal},
		{decisionDeny, questionnaire.DecisionPolarityNegative, questionnaire.DecisionScopeCurrent},
	}
	for _, tc := range cases {
		action := byID[tc.id]
		if action.Polarity != tc.polarity || action.Scope != tc.scope {
			t.Errorf("%s = polarity %q scope %q, quer %q/%q", tc.id, action.Polarity, action.Scope, tc.polarity, tc.scope)
		}
	}
	for _, id := range []string{"workspace", "profile"} {
		if action := byID[id]; action.Scope != "" {
			t.Errorf("%s não deve disputar o chord persistente: %+v", id, action)
		}
	}
}
