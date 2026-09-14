package commandexecution

import (
	"context"
	"errors"
	"testing"
	"time"

	"assistente/internal/auth"
	"assistente/internal/commandledger"
)

func TestServicePersistsPolicyDenialWithoutStartingHandler(t *testing.T) {
	f := newExecutionFixture(t)
	f.authorizeHook = func(int) error { return errors.New("política de teste negou") }
	request := f.request(t, "workspace.read")

	record, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
	if err != nil || record.Status != commandledger.Denied {
		t.Fatalf("negação: record=%+v err=%v", record, err)
	}
	if got := f.startCalls.Load(); got != 0 {
		t.Fatalf("Start calls = %d, want 0", got)
	}
	if got := f.snapshotCalls.Load(); got != 2 {
		t.Fatalf("Snapshot calls = %d, want 2 (capture e queue)", got)
	}
	if got := f.authorizeCalls.Load(); got != 1 {
		t.Fatalf("Authorize calls = %d, want 1", got)
	}
	if persisted := f.record(t, request); persisted.Status != commandledger.Denied {
		t.Fatalf("negação não persistida: %q", persisted.Status)
	}
}

func TestServiceInvalidTokenDoesNotCreateLedger(t *testing.T) {
	f := newExecutionFixture(t)
	request := f.request(t, "workspace.read")

	record, err := f.service.Execute(context.Background(), "token inválido", request)
	if record != (commandledger.Record{}) || !errors.Is(err, auth.ErrUnauthenticatedLocalSession) {
		t.Fatalf("token inválido: record=%+v err=%v", record, err)
	}
	if got := f.snapshotCalls.Load(); got != 0 {
		t.Fatalf("Snapshot calls = %d, want 0", got)
	}
	var ledgers, audits int64
	if err := f.db.Table("command_idempotency_keys").Count(&ledgers).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.Table("command_invocations").Count(&audits).Error; err != nil {
		t.Fatal(err)
	}
	if ledgers != 0 || audits != 0 {
		t.Fatalf("token inválido criou persistência: ledger=%d audit=%d", ledgers, audits)
	}
}

func TestServiceCancellationAfterStartPersistsUnknown(t *testing.T) {
	f := newExecutionFixture(t)
	started := make(chan struct{})
	f.startHook = func(_ context.Context, _ Invocation) (ExecutionHandle, error) {
		close(started)
		return ExecutionHandle{ID: newTestUUID(), Done: make(chan Outcome), Cancel: func() { f.cancelCalls.Add(1) }}, nil
	}
	request := f.request(t, "workspace.read")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan struct {
		record commandledger.Record
		err    error
	}, 1)
	go func() {
		record, err := f.service.Execute(ctx, f.pair.AccessToken, request)
		result <- struct {
			record commandledger.Record
			err    error
		}{record: record, err: err}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Start não ocorreu")
	}
	cancel()
	select {
	case got := <-result:
		if got.err != nil || got.record.Status != commandledger.OutcomeUnknown {
			t.Fatalf("cancelamento após Start: record=%+v err=%v", got.record, got.err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Execute não finalizou após expirar o contexto")
	}
	if got := f.cancelCalls.Load(); got != 1 {
		t.Fatalf("Cancel calls = %d, want 1", got)
	}
	if got := f.record(t, request).Status; got != commandledger.OutcomeUnknown {
		t.Fatalf("outcome desconhecido não persistido: %q", got)
	}
}

func TestServiceStartPanicErrorOrClosedChannelPersistUnknown(t *testing.T) {
	tests := []struct {
		name  string
		start func(*executionFixture) func(context.Context, Invocation) (ExecutionHandle, error)
	}{
		{name: "panic", start: func(*executionFixture) func(context.Context, Invocation) (ExecutionHandle, error) {
			return func(context.Context, Invocation) (ExecutionHandle, error) { panic("panic do adapter") }
		}},
		{name: "erro", start: func(*executionFixture) func(context.Context, Invocation) (ExecutionHandle, error) {
			return func(context.Context, Invocation) (ExecutionHandle, error) {
				return ExecutionHandle{}, errors.New("erro de Start")
			}
		}},
		{name: "canal fechado", start: func(f *executionFixture) func(context.Context, Invocation) (ExecutionHandle, error) {
			return func(context.Context, Invocation) (ExecutionHandle, error) {
				done := make(chan Outcome)
				close(done)
				return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() { f.cancelCalls.Add(1) }}, nil
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newExecutionFixture(t)
			f.startHook = tc.start(f)
			request := f.request(t, "workspace.read")

			record, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
			if err != nil || record.Status != commandledger.OutcomeUnknown {
				t.Fatalf("%s: record=%+v err=%v", tc.name, record, err)
			}
			if got := f.startCalls.Load(); got != 1 {
				t.Fatalf("Start calls = %d, want 1", got)
			}
			if got := f.record(t, request).Status; got != commandledger.OutcomeUnknown {
				t.Fatalf("%s não persistiu unknown: %q", tc.name, got)
			}
		})
	}
}

func TestServiceVersionChangesBetweenQueueAndRunningBecomeStale(t *testing.T) {
	tests := []struct {
		name   string
		change func(*executionFixture, *Versions)
	}{
		{name: "registry", change: func(_ *executionFixture, versions *Versions) { versions.Registry = "registry-v2" }},
		{name: "configuração global", change: func(_ *executionFixture, versions *Versions) { versions.GlobalConfig = "global-v2" }},
		{name: "camadas ativas", change: func(_ *executionFixture, versions *Versions) { versions.ActiveLayers = "layers-v2" }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newExecutionFixture(t)
			f.snapshotHook = func(call int, versions Versions) (Versions, error) {
				if call == 3 {
					tc.change(f, &versions)
				}
				return versions, nil
			}
			request := f.request(t, "workspace.read")

			record, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
			if err != nil || record.Status != commandledger.CancelledStale {
				t.Fatalf("stale %s: record=%+v err=%v", tc.name, record, err)
			}
			if got := f.snapshotCalls.Load(); got != 3 {
				t.Fatalf("Snapshot calls = %d, want capture+queue+running", got)
			}
			if got := f.startCalls.Load(); got != 0 {
				t.Fatalf("Start calls = %d, want 0", got)
			}
			if got := f.record(t, request).Status; got != commandledger.CancelledStale {
				t.Fatalf("stale não persistido: %q", got)
			}
		})
	}
}

