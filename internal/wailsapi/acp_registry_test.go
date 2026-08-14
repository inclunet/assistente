package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestACPRegistryNotWired(t *testing.T) {
	t.Parallel()
	api := NewACPRegistry()
	if _, err := api.GetACPCatalog(); !errors.Is(err, ErrACPRegistryNotWired) {
		t.Fatalf("GetACPCatalog: got %v", err)
	}
	if _, err := api.RefreshACPCatalog(); !errors.Is(err, ErrACPRegistryNotWired) {
		t.Fatalf("RefreshACPCatalog: got %v", err)
	}
}

func TestACPRegistryUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "acp_registry.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("acp_registry.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("acp_registry.go deve chamar WithUser(session,")
	}
}
