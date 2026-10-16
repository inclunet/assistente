package workspace

import (
	"context"
	"errors"
	"testing"
)

func TestTerminalCommandSnapshotPinsBinding(t *testing.T) {
	m := commandSnapshotManager(t)
	m.mu.Lock()
	m.active.Tabs.Items = []Tab{terminalCommandTab("terminal", "session-original")}
	m.active.Tabs.Active = "terminal"
	m.mu.Unlock()
	var original CommandSnapshot
	err := m.WithTerminalCommandSnapshot(context.Background(), func(snapshot CommandSnapshot, id string) error {
		original = snapshot
		if id != "session-original" || snapshot.Tab.Type != TabTypeTerminal || snapshot.ActiveTabID != "terminal" {
			t.Fatalf("wrong target: %+v %s", snapshot, id)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	m.mu.Lock()
	m.active.Tabs.Items[0].State["sessionId"] = "session-new"
	m.mu.Unlock()
	err = m.WithTerminalCommandSnapshot(context.Background(), func(snapshot CommandSnapshot, id string) error {
		if snapshot == original || id != "session-new" {
			t.Fatal("binding change did not invalidate snapshot")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestTerminalCommandSnapshotRejectsMissingOrInvalidTarget(t *testing.T) {
	for _, value := range []any{nil, "", " with-space ", 3} {
		m := commandSnapshotManager(t)
		m.mu.Lock()
		m.active.Tabs.Items = []Tab{terminalCommandTab("terminal", "session")}
		m.active.Tabs.Items[0].State["sessionId"] = value
		m.active.Tabs.Active = "terminal"
		m.mu.Unlock()
		if err := m.WithTerminalCommandSnapshot(context.Background(), func(CommandSnapshot, string) error { t.Fatal("invalid callback"); return nil }); err == nil {
			t.Fatal("invalid binding accepted", value)
		}
	}
	m := commandSnapshotManager(t)
	if err := m.WithTerminalCommandSnapshot(context.Background(), func(CommandSnapshot, string) error { t.Fatal("chat accepted"); return nil }); err == nil {
		t.Fatal("non-terminal allowed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.WithTerminalCommandSnapshot(ctx, func(CommandSnapshot, string) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
