package commandconfig

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"assistente/internal/commandbindings"
)

func TestProjectCompletePresentationUsesMaterializedBindingIDs(t *testing.T) {
	user := completeProjectionUUID(t)
	layer := Layer{ID: completeProjectionUUID(t), UserID: user, Name: "Global", Enabled: true, Source: "user"}
	row := completeProjectionBinding(t, user, nil, layer, completeProjectionUUID(t))
	row.LayerRefKind, row.LayerRef = "builtin", "application.defaults"
	row.ReplacesDefaultID, row.ReplacesDefaultVersion, row.ReplacesDefaultFingerprint = stringPtr("builtin.tab.new"), stringPtr("1"), stringPtr("fp-v1")
	row.Presentation = `{"version":1,"title_by_locale":{"pt-BR":"Título materializado","en":"Override"},"icon":"deck-edit","image_ref":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}`
	options := completeProjectionOptions(completeProjectionRegistry(t))
	snapshot := Snapshot{Scope: Scope{UserID: user}, Bindings: []Binding{row}}
	config, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	result, err := config.Resolve("keyboard.local:KeyA", nil, nil)
	if err != nil || !reflect.DeepEqual(result.BindingIDs, []string{row.ID}) {
		t.Fatalf("delta ID: %+v %v", result, err)
	}
	if title, ok := config.TitleForBindings(result.BindingIDs, "pt-BR"); !ok || title != "Título materializado" {
		t.Fatalf("title: %q %v", title, ok)
	}
	if icon := config.IconForBindings(result.BindingIDs); icon != "deck-edit" {
		t.Fatalf("icon: %q", icon)
	}
	if imageRef := config.ImageForBindings(result.BindingIDs); imageRef != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("image ref: %q", imageRef)
	}
	if _, ok := config.TitleForBindings([]string{"builtin.tab.new"}, "en"); ok {
		t.Fatal("delta title leaked to default")
	}
	if icon := config.IconForBindings([]string{"builtin.tab.new"}); icon != "" {
		t.Fatalf("delta icon leaked to default: %q", icon)
	}
	snapshot.Bindings[0].Presentation = `{"version":1,"icon":"deck-icon-only","image_ref":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`
	withoutTitle, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	other, _ := withoutTitle.Resolve("keyboard.local:KeyA", nil, nil)
	if !reflect.DeepEqual(result, other) {
		t.Fatal("presentation changed execution")
	}
	if config.Equivalent(withoutTitle) {
		t.Fatal("presentation-only update would not publish")
	}
	if _, ok := withoutTitle.TitleForBindings(other.BindingIDs, "en"); ok {
		t.Fatal("removed title survived")
	}
	if icon := withoutTitle.IconForBindings(other.BindingIDs); icon != "deck-icon-only" {
		t.Fatalf("icon-only presentation: %q", icon)
	}
	if imageRef := withoutTitle.ImageForBindings(other.BindingIDs); imageRef != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("icon-only image ref: %q", imageRef)
	}
	if title, _ := config.TitleForBindings(result.BindingIDs, "en"); title != "Override" {
		t.Fatal("old snapshot changed")
	}
	if icon := config.IconForBindings(result.BindingIDs); icon != "deck-edit" {
		t.Fatalf("old icon snapshot changed: %q", icon)
	}
	if imageRef := config.ImageForBindings(result.BindingIDs); imageRef != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("old image snapshot changed: %q", imageRef)
	}

	snapshot.Bindings[0].Presentation = `{"version":1,"image_ref":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"}`
	imageOnly, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	imageOnlyResult, _ := imageOnly.Resolve("keyboard.local:KeyA", nil, nil)
	if got := imageOnly.ImageForBindings(imageOnlyResult.BindingIDs); got != "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc" {
		t.Fatalf("image-only presentation: %q", got)
	}
	if _, ok := imageOnly.TitleForBindings(imageOnlyResult.BindingIDs, "en"); ok {
		t.Fatal("image-only presentation retained title")
	}
	if got := imageOnly.IconForBindings(imageOnlyResult.BindingIDs); got != "" {
		t.Fatalf("image-only presentation retained icon: %q", got)
	}
	if config.Equivalent(imageOnly) {
		t.Fatal("image-only presentation update would not publish")
	}

	row.Enabled = false
	snapshot.Bindings[0] = row
	disabled, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	fallback, _ := disabled.Resolve("keyboard.local:KeyA", nil, nil)
	if fallback.Status != commandbindings.Selected || !reflect.DeepEqual(fallback.BindingIDs, []string{"builtin.tab.new"}) {
		t.Fatalf("fallback: %+v", fallback)
	}
	if _, ok := disabled.TitleForBindings(fallback.BindingIDs, "en"); ok {
		t.Fatal("disabled title leaked")
	}
	if icon := disabled.IconForBindings(fallback.BindingIDs); icon != "" {
		t.Fatal("disabled delta icon leaked")
	}
	if imageRef := disabled.ImageForBindings(fallback.BindingIDs); imageRef != "" {
		t.Fatalf("disabled delta image ref leaked: %q", imageRef)
	}
}

