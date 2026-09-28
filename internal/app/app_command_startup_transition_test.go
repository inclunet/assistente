package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type startupTransitionEmitter struct {
	*testEmitter
	onPublished func()
}

// Done é consultado na aquisição da barreira. A notificação prova que o
// segundo caller chegou à aquisição enquanto o primeiro ainda a possui.
type startupAttemptContext struct {
	context.Context
	once      sync.Once
	attempted chan struct{}
}

func (c *startupAttemptContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.attempted) })
	return c.Context.Done()
}

func (e *startupTransitionEmitter) Emit(event string, data any) {
	e.testEmitter.Emit(event, data)
	if event == "command:keyboard-map-changed" {
		e.onPublished()
	}
}

func TestCommandJobsConcurrentRetriesOwnPreparationThroughStart(t *testing.T) {
	a, observer := commandJobsStartupFixture(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var unblockOnce sync.Once
	unblock := func() { unblockOnce.Do(func() { close(release) }) }
	var publications atomic.Int32
	var missingOwnership atomic.Bool
	a.emitter = &startupTransitionEmitter{testEmitter: &testEmitter{}, onPublished: func() {
		if len(a.commandStartup) != 1 {
			missingOwnership.Store(true)
		}
		if publications.Add(1) == 1 {
			close(entered)
			<-release
		}
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	a.ctx = ctx
	done := make(chan error, 2)
	run := func(attemptCtx context.Context) {
		result, err := a.retryUserRuntimeInit(attemptCtx)
		if err == nil && len(result.Subsystems) != 0 {
			err = errors.New("retry retornou subsistemas indisponíveis")
		}
		done <- err
	}
	launched, received := 1, 0
	t.Cleanup(func() {
		unblock()
		cancel()
		for received < launched {
			select {
			case <-done:
				received++
			case <-time.After(5 * time.Second):
				t.Error("retry não terminou após cancelamento")
				return
			}
		}
	})
	go run(ctx)
	select {
	case <-entered:
	case <-ctx.Done():
		t.Fatal("primeiro retry não publicou")
	}
	// Publicação já ocorreu, mas Start ainda não: este era o intervalo vulnerável.
	if observer.starts.Load() != 0 || len(a.commandStartup) != 1 {
		t.Fatal("transição liberada entre preparação e Start")
	}
	launched++
	secondCtx := &startupAttemptContext{Context: ctx, attempted: make(chan struct{})}
	go run(secondCtx)
	select {
	case <-secondCtx.attempted:
	case <-ctx.Done():
		t.Fatal("segundo retry não chegou à aquisição concorrente")
	}
	unblock()
	for received < launched {
		select {
		case err := <-done:
			received++
			if err != nil {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal("retries concorrentes não terminaram")
		}
	}
	if missingOwnership.Load() || observer.starts.Load() != 2 || observer.beforeReady.Load() || a.commandJobsPending != nil {
		t.Fatalf("sequência inválida: ownership=%v starts=%d beforeReady=%v", !missingOwnership.Load(), observer.starts.Load(), observer.beforeReady.Load())
	}
}

func TestCommandJobsOSUnlockWaitingForStartupIsCancelable(t *testing.T) {
	a, observer := commandJobsStartupFixture(t)
	prepareCommandJobsResume(t, a)
	if err := a.lockCommandStartup(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer a.unlockCommandStartup()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		a.bootstrapCommandLifecycleAfterOSUnlock(ctx, a.commandHost)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("worker cancelado ficou preso atrás da transição de startup")
	}
	if observer.starts.Load() != 0 || a.commandJobsPending == nil {
		t.Fatal("worker cancelado iniciou ou consumiu jobs pendentes")
	}
}

func TestCommandJobsLateOSUnlockKeepsAlreadyPublishedGeneration(t *testing.T) {
	a, observer := commandJobsStartupFixture(t)
	if result, err := a.RetryUserRuntimeInit(); err != nil || len(result.Subsystems) != 0 {
		t.Fatalf("retry: %+v %v", result, err)
	}
	principal, err := a.currentCommandPrincipal()
	if err != nil {
		t.Fatal(err)
	}
	before, err := a.commandHost.Snapshot(context.Background(), principal)
	if err != nil {
		t.Fatal(err)
	}
	a.bootstrapCommandLifecycleAfterOSUnlock(context.Background(), a.commandHost)
	after, err := a.commandHost.Snapshot(context.Background(), principal)
	if err != nil || before != after || observer.starts.Load() != 1 {
		t.Fatalf("unlock tardio refez publicação válida: before=%+v after=%+v starts=%d err=%v", before, after, observer.starts.Load(), err)
	}
}
