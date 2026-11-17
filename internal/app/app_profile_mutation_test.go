package app

import (
	"assistente/internal/workspace"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/commandexecution"
	"assistente/internal/commandruntime"
	"assistente/internal/commandsecurity"
	"assistente/internal/configdir"
	"assistente/internal/database"
	"assistente/internal/profileaccess"
	"assistente/internal/profiles"
)

func profileMutationAppFixture(t *testing.T) (*App, context.Context, *profiles.CommandMutation) {
	t.Helper()
	a, _ := appLifecycleProductMountFixture(t)
	ctx, mutation := prepareProfileMutationAppFixture(t, a)
	return a, ctx, mutation
}

func prepareProfileMutationAppFixture(t *testing.T, a *App) (context.Context, *profiles.CommandMutation) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("USERPROFILE", root)
	t.Chdir(root)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)
	a.profileManager = profiles.NewManager()
	a.profileAccessOnce.Do(func() {
		a.profileAccess = profileaccess.NewService(a.profileManager, nil, nil, nil)
	})
	profile := profiles.DefaultProfile()
	profile.Name, profile.Active = "Coordinated profile", false
	fingerprint, err := a.profileManager.CommandMutationSnapshot("")
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := a.profileManager.PrepareCommandMutation(profiles.CommandMutationCreate, "", profile, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	return database.WithUserID(context.Background(), a.currentUserID), mutation
}

