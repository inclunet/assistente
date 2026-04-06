package messaging

import (
	"sync"
	"testing"
	"time"
)

func TestNotifier_RegisterAndNotify(t *testing.T) {
	n := NewResponseNotifier()

	var received string
	var mu sync.Mutex

	n.Register(1, ResponseCallback{
		Channel: "telegram",
		ChatID:  "123",
		Callback: func(response string, msgID uint) {
			mu.Lock()
			received = response
			mu.Unlock()
		},
	})

	if n.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", n.PendingCount())
	}

	n.Notify(1, "Hello from assistant", 42)

	// Aguarda goroutine do callback
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if received != "Hello from assistant" {
		t.Fatalf("expected 'Hello from assistant', got %q", received)
	}
	mu.Unlock()

	// Callback deve ter sido removido
	if n.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after notify, got %d", n.PendingCount())
	}
}

func TestNotifier_NotifyWithoutCallbacks(t *testing.T) {
	n := NewResponseNotifier()

	// Não deve causar panic nem erro
	n.Notify(999, "response without listener", 0)

	if n.PendingCount() != 0 {
		t.Fatalf("expected 0 pending, got %d", n.PendingCount())
	}
}

func TestNotifier_MultipleCallbacksSameConversation(t *testing.T) {
	n := NewResponseNotifier()

	var mu sync.Mutex
	results := make([]string, 0)

	n.Register(1, ResponseCallback{
		Channel: "telegram",
		ChatID:  "123",
		Callback: func(response string, msgID uint) {
			mu.Lock()
			results = append(results, "telegram:"+response)
			mu.Unlock()
		},
	})

	n.Register(1, ResponseCallback{
		Channel: "signal",
		ChatID:  "+55119999",
		Callback: func(response string, msgID uint) {
			mu.Lock()
			results = append(results, "signal:"+response)
			mu.Unlock()
		},
	})

	if n.PendingCount() != 2 {
		t.Fatalf("expected 2 pending, got %d", n.PendingCount())
	}

	n.Notify(1, "multi-channel", 100)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d: %v", len(results), results)
	}
	mu.Unlock()

	if n.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after notify, got %d", n.PendingCount())
	}
}

func TestNotifier_DifferentConversations(t *testing.T) {
	n := NewResponseNotifier()

	var mu sync.Mutex
	results := make(map[uint]string)

	n.Register(1, ResponseCallback{
		Channel: "telegram",
		ChatID:  "111",
		Callback: func(response string, msgID uint) {
			mu.Lock()
			results[1] = response
			mu.Unlock()
		},
	})

	n.Register(2, ResponseCallback{
		Channel: "telegram",
		ChatID:  "222",
		Callback: func(response string, msgID uint) {
			mu.Lock()
			results[2] = response
			mu.Unlock()
		},
	})

	// Notifica apenas conversa 1
	n.Notify(1, "response for conv 1", 200)

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if results[1] != "response for conv 1" {
		t.Fatalf("expected 'response for conv 1', got %q", results[1])
	}
	if _, ok := results[2]; ok {
		t.Fatalf("conv 2 should not have been notified")
	}
	mu.Unlock()

	// Conv 2 ainda deve estar pendente
	if n.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", n.PendingCount())
	}
}

func TestNotifier_Cancel(t *testing.T) {
	n := NewResponseNotifier()

	called := false
	n.Register(1, ResponseCallback{
		Channel: "sip",
		ChatID:  "caller@pbx",
		TraceID: "trace-cancel-test",
		Callback: func(response string, msgID uint) {
			called = true
		},
	})

	if n.PendingCount() != 1 {
		t.Fatalf("expected 1 pending, got %d", n.PendingCount())
	}

	// Cancel remove sem chamar callback
	n.Cancel(1)

	if n.PendingCount() != 0 {
		t.Fatalf("expected 0 pending after cancel, got %d", n.PendingCount())
	}

	// Notify após cancel não deve chamar callback
	n.Notify(1, "late response", 42)
	time.Sleep(50 * time.Millisecond)

	if called {
		t.Fatal("callback should not have been called after cancel")
	}
}

func TestNotifier_CancelNonExistent(t *testing.T) {
	n := NewResponseNotifier()

	// Cancel de conversa inexistente não deve causar panic
	n.Cancel(999)

	if n.PendingCount() != 0 {
		t.Fatalf("expected 0 pending, got %d", n.PendingCount())
	}
}
