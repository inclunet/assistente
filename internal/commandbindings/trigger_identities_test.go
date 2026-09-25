package commandbindings

import (
	"slices"
	"testing"
)

func TestConfigurationTriggerIdentitiesIsSortedUniqueAndDetached(t *testing.T) {
	candidate := func(id, trigger string) Candidate {
		return Candidate{ID: id, Trigger: trigger, CommandID: "workspace.list", ArgumentsKey: "{}", ExecutionScopeKey: "global", Scope: Application, Enabled: true, LayerActive: true}
	}
	c, err := NewConfiguration([]Default{{Candidate: candidate("default", "palette:workspace.list"), Version: "1", Fingerprint: "fp"}}, nil, []Candidate{candidate("b", "keyboard.local:Control+KeyK"), candidate("a", "keyboard.local:Control+KeyK")})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"keyboard.local:Control+KeyK", "palette:workspace.list"}
	got := c.TriggerIdentities()
	if !slices.Equal(got, want) {
		t.Fatalf("índice incorreto: %v", got)
	}
	got[0] = "alterado"
	if !slices.Equal(c.TriggerIdentities(), want) {
		t.Fatal("retorno alterou índice privado")
	}
}
