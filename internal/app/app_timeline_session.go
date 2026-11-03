package app

import (
	"context"

	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/wailsapi"
)

func SetChatTimelineAPI(a *App, api *wailsapi.ChatTimeline) { a.chatTimelineAPI = api }

type capturedTimelineSession struct {
	app               *App
	userID, sessionID string
}

func (s wailsSession) CaptureTimelineSession(ctx context.Context) (chat.TimelineSession, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.app.authMu.RLock()
	defer s.app.authMu.RUnlock()
	u := s.app.currentAuthUser
	if u == nil || u.SessionID == "" || u.UserID != userID || s.app.currentUserID != userID {
		return nil, chat.ErrTimelineSubscription
	}
	return capturedTimelineSession{app: s.app, userID: userID, sessionID: u.SessionID}, nil
}

func (s capturedTimelineSession) AuthorizeTimeline(ctx context.Context) (context.Context, string, error) {
	userID, err := database.RequireUserID(ctx)
	if err != nil {
		return nil, "", err
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	s.app.authMu.RLock()
	defer s.app.authMu.RUnlock()
	u := s.app.currentAuthUser
	if userID != s.userID || s.app.currentUserID != s.userID || u == nil || u.UserID != s.userID || u.SessionID != s.sessionID {
		return nil, "", chat.ErrTimelineSubscription
	}
	return ctx, s.sessionID, nil
}
