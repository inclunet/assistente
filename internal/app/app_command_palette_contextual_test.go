package app

import (
	"testing"

	"assistente/internal/commandledger"
	"assistente/internal/database"
)

func contextualPaletteFixture(t *testing.T) (*App, LocalCommandKeyboardMap, LocalCommandKeyboardContext) {
	t.Helper()
	a, decisions := contextualKeyboardChatFixture(t)
	if err := a.workspaceMgr.SetProfile("dev"); err != nil {
		t.Fatal(err)
	}
	observed := contextualKeyboardContext(t, a)
	observed.Profile = "dev"
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Paleta de trabalho", Enabled: true}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &CommandSettingsBindingInput{
		LayerID: layer.ID, CommandID: commandWorkspaceTabChatCreateID, TriggerType: "palette", TriggerSpec: `{"version":1,"selection":"workspace.tab.chat.create"}`, Effect: "execute", Enabled: true,
		Condition: &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{
			{Field: "app.focused", Value: true}, {Field: "surface.type", Value: "chat"}, {Field: "surface.id", Value: observed.SurfaceID}, {Field: "profile", Value: "dev"},
		}},
	}})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "rule_create", Rule: &CommandSettingsRuleInput{LayerID: layer.ID, Mode: "always", Lifecycle: "persistent", Enabled: true}})
	view, err := a.GetLocalCommandKeyboardMap()
	if err != nil {
		t.Fatal(err)
	}
	return a, view, observed
}

func TestContextualPaletteWorkspaceCommitsOnceAndKeepsLedger(t *testing.T) {
	a, view, observed := contextualPaletteFixture(t)
	before, err := a.workspaceMgr.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	reservation, err := a.BeginContextualPaletteUICommand(view.Generation, commandWorkspaceTabChatCreateID, observed)
	if err != nil {
		t.Fatal(err)
	}
	handoff := takeUICommandFor(t, a, reservation.Ticket, commandWorkspaceTabChatCreateID)
	prepared, err := a.workspaceMgr.CommandSnapshot()
	if err != nil || prepared != before {
		t.Fatalf("Begin/Take mutated workspace: %+v %v", prepared, err)
	}
	if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID); err != nil {
		t.Fatal(err)
	}
	result := getUIResultEventually(t, a, reservation.Ticket)
	if result.Status != string(commandledger.Succeeded) {
		t.Fatalf("commit did not succeed: %+v", result)
	}
	after, err := a.workspaceMgr.CommandSnapshot()
	if err != nil || after.ActiveTabID == before.ActiveTabID {
		t.Fatalf("chat was not created: %+v %v", after, err)
	}
	_ = a.CommitWorkspaceTabCommand(reservation.Ticket, handoff.HandoffID)
	again, err := a.workspaceMgr.CommandSnapshot()
	if err != nil || again != after {
		t.Fatalf("duplicate commit mutated workspace: %+v %v", again, err)
	}
	var count int64
	if err := database.DB().Table("command_invocations").Where("invocation_id = ? AND source_type = ? AND status = ?", reservation.InvocationID, "palette", "succeeded").Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("missing unique successful palette ledger: %d %v", count, err)
	}
}

func TestContextualPaletteWorkspaceRejectsForgedContextAndWrongRoute(t *testing.T) {
	a, view, observed := contextualPaletteFixture(t)
	for _, invalid := range []LocalCommandKeyboardContext{
		{SurfaceID: "foreign-tab", SurfaceType: "chat", Profile: "dev"},
		{SurfaceID: observed.SurfaceID, SurfaceType: "editor", Profile: "dev"},
		{SurfaceID: observed.SurfaceID, SurfaceType: "chat", Profile: "foreign"},
		{SurfaceID: observed.SurfaceID, SurfaceType: "chat"},
	} {
		if reservation, err := a.BeginContextualPaletteUICommand(view.Generation, commandWorkspaceTabChatCreateID, invalid); err == nil || reservation.Ticket != "" {
			t.Fatalf("forged context accepted: %+v %v", reservation, err)
		}
	}
	if _, err := a.BeginContextualPaletteUICommand("old-generation", commandWorkspaceTabChatCreateID, observed); err == nil {
		t.Fatal("old map accepted")
	}
	for _, command := range []string{commandProductWorkspaceListID, commandConversationClearID, "navigation.settings.open", "layer.activate"} {
		if _, err := a.BeginContextualPaletteUICommand(view.Generation, command, observed); err == nil {
			t.Fatalf("unsupported route accepted %s", command)
		}
	}
	// The ordinary ingress cannot acquire the host proof used by the new path.
	reservation, err := a.BeginUICommand(commandWorkspaceTabChatCreateID)
	if err != nil {
		t.Fatal(err)
	}
	if result := getUIResultEventually(t, a, reservation.Ticket); result.Status == string(commandledger.Succeeded) {
		t.Fatalf("ordinary Begin bypassed contextual conditions: %+v", result)
	}
	if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
		t.Fatal("ordinary Begin handed off a contextual command")
	}
}

func TestContextualPaletteWorkspaceRejectsProfileABAAtTakeAndCommit(t *testing.T) {
	for _, phase := range []string{"take", "commit"} {
		t.Run(phase, func(t *testing.T) {
			a, view, observed := contextualPaletteFixture(t)
			reservation, err := a.BeginContextualPaletteUICommand(view.Generation, commandWorkspaceTabChatCreateID, observed)
			if err != nil {
				t.Fatal(err)
			}
			handoffID := ""
			if phase == "commit" {
				handoffID = takeUICommandFor(t, a, reservation.Ticket, commandWorkspaceTabChatCreateID).HandoffID
			}
			if err := a.workspaceMgr.SetProfile("other"); err != nil {
				t.Fatal(err)
			}
			if err := a.workspaceMgr.SetProfile("dev"); err != nil {
				t.Fatal(err)
			}
			if phase == "take" {
				if _, err := a.TakeUICommand(reservation.Ticket); err == nil {
					t.Fatal("profile A-B-A accepted at Take")
				}
			} else if err := a.CommitWorkspaceTabCommand(reservation.Ticket, handoffID); err == nil {
				t.Fatal("profile A-B-A accepted at Commit")
			}
			after, err := a.workspaceMgr.CommandSnapshot()
			if err != nil || after.ActiveTabID != observed.SurfaceID {
				t.Fatalf("stale command created a tab: %+v %v", after, err)
			}
		})
	}
}
