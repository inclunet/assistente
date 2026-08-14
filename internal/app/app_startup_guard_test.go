package app

import (
	"strings"
	"testing"

	"assistente/internal/wailsapi"
)

func TestWorkspaceBindingsAreSafeBeforeStartup(t *testing.T) {
	a := &App{}

	if got := a.GetActiveWorkspace(); got != nil {
		t.Fatalf("GetActiveWorkspace() = %+v, want nil before startup", got)
	}

	if _, err := a.ListWorkspaces(); err == nil || !strings.Contains(err.Error(), "workspace controller not initialized") {
		t.Fatalf("ListWorkspaces() error = %v, want initialization error", err)
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

	if _, err := api.RunWelcomeWizard(); err == nil || !strings.Contains(err.Error(), "welcome bind not wired") {
		t.Fatalf("RunWelcomeWizard() error = %v, want not wired", err)
	}
}
