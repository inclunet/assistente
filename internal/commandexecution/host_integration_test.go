package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandbindings"
	"assistente/internal/commandledger"
)

func newHostStateService(t *testing.T, f *executionFixture, state *HostState) *Service {
	t.Helper()
	config := f.service.config
	config.Snapshot = state.Snapshot
	service, err := New(config)
	if err != nil {
		t.Fatalf("criar executor com HostState: %v", err)
	}
	return service
}

func TestHostStateImpedeStartSemEstadoDoOSOuComCofreSozinho(t *testing.T) {
	ctx := context.Background()
	f := newExecutionFixture(t)
	state, err := NewHostState(f.epochs, f.versions.Registry)
	if err != nil {
		t.Fatal(err)
	}
	service := newHostStateService(t, f, state)
	bindings, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.PublishUserConfiguration(ctx, f.user.ID, bindings); err != nil {
		t.Fatal(err)
	}

	f.startHook = func(context.Context, Invocation) (ExecutionHandle, error) {
		t.Fatal("Start não deveria ocorrer sem estado conhecido do OS")
		return ExecutionHandle{}, nil
	}
	request := f.request(t, "workspace.read")
	record, err := service.Execute(ctx, f.pair.AccessToken, request)
	if err != nil || record.Status != commandledger.CancelledStale {
		t.Fatalf("execução sem estado do OS: record=%+v err=%v", record, err)
	}
	if got := f.startCalls.Load(); got != 0 {
		t.Fatalf("Start sem estado conhecido do OS = %d, want 0", got)
	}

	f.startHook = func(context.Context, Invocation) (ExecutionHandle, error) {
		t.Fatal("Start não deveria ocorrer com cofre verdadeiro sozinho")
		return ExecutionHandle{}, nil
	}
	if err := state.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	record, err = service.Execute(ctx, f.pair.AccessToken, f.request(t, "workspace.read"))
	if err != nil || record.Status != commandledger.CancelledStale {
		t.Fatalf("execução com cofre desbloqueado e OS desconhecido: record=%+v err=%v", record, err)
	}
	if got := f.startCalls.Load(); got != 0 {
		t.Fatalf("Start com cofre sozinho = %d, want 0", got)
	}
}

func TestHostStatePermiteStartComOSDesbloqueadoEMapaPublicado(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	f := newExecutionFixture(t)
	f.service.config.ExecutionTimeout = 30 * time.Second
	state, err := NewHostState(f.epochs, f.versions.Registry)
	if err != nil {
		t.Fatal(err)
	}
	service := newHostStateService(t, f, state)
	bindings, err := commandbindings.NewConfiguration(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := state.PublishUserConfiguration(ctx, f.user.ID, bindings); err != nil {
		t.Fatal(err)
	}
	if err := state.SetVaultUnlocked(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := state.SetOSSessionState(ctx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := state.RebuildUserConfiguration(ctx, func(ctx context.Context) (auth.LocalSessionPrincipal, error) {
		return f.sessions.AuthenticateLocalAccess(ctx, f.pair.AccessToken)
	}, func(context.Context, auth.LocalSessionPrincipal) (*commandbindings.Configuration, []string, error) {
		return bindings, nil, nil
	}); err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	done := make(chan Outcome, 1)
	var handlerCtx context.Context
	f.startHook = func(startCtx context.Context, _ Invocation) (ExecutionHandle, error) {
		handlerCtx = startCtx
		close(started)
		return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() { f.cancelCalls.Add(1) }}, nil
	}
	request := f.request(t, "workspace.read")
	result := make(chan error, 1)
	go func() {
		record, executeErr := service.Execute(ctx, f.pair.AccessToken, request)
		if executeErr == nil && record.Status != commandledger.OutcomeUnknown {
			executeErr = errors.New("resultado antes do encerramento: " + string(record.Status))
		}
		result <- executeErr
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("Start não ocorreu com OS desbloqueado e configuração publicada")
	}
	if got := f.startCalls.Load(); got != 1 {
		t.Fatalf("Start calls = %d, want 1", got)
	}

	if err := state.SetOSSessionState(ctx, true, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-handlerCtx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("contexto recebido pelo handler não foi cancelado após lock do OS")
	}
	select {
	case err := <-result:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("execução não terminou após lock do OS")
	}
	if got := f.cancelCalls.Load(); got != 1 {
		t.Fatalf("Cancel chamado %d vezes, want exatamente 1", got)
	}
	select {
	case done <- Outcome{Status: commandledger.Succeeded}:
	default:
	}
}
