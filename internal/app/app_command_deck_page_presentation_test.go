package app

import (
	"context"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commanddeck"
	"assistente/internal/commandexecution"
)

func TestCommandDeckPagePresentationSwitchesPageAndRejectsStaleRevisions(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	product := a.commandProduct.Load()
	keyboardMap, err := a.GetLocalCommandKeyboardMap()
	if err != nil || keyboardMap.Generation == "" {
		t.Fatalf("keyboard generation unavailable: generation=%q err=%v", keyboardMap.Generation, err)
	}

	if err := a.PublishCommandDeckPagePresentation("settings", keyboardMap.Generation, 10); err != nil {
		t.Fatalf("publish settings: %v", err)
	}
	if got := product.currentDeckPagePresentation(); got != "settings" {
		t.Fatalf("published page = %q, want settings", got)
	}
	if err := a.PublishCommandDeckPagePresentation("profiles", keyboardMap.Generation, 10); err == nil {
		t.Fatal("duplicate revision replaced the current page")
	}
	if err := a.PublishCommandDeckPagePresentation("settings.commands", keyboardMap.Generation, 11); err == nil {
		t.Fatal("unknown page was accepted")
	}
	if err := a.PublishCommandDeckPagePresentation("profiles", "019b4e53-5f14-7c23-8b55-442200000001", 11); err == nil {
		t.Fatal("stale keyboard generation was accepted")
	}
	if got := product.currentDeckPagePresentation(); got != "settings" {
		t.Fatalf("rejected updates changed page = %q", got)
	}
	if err := a.PublishCommandDeckPagePresentation("profiles", keyboardMap.Generation, 12); err != nil {
		t.Fatalf("publish profiles: %v", err)
	}
	if got := product.currentDeckPagePresentation(); got != "profiles" {
		t.Fatalf("page switch = %q, want profiles", got)
	}

	if err := a.ClearCommandDeckPagePresentation("019b4e53-5f14-7c23-8b55-442200000001", 13); err != nil {
		t.Fatalf("stale-generation clear: %v", err)
	}
	if got := product.currentDeckPagePresentation(); got != "profiles" {
		t.Fatalf("stale clear erased current page = %q", got)
	}
	if err := a.ClearCommandDeckPagePresentation(keyboardMap.Generation, 14); err != nil {
		t.Fatalf("clear current page: %v", err)
	}
	if got := product.currentDeckPagePresentation(); got != "" {
		t.Fatalf("clear retained page = %q", got)
	}
	if err := a.PublishCommandDeckPagePresentation("history", keyboardMap.Generation, 13); err == nil {
		t.Fatal("older revision resurrected a cleared page")
	}
}

func TestCommandDeckPagePresentationExpiresAndIsBoundToRuntimeIdentity(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	product := a.commandProduct.Load()
	keyboardMap, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.PublishCommandDeckPagePresentation("tasklists", keyboardMap.Generation, 20); err != nil {
		t.Fatal(err)
	}

	mutateSnapshot := func(change func(*commandDeckPagePresentationSnapshot)) {
		t.Helper()
		product.deckPagePresentationMu.Lock()
		defer product.deckPagePresentationMu.Unlock()
		change(product.deckPagePresentation)
	}
	revision := int64(20)
	for _, test := range []struct {
		name   string
		change func(*commandDeckPagePresentationSnapshot)
	}{{"wrong user", func(s *commandDeckPagePresentationSnapshot) { s.userID = "other-user" }},
		{"wrong session", func(s *commandDeckPagePresentationSnapshot) { s.sessionID = "other-session" }},
		{"wrong workspace", func(s *commandDeckPagePresentationSnapshot) { s.workspaceID = "other-workspace" }},
		{"wrong map generation", func(s *commandDeckPagePresentationSnapshot) { s.generation = "019b4e53-5f14-7c23-8b55-442200000001" }},
		{"expired lease", func(s *commandDeckPagePresentationSnapshot) { s.expiresAt = time.Now().Add(-time.Millisecond) }}} {
		t.Run(test.name, func(t *testing.T) {
			revision++
			if err := a.PublishCommandDeckPagePresentation("tasklists", keyboardMap.Generation, revision); err != nil {
				t.Fatal(err)
			}
			mutateSnapshot(test.change)
			if got := product.currentDeckPagePresentation(); got != "" {
				t.Fatalf("out-of-scope snapshot was rendered: %q", got)
			}
		})
	}

	if err := a.PublishCommandDeckPagePresentation("tasklists", keyboardMap.Generation, 30); err != nil {
		t.Fatal(err)
	}
	product.clearLocalCommandKeyboard(keyboardMap.Generation)
	if got := product.currentDeckPagePresentation(); got != "" {
		t.Fatalf("keyboard-map reset retained page visual: %q", got)
	}
	newMap, err := a.GetLocalCommandKeyboardMap()
	if err != nil || newMap.Generation == keyboardMap.Generation {
		t.Fatalf("new keyboard generation: old=%q new=%q err=%v", keyboardMap.Generation, newMap.Generation, err)
	}
	if err := a.PublishCommandDeckPagePresentation("tasklists", keyboardMap.Generation, 31); err == nil {
		t.Fatal("retired generation republished a page visual")
	}
}

