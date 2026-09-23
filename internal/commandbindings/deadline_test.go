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
