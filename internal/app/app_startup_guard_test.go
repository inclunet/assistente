package app

import (
	"errors"
	"testing"

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
