package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/apidto"
)

func TestCredentialsNotWired(t *testing.T) {
	t.Parallel()
	api := NewCredentials()
	if _, err := api.ListCredentials(); !errors.Is(err, ErrCredentialsNotWired) {
		t.Fatalf("ListCredentials: got %v", err)
	}
	if err := api.UpsertCredential(apidto.CredentialInput{}); !errors.Is(err, ErrCredentialsNotWired) {
		t.Fatalf("UpsertCredential: got %v", err)
	}
	if err := api.DeleteCredential("x"); !errors.Is(err, ErrCredentialsNotWired) {
		t.Fatalf("DeleteCredential: got %v", err)
	}
	if _, err := api.ListExternalSources("env://"); !errors.Is(err, ErrCredentialsNotWired) {
		t.Fatalf("ListExternalSources: got %v", err)
	}
}

func TestCredentialsUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "credentials.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("credentials.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("credentials.go deve chamar WithUser(")
	}
}
