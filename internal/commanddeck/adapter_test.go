package commanddeck

import (
	"context"
	"errors"
	"testing"

	"assistente/internal/commandadapter"
	"assistente/internal/commandbridge"
	"assistente/internal/commandinput"
)

type adapterControllerSpy struct {
	events []commandadapter.Event
	locks  int
	logout int
}

func (s *adapterControllerSpy) Input(_ context.Context, event commandadapter.Event) (commandbridge.InvocationAck, error) {
	s.events = append(s.events, event)
	return commandbridge.InvocationAck{Accepted: true, InvocationID: "accepted"}, nil
}

func (s *adapterControllerSpy) Lock(context.Context) error {
	s.locks++
	return nil
}

func (s *adapterControllerSpy) Logout(context.Context) error {
	s.logout++
	return nil
}

func TestDeviceAdapterForwardsOnlyActiveDeckKeys(t *testing.T) {
	controller := &adapterControllerSpy{}
	adapter, err := NewDeviceAdapter(NewManager(NewRenderer(), BackoffPolicy{}), controller)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Open("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Key(context.Background(), "deck-a", 1, true, false); !errors.Is(err, ErrDeviceSafe) {
		t.Fatalf("evento antes de ativar deveria ser seguro: %v", err)
	}
	snapshot, _ := adapter.manager.Snapshot("deck-a")
	if err := adapter.Activate("deck-a", snapshot.Generation); err != nil {
		t.Fatal(err)
	}
	ack, err := adapter.Key(context.Background(), "deck-a", 1, true, false)
	if err != nil || !ack.Accepted {
		t.Fatalf("ack=%+v err=%v", ack, err)
	}
	_, _ = adapter.Key(context.Background(), "deck-a", 1, false, false)
	if len(controller.events) != 2 {
		t.Fatalf("eventos=%+v", controller.events)
	}
	if got := controller.events[0]; got.SourceInstance != "streamdeck.key:deck-a" || got.Key != "key:1" || got.Kind != commandinput.KeyDown || got.Repeat {
		t.Fatalf("keydown inválido: %+v", got)
	}
	if got := controller.events[1]; got.Kind != commandinput.KeyUp {
		t.Fatalf("keyup inválido: %+v", got)
	}
}

func TestDeviceAdapterRejectsInvalidKeysAndSafeOnLock(t *testing.T) {
	controller := &adapterControllerSpy{}
	adapter, err := NewDeviceAdapter(NewManager(NewRenderer(), BackoffPolicy{}), controller)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Open("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := adapter.manager.Snapshot("deck-a")
	if err := adapter.Activate("deck-a", snapshot.Generation); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.Key(context.Background(), "deck-a", testModel.KeyCount(), true, false); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("índice inválido err=%v", err)
	}
	if _, err := adapter.Key(context.Background(), "deck-a", 0, false, true); !errors.Is(err, ErrInvalidKey) {
		t.Fatalf("repeat em keyup err=%v", err)
	}
	plans, err := adapter.Lock(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if controller.locks != 1 || len(plans) != 1 || !plans[0].FullFrame {
		t.Fatalf("lock não tornou seguro: locks=%d plans=%+v", controller.locks, plans)
	}
	if _, err := adapter.Key(context.Background(), "deck-a", 0, true, false); !errors.Is(err, ErrDeviceSafe) {
		t.Fatalf("evento após lock deveria ser seguro: %v", err)
	}
}
