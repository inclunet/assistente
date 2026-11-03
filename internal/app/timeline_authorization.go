package app

import (
	"context"
	"sync"
	"time"

	"assistente/internal/chat"
)

const timelineAuthorizationLifetime = time.Second

// Uma lease é compartilhada pelos emissores de um principal/geração. Somente
// o worker consulta persistência; a admissão de eventos apenas lê o prazo.
type timelineAuthorization struct {
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	mu         sync.Mutex
	validUntil time.Time
	changed    chan struct{}
	validate   func(context.Context) (time.Time, error)
}

func timelineAuthorizationDeadline(start, expiry time.Time) time.Time {
	duration := timelineAuthorizationLifetime
	if remaining := expiry.Sub(start); remaining < duration {
		duration = remaining
	}
	return start.Add(duration)
}

func newTimelineAuthorization(ctx context.Context, validUntil time.Time, validate func(context.Context) (time.Time, error)) *timelineAuthorization {
	ctx, cancel := context.WithCancel(ctx)
	a := &timelineAuthorization{ctx: ctx, cancel: cancel, done: make(chan struct{}), validUntil: validUntil, changed: make(chan struct{}), validate: validate}
	go a.run()
	return a
}

func (a *timelineAuthorization) run() {
	defer close(a.done)
	defer a.cancel()
	for {
		a.mu.Lock()
		deadline := a.validUntil
		a.mu.Unlock()
		timer := time.NewTimer(time.Until(deadline))
		select {
		case <-a.ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
		start := time.Now()
		check, cancel := context.WithTimeout(a.ctx, timelineAuthorizationLifetime)
		expiry, err := a.validate(check)
		checkErr := check.Err()
		cancel()
		deadline = timelineAuthorizationDeadline(start, expiry)
		if err != nil || checkErr != nil || a.ctx.Err() != nil || !time.Now().Before(deadline) {
			return
		}
		a.mu.Lock()
		a.validUntil = deadline
		close(a.changed)
		a.changed = make(chan struct{})
		a.mu.Unlock()
	}
}

// O relógio é consultado mesmo quando o timer do worker ainda não executou.
// Durante a revalidação, um prazo vencido bloqueia a admissão sem iniciar SQL.
func (a *timelineAuthorization) await(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if a.ctx.Err() != nil {
			return chat.ErrTimelineSubscription
		}
		a.mu.Lock()
		valid, changed := time.Now().Before(a.validUntil), a.changed
		a.mu.Unlock()
		if valid {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-a.ctx.Done():
			return chat.ErrTimelineSubscription
		case <-changed:
		}
	}
}

func (a *timelineAuthorization) close() {
	a.cancel()
	<-a.done
}
