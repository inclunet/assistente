package app

import (
	"errors"
	"testing"

	"assistente/internal/portability"
	"assistente/internal/wailsapi"
)

func TestWorkspaceBindingsAreSafeBeforeStartup(t *testing.T) {
	api := wailsapi.NewWorkspace()

	if _, err := api.GetActiveWorkspace(); !errors.Is(err, wailsapi.ErrWorkspaceNotWired) {
		t.Fatalf("GetActiveWorkspace() error = %v, want ErrWorkspaceNotWired", err)
	}
	if _, err := api.ListWorkspaces(); !errors.Is(err, wailsapi.ErrWorkspaceNotWired) {
		t.Fatalf("ListWorkspaces() error = %v, want ErrWorkspaceNotWired", err)
	}
}

func TestMessagingBindingsAreSafeBeforeStartup(t *testing.T) {
	api := wailsapi.NewMessaging()

	if _, err := api.GetMessagingStatus(); !errors.Is(err, wailsapi.ErrMessagingNotWired) {
		t.Fatalf("GetMessagingStatus() error = %v, want ErrMessagingNotWired", err)
	}
	if _, err := api.GetAllChannelConfigs(); !errors.Is(err, wailsapi.ErrMessagingNotWired) {
		t.Fatalf("GetAllChannelConfigs() error = %v, want ErrMessagingNotWired", err)
	}
}

func TestEditorBindingsAreSafeBeforeStartup(t *testing.T) {
	api := wailsapi.NewEditor()

	if _, err := api.EditorLoadState(); !errors.Is(err, wailsapi.ErrEditorNotWired) {
		t.Fatalf("EditorLoadState() error = %v, want ErrEditorNotWired", err)
	}
	if err := api.EditorWriteFile("x", ""); !errors.Is(err, wailsapi.ErrEditorNotWired) {
		t.Fatalf("EditorWriteFile() error = %v, want ErrEditorNotWired", err)
	}
}

func TestExportImportBindingsAreSafeBeforeStartup(t *testing.T) {
	api := wailsapi.NewExportImport()

	if _, err := api.ExportData(portability.ExportRequest{}); !errors.Is(err, wailsapi.ErrExportImportNotWired) {
		t.Fatalf("ExportData() error = %v, want ErrExportImportNotWired", err)
	}
	if _, err := api.ImportData("", ""); !errors.Is(err, wailsapi.ErrExportImportNotWired) {
		t.Fatalf("ImportData() error = %v, want ErrExportImportNotWired", err)
	}
}

func TestWelcomeBindingsAreSafeBeforeStartup(t *testing.T) {
	a := &App{}
	api := wailsapi.NewWelcome()

	if !api.NeedsWelcomeWizard() {
		t.Fatal("NeedsWelcomeWizard() = false before startup, want conservative true")
	}
	if !NeedsWelcomeWizard(a) {
		t.Fatal("NeedsWelcomeWizard(a) = false before startup, want conservative true")
	}

	result := a.validateWizardConnection("https://example.com", "")
	if result.ErrorType != "app_initializing" {
		t.Fatalf("validateWizardConnection() ErrorType = %q, want app_initializing", result.ErrorType)
	}

	if _, err := api.RunWelcomeWizard(); !errors.Is(err, wailsapi.ErrWelcomeNotWired) {
		t.Fatalf("RunWelcomeWizard() error = %v, want ErrWelcomeNotWired", err)
	}
}
