package commandadapter

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"assistente/internal/commandbridge"
	"assistente/internal/commandinput"
)

func newSequenceFixture(t *testing.T, now *time.Time, resolve InvocationFactory) (*Controller, *bridgeSpy) {
	t.Helper()
	bridge := &bridgeSpy{releases: make(map[uint64]chan struct{})}
	controller, err := New(Config{
		Bridge:     bridge,
		SessionID:  owner().SessionID,
		Owner:      owner(),
		Generation: 1,
		Resolve:    resolve,
		Sequences: []Sequence{{
			PrefixKey: "Ctrl+N",
			Keys:      []string{"C"},
			Timeout:   time.Second,
		}},
		Now: func() time.Time { return *now },
	})
	if err != nil {
		t.Fatal(err)
	}
	return controller, bridge
}

func sequenceEvent(source, key string) Event {
	return Event{SourceInstance: source, Key: key, Kind: commandinput.KeyDown}
}

func bridgeInputCount(bridge *bridgeSpy) int {
	bridge.mu.Lock()
	defer bridge.mu.Unlock()
	return len(bridge.inputs)
}

func TestControllerSequencePrefixIsolatedBySource(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	controller, bridge := newSequenceFixture(t, &now, func(event Event) (commandbridge.Invocation, bool, error) {
		if event.Key == "Ctrl+N C" {
			return invocation(101), true, nil
		}
		return commandbridge.Invocation{}, false, nil
	})

	if ack, err := controller.Input(context.Background(), sequenceEvent("source-a", "Ctrl+N")); err != nil || ack.Reason != "sequence-pending" {
		t.Fatalf("prefixo de A: ack=%+v err=%v", ack, err)
	}
	if ack, err := controller.Input(context.Background(), sequenceEvent("source-b", "C")); err != nil || ack.Accepted || ack.Reason != "no-binding" {
		t.Fatalf("ramo de B consumiu prefixo de A: ack=%+v err=%v", ack, err)
	}
	if ack, err := controller.Input(context.Background(), sequenceEvent("source-a", "C")); err != nil || !ack.Accepted {
		t.Fatalf("ramo de A não completou sua própria sequência: ack=%+v err=%v", ack, err)
	}
	if got := bridgeInputCount(bridge); got != 1 {
		t.Fatalf("inputs na ponte=%d, esperado 1", got)
	}
}

func TestControllerSequenceExpiresAtExactDeadline(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	controller, bridge := newSequenceFixture(t, &now, func(event Event) (commandbridge.Invocation, bool, error) {
		if event.Key == "Ctrl+N C" {
			return invocation(102), true, nil
		}
		return commandbridge.Invocation{}, false, nil
	})

	if _, err := controller.Input(context.Background(), sequenceEvent("source-a", "Ctrl+N")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	ack, err := controller.Input(context.Background(), sequenceEvent("source-a", "C"))
	if err != nil || ack.Accepted || ack.Reason != "no-binding" {
		t.Fatalf("ramo no deadline exato completou: ack=%+v err=%v", ack, err)
	}
	if got := bridgeInputCount(bridge); got != 0 {
		t.Fatalf("sequência expirada chegou à ponte: %d", got)
	}
}

func TestControllerCancelledInputDoesNotResolveBridgeOrMutatePending(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	var mu sync.Mutex
	resolveCalls := 0
	controller, bridge := newSequenceFixture(t, &now, func(event Event) (commandbridge.Invocation, bool, error) {
		mu.Lock()
		resolveCalls++
		mu.Unlock()
		if event.Key == "Ctrl+N C" {
			return invocation(103), true, nil
		}
		return commandbridge.Invocation{}, false, nil
	})

	if _, err := controller.Input(context.Background(), sequenceEvent("source-a", "Ctrl+N")); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := controller.Input(ctx, sequenceEvent("source-a", "C")); !errors.Is(err, context.Canceled) {
		t.Fatalf("input cancelado=%v, esperado context.Canceled", err)
	}
	mu.Lock()
	gotResolveCalls := resolveCalls
	mu.Unlock()
	if gotResolveCalls != 0 || bridgeInputCount(bridge) != 0 {
		t.Fatalf("input cancelado chamou resolve/bridge: resolve=%d bridge=%d", gotResolveCalls, bridgeInputCount(bridge))
	}
	if ack, err := controller.Input(context.Background(), sequenceEvent("source-a", "C")); err != nil || !ack.Accepted {
		t.Fatalf("pendência foi alterada pelo input cancelado: ack=%+v err=%v", ack, err)
	}
}

func TestControllerResolutionInvalidatedByLifecycle(t *testing.T) {
	lifecycleCases := []struct {
		name   string
		action func(*Controller) error
	}{
		{name: "blur", action: func(controller *Controller) error { return controller.Blur(context.Background()) }},
		{name: "lock", action: func(controller *Controller) error { return controller.Lock(context.Background()) }},
		{name: "shutdown", action: func(controller *Controller) error { return controller.Shutdown(context.Background()) }},
		{name: "advance-generation", action: func(controller *Controller) error {
			return controller.AdvanceGeneration(context.Background(), 2)
		}},
	}

	for _, testCase := range lifecycleCases {
		t.Run(testCase.name, func(t *testing.T) {
			resolved := make(chan struct{})
			release := make(chan struct{})
			bridge := &bridgeSpy{releases: make(map[uint64]chan struct{})}
			controller, err := New(Config{
				Bridge:     bridge,
				SessionID:  owner().SessionID,
				Owner:      owner(),
				Generation: 1,
				Resolve: func(Event) (commandbridge.Invocation, bool, error) {
					close(resolved)
					<-release
					return invocation(104), true, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			done := make(chan error, 1)
			go func() {
				_, inputErr := controller.Input(context.Background(), Event{
					SourceInstance: "source-a",
					Key:            "Ctrl+N",
					Kind:           commandinput.KeyDown,
				})
				done <- inputErr
			}()
			<-resolved
			if err := testCase.action(controller); err != nil {
				t.Fatal(err)
			}
			close(release)
			if err := <-done; err == nil {
				t.Fatal("resolução antiga chegou à bridge sem erro")
			}
			if got := bridgeInputCount(bridge); got != 0 {
				t.Fatalf("resolução antiga chegou à bridge: %d", got)
			}
		})
	}
}
