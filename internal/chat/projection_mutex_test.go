package chat

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type observedLockContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *observedLockContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}
func waitProjectionTest(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("operação não terminou")
	}
}

func TestTimelineReconciliationCancelsBeforeWriterReleases(t *testing.T) {
	sub, session, _, reference, snapshot := reconciliationFixture(t)
	c, release := sub.owner.lockConversation(sub.key, false)
	releaseOnce := sync.OnceFunc(release)
	defer releaseOnce()
	parent, cancel := context.WithCancel(session.ctx)
	defer cancel()
	ctx := &observedLockContext{Context: parent, waiting: make(chan struct{})}
	finished := make(chan struct{})
	var readErr error
	go func() {
		_, readErr = sub.ReconcileRecovery(ctx, reference, reconciliationReader(func(context.Context, string, []string) (*TimelineSelectionSnapshot, error) {
			return nil, errors.New("reader não deve ser chamado enquanto writer está ativo")
		}))
		close(finished)
	}()
	waitProjectionTest(t, ctx.waiting)
	sub.owner.mu.Lock()
	reserved := c.readers
	sub.owner.mu.Unlock()
	if reserved != 2 {
		t.Fatalf("reservas antes do cancelamento: %d", reserved)
	}
	cancel()
	waitProjectionTest(t, finished) // writer ainda possui a barreira
	if !errors.Is(readErr, context.Canceled) {
		t.Fatal(readErr)
	}
	sub.owner.mu.Lock()
	reserved = c.readers
	sub.owner.mu.Unlock()
	if reserved != 1 {
		t.Fatalf("reserva abandonada: %d", reserved)
	}
	releaseOnce()
	result, err := sub.ReconcileRecovery(session.ctx, reference, reconciliationReader(func(context.Context, string, []string) (*TimelineSelectionSnapshot, error) { return snapshot, nil }))
	if err != nil || result.State != "active" {
		t.Fatal("barreira indisponível após cancelamento", result, err)
	}
}

func TestProjectionMutexCancellationDoesNotLoseToken(t *testing.T) {
	var mutex projectionMutex
	for range 100 {
		mutex.Lock()
		parent, cancel := context.WithCancel(context.Background())
		ctx := &observedLockContext{Context: parent, waiting: make(chan struct{})}
		done := make(chan struct{})
		go func() {
			if mutex.LockContext(ctx) == nil {
				mutex.Unlock()
			}
			close(done)
		}()
		waitProjectionTest(t, ctx.waiting)
		cancel()
		mutex.Unlock()
		waitProjectionTest(t, done)
		probe, stop := context.WithTimeout(context.Background(), time.Second)
		err := mutex.LockContext(probe)
		stop()
		if err != nil {
			t.Fatal("token perdido", err)
		}
		mutex.Unlock()
	}
}

func TestProjectionCancelledLastReservationStillCleansUp(t *testing.T) {
	for _, withTimer := range []bool{false, true} {
		t.Run(map[bool]string{false: "without_timer", true: "retention_timer"}[withTimer], func(t *testing.T) {
			testCancelledLastReservation(t, withTimer)
		})
	}
}

func testCancelledLastReservation(t *testing.T, withTimer bool) {
	t.Helper()
	p := NewLiveProjection(nil)
	key := projectionKey{"user", "conversation"}
	c := newProjectionConversation()
	p.conversations[key] = c
	if withTimer {
		p.mu.Lock()
		p.armCleanupLocked(key, c, projectionRetention)
		p.mu.Unlock()
	}
	c.mu.Lock()
	unlock := sync.OnceFunc(c.mu.Unlock)
	defer unlock()
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	ctx := &observedLockContext{Context: parent, waiting: make(chan struct{})}
	done := make(chan struct{})
	var lockErr error
	go func() {
		_, release, err := p.lockConversationContext(ctx, key, false)
		release()
		lockErr = err
		close(done)
	}()
	waitProjectionTest(t, ctx.waiting)
	cancel()
	waitProjectionTest(t, done)
	if !errors.Is(lockErr, context.Canceled) {
		t.Fatal(lockErr)
	}
	unlock()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		p.mu.Lock()
		exists := p.conversations[key] != nil
		p.mu.Unlock()
		if !exists {
			break
		}
		select {
		case <-tick.C:
		case <-timeout.C:
			t.Fatal("cancelamento perdeu cleanup")
		}
	}
}
