package commanddeck

import (
	"errors"
	"testing"
	"time"
)

func TestManagerOpensInSafeStateAndRequiresActivation(t *testing.T) {
	manager := NewManager(NewRenderer(), BackoffPolicy{Initial: time.Second, Max: 4 * time.Second})
	plan, err := manager.Open("deck-a", testModel)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.FullFrame || len(plan.Updates) != testModel.KeyCount() {
		t.Fatalf("abertura deve enviar estado seguro completo: %+v", plan)
	}
	if _, err := manager.Render(Frame{Device: "deck-a", Model: testModel, Keys: map[int]KeyView{0: {Title: "A"}}}); !errors.Is(err, ErrDeviceSafe) {
		t.Fatalf("render ativo antes de ativação err=%v", err)
	}
	snapshot, err := manager.Snapshot("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Status != DeviceSafe || !snapshot.SafeFrameSent || snapshot.Generation != 1 {
		t.Fatalf("snapshot seguro inesperado: %+v", snapshot)
	}
	if err := manager.Activate("deck-a", snapshot.Generation); err != nil {
		t.Fatal(err)
	}
	active, err := manager.Render(Frame{Device: "deck-a", Model: testModel, Keys: map[int]KeyView{0: {Title: "A"}}})
	if err != nil {
		t.Fatal(err)
	}
	if active.FullFrame || len(active.Updates) != 1 || active.Updates[0].Index != 0 {
		t.Fatalf("render ativo deveria ser diff: %+v", active)
	}
}

func TestManagerEnforcesExclusiveOpenAndStaleGeneration(t *testing.T) {
	manager := NewManager(NewRenderer(), BackoffPolicy{})
	if _, err := manager.Open("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Open("deck-a", testModel); !errors.Is(err, ErrDeviceAlreadyOpen) {
		t.Fatalf("segunda abertura deveria falhar: %v", err)
	}
	snapshot, _ := manager.Snapshot("deck-a")
	manager.LockOrLogout()
	if err := manager.Activate("deck-a", snapshot.Generation); !errors.Is(err, ErrInvalidDevice) {
		t.Fatalf("geração obsoleta deveria falhar: %v", err)
	}
	next, _ := manager.Snapshot("deck-a")
	if next.Generation != snapshot.Generation+1 || next.Status != DeviceSafe {
		t.Fatalf("lock/logout não renovou geração segura: %+v", next)
	}
}

func TestManagerDisconnectBackoffAndReconnectFullFrame(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	manager := NewManager(NewRenderer(), BackoffPolicy{Initial: time.Second, Max: 2 * time.Second})
	manager.SetClock(func() time.Time { return now })
	if _, err := manager.Open("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	delay, err := manager.Disconnect("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	if delay != time.Second || manager.CanReconnect("deck-a") {
		t.Fatalf("backoff inicial inesperado delay=%v can=%v", delay, manager.CanReconnect("deck-a"))
	}
	now = now.Add(time.Second)
	if !manager.CanReconnect("deck-a") {
		t.Fatal("deveria permitir reconexão após backoff")
	}
	plan, err := manager.Open("deck-a", testModel)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.FullFrame || len(plan.Updates) != testModel.KeyCount() {
		t.Fatalf("reconexão deve reenviar frame completo seguro: %+v", plan)
	}
	delay, err = manager.Disconnect("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	if delay != 2*time.Second {
		t.Fatalf("backoff deveria crescer até max: %v", delay)
	}
}