func TestProjectCompletePresentationStatesValidateAndProjectWithoutBase(t *testing.T) {
	user := completeProjectionUUID(t)
	layer := Layer{ID: completeProjectionUUID(t), UserID: user, Name: "Global", Description: "fixture", Enabled: true, Source: "user", ResolutionPriority: 2}
	row := completeProjectionBinding(t, user, nil, layer, completeProjectionUUID(t))
	row.Presentation = `{"version":1,"states":{"running":{"title_by_locale":{"en":"Running"},"icon":"deck-running"}}}`
	snapshot := Snapshot{Scope: Scope{UserID: user}, Layers: []Layer{layer}, Bindings: []Binding{row}}
	options := completeProjectionOptions(completeProjectionRegistry(t))
	options.ActiveUserLayerIDs = []string{layer.ID}
	config, err := ProjectComplete(context.Background(), snapshot, options)
	if err != nil {
		t.Fatal(err)
	}
	got := config.PresentationForState([]string{row.ID}, "running")
	if !reflect.DeepEqual(got.TitleByLocale, map[string]string{"en": "Running"}) || got.Icon != "deck-running" || got.ImageRef != "" {
		t.Fatalf("variante sem apresentação base não projetada: %+v", got)
	}
	if base := config.PresentationForState([]string{row.ID}, "off"); !reflect.DeepEqual(base, commandbindings.BindingPresentation{}) {
		t.Fatalf("estado ausente deveria produzir apresentação vazia: %+v", base)
	}

	invalid := []string{
		`{"version":1,"states":null}`,
		`{"version":1,"states":[]}`,
		`{"version":1,"states":{"unknown":{}}}`,
		`{"version":1,"states":{"running":null}}`,
		`{"version":1,"states":{"running":[]}}`,
		`{"version":1,"states":{"running":{"version":1}}}`,
		`{"version":1,"states":{"running":{"title_key":"command.title"}}}`,
		`{"version":1,"states":{"running":{"status_label_keys":{"running":"label"}}}}`,
		`{"version":1,"states":{"running":{"states":{}}}}`,
		`{"version":1,"states":{"running":{"icon":null}}}`,
		`{"version":1,"states":{"running":{"image_ref":null}}}`,
		`{"version":1,"states":{"running":{"title_by_locale":{"fr":"Titre"}}}}`,
	}
	for _, presentation := range invalid {
		row.Presentation = presentation
		snapshot.Bindings[0] = row
		if config, err := ProjectComplete(context.Background(), snapshot, options); !errors.Is(err, ErrInvalid) || config != nil {
			t.Errorf("schema de variante inválido aceito: %s (config=%v err=%v)", presentation, config, err)
		}
	}
}
