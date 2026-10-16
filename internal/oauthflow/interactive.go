package oauthflow

import "context"

// Interactive serializes browser consent across integrations, including legacy MCP.
var Interactive = &Arbiter{gate: make(chan struct{}, 1)}

type Arbiter struct{ gate chan struct{} }

func (a *Arbiter) Lock()   { a.gate <- struct{}{} }
func (a *Arbiter) Unlock() { <-a.gate }
func (a *Arbiter) Acquire(ctx context.Context) (func(), error) {
	select {
	case a.gate <- struct{}{}:
		return a.Unlock, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func sessionContext(ctx context.Context, store Store) (context.Context, context.CancelFunc) {
	if scoped, ok := store.(interface {
		SessionContext(context.Context) (context.Context, context.CancelFunc)
	}); ok {
		return scoped.SessionContext(ctx)
	}
	return context.WithCancel(ctx)
}
