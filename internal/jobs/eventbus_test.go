package jobs

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEventBus_SubscribeAndPublish(t *testing.T) {
	eb := NewEventBus()
	var received atomic.Value
	done := make(chan struct{})

	eb.Subscribe("test.event", "sub-1", func(ctx context.Context, name string, payload map[string]any) {
		received.Store(payload["key"])
		close(done)
	})

	eb.Publish(context.Background(), "test.event", map[string]any{"key": "value"})

	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for event")
	}

	if received.Load() != "value" {
		t.Errorf("expected 'value', got %v", received.Load())
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	eb := NewEventBus()
	var count atomic.Int32
	var wg sync.WaitGroup
	wg.Add(3)

	for i := 0; i < 3; i++ {
		id := i
		eb.Subscribe("multi.event", "sub-"+string(rune('a'+id)), func(ctx context.Context, name string, payload map[string]any) {
			count.Add(1)
			wg.Done()
		})
	}

	eb.Publish(context.Background(), "multi.event", map[string]any{})

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout waiting for all subscribers")
	}

	if got := int(count.Load()); got != 3 {
		t.Errorf("expected 3 calls, got %d", got)
	}
}

func TestEventBus_Unsubscribe(t *testing.T) {
	eb := NewEventBus()
	var count atomic.Int32

	eb.Subscribe("unsub.event", "sub-1", func(ctx context.Context, name string, payload map[string]any) {
		count.Add(1)
	})

	eb.Unsubscribe("unsub.event", "sub-1")
	eb.Publish(context.Background(), "unsub.event", map[string]any{})

	time.Sleep(50 * time.Millisecond)

	if got := int(count.Load()); got != 0 {
		t.Errorf("expected 0 calls after unsubscribe, got %d", got)
	}
}

func TestEventBus_UnsubscribeAll(t *testing.T) {
	eb := NewEventBus()
	var count atomic.Int32

	handler := func(ctx context.Context, name string, payload map[string]any) {
		count.Add(1)
	}

	eb.Subscribe("event.a", "job-1", handler)
	eb.Subscribe("event.b", "job-1", handler)

	eb.UnsubscribeAll("job-1")

	eb.Publish(context.Background(), "event.a", map[string]any{})
	eb.Publish(context.Background(), "event.b", map[string]any{})

	time.Sleep(50 * time.Millisecond)

	if got := int(count.Load()); got != 0 {
		t.Errorf("expected 0 calls after UnsubscribeAll, got %d", got)
	}
}

func TestEventBus_PublishNoListeners(t *testing.T) {
	eb := NewEventBus()
	eb.Publish(context.Background(), "no.listeners", map[string]any{})
}

func TestEventBus_HandlerPanicDoesNotCrash(t *testing.T) {
	eb := NewEventBus()
	var count atomic.Int32
	var wg sync.WaitGroup
	wg.Add(2)

	eb.Subscribe("panic.event", "panic-sub", func(ctx context.Context, name string, payload map[string]any) {
		count.Add(1)
		wg.Done()
		panic("boom")
	})

	eb.Subscribe("panic.event", "safe-sub", func(ctx context.Context, name string, payload map[string]any) {
		count.Add(1)
		wg.Done()
	})

	eb.Publish(context.Background(), "panic.event", map[string]any{})

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(1 * time.Second):
		t.Fatal("timeout: panic in one handler should not affect others")
	}

	if got := int(count.Load()); got != 2 {
		t.Errorf("expected 2 handlers called, got %d", got)
	}
}

func TestEventBus_CloseStopsPublish(t *testing.T) {
	eb := NewEventBus()
	var count atomic.Int32

	eb.Subscribe("close.event", "sub-1", func(ctx context.Context, name string, payload map[string]any) {
		count.Add(1)
	})

	eb.Close()
	eb.Publish(context.Background(), "close.event", map[string]any{})

	time.Sleep(50 * time.Millisecond)

	if got := int(count.Load()); got != 0 {
		t.Errorf("expected 0 after Close, got %d", got)
	}
}

func TestEventBus_CloseStopsSubscribe(t *testing.T) {
	eb := NewEventBus()
	eb.Close()

	eb.Subscribe("post.close", "sub-1", func(ctx context.Context, name string, payload map[string]any) {
		t.Error("should not be called")
	})

	if count := eb.SubscriberCount("post.close"); count != 0 {
		t.Errorf("expected 0 subscribers after Close, got %d", count)
	}
}

func TestEventBus_SubscriberCount(t *testing.T) {
	eb := NewEventBus()

	eb.Subscribe("count.event", "sub-1", func(ctx context.Context, name string, payload map[string]any) {})
	eb.Subscribe("count.event", "sub-2", func(ctx context.Context, name string, payload map[string]any) {})

	if got := eb.SubscriberCount("count.event"); got != 2 {
		t.Errorf("expected 2, got %d", got)
	}

	if got := eb.SubscriberCount("other.event"); got != 0 {
		t.Errorf("expected 0 for unknown event, got %d", got)
	}
}

func TestEventBus_Events(t *testing.T) {
	eb := NewEventBus()

	eb.Subscribe("alpha", "sub", func(ctx context.Context, name string, payload map[string]any) {})
	eb.Subscribe("beta", "sub", func(ctx context.Context, name string, payload map[string]any) {})

	events := eb.Events()
	found := make(map[string]bool)
	for _, e := range events {
		found[e] = true
	}

	if !found["alpha"] || !found["beta"] {
		t.Errorf("expected alpha and beta, got %v", events)
	}
}
