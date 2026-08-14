package wailsapi

import (
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
