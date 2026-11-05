package app

import (
	"context"
	"time"

	"assistente/internal/auth"
	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/wailsapi"
)

func SetChatTimelineAPI(a *App, api *wailsapi.ChatTimeline) { a.chatTimelineAPI = api }

type timelineWailsSession struct{ wailsSession }
type timelinePrincipalKey struct{}
type timelinePrincipalProof struct {
	principal  capturedTimelineSession
	sessions   *auth.SessionService
	validUntil time.Time
}
type capturedTimelineSession struct {
	app               *App
	userID, sessionID string
	generation        uint64
}

// A consulta persistida ocorre somente na entrada do RPC, fora dos locks do App
// e do projetor. O comprovante privado vincula Capture ao principal revalidado.
func (s timelineWailsSession) AuthenticatedContext() (context.Context, error) {
	return s.AuthenticatedTimelineContext(context.Background())
}

func (s timelineWailsSession) AuthenticatedTimelineContext(parent context.Context) (context.Context, error) {
	if err := parent.Err(); err != nil {
		return nil, err
	}
	ctx, err := s.wailsSession.AuthenticatedContext()
	if err != nil {
		return nil, err
	}
	captured, sessions, err := s.app.captureTimelinePrincipal(ctx)
	if err != nil {
		return nil, err
	}
	principal := auth.LocalSessionPrincipal{UserID: captured.userID, SessionID: captured.sessionID}
	check, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	stop := context.AfterFunc(parent, cancel)
	defer stop()
	if err := parent.Err(); err != nil {
		return nil, err
	}
	start := time.Now()
	validated, expiry, err := sessions.RevalidateLocalSessionExpiry(check, principal)
	if err != nil {
		return nil, err
	}
	if validated != principal {
		return nil, chat.ErrTimelineSubscription
	}
	if _, _, err := captured.AuthorizeTimeline(ctx); err != nil {
		return nil, err
	}
	return context.WithValue(ctx, timelinePrincipalKey{}, timelinePrincipalProof{captured, sessions, timelineAuthorizationDeadline(start, expiry)}), nil
}

func (s timelineWailsSession) CaptureTimelineSession(ctx context.Context) (chat.TimelineSession, error) {
	proof, ok := ctx.Value(timelinePrincipalKey{}).(timelinePrincipalProof)
	if !ok || proof.principal.app != s.app {
		return nil, chat.ErrTimelineSubscription
	}
	captured := proof.principal
	if _, _, err := captured.AuthorizeTimeline(ctx); err != nil {
		return nil, err
	}
	return captured, nil
}

func (a *App) captureTimelinePrincipal(ctx context.Context) (capturedTimelineSession, *auth.SessionService, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return capturedTimelineSession{}, nil, err
	}
	if err := ctx.Err(); err != nil {
		return capturedTimelineSession{}, nil, err
	}
	a.authMu.RLock()
	defer a.authMu.RUnlock()
	u := a.currentAuthUser
	if u == nil || u.SessionID == "" || u.UserID != userID || a.currentUserID != userID {
		return capturedTimelineSession{}, nil, chat.ErrTimelineSubscription
	}
	return capturedTimelineSession{app: a, userID: userID, sessionID: u.SessionID, generation: a.timelineAuthGeneration.Load()}, a.sessionSvc, nil
}

// Publicação pode executar dentro de um guard que já detém authMu.RLock.
// Somente a geração atômica é consultada aqui: sem reentrada de lock ou SQL.
func (s capturedTimelineSession) AuthorizeTimeline(ctx context.Context) (context.Context, string, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if userID != s.userID || s.app.timelineAuthGeneration.Load() != s.generation {
		return nil, "", chat.ErrTimelineSubscription
	}
	return ctx, s.sessionID, nil
}
