package wailsapi

import (
	"assistente/internal/acp"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestACPCommandsNotWired(t *testing.T) {
	t.Parallel()
	api := NewACPCommands()
	if _, err := api.GetAgentSessionCommands("conversa-1"); !errors.Is(err, ErrACPCommandsNotWired) {
		t.Fatalf("got %v", err)
	}
}

func TestACPCommandsPreservesConversationIDOnAuthError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("sem sessão")
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
	})
	t.Cleanup(mgr.Shutdown)
	api := NewACPCommands()
	AttachACPCommands(api, stubSession{err: wantErr}, mgr)

	got, err := api.GetAgentSessionCommands(" conversa-1 ")

	if !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}
	if got.ConversationID != "conversa-1" {
		t.Fatalf("conversationId = %q, want %q", got.ConversationID, "conversa-1")
	}
	if got.Commands == nil {
		t.Fatal("commands deve permanecer lista vazia, não nil")
	}
}

func TestACPCommandsUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "acp_commands.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("acp_commands.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("acp_commands.go deve chamar WithUser(session,")
	}
}
