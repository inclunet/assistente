package commanddeck

import (
	"errors"
	"sync"
	"testing"
	"time"
)

func TestManagerDisconnectedIsObservableAndFailClosed(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	manager := NewManager(NewRenderer(), BackoffPolicy{Initial: time.Second, Max: 4 * time.Second})
	manager.SetClock(func() time.Time { return now })
	if _, err := manager.Open("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	snapshot, err := manager.Snapshot("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Activate("deck-a", snapshot.Generation); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Disconnect("deck-a"); err != nil {
		t.Fatal(err)
	}
	disconnected, err := manager.Snapshot("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	if disconnected.Status != DeviceDisconnected {
		t.Fatalf("desconexão não observável: %+v", disconnected)
	}
	if err := manager.Activate("deck-a", disconnected.Generation); !errors.Is(err, ErrInvalidDevice) {
		t.Fatalf("activate reativou dispositivo desconectado: %v", err)
	}
	if _, err := manager.RenderSafe("deck-a"); !errors.Is(err, ErrDeviceSafe) {
		t.Fatalf("render seguro de dispositivo desconectado=%v", err)
	}
	if _, err := manager.Render(Frame{Device: "deck-a", Model: testModel}); !errors.Is(err, ErrDeviceSafe) {
		t.Fatalf("render ativo de dispositivo desconectado=%v", err)
	}
	after, err := manager.Snapshot("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	if after != disconnected {
		t.Fatalf("tentativas failclosed alteraram estado: antes=%+v depois=%+v", disconnected, after)
	}
}

func TestManagerDisconnectIsIdempotentAndBackoffIsOverflowSafe(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	maxDuration := time.Duration(1<<63 - 1)
	manager := NewManager(NewRenderer(), BackoffPolicy{Initial: time.Duration(1 << 62), Max: maxDuration})
	manager.SetClock(func() time.Time { return now })
	if _, err := manager.Open("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	firstDelay, err := manager.Disconnect("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.Snapshot("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	secondDelay, err := manager.Disconnect("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	second, err := manager.Snapshot("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	if secondDelay != firstDelay || second != first {
		t.Fatalf("desconexão repetida alterou backoff/generation: primeira=%+v/%v segunda=%+v/%v", first, firstDelay, second, secondDelay)
	}
	if first.NextBackoff != maxDuration {
		t.Fatalf("backoff deveria saturar sem overflow: %v", first.NextBackoff)
	}

	now = now.Add(firstDelay)
	if _, err := manager.Open("deck-a", testModel); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Disconnect("deck-a"); err != nil {
		t.Fatal(err)
	}
	third, err := manager.Snapshot("deck-a")
	if err != nil {
		t.Fatal(err)
	}
	if third.NextBackoff != maxDuration || third.Generation != first.Generation+2 || third.Reconnects != first.Reconnects+1 {
		t.Fatalf("reconexão/desconexão perdeu saturação ou contou errado: %+v", third)
	}
}

func TestManagerConcurrentSnapshotsRendersAndLifecycleAreSafe(t *testing.T) {
	manager := NewManager(NewRenderer(), BackoffPolicy{Initial: time.Second, Max: time.Minute})
	const devices = 8
	for index := 0; index < devices; index++ {
		device := DeviceID("deck-" + string(rune('a'+index)))
		if _, err := manager.Open(device, testModel); err != nil {
			t.Fatal(err)
		}
		snapshot, err := manager.Snapshot(device)
		if err != nil {
			t.Fatal(err)
		}
		if err := manager.Activate(device, snapshot.Generation); err != nil {
			t.Fatal(err)
		}
	}

	var wg sync.WaitGroup
	for index := 0; index < devices; index++ {
		device := DeviceID("deck-" + string(rune('a'+index)))
		wg.Add(1)
		go func(device DeviceID) {
			defer wg.Done()
			for attempt := 0; attempt < 32; attempt++ {
				if _, err := manager.Snapshot(device); err != nil {
					t.Errorf("snapshot concorrente: %v", err)
					return
				}
				if manager.CanReconnect(device) {
					t.Errorf("dispositivo conectado não deveria permitir reconexão")
					return
				}
				if _, err := manager.Render(Frame{Device: device, Model: testModel, Keys: map[int]KeyView{attempt % testModel.KeyCount(): {Title: "A"}}}); err != nil {
					if !errors.Is(err, ErrDeviceSafe) {
						t.Errorf("render concorrente: %v", err)
						return
					}
				}
			}
		}(device)
	}
	for attempt := 0; attempt < 8; attempt++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = manager.LockOrLogout()
		}()
	}
	wg.Wait()

	for index := 0; index < devices; index++ {
		device := DeviceID("deck-" + string(rune('a'+index)))
		snapshot, err := manager.Snapshot(device)
		if err != nil {
			t.Fatal(err)
		}
		if snapshot.Status != DeviceSafe || snapshot.Generation < 2 {
			t.Fatalf("lifecycle concorrente deixou estado inválido: %+v", snapshot)
		}
	}
}
