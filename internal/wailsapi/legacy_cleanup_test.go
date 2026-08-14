package wailsapi

import (
	"assistente/internal/apidto"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLegacyCleanupNotWired(t *testing.T) {
	t.Parallel()
	api := NewLegacyCleanup()
	if _, err := api.CleanupLegacyChannelJSON(apidto.CleanupLegacyChannelJSONOptions{}); !errors.Is(err, ErrLegacyCleanupNotWired) {
		t.Fatalf("got %v", err)
	}
}

func TestLegacyCleanupUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "legacy_cleanup.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("legacy_cleanup.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("legacy_cleanup.go deve chamar WithUser(")
	}
}
