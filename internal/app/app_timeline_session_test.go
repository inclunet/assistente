package app

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/wailsapi"
)

func TestChatTimelineSessionRequiresCoherentPrincipal(t *testing.T) {
	a := &App{ctx: context.Background()}
	a.setCurrentUserID("user")
	adapter := wailsSession{app: a}
	ctx := database.WithUserID(context.Background(), "user")
	if _, err := adapter.CaptureTimelineSession(ctx); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("sessão ausente aceita", err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: "user", SessionID: "session"})
	captured, err := adapter.CaptureTimelineSession(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: "user", SessionID: "session", Role: "admin"})
	if _, id, err := captured.AuthorizeTimeline(ctx); err != nil || id != "session" {
		t.Fatal("refresh da mesma sessão invalidou principal", id, err)
	}
	a.setCurrentUserID("other")
	other := database.WithUserID(context.Background(), "other")
	if _, err := adapter.CaptureTimelineSession(other); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("estado intermediário de login aceito", err)
	}
	if _, _, err := captured.AuthorizeTimeline(ctx); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("principal antigo aceito", err)
	}
	a.setCurrentUserID("user")
	a.setCurrentAuthUser(&AuthUser{UserID: "user", SessionID: "new-session"})
	if _, _, err := captured.AuthorizeTimeline(ctx); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("nova sessão herdou principal", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := adapter.CaptureTimelineSession(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("captura cancelada aceita", err)
	}
}

func TestChatTimelineAuthChangeDoesNotResurrectOldHandle(t *testing.T) {
	cache := t.TempDir()
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
	restore := database.SetDB(database.DB())
	if err := database.InitPath(filepath.Join(t.TempDir(), "app-timeline.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(); restore() })
	if err := database.DB().Create(&database.User{UUIDModel: database.UUIDModel{ID: "timeline-user"}, Username: "timeline-user"}).Error; err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), "timeline-user")
	conversation, err := database.CreateConversationWithContext(ctx, "timeline", "")
	if err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background()}
	api := wailsapi.NewChatTimeline()
	SetChatTimelineAPI(a, api)
	a.setCurrentUserID("timeline-user")
	a.setCurrentAuthUser(&AuthUser{UserID: "timeline-user", SessionID: "session"})
	reader := chat.NewDBTimelineReader(database.DB())
	wailsapi.AttachChatTimeline(api, wailsSession{app: a}, chat.NewLiveProjection(reader), reader)
	t.Cleanup(func() { wailsapi.CloseChatTimeline(api) })
	request := apidto.OpenTimelineRequest{ConversationID: conversation.ID, SurfaceID: "surface", TimelineWindowPosition: apidto.TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 2}}
	snapshot, err := api.Open(request)
	if err != nil {
		t.Fatal(err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: "timeline-user", SessionID: "session", Role: "admin"})
	snapshot, err = api.Refresh(snapshot.WindowID, apidto.RefreshTimelineRequest{WindowRevision: snapshot.WindowRevision})
	if err != nil {
		t.Fatal("mesma sessão perdeu inscrição", err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: "timeline-user", SessionID: "replacement"})
	a.setCurrentAuthUser(&AuthUser{UserID: "timeline-user", SessionID: "session"})
	if _, err := api.Refresh(snapshot.WindowID, apidto.RefreshTimelineRequest{WindowRevision: snapshot.WindowRevision}); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("troca de sessão ressuscitou handle", err)
	}
	if _, err := api.Open(request); err != nil {
		t.Fatal("superfície anterior não foi liberada", err)
	}
}