func TestServiceAuthorizationChangeBetweenQueueAndRunningBecomesStale(t *testing.T) {
	f := newExecutionFixture(t)
	f.authorizeHook = func(call int) error {
		if call == 2 {
			return errors.New("autorização revogada antes do running")
		}
		return nil
	}
	request := f.request(t, "workspace.read")

	record, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
	if err != nil || record.Status != commandledger.CancelledStale {
		t.Fatalf("autorização stale: record=%+v err=%v", record, err)
	}
	if got := f.snapshotCalls.Load(); got != 3 {
		t.Fatalf("Snapshot calls = %d, want 3", got)
	}
	if got := f.authorizeCalls.Load(); got != 2 {
		t.Fatalf("Authorize calls = %d, want 2", got)
	}
	if got := f.startCalls.Load(); got != 0 {
		t.Fatalf("Start calls = %d, want 0", got)
	}
	if got := f.record(t, request).Status; got != commandledger.CancelledStale {
		t.Fatalf("autorização stale não persistida: %q", got)
	}
}

func TestServiceLogoutDuringHandleWaitReleasesGateAndDoesNotRepeatStart(t *testing.T) {
	f := newExecutionFixture(t)
	started := make(chan struct{})
	done := make(chan Outcome, 1)
	f.startHook = func(_ context.Context, _ Invocation) (ExecutionHandle, error) {
		close(started)
		return ExecutionHandle{ID: newTestUUID(), Done: done, Cancel: func() { f.cancelCalls.Add(1) }}, nil
	}
	request := f.request(t, "workspace.read")
	result := make(chan struct {
		record commandledger.Record
		err    error
	}, 1)
	go func() {
		record, err := f.service.Execute(context.Background(), f.pair.AccessToken, request)
		result <- struct {
			record commandledger.Record
			err    error
		}{record: record, err: err}
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("Start não ocorreu")
	}

	logout := make(chan error, 1)
	go func() {
		logout <- f.epochs.MutatePrincipal(context.Background(), f.user.ID, f.pair.SessionID, func() error {
			return f.sessions.Logout(context.Background(), f.pair.RefreshToken)
		})
	}()
	select {
	case err := <-logout:
		if err != nil {
			t.Fatalf("logout coordenado: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("logout ficou bloqueado: o handle deveria ser aguardado fora do DispatchGate")
	}

	select {
	case got := <-result:
		if got.err != nil || got.record.Status != commandledger.OutcomeUnknown {
			t.Fatalf("resultado após logout: record=%+v err=%v", got.record, got.err)
		}
	case <-time.After(time.Second):
		t.Fatal("Execute não concluiu após invalidação")
	}
	done <- Outcome{Status: commandledger.Succeeded} // Tardio: não pode sobrescrever terminal.
	if f.record(t, request).Status != commandledger.OutcomeUnknown {
		t.Fatal("outcome tardio alterou terminal")
	}
	if got := f.startCalls.Load(); got != 1 {
		t.Fatalf("Start calls = %d, want 1", got)
	}
	if got := f.cancelCalls.Load(); got != 1 {
		t.Fatalf("Cancel calls = %d, want 1 após invalidação", got)
	}
	var ledgers int64
	if err := f.db.Table("command_idempotency_keys").Count(&ledgers).Error; err != nil {
		t.Fatal(err)
	}
	if ledgers != 1 {
		t.Fatalf("logout/replay criou ledgers adicionais: %d", ledgers)
	}
}
