package app

import (
	httpclient "assistente/internal/tools/http"
	"context"
	"net"
	"testing"
	"time"

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

func TestRepositoryNetworkDecisionUsesDesktopWithoutConversation(t *testing.T) {
	a := &App{}
	asked := false
	a.questionnaireMgr = questionnaire.NewManager(func(event string, payload any) {
		if event == questionnaire.EventQuestionnaire {
			asked = true
			id := payload.(map[string]any)["id"].(string)
			if err := a.questionnaireMgr.Respond(id, nil, true); err != nil {
				t.Error(err)
			}
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, allowed, err := a.repositoryNetworkAuthorizer().Authorize(ctx, httpclient.BlockedDestination{
		Host: "store.example", Port: "443", PortExplicit: true, IPs: []net.IP{net.IPv4(10, 0, 0, 1)},
	})
	if err != nil || allowed || !asked {
		t.Fatalf("asked=%v allowed=%v err=%v", asked, allowed, err)
	}
}
