package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	"assistente/internal/toolinvocations"
	"assistente/internal/tools"
)

func TestPreparedHotkeyBindingRejectsReplacementDisabledAndCanceledOwner(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("prepared-hotkey", "Prepared hotkey")
	job.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+J", When: `{{ eq 1 1 }}`}}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatal(err)
	}

	managerContext := userA
	mgr := mustNewManager(t, ManagerConfig{Repository: repo, ContextProvider: func() context.Context { return managerContext }})
	binding, err := mgr.prepareHotkeyBinding(userA, job, "Ctrl+Alt+J", `{{ eq 1 1 }}`)
	if err != nil {
		t.Fatalf("prepare hotkey: %v", err)
	}
	if binding.ownerUserID != "user-a" || binding.jobDatabaseID != job.DatabaseID || binding.jobSlug != job.ID || binding.definitionFingerprint == "" {
		t.Fatalf("snapshot não capturou identidade completa: %+v", binding)
	}
	if _, err := mgr.revalidateHotkeyBinding(userA, binding); err != nil {
		t.Fatalf("binding inalterado recusado: %v", err)
	}

	replacement := *job
	replacement.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+K", When: `{{ eq 1 1 }}`}}
	if err := repo.SaveJob(userA, &replacement); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.revalidateHotkeyBinding(userA, binding); !errors.Is(err, errPreparedHotkeyDenied) {
		t.Fatalf("trigger substituído deveria ser recusado, erro=%v", err)
	}

	replacement.Triggers = job.Triggers
	replacement.Enabled = false
	if err := repo.SaveJob(userA, &replacement); err != nil {
		t.Fatal(err)
	}
	if _, err := mgr.revalidateHotkeyBinding(userA, binding); !errors.Is(err, errPreparedHotkeyDenied) {
		t.Fatalf("job desabilitado deveria ser recusado, erro=%v", err)
	}

	replacement.Enabled = true
	if err := repo.SaveJob(userA, &replacement); err != nil {
		t.Fatal(err)
	}
	managerContext, cancel := context.WithCancel(userA)
	cancel()
	if _, err := mgr.revalidateHotkeyBinding(userA, binding); !errors.Is(err, errPreparedHotkeyDenied) {
		t.Fatalf("owner cancelado deveria ser recusado, erro=%v", err)
	}
}

func TestPreparedHotkeyDispatchRejectsRegistryDefinitionReplacement(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	job := testRepositoryJob("registry-hotkey", "Registry hotkey")
	job.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+R"}}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatal(err)
	}
	mgr := mustNewManager(t, ManagerConfig{Repository: repo, ContextProvider: func() context.Context { return userA }})
	mgr.registry.Set(job)
	binding, err := mgr.prepareHotkeyBinding(userA, job, "Ctrl+Alt+R", "")
	if err != nil {
		t.Fatalf("prepare hotkey: %v", err)
	}

	registryReplacement := *job
	registryReplacement.Name = "Registry hotkey replaced"
	mgr.registry.Set(&registryReplacement)
	mgr.executeJob(withPreparedHotkeyDispatch(userA, binding), &registryReplacement, &TriggerContext{
		Type: TriggerHotkey, Keys: binding.keys, EventPayload: map[string]any{},
	})

	runs, err := repo.GetRuns(userA, job.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("registry substituído permitiu execução: %#v", runs)
	}
}

func TestManagerHotkeyCallbackRejectsOwnerAndDefinitionSwap(t *testing.T) {
	repo, userA, userB := setupJobsRepositoryTest(t)
	registrar := &capturedHotkeyRegistrar{}
	managerContext := userA
	var dispatches atomic.Int32
	mgr := mustNewManager(t, ManagerConfig{
		Repository: repo, ContextProvider: func() context.Context { return managerContext }, HotkeyManager: registrar,
		DispatchCommandHotkey: func(ctx context.Context, occurrence CommandHotkeyOccurrence) error {
			prepared, err := occurrence.Context(ctx)
			if err != nil {
				return err
			}
			if prepared.Err() != nil {
				return prepared.Err()
			}
			dispatches.Add(1)
			return nil
		},
	})
	job := testRepositoryJob("hotkey-owner-definition", "Hotkey owner/definition")
	job.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+O"}}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Stop)
	callback := registrar.callback(t, 0)

	managerContext = userB
	callback()
	if runs, err := repo.GetRuns(userA, job.ID, 1); err != nil || len(runs) != 0 {
		t.Fatalf("owner trocado permitiu execução: runs=%d err=%v", len(runs), err)
	}
	if got := dispatches.Load(); got != 0 {
		t.Fatalf("owner trocado alcançou dispatcher: %d", got)
	}

	managerContext = userA
	changed := *job
	changed.Name = "Hotkey definition changed"
	if err := repo.SaveJob(userA, &changed); err != nil {
		t.Fatal(err)
	}
	callback()
	if runs, err := repo.GetRuns(userA, job.ID, 1); err != nil || len(runs) != 0 {
		t.Fatalf("definição trocada permitiu execução: runs=%d err=%v", len(runs), err)
	}
	if got := dispatches.Load(); got != 0 {
		t.Fatalf("definição trocada alcançou dispatcher: %d", got)
	}

	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatal(err)
	}
	mgr.unregisterTriggers(job)
	mgr.registry.Set(job)
	mgr.registerTriggers(job)
	positive := registrar.callback(t, 1)
	managerContext = userA
	positive()
	if got := dispatches.Load(); got != 1 {
		t.Fatalf("controle positivo não alcançou dispatcher: %d", got)
	}
}

func TestManagerHotkeyLifetimeRejectsOldCallbackAfterReregister(t *testing.T) {
	repo, userA, _ := setupJobsRepositoryTest(t)
	registrar := &capturedHotkeyRegistrar{}
	registry := tools.NewRegistry()
	registry.MustRegister(&fakeTool{
		name: "test_tool", params: json.RawMessage(`{"type":"object"}`), response: `{"ok":true}`,
	})
	invocations := toolinvocations.NewService(toolinvocations.NewDBRepository(repo.db), tools.NewExecutor(registry, tools.DefaultExecutorConfig()))
	var dispatches atomic.Int32
	mgr := mustNewManager(t, ManagerConfig{
		Repository: repo, ToolRegistry: registry, ToolInvocations: invocations,
		ContextProvider: func() context.Context { return userA }, HotkeyManager: registrar,
		DispatchCommandHotkey: func(ctx context.Context, occurrence CommandHotkeyOccurrence) error {
			prepared, err := occurrence.Context(ctx)
			if err != nil {
				return err
			}
			if prepared.Err() != nil {
				return prepared.Err()
			}
			dispatches.Add(1)
			return nil
		},
	})
	job := testRepositoryJob("hotkey-lifetime", "Hotkey lifetime")
	job.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+L"}}
	if err := repo.SaveJob(userA, job); err != nil {
		t.Fatal(err)
	}
	if err := mgr.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(mgr.Stop)
	oldCallback := registrar.callback(t, 0)

	mgr.unregisterTriggers(job)
	mgr.registerTriggers(job)
	newCallback := registrar.callback(t, 1)
	oldCallback()
	if got := dispatches.Load(); got != 0 {
		t.Fatalf("callback antigo foi reativado após re-registro: dispatches=%d", got)
	}
	newCallback()
	if got := dispatches.Load(); got != 1 {
		t.Fatalf("callback novo não alcançou dispatcher: dispatches=%d", got)
	}
}
