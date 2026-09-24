package controllers

import (
	"assistente/internal/configdir"
	"assistente/internal/profiles"
	"context"
	"errors"
	"os"
	"testing"
)

type profilesControllerEvent struct {
	name    string
	payload any
}

type profilesControllerEmitter struct {
	events []profilesControllerEvent
}

func (e *profilesControllerEmitter) Emit(name string, payload any) {
	e.events = append(e.events, profilesControllerEvent{name: name, payload: payload})
}

type profilesMutationCommitter struct {
	calls        int
	publishCalls int
	ctx          context.Context
	err          error
}

func (c *profilesMutationCommitter) commit(ctx context.Context, mutation *profiles.CommandMutation, publish func(string) error) (string, error) {
	c.calls++
	c.ctx = ctx
	if c.err != nil {
		return "", c.err
	}
	result, err := mutation.CommitCoordinated(nil, nil)
	if err != nil {
		return "", err
	}
	c.publishCalls++
	if err := publish(result); err != nil {
		return "", err
	}
	return result, nil
}

func setupProfilesControllerTest(t *testing.T) *profiles.Manager {
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

func controllerProfile(name string, active bool) *profiles.Profile {
	profile := profiles.DefaultProfile()
	profile.Name = name
	profile.Active = active
	return profile
}

func newCoordinatedProfilesController(manager *profiles.Manager, emitter *profilesControllerEmitter, committer *profilesMutationCommitter, changed *[]string) *ProfilesController {
	return NewProfilesController(ProfilesControllerConfig{
		ProfileMgr: manager,
		Emitter:    emitter,
		OnProfileChanged: func(slug string) {
			*changed = append(*changed, slug)
		},
		CommitProfileMutation: committer.commit,
	})
}

func TestProfilesControllerContextMutationsUseConfiguredCoordinator(t *testing.T) {
	cases := []struct {
		name      string
		wantEvent string
		run       func(*ProfilesController, context.Context, *profiles.Manager) error
	}{
		{
			name:      "create",
			wantEvent: "profile:created",
			run: func(controller *ProfilesController, ctx context.Context, _ *profiles.Manager) error {
				_, err := controller.CreateProfileContext(ctx, *controllerProfile("Criado", false))
				return err
			},
		},
		{
			name:      "update",
			wantEvent: "profile:updated",
			run: func(controller *ProfilesController, ctx context.Context, manager *profiles.Manager) error {
				target, err := manager.Create(controllerProfile("Atualizado", false))
				if err != nil {
					return err
				}
				profile := controllerProfile("Atualizado 2", false)
				return controller.UpdateProfileContext(ctx, target, *profile)
			},
		},
		{
			name:      "duplicate",
			wantEvent: "profile:created",
			run: func(controller *ProfilesController, ctx context.Context, manager *profiles.Manager) error {
				source, err := manager.Create(controllerProfile("Origem", true))
				if err != nil {
					return err
				}
				_, err = controller.DuplicateProfileContext(ctx, source)
				return err
			},
		},
		{
			name:      "delete",
			wantEvent: "profile:deleted",
			run: func(controller *ProfilesController, ctx context.Context, manager *profiles.Manager) error {
				if _, err := manager.Create(controllerProfile("Ativo", true)); err != nil {
					return err
				}
				target, err := manager.Create(controllerProfile("Remover", false))
				if err != nil {
					return err
				}
				return controller.DeleteProfileContext(ctx, target)
			},
		},
		{
			name:      "activate",
			wantEvent: "profile:changed",
			run: func(controller *ProfilesController, ctx context.Context, manager *profiles.Manager) error {
				if _, err := manager.Create(controllerProfile("Ativo", true)); err != nil {
					return err
				}
				target, err := manager.Create(controllerProfile("Ativar", false))
				if err != nil {
					return err
				}
				return controller.SetActiveProfileContext(ctx, target)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			manager := setupProfilesControllerTest(t)
			emitter := &profilesControllerEmitter{}
			committer := &profilesMutationCommitter{}
			var changed []string
			controller := newCoordinatedProfilesController(manager, emitter, committer, &changed)
			type authenticatedTestKey struct{}
			ctx := context.WithValue(context.Background(), authenticatedTestKey{}, "authenticated")

			if err := tc.run(controller, ctx, manager); err != nil {
				t.Fatal(err)
			}
			if committer.calls != 1 || committer.publishCalls != 1 {
				t.Fatalf("coordinator calls = %d, publish calls = %d", committer.calls, committer.publishCalls)
			}
			if committer.ctx != ctx {
				t.Fatal("contexto autenticado não chegou ao coordenador")
			}
			if len(emitter.events) != 1 {
				t.Fatalf("eventos = %d, want 1", len(emitter.events))
			}
			if emitter.events[0].name != tc.wantEvent {
				t.Fatalf("evento = %q, want %q", emitter.events[0].name, tc.wantEvent)
			}
		})
	}
}

func TestProfilesControllerPrepareRejectsStaleFingerprintBeforeCommit(t *testing.T) {
	manager := setupProfilesControllerTest(t)
	target, err := manager.Create(controllerProfile("Original", false))
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := manager.CommandMutationSnapshot(target)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Update(target, controllerProfile("Alterado antes do commit", false)); err != nil {
		t.Fatal(err)
	}

	emitter := &profilesControllerEmitter{}
	committer := &profilesMutationCommitter{}
	var changed []string
	controller := newCoordinatedProfilesController(manager, emitter, committer, &changed)
	_, err = controller.PrepareCommandMutation(
		profiles.CommandMutationUpdate,
		target,
		controllerProfile("Payload obsoleto", false),
		fingerprint,
	)
	if !errors.Is(err, profiles.ErrStaleCommandMutation) {
		t.Fatalf("erro = %v, want stale mutation", err)
	}
	if committer.calls != 0 || len(emitter.events) != 0 || len(changed) != 0 {
		t.Fatalf("efeitos após stale: commits=%d eventos=%d callbacks=%d", committer.calls, len(emitter.events), len(changed))
	}
	current, err := manager.Get(target)
	if err != nil {
		t.Fatal(err)
	}
	if current.Name != "Alterado antes do commit" {
		t.Fatalf("perfil atual = %q, payload obsoleto foi aplicado", current.Name)
	}
}

func TestProfilesControllerPrepareFreezesPayloadUntilCommit(t *testing.T) {
	manager := setupProfilesControllerTest(t)
	fingerprint, err := manager.CommandMutationSnapshot("")
	if err != nil {
		t.Fatal(err)
	}
	emitter := &profilesControllerEmitter{}
	committer := &profilesMutationCommitter{}
	var changed []string
	controller := newCoordinatedProfilesController(manager, emitter, committer, &changed)

	enabled := true
	payload := controllerProfile("Payload congelado", false)
	payload.ContextProviders = map[string]profiles.ContextProviderProfileConfig{
		"docs": {Enabled: &enabled, Settings: map[string]any{"scope": "original"}},
	}
	prepared, err := controller.PrepareCommandMutation(profiles.CommandMutationCreate, "", payload, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	payload.Name = "Payload mutado depois da preparação"
	payload.ContextProviders["docs"] = profiles.ContextProviderProfileConfig{Settings: map[string]any{"scope": "mutado"}}

	slug, err := committer.commit(context.Background(), prepared.Mutation, prepared.Publish)
	if err != nil {
		t.Fatal(err)
	}
	created, err := manager.Get(slug)
	if err != nil {
		t.Fatal(err)
	}
	if created.Name != "Payload congelado" {
		t.Fatalf("nome persistido = %q, want payload congelado", created.Name)
	}
	if got := created.ContextProviders["docs"].Settings["scope"]; got != "original" {
		t.Fatalf("context provider persistido = %#v, want original", got)
	}
	if len(emitter.events) != 1 || emitter.events[0].name != "profile:created" {
		t.Fatalf("eventos = %#v", emitter.events)
	}
	if got := emitter.events[0].payload.(map[string]interface{})["name"]; got != "Payload congelado" {
		t.Fatalf("nome publicado = %#v, want resultado persistido", got)
	}
}

func TestProfilesControllerPublishUsesActualResultAndReadonlyCallback(t *testing.T) {
	manager := setupProfilesControllerTest(t)
	emitter := &profilesControllerEmitter{}
	committer := &profilesMutationCommitter{}
	var callbackSlug string
	var callbackName string
	var callbackErr error
	controller := NewProfilesController(ProfilesControllerConfig{
		ProfileMgr:            manager,
		Emitter:               emitter,
		CommitProfileMutation: committer.commit,
		OnProfileChanged: func(slug string) {
			callbackSlug = slug
			var profile *profiles.Profile
			profile, callbackErr = manager.Get(slug)
			if profile != nil {
				callbackName = profile.Name
			}
		},
	})

	resultSlug, err := controller.CreateProfileContext(context.Background(), *controllerProfile("Resultado real", true))
	if err != nil {
		t.Fatal(err)
	}
	if callbackErr != nil {
		t.Fatalf("callback somente-leitura falhou: %v", callbackErr)
	}
	if callbackSlug != resultSlug {
		t.Fatalf("callback slug = %q, want resultado %q", callbackSlug, resultSlug)
	}
	if callbackName != "Resultado real" {
		t.Fatalf("callback name = %q, want resultado persistido", callbackName)
	}
	if len(emitter.events) != 1 {
		t.Fatalf("eventos = %d, want 1", len(emitter.events))
	}
	payload := emitter.events[0].payload.(map[string]interface{})
	if payload["slug"] != resultSlug || payload["name"] != "Resultado real" {
		t.Fatalf("payload = %#v, want slug/name do resultado efetivo", payload)
	}
}

func TestProfilesControllerCoordinatorErrorDoesNotPublish(t *testing.T) {
	manager := setupProfilesControllerTest(t)
	active, err := manager.Create(controllerProfile("Ativo", true))
	if err != nil {
		t.Fatal(err)
	}
	emitter := &profilesControllerEmitter{}
	committer := &profilesMutationCommitter{err: errors.New("falha do coordenador")}
	var changed []string
	controller := newCoordinatedProfilesController(manager, emitter, committer, &changed)

	profile := controllerProfile("Ativo atualizado", true)
	if err := controller.UpdateProfileContext(context.Background(), active, *profile); !errors.Is(err, committer.err) {
		t.Fatalf("erro = %v, want %v", err, committer.err)
	}
	if len(emitter.events) != 0 || len(changed) != 0 {
		t.Fatalf("publicação após erro: eventos=%d callbacks=%d", len(emitter.events), len(changed))
	}
}

func TestProfilesControllerActiveCreationPublishesRuntimeRefreshAfterCommit(t *testing.T) {
	manager := setupProfilesControllerTest(t)
	emitter := &profilesControllerEmitter{}
	committer := &profilesMutationCommitter{}
	var changed []string
	controller := newCoordinatedProfilesController(manager, emitter, committer, &changed)

	profile := controllerProfile("Novo ativo", true)
	slug, err := controller.CreateProfileContext(context.Background(), *profile)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != slug {
		t.Fatalf("callbacks = %#v, want [%q]", changed, slug)
	}
	if len(emitter.events) != 1 || emitter.events[0].name != "profile:created" {
		t.Fatalf("eventos = %#v", emitter.events)
	}
}

func TestProfilesControllerWithoutCoordinatorFailsClosed(t *testing.T) {
	manager := setupProfilesControllerTest(t)
	controller := NewProfilesController(ProfilesControllerConfig{ProfileMgr: manager})
	profile := controllerProfile("Sem coordenador", false)
	if _, err := controller.CreateProfileContext(context.Background(), *profile); err == nil {
		t.Fatal("create sem coordenador deveria falhar")
	}
}