func TestCommandDeckPagePresentationOldGenerationClearDoesNotConsumeCurrentRevision(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	product := a.commandProduct.Load()
	oldMap, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	if err := a.PublishCommandDeckPagePresentation("tasklists", oldMap.Generation, 100); err != nil {
		t.Fatal(err)
	}
	product.clearLocalCommandKeyboard(oldMap.Generation)
	newMap, err := a.GetLocalCommandKeyboardMap()
	if err != nil || newMap.Generation == oldMap.Generation {
		t.Fatalf("new map generation: old=%q new=%q err=%v", oldMap.Generation, newMap.Generation, err)
	}
	if err := a.PublishCommandDeckPagePresentation("profiles", newMap.Generation, 1); err != nil {
		t.Fatalf("new generation should start its own revision sequence: %v", err)
	}
	if err := a.ClearCommandDeckPagePresentation(oldMap.Generation, 101); err != nil {
		t.Fatalf("late clear from retired generation: %v", err)
	}
	if err := a.PublishCommandDeckPagePresentation("tasklists", newMap.Generation, 2); err != nil {
		t.Fatalf("old clear consumed new generation high-water mark: %v", err)
	}
	if got := product.currentDeckPagePresentation(); got != "tasklists" {
		t.Fatalf("current generation page after reverse-order clear/publish = %q", got)
	}
}

func TestCommandDeckPagePresentationClearAndExpiryRenderNeutralPageBindings(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	product := a.commandProduct.Load()
	settings := deckConditionCandidate("settings-page", "navigation.settings.open", commandbindings.Facts{commandbindings.AppPage: "settings"})
	profiles := deckConditionCandidate("profiles-page", "navigation.profiles.open", commandbindings.Facts{commandbindings.AppPage: "profiles"})
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{settings, profiles})
	if err != nil {
		t.Fatal(err)
	}
	configuration = configuration.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
		settings.ID: {TitleByLocale: map[string]string{"en": "Settings page"}, Icon: "settings-icon"},
		profiles.ID: {TitleByLocale: map[string]string{"en": "Profiles page"}, Icon: "profiles-icon"},
	}))
	if err := product.host.RebuildUserConfiguration(context.Background(),
		func(context.Context) (auth.LocalSessionPrincipal, error) { return product.principal, nil },
		func(_ context.Context, principal auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
			if principal != product.principal {
				return nil, nil, commandexecution.ErrDenied
			}
			return configuration, nil, nil
		}); err != nil {
		t.Fatalf("install in-memory test configuration: %v", err)
	}
	keyboardMap, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	model := commanddeck.Model{ID: "test", Name: "Test", Rows: 1, Columns: 4, KeyImageW: 72, KeyImageH: 72}
	renderCurrentPage := func() commanddeck.KeyView {
		t.Helper()
		bindings, _, err := product.deckMap(context.Background())
		if err != nil {
			t.Fatalf("deckMap: %v", err)
		}
		binding, ok := bindings["test-deck"][0]
		if !ok {
			t.Fatal("page-dependent key disappeared from deckMap")
		}
		return commandDeckKeyView(binding, "en", model)
	}
	if got := renderCurrentPage().Title; got != "" {
		t.Fatalf("missing page snapshot rendered contextual title %q", got)
	}
	if err := a.PublishCommandDeckPagePresentation("settings", keyboardMap.Generation, 1); err != nil {
		t.Fatal(err)
	}
	if got := renderCurrentPage().Title; got != "Settings page" {
		t.Fatalf("published page title = %q", got)
	}
	if err := a.ClearCommandDeckPagePresentation(keyboardMap.Generation, 2); err != nil {
		t.Fatal(err)
	}
	if got := renderCurrentPage().Title; got != "" {
		t.Fatalf("clear retained contextual title %q", got)
	}
	if err := a.PublishCommandDeckPagePresentation("profiles", keyboardMap.Generation, 3); err != nil {
		t.Fatal(err)
	}
	product.deckPagePresentationMu.Lock()
	product.deckPagePresentation.expiresAt = time.Now().Add(-time.Millisecond)
	product.deckPagePresentationMu.Unlock()
	if got := renderCurrentPage().Title; got != "" {
		t.Fatalf("expired snapshot retained contextual title %q", got)
	}
}
