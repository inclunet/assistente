package jobs

import (
	"context"
	"errors"
	"testing"
)

func TestCommandHotkeyOccurrenceDispatchesOnceAndCannotBeReused(t *testing.T) {
	repo, userA, userB := setupJobsRepositoryTest(t)
	registrar := &capturedHotkeyRegistrar{}
	managerContext := userA
	var dispatched []CommandHotkeyOccurrence
	mgr := mustNewManager(t, ManagerConfig{
		Repository: repo,
		ContextProvider: func() context.Context {
			return managerContext
		},
		HotkeyManager: registrar,
		DispatchCommandHotkey: func(ctx context.Context, occurrence CommandHotkeyOccurrence) error {
			dispatched = append(dispatched, occurrence)
			prepared, err := occurrence.Context(ctx)
			if err != nil {
				return err
			}
			if prepared.Value(preparedHotkeyDispatchKey{}) == nil {
				t.Fatal("dispatcher recebeu contexto sem preparação privada")
			}
			return nil
		},
	})
	job := testRepositoryJob("hotkey-dispatch", "Hotkey dispatch")
	job.Triggers = []Trigger{{Type: TriggerHotkey, Keys: " Ctrl+Alt+J ", When: " {{ eq 1 1 }} "}}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Stop)

	callback := registrar.callback(t, 0)
	callback()
	if len(dispatched) != 1 {
		t.Fatalf("dispatcher chamado %d vezes, esperado 1", len(dispatched))
	}
	occurrence := dispatched[0]
	binding := occurrence.Binding()
	if binding.JobDatabaseID != job.DatabaseID || binding.JobSlug != job.ID || binding.Keys != "Ctrl+Alt+J" || binding.When != "{{ eq 1 1 }}" || binding.DefinitionFingerprint == "" || binding.BindingFingerprint == "" {
		t.Fatalf("binding opaco incompleto: %+v", binding)
	}
	if !occurrence.Current() || occurrence.Validate(userA) != nil {
		t.Fatal("ocorrência válida foi recusada")
	}

	managerContext = userB
	callback()
	if len(dispatched) != 1 {
		t.Fatal("owner trocado permitiu novo dispatch")
	}
	managerContext = userA
	changed := *job
	changed.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+K", When: "{{ eq 1 1 }}"}}
	if err := repo.SaveJob(userA, &changed); err != nil {
		t.Fatal(err)
	}
	callback()
	if len(dispatched) != 1 {
		t.Fatal("trigger alterado permitiu novo dispatch")
	}

	retired := occurrence
	mgr.Stop()
	if retired.Current() || !errors.Is(retired.Validate(userA), errPreparedHotkeyDenied) {
		t.Fatal("ocorrência aposentada continuou válida")
	}
	if _, err := retired.Context(userA); !errors.Is(err, errPreparedHotkeyDenied) {
		t.Fatalf("contexto de ocorrência aposentada: %v", err)
	}
	callback()
	if len(dispatched) != 1 {
		t.Fatal("callback aposentado reativou dispatch")
	}

	var zero CommandHotkeyOccurrence
	if zero.Current() || zero.Validate(userA) == nil {
		t.Fatal("ocorrência zero foi aceita")
	}
}

func TestCommandHotkeyBindingsReadsAuthoritativeEnabledOwnerJobs(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := mustNewManager(t, ManagerConfig{Repository: repo, ContextProvider: func() context.Context { return userA }})

	first := testRepositoryJob("binding-z", "Binding Z")
	first.Triggers = []Trigger{{Type: TriggerHotkey, Keys: " Ctrl+Alt+Z ", When: " {{ eq 1 1 }} "}}
	if err := repo.SaveJob(userA, first); err != nil {
		t.Fatal(err)
	}
	second := testRepositoryJob("binding-a", "Binding A")
	second.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+A"}}
	if err := repo.SaveJob(userA, second); err != nil {
		t.Fatal(err)
	}
	disabled := testRepositoryJob("binding-disabled", "Disabled")
	disabled.Enabled = false
	disabled.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+D"}}
	if err := repo.SaveJob(userA, disabled); err != nil {
		t.Fatal(err)
	}

	bindings, err := mgr.CommandHotkeyBindings(userA)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 2 || bindings[0].JobSlug != "binding-a" || bindings[1].JobSlug != "binding-z" {
		t.Fatalf("projeção autoritativa inesperada: %+v", bindings)
	}
	if bindings[1].Keys != "Ctrl+Alt+Z" || bindings[1].When != "{{ eq 1 1 }}" || bindings[1].BindingFingerprint == "" {
		t.Fatalf("projeção não normalizou trigger: %+v", bindings[1])
	}
	if bindings[0].DefinitionFingerprint == bindings[1].DefinitionFingerprint {
		t.Fatal("jobs distintos compartilharam fingerprint de definição")
	}
}

func TestCommandHotkeyBindingsSkipsInvalidPersistedJobAndKeepsValidBinding(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	mgr := mustNewManager(t, ManagerConfig{Repository: repo, ContextProvider: func() context.Context { return userA }})

	invalid := testRepositoryJob("binding-invalid", "Binding invalid")
	invalid.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+DefinitelyNotAKey"}}
	if err := repo.SaveJob(userA, invalid); err != nil {
		t.Fatal(err)
	}
	valid := testRepositoryJob("binding-valid", "Binding valid")
	valid.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+V"}}
	if err := repo.SaveJob(userA, valid); err != nil {
		t.Fatal(err)
	}
	empty := testRepositoryJob("binding-empty", "Binding empty")
	empty.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "   "}}
	if err := repo.SaveJob(userA, empty); err != nil {
		t.Fatal(err)
	}

	bindings, err := mgr.CommandHotkeyBindings(userA)
	if err != nil {
		t.Fatal(err)
	}
	if len(bindings) != 1 || bindings[0].JobSlug != valid.ID || bindings[0].Keys != "Ctrl+Alt+V" {
		t.Fatalf("hotkey válida foi bloqueada por configuração inválida: %+v", bindings)
	}
}
