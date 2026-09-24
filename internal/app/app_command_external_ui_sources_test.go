package app

import (
	"testing"

	"assistente/internal/commandcatalog"
)

// Keep the external authorization contract aligned with Topbar's dispatcher,
// without treating AllowedSources.UI as external-only (it also serves Wails).
func TestExternalUIAuthorizationMatchesTopbarNavigationContract(t *testing.T) {
	a, _ := settingsSecurityFixture(t)
	registry := a.commandProduct.Load().registry
	want := []string{
		"navigation.workspace.open", "navigation.history.open", "navigation.memories.open",
		"navigation.tasklists.open", "navigation.jobs.open", "navigation.profiles.open",
		"navigation.settings.open", "navigation.data.export.open", "navigation.data.import.open",
		"navigation.help.open", "navigation.about.open",
		commandWorkspaceTabNextID, commandWorkspaceTabPreviousID, commandWorkspaceTabFirstID,
		commandWorkspaceTabSecondID, commandWorkspaceTabThirdID, commandWorkspaceTabFourthID,
		commandWorkspaceTabFifthID, commandWorkspaceTabSixthID, commandWorkspaceTabSeventhID,
		commandWorkspaceTabEighthID, commandWorkspaceTabNinthID,
	}
	defs := registry.List()
	rules := externalCommandAuthorizationRules(defs)
	if len(rules) != len(want)+1 {
		t.Fatalf("external rule count=%d, want %d (Topbar route/tab + backend workspace.list)", len(rules), len(want)+1)
	}
	got := make(map[string]bool, len(rules))
	for _, rule := range rules {
		got[rule.CommandID] = true
	}
	for _, id := range want {
		definition, ok := registry.Lookup(id)
		if !ok || !definition.AllowsSource(commandcatalog.UI) || !definition.AllowsSource(commandcatalog.Palette) ||
			!commandExternalUICommandSupported(id) || !got[id] {
			t.Errorf("Topbar-supported route/tab missing from external rules or UI sources: %s", id)
		}
	}
	for _, id := range []string{"chat.focus.input", "chat.message.read.open", "tasklists.create.open", "terminal.focus.input"} {
		definition, ok := registry.Lookup(id)
		if !ok || !definition.AllowsSource(commandcatalog.UI) {
			t.Errorf("local Wails UI source unexpectedly removed: %s", id)
		}
		if got[id] || commandExternalUICommandSupported(id) {
			t.Errorf("unsupported contextual/page command exposed externally: %s", id)
		}
	}
	if definition, ok := registry.Lookup(commandProductWorkspaceListID); !ok || !definition.AllowsSource(commandcatalog.UI) ||
		definition.HandlerClassification != commandcatalog.HandlerBackend || !got[commandProductWorkspaceListID] {
		t.Error("workspace.list backend must retain local UI source and external authorization")
	}
}
