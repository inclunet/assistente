package shell

import (
	"assistente/internal/terminal"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

type fakeTerminalSessionManager struct {
	sessions      []terminal.SessionInfo
	createdName   string
	createdDir    string
	interruptedID string
	closedID      string
	err           error
}

func (f *fakeTerminalSessionManager) List() []terminal.SessionInfo { return f.sessions }
func (f *fakeTerminalSessionManager) CreateInfo(name, workDir string) (terminal.SessionInfo, error) {
	f.createdName, f.createdDir = name, workDir
	if f.err != nil {
		return terminal.SessionInfo{}, f.err
	}
	return terminal.SessionInfo{ID: "term-1", Name: name, CWD: workDir, State: "idle"}, nil
}
func (f *fakeTerminalSessionManager) Interrupt(id string) error {
	f.interruptedID = id
	return f.err
}
func (f *fakeTerminalSessionManager) Close(id string) error {
	f.closedID = id
	return f.err
}

func executeTerminalSession(t *testing.T, tool *TerminalSession, args string) map[string]any {
	t.Helper()
	result, err := tool.Execute(context.Background(), json.RawMessage(args))
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.IsError {
		t.Fatalf("resultado inesperadamente falhou: %s", result.Content)
	}
	return result.Metadata
}

func TestTerminalSessionList(t *testing.T) {
	manager := &fakeTerminalSessionManager{
		sessions: []terminal.SessionInfo{{ID: "one"}, {ID: "two"}},
	}
	metadata := executeTerminalSession(t, NewTerminalSession(manager, "/workspace"), `{"action":"list"}`)
	if got := metadata["count"]; got != 2 {
		t.Fatalf("count = %#v, queria 2", got)
	}
}

func TestTerminalSessionCreateReturnsDeepLink(t *testing.T) {
	manager := &fakeTerminalSessionManager{}
	metadata := executeTerminalSession(t, NewTerminalSession(manager, "/workspace"), `{"action":"create","name":"build"}`)

	if manager.createdName != "build" || manager.createdDir != "/workspace" {
		t.Fatalf("create = (%q, %q)", manager.createdName, manager.createdDir)
	}
	if got := metadata["deepLink"]; got != "assistente://terminal/term-1" {
		t.Fatalf("deepLink = %#v", got)
	}
}

func TestTerminalSessionCreateUsesIDWhenNameIsEmpty(t *testing.T) {
	manager := &fakeTerminalSessionManager{}
	result, err := NewTerminalSession(manager, "/workspace").Execute(
		context.Background(),
		json.RawMessage(`{"action":"create"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || !strings.Contains(result.Content, "Terminal criado: term-1") {
		t.Fatalf("resultado=%#v", result)
	}
}

func TestTerminalSessionCreateRejectsWorkingDirectoryOutsideProject(t *testing.T) {
	manager := &fakeTerminalSessionManager{}
	result, err := NewTerminalSession(manager, t.TempDir()).Execute(
		context.Background(),
		json.RawMessage(`{"action":"create","working_directory":"../../outside"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError || manager.createdDir != "" {
		t.Fatalf("resultado=%#v createdDir=%q", result, manager.createdDir)
	}
}

func TestTerminalSessionInterruptAndClose(t *testing.T) {
	manager := &fakeTerminalSessionManager{}
	tool := NewTerminalSession(manager, "/workspace")

	executeTerminalSession(t, tool, `{"action":"interrupt","terminal_id":"term-1"}`)
	executeTerminalSession(t, tool, `{"action":"close","terminal_id":"term-2"}`)

	if manager.interruptedID != "term-1" || manager.closedID != "term-2" {
		t.Fatalf("interrupt=%q close=%q", manager.interruptedID, manager.closedID)
	}
}

func TestTerminalSessionReportsManagerError(t *testing.T) {
	manager := &fakeTerminalSessionManager{err: errors.New("boom")}
	result, err := NewTerminalSession(manager, "/workspace").Execute(
		context.Background(),
		json.RawMessage(`{"action":"close","terminal_id":"term-1"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatalf("esperava erro, recebeu %#v", result)
	}
}
