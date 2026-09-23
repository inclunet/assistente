package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"reflect"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commanddeck"
)

func TestCommandDeckIconsDistinctBoundedAndTextPreserved(t *testing.T) {
	for _, size := range []int{16, 40, 72, 96} {
		model := commanddeck.Model{KeyImageW: size, KeyImageH: size}
		base := commandDeckKeyView(commandDeckBinding{commandID: "command", title: "Title"}, "en", model)
		seen := make(map[[32]byte]string)
		for _, icon := range []string{"settings", "chat", "folder", "play", "stop", "back", "star"} {
			view := commandDeckKeyView(commandDeckBinding{commandID: "command", title: "Title", icon: icon}, "en", model)
			if view.Title != base.Title || view.Announce != base.Announce || view.State != base.State || len(view.ImageRGBA) != size*size*4 {
				t.Fatalf("icon %s size %d changed text/state or image geometry", icon, size)
			}
			if size >= 40 {
				if bytes.Equal(view.ImageRGBA, base.ImageRGBA) {
					t.Fatalf("inert icon %s", icon)
				}
				digest := sha256.Sum256(view.ImageRGBA)
				if other, exists := seen[digest]; exists {
					t.Fatalf("icons %s and %s identical", icon, other)
				}
				seen[digest] = icon
			} else if !bytes.Equal(view.ImageRGBA, base.ImageRGBA) {
				t.Fatal("tiny geometry must retain text-only rendering")
			}
		}
		unknown := commandDeckKeyView(commandDeckBinding{commandID: "command", title: "Title", icon: "unknown:token"}, "en", model)
		if !reflect.DeepEqual(base, unknown) {
			t.Fatal("unknown icon did not fall back to text-only view")
		}
	}
}

func TestCommandDeckIconsContextualAgreementUsesOnlyEligibleBindings(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	first := deckConditionCandidate("chat", "navigation.settings.open", commandbindings.Facts{commandbindings.SurfaceType: "chat"})
	second := deckConditionCandidate("editor", "navigation.menu.open", commandbindings.Facts{commandbindings.SurfaceType: "editor"})
	disabled := first
	disabled.ID, disabled.Enabled = "disabled", false
	for _, secondIcon := range []string{"folder", "chat", ""} {
		config, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{first, second, disabled})
		if err != nil {
			t.Fatal(err)
		}
		config = config.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
			first.ID: {Icon: "folder"}, second.ID: {Icon: secondIcon}, disabled.ID: {Icon: "PRIVATE"},
		}))
		conditions := contextualDeckUIConditions(config, registry, deckConditionTestTrigger)
		title, icon := localDeckPresentation(config, registry, deckConditionTestTrigger, conditions, "en")
		want := ""
		if secondIcon == "folder" {
			want = "folder"
		}
		if title == "" || icon != want {
			t.Fatalf("second=%q: title=%q icon=%q want=%q", secondIcon, title, icon, want)
		}
	}
}

func TestCommandDeckIconsConfirmedChangesReachRenderer(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	input := CommandSettingsBindingInput{
		LayerID: layer, CommandID: "navigation.settings.open", TriggerType: "streamdeck.key",
		TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Effect: "execute", Enabled: true,
		Presentation: map[string]any{"version": 1, "title_by_locale": map[string]any{"en": "Preferences"}},
	}
	created := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &input})
	input.ID = created.ID
	if _, err := a.SetCommandLayerActive(layer, true); err != nil {
		t.Fatal(err)
	}
	model := commanddeck.Model{ID: "test", Name: "Test", Rows: 1, Columns: 1, KeyImageW: 72, KeyImageH: 72}
	renderer := commanddeck.NewRenderer()
	if err := renderer.OpenDevice("test-deck", model); err != nil {
		t.Fatal(err)
	}
	var original []byte
	for index, icon := range []string{"", "folder", "chat", ""} {
		if index > 0 {
			if icon == "" {
				delete(input.Presentation, "icon")
			} else {
				input.Presentation["icon"] = icon
			}
			settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_update", ID: input.ID, Binding: &input})
		}
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
		if binding.icon != icon || binding.title != "Preferences" {
			t.Fatalf("map: %+v", binding)
		}
		view := commandDeckKeyView(binding, "en", model)
		if index == 0 {
			original = append([]byte{}, view.ImageRGBA...)
		}
		if (icon == "") != bytes.Equal(original, view.ImageRGBA) {
			t.Fatalf("icon=%q did not change/restore pixels", icon)
		}
		frame := commanddeck.Frame{Device: "test-deck", Model: model, Keys: map[int]commanddeck.KeyView{0: view}}
		plan, err := renderer.Render(frame)
		if err != nil || len(plan.Updates) != 1 {
			t.Fatalf("render update: %+v %v", plan, err)
		}
		plan, err = renderer.Render(frame)
		if err != nil || len(plan.Updates) != 0 {
			t.Fatalf("unchanged frame: %+v %v", plan, err)
		}
		snapshot, err := a.GetCommandSettingsForScope("en", "global")
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range snapshot.Bindings {
			if row.ID != input.ID {
				continue
			}
			found = true
			value, exists := row.Presentation["icon"]
			if icon == "" && exists || icon != "" && value != icon {
				t.Fatalf("reloaded icon: %v", row.Presentation)
			}
		}
		if !found {
			t.Fatal("binding missing on reload")
		}
	}
}
