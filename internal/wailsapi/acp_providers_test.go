package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestACPProvidersNotWired(t *testing.T) {
	t.Parallel()
	api := NewACPProviders()
	if _, err := api.TestACPAgent("cmd", nil); !errors.Is(err, ErrACPProvidersNotWired) {
		t.Fatalf("TestACPAgent: got %v", err)
	}
	if _, err := api.DetectACPAgent("cursor"); !errors.Is(err, ErrACPProvidersNotWired) {
		t.Fatalf("DetectACPAgent: got %v", err)
	}
}

func TestACPProvidersUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "acp_providers.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("acp_providers.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("acp_providers.go deve chamar WithUser(session,")
	}
}
