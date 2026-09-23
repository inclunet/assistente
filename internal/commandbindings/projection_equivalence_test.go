package commandbindings

import "testing"

func TestProjectionEquivalenceIncludesExecutableSemantics(t *testing.T) {
	build := func(d Default, x Delta) *Configuration {
		t.Helper()
		c, err := NewConfiguration([]Default{d}, []Delta{x}, nil)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	d, x := defaultFixture()
	baseline := build(d, x)
	if !baseline.Equivalent(build(d, x)) || baseline.Equivalent(nil) {
		t.Fatal("igualdade de snapshots independentes/nil incorreta")
	}
	for _, tc := range []struct {
		name   string
		change func(*Default, *Delta)
	}{
		{"layer", func(_ *Default, x *Delta) { x.LayerActive = false }},
		{"binding", func(_ *Default, x *Delta) { x.Enabled = false }},
		{"target", func(_ *Default, x *Delta) { x.CommandID = "other" }},
		{"arguments", func(_ *Default, x *Delta) { x.ArgumentsKey = "other-args" }},
		{"condition", func(_ *Default, x *Delta) { x.Condition = Facts{Profile: "dev"} }},
		{"priority", func(_ *Default, x *Delta) { x.BindingPriority++ }},
		{"review", func(_ *Default, x *Delta) { x.ReviewStatus = NeedsReview }},
		{"default", func(d *Default, _ *Delta) { d.Fingerprint = "new-semantics" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d, x := defaultFixture()
			tc.change(&d, &x)
			if baseline.Equivalent(build(d, x)) {
				t.Fatal("mudança executável tratada como simples renovação de prova")
			}
		})
	}
}
