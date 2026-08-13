package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMCPNotWired(t *testing.T) {
	t.Parallel()
	api := NewMCP()
	if _, err := api.ListMCPServers(); !errors.Is(err, ErrMCPNotWired) {
		t.Fatalf("ListMCPServers: got %v", err)
	}
	if err := api.ConnectMCPServer("x"); !errors.Is(err, ErrMCPNotWired) {
		t.Fatalf("ConnectMCPServer: got %v", err)
	}
	if _, err := api.GetMCPServerAuthInfo("x"); !errors.Is(err, ErrMCPNotWired) {
		t.Fatalf("GetMCPServerAuthInfo: got %v", err)
	}
	if _, err := api.DiscoverMCPServerAuth("https://example.com"); !errors.Is(err, ErrMCPNotWired) {
		t.Fatalf("DiscoverMCPServerAuth: got %v", err)
	}
}

func TestMCPUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "mcp.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("mcp.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("mcp.go deve chamar WithUser(")
	}
}
