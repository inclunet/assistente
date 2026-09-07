package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSubagentNotWired(t *testing.T) {
	t.Parallel()
	api := NewSubagent()
	if _, err := api.ListSubAgentRuns(10); !errors.Is(err, ErrSubagentNotWired) {
		t.Fatalf("ListSubAgentRuns: got %v", err)
	}
	if _, err := api.CancelSubAgentRun("conv", "run"); !errors.Is(err, ErrSubagentNotWired) {
		t.Fatalf("CancelSubAgentRun: got %v", err)
	}
}

func TestSubagentUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "subagent.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("subagent.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("subagent.go deve chamar WithUser(session,")
	}
}
