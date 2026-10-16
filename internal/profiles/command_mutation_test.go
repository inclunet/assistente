package profiles

import (
	"assistente/internal/configdir"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestCommandMutationReadTargetUsesOneFingerprintAndCAS(t *testing.T) {
	manager := setupProfileTestEnv(t)
	profile := DefaultProfile()
	profile.Name = "CAS"
	slug, err := manager.Create(profile)
	if err != nil {
		t.Fatal(err)
	}

	target, fingerprint, err := manager.ReadCommandTarget(slug)
	if err != nil {
		t.Fatal(err)
	}
	if target.Name != profile.Name {
		t.Fatalf("target name = %q, want %q", target.Name, profile.Name)
	}
	if snapshot, err := manager.CommandMutationSnapshot(slug); err != nil || snapshot != fingerprint {
		t.Fatalf("snapshot = %q, err = %v; want the fingerprint captured with target", snapshot, err)
	}

	target.Description = "alterado"
	mutation, err := manager.PrepareCommandMutation(CommandMutationUpdate, slug, target, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := mutation.CommitCoordinated(nil, nil); err != nil || got != slug {
		t.Fatalf("commit = %q, err = %v", got, err)
	}
	if _, err := mutation.CommitCoordinated(nil, nil); !errors.Is(err, ErrConsumedCommandMutation) {
		t.Fatalf("replay error = %v, want consumed mutation", err)
	}
	updated, err := manager.Get(slug)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Description != "alterado" {
		t.Fatalf("description = %q, want alterado", updated.Description)
	}
}

func TestReadActiveCommandTargetKeepsTargetSlugAndFingerprintTogether(t *testing.T) {
	manager := setupProfileTestEnv(t)
	profile := DefaultProfile()
	profile.Name = "Leitura ativa"
	profile.Active = true
	slug, err := manager.Create(profile)
	if err != nil {
		t.Fatal(err)
	}

	target, activeSlug, fingerprint, err := manager.ReadActiveCommandTarget()
	if err != nil {
		t.Fatal(err)
	}
	if activeSlug != slug || target == nil || target.Name != profile.Name {
		t.Fatalf("active target = %#v/%q, want profile %q/%q", target, activeSlug, profile.Name, slug)
	}
	current, err := manager.CommandMutationSnapshot(activeSlug)
	if err != nil {
		t.Fatal(err)
	}
	if current != fingerprint {
		t.Fatalf("active target fingerprint = %q, snapshot = %q", fingerprint, current)
	}
	updated := *target
	updated.Description = "atualizado pelo alvo ativo"
	mutation, err := manager.PrepareCommandMutation(CommandMutationUpdate, activeSlug, &updated, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := mutation.CommitCoordinated(nil, nil); err != nil || got != activeSlug {
		t.Fatalf("active target commit = %q, err = %v", got, err)
	}
}

func TestCommandMutationCASRunsBeforeCallback(t *testing.T) {
	manager := setupProfileTestEnv(t)
	profile := DefaultProfile()
	profile.Name = "CAS callback"
	slug, err := manager.Create(profile)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := manager.CommandMutationSnapshot(slug)
	if err != nil {
		t.Fatal(err)
	}
	prepared := *profile
	prepared.Description = "proposta"
	mutation, err := manager.PrepareCommandMutation(CommandMutationUpdate, slug, &prepared, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	changed := *profile
	changed.Description = "mudança concorrente"
	if err := manager.Update(slug, &changed); err != nil {
		t.Fatal(err)
	}

	called := false
	if _, err := mutation.CommitCoordinated(func(MutationImpact) error {
		called = true
		return nil
	}, nil); !errors.Is(err, ErrStaleCommandMutation) {
		t.Fatalf("commit error = %v, want stale mutation", err)
	}
	if called {
		t.Fatal("before callback ran before the final CAS rejected the mutation")
	}
}

func TestCommandMutationImpactActivationIncludesEveryAffectedSlug(t *testing.T) {
	manager := setupProfileTestEnv(t)
	first := DefaultProfile()
	first.Name = "Impacto ativo"
	first.Active = true
	firstSlug, err := manager.Create(first)
	if err != nil {
		t.Fatal(err)
	}
	second := DefaultProfile()
	second.Name = "Impacto novo"
	second.Active = false
	secondSlug, err := manager.Create(second)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := manager.CommandMutationSnapshot(secondSlug)
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := manager.PrepareCommandMutation(CommandMutationActivate, secondSlug, nil, fingerprint)
	if err != nil {
		t.Fatal(err)
	}

	var got MutationImpact
	if _, err := mutation.CommitCoordinated(nil, func(impact MutationImpact) error {
		got = impact
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expectedIdentity, err := profileIdentity(second)
	if err != nil {
		t.Fatal(err)
	}
	if got.Operation != CommandMutationActivate || got.ResultSlug != secondSlug {
		t.Fatalf("impact operation/result = %q/%q", got.Operation, got.ResultSlug)
	}
	expectedAffected := []string{firstSlug, secondSlug}
	sort.Strings(expectedAffected)
	if fmt.Sprint(got.AffectedSlugs) != fmt.Sprint(expectedAffected) {
		t.Fatalf("affected slugs = %#v, want %#v", got.AffectedSlugs, expectedAffected)
	}
	if got.OriginalTargetIdentity != expectedIdentity {
		t.Fatalf("original target identity = %q, want %q", got.OriginalTargetIdentity, expectedIdentity)
	}
	if got.DeletedSlug != "" {
		t.Fatalf("deleted slug = %q, want empty", got.DeletedSlug)
	}
}

func TestCommandMutationCallbacksObserveBeforeAndAfterInOrder(t *testing.T) {
	manager := setupProfileTestEnv(t)
	profile := DefaultProfile()
	profile.Name = "Ordem callbacks"
	slug, err := manager.Create(profile)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := manager.CommandMutationSnapshot(slug)
	if err != nil {
		t.Fatal(err)
	}
	updated := *profile
	updated.Description = "gravado"
	mutation, err := manager.PrepareCommandMutation(CommandMutationUpdate, slug, &updated, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(manager.resolver.GetHomeDir(), slug+".json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	var beforeImpact MutationImpact
	if _, err := mutation.CommitCoordinated(func(impact MutationImpact) error {
		order = append(order, "before")
		beforeImpact = impact
		if got, readErr := os.ReadFile(path); readErr != nil || !bytes.Equal(got, original) {
			return fmt.Errorf("before callback observed changed file: %v", readErr)
		}
		if len(impact.AffectedSlugs) > 0 {
			impact.AffectedSlugs[0] = "callback-must-not-leak"
		}
		return nil
	}, func(impact MutationImpact) error {
		order = append(order, "after")
		if len(impact.AffectedSlugs) > 0 && impact.AffectedSlugs[0] == "callback-must-not-leak" {
			return errors.New("impact slice leaked between callbacks")
		}
		var persisted Profile
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if err := json.Unmarshal(data, &persisted); err != nil {
			return err
		}
		if persisted.Description != "gravado" {
			return fmt.Errorf("after callback saw description %q", persisted.Description)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(order) != "[before after]" {
		t.Fatalf("callback order = %v, want [before after]", order)
	}
	if beforeImpact.Operation != CommandMutationUpdate {
		t.Fatalf("before impact operation = %q", beforeImpact.Operation)
	}
}

func TestCommandMutationDeleteAfterErrorRestoresAndConsumes(t *testing.T) {
	manager := setupProfileTestEnv(t)
	active := DefaultProfile()
	active.Name = "Ativo para rollback"
	active.Active = true
	if _, err := manager.Create(active); err != nil {
		t.Fatal(err)
	}
	deletable := DefaultProfile()
	deletable.Name = "Delete rollback"
	deletable.Active = false
	slug, err := manager.Create(deletable)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := manager.CommandMutationSnapshot(slug)
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := manager.PrepareCommandMutation(CommandMutationDelete, slug, nil, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(manager.resolver.GetHomeDir(), slug+".json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	afterErr := errors.New("revogação falhou")
	if _, err := mutation.CommitCoordinated(nil, func(MutationImpact) error { return afterErr }); !errors.Is(err, ErrCommandMutationRolledBack) || !errors.Is(err, afterErr) {
		t.Fatalf("delete after error = %v, want rollback and callback error", err)
	}
	restored, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(restored, original) {
		t.Fatalf("restored file err=%v equal=%v", err, bytes.Equal(restored, original))
	}
	if _, err := mutation.CommitCoordinated(nil, nil); !errors.Is(err, ErrConsumedCommandMutation) {
		t.Fatalf("replay after rollback = %v, want consumed mutation", err)
	}
}

func TestCommandMutationWriteAndAfterErrorsConsumeReplay(t *testing.T) {
	manager := setupProfileTestEnv(t)
	profile := DefaultProfile()
	profile.Name = "Replay errors"
	slug, err := manager.Create(profile)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := manager.CommandMutationSnapshot(slug)
	if err != nil {
		t.Fatal(err)
	}
	updated := *profile
	updated.Description = "write error"
	writeMutation, err := manager.PrepareCommandMutation(CommandMutationUpdate, slug, &updated, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	jJournal := filepath.Join(manager.resolver.GetHomeDir(), ".profile-mutation.journal")
	t.Cleanup(func() { _ = os.Remove(jJournal) })
	if _, err := writeMutation.CommitCoordinated(func(MutationImpact) error {
		return os.Mkdir(jJournal, 0755)
	}, nil); !errors.Is(err, ErrCommandMutationOutcomeUnknown) {
		t.Fatalf("write error = %v, want unknown outcome", err)
	}
	if _, err := writeMutation.CommitCoordinated(nil, nil); !errors.Is(err, ErrConsumedCommandMutation) {
		t.Fatalf("replay after write error = %v, want consumed mutation", err)
	}

	if err := os.Remove(jJournal); err != nil {
		t.Fatal(err)
	}
	fingerprint, err = manager.CommandMutationSnapshot(slug)
	if err != nil {
		t.Fatal(err)
	}
	updated.Description = "after error"
	afterMutation, err := manager.PrepareCommandMutation(CommandMutationUpdate, slug, &updated, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	afterErr := errors.New("coordinator failed")
	if _, err := afterMutation.CommitCoordinated(nil, func(MutationImpact) error { return afterErr }); !errors.Is(err, ErrCommandMutationOutcomeUnknown) || !errors.Is(err, afterErr) {
		t.Fatalf("after error = %v, want unknown outcome and callback error", err)
	}
	if _, err := afterMutation.CommitCoordinated(nil, nil); !errors.Is(err, ErrConsumedCommandMutation) {
		t.Fatalf("replay after callback error = %v, want consumed mutation", err)
	}
}

func TestCommandMutationActivateIsAtomicAndUnique(t *testing.T) {
	manager := setupProfileTestEnv(t)
	first := DefaultProfile()
	first.Name = "Primeiro"
	first.Active = true
	firstSlug, err := manager.Create(first)
	if err != nil {
		t.Fatal(err)
	}
	second := DefaultProfile()
	second.Name = "Segundo"
	second.Active = false
	secondSlug, err := manager.Create(second)
	if err != nil {
		t.Fatal(err)
	}

	fingerprint, err := manager.CommandMutationSnapshot(secondSlug)
	if err != nil {
		t.Fatal(err)
	}
	mutation, err := manager.PrepareCommandMutation(CommandMutationActivate, secondSlug, nil, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := mutation.CommitCoordinated(nil, nil); err != nil || got != secondSlug {
		t.Fatalf("activate = %q, err = %v", got, err)
	}
	firstAfter, err := manager.Get(firstSlug)
	if err != nil {
		t.Fatal(err)
	}
	secondAfter, err := manager.Get(secondSlug)
	if err != nil {
		t.Fatal(err)
	}
	if firstAfter.Active || !secondAfter.Active {
		t.Fatalf("active flags = first:%v second:%v, want false/true", firstAfter.Active, secondAfter.Active)
	}
}

func TestCommandMutationCreateDuplicateAndDeleteRespectActiveProfile(t *testing.T) {
	manager := setupProfileTestEnv(t)
	active := DefaultProfile()
	active.Name = "Ativo"
	active.Active = true
	activeSlug, err := manager.Create(active)
	if err != nil {
		t.Fatal(err)
	}
	nonActive := DefaultProfile()
	nonActive.Name = "Removível"
	nonActive.Active = false
	nonActiveSlug, err := manager.Create(nonActive)
	if err != nil {
		t.Fatal(err)
	}

	fingerprint, err := manager.CommandMutationSnapshot("")
	if err != nil {
		t.Fatal(err)
	}
	created := DefaultProfile()
	created.Name = "Criado por comando"
	created.Active = false
	createMutation, err := manager.PrepareCommandMutation(CommandMutationCreate, "", created, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	createdSlug, err := createMutation.CommitCoordinated(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Get(createdSlug); err != nil {
		t.Fatalf("created profile %q: %v", createdSlug, err)
	}

	fingerprint, err = manager.CommandMutationSnapshot(activeSlug)
	if err != nil {
		t.Fatal(err)
	}
	duplicateMutation, err := manager.PrepareCommandMutation(CommandMutationDuplicate, activeSlug, nil, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	duplicateSlug, err := duplicateMutation.CommitCoordinated(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	duplicated, err := manager.Get(duplicateSlug)
	if err != nil {
		t.Fatal(err)
	}
	if duplicated.Active {
		t.Fatal("duplicate unexpectedly became active")
	}

	fingerprint, err = manager.CommandMutationSnapshot(nonActiveSlug)
	if err != nil {
		t.Fatal(err)
	}
	deleteMutation, err := manager.PrepareCommandMutation(CommandMutationDelete, nonActiveSlug, nil, fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := deleteMutation.CommitCoordinated(nil, nil); err != nil || got != nonActiveSlug {
		t.Fatalf("delete = %q, err = %v", got, err)
	}
	if _, err := manager.Get(nonActiveSlug); err == nil {
		t.Fatal("deleted profile still readable")
	}

	fingerprint, err = manager.CommandMutationSnapshot(activeSlug)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PrepareCommandMutation(CommandMutationDelete, activeSlug, nil, fingerprint); err == nil {
		t.Fatal("preparing deletion of active profile succeeded")
	}
}

func TestCommandMutationDetectsABAByMTimeAndPreservesResolvedLayer(t *testing.T) {
	manager := NewManager()
	base := t.TempDir()
	manager.resolver = configdir.NewResolverWithBase(base)
	if err := manager.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}

	profile := DefaultProfile()
	profile.Name = "Camada"
	slug, err := manager.Create(profile)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, slug+".json")
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := manager.CommandMutationSnapshot(slug)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(original, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	future := time.Now().Add(10 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.PrepareCommandMutation(CommandMutationUpdate, slug, profile, fingerprint); !errors.Is(err, ErrStaleCommandMutation) {
		t.Fatalf("ABA prepare error = %v, want stale due to mtime", err)
	}
	got := manager.GetSearchPaths()
	if len(got) != 1 || got[0] != base {
		t.Fatalf("resolved search paths = %#v, want [%q]", got, base)
	}
}

func TestCommandMutationRecoveryRollsForwardMixedJournal(t *testing.T) {
	manager := NewManager()
	base := t.TempDir()
	manager.resolver = configdir.NewResolverWithBase(base)
	if err := manager.EnsureDefaults(); err != nil {
		t.Fatal(err)
	}
	firstPath := filepath.Join(base, "one.json")
	secondPath := filepath.Join(base, "two.json")
	firstBefore := []byte(`{"name":"one"}`)
	secondBefore := []byte(`{"name":"two"}`)
	firstAfter := []byte(`{"name":"one-updated"}`)
	secondAfter := []byte(`{"name":"two-updated"}`)
	if err := os.WriteFile(firstPath, firstAfter, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, secondBefore, 0644); err != nil {
		t.Fatal(err)
	}
	journalPath := filepath.Join(base, ".profile-mutation.journal")
	journal := map[string]any{
		"version": 1,
		"created": time.Now().UTC().Format(time.RFC3339Nano),
		"changes": []any{
			map[string]any{"path": firstPath, "before": firstBefore, "before_exists": true, "after": firstAfter, "after_exists": true},
			map[string]any{"path": secondPath, "before": secondBefore, "before_exists": true, "after": secondAfter, "after_exists": true},
		},
	}
	data, err := json.Marshal(journal)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(journalPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := configdir.RecoverProfileTransaction(journalPath, []string{base}); err != nil {
		t.Fatalf("recovery error = %v, want roll-forward", err)
	}
	if got, err := os.ReadFile(secondPath); err != nil {
		t.Fatal(err)
	} else if !bytes.Equal(got, secondAfter) {
		t.Fatalf("second file = %s, want %s", got, secondAfter)
	}
	if _, err := os.Stat(journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("journal stat error = %v, want removed journal", err)
	}
}
