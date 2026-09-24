package commandbindings

import (
	"testing"
	"time"
)

func TestConfigurationDeadlineIsImmutableAndSurvivesCopies(t *testing.T) {
	base, err := NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Minute)
	timed := base.WithValidityDeadline(deadline)
	if !base.ValidUntil().IsZero() || timed.ValidUntil() != deadline || timed.Equivalent(base) {
		t.Fatal("deadline perdeu identidade imutável")
	}
	copy, err := timed.WithLayerProvenance(nil)
	if err != nil || copy.ValidUntil() != deadline {
		t.Fatalf("provenance: %v", err)
	}
	copy, err = timed.WithoutDeltas(nil)
	if err != nil || copy.ValidUntil() != deadline {
		t.Fatalf("restore: %v", err)
	}
}

func TestEquivalentExceptValidityDeadlineIgnoresOnlyDeadline(t *testing.T) {
	base, err := NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	base, err = base.WithPersistedBaseline("base:one")
	if err != nil {
		t.Fatal(err)
	}
	first := base.WithValidityDeadline(time.Now().Add(time.Minute))
	second := base.WithValidityDeadline(time.Now().Add(2 * time.Minute))
	if !first.EquivalentExceptValidityDeadline(second) || first.Equivalent(second) {
		t.Fatal("comparação não isolou somente o deadline renovável")
	}
	changedBase, err := second.WithPersistedBaseline("base:two")
	if err != nil {
		t.Fatal(err)
	}
	if first.EquivalentExceptValidityDeadline(changedBase) {
		t.Fatal("comparação ignorou também a base persistida")
	}
}
