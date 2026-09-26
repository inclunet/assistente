package app

import (
	"context"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"assistente/internal/commandbindings"
	"assistente/internal/commandcatalog"
	"assistente/internal/commanddeck"
)

func TestCommandDeckPresentedBindingPrefersFeedbackAndPreservesPNG(t *testing.T) {
	baseImage := strings.Repeat("a", 64)
	feedbackImage := strings.Repeat("b", 64)
	imageBytes := []byte{1, 2, 3, 4}
	binding := commandDeckBinding{
		title: "Base", icon: "base-icon", imageRef: baseImage, imagePNG: imageBytes,
		feedbackState: "succeeded", persistentState: "on",
		variants: map[string]commandDeckVisual{
			"on":        {title: "Ligado", icon: "on-icon", imageRef: baseImage},
			"succeeded": {title: "Concluído", icon: "success-icon", imageRef: feedbackImage},
		},
	}

	got := commandDeckPresentedBinding(binding)
	if got.title != "Concluído" || got.icon != "success-icon" || got.imageRef != feedbackImage || got.imagePNG != nil {
		t.Fatalf("feedback não prevaleceu ou PNG obsoleto foi preservado: %+v", got)
	}
	if binding.title != "Base" || binding.imageRef != baseImage || !reflect.DeepEqual(binding.imagePNG, imageBytes) {
		t.Fatal("aplicar variante mutou o binding de entrada")
	}

	binding.feedbackState = ""
	got = commandDeckPresentedBinding(binding)
	if got.title != "Ligado" || got.icon != "on-icon" || got.imageRef != baseImage || !reflect.DeepEqual(got.imagePNG, imageBytes) {
		t.Fatalf("variante persistente não preservou PNG de mesma referência: %+v", got)
	}

	binding.persistentState = "off"
	got = commandDeckPresentedBinding(binding)
	if got.title != "Base" || got.icon != "base-icon" || got.imageRef != baseImage || !reflect.DeepEqual(got.imagePNG, imageBytes) {
		t.Fatalf("estado sem variante alterou apresentação base: %+v", got)
	}
}

func TestCommandDeckStateVisualsProjectsVariantsAndBaseInheritance(t *testing.T) {
	candidate := deckConditionCandidate("visual", "navigation.settings.open", nil)
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{candidate})
	if err != nil {
		t.Fatal(err)
	}
	baseImage := strings.Repeat("a", 64)
	variantImage := strings.Repeat("b", 64)
	configuration = configuration.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
		candidate.ID: {
			TitleByLocale: map[string]string{"en": "Configured"}, Icon: "base-icon", ImageRef: baseImage,
			States: map[string]commandbindings.BindingPresentation{
				"running":   {TitleByLocale: map[string]string{"en": "Running"}, Icon: "running-icon", ImageRef: variantImage},
				"failed":    {Icon: "failed-icon"},
				"succeeded": {TitleByLocale: map[string]string{"es": "Terminado"}},
			},
		},
	}))
	definition := commandcatalog.Definition{
		ID: candidate.CommandID,
		Presentation: &commandcatalog.Presentation{Locales: map[string]commandcatalog.LocalizedMetadata{
			"en": {Name: "Localized fallback"},
		}},
	}
	visuals := commandDeckStateVisuals(configuration, []string{candidate.ID}, definition, "en")
	if len(visuals) != 10 {
		t.Fatalf("states materializados=%d, want 10", len(visuals))
	}
	if got := visuals["running"]; got != (commandDeckVisual{title: "Running", icon: "running-icon", imageRef: variantImage}) {
		t.Fatalf("variante running: %+v", got)
	}
	if got := visuals["failed"]; got != (commandDeckVisual{title: "Configured", icon: "failed-icon", imageRef: baseImage}) {
		t.Fatalf("falha não herdou título/imagem base: %+v", got)
	}
	if got := visuals["succeeded"]; got != (commandDeckVisual{title: "Configured", icon: "base-icon", imageRef: baseImage}) {
		t.Fatalf("locale ausente não herdou base exata: %+v", got)
	}
	if got := visuals["off"]; got != (commandDeckVisual{title: "Configured", icon: "base-icon", imageRef: baseImage}) {
		t.Fatalf("estado sem variante não herdou base: %+v", got)
	}
}

