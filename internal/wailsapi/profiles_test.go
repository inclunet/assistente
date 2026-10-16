package wailsapi

import (
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/profiles"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/controllers"
)

func setupWailsProfilesTest(t *testing.T) *profiles.Manager {
	t.Helper()
	tempDir := t.TempDir()
	oldHome := os.Getenv("HOME")
	oldUserProfile := os.Getenv("USERPROFILE")
	oldCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("HOME", tempDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Setenv("USERPROFILE", tempDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(tempDir); err != nil {
		t.Fatal(err)
	}
	configdir.ResetForTests()
	t.Cleanup(func() {
		_ = os.Chdir(oldCwd)
		_ = os.Setenv("HOME", oldHome)
		_ = os.Setenv("USERPROFILE", oldUserProfile)
		configdir.ResetForTests()
	})
	return profiles.NewManager()
}

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

func TestProfilesWailsPassesAuthenticatedContextToCreateAndMediaMutation(t *testing.T) {
	manager := setupWailsProfilesTest(t)
	if _, err := manager.Create(profileForWailsTest("Ativo", true)); err != nil {
		t.Fatal(err)
	}

	userCtx := database.WithUserID(context.Background(), "profile-owner")
	var received []context.Context
	ctrl := controllers.NewProfilesController(controllers.ProfilesControllerConfig{
		ProfileMgr: manager,
		CommitProfileMutation: func(ctx context.Context, mutation *profiles.CommandMutation, publish func(string) error) (string, error) {
			received = append(received, ctx)
			result, err := mutation.CommitCoordinated(nil, nil)
			if err != nil {
				return "", err
			}
			if err := publish(result); err != nil {
				return "", err
			}
			return result, nil
		},
	})
	api := NewProfiles()
	AttachProfiles(api, stubSession{ctx: userCtx}, ctrl)

	if _, err := api.CreateProfile(*profileForWailsTest("Criado", false)); err != nil {
		t.Fatal(err)
	}
	if err := api.UpdateProfileMediaSupport("audio", false); err != nil {
		t.Fatal(err)
	}
	if len(received) != 2 {
		t.Fatalf("contextos recebidos = %d, want 2", len(received))
	}
	for i, ctx := range received {
		userID, ok := database.UserIDFromContext(ctx)
		if !ok || userID != "profile-owner" {
			t.Fatalf("contexto %d sem user id autenticado: %q, %v", i, userID, ok)
		}
	}
}

func TestProfilesWailsPropagatesMediaMutationError(t *testing.T) {
	manager := setupWailsProfilesTest(t)
	if _, err := manager.Create(profileForWailsTest("Ativo", true)); err != nil {
		t.Fatal(err)
	}
	want := errors.New("falha do coordenador")
	ctrl := controllers.NewProfilesController(controllers.ProfilesControllerConfig{
		ProfileMgr: manager,
		CommitProfileMutation: func(context.Context, *profiles.CommandMutation, func(string) error) (string, error) {
			return "", want
		},
	})
	api := NewProfiles()
	AttachProfiles(api, stubSession{ctx: database.WithUserID(context.Background(), "profile-owner")}, ctrl)

	if err := api.UpdateProfileMediaSupport("audio", false); !errors.Is(err, want) {
		t.Fatalf("erro = %v, want %v", err, want)
	}
}

func profileForWailsTest(name string, active bool) *profiles.Profile {
	profile := profiles.DefaultProfile()
	profile.Name = name
	profile.Active = active
	return profile
}
