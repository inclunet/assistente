package confirmation

import (
	"context"
	"testing"
	"time"
)

func TestRequestConfirmation_Approved(t *testing.T) {
	var mgr *Manager
	mgr = NewManager(func(event string, data any) {
		// Callback roda na goroutine do teste, antes do select — sem race com sleep/goroutine extra.
		if event != "tool:confirm_command" {
			t.Errorf("expected event 'tool:confirm_command', got %q", event)
		}
		dataMap, ok := data.(map[string]string)
		if !ok {
			t.Error("expected data to be map[string]string")
			return
		}
		if err := mgr.Respond(dataMap["id"], true); err != nil {
			t.Errorf("Respond error: %v", err)
		}
	})

	approved, err := mgr.RequestConfirmation(context.Background(), "ls -la", "/home")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !approved {
		t.Error("expected approved=true")
	}
	if mgr.PendingCount() != 0 {
		t.Errorf("expected 0 pending, got %d", mgr.PendingCount())
	}
}

func TestRequestConfirmation_Denied(t *testing.T) {
	var mgr *Manager
	mgr = NewManager(func(event string, data any) {
		// Auto-respond com negação
		go func() {
			time.Sleep(10 * time.Millisecond)
			dataMap := data.(map[string]string)
			mgr.Respond(dataMap["id"], false)
		}()
	})

	approved, err := mgr.RequestConfirmation(context.Background(), "rm -rf tmp", "/home")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if approved {
		t.Error("expected approved=false")
	}
}

func TestRequestConfirmation_ContextCancelled(t *testing.T) {
	mgr := NewManager(func(string, any) {})

	ctx, cancel := context.WithCancel(context.Background())

	// Cancela após 50ms
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	approved, err := mgr.RequestConfirmation(ctx, "rm -rf /", "/")
	if err == nil {
		t.Error("expected error on cancel")
	}
	if approved {
		t.Error("cancelled request should not be approved")
	}
}

func TestRespond_NotFound(t *testing.T) {
	mgr := NewManager(func(string, any) {})

	err := mgr.Respond("nonexistent", true)
	if err == nil {
		t.Error("expected error for nonexistent request")
	}
}

func TestRespond_DoubleResponse(t *testing.T) {
	var mgr *Manager
	done := make(chan struct{})
	var secondErr error

	mgr = NewManager(func(event string, data any) {
		go func() {
			defer close(done)
			time.Sleep(10 * time.Millisecond)
			dataMap := data.(map[string]string)
			mgr.Respond(dataMap["id"], true)
			// Espera cleanup do RequestConfirmation (defer delete)
			time.Sleep(50 * time.Millisecond)
			// Segunda resposta deve falhar (request já foi removida do pending)
			secondErr = mgr.Respond(dataMap["id"], false)
		}()
	})

	mgr.RequestConfirmation(context.Background(), "test", "/")

	// Espera a goroutine interna completar
	<-done
	if secondErr == nil {
		t.Error("expected error on double response")
	}
}
