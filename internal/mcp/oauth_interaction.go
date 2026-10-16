package mcp

import (
	"context"
	"sync"
	"time"
)

type oauthInteractionKey struct{}
type oauthInteraction struct {
	mu      sync.Mutex
	active  int
	since   time.Time
	elapsed time.Duration
	changed chan struct{}
}

func withOAuthInteraction(ctx context.Context) context.Context {
	return context.WithValue(ctx, oauthInteractionKey{}, &oauthInteraction{changed: make(chan struct{})})
}
func beginOAuthInteraction(ctx context.Context) func() {
	s, _ := ctx.Value(oauthInteractionKey{}).(*oauthInteraction)
	if s == nil {
		return func() {}
	}
	s.mu.Lock()
	if s.active == 0 {
		s.since = time.Now()
	}
	s.active++
	close(s.changed)
	s.changed = make(chan struct{})
	s.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.active--
			if s.active == 0 {
				s.elapsed += time.Since(s.since)
			}
			close(s.changed)
			s.changed = make(chan struct{})
		})
	}
}
func (s *oauthInteraction) snapshot() (bool, time.Duration, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	elapsed := s.elapsed
	if s.active > 0 {
		elapsed += time.Since(s.since)
	}
	return s.active > 0, elapsed, s.changed
}

// The SDK keeps its session context. Only the handshake budget pauses during
// OAuth interaction; parent cancellation and Disconnect remain effective.
func oauthHandshakeContext(parent, session context.Context, budget time.Duration) (context.Context, context.CancelFunc) {
	s, _ := session.Value(oauthInteractionKey{}).(*oauthInteraction)
	if s == nil {
		return context.WithTimeout(parent, budget)
	}
	ctx, cancel := context.WithCancelCause(parent)
	started := time.Now()
	_, initialPause, _ := s.snapshot()
	go func() {
		timer := time.NewTimer(budget)
		defer timer.Stop()
		for {
			active, paused, changed := s.snapshot()
			remaining := budget - (time.Since(started) - (paused - initialPause))
			if !active && remaining <= 0 {
				cancel(context.DeadlineExceeded)
				return
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			var deadline <-chan time.Time
			if !active {
				timer.Reset(remaining)
				deadline = timer.C
			}
			select {
			case <-ctx.Done():
				return
			case <-session.Done():
				cancel(session.Err())
				return
			case <-changed:
			case <-deadline:
			}
		}
	}()
	return handshakeBudgetContext{ctx}, func() { cancel(context.Canceled) }
}

type handshakeBudgetContext struct{ context.Context }

func (c handshakeBudgetContext) Err() error {
	if c.Context.Err() != nil {
		return context.Cause(c.Context)
	}
	return nil
}
