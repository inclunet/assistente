package app

import (
	"context"
	"sync"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/auth"
	"assistente/internal/chat"
	"assistente/internal/wailsapi"
)

type timelineSharedAuthorization struct {
	lease *timelineAuthorization
	refs  int // protegido por timelineAuthorizationMu
}

type timelineWailsEmitter struct {
	principal capturedTimelineSession
	shared    *timelineSharedAuthorization
	once      sync.Once
}

func (s timelineWailsSession) CaptureTimelineEmitter(ctx context.Context) (context.Context, wailsapi.TimelineEventEmitter, error) {
	proof, ok := ctx.Value(timelinePrincipalKey{}).(timelinePrincipalProof)
	if !ok || proof.principal.app != s.app || s.app.emitter == nil {
		return nil, nil, chat.ErrTimelineSubscription
	}
	if _, _, err := proof.principal.AuthorizeTimeline(ctx); err != nil {
		return nil, nil, err
	}
	a := s.app
	a.timelineAuthorizationMu.Lock()
	if a.timelineAuthorizations == nil {
		a.timelineAuthorizations = make(map[capturedTimelineSession]*timelineSharedAuthorization)
	}
	shared := a.timelineAuthorizations[proof.principal]
	if shared == nil || shared.lease.ctx.Err() != nil {
		shared = &timelineSharedAuthorization{lease: newTimelineAuthorization(ctx, proof.validUntil, func(check context.Context) (time.Time, error) {
			if _, _, err := proof.principal.AuthorizeTimeline(check); err != nil {
				return time.Time{}, err
			}
			_, expiry, err := proof.sessions.RevalidateLocalSessionExpiry(check, auth.LocalSessionPrincipal{UserID: proof.principal.userID, SessionID: proof.principal.sessionID})
			if err == nil {
				_, _, err = proof.principal.AuthorizeTimeline(check)
			}
			return expiry, err
		})}
		a.timelineAuthorizations[proof.principal] = shared
	}
	shared.refs++
	a.timelineAuthorizationMu.Unlock()
	return shared.lease.ctx, &timelineWailsEmitter{principal: proof.principal, shared: shared}, nil
}

func (e *timelineWailsEmitter) Emit(ctx context.Context, event apidto.TimelineEvent) error {
	if err := e.shared.lease.await(ctx); err != nil {
		return err
	}
	if _, _, err := e.principal.AuthorizeTimeline(ctx); err != nil {
		return err
	}
	e.principal.app.emitter.Emit(apidto.TimelineEventName, event)
	return nil
}

func (e *timelineWailsEmitter) Close() {
	e.once.Do(func() {
		a := e.principal.app
		a.timelineAuthorizationMu.Lock()
		e.shared.refs--
		last := e.shared.refs == 0
		if last && a.timelineAuthorizations[e.principal] == e.shared {
			delete(a.timelineAuthorizations, e.principal)
		}
		a.timelineAuthorizationMu.Unlock()
		if last {
			e.shared.lease.close()
		}
	})
}
