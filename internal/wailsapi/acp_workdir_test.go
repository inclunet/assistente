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

func TestACPWorkDirNotWired(t *testing.T) {
	t.Parallel()
	api := NewACPWorkDir()
	if _, err := api.GetAgentConversationWorkDir("conversa-1"); !errors.Is(err, ErrACPWorkDirNotWired) {
		t.Fatalf("GetAgentConversationWorkDir: got %v", err)
	}
	if _, err := api.SetAgentConversationWorkDir("conversa-1", ""); !errors.Is(err, ErrACPWorkDirNotWired) {
		t.Fatalf("SetAgentConversationWorkDir: got %v", err)
	}
}

func TestACPWorkDirPreservesConversationIDOnAuthError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("sem sessão")
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
	})
	t.Cleanup(mgr.Shutdown)
	api := NewACPWorkDir()
	AttachACPWorkDir(api, stubSession{err: wantErr}, mgr)

	got, err := api.GetAgentConversationWorkDir(" conversa-1 ")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Get: got error %v, want %v", err, wantErr)
	}
	if got.ConversationID != "conversa-1" {
		t.Fatalf("Get conversationId = %q, want %q", got.ConversationID, "conversa-1")
	}

	got, err = api.SetAgentConversationWorkDir(" conversa-2 ", t.TempDir())
	if !errors.Is(err, wantErr) {
		t.Fatalf("Set: got error %v, want %v", err, wantErr)
	}
	if got.ConversationID != "conversa-2" {
		t.Fatalf("Set conversationId = %q, want %q", got.ConversationID, "conversa-2")
	}
}

func TestACPWorkDirUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "acp_workdir.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("acp_workdir.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("acp_workdir.go deve chamar WithUser(session,")
	}
}
