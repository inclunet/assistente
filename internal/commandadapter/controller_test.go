package commandadapter

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandbridge"
	"assistente/internal/commandinput"

	"github.com/google/uuid"
)

type bridgeSpy struct {
	mu         sync.Mutex
	inputs     []commandbridge.Input
	lifecycles []commandbridge.LifecycleEvent
	inputErr   error
	lifeErr    error
	releases   map[uint64]chan struct{}
}

func (b *bridgeSpy) Input(_ context.Context, input commandbridge.Input) (commandbridge.InvocationAck, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.inputs = append(b.inputs, input)
	if b.inputErr != nil {
		return commandbridge.InvocationAck{}, b.inputErr
	}
	return commandbridge.InvocationAck{InvocationID: input.Invocation.InvocationID, Accepted: true}, nil
}

func (b *bridgeSpy) Lifecycle(ctx context.Context, event commandbridge.LifecycleEvent) error {
	b.mu.Lock()
	b.lifecycles = append(b.lifecycles, event)
	release := b.releases[event.Generation]
	err := b.lifeErr
	b.mu.Unlock()
	if release != nil {
		select {
		case <-release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return err
}

func owner() commandbridge.Owner {
	return commandbridge.Owner{UserID: "user-a", SessionID: "session-a", WorkspaceID: "workspace-a"}
}

func invocation(number int) commandbridge.Invocation {
	return commandbridge.Invocation{
		InvocationID: fmt.Sprintf("01900000-0000-7000-8000-%012d", number),
		CommandID:    "command.a",
		CapabilityID: "cap-a",
		Ownership:    commandbridge.OwnershipLocal,
		Source:       commandbridge.SourceKeyboardLocal,
	}
}

func uuidV7(value string) bool {
	id, err := uuid.Parse(value)
	return err == nil && id.Version() == 7 && id.Variant() == uuid.RFC4122 && id.String() == value
}

func newFixture(t *testing.T) (*Controller, *bridgeSpy) {
	t.Helper()
	bridge := &bridgeSpy{releases: make(map[uint64]chan struct{})}
	controller, err := New(Config{
		Bridge:     bridge,
		SessionID:  owner().SessionID,
		Owner:      owner(),
		Generation: 1,
		Resolve: func(Event) (commandbridge.Invocation, bool, error) {
			return invocation(1), true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return controller, bridge
}

func TestControllerInputDelegatesOnlyToBridgeWithGenerationAndOwner(t *testing.T) {
	controller, bridge := newFixture(t)
	ack, err := controller.Input(context.Background(), Event{
		SourceInstance: "keyboard-local",
		Key:            "Ctrl+N",
		Kind:           commandinput.KeyDown,
	})
	if err != nil || !ack.Accepted {
		t.Fatalf("ack=%+v err=%v", ack, err)
	}
	if len(bridge.inputs) != 1 {
		t.Fatalf("inputs=%d", len(bridge.inputs))
	}
	got := bridge.inputs[0]
	if got.SessionID != owner().SessionID || got.Generation != 1 || got.Owner != owner() {
		t.Fatalf("input sem identidade/generation do controller: %+v", got)
	}
	if got.Invocation.SessionID != owner().SessionID || got.Invocation.Generation != 1 ||
		got.Invocation.OccurrenceID != "" || got.Invocation.SourceEventID == "" {
		t.Fatalf("invocation não normalizada pela ponte: %+v", got.Invocation)
	}
	if !uuidV7(got.Invocation.SourceEventID) {
		t.Fatalf("source event id não é UUIDv7: %q", got.Invocation.SourceEventID)
	}
}

func TestControllerNoBindingDoesNotCallBridgeInput(t *testing.T) {
	bridge := &bridgeSpy{releases: make(map[uint64]chan struct{})}
	controller, err := New(Config{
		Bridge:     bridge,
		SessionID:  owner().SessionID,
		Owner:      owner(),
		Generation: 1,
		Resolve: func(Event) (commandbridge.Invocation, bool, error) {
			return commandbridge.Invocation{}, false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	ack, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "Ctrl+Tab", Kind: commandinput.KeyDown})
	if err != nil || ack.Accepted || ack.Reason != "no-binding" {
		t.Fatalf("no binding ack=%+v err=%v", ack, err)
	}
	if len(bridge.inputs) != 0 {
		t.Fatalf("input sem binding chegou à ponte: %d", len(bridge.inputs))
	}
}

func TestControllerDoesNotMintSourceEventForReleaseOrUnboundInput(t *testing.T) {
	bridge := &bridgeSpy{releases: make(map[uint64]chan struct{})}
	controller, err := New(Config{
		Bridge:     bridge,
		SessionID:  owner().SessionID,
		Owner:      owner(),
		Generation: 1,
		Resolve: func(event Event) (commandbridge.Invocation, bool, error) {
			if event.Kind == commandinput.KeyUp {
				return invocation(7), true, nil
			}
			return commandbridge.Invocation{}, false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if ack, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "Ctrl+N", Kind: commandinput.KeyDown}); err != nil || ack.Accepted {
		t.Fatalf("down sem binding ack=%+v err=%v", ack, err)
	}
	if _, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "Ctrl+N", Kind: commandinput.KeyUp}); err != nil {
		t.Fatal(err)
	}
	if len(bridge.inputs) != 1 {
		t.Fatalf("inputs=%d", len(bridge.inputs))
	}
	if bridge.inputs[0].Invocation.SourceEventID != "" {
		t.Fatalf("release ganhou source event id: %+v", bridge.inputs[0].Invocation)
	}
}

func TestControllerSequencePrefixBranchesAndTimeout(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	bridge := &bridgeSpy{releases: make(map[uint64]chan struct{})}
	controller, err := New(Config{
		Bridge:     bridge,
		SessionID:  owner().SessionID,
		Owner:      owner(),
		Generation: 1,
		Sequences:  []Sequence{{PrefixKey: "Ctrl+N", Keys: []string{"C", "E"}, Timeout: 1500 * time.Millisecond}},
		Now:        func() time.Time { return now },
		Resolve: func(event Event) (commandbridge.Invocation, bool, error) {
			if event.Key == "Ctrl+N C" {
				return invocation(8), true, nil
			}
			return commandbridge.Invocation{}, false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "Ctrl+N", Kind: commandinput.KeyDown})
	if err != nil || pending.Accepted || pending.Reason != "sequence-pending" {
		t.Fatalf("prefix ack=%+v err=%v", pending, err)
	}
	if len(bridge.inputs) != 0 {
		t.Fatalf("prefix disparou input: %d", len(bridge.inputs))
	}
	ack, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "C", Kind: commandinput.KeyDown})
	if err != nil || !ack.Accepted {
		t.Fatalf("branch ack=%+v err=%v", ack, err)
	}
	if len(bridge.inputs) != 1 || bridge.inputs[0].Key != "Ctrl+N C" {
		t.Fatalf("branch key=%+v inputs=%d", bridge.inputs, len(bridge.inputs))
	}

	now = now.Add(2 * time.Second)
	pending, err = controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "Ctrl+N", Kind: commandinput.KeyDown})
	if err != nil || pending.Accepted || pending.Reason != "sequence-pending" {
		t.Fatalf("prefix2 ack=%+v err=%v", pending, err)
	}
	now = now.Add(2 * time.Second)
	ack, err = controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "C", Kind: commandinput.KeyDown})
	if err != nil || ack.Accepted || ack.Reason != "no-binding" {
		t.Fatalf("branch após timeout ack=%+v err=%v", ack, err)
	}
	if len(bridge.inputs) != 1 {
		t.Fatalf("timeout disparou input extra: %d", len(bridge.inputs))
	}
}

