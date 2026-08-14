package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestWelcomeNotWired(t *testing.T) {
	t.Parallel()
	api := NewWelcome()
	if !api.NeedsWelcomeWizard() {
		t.Fatal("NeedsWelcomeWizard com bind vazio deve ser fail-safe true")
	}
	if _, err := api.RunWelcomeWizard(); !errors.Is(err, ErrWelcomeNotWired) {
		t.Fatalf("RunWelcomeWizard: got %v, want ErrWelcomeNotWired", err)
	}
}

func TestWelcomeDoesNotCallRequireAuthenticatedContext(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "welcome.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("welcome.go não deve chamar requireAuthenticatedContext(; use Session.AuthenticatedContext no pós-login")
	}
	// Dual-mode: NeedsWelcomeWizard não força WithUser (pré-login sem sessão).
	if strings.Contains(body, "WithUser(") {
		t.Fatal("welcome.go não deve chamar WithUser(; NeedsWelcomeWizard é dual-mode e RunWelcomeWizard usa AppContext")
	}
}