func TestLocalDeckPresentationsUsesResolvedBindingConsensusAndInheritance(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	first := deckConditionCandidate("settings-chat", "navigation.settings.open", commandbindings.Facts{commandbindings.SurfaceType: "chat"})
	second := deckConditionCandidate("settings-editor", "navigation.settings.open", commandbindings.Facts{commandbindings.SurfaceType: "editor"})
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{first, second})
	if err != nil {
		t.Fatal(err)
	}
	baseImage := strings.Repeat("a", 64)
	configuration = configuration.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
		first.ID: {
			TitleByLocale: map[string]string{"en": "Settings"}, Icon: "base-icon", ImageRef: baseImage,
			States: map[string]commandbindings.BindingPresentation{
				"running": {TitleByLocale: map[string]string{"en": "Settings running"}, Icon: "running-icon"},
			},
		},
		second.ID: {
			TitleByLocale: map[string]string{"en": "Settings"}, Icon: "base-icon", ImageRef: baseImage,
			States: map[string]commandbindings.BindingPresentation{
				"running": {TitleByLocale: map[string]string{"en": "Settings running"}, Icon: "running-icon", ImageRef: baseImage},
			},
		},
	}))
	conditions := contextualDeckUIConditions(configuration, registry, deckConditionTestTrigger)
	if len(conditions) != 1 || conditions[0].CommandID != first.CommandID {
		t.Fatalf("resoluções contextuais reais: %+v", conditions)
	}
	base, variants := localDeckPresentations(configuration, registry, deckConditionTestTrigger, conditions, "en")
	if base.title != "Settings" || base.icon != "base-icon" || base.imageRef != baseImage {
		t.Fatalf("consenso base entre bindings resolvidos: %+v", base)
	}
	if got := variants["running"]; got != (commandDeckVisual{title: "Settings running", icon: "running-icon", imageRef: baseImage}) {
		t.Fatalf("variante unânime + imagem herdada: %+v", got)
	}
	if got := variants["failed"]; got != (commandDeckVisual{title: "Settings", icon: "base-icon", imageRef: baseImage}) {
		t.Fatalf("estado sem variante não herdou a apresentação efetiva: %+v", got)
	}
}

func TestLocalDeckPresentationsSelectsLivePageWithoutMutatingExecutionConditions(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
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
	conditions := contextualDeckUIConditions(configuration, registry, deckConditionTestTrigger)
	if len(conditions) != 2 {
		t.Fatalf("condition projection changed: %+v", conditions)
	}
	before := cloneLocalCommandPaletteConditions(conditions)

	settingsVisual, _ := localDeckPresentationsForPage(configuration, registry, deckConditionTestTrigger, conditions, "en", "settings")
	profilesVisual, _ := localDeckPresentationsForPage(configuration, registry, deckConditionTestTrigger, conditions, "en", "profiles")
	emptyVisual, _ := localDeckPresentationsForPage(configuration, registry, deckConditionTestTrigger, conditions, "en", "jobs")
	noSnapshotVisual, _ := localDeckPresentationsForPage(configuration, registry, deckConditionTestTrigger, conditions, "en", "")
	if settingsVisual.title != "Settings page" || settingsVisual.icon != "settings-icon" {
		t.Fatalf("settings page visual = %+v", settingsVisual)
	}
	if profilesVisual.title != "Profiles page" || profilesVisual.icon != "profiles-icon" {
		t.Fatalf("profiles page visual = %+v", profilesVisual)
	}
	if emptyVisual.title != "" || emptyVisual.icon != "" || emptyVisual.imageRef != "" {
		t.Fatalf("unmatched page retained another page's visual: %+v", emptyVisual)
	}
	if noSnapshotVisual.title != "" || noSnapshotVisual.icon != "" || noSnapshotVisual.imageRef != "" {
		t.Fatalf("missing page snapshot exposed the union of page visuals: %+v", noSnapshotVisual)
	}
	if !reflect.DeepEqual(conditions, before) {
		t.Fatalf("render-only page selection mutated execution conditions: before=%+v after=%+v", before, conditions)
	}
}

func TestLocalDeckPresentationsWithoutPageKeepsPageIndependentBinding(t *testing.T) {
	registry := paletteConditionTestRegistry(t)
	common := deckConditionCandidate("common", "navigation.settings.open", nil)
	pageSpecific := deckConditionCandidate("settings-page", "navigation.settings.open", commandbindings.Facts{commandbindings.AppPage: "settings"})
	configuration, err := commandbindings.NewConfiguration(nil, nil, []commandbindings.Candidate{common, pageSpecific})
	if err != nil {
		t.Fatal(err)
	}
	configuration = configuration.WithPresentation(commandbindings.NewPresentationSnapshot(map[string]commandbindings.BindingPresentation{
		common.ID:       {TitleByLocale: map[string]string{"en": "Common settings"}, Icon: "common-icon"},
		pageSpecific.ID: {TitleByLocale: map[string]string{"en": "Settings page"}, Icon: "page-icon"},
	}))
	conditions := contextualDeckUIConditions(configuration, registry, deckConditionTestTrigger)
	visual, _ := localDeckPresentationsForPage(configuration, registry, deckConditionTestTrigger, conditions, "en", "")
	if visual.title != "Common settings" || visual.icon != "common-icon" {
		t.Fatalf("page-independent fallback missing or page union leaked: %+v", visual)
	}
}

