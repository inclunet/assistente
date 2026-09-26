package app

import (
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/workspace"
)

func TestWorkspaceVisualCommandFactsAppPage(t *testing.T) {
	snapshot := workspace.CommandSnapshot{Version: "version", WorkspaceProfile: "dev", Tab: workspace.CommandTabSnapshot{ID: "background", Type: workspace.TabTypeChat}}
	for _, tc := range []struct {
		name, page, surface, id  string
		route, requireID, denied bool
	}{
		{"workspace", "workspace", "chat", "background", false, true, false},
		{"profiles", "profiles", "profiles", "command-toolbar", true, false, false},
		{"settings", "settings", "toolbar", "command-toolbar", true, false, false},
		{"missing", "", "chat", "background", false, false, true},
		{"unknown", "unknown", "toolbar", "command-toolbar", true, false, true},
		{"wrong-route-pair", "profiles", "toolbar", "command-toolbar", true, false, true},
		{"tab-cannot-claim-route", "profiles", "chat", "background", false, false, true},
		{"route-cannot-claim-tab-id", "profiles", "profiles", "command-toolbar", true, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			proof := &localCommandKeyboardContextProof{snapshot: snapshot, routePage: tc.route, observed: LocalCommandKeyboardContext{AppPage: tc.page, SurfaceType: tc.surface, SurfaceID: tc.id, Profile: "dev"}}
			required := []commandbindings.Field{commandbindings.AppPage, commandbindings.Profile}
			if tc.requireID {
				required = append(required, commandbindings.SurfaceID)
			}
			origin, err := workspaceVisualCommandFacts(proof, required)
			if tc.denied {
				if err == nil {
					t.Fatal("invalid page proof admitted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if origin.facts[commandbindings.AppPage] != tc.page || origin.facts[commandbindings.SurfaceType] != tc.surface || origin.facts[commandbindings.Profile] != "dev" {
				t.Fatalf("unexpected facts: %+v", origin.facts)
			}
			if tc.route {
				if _, exists := origin.facts[commandbindings.SurfaceID]; exists {
					t.Fatal("background tab leaked into page facts")
				}
			}
		})
	}
}

func TestContextualPagePaletteAppPageReachesExecution(t *testing.T) {
	a, events, _, list := pagePaletteTaskFixture(t, workspace.TabTypeChat)
	view := configurePagePaletteCommand(t, a, events, "tasklists.duplicate", "tasklists", CommandSettingsConditionClause{Field: "app.page", Value: "tasklists"})
	for _, page := range []string{"", "unknown", "profiles", "workspace"} {
		result, err := a.BeginContextualPagePaletteUICommand(view.Generation, "tasklists.duplicate", LocalCommandKeyboardContext{SurfaceType: "tasklists", Profile: "dev", AppPage: page})
		if err == nil || result.Ticket != "" {
			t.Fatalf("page %q admitted: %+v %v", page, result, err)
		}
	}
	target, err := a.ReadTaskListCommandTarget(list.ID)
	if err != nil {
		t.Fatal(err)
	}
	r, err := a.BeginContextualPagePaletteUICommand(view.Generation, "tasklists.duplicate", LocalCommandKeyboardContext{SurfaceType: "tasklists", Profile: "dev", AppPage: "tasklists"})
	if err != nil {
		t.Fatal(err)
	}
	if err = a.PreparePageMutationCommand(r.Ticket, CommandPageMutationRequest{TargetID: list.ID, ExpectedFingerprint: target.Fingerprint, Title: "Page-conditioned copy"}); err != nil {
		t.Fatal(err)
	}
	handoff := takeUICommandFor(t, a, r.Ticket, r.CommandID)
	if err = a.CommitWorkspaceTabCommand(r.Ticket, handoff.HandoffID); err != nil {
		t.Fatal(err)
	}
	assertSurfacePaletteSucceeded(t, a, r.Ticket, r.InvocationID)
	result, err := a.GetPageMutationCommandResult(r.Ticket)
	if err != nil || result.ID == "" || result.ID == list.ID {
		t.Fatalf("copy did not execute: %+v %v", result, err)
	}
}
