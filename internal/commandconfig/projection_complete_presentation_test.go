package commandconfig

import (
	"context"
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
	row.Presentation = `{"version":1,"title_by_locale":{"pt-BR":"Título materializado","en":"Override"}}`
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
	if _, ok := config.TitleForBindings([]string{"builtin.tab.new"}, "en"); ok {
		t.Fatal("delta title leaked to default")
	}
	snapshot.Bindings[0].Presentation = `{"version":1}`
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
	if title, _ := config.TitleForBindings(result.BindingIDs, "en"); title != "Override" {
		t.Fatal("old snapshot changed")
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
}
