package app

import (
	"testing"
	"time"

	"assistente/internal/commandbindings"
	"assistente/internal/database"
	"assistente/internal/workspace"
)

func TestContextualDeckPageFactsDoNotBorrowBackgroundSurface(t *testing.T) {
	proof := &localCommandKeyboardContextProof{
		observed: LocalCommandKeyboardContext{SurfaceType: "profiles", Profile: "override"},
		snapshot: workspace.CommandSnapshot{WorkspaceProfile: "workspace", Tab: workspace.CommandTabSnapshot{ID: "background-chat", Type: workspace.TabTypeChat, ProfileOverrideSlug: "override"}},
	}
	origin, err := deckPageCommandFacts(proof, []commandbindings.Field{commandbindings.AppFocused, commandbindings.SurfaceType, commandbindings.Profile})
	if err != nil || origin.facts[commandbindings.SurfaceType] != "profiles" || origin.facts[commandbindings.Profile] != "override" {
		t.Fatalf("page facts: %+v %v", origin, err)
	}
	if _, exists := origin.facts[commandbindings.SurfaceID]; exists {
		t.Fatal("page leaked background tab ID")
	}
	for _, unsupported := range []commandbindings.Field{commandbindings.SurfaceID, commandbindings.Process, commandbindings.Device} {
		if _, err := deckPageCommandFacts(proof, []commandbindings.Field{unsupported}); err == nil {
			t.Fatalf("accepted unsupported fact %s", unsupported)
		}
	}
	proof.observed.Profile = "workspace"
	if _, err := deckPageCommandFacts(proof, []commandbindings.Field{commandbindings.Profile}); err == nil {
		t.Fatal("ignored canonical profile override")
	}
	proof.observed.SurfaceID = "background-chat"
	if _, err := deckPageCommandFacts(proof, nil); err == nil {
		t.Fatal("accepted a page surface ID")
	}
}

func TestContextualDeckPageSurfaceAllowlist(t *testing.T) {
	allowed := map[string]map[string]bool{
		"profiles.duplicate": {"profiles": true}, "profiles.delete": {"profiles": true}, "profiles.activate": {"profiles": true},
		"tasklists.duplicate": {"tasklists": true, "tasklist": true}, "tasklists.delete": {"tasklists": true}, "tasklists.clear": {"tasklists": true, "tasklist": true},
		"profiles.create": {}, "profiles.update": {}, "tasklists.create": {}, "tasklists.update": {}, "layers.toggle": {},
	}
	for command, surfaces := range allowed {
		for _, surface := range []string{"", "profiles", "tasklists", "tasklist", "chat", "editor", "terminal", "toolbar", "modal"} {
			if got := deckPageCommandSurface(command, surface); got != surfaces[surface] {
				t.Errorf("%s on %s: got %v", command, surface, got)
			}
		}
	}
}

func TestContextualDeckPageMapRetiredBeforeCommit(t *testing.T) {
	for _, mode := range []string{"expired", "replaced"} {
		t.Run(mode, func(t *testing.T) {
			a, decisions, ctx, list := pagePaletteTaskFixture(t, workspace.TabTypeChat)
			configurePageDeckCommand(t, a, decisions, "tasklists.duplicate", "tasklists")
			target, err := a.ReadTaskListCommandTarget(list.ID)
			if err != nil {
				t.Fatal(err)
			}
			event, _ := pageDeckOffer(t, a)
			r := beginPageDeckCommand(t, a, event, "tasklists.duplicate", "tasklists")
			if err := a.PreparePageMutationCommand(r.Ticket, CommandPageMutationRequest{TargetID: list.ID, ExpectedFingerprint: target.Fingerprint, Title: "Must not exist"}); err != nil {
				t.Fatal(err)
			}
			h := takeUICommandFor(t, a, r.Ticket, r.CommandID)
			if mode == "expired" {
				p := a.commandProduct.Load()
				p.keyboardMu.Lock()
				p.keyboardMap.view.ValidUntil = time.Now().Add(-time.Second).UnixMilli()
				p.keyboardMu.Unlock()
			} else {
				a.ResetLocalCommandKeyboard(event.Generation)
				if _, err := a.GetLocalCommandKeyboardMap(); err != nil {
					t.Fatal(err)
				}
			}
			if err := a.CommitWorkspaceTabCommand(r.Ticket, h.HandoffID); err == nil {
				t.Fatal("retired page map committed")
			}
			_ = a.CancelUICommand(r.Ticket)
			assertPageDeckCleanup(t, a, r.Ticket)
			lists, err := database.GetAllTaskListsWithContext(ctx)
			if err != nil || len(lists) != 1 {
				t.Fatalf("unexpected clone: %v %v", lists, err)
			}
		})
	}
}
