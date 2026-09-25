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
		d.Candidate.ID: {TitleByLocale: map[string]string{"en": "Base"}, Icon: "deck-base", ImageRef: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		delta.ID:       {TitleByLocale: map[string]string{"en": "Custom"}, Icon: "deck-custom", ImageRef: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
	}
	snapshot := NewPresentationSnapshot(entries)
	projected := base.WithPresentation(snapshot)
	entries[delta.ID].TitleByLocale["en"] = "input mutation"
	entries[delta.ID] = BindingPresentation{TitleByLocale: map[string]string{"en": "input mutation"}, Icon: "input mutation"}
	snapshot.byBindingID[delta.ID] = BindingPresentation{TitleByLocale: map[string]string{"en": "snapshot mutation"}, Icon: "snapshot mutation"}
	output := projected.Presentation()
	output.byBindingID[delta.ID] = BindingPresentation{TitleByLocale: map[string]string{"en": "output mutation"}, Icon: "output mutation"}
	value, _ := projected.Presentation().Binding(delta.ID)
	if value.ImageRef != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("binding perdeu image ref: %q", value.ImageRef)
	}
	value.TitleByLocale["en"] = "binding mutation"
	value.Icon = "binding mutation"
	value.ImageRef = "cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"
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
	if icon := projected.IconForBindings(after.BindingIDs); icon != "deck-custom" {
		t.Fatalf("aliased icon: %q", icon)
	}
	if imageRef := projected.ImageForBindings(after.BindingIDs); imageRef != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("aliased image ref: %q", imageRef)
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
	if icon := restored.IconForBindings(result.BindingIDs); icon != "deck-base" {
		t.Fatalf("restore icon: %q", icon)
	}
	if imageRef := restored.ImageForBindings(result.BindingIDs); imageRef != "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("restore image ref: %q", imageRef)
	}
	if _, ok := restored.Presentation().Binding(delta.ID); ok {
		t.Fatal("removed delta presentation retained")
	}
	if title, _ := projected.TitleForBindings(after.BindingIDs, "en"); title != "Custom" {
		t.Fatal("restore mutated old snapshot")
	}
	if icon := projected.IconForBindings(after.BindingIDs); icon != "deck-custom" {
		t.Fatalf("restore mutated old icon snapshot: %q", icon)
	}
	if imageRef := projected.ImageForBindings(after.BindingIDs); imageRef != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("restore mutated old image snapshot: %q", imageRef)
	}
}

