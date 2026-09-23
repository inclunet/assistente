package commandadapter

import (
	"context"
	"testing"
	"time"

	"assistente/internal/commandbridge"
	"assistente/internal/commandinput"
)

func TestControllerOwnsSourceEventIdentity(t *testing.T) {
	c, b := newFixture(t)
	c.resolve = func(Event) (commandbridge.Invocation, bool, error) {
		v := invocation(1)
		v.SourceEventID = "01900000-0000-7000-8000-000000000001"
		return v, true, nil
	}
	for _, event := range []Event{
		{SourceInstance: "keyboard", Key: "N", Kind: commandinput.KeyDown},
		{SourceInstance: "keyboard", Key: "N", Kind: commandinput.KeyDown, Repeat: true},
		{SourceInstance: "keyboard", Key: "N", Kind: commandinput.KeyUp},
		{SourceInstance: "keyboard", Key: "N", Kind: commandinput.KeyDown},
	} {
		if _, err := c.Input(context.Background(), event); err != nil {
			t.Fatal(err)
		}
	}
	first, last := b.inputs[0].Invocation.SourceEventID, b.inputs[3].Invocation.SourceEventID
	if !uuidV7(first) || !uuidV7(last) || first == last || first == "01900000-0000-7000-8000-000000000001" {
		t.Fatalf("identidades não pertencem ao adapter: %q %q", first, last)
	}
	if b.inputs[1].Invocation.SourceEventID != "" || b.inputs[2].Invocation.SourceEventID != "" {
		t.Fatal("repeat/release conservaram identidade injetada pelo resolvedor")
	}
}

func TestControllerSequenceReleaseUsesDispatchKey(t *testing.T) {
	c, b := newFixture(t)
	c.sequences = map[string]Sequence{"Ctrl+K": {PrefixKey: "Ctrl+K", Keys: []string{"C"}, Timeout: time.Second}}
	for round := 0; round < 2; round++ {
		for _, event := range []Event{
			{SourceInstance: "keyboard", Key: "Ctrl+K", Kind: commandinput.KeyDown},
			{SourceInstance: "keyboard", Key: "C", Kind: commandinput.KeyDown},
			{SourceInstance: "keyboard", Key: "C", Kind: commandinput.KeyDown, Repeat: true},
			{SourceInstance: "other", Key: "C", Kind: commandinput.KeyUp},
			{SourceInstance: "keyboard", Key: "C", Kind: commandinput.KeyUp},
		} {
			if _, err := c.Input(context.Background(), event); err != nil {
				t.Fatal(err)
			}
		}
	}
	for i, want := range []string{"Ctrl+K C", "Ctrl+K C", "C", "Ctrl+K C", "Ctrl+K C", "Ctrl+K C", "C", "Ctrl+K C"} {
		if b.inputs[i].Key != want {
			t.Fatalf("input %d: key=%q want=%q", i, b.inputs[i].Key, want)
		}
	}
	if len(c.releases) != 0 {
		t.Fatal("release não removeu a correlação de sequência")
	}
}

type sequenceDispatchPort struct{ calls []commandbridge.Invocation }

func (p *sequenceDispatchPort) Dispatch(_ context.Context, v commandbridge.Invocation) (commandbridge.InvocationAck, error) {
	p.calls = append(p.calls, v)
	return commandbridge.InvocationAck{InvocationID: v.InvocationID, Accepted: true}, nil
}
func (*sequenceDispatchPort) Cancel(context.Context, commandbridge.CancelRequest) error { return nil }

func TestControllerSequenceCanDispatchAgainThroughRealBridge(t *testing.T) {
	port := &sequenceDispatchPort{}
	b, err := commandbridge.New(commandbridge.Config{Port: port, Capabilities: []commandbridge.Capability{{ID: "cap-a", CommandID: "command.a", Generation: 1, Source: commandbridge.SourceKeyboardLocal, Owner: owner()}}})
	if err != nil {
		t.Fatal(err)
	}
	if err := b.OpenSession(commandbridge.Session{ID: owner().SessionID, Generation: 1, Owner: owner()}); err != nil {
		t.Fatal(err)
	}
	number := 0
	c, err := New(Config{Bridge: b, SessionID: owner().SessionID, Owner: owner(), Generation: 1,
		Sequences: []Sequence{{PrefixKey: "Ctrl+K", Keys: []string{"C"}, Timeout: time.Second}},
		Resolve: func(e Event) (commandbridge.Invocation, bool, error) {
			number++
			return invocation(number), e.Key == "Ctrl+K C", nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 2; round++ {
		if _, err := c.Input(context.Background(), Event{SourceInstance: "keyboard", Key: "Ctrl+K", Kind: commandinput.KeyDown}); err != nil {
			t.Fatal(err)
		}
		ack, err := c.Input(context.Background(), Event{SourceInstance: "keyboard", Key: "C", Kind: commandinput.KeyDown})
		if err != nil || !ack.Accepted {
			t.Fatalf("round %d: ack=%+v err=%v", round, ack, err)
		}
		v := port.calls[len(port.calls)-1]
		if _, err := b.AcceptResult(commandbridge.Result{SessionID: v.SessionID, InvocationID: v.InvocationID, CommandID: v.CommandID, Generation: v.Generation, CapabilityID: v.CapabilityID, Ownership: v.Ownership, OccurrenceID: v.OccurrenceID, SourceEventID: v.SourceEventID, Owner: owner(), Status: commandbridge.ResultSucceeded}); err != nil {
			t.Fatal(err)
		}
		if _, err := c.Input(context.Background(), Event{SourceInstance: "keyboard", Key: "C", Kind: commandinput.KeyUp}); err != nil {
			t.Fatal(err)
		}
	}
	if len(port.calls) != 2 {
		t.Fatalf("dispatches=%d", len(port.calls))
	}
}