func TestControllerSequenceClearsOnBlurAndGeneration(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	bridge := &bridgeSpy{releases: make(map[uint64]chan struct{})}
	controller, err := New(Config{
		Bridge:     bridge,
		SessionID:  owner().SessionID,
		Owner:      owner(),
		Generation: 1,
		Sequences:  []Sequence{{PrefixKey: "Ctrl+N", Keys: []string{"C"}, Timeout: time.Second}},
		Now:        func() time.Time { return now },
		Resolve: func(event Event) (commandbridge.Invocation, bool, error) {
			if event.Key == "Ctrl+N C" {
				return invocation(9), true, nil
			}
			return commandbridge.Invocation{}, false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "Ctrl+N", Kind: commandinput.KeyDown}); err != nil {
		t.Fatal(err)
	}
	if err := controller.Blur(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ack, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "C", Kind: commandinput.KeyDown}); err != nil || ack.Accepted {
		t.Fatalf("branch após blur ack=%+v err=%v", ack, err)
	}
	if _, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "Ctrl+N", Kind: commandinput.KeyDown}); err != nil {
		t.Fatal(err)
	}
	if err := controller.AdvanceGeneration(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if ack, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "C", Kind: commandinput.KeyDown}); err != nil || ack.Accepted {
		t.Fatalf("branch após geração ack=%+v err=%v", ack, err)
	}
	if len(bridge.inputs) != 0 {
		t.Fatalf("sequência limpa disparou input: %d", len(bridge.inputs))
	}
}

