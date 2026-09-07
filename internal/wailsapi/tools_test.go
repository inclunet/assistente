package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/apidto"
)

func TestToolsNotWired(t *testing.T) {
	t.Parallel()
	api := NewTools()
	if _, err := api.GetAvailableTools(); !errors.Is(err, ErrToolsNotWired) {
		t.Fatalf("GetAvailableTools: got %v", err)
	}
	if _, err := api.GetRuntimeToolCatalog(apidto.RuntimeToolCatalogFilter{}); !errors.Is(err, ErrToolsNotWired) {
		t.Fatalf("GetRuntimeToolCatalog: got %v", err)
	}
}

func TestToolsUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "tools.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("tools.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("tools.go deve chamar WithUser(")
	}
}
