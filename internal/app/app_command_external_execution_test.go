package app

import (
	"testing"
	"time"

	"assistente/internal/commandcatalog"
)

func TestExternalCommandAuthorizationOnlyAllowsWorkspaceListBackend(t *testing.T) {
	definitions := []commandcatalog.Definition{
		{ID: commandProductWorkspaceListID, AllowedSources: []commandcatalog.Source{commandcatalog.UI}, HandlerClassification: commandcatalog.HandlerBackend},
		{ID: "workspace.create", AllowedSources: []commandcatalog.Source{commandcatalog.UI}, HandlerClassification: commandcatalog.HandlerBackend},
		{ID: "future.backend.read", AllowedSources: []commandcatalog.Source{commandcatalog.UI}, HandlerClassification: commandcatalog.HandlerBackend},
	}
	rules := externalCommandAuthorizationRules(definitions)
	if len(rules) != 1 || rules[0].CommandID != commandProductWorkspaceListID {
		t.Fatalf("external backend authorization rules = %#v, want only %q", rules, commandProductWorkspaceListID)
	}
}

func TestExternalCommandHTTPDeadlinesIncludeBoundedAuthAndFinalizationMargin(t *testing.T) {
	if externalCommandHTTPExecutionTimeout != 5*time.Minute {
		t.Fatalf("execution timeout = %s, want 5m", externalCommandHTTPExecutionTimeout)
	}
	if externalCommandHTTPWriteMargin != 30*time.Second {
		t.Fatalf("HTTP write margin = %s, want bounded 30s for auth, finalization, and response headroom", externalCommandHTTPWriteMargin)
	}
}
