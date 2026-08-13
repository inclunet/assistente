package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSettingsNotWired(t *testing.T) {
	t.Parallel()
	api := NewSettings()
	if _, err := api.GetNativeTTSProviders(); !errors.Is(err, ErrSettingsNotWired) {
		t.Fatalf("got %v", err)
	}
	if _, err := api.TestConnection(); !errors.Is(err, ErrSettingsNotWired) {
		t.Fatalf("got %v", err)
	}
	if err := api.ResetConfig(); !errors.Is(err, ErrSettingsNotWired) {
		t.Fatalf("got %v", err)
	}
	if err := api.ClearAllCredentials(); !errors.Is(err, ErrSettingsNotWired) {
		t.Fatalf("got %v", err)
	}
}

func TestSettingsUsesWithUserOrAdminNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "settings.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("settings.go não deve chamar requireAuthenticatedContext(; use WithUser/WithAdmin")
	}
	if strings.Contains(body, "requireAdminContext(") {
		t.Fatal("settings.go não deve chamar requireAdminContext(; use WithAdmin")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("settings.go deve chamar WithUser(")
	}
	if !strings.Contains(body, "WithAdmin(") {
		t.Fatal("settings.go deve chamar WithAdmin(")
	}
}
