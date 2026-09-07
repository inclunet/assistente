package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/controllers"
)

func TestProfilesNotWired(t *testing.T) {
	t.Parallel()
	api := NewProfiles()
	if _, err := api.GetProfiles(); !errors.Is(err, ErrProfilesNotWired) {
		t.Fatalf("GetProfiles: got %v", err)
	}
	if err := api.UpdateProfileMediaSupport("audio", false); !errors.Is(err, ErrProfilesNotWired) {
		t.Fatalf("UpdateProfileMediaSupport: got %v", err)
	}
}

func TestUpdateProfileMediaSupportUnknownTypeNoOp(t *testing.T) {
	t.Parallel()
	api := NewProfiles()
	AttachProfiles(api, stubSession{ctx: context.Background()}, &controllers.ProfilesController{})
	if err := api.UpdateProfileMediaSupport("unknown-type", true); err != nil {
		t.Fatalf("tipo desconhecido deve ser no-op após auth, got %v", err)
	}
}

func TestProfilesUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "profiles.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("profiles.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(") {
		t.Fatal("profiles.go deve chamar WithUser(")
	}
}
