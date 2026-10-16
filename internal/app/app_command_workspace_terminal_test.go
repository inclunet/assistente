package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/terminal"
	"assistente/internal/workspace"
)

type fakeCommandTerminalCreator struct {
	info        terminal.SessionInfo
	live        bool
	closeCalls  []string
	createCalls int
	createErr   error
	closeErr    error
	afterCreate func()
}

func (f *fakeCommandTerminalCreator) CreateInfo(name, workDir string) (terminal.SessionInfo, error) {
	f.createCalls++
	if f.createErr != nil {
		return terminal.SessionInfo{}, f.createErr
	}
	if name != "" || workDir == "" {
		return terminal.SessionInfo{}, errors.New("cwd/nome inesperados")
	}
	f.live = true
	if f.afterCreate != nil {
		f.afterCreate()
	}
	return f.info, nil
}

func (f *fakeCommandTerminalCreator) Close(id string) error {
	f.closeCalls = append(f.closeCalls, id)
	f.live = false
	return f.closeErr
}

func (f *fakeCommandTerminalCreator) Has(id string) bool { return f.live && id == f.info.ID }

func TestCreateTerminalSessionAndCommitPersisteAbaSemCleanupNoSucesso(t *testing.T) {
	root := t.TempDir()
	manager := workspace.NewManager(filepath.Join(root, "home"))
	workspacePath := filepath.Join(root, "workspace")
	if err := manager.Initialize(workspacePath); err != nil {
		t.Fatal(err)
	}
	expected, err := manager.CommandSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	sessions := &fakeCommandTerminalCreator{info: terminal.SessionInfo{ID: "session-created"}}
	created, err := createTerminalSessionAndCommit(context.Background(), "tab-created", sessions, func(ctx context.Context, info terminal.SessionInfo, _ string) (*workspace.Workspace, error) {
		return manager.AddTerminalTabForCommand(ctx, expected, workspace.Tab{ID: "tab-created", Type: workspace.TabTypeTerminal, State: map[string]any{"sessionId": info.ID}}, sessions)
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.FindTab("tab-created").State["sessionId"] != "session-created" || len(sessions.closeCalls) != 0 {
		t.Fatalf("sucesso alterou lifecycle indevidamente: created=%#v closes=%v", created, sessions.closeCalls)
	}
	data, err := os.ReadFile(filepath.Join(workspacePath, ".assistente", "workspace.yaml"))
	if err != nil || !strings.Contains(string(data), "session-created") {
		t.Fatalf("persistência terminal ausente: err=%v data=%s", err, data)
	}
}

func TestCreateTerminalSessionAndCommitCompensaSomenteSessaoNova(t *testing.T) {
	sessions := &fakeCommandTerminalCreator{info: terminal.SessionInfo{ID: "session-new"}}
	want := errors.New("commit falhou")
	_, err := createTerminalSessionAndCommit(context.Background(), "tab-failed", sessions, func(context.Context, terminal.SessionInfo, string) (*workspace.Workspace, error) {
		return nil, want
	})
	if !errors.Is(err, want) {
		t.Fatalf("erro primário perdido: %v", err)
	}
	if len(sessions.closeCalls) != 1 || sessions.closeCalls[0] != "session-new" {
		t.Fatalf("cleanup incorreto: %v", sessions.closeCalls)
	}
}

func TestCreateTerminalSessionAndCommitCancellationAndInvalidSession(t *testing.T) {
	for _, scenario := range []string{"cancel-before", "cancel-during-create", "exited", "empty-id", "create-error"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			sessions := &fakeCommandTerminalCreator{info: terminal.SessionInfo{ID: "new-session"}}
			wantCreate, wantClose := 1, 1
			switch scenario {
			case "cancel-before":
				cancel()
				wantCreate, wantClose = 0, 0
			case "cancel-during-create":
				sessions.afterCreate = cancel
			case "exited":
				sessions.afterCreate = func() { sessions.live = false }
			case "empty-id":
				sessions.info.ID = ""
				wantClose = 0
			case "create-error":
				sessions.createErr = errors.New("create failed")
				wantClose = 0
			}
			commits := 0
			_, err := createTerminalSessionAndCommit(ctx, "tab", sessions, func(context.Context, terminal.SessionInfo, string) (*workspace.Workspace, error) {
				commits++
				return &workspace.Workspace{}, nil
			})
			if err == nil || commits != 0 || sessions.createCalls != wantCreate || len(sessions.closeCalls) != wantClose {
				t.Fatalf("err=%v commits=%d creates=%d closes=%v", err, commits, sessions.createCalls, sessions.closeCalls)
			}
			if wantClose == 1 && sessions.closeCalls[0] != "new-session" {
				t.Fatal("compensou outra sessão")
			}
		})
	}
}

func TestCreateTerminalSessionAndCommitPreservesCleanupFailure(t *testing.T) {
	primary, cleanup := errors.New("stale target"), errors.New("close failed")
	sessions := &fakeCommandTerminalCreator{info: terminal.SessionInfo{ID: "new-session"}, closeErr: cleanup}
	result, err := createTerminalSessionAndCommit(context.Background(), "tab", sessions, func(context.Context, terminal.SessionInfo, string) (*workspace.Workspace, error) {
		return nil, primary
	})
	if result != nil || !errors.Is(err, primary) || !errors.Is(err, cleanup) || len(sessions.closeCalls) != 1 {
		t.Fatalf("resultado=%v erro=%v cleanup=%v", result, err, sessions.closeCalls)
	}
}
