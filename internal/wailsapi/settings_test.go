package wailsapi

import (
	"assistente/internal/logging"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestLogSettingsExigeAdminParaSalvar(t *testing.T) {
	manager, err := logging.NewManager(filepath.Join(t.TempDir(), "logging.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := manager.Close(); err != nil {
			t.Error(err)
		}
	}()
	denied := errors.New("admin necessário")
	api := NewSettings()
	api.session = stubSession{ctx: context.Background(), adminErr: denied}
	AttachLogManager(api, manager)
	if _, err := api.GetLogSettings(); err != nil {
		t.Fatal(err)
	}
	settings := logging.Settings{MaxMiB: 50, Backups: 5}
	if err := api.SaveLogSettings(settings); !errors.Is(err, denied) {
		t.Fatalf("erro = %v", err)
	}
}

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