func TestPresentationExactLocaleAndEquivalentBindings(t *testing.T) {
	p := NewPresentationSnapshot(map[string]BindingPresentation{
		"a":     {TitleByLocale: map[string]string{"pt-BR": "Meu título", "en": "Same"}, Icon: "deck-same"},
		"b":     {TitleByLocale: map[string]string{"en": "Same"}, Icon: "deck-same"},
		"c":     {TitleByLocale: map[string]string{"en": "Different"}, Icon: "deck-different"},
		"empty": {Icon: ""},
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

func TestIconForBindingsRequiresUnanimityAndResolvedIDs(t *testing.T) {
	p := NewPresentationSnapshot(map[string]BindingPresentation{
		"a":     {Icon: "deck-same"},
		"b":     {Icon: "deck-same"},
		"c":     {Icon: "deck-other"},
		"empty": {},
	})
	for _, tc := range []struct {
		ids  []string
		want string
	}{
		{[]string{"a"}, "deck-same"},
		{[]string{"a", "b"}, "deck-same"},
		{[]string{"b", "a"}, "deck-same"},
		{[]string{"a", "c"}, ""},
		{[]string{"a", "empty"}, ""},
		{[]string{"a", "unknown"}, ""},
		{nil, ""},
	} {
		if got := p.IconForBindings(tc.ids); got != tc.want {
			t.Fatalf("%v: icon %q, want %q", tc.ids, got, tc.want)
		}
	}
}

func TestImageForBindingsRequiresUnanimityAndResolvedIDs(t *testing.T) {
	p := NewPresentationSnapshot(map[string]BindingPresentation{
		"a":     {ImageRef: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"b":     {ImageRef: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		"c":     {ImageRef: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		"empty": {},
	})
	for _, tc := range []struct {
		ids  []string
		want string
	}{
		{[]string{"a"}, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{[]string{"a", "b"}, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{[]string{"b", "a"}, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		{[]string{"a", "c"}, ""},
		{[]string{"a", "empty"}, ""},
		{[]string{"a", "unknown"}, ""},
		{nil, ""},
	} {
		if got := p.ImageForBindings(tc.ids); got != tc.want {
			t.Fatalf("%v: image ref %q, want %q", tc.ids, got, tc.want)
		}
	}
}

func TestPresentationForStateUsesPerFieldFallbackAndUnanimity(t *testing.T) {
	p := NewPresentationSnapshot(map[string]BindingPresentation{
		"a": {
			TitleByLocale: map[string]string{"en": "Base", "es": "Base ES"}, Icon: "deck-base", ImageRef: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			States: map[string]BindingPresentation{"running": {TitleByLocale: map[string]string{"en": "Running"}, ImageRef: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
		},
		"b": {
			TitleByLocale: map[string]string{"en": "Base", "es": "Base ES"}, Icon: "deck-base", ImageRef: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			States: map[string]BindingPresentation{"running": {TitleByLocale: map[string]string{"en": "Running"}, ImageRef: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
		},
		"c": {TitleByLocale: map[string]string{"en": "Different"}, Icon: "deck-other"},
	})
	got := p.PresentationForState([]string{"a", "b"}, "running")
	if !reflect.DeepEqual(got.TitleByLocale, map[string]string{"en": "Running", "es": "Base ES"}) || got.Icon != "deck-base" || got.ImageRef != "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("variant + fallback: %+v", got)
	}
	got = p.PresentationForState([]string{"a", "c"}, "running")
	if got.TitleByLocale["en"] != "" || got.Icon != "" || got.ImageRef != "" {
		t.Fatalf("discordância escolheu apresentação: %+v", got)
	}
	if got := p.PresentationForState([]string{"a"}, "not-a-state"); !reflect.DeepEqual(got, BindingPresentation{}) {
		t.Fatalf("estado desconhecido: %+v", got)
	}
}

func TestPresentationStateDeepCopyAndDeltaRestore(t *testing.T) {
	d, delta := defaultFixture()
	base, err := NewConfiguration([]Default{d}, []Delta{delta}, nil)
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]BindingPresentation{"running": {TitleByLocale: map[string]string{"en": "Running"}, Icon: "deck-running"}}
	snapshot := NewPresentationSnapshot(map[string]BindingPresentation{
		d.Candidate.ID: {States: map[string]BindingPresentation{"running": {Icon: "deck-default"}}},
		delta.ID:       {States: states},
	})
	projected := base.WithPresentation(snapshot)
	states["running"] = BindingPresentation{Icon: "caller mutation"}
	read := projected.Presentation()
	entry, _ := read.Binding(delta.ID)
	entry.States["running"].TitleByLocale["en"] = "returned mutation"
	entry.States["running"] = BindingPresentation{Icon: "returned mutation"}
	read.byBindingID[delta.ID].States["running"] = BindingPresentation{Icon: "snapshot mutation"}
	value := projected.PresentationForState([]string{delta.ID}, "running")
	if value.TitleByLocale["en"] != "Running" || value.Icon != "deck-running" {
		t.Fatalf("estado sofreu alias: %+v", value)
	}

	restored, err := projected.WithoutDeltas([]string{delta.ID})
	if err != nil {
		t.Fatal(err)
	}
	value = restored.PresentationForState([]string{d.Candidate.ID}, "running")
	if value.Icon != "deck-default" || value.TitleByLocale != nil {
		t.Fatalf("apresentação do default após restaurar delta: %+v", value)
	}
	if _, ok := restored.Presentation().Binding(delta.ID); ok {
		t.Fatal("variante do delta removido permaneceu")
	}
}
