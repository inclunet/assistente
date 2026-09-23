package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/commandcatalog"
	"assistente/internal/commandcontract"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/database"
)

func prepareGlobalCommandHandler(t *testing.T, when string) (*commandHandlerFixture, preparedHotkeyBinding) {
	t.Helper()
	f := newCommandHandlerFixture(t, nil)
	f.job.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+J", When: when}}
	if err := f.repo.SaveJob(f.ctx, f.job); err != nil {
		t.Fatal(err)
	}
	var err error
	f.job, err = f.repo.GetJob(f.ctx, f.job.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.definition.AllowedSources = []commandcatalog.Source{commandcatalog.KeyboardGlobal}
	f.definitionFingerprint = mustDefinitionFingerprint(t, f.job)
	f.rebuildHandler()
	binding, err := f.manager.prepareHotkeyBinding(f.ctx, f.job, "Ctrl+Alt+J", when)
	if err != nil {
		t.Fatalf("prepare hotkey: %v", err)
	}
	return f, binding
}

func globalCommandInvocation(t *testing.T, f *commandHandlerFixture) commandexecution.Invocation {
	t.Helper()
	in := f.invocation(f.definition, f.job)
	in.Source = commandcatalog.KeyboardGlobal
	source := commandcontract.SourceKeyboardGlobal
	in.Envelope.SourceType = &source
	triggerType := string(commandcontract.SourceKeyboardGlobal)
	in.Envelope.TriggerType = &triggerType
	// O handler deve ignorar completamente qualquer tentativa de transportar
	// keys/when pelo envelope. A autoridade é o snapshot privado preparado.
	spec := json.RawMessage(`{"keys":"Ctrl+Alt+Z","when":"{{ eq 1 2 }}"}`)
	in.Envelope.TriggerSpec = &spec
	return in
}

func TestCommandHandlerKeyboardGlobalPreservesPreparedHotkey(t *testing.T) {
	f, binding := prepareGlobalCommandHandler(t, `{{ eq 1 1 }}`)
	in := globalCommandInvocation(t, f)
	handle, err := f.handler.Start(withPreparedHotkeyDispatch(f.ctx, binding), in)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := <-handle.Done; outcome.Status != commandledger.Succeeded {
		t.Fatalf("hotkey global não executou: %+v", outcome)
	}
	if got := f.calls.Load(); got != 1 {
		t.Fatalf("tool calls=%d, esperado 1", got)
	}
	runs, err := f.repo.GetRuns(f.ctx, f.job.ID, 1)
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs=%d err=%v, esperado um run", len(runs), err)
	}
	trigger := runs[0].Trigger
	if trigger.Type != TriggerHotkey || trigger.Keys != binding.keys || trigger.When != binding.when {
		t.Fatalf("trigger perdido/forjado: %+v", trigger)
	}
}

func TestCommandHandlerKeyboardGlobalRequiresPreparedSnapshotBeforeRuntime(t *testing.T) {
	f, _ := prepareGlobalCommandHandler(t, "")
	_, err := f.handler.Start(f.ctx, globalCommandInvocation(t, f))
	if !errors.Is(err, ErrCommandJobDenied) {
		t.Fatalf("snapshot ausente deveria ser recusado no ingresso, erro=%v", err)
	}
	if got := f.calls.Load(); got != 0 {
		t.Fatalf("runtime executou sem snapshot: %d", got)
	}
}

func TestCommandToolContextKeyboardGlobalRequiresConcreteJobTarget(t *testing.T) {
	f, binding := prepareGlobalCommandHandler(t, "")
	if _, _, err := f.manager.CommandToolContext(withPreparedHotkeyDispatch(f.ctx, binding), globalCommandInvocation(t, f), nil); !errors.Is(err, ErrCommandJobDenied) {
		t.Fatalf("tool context sem alvo deveria ser recusado, erro=%v", err)
	}
}

func TestCommandHandlerKeyboardGlobalRejectsForgedSource(t *testing.T) {
	f, binding := prepareGlobalCommandHandler(t, "")
	in := globalCommandInvocation(t, f)
	forged := commandcontract.SourcePalette
	in.Envelope.SourceType = &forged
	if _, err := f.handler.Start(withPreparedHotkeyDispatch(f.ctx, binding), in); !errors.Is(err, ErrCommandJobDenied) {
		t.Fatalf("source forjada deveria ser recusada, erro=%v", err)
	}
	if got := f.calls.Load(); got != 0 {
		t.Fatalf("source forjada alcançou runtime: %d", got)
	}
}

