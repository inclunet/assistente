package terminal

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestRunEphemeralNaoCriaSecaoPersistente(t *testing.T) {
	events := []string{}
	mgr := NewManager(DefaultManagerConfig(), func(event string, _ any) {
		events = append(events, event)
	})

	entry, err := mgr.RunEphemeral(context.Background(), "", "echo hello", 0, "test")
	if err != nil {
		t.Fatalf("RunEphemeral erro: %v", err)
	}
	if entry == nil || entry.Output == "" {
		t.Fatalf("entry vazia: %#v", entry)
	}
	if len(mgr.List()) != 0 {
		t.Fatalf("RunEphemeral não deve deixar sessão no pool, got %d", len(mgr.List()))
	}
	for _, ev := range events {
		if ev == "terminal:session_created" || ev == "terminal:session_closed" {
			t.Errorf("RunEphemeral não deve emitir %q, eventos: %v", ev, events)
		}
	}
	if mgr.Stats().TotalSessions != 0 {
		t.Errorf("Stats deve ser 0 após ephemeral, got %d", mgr.Stats().TotalSessions)
	}
}

func TestRunEphemeralTimeoutCompletesCleanupBeforeReturning(t *testing.T) {
	config := DefaultManagerConfig()
	manager := NewManager(config, nil)
	command := "printf 'antes\\n'; sleep 10"
	if runtime.GOOS == "windows" {
		command = "Write-Output 'antes'; Start-Sleep -Seconds 10"
	}

	started := time.Now()
	entry, err := manager.RunEphemeral(context.Background(), "", command, 150*time.Millisecond, "test")
	elapsed := time.Since(started)

	if err == nil {
		t.Fatal("esperava timeout")
	}
	if entry == nil || entry.ExitCode != -1 {
		t.Fatalf("entry de timeout inválida: %#v", entry)
	}
	if elapsed > 6*time.Second {
		t.Fatalf("cleanup após timeout demorou %s", elapsed)
	}
	if got := manager.Stats().TotalSessions; got != 0 {
		t.Fatalf("sessão efêmera vazou para o pool: %d", got)
	}
}
