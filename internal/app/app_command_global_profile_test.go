package app

import (
	"context"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
)

func globalProfileFixture(t *testing.T) (*App, commandGlobalBinding, *commandbindings.Configuration) {
	t.Helper()
	a, binding := globalVoiceExecutionFixture(t)
	p := a.commandProduct.Load()
	if err := a.workspaceMgr.SetProfile("profile-a"); err != nil {
		t.Fatal(err)
	}
	configuration, err := commandbindings.NewConfiguration([]commandbindings.Default{{Version: "1", Fingerprint: binding.Fingerprint, Candidate: commandbindings.Candidate{ID: binding.ID, Trigger: binding.Identity, CommandID: binding.CommandID, ArgumentsKey: string(binding.Arguments), ExecutionScopeKey: "global", Scope: commandbindings.Application, Enabled: true, LayerActive: true, Condition: commandbindings.Facts{commandbindings.Profile: "profile-a"}}}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.host.RebuildUserConfiguration(context.Background(), func(context.Context) (auth.LocalSessionPrincipal, error) { return p.principal, nil }, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return configuration, nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	return a, binding, configuration
}

func TestCommandGlobalProfileMismatchDoesNotRebuildConfiguration(t *testing.T) {
	a, binding, configuration := globalProfileFixture(t)
	p := a.commandProduct.Load()
	if err := a.workspaceMgr.SetProfile("profile-b"); err != nil {
		t.Fatal(err)
	}
	if occurrence, err := p.prepareGlobalOccurrence(context.Background(), binding, func() bool { return true }); err == nil || occurrence != nil {
		t.Fatalf("wrong profile admitted: %+v %v", occurrence, err)
	}
	current, _, _, err := p.host.ResolutionSnapshot(context.Background(), p.principal)
	if err != nil || current != configuration {
		t.Fatalf("false condition rebuilt configuration: %v", err)
	}
	if err := a.workspaceMgr.SetProfile("profile-a"); err != nil {
		t.Fatal(err)
	}
	if occurrence, err := p.prepareGlobalOccurrence(context.Background(), binding, func() bool { return true }); err != nil || occurrence == nil || occurrence.originVersion == "" {
		t.Fatalf("matching profile not admitted: %+v %v", occurrence, err)
	}
}

func TestCommandGlobalProfileABAReturnDoesNotRevivePreparedOccurrence(t *testing.T) {
	a, binding, _ := globalProfileFixture(t)
	reservation := reserveGlobalVoiceForTest(t, a, binding, func() bool { return true })
	if err := a.workspaceMgr.SetProfile("profile-b"); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetProfile("profile-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.TakeGlobalVoiceCommand(reservation.Ticket); err == nil {
		t.Fatal("A-B-A revived the physical occurrence")
	}
}
