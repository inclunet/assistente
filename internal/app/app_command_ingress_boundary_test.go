package app

import (
	"testing"

	"assistente/internal/commandbridge"
	"assistente/internal/commandinput"
)

// Exercita a fachada pública com a montagem de produto, não uma porta que
// confirma qualquer pedido. Uma recusa não pode consumir a próxima borda válida.
func TestCommandIngressInvalidInputDoesNotPoisonRealProduct(t *testing.T) {
	for _, scenario := range []struct {
		name   string
		mutate func(*commandbridge.Input)
	}{
		{"workspace", func(i *commandbridge.Input) { i.Owner.WorkspaceID = "another-workspace" }},
		{"capability", func(i *commandbridge.Input) { i.Invocation.CapabilityID = "unknown" }},
		{"command", func(i *commandbridge.Input) { i.Invocation.CommandID = "unknown.command" }},
		{"nested-session", func(i *commandbridge.Input) { i.Invocation.SessionID = "another-session" }},
		{"nested-generation", func(i *commandbridge.Input) { i.Invocation.Generation++ }},
		{"occurrence", func(i *commandbridge.Input) { i.Invocation.OccurrenceID = "forged-occurrence" }},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			a := readyCommandProduct(t)
			p := a.commandProduct.Load()
			valid := commandbridge.Input{
				SessionID: p.principal.SessionID, Source: "palette", Key: "workspace.list",
				Generation: p.capability.Generation, Kind: commandinput.KeyDown,
				Invocation: commandProductBridgeInvocation(p), Owner: p.owner(),
			}
			invalid := valid
			scenario.mutate(&invalid)
			if ack, err := a.CommandBridgeInput(invalid); err == nil || ack.Accepted {
				t.Fatalf("entrada inválida aceita: ack=%+v err=%v", ack, err)
			}
			if countPaletteRows(t, "command_invocations", valid.Invocation.InvocationID) != 0 ||
				countPaletteRows(t, "command_idempotency_keys", valid.Invocation.InvocationID) != 0 {
				t.Fatal("entrada recusada na ponte alcançou o executor")
			}
			if ack, err := a.CommandBridgeInput(valid); err != nil || !ack.Accepted {
				t.Fatalf("entrada inválida consumiu a borda válida: ack=%+v err=%v", ack, err)
			}
			waitCommandProductWorker(t, p)
			result, err := a.GetPaletteInvocation(valid.Invocation.InvocationID)
			if err != nil || result.Status != "succeeded" {
				t.Fatalf("operação real não concluiu: result=%+v err=%v", result, err)
			}
			if countPaletteRows(t, "command_invocations", valid.Invocation.InvocationID) != 1 {
				t.Fatal("operação real não foi persistida exatamente uma vez")
			}
		})
	}
}

func TestCommandIngressForgedLifecycleReleaseCannotCreateSecondEdge(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	input := commandbridge.Input{
		SessionID: p.principal.SessionID, Source: "palette", Key: "workspace.list",
		Generation: p.capability.Generation, Kind: commandinput.KeyDown,
		Invocation: commandProductBridgeInvocation(p), Owner: p.owner(),
	}
	if ack, err := a.CommandBridgeInput(input); err != nil || !ack.Accepted {
		t.Fatalf("primeira borda: ack=%+v err=%v", ack, err)
	}
	waitCommandProductWorker(t, p)
	forged := input
	forged.Owner.WorkspaceID = "another-workspace"
	event := commandbridge.LifecycleEvent{
		Kind: commandbridge.LifecycleRelease, SessionID: input.SessionID,
		Generation: input.Generation, Input: &forged,
	}
	if err := a.CommandBridgeLifecycle(event); err == nil {
		t.Fatal("release com owner divergente foi aceito")
	}
	input.Invocation = commandProductBridgeInvocation(p)
	if ack, err := a.CommandBridgeInput(input); err != nil || ack.Accepted || ack.Reason != "not-a-dispatch-edge" {
		t.Fatalf("release forjado criou nova borda: ack=%+v err=%v", ack, err)
	}
	if countPaletteRows(t, "command_idempotency_keys", input.Invocation.InvocationID) != 0 {
		t.Fatal("repetição alcançou o ledger")
	}
	event.Input = &input
	if err := a.CommandBridgeLifecycle(event); err != nil {
		t.Fatalf("release legítimo: %v", err)
	}
	if ack, err := a.CommandBridgeInput(input); err != nil || !ack.Accepted {
		t.Fatalf("borda após release legítimo: ack=%+v err=%v", ack, err)
	}
	waitCommandProductWorker(t, p)
	result, err := a.GetPaletteInvocation(input.Invocation.InvocationID)
	if err != nil || result.Status != "succeeded" {
		t.Fatalf("nova operação: result=%+v err=%v", result, err)
	}
}