func TestProfileMutationAppRebuildsMountedMapWithoutRepeatingWrite(t *testing.T) {
	for _, publicationFails := range []bool{false, true} {
		name := "success"
		if publicationFails {
			name = "publication-failure"
		}
		t.Run(name, func(t *testing.T) {
			a := readyCommandProduct(t)
			ctx, mutation := prepareProfileMutationAppFixture(t, a)
			before, err := CommandLifecycleSnapshot(a)
			if err != nil || before.State != commandruntime.StateReady {
				t.Fatalf("fixture not ready: %+v %v", before, err)
			}
			calls := 0
			_, err = a.commitProfileMutation(ctx, mutation, func(string) error {
				calls++
				if publicationFails {
					return errors.New("failed publication")
				}
				return nil
			})
			if publicationFails != errors.Is(err, profiles.ErrCommandMutationOutcomeUnknown) || (!publicationFails && err != nil) {
				t.Fatalf("unexpected outcome: %v", err)
			}
			after, snapshotErr := CommandLifecycleSnapshot(a)
			if snapshotErr != nil {
				t.Fatal(snapshotErr)
			}
			if publicationFails {
				if after.Published || after.State == commandruntime.StateReady {
					t.Fatalf("uncertain publication reopened map: %+v", after)
				}
			} else if !after.Published || after.State != commandruntime.StateReady {
				t.Fatalf("confirmed change left map unavailable: %+v", after)
			}
			if calls != 1 {
				t.Fatalf("publication repeated: %d", calls)
			}
			if _, err := a.profileManager.Get("coordinated-profile"); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestProfileMutationAppInvalidatesBeforePublication(t *testing.T) {
	a, ctx, mutation := profileMutationAppFixture(t)
	epochs, _ := a.commandSecurityService()
	principal, _ := a.currentCommandPrincipal()
	before, err := epochs.Capture(ctx, principal.UserID, principal.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	published := false
	slug, err := a.commitProfileMutation(ctx, mutation, func(slug string) error {
		published = true
		if _, err := a.profileManager.Get(slug); err != nil {
			t.Fatal("publication before filesystem commit", err)
		}
		if _, err := epochs.Capture(ctx, principal.UserID, principal.SessionID); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
			t.Fatalf("admission open during publication: %v", err)
		}
		if _, _, err := a.commandHost.UserConfiguration(ctx, principal.UserID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
			t.Fatalf("old map survived: %v", err)
		}
		return nil
	})
	if err != nil || !published || slug == "" {
		t.Fatalf("commit=%q published=%v err=%v", slug, published, err)
	}
	if err := epochs.Admit(ctx, before, func(context.Context) error { return nil }, func() error { t.Fatal("old epoch admitted"); return nil }); !errors.Is(err, commandsecurity.ErrStaleEpoch) {
		t.Fatal(err)
	}
	if _, err := epochs.Capture(ctx, principal.UserID, principal.SessionID); err != nil {
		t.Fatal("transition leaked", err)
	}
	if _, err := a.commitProfileMutation(ctx, mutation, func(string) error { t.Fatal("replayed publication"); return nil }); !errors.Is(err, profiles.ErrConsumedCommandMutation) {
		t.Fatalf("replay: %v", err)
	}
}

func TestProfileMutationAppRefusesOtherOwnerAndCancelledContext(t *testing.T) {
	for _, mode := range []string{"other-owner", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			a, ctx, mutation := profileMutationAppFixture(t)
			if mode == "other-owner" {
				ctx = database.WithUserID(ctx, "other-owner")
			} else {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if _, err := a.commitProfileMutation(ctx, mutation, func(string) error { t.Fatal("unexpected publication"); return nil }); err == nil {
				t.Fatal("accepted invalid caller")
			}
			if _, err := a.profileManager.Get("coordinated-profile"); err == nil {
				t.Fatal("invalid caller wrote profile")
			}
		})
	}
}

func TestProfileMutationAppPublicationFailureKeepsCommittedFile(t *testing.T) {
	a, ctx, mutation := profileMutationAppFixture(t)
	_, err := a.commitProfileMutation(ctx, mutation, func(string) error { return errors.New("publication failed") })
	if !errors.Is(err, profiles.ErrCommandMutationOutcomeUnknown) {
		t.Fatalf("lost outcome classification: %v", err)
	}
	if _, err := a.profileManager.Get("coordinated-profile"); err != nil {
		t.Fatal("successful file commit was undone", err)
	}
	if _, _, err := a.commandHost.UserConfiguration(ctx, a.currentUserID); !errors.Is(err, commandexecution.ErrHostUserNotPublished) {
		t.Fatalf("failure restored old map: %v", err)
	}
}

func TestDefaultProfileSyncThroughWiringAndActivation(t *testing.T) {
	a, ctx, create := profileMutationAppFixture(t)
	if a.workspaceMgr == nil {
		t.Fatal("fixture sem workspace")
	}
	var published []*workspace.Workspace
	a.emitter = commandOSBootstrapEmitter(func(name string, data any) {
		if name == "workspace:default_profile_changed" {
			published = append(published, data.(*workspace.Workspace))
		}
	})
	a.wireProfiles()
	initial := a.workspaceMgr.Active()
	if initial.DefaultProfile == "" || initial.DefaultProfile != a.profileManager.GetActiveSlug() || len(published) != 1 {
		t.Fatalf("bootstrap não publicou padrão: snapshot=%+v eventos=%d", initial, len(published))
	}
	tab := initial.Tabs.Active
	if err := a.workspaceMgr.UpdateTab(tab, map[string]any{"profile_override": map[string]any{"slug": "conversation-profile"}}); err != nil {
		t.Fatal(err)
	}
	slug, err := a.commitProfileMutation(ctx, create, func(string) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := a.profileManager.CommandMutationSnapshot(slug)
	if err != nil {
		t.Fatal(err)
	}
	activation, err := a.profileManager.PrepareCommandMutation(profiles.CommandMutationActivate, slug, nil, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.commitProfileMutation(ctx, activation, func(string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	active := a.workspaceMgr.Active()
	if active.DefaultProfile != slug || active.ActiveTab().ProfileOverride["slug"] != "conversation-profile" {
		t.Fatalf("ativação não sincronizou padrão ou apagou override: %+v", active)
	}
	if len(published) != 2 || published[1].DefaultProfile != slug {
		t.Fatalf("eventos inesperados: %+v", published)
	}
	a.syncWorkspaceDefaultProfile()
	if len(published) != 2 {
		t.Fatal("sincronização sem mudança repetiu evento")
	}
}

func TestDefaultProfileSyncKeepsProjectionOnReadFailure(t *testing.T) {
	a, _, _ := profileMutationAppFixture(t)
	a.wireProfiles()
	initial := a.workspaceMgr.Active().DefaultProfile
	journal := filepath.Join(configdir.NewResolver("profiles").GetHomeDir(), ".profile-mutation.journal")
	if err := os.MkdirAll(filepath.Dir(journal), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journal, []byte("invalid journal"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := a.profileManager.ReadActiveCommandTarget(); err == nil {
		t.Fatal("fixture não provocou erro de leitura")
	}
	a.emitter = commandOSBootstrapEmitter(func(name string, _ any) {
		if name == "workspace:default_profile_changed" {
			t.Error("falha de leitura publicou mudança")
		}
	})
	a.syncWorkspaceDefaultProfile()
	if a.workspaceMgr.Active().DefaultProfile != initial {
		t.Fatal("falha de leitura apagou projeção válida")
	}
}
