package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestTokensNotWired(t *testing.T) {
	t.Parallel()
	tok := NewTokens()
	if _, err := tok.GetConversationTokenStats("c1"); !errors.Is(err, ErrTokensNotWired) {
		t.Fatalf("got %v", err)
	}
}

func TestTokensUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "tokens.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("tokens.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("tokens.go deve chamar WithUser(")
	}
}
