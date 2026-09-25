package commandsecurity

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

const testTimeout = time.Second

func TestDispatchGateZeroValueAndNilInputsFailClosed(t *testing.T) {
	var gate DispatchGate
	if err := gate.WithAdmission(context.Background(), nil); err == nil {
		t.Fatal("callback nil deveria falhar fechado")
	}
	if err := gate.WithMutation(nil, func() error { return nil }); err == nil { //nolint:staticcheck // Testa deliberadamente a recusa de contexto nil.
		t.Fatal("contexto nil deveria falhar fechado")
	}
	called := false
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := gate.WithAdmission(ctx, func() error { called = true; return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("callback não deveria executar")
	}
}

func TestDispatchGateNilReceiverFailsClosed(t *testing.T) {
	var gate *DispatchGate
	called := false
	if err := gate.WithAdmission(context.Background(), func() error { called = true; return nil }); err == nil {
		t.Fatal("receiver nil deveria falhar fechado")
	}
	if err := gate.WithMutation(context.Background(), func() error { called = true; return nil }); err == nil {
		t.Fatal("receiver nil deveria falhar fechado")
	}
	if called {
		t.Fatal("callback não deveria executar")
	}
}

func TestDispatchGateAdmissionHoldsSharedLock(t *testing.T) {
	var gate DispatchGate
	checks := make(chan bool, 1)
	if err := gate.WithAdmission(context.Background(), func() error { checks <- !gate.mu.TryLock(); return nil }); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, checks); !got {
		t.Fatal("admission não manteve o lock compartilhado")
	}
}

func TestDispatchGateMutationHoldsExclusiveLock(t *testing.T) {
	var gate DispatchGate
	checks := make(chan bool, 1)
	if err := gate.WithMutation(context.Background(), func() error { checks <- !gate.mu.TryRLock(); return nil }); err != nil {
		t.Fatal(err)
	}
	if got := receive(t, checks); !got {
		t.Fatal("mutation não manteve o lock exclusivo")
	}
}

func TestDispatchGateAdmissionsOverlap(t *testing.T) {
	var gate DispatchGate
	entered := make(chan struct{}, 2)
	release := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- gate.WithAdmission(context.Background(), func() error {
				entered <- struct{}{}
				return waitFor(release)
			})
		}()
	}
	for range 2 {
		receive(t, entered)
	}
	close(release)
	for range 2 {
		if err := receive(t, results); err != nil {
			t.Fatal(err)
		}
	}
	wg.Wait()
}

func TestDispatchGateMutationWaitsAdmissionAndAdmissionWaitsMutation(t *testing.T) {
	var gate DispatchGate
	admissionEntered := make(chan struct{}, 1)
	releaseAdmission := make(chan struct{})
	admissionResult := make(chan error, 1)
	go func() {
		admissionResult <- gate.WithAdmission(context.Background(), func() error { admissionEntered <- struct{}{}; return waitFor(releaseAdmission) })
	}()
	receive(t, admissionEntered)
	if gate.mu.TryLock() {
		gate.mu.Unlock()
		t.Fatal("admission deveria manter o lock compartilhado")
	}

	mutationEntered := make(chan struct{}, 1)
	releaseMutation := make(chan struct{})
	mutationResult := make(chan error, 1)
	go func() {
		mutationResult <- gate.WithMutation(context.Background(), func() error { mutationEntered <- struct{}{}; return waitFor(releaseMutation) })
	}()
	close(releaseAdmission)
	if err := receive(t, admissionResult); err != nil {
		t.Fatal(err)
	}
	receive(t, mutationEntered)
	if gate.mu.TryRLock() {
		gate.mu.RUnlock()
		t.Fatal("mutation deveria manter o lock exclusivo")
	}

	secondAdmission := make(chan struct{}, 1)
	secondResult := make(chan error, 1)
	go func() {
		secondResult <- gate.WithAdmission(context.Background(), func() error { secondAdmission <- struct{}{}; return nil })
	}()
	close(releaseMutation)
	if err := receive(t, mutationResult); err != nil {
		t.Fatal(err)
	}
	receive(t, secondAdmission)
	if err := receive(t, secondResult); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchGateCancelledAfterAdmissionAttemptSkipsCallback(t *testing.T) {
	var gate DispatchGate
	holderEntered := make(chan struct{}, 1)
	releaseHolder := make(chan struct{})
	holderResult := make(chan error, 1)
	go func() {
		holderResult <- gate.WithMutation(context.Background(), func() error { holderEntered <- struct{}{}; return waitFor(releaseHolder) })
	}()
	receive(t, holderEntered)

	ctx, cancel := newFirstErrContext(context.Background())
	defer cancel()
	called := make(chan struct{}, 1)
	result := make(chan error, 1)
	go func() { result <- gate.WithAdmission(ctx, func() error { called <- struct{}{}; return nil }) }()
	receive(t, ctx.firstErrSeen)
	cancel()
	close(releaseHolder)
	if err := receive(t, holderResult); err != nil {
		t.Fatal(err)
	}
	if err := receive(t, result); !errors.Is(err, context.Canceled) {
		t.Fatalf("erro = %v, want context.Canceled", err)
	}
	assertNoSignal(t, called)
}

func TestDispatchGatePanicUnlocksAdmissionAndMutation(t *testing.T) {
	var gate DispatchGate
	assertPanicCompletes(t, func() { _ = gate.WithAdmission(context.Background(), func() error { panic("admission") }) })
	assertPanicCompletes(t, func() { _ = gate.WithMutation(context.Background(), func() error { panic("mutation") }) })
	assertCompletes(t, func() error { return gate.WithAdmission(context.Background(), func() error { return nil }) })
	assertCompletes(t, func() error { return gate.WithMutation(context.Background(), func() error { return nil }) })
}

func TestDispatchGateDoesNotHoldLockOutsideCallback(t *testing.T) {
	var gate DispatchGate
	if err := gate.WithAdmission(context.Background(), func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := gate.WithMutation(context.Background(), func() error { return nil }); err != nil {
		t.Fatal(err)
	}
}

func waitFor(ch <-chan struct{}) error {
	select {
	case <-ch:
		return nil
	case <-time.After(testTimeout):
		return errors.New("liberação não recebida")
	}
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(testTimeout):
		t.Fatal("espera do teste excedeu o limite")
		var zero T
		return zero
	}
}

func assertNoSignal[T any](t *testing.T, ch <-chan T) {
	t.Helper()
	select {
	case value := <-ch:
		t.Fatalf("sinal inesperado: %v", value)
	default:
	}
}

func assertCompletes(t *testing.T, fn func() error) {
	t.Helper()
	result := make(chan error, 1)
	go func() { result <- fn() }()
	if err := receive(t, result); err != nil {
		t.Fatal(err)
	}
}

func assertPanicCompletes(t *testing.T, fn func()) {
	t.Helper()
	result := make(chan bool, 1)
	go func() {
		panicked := false
		defer func() { result <- panicked }()
		defer func() {
			if recover() != nil {
				panicked = true
			}
		}()
		fn()
	}()
	if !receive(t, result) {
		t.Fatal("panic deveria ser propagado")
	}
}

type firstErrContext struct {
	context.Context
	firstErrSeen chan struct{}
	once         sync.Once
}

func newFirstErrContext(parent context.Context) (*firstErrContext, context.CancelFunc) {
	ctx, cancel := context.WithCancel(parent)
	return &firstErrContext{Context: ctx, firstErrSeen: make(chan struct{})}, cancel
}

func (c *firstErrContext) Err() error {
	err := c.Context.Err()
	c.once.Do(func() { close(c.firstErrSeen) })
	return err
}
