package commandsecurity

import (
	"context"
	"errors"
	"runtime"
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
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- gate.WithAdmission(context.Background(), func() error {
			entered <- struct{}{}
			return waitFor(release)
		})
	}()
	receive(t, entered)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observed := &doneObservedContext{Context: ctx, doneSeen: make(chan struct{})}
	mutation := make(chan error, 1)
	go func() { mutation <- gate.WithMutation(observed, func() error { return nil }) }()
	receive(t, observed.doneSeen)
	assertNoSignal(t, mutation)
	close(release)
	if err := receive(t, result); err != nil {
		t.Fatal(err)
	}
	if err := receive(t, mutation); err != nil {
		t.Fatal(err)
	}
}

func TestDispatchGateMutationHoldsExclusiveLock(t *testing.T) {
	var gate DispatchGate
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- gate.WithMutation(context.Background(), func() error {
			entered <- struct{}{}
			return waitFor(release)
		})
	}()
	receive(t, entered)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	observed := &doneObservedContext{Context: ctx, doneSeen: make(chan struct{})}
	admission := make(chan error, 1)
	go func() { admission <- gate.WithAdmission(observed, func() error { return nil }) }()
	receive(t, observed.doneSeen)
	assertNoSignal(t, admission)
	close(release)
	if err := receive(t, result); err != nil {
		t.Fatal(err)
	}
	if err := receive(t, admission); err != nil {
		t.Fatal(err)
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

func TestDispatchGateCancellationWhileBlockedSkipsCallback(t *testing.T) {
	for _, mode := range []string{"reader", "writer"} {
		t.Run(mode, func(t *testing.T) {
			var gate DispatchGate
			releaseHolder, holderResult := holdGateAgainst(t, &gate, mode)

			base, cancel := context.WithCancel(context.Background())
			ctx := &doneObservedContext{Context: base, doneSeen: make(chan struct{})}
			defer cancel()
			called := make(chan struct{}, 1)
			result := make(chan error, 1)
			go func() {
				fn := func() error { called <- struct{}{}; return nil }
				if mode == "reader" {
					result <- gate.WithAdmission(ctx, fn)
				} else {
					result <- gate.WithMutation(ctx, fn)
				}
			}()
			// Done informa que o Acquire começou; não é usado como prova de fila.
			receive(t, ctx.doneSeen)
			if mode == "writer" {
				waitForWriterQueued(t, &gate)
			}
			cancel()
			if err := receive(t, result); !errors.Is(err, context.Canceled) {
				t.Fatalf("erro = %v, want context.Canceled", err)
			}
			assertNoSignal(t, called)
			close(releaseHolder)
			if err := receive(t, holderResult); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDispatchGateDeadlineWhileBlockedSkipsCallback(t *testing.T) {
	for _, mode := range []string{"reader", "writer"} {
		t.Run(mode, func(t *testing.T) {
			var gate DispatchGate
			releaseHolder, holderResult := holdGateAgainst(t, &gate, mode)

			base, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
			defer cancel()
			ctx := &doneObservedContext{Context: base, doneSeen: make(chan struct{})}
			called := make(chan struct{}, 1)
			result := make(chan error, 1)
			go func() {
				fn := func() error { called <- struct{}{}; return nil }
				if mode == "reader" {
					result <- gate.WithAdmission(ctx, fn)
				} else {
					result <- gate.WithMutation(ctx, fn)
				}
			}()
			// O canal só confirma que Acquire consultou Done, antes de enfileirar.
			receive(t, ctx.doneSeen)
			// O deadline pode vencer antes de observarmos a fila num runner lento.
			// O holder permanece ocupado até comprovar o retorno por deadline.
			if err := receive(t, result); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("erro = %v, want context.DeadlineExceeded", err)
			}
			assertNoSignal(t, called)
			close(releaseHolder)
			if err := receive(t, holderResult); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func holdGateAgainst(t *testing.T, gate *DispatchGate, waiterMode string) (chan struct{}, chan error) {
	t.Helper()
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- gate.WithAdmission(context.Background(), func() error {
			entered <- struct{}{}
			return waitFor(release)
		})
	}()
	receive(t, entered)
	if waiterMode == "reader" {
		writerEntered := make(chan struct{}, 1)
		writerRelease := make(chan struct{})
		writerResult := make(chan error, 1)
		go func() {
			writerResult <- gate.WithMutation(context.Background(), func() error {
				writerEntered <- struct{}{}
				return waitFor(writerRelease)
			})
		}()
		waitForWriterQueued(t, gate)
		close(release)
		if err := receive(t, result); err != nil {
			t.Fatal(err)
		}
		receive(t, writerEntered)
		return writerRelease, writerResult
	}
	return release, result
}

func TestDispatchGateWriterPreferencePreventsStarvation(t *testing.T) {
	var gate DispatchGate
	readerEntered := make(chan struct{}, 1)
	releaseReader := make(chan struct{})
	readerResult := make(chan error, 1)
	go func() {
		readerResult <- gate.WithAdmission(context.Background(), func() error { readerEntered <- struct{}{}; return waitFor(releaseReader) })
	}()
	receive(t, readerEntered)

	writerEntered := make(chan struct{}, 1)
	releaseWriter := make(chan struct{})
	writerResult := make(chan error, 1)
	go func() {
		writerResult <- gate.WithMutation(context.Background(), func() error { writerEntered <- struct{}{}; return waitFor(releaseWriter) })
	}()
	waitForWriterQueued(t, &gate)

	lateReader := make(chan struct{}, 1)
	lateReaderResult := make(chan error, 1)
	go func() {
		lateReaderResult <- gate.WithAdmission(context.Background(), func() error { lateReader <- struct{}{}; return nil })
	}()
	close(releaseReader)
	receive(t, writerEntered)
	assertNoSignal(t, lateReader)
	close(releaseWriter)
	if err := receive(t, writerResult); err != nil {
		t.Fatal(err)
	}
	receive(t, lateReader)
	if err := receive(t, lateReaderResult); err != nil {
		t.Fatal(err)
	}
	if err := receive(t, readerResult); err != nil {
		t.Fatal(err)
	}
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

func assertGateSharedHeld(t *testing.T, gate *DispatchGate) {
	t.Helper()
	const capacity = int64(1<<63 - 1)
	if gate.semaphore().TryAcquire(capacity) {
		gate.semaphore().Release(capacity)
		t.Fatal("callback deveria manter o gate compartilhado")
	}
}

func assertGateExclusiveHeld(t *testing.T, gate *DispatchGate) {
	t.Helper()
	if gate.semaphore().TryAcquire(1) {
		gate.semaphore().Release(1)
		t.Fatal("callback deveria manter o gate exclusivo")
	}
}

func assertGateReleased(t *testing.T, gate *DispatchGate) {
	t.Helper()
	const capacity = int64(1<<63 - 1)
	if !gate.semaphore().TryAcquire(capacity) {
		t.Fatal("gate deveria estar liberado")
	}
	gate.semaphore().Release(capacity)
}

// Com um leitor mantendo uma unidade, só um waiter (o writer que solicita
// toda a capacidade) faz TryAcquire(1) falhar: sem waiters, sobra capacidade.
func waitForWriterQueued(t *testing.T, gate *DispatchGate) {
	t.Helper()
	deadline := time.Now().Add(testTimeout)
	for time.Now().Before(deadline) {
		if !gate.semaphore().TryAcquire(1) {
			return
		}
		gate.semaphore().Release(1)
		runtime.Gosched()
	}
	t.Fatal("writer não apareceu na fila do semáforo")
}

type doneObservedContext struct {
	context.Context
	doneSeen chan struct{}
	once     sync.Once
}

func (c *doneObservedContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.doneSeen) })
	return c.Context.Done()
}
