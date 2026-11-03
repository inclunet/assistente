package app

import (
	"assistente/internal/auth"
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/chat"
	"assistente/internal/database"
	"assistente/internal/wailsapi"
)

func TestChatTimelineSessionRequiresCoherentPrincipal(t *testing.T) {
	a := &App{ctx: context.Background()}
	a.setCurrentUserID("user")

	ctx := database.WithUserID(context.Background(), "user")
	if _, _, err := a.captureTimelinePrincipal(ctx); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("sessão ausente aceita", err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: "user", SessionID: "session"})
	captured, _, err := a.captureTimelinePrincipal(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: "user", SessionID: "session", Role: "admin"})
	if _, id, err := captured.AuthorizeTimeline(ctx); err != nil || id != "session" {
		t.Fatal("refresh da mesma sessão invalidou principal", id, err)
	}
	a.setCurrentUserID("other")
	other := database.WithUserID(context.Background(), "other")
	if _, _, err := a.captureTimelinePrincipal(other); !errors.Is(err, chat.ErrTimelineSubscription) {
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
	if _, _, err := a.captureTimelinePrincipal(cancelled); !errors.Is(err, context.Canceled) {
		t.Fatal("captura cancelada aceita", err)
	}
}

const timelineTestUserID = "01991f7c-1000-7000-8000-000000000201"
const timelineTestSessionID = "01991f7c-1000-7000-8000-000000000202"

func newAppTimelineFixture(t *testing.T) (*App, *wailsapi.ChatTimeline, apidto.OpenTimelineRequest, *chat.LiveProjection) {
	t.Helper()
	cache := t.TempDir()
	t.Setenv("LOCALAPPDATA", cache)
	t.Setenv("XDG_CACHE_HOME", cache)
	restore := database.SetDB(database.DB())
	if err := database.InitPath(filepath.Join(t.TempDir(), "app-timeline.db")); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close(); restore() })
	if err := database.DB().Create(&database.User{UUIDModel: database.UUIDModel{ID: timelineTestUserID}, Username: "timeline-user", IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.DB().Create(&database.Session{UUIDModel: database.UUIDModel{ID: timelineTestSessionID}, UserID: timelineTestUserID, RefreshTokenHash: "timeline-hash", ExpiresAt: time.Now().Add(time.Hour)}).Error; err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessionService(database.DB(), auth.SessionConfig{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), timelineTestUserID)
	conversation, err := database.CreateConversationWithContext(ctx, "timeline", "")
	if err != nil {
		t.Fatal(err)
	}
	a := &App{ctx: context.Background(), sessionSvc: sessions}
	api := wailsapi.NewChatTimeline()
	SetChatTimelineAPI(a, api)
	a.setCurrentUserID(timelineTestUserID)
	a.setCurrentAuthUser(&AuthUser{UserID: timelineTestUserID, SessionID: timelineTestSessionID})
	reader := chat.NewDBTimelineReader(database.DB())
	projection := chat.NewLiveProjection(reader)
	wailsapi.AttachChatTimeline(api, timelineWailsSession{wailsSession{app: a}}, projection, reader)
	t.Cleanup(func() { wailsapi.CloseChatTimeline(api) })
	request := apidto.OpenTimelineRequest{ConversationID: conversation.ID, SurfaceID: "surface", TimelineWindowPosition: apidto.TimelineWindowPosition{Anchor: "end", Direction: "before", Limit: 2}}
	return a, api, request, projection
}

func TestChatTimelineAuthChangeDoesNotResurrectOldHandle(t *testing.T) {
	a, api, request, _ := newAppTimelineFixture(t)
	snapshot, err := api.Open(request)
	if err != nil {
		t.Fatal(err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: timelineTestUserID, SessionID: timelineTestSessionID, Role: "admin"})
	snapshot, err = api.Refresh(snapshot.WindowID, apidto.RefreshTimelineRequest{WindowRevision: snapshot.WindowRevision})
	if err != nil {
		t.Fatal("mesma sessão perdeu inscrição", err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: timelineTestUserID, SessionID: "replacement"})
	a.setCurrentAuthUser(&AuthUser{UserID: timelineTestUserID, SessionID: timelineTestSessionID})
	if _, err := api.Refresh(snapshot.WindowID, apidto.RefreshTimelineRequest{WindowRevision: snapshot.WindowRevision}); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("troca de sessão ressuscitou handle", err)
	}
	if _, err := api.Open(request); err != nil {
		t.Fatal("superfície anterior não foi liberada", err)
	}
}

