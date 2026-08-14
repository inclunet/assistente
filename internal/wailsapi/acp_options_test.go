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

func TestACPOptionsNotWired(t *testing.T) {
	t.Parallel()
	api := NewACPOptions()
	if _, err := api.GetAgentSessionOptions("conversa-1"); !errors.Is(err, ErrACPOptionsNotWired) {
		t.Fatalf("GetAgentSessionOptions: got %v", err)
	}
	if _, err := api.SetAgentSessionOption("conversa-1", "model", "x"); !errors.Is(err, ErrACPOptionsNotWired) {
		t.Fatalf("SetAgentSessionOption: got %v", err)
	}
}

func TestACPOptionsPreservesConversationIDOnAuthError(t *testing.T) {
	t.Parallel()
	wantErr := errors.New("sem sessão")
	mgr := acp.NewManager(acp.ManagerConfig{
		WorkDir: func() (string, error) { return t.TempDir(), nil },
	})
	t.Cleanup(mgr.Shutdown)
	api := NewACPOptions()
	AttachACPOptions(api, stubSession{err: wantErr}, mgr, nil)

	got, err := api.GetAgentSessionOptions(" conversa-1 ")

	if !errors.Is(err, wantErr) {
		t.Fatalf("got error %v, want %v", err, wantErr)
	}
	if got.ConversationID != "conversa-1" {
		t.Fatalf("conversationId = %q, want %q", got.ConversationID, "conversa-1")
	}
	if got.Options == nil {
		t.Fatal("options deve permanecer lista vazia, não nil")
	}

	gotSet, err := api.SetAgentSessionOption(" conversa-2 ", "model", "x")
	if !errors.Is(err, wantErr) {
		t.Fatalf("Set: got error %v, want %v", err, wantErr)
	}
	if gotSet.ConversationID != "conversa-2" {
		t.Fatalf("Set conversationId = %q, want %q", gotSet.ConversationID, "conversa-2")
	}
}

func TestACPOptionsUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "acp_options.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("acp_options.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("acp_options.go deve chamar WithUser(session,")
	}
}
