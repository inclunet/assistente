package app

import (
	"testing"

	"assistente/internal/nettrust"
	"assistente/internal/questionnaire"
)

func TestNetworkDecisionActionsDeclaramAtalhosSemanticosSemColisao(t *testing.T) {
	actions := networkConfirmationPayload(nettrust.PromptRequest{AvailableScopes: allAuthorizationScopesForTest()}).Actions
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
