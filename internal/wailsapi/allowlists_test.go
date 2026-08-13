package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAllowlistsNotWired(t *testing.T) {
	t.Parallel()
	api := NewAllowlists()
	if _, err := api.GetAllowlists(); !errors.Is(err, ErrAllowlistsNotWired) {
		t.Fatalf("got %v", err)
	}
}

func TestAllowlistsUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "allowlists.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("allowlists.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("allowlists.go deve chamar WithUser(")
	}
}
