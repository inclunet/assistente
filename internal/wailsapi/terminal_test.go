package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTerminalNotWired(t *testing.T) {
	t.Parallel()
	api := NewTerminal()
	if _, err := api.ListTerminalSessions(); !errors.Is(err, ErrTerminalNotWired) {
		t.Fatalf("ListTerminalSessions: got %v", err)
	}
	if _, err := api.CreateTerminalSession("x"); !errors.Is(err, ErrTerminalNotWired) {
		t.Fatalf("CreateTerminalSession: got %v", err)
	}
	if err := api.CloseTerminalSession("s1"); !errors.Is(err, ErrTerminalNotWired) {
		t.Fatalf("CloseTerminalSession: got %v", err)
	}
	if _, err := api.GetTerminalHistory("s1"); !errors.Is(err, ErrTerminalNotWired) {
		t.Fatalf("GetTerminalHistory: got %v", err)
	}
	if err := api.RunTerminalCommand("s1", "ls"); !errors.Is(err, ErrTerminalNotWired) {
		t.Fatalf("RunTerminalCommand: got %v", err)
	}
	if err := api.SendTerminalInput("s1", "ls\n"); !errors.Is(err, ErrTerminalNotWired) {
		t.Fatalf("SendTerminalInput: got %v", err)
	}
	if err := api.InterruptTerminalCommand("s1"); !errors.Is(err, ErrTerminalNotWired) {
		t.Fatalf("InterruptTerminalCommand: got %v", err)
	}
	if _, err := api.GetTerminalStats(); !errors.Is(err, ErrTerminalNotWired) {
		t.Fatalf("GetTerminalStats: got %v", err)
	}
}

func TestTerminalUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "terminal.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("terminal.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("terminal.go deve chamar WithUser(")
	}
}
