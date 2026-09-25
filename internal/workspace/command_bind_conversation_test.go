package workspace

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const (
	bindConversationID  = "01970a9e-1234-7000-8000-abcdef123456"
	otherConversationID = "01970a9e-1234-7000-8000-fedcba654321"
)

func bindConversationManager(t *testing.T) (*Manager, string) {
	t.Helper()
	root := t.TempDir()
	workspacePath := filepath.Join(root, "workspace")
	manager := NewManager(filepath.Join(root, "home"))
	if err := manager.Initialize(workspacePath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if err := manager.AddTab(Tab{ID: "editor-bind", Type: TabTypeEditor, Title: "Editor"}); err != nil {
		t.Fatalf("AddTab editor: %v", err)
	}
	return manager, workspacePath
}

func TestBindConversationForCommandPersistsAndReloads(t *testing.T) {
	manager, workspacePath := bindConversationManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	if expected.Tab.Type != TabTypeEditor {
		t.Fatalf("expected editor target, got %q", expected.Tab.Type)
	}

	bound, err := manager.BindConversationForCommand(context.Background(), expected, bindConversationID)
	if err != nil {
		t.Fatalf("BindConversationForCommand: %v", err)
	}
	if got := bound.FindTab(expected.ActiveTabID); got == nil || got.ConversationID != bindConversationID {
		t.Fatalf("returned binding incorrect: %#v", got)
	}

	reloaded := NewManager(manager.homeDir)
	if err := reloaded.Initialize(workspacePath); err != nil {
		t.Fatalf("reload Initialize: %v", err)
	}
	if got := reloaded.Active().FindTab(expected.ActiveTabID); got == nil || got.ConversationID != bindConversationID {
		t.Fatalf("persisted binding incorrect: %#v", got)
	}
}

func TestBindConversationForCommandRejectsStaleCancelAndInvalidID(t *testing.T) {
	manager, _ := bindConversationManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	if _, err := manager.BindConversationForCommand(nil, expected, bindConversationID); !errors.Is(err, ErrCommandBindConversationNilContext) { //nolint:staticcheck // Verifica a rejeição explícita de contexto nil.
		t.Fatalf("nil context error: %v", err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.BindConversationForCommand(canceled, expected, bindConversationID); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled context error: %v", err)
	}
	for _, invalid := range []string{"", "not-a-uuid", "550e8400-e29b-41d4-a716-446655440000"} {
		if _, err := manager.BindConversationForCommand(context.Background(), expected, invalid); !errors.Is(err, ErrCommandBindConversationInvalidID) {
			t.Errorf("invalid ID %q error: %v", invalid, err)
		}
	}
	if err := manager.AddTab(Tab{ID: "stale-bind", Type: TabTypeChat}); err != nil {
		t.Fatalf("AddTab stale fixture: %v", err)
	}
	if _, err := manager.BindConversationForCommand(context.Background(), expected, bindConversationID); !errors.Is(err, ErrCommandBindConversationStale) {
		t.Fatalf("stale snapshot error: %v", err)
	}
}

func TestBindConversationForCommandNoOverwriteAndNoopWithoutWrite(t *testing.T) {
	manager, _ := bindConversationManager(t)
	active := manager.Active()
	if err := manager.UpdateTab(active.Tabs.Active, map[string]any{"conversation_id": bindConversationID}); err != nil {
		t.Fatalf("seed conversation: %v", err)
	}
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	workspaceFile := filepath.Join(manager.activePath, assistenteDir, workspaceFile)
	before, err := os.ReadFile(workspaceFile)
	if err != nil {
		t.Fatalf("read workspace before noop: %v", err)
	}
	if _, err := manager.BindConversationForCommand(context.Background(), expected, bindConversationID); err != nil {
		t.Fatalf("same conversation should be no-op: %v", err)
	}
	after, err := os.ReadFile(workspaceFile)
	if err != nil {
		t.Fatalf("read workspace after noop: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("same conversation rewrote workspace")
	}
	if _, err := manager.BindConversationForCommand(context.Background(), expected, otherConversationID); !errors.Is(err, ErrCommandBindConversationConflict) {
		t.Fatalf("different conversation should be rejected: %v", err)
	}
}

func TestBindConversationForCommandRejectsUnsupportedChat(t *testing.T) {
	manager := NewManager(t.TempDir())
	workspacePath := t.TempDir()
	if err := manager.Initialize(workspacePath); err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	if _, err := manager.BindConversationForCommand(context.Background(), expected, bindConversationID); !errors.Is(err, ErrCommandBindConversationUnsupportedType) {
		t.Fatalf("chat target should be rejected: %v", err)
	}
}

func TestBindConversationForCommandPersistenceFailureRollsBack(t *testing.T) {
	manager, _ := bindConversationManager(t)
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("CommandSnapshot: %v", err)
	}
	before, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot before failure: %v", err)
	}
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block"), 0600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	manager.activePath = filepath.Join(blocker, "workspace.yaml")

	if _, err := manager.BindConversationForCommand(context.Background(), expected, bindConversationID); err == nil {
		t.Fatal("expected persistence failure")
	}
	after, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatalf("snapshot after failure: %v", err)
	}
	if after != before {
		t.Fatalf("persistence failure changed snapshot: before=%#v after=%#v", before, after)
	}
	if got := manager.Active().FindTab(expected.ActiveTabID); got == nil || got.ConversationID != "" {
		t.Fatalf("persistence failure left binding in memory: %#v", got)
	}
}