func TestControllerLockLogoutAndShutdownSuspendCallbacks(t *testing.T) {
	controller, bridge := newFixture(t)
	if err := controller.Lock(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "Ctrl+N", Kind: commandinput.KeyDown}); !errors.Is(err, ErrAdapterSuspended) {
		t.Fatalf("input após lock=%v", err)
	}
	if err := controller.AdvanceGeneration(context.Background(), 2); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "Ctrl+N", Kind: commandinput.KeyDown}); err != nil {
		t.Fatalf("input após nova geração=%v", err)
	}
	if err := controller.Logout(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "Ctrl+N", Kind: commandinput.KeyDown}); !errors.Is(err, ErrAdapterSuspended) {
		t.Fatalf("input após logout=%v", err)
	}
	if err := controller.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "Ctrl+N", Kind: commandinput.KeyDown}); !errors.Is(err, ErrAdapterClosed) {
		t.Fatalf("input após shutdown=%v", err)
	}
	if err := controller.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown idempotente=%v", err)
	}
	kinds := make([]commandbridge.LifecycleKind, 0, len(bridge.lifecycles))
	for _, event := range bridge.lifecycles {
		kinds = append(kinds, event.Kind)
	}
	want := []commandbridge.LifecycleKind{commandbridge.LifecycleLock, commandbridge.LifecycleGeneration, commandbridge.LifecycleLogout, commandbridge.LifecycleLock}
	if !slices.Equal(kinds, want) {
		t.Fatalf("lifecycles=%v want=%v", kinds, want)
	}
}

func TestControllerGenerationDoesNotRegressWhenLifecycleCompletesOutOfOrder(t *testing.T) {
	bridge := &bridgeSpy{releases: map[uint64]chan struct{}{
		2: make(chan struct{}),
		3: make(chan struct{}),
	}}
	controller, err := New(Config{
		Bridge:     bridge,
		SessionID:  owner().SessionID,
		Owner:      owner(),
		Generation: 1,
		Resolve: func(Event) (commandbridge.Invocation, bool, error) {
			return invocation(2), true, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	done2 := make(chan error, 1)
	done3 := make(chan error, 1)
	go func() { done2 <- controller.AdvanceGeneration(context.Background(), 2) }()
	go func() { done3 <- controller.AdvanceGeneration(context.Background(), 3) }()
	close(bridge.releases[3])
	if err := <-done3; err != nil {
		t.Fatal(err)
	}
	close(bridge.releases[2])
	if err := <-done2; err != nil && !errors.Is(err, ErrStaleGeneration) {
		t.Fatal(err)
	}
	if _, err := controller.Input(context.Background(), Event{SourceInstance: "keyboard-local", Key: "Ctrl+N", Kind: commandinput.KeyDown}); err != nil {
		t.Fatal(err)
	}
	if got := bridge.inputs[len(bridge.inputs)-1].Generation; got != 3 {
		t.Fatalf("geração regrediu para %d", got)
	}
}

func TestControllerValidatesConfigurationAndEventsFailClosed(t *testing.T) {
	if _, err := New(Config{}); !errors.Is(err, ErrInvalidConfiguration) {
		t.Fatalf("config vazia=%v", err)
	}
	controller, _ := newFixture(t)
	for _, event := range []Event{
		{Key: "Ctrl+N", Kind: commandinput.KeyDown},
		{SourceInstance: "keyboard", Kind: commandinput.KeyDown},
		{SourceInstance: "keyboard", Key: "Ctrl+N"},
		{SourceInstance: "keyboard", Key: "Ctrl+N", Kind: commandinput.KeyUp, Repeat: true},
	} {
		if _, err := controller.Input(context.Background(), event); !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("evento inválido %+v err=%v", event, err)
		}
	}
	if err := controller.AdvanceGeneration(context.Background(), 1); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("geração antiga=%v", err)
	}
}
