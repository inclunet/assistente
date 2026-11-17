package questionnaire

import (
	"assistente/internal/authorization"
	"testing"
)

func TestAuthorizationActionsRejectUnavailableAndHumanLabels(t *testing.T) {
	options := AuthorizationActions{Scopes: []authorization.Scope{authorization.ScopeOnce, authorization.ScopeSession}, Directory: true, RememberDenial: true}
	for _, id := range []string{"global", "dir-global", "deny-profile", "deny-once", "session — Durante esta conversa", "Durante esta conversa", "banana", ""} {
		if _, err := options.Parse(Response{Answers: map[string]any{AnswerActionID: id}}); err == nil {
			t.Errorf("aceitou ação não oferecida %q", id)
		}
	}
	for _, tc := range []struct {
		id        string
		scope     authorization.Scope
		effect    string
		directory bool
	}{
		{"once", authorization.ScopeOnce, "allow", false},
		{"session", authorization.ScopeSession, "allow", false},
		{"dir-session", authorization.ScopeSession, "allow", true},
		{"deny-session", authorization.ScopeSession, "deny", false},
		{"deny", authorization.ScopeOnce, "deny", false},
		{"  deny  ", authorization.ScopeOnce, "deny", false},
	} {
		choice, err := options.Parse(Response{Answers: map[string]any{AnswerActionID: tc.id}})
		if err != nil || choice.Scope != tc.scope || choice.Effect != tc.effect || choice.Directory != tc.directory {
			t.Errorf("%s: %+v %v", tc.id, choice, err)
		}
	}
	choice, err := options.Parse(Response{Cancelled: true})
	if err != nil || choice.Effect != "deny" || choice.Scope != authorization.ScopeOnce {
		t.Fatalf("cancelamento: %+v %v", choice, err)
	}
	for _, action := range options.Actions() {
		if action.ID == "dir-global" || action.ID == "deny-profile" {
			t.Fatalf("expôs escopo ausente: %s", action.ID)
		}
		if action.Label.Key == "" || action.Label.Fallback == "" {
			t.Fatalf("ação sem tradução: %+v", action)
		}
	}
}

func TestAuthorizationShortcutHasNoCollisions(t *testing.T) {
	for _, persistent := range []authorization.Scope{authorization.ScopeWorkspace, authorization.ScopeGlobal} {
		actions := (AuthorizationActions{Scopes: []authorization.Scope{authorization.ScopeOnce, authorization.ScopeSession, authorization.ScopeWorkspace, authorization.ScopeProfile, authorization.ScopeGlobal}, Directory: true, RememberDenial: true, PersistentShortcut: persistent}).Actions()
		seen := map[string]bool{}
		for _, action := range actions {
			if action.Scope == "" {
				continue
			}
			key := string(action.Polarity) + ":" + string(action.Scope)
			if seen[key] {
				t.Fatalf("atalho duplicado %s: %s", key, action.ID)
			}
			seen[key] = true
		}
	}
}