func TestCommandHandlerKeyboardGlobalWhenFalseDoesNotRun(t *testing.T) {
	f, binding := prepareGlobalCommandHandler(t, `{{ eq 1 2 }}`)
	handle, err := f.handler.Start(withPreparedHotkeyDispatch(f.ctx, binding), globalCommandInvocation(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if outcome := <-handle.Done; outcome.Status == commandledger.Succeeded {
		t.Fatalf("when falso produziu sucesso: %+v", outcome)
	}
	if got := f.calls.Load(); got != 0 {
		t.Fatalf("when falso executou tool: %d", got)
	}
	runs, err := f.repo.GetRuns(f.ctx, f.job.ID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("when falso criou runtime: %#v", runs)
	}
}

func TestCommandHandlerKeyboardGlobalRejectsStalePreparedTrigger(t *testing.T) {
	f, binding := prepareGlobalCommandHandler(t, "")
	changed := *f.job
	changed.Triggers = []Trigger{{Type: TriggerHotkey, Keys: "Ctrl+Alt+K"}}
	if err := f.repo.SaveJob(f.ctx, &changed); err != nil {
		t.Fatal(err)
	}
	handle, err := f.handler.Start(withPreparedHotkeyDispatch(f.ctx, binding), globalCommandInvocation(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if outcome := <-handle.Done; outcome.Status == commandledger.Succeeded {
		t.Fatalf("trigger stale produziu sucesso: %+v", outcome)
	}
	if got := f.calls.Load(); got != 0 {
		t.Fatalf("trigger stale alcançou runtime: %d", got)
	}
}

func TestCommandHandlerKeyboardGlobalRejectsPreparedOwnerSwap(t *testing.T) {
	f, binding := prepareGlobalCommandHandler(t, "")
	managerContext := database.WithUserID(context.Background(), "foreign-owner")
	f.manager.cfg.ContextProvider = func() context.Context { return managerContext }
	handle, err := f.handler.Start(withPreparedHotkeyDispatch(f.ctx, binding), globalCommandInvocation(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if outcome := <-handle.Done; outcome.Status == commandledger.Succeeded {
		t.Fatalf("owner stale produziu sucesso: %+v", outcome)
	}
	if got := f.calls.Load(); got != 0 {
		t.Fatalf("owner stale alcançou runtime: %d", got)
	}
}

func TestCommandHandlerKeyboardGlobalRejectsOwnerSwapDuringRetryAuthorization(t *testing.T) {
	f, binding := prepareGlobalCommandHandler(t, "")
	f.tool.retryFirst = true
	f.job.ErrorPolicy = ErrorPolicy{Strategy: ErrorRetry, MaxRetries: 1, RetryDelay: "1ms"}
	if err := f.repo.SaveJob(f.ctx, f.job); err != nil {
		t.Fatal(err)
	}
	var err error
	f.job, err = f.repo.GetJob(f.ctx, f.job.ID)
	if err != nil {
		t.Fatal(err)
	}
	f.definitionFingerprint = mustDefinitionFingerprint(t, f.job)
	f.rebuildHandler()
	binding, err = f.manager.prepareHotkeyBinding(f.ctx, f.job, binding.keys, binding.when)
	if err != nil {
		t.Fatal(err)
	}
	f.authorize = func(context.Context, commandexecution.Invocation, *Job) error {
		// Change ownership after the first real attempt, without relying on
		// how many internal preflight authorization calls precede a retry.
		if f.calls.Load() == 1 {
			f.manager.cfg.ContextProvider = func() context.Context {
				return database.WithUserID(context.Background(), "foreign-owner")
			}
		}
		return nil
	}
	f.rebuildHandler()
	handle, err := f.handler.Start(withPreparedHotkeyDispatch(f.ctx, binding), globalCommandInvocation(t, f))
	if err != nil {
		t.Fatal(err)
	}
	if outcome := <-handle.Done; outcome.Status == commandledger.Succeeded {
		t.Fatalf("owner trocado durante retry produziu sucesso: %+v", outcome)
	}
	if got := f.calls.Load(); got != 1 {
		t.Fatalf("retry após troca de owner executou %d vezes, esperado 1", got)
	}
}
