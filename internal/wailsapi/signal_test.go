package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestSignalNotWired(t *testing.T) {
	t.Parallel()
	api := NewSignal()
	if err := api.SignalRegister("http://x", "+1", "sms", "", ""); !errors.Is(err, ErrSignalNotWired) {
		t.Fatalf("SignalRegister: got %v", err)
	}
	if err := api.SignalVerify("http://x", "+1", "123", ""); !errors.Is(err, ErrSignalNotWired) {
		t.Fatalf("SignalVerify: got %v", err)
	}
	if _, err := api.SignalLink("http://x", "device", ""); !errors.Is(err, ErrSignalNotWired) {
		t.Fatalf("SignalLink: got %v", err)
	}
	if _, err := api.SignalLinkRaw("http://x", "device", ""); !errors.Is(err, ErrSignalNotWired) {
		t.Fatalf("SignalLinkRaw: got %v", err)
	}
	if err := api.SignalUnregister("http://x", "+1", true, ""); !errors.Is(err, ErrSignalNotWired) {
		t.Fatalf("SignalUnregister: got %v", err)
	}
	if _, err := api.SignalCheckAPI("http://x", ""); !errors.Is(err, ErrSignalNotWired) {
		t.Fatalf("SignalCheckAPI: got %v", err)
	}
	if _, err := api.SignalListAccounts("http://x", ""); !errors.Is(err, ErrSignalNotWired) {
		t.Fatalf("SignalListAccounts: got %v", err)
	}
}

func TestSignalUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "signal.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("signal.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("signal.go deve chamar WithUser(")
	}
}
