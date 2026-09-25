package app

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"assistente/internal/commandconfig"
	"assistente/internal/database"
)

func TestCommandSettingsPresentationConfirmedRoundTrip(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Títulos do dispositivo", Enabled: true}})
	input := CommandSettingsBindingInput{
		LayerID: layer.ID, CommandID: "navigation.settings.open", TriggerType: "streamdeck.key",
		TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Effect: "execute", Enabled: true,
		Presentation: map[string]any{"version": 1, "icon": "settings", "title_by_locale": map[string]any{"pt-BR": "Minhas configurações", "en": "My settings", "es": "Mis ajustes"}},
	}
	created := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_create", Binding: &input})
	input.ID = created.ID
	assertPresentation := func() {
		t.Helper()
		if err := a.rebuildCommandLifecyclePersistedConfiguration(a.ctx); err != nil {
			t.Fatal(err)
		}
		var stored commandconfig.Binding
		if err := database.DB().First(&stored, "id = ?", input.ID).Error; err != nil {
			t.Fatal(err)
		}
		var want, persisted map[string]any
		raw, err := json.Marshal(input.Presentation)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &want); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(stored.Presentation), &persisted); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(want, persisted) {
			t.Fatalf("persistência: want=%v got=%v", want, persisted)
		}
		for _, locale := range []string{"pt-BR", "en", "es"} {
			snapshot, err := a.GetCommandSettingsForScope(locale, "global")
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, binding := range snapshot.Bindings {
				if binding.ID == input.ID {
					found = true
					if !reflect.DeepEqual(want, binding.Presentation) {
						t.Fatalf("snapshot %s: %v", locale, binding.Presentation)
					}
				}
			}
			if !found {
				t.Fatal("binding ausente após reconstrução")
			}
		}
	}
	assertPresentation()
	input.Presentation["title_by_locale"] = map[string]any{"en": "Preferences"}
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_update", ID: input.ID, Binding: &input})
	assertPresentation()
	delete(input.Presentation, "title_by_locale")
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "binding_update", ID: input.ID, Binding: &input})
	assertPresentation()
}

func TestCommandSettingsPresentationRejectsInvalidTitlesBeforeConfirmation(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	layer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{Scope: CommandSettingsScopeGlobal, Operation: "layer_create", Layer: &CommandSettingsLayerInput{Name: "Validação de títulos", Enabled: true}})
	for _, titles := range []map[string]any{
		{"pt-BR": ""}, {"pt-BR": " espaço "}, {"en": "nul\x00title"}, {"es": strings.Repeat("á", 257)}, {"fr": "Titre"}, {"en": 123},
	} {
		snapshot, err := a.GetCommandSettingsForScope("pt-BR", "global")
		if err != nil {
			t.Fatal(err)
		}
		result, err := a.MutateCommandSettings(CommandSettingsMutationRequest{
			Scope: CommandSettingsScopeGlobal, Locale: "pt-BR", ExpectedRevision: snapshot.Revision, ExpectedFingerprint: snapshot.Fingerprint, Operation: "binding_create",
			Binding: &CommandSettingsBindingInput{LayerID: layer.ID, CommandID: "navigation.settings.open", TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"test-deck","key":0}`, Effect: "execute", Enabled: true, Presentation: map[string]any{"version": 1, "title_by_locale": titles}},
		})
		if err == nil || result.Committed || result.Published {
			t.Fatalf("título inválido aceito: %v result=%+v err=%v", titles, result, err)
		}
		select {
		case <-decisions:
			t.Fatal("título inválido abriu confirmação")
		default:
		}
	}
	var count int64
	if err := database.DB().Model(&commandconfig.Binding{}).Where("layer_ref = ?", layer.ID).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("binding inválido persistido: count=%d err=%v", count, err)
	}
}

func TestCommandSettingsPresentationOnlyDiffIsVisible(t *testing.T) {
	before := commandconfig.Binding{ID: "binding", LayerRef: "layer", TriggerType: "streamdeck.key", TriggerSpec: `{"version":1,"device":"private-device-id","key":0}`, Presentation: `{"version":1,"title_by_locale":{"en":"Previous title"}}`}
	after := before
	after.Presentation = `{"version":1,"title_by_locale":{"en":"New title"}}`
	for locale, label := range map[string]string{"pt-BR": "Apresentação", "en": "Presentation", "es": "Presentación"} {
		text, err := renderCommandSettingsDiff(locale, commandconfig.MutationDiff{
			Scope:          commandconfig.Scope{UserID: "owner"},
			BeforeBindings: []commandconfig.Binding{before}, AfterBindings: []commandconfig.Binding{after},
		})
		if err != nil || !strings.Contains(text, label) || !strings.Contains(text, "Previous title") || !strings.Contains(text, "New title") {
			t.Fatalf("diff %s não mostra alteração de título: %q err=%v", locale, text, err)
		}
		if strings.Contains(text, "private-device-id") {
			t.Fatal("diff revelou identidade do dispositivo")
		}
	}
}
