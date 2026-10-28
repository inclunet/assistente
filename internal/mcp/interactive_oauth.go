package mcp

import (
	"context"
	"errors"
	"sync"

	"assistente/internal/oauthflow"
)

type interactiveOAuthKey struct{}
type interactiveOAuthScope struct {
	mu       sync.Mutex
	attempts map[string]*interactiveOAuthAttempt
}
type interactiveOAuthAttempt struct {
	done chan struct{}
	err  error
}

// WithInteractiveOAuth permits consent while serving this user action. The
// shared scope remembers cancellation/failure so concurrent tools and later
// iterations of the same turn cannot repeatedly open the browser.
func WithInteractiveOAuth(ctx context.Context) context.Context {
	if _, ok := ctx.Value(interactiveOAuthKey{}).(*interactiveOAuthScope); ok {
		return ctx
	}
	return context.WithValue(ctx, interactiveOAuthKey{}, &interactiveOAuthScope{attempts: make(map[string]*interactiveOAuthAttempt)})
}

// WithoutInteractiveOAuth prevents background work from inheriting desktop consent.
func WithoutInteractiveOAuth(ctx context.Context) context.Context {
	return context.WithValue(ctx, interactiveOAuthKey{}, struct{}{})
}

func (s *interactiveOAuthScope) hasAttempt(cfg ServerConfig) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.attempts[cfg.UserID+":"+cfg.OAuthAuthorizationID]
	return exists
}

func (m *Manager) recoverInteractiveOAuth(ctx context.Context, cfg ServerConfig) error {
	scope, ok := ctx.Value(interactiveOAuthKey{}).(*interactiveOAuthScope)
	if !ok || cfg.AuthType != AuthOAuth2PKCE || managedAttemptFromContext(ctx) != nil {
		return oauthflow.ErrReauthorize
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	key := cfg.UserID + ":" + cfg.OAuthAuthorizationID
	scope.mu.Lock()
	attempt, exists := scope.attempts[key]
	if !exists {
		attempt = &interactiveOAuthAttempt{done: make(chan struct{})}
		scope.attempts[key] = attempt
	}
	scope.mu.Unlock()
	if exists {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-attempt.done:
			return attempt.err
		}
	}
	defer close(attempt.done)
	attempt.err = m.authorizeManagedOAuth(ctx, cfg.Slug, cfg)
	if errors.Is(attempt.err, oauthflow.ErrConflict) {
		// Another explicit action may already own this server's consent. Join its
		// completion instead of starting another flow; never repeat a failed flow.
		m.mu.RLock()
		active := m.managedAttempts[cfg.Slug]
		m.mu.RUnlock()
		if active != nil {
			select {
			case <-ctx.Done():
				attempt.err = ctx.Err()
			case <-active.done:
				_, attempt.err = m.resolveManagedOAuthSilent(ctx, cfg, "")
			}
		}
	}
	return attempt.err
}
