package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/config"
)

func TestDatabaseNotWired(t *testing.T) {
	t.Parallel()
	api := NewDatabase()
	if err := api.ResetDatabase(); !errors.Is(err, ErrDatabaseNotWired) {
		t.Fatalf("ResetDatabase: got %v", err)
	}
	if err := api.ClearMessages(); !errors.Is(err, ErrDatabaseNotWired) {
		t.Fatalf("ClearMessages: got %v", err)
	}
	if _, err := api.GetMaintenanceSettings(); !errors.Is(err, ErrDatabaseNotWired) {
		t.Fatalf("GetMaintenanceSettings: got %v", err)
	}
	if err := api.SaveMaintenanceSettings(config.MaintenanceSettings{}); !errors.Is(err, ErrDatabaseNotWired) {
		t.Fatalf("SaveMaintenanceSettings: got %v", err)
	}
	if _, err := api.GetDatabaseStats(); !errors.Is(err, ErrDatabaseNotWired) {
		t.Fatalf("GetDatabaseStats: got %v", err)
	}
	if _, err := api.RunDatabaseMaintenance(false); !errors.Is(err, ErrDatabaseNotWired) {
		t.Fatalf("RunDatabaseMaintenance: got %v", err)
	}
}

func TestDatabaseUsesWithUserOrAdminNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "database.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("database.go não deve chamar requireAuthenticatedContext(; use WithUser/WithAdmin")
	}
	if strings.Contains(body, "requireAdminContext(") {
		t.Fatal("database.go não deve chamar requireAdminContext(; use WithAdmin")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("database.go deve chamar WithUser(")
	}
	if !strings.Contains(body, "WithAdmin(") {
		t.Fatal("database.go deve chamar WithAdmin(")
	}
}
