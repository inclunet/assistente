package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHotkeysNotWired(t *testing.T) {
	t.Parallel()
	api := NewHotkeys()
	if _, err := api.IsGlobalHotkeySupported(); !errors.Is(err, ErrHotkeysNotWired) {
		t.Fatalf("got %v", err)
	}
}

func TestHotkeysUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "hotkeys.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("hotkeys.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("hotkeys.go deve chamar WithUser(")
	}
}
