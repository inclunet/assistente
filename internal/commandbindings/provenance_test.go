package commandbindings

import (
	"reflect"
	"testing"
)

func TestResolveLayerRefsSelecionaConflitoEDeduplica(t *testing.T) {
	a := binding("a", Global, nil)
	b := binding("b", Global, nil)
	a.LayerRef = "user.z"
	b.LayerRef = "user.a"
	r, err := New([]Candidate{a, b})
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Resolve(a.Trigger, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != Conflict || !reflect.DeepEqual(got.LayerRefs, []string{"user.a", "user.z"}) {
		t.Fatalf("proveniência de conflito incorreta: %#v", got)
	}
}

func TestConfigurationLayerRefsOverrideSuppressReviewEDetached(t *testing.T) {
	base := binding("default", Global, nil)
	base.LayerRef = "builtin.application"
	override := Delta{ID: "override", DefaultID: base.ID, DefaultVersion: "1", DefaultFingerprint: "fp", Trigger: base.Trigger, Effect: Execute, CommandID: "custom", ArgumentsKey: "empty-args", Enabled: true, LayerActive: true, ReviewStatus: Active, LayerRef: "user.custom"}
	config, err := NewConfiguration([]Default{{Candidate: base, Version: "1", Fingerprint: "fp"}}, []Delta{override}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err := config.Resolve(base.Trigger, nil, nil)
	if err != nil || !reflect.DeepEqual(got.LayerRefs, []string{"user.custom"}) {
		t.Fatalf("proveniência do override incorreta: %#v, %v", got, err)
	}

	suppressed := override
	suppressed.ID = "suppress"
	suppressed.Effect = Suppress
	suppressed.CommandID = ""
	suppressed.ArgumentsKey = ""
	suppressed.LayerRef = "user.suppress"
	config, err = NewConfiguration([]Default{{Candidate: base, Version: "1", Fingerprint: "fp"}}, []Delta{suppressed}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err = config.Resolve(base.Trigger, nil, nil)
	if err != nil || got.Status != Suppressed || !reflect.DeepEqual(got.LayerRefs, []string{"user.suppress"}) {
		t.Fatalf("proveniência da supressão incorreta: %#v, %v", got, err)
	}

	pending := override
	pending.ID = "review"
	pending.ReviewStatus = NeedsReview
	pending.LayerRef = "user.review"
	config, err = NewConfiguration([]Default{{Candidate: base, Version: "1", Fingerprint: "fp"}}, []Delta{pending}, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err = config.Resolve(base.Trigger, nil, nil)
	if err != nil || got.Status != ReviewRequired || !reflect.DeepEqual(got.LayerRefs, []string{"user.review"}) {
		t.Fatalf("proveniência da revisão incorreta: %#v, %v", got, err)
	}
	got.LayerRefs[0] = "mutated"
	again, _ := config.Resolve(base.Trigger, nil, nil)
	if !reflect.DeepEqual(again.LayerRefs, []string{"user.review"}) {
		t.Fatal("LayerRefs não é detached")
	}
}

func TestConfigurationRequiredFactsOrdenadosDetachedEIncluiPendenciaDoTriggerAntigo(t *testing.T) {
	base := binding("default", Global, Facts{Profile: "dev"})
	delta := Delta{ID: "review", DefaultID: base.ID, DefaultVersion: "1", DefaultFingerprint: "changed-fp", Trigger: "other", Effect: Execute, CommandID: "custom", ArgumentsKey: "empty-args", Condition: Facts{Device: "keyboard"}, Enabled: true, LayerActive: true, ReviewStatus: NeedsReview}
	custom := binding("custom", Global, Facts{Process: "editor"})
	config, err := NewConfiguration([]Default{{Candidate: base, Version: "1", Fingerprint: "fp"}}, []Delta{delta}, []Candidate{custom})
	if err != nil {
		t.Fatal(err)
	}
	got := config.RequiredFacts(base.Trigger)
	want := []Field{Device, Process, Profile}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("facts requeridos incorretos: got=%v want=%v", got, want)
	}
	got[0] = AppFocused
	if reflect.DeepEqual(config.RequiredFacts(base.Trigger), got) {
		t.Fatal("RequiredFacts não é detached")
	}
}

func TestConfigurationRequiredFactsIgnoraBindingInativoOuDesabilitado(t *testing.T) {
	active := binding("active", Global, Facts{Profile: "active"})
	disabled := binding("disabled", Global, Facts{Device: "disabled"})
	disabled.Enabled = false
	inactive := binding("inactive", Global, Facts{Process: "inactive"})
	inactive.LayerActive = false
	config, err := NewConfiguration(nil, nil, []Candidate{active, disabled, inactive})
	if err != nil {
		t.Fatal(err)
	}
	if got := config.RequiredFacts(active.Trigger); !reflect.DeepEqual(got, []Field{Profile}) {
		t.Fatalf("bindings inativos/desabilitados exigiram fatos: %v", got)
	}

	base := binding("default", Global, nil)
	base.LayerRef = "builtin.application"
	delta := Delta{ID: "active-override", DefaultID: base.ID, DefaultVersion: "1", DefaultFingerprint: "fp", Trigger: base.Trigger, Effect: Execute, CommandID: "custom", ArgumentsKey: "empty-args", Condition: Facts{Device: "active"}, Enabled: true, LayerActive: true, ReviewStatus: Active}
	config, err = NewConfiguration([]Default{{Candidate: base, Version: "1", Fingerprint: "fp"}}, []Delta{delta}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := config.RequiredFacts(base.Trigger); !reflect.DeepEqual(got, []Field{Device}) {
		t.Fatalf("binding ativo não exigiu fato: %v", got)
	}
}