func TestChatTimelinePublicationDoesNotReenterAuthGuard(t *testing.T) {
	a, api, request, projection := newAppTimelineFixture(t)
	if _, err := api.Open(request); err != nil {
		t.Fatal(err)
	}
	a.authMu.RLock()
	release := sync.OnceFunc(a.authMu.RUnlock)
	t.Cleanup(release)
	writerDone := make(chan struct{})
	go func() { a.authMu.Lock(); close(writerDone); a.authMu.Unlock() }()
	deadline := time.Now().Add(5 * time.Second)
	for a.authMu.TryRLock() {
		a.authMu.RUnlock()
		if time.Now().After(deadline) {
			t.Fatal("escritor não entrou na fila")
		}
		runtime.Gosched()
	}
	done := make(chan error, 1)
	ctx := database.WithUserID(context.Background(), timelineTestUserID)
	go func() {
		done <- projection.ApplyStorageMutation(ctx, request.ConversationID, []string{}, func() ([]string, error) {
			message, err := database.CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: request.ConversationID, Role: "user", Content: "publicação sob guard"})
			if err != nil {
				return nil, err
			}
			return []string{"message:" + message.ID}, nil
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		release()
		if err := <-done; err != nil {
			t.Logf("publicação após liberar guard: %v", err)
		}
		t.Fatal("publicação bloqueou no authMu já protegido pelo guard")
	}
	release()
	<-writerDone
}

func TestChatTimelineRPCsRevalidatePersistedSession(t *testing.T) {
	for _, kind := range []string{"revogada", "expirada", "conta inativa"} {
		t.Run(kind, func(t *testing.T) {
			a, api, request, _ := newAppTimelineFixture(t)
			snapshot, err := api.Open(request)
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "revogada":
				err = database.DB().Model(&database.Session{}).Where("id = ?", timelineTestSessionID).Update("revoked_at", time.Now()).Error
			case "expirada":
				err = database.DB().Model(&database.Session{}).Where("id = ?", timelineTestSessionID).Update("expires_at", time.Now().Add(-time.Minute)).Error
			case "conta inativa":
				err = database.DB().Model(&database.User{}).Where("id = ?", timelineTestUserID).Update("is_active", false).Error
			}
			if err != nil {
				t.Fatal(err)
			}
			if a.currentAuthUser.SessionID != timelineTestSessionID {
				t.Fatal("teste alterou estado em memória")
			}
			checks := []func() error{
				func() error { _, err := api.Open(request); return err },
				func() error {
					_, err := api.Refresh(snapshot.WindowID, apidto.RefreshTimelineRequest{WindowRevision: snapshot.WindowRevision})
					return err
				},
				func() error { _, err := api.Navigate(snapshot.WindowID, apidto.TimelineWindowNavigation{}); return err },
				func() error { return api.SetRetention(snapshot.WindowID, apidto.TimelineRetentionRequest{}) },
				func() error {
					_, err := api.Replay(snapshot.WindowID, apidto.ReplayTimelineRequest{AfterSequence: "0"})
					return err
				},
				func() error { return api.Close(snapshot.WindowID) },
			}
			for i, check := range checks {
				if err := check(); !errors.Is(err, auth.ErrUnauthenticatedLocalSession) {
					t.Fatal("RPC aceitou sessão persistida inválida", i, err)
				}
			}
		})
	}
}

func TestChatTimelineCaptureCannotChangeValidatedPrincipal(t *testing.T) {
	a, _, _, _ := newAppTimelineFixture(t)
	adapter := timelineWailsSession{wailsSession{app: a}}
	ctx, err := adapter.AuthenticatedContext()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CaptureTimelineSession(database.WithUserID(context.Background(), timelineTestUserID)); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("captura sem comprovante aceita", err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: timelineTestUserID, SessionID: "replacement"})
	if _, err := adapter.CaptureTimelineSession(ctx); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("sessão não validada herdou captura", err)
	}
	a.setCurrentAuthUser(&AuthUser{UserID: timelineTestUserID, SessionID: timelineTestSessionID})
	if _, err := adapter.CaptureTimelineSession(ctx); !errors.Is(err, chat.ErrTimelineSubscription) {
		t.Fatal("retorno à sessão anterior ressuscitou comprovante", err)
	}
}
