package wailsapi

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNetTrustNotWired(t *testing.T) {
	t.Parallel()
	api := NewNetTrust()
	if _, err := api.GetNetworkAllowlist(); !errors.Is(err, ErrNetTrustNotWired) {
		t.Fatalf("GetNetworkAllowlist: got %v", err)
	}
	if err := api.RemoveNetworkAllowlistEntry("global", "h", ""); !errors.Is(err, ErrNetTrustNotWired) {
		t.Fatalf("RemoveNetworkAllowlistEntry: got %v", err)
	}
}

func TestNetTrustUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "nettrust.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("nettrust.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("nettrust.go deve chamar WithUser(")
	}
}
