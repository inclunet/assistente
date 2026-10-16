package app

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commanddeck"
)

func TestCommandDeckPresentationContextualBranches(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	first := deckConditionCandidate("chat", "navigation.settings.open", commandbindings.Facts{commandbindings.SurfaceType: "chat"})
	second := deckConditionCandidate("editor", "navigation.menu.open", commandbindings.Facts{commandbindings.SurfaceType: "editor"})
	disabled := deckConditionCandidate("disabled", first.CommandID, first.Condition)
	disabled.Enabled = false
	inactive := deckConditionCandidate("inactive", first.CommandID, first.Condition)
	inactive.LayerActive = false
	unfocused := deckConditionCandidate("unfocused", first.CommandID, commandbindings.Facts{commandbindings.AppFocused: false})
	for _, variant := range []string{"distinct", "same-command", "equivalent-divergent", "equivalent-equal", "missing-locale"} {
		t.Run(variant, func(t *testing.T) {
			rows := []commandbindings.Candidate{first, second, disabled, inactive, unfocused}
			titles := map[string]commandbindings.BindingPresentation{
				first.ID:     {TitleByLocale: map[string]string{"en": "My settings"}},
				second.ID:    {TitleByLocale: map[string]string{"en": "My menu"}},
				disabled.ID:  {TitleByLocale: map[string]string{"en": "SECRET DISABLED"}},
				inactive.ID:  {TitleByLocale: map[string]string{"en": "SECRET INACTIVE"}},
				unfocused.ID: {TitleByLocale: map[string]string{"en": "SECRET UNFOCUSED"}},
			}
			switch variant {
			case "same-command":
				rows[1].CommandID = first.CommandID
			case "equivalent-divergent", "equivalent-equal":
				duplicate := first
				duplicate.ID = "equivalent"
				rows = append(rows, duplicate)
				title := "Different"
				if variant == "equivalent-equal" {
					title = "My settings"
				}
				titles[duplicate.ID] = commandbindings.BindingPresentation{TitleByLocale: map[string]string{"en": title}}
			case "missing-locale":
				titles[first.ID] = commandbindings.BindingPresentation{TitleByLocale: map[string]string{"pt-BR": "Só português"}}
			}
			base, err := commandbindings.NewConfiguration(nil, nil, rows)
			if err != nil {
				t.Fatal(err)
			}
			config := base.WithPresentation(commandbindings.NewPresentationSnapshot(titles))
			conditions := contextualDeckUIConditions(config, registry, deckConditionTestTrigger)
			if !reflect.DeepEqual(conditions, contextualDeckUIConditions(base, registry, deckConditionTestTrigger)) {
				t.Fatal("title changed branches")
			}
			visual, _ := localDeckPresentations(config, registry, deckConditionTestTrigger, conditions, "en")
			got := visual.title
			definition, _ := registry.Lookup(first.CommandID)
			fallback := definition.Presentation.Locales["en"].Name
			want := []string{"My settings", "My menu"}
			switch variant {
			case "same-command":
				want = []string{fallback}
			case "equivalent-divergent", "missing-locale":
				want = []string{fallback, "My menu"}
			}
			slices.Sort(want)
			if got != strings.Join(want, " / ") {
				t.Fatalf("title: got=%q want=%q", got, strings.Join(want, " / "))
			}
		})
	}
}

func TestCommandDeckPresentationSettingsReachContextualMapAndRenderer(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	input := CommandSettingsBindingInput{
		LayerID: layer, CommandID: "navigation.settings.open", TriggerType: "streamdeck.key",
		TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Effect: "execute", Enabled: true,
		Condition:    &CommandSettingsCondition{Version: 1, Clauses: []CommandSettingsConditionClause{{Field: "surface.type", Value: "chat"}}},
		Presentation: map[string]any{"version": 1, "title_by_locale": map[string]any{"en": "My settings"}},
	}
	created := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &input})
	input.ID = created.ID
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	model := commanddeck.Model{ID: "test", Name: "Test", Rows: 1, Columns: 2, KeyImageW: 72, KeyImageH: 72}
	renderer := commanddeck.NewRenderer()
	if err := renderer.OpenDevice("test-deck", model); err != nil {
		t.Fatal(err)
	}
	var previousImageID string
	for index, title := range []string{"My settings", "Updated settings"} {
		if index != 0 {
			input.Presentation["title_by_locale"] = map[string]any{"en": title}
			settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_update", ID: input.ID, Binding: &input})
		}
		// Read the published projection rebuilt from real persisted settings.
		if err := a.rebuildCommandLifecyclePersistedConfiguration(a.ctx); err != nil {
			t.Fatal(err)
		}
		p := a.commandProduct.Load()
		p.setDeckLocale("en")
		bindings, _, err := p.deckMap(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		binding := bindings["test-deck"][0]
		if len(binding.conditions) == 0 || binding.title != title {
			t.Fatalf("contextual map: %+v", binding)
		}
		view := commandDeckKeyView(binding, "en", model)
		if view.Title != title || view.Announce != title || view.State != "conditional" || len(view.ImageRGBA) == 0 {
			t.Fatalf("view: %+v", view)
		}
		if index != 0 && view.ImageID == previousImageID {
			t.Fatal("title absent from image identity")
		}
		previousImageID = view.ImageID
		frame := commanddeck.Frame{Device: "test-deck", Model: model, Keys: map[int]commanddeck.KeyView{0: view}}
		plan, err := renderer.Render(frame)
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 && !plan.FullFrame || index != 0 && (plan.FullFrame || len(plan.Updates) != 1 || plan.Updates[0].Index != 0) {
			t.Fatalf("diff: %+v", plan)
		}
		unchanged, err := renderer.Render(frame)
		if err != nil || len(unchanged.Updates) != 0 {
			t.Fatalf("unchanged resent: %+v %v", unchanged, err)
		}
		p.setDeckLocale("es")
		spanish, _, err := p.deckMap(context.Background())
		definition, _ := p.registry.Lookup(input.CommandID)
		if err != nil || spanish["test-deck"][0].title != definition.Presentation.Locales["es"].Name {
			t.Fatalf("locale fallback: %+v %v", spanish, err)
		}
	}
}

func TestCommandDeckPresentationNativeSelectionExcludesIneligible(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	selected := deckConditionCandidate("selected", commandWorkspaceTabCloseID, nil)
	other := deckConditionCandidate("other", selected.CommandID, commandbindings.Facts{commandbindings.Profile: "private"})
	config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{selected, other})
	if err != nil {
		t.Fatal(err)
	}
	config = config.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
		selected.ID: {TitleByLocale: map[string]string{"en": "Close mine"}},
		other.ID:    {TitleByLocale: map[string]string{"en": "Private"}},
	}))
	resolved, err := config.Resolve(selected.Trigger, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	definition, _ := registry.Lookup(selected.CommandID)
	if title := commandDeckTitle(config, resolved.BindingIDs, definition, "en"); title != "Close mine" {
		t.Fatal(title)
	}
	view := commandDeckKeyView(commandDeckBinding{commandID: selected.CommandID, title: "Close mine"}, "en", commanddeck.Model{KeyImageW: 72, KeyImageH: 72})
	if !strings.Contains(view.ImageID, "Close mine") || view.State != "ready" {
		t.Fatalf("native view: %+v", view)
	}
}