func TestDeckPersistentStateChangedBaselineDedupLimitAndClosed(t *testing.T) {
	p := &commandProductRuntime{}
	if p.deckPersistentStateChanged("key-1", "on") {
		t.Fatal("baseline inicial deve ser silencioso")
	}
	if p.deckPersistentStateChanged("key-1", "on") {
		t.Fatal("estado repetido não deve ser anunciado")
	}
	if !p.deckPersistentStateChanged("key-1", "off") {
		t.Fatal("mudança persistente não foi detectada")
	}
	if p.deckPersistentStateChanged("key-1", "off") || p.deckPersistentStateChanged("", "on") {
		t.Fatal("estado repetido ou identidade vazia foi tratado como mudança")
	}

	p.deckPresentedStates = make(map[string]string, 64)
	for i := 0; i < 64; i++ {
		p.deckPresentedStates["existing-"+strconv.Itoa(i)] = "on"
	}
	if p.deckPersistentStateChanged("new-key", "running") {
		t.Fatal("primeira observação após limite deve estabelecer baseline")
	}
	if len(p.deckPresentedStates) != 1 || p.deckPresentedStates["new-key"] != "running" {
		t.Fatalf("limite 64 não limpou o baseline antigo: len=%d map=%v", len(p.deckPresentedStates), p.deckPresentedStates)
	}

	p.closed = true
	if p.deckPersistentStateChanged("new-key", "failed") || p.deckPresentedStates["new-key"] != "running" {
		t.Fatal("runtime fechado alterou ou anunciou estado")
	}
}

func TestCommandDeckProductMapProjectsPersistentTargetStateVariants(t *testing.T) {
	a, decisions := settingsSecurityFixture(t)
	controlLayerID, _ := settingsActivationSecurityLayerAndRule(t, a, decisions)
	targetLayer := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "layer_create",
		Layer: &CommandSettingsLayerInput{Name: "Deck target state", Enabled: true},
	})
	targetRule := settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "rule_create",
		Rule: &CommandSettingsRuleInput{LayerID: targetLayer.ID, Mode: "manual", Lifecycle: "persistent", Enabled: true},
	})
	settingsContractApply(t, a, decisions, CommandSettingsMutationRequest{
		Scope: CommandSettingsScopeGlobal, Operation: "binding_create",
		Binding: &CommandSettingsBindingInput{
			LayerID: controlLayerID, CommandID: commandLayerToggleID, TriggerType: "streamdeck.key",
			TriggerSpec: `{"version":1,"device":"STATEDECK01","key":0}`,
			Arguments:   map[string]any{"scope": "global", "rule_id": targetRule.ID, "duration_seconds": 0},
			Effect:      "execute", Enabled: true,
			Presentation: map[string]any{
				"version": 1,
				"states": map[string]any{
					"on":  map[string]any{"title_by_locale": map[string]any{"en": "Target is on"}, "icon": "deck-on"},
					"off": map[string]any{"title_by_locale": map[string]any{"en": "Target is off"}, "icon": "deck-off"},
				},
			},
		},
	})
	if _, err := a.SetCommandLayerActive(controlLayerID, true); err != nil {
		t.Fatal(err)
	}
	p := a.commandProduct.Load()
	p.setDeckLocale("en")
	model := commanddeck.Model{ID: "test", Name: "Test", Rows: 1, Columns: 1, KeyImageW: 72, KeyImageH: 72}
	readDeckBinding := func(wantState, wantTitle string) {
		t.Helper()
		bindings, _, err := p.deckMap(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		binding, ok := bindings["STATEDECK01"][0]
		if !ok || binding.commandID != commandLayerToggleID || binding.persistentState != wantState {
			t.Fatalf("deckMap binding: %+v, present=%v, want state=%q", binding, ok, wantState)
		}
		view := commandDeckKeyView(binding, "en", model)
		if view.State != wantState || view.Title != wantTitle {
			t.Fatalf("deck view: state=%q title=%q, want %q/%q", view.State, view.Title, wantState, wantTitle)
		}
	}
	readDeckBinding("off", "Target is off")
	if _, err := a.ApplyCommandLayerAction("global", targetRule.ID, "pin", 0); err != nil {
		t.Fatal(err)
	}
	readDeckBinding("on", "Target is on")
}
