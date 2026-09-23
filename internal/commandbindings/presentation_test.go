package commandbindings

import (
	"reflect"
	"testing"
)

func TestPresentationSnapshotIsolationAndResolution(t *testing.T) {
	d, delta := defaultFixture()
	base, err := NewConfiguration([]Default{d}, []Delta{delta}, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := map[string]BindingPresentation{
		d.Candidate.ID: {TitleByLocale: map[string]string{"en": "Base"}},
		delta.ID:       {TitleByLocale: map[string]string{"en": "Custom"}},
	}
	snapshot := NewPresentationSnapshot(entries)
	projected := base.WithPresentation(snapshot)
	entries[delta.ID].TitleByLocale["en"] = "input mutation"
	snapshot.byBindingID[delta.ID].TitleByLocale["en"] = "snapshot mutation"
	output := projected.Presentation()
	output.byBindingID[delta.ID].TitleByLocale["en"] = "output mutation"
	value, _ := projected.Presentation().Binding(delta.ID)
	value.TitleByLocale["en"] = "binding mutation"
	before, err := base.Resolve(delta.Trigger, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	after, err := projected.Resolve(delta.Trigger, nil, nil)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("execution changed: %+v %+v %v", before, after, err)
	}
	if title, ok := projected.TitleForBindings(after.BindingIDs, "en"); !ok || title != "Custom" {
		t.Fatalf("aliased title: %q %v", title, ok)
	}
	if _, ok := base.TitleForBindings(after.BindingIDs, "en"); ok {
		t.Fatal("original changed")
	}
	restored, err := projected.WithoutDeltas([]string{delta.ID})
	if err != nil {
		t.Fatal(err)
	}
	result, err := restored.Resolve(delta.Trigger, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if title, ok := restored.TitleForBindings(result.BindingIDs, "en"); !ok || title != "Base" {
		t.Fatalf("restore title: %q %v", title, ok)
	}
	if _, ok := restored.Presentation().Binding(delta.ID); ok {
		t.Fatal("removed delta presentation retained")
	}
	if title, _ := projected.TitleForBindings(after.BindingIDs, "en"); title != "Custom" {
		t.Fatal("restore mutated old snapshot")
	}
}

func TestPresentationExactLocaleAndEquivalentBindings(t *testing.T) {
	p := NewPresentationSnapshot(map[string]BindingPresentation{
		"a": {TitleByLocale: map[string]string{"pt-BR": "Meu título", "en": "Same"}},
		"b": {TitleByLocale: map[string]string{"en": "Same"}},
		"c": {TitleByLocale: map[string]string{"en": "Different"}},
	})
	for _, tc := range []struct {
		ids          []string
		locale, want string
	}{
		{[]string{"a"}, "pt-BR", "Meu título"},
		{[]string{"a"}, "pt", ""},
		{[]string{"a"}, "es", ""},
		{[]string{"a", "b"}, "en", "Same"},
		{[]string{"b", "a"}, "en", "Same"},
		{[]string{"a", "b"}, "pt-BR", ""},
		{[]string{"a", "c"}, "en", ""},
		{[]string{"a", "unknown"}, "en", ""},
		{nil, "en", ""},
	} {
		title, ok := p.TitleForBindings(tc.ids, tc.locale)
		if title != tc.want || ok != (tc.want != "") {
			t.Fatalf("%v/%s: %q %v", tc.ids, tc.locale, title, ok)
		}
	}
}
