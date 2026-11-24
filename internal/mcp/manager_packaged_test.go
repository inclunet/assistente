package mcp

import (
	"bytes"
	"context"
	"errors"
	"gorm.io/gorm"
	"log/slog"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/userctx"
)

type packagedListRepo struct {
	Repository
	list func(context.Context) ([]ServerConfig, error)
	log  func(*MCPServerLog)
}

func (r packagedListRepo) LogEvent(_ context.Context, entry *MCPServerLog) error {
	r.log(entry)
	return nil
}

func (r packagedListRepo) ListServers(ctx context.Context) ([]ServerConfig, error) {
	return r.list(ctx)
}

func TestPackagedReloadDiscardsSnapshotAfterSessionChange(t *testing.T) {
	alice := userctx.WithSessionID(database.WithUserID(context.Background(), "alice"), "first")
	for _, tc := range []struct {
		name string
		next context.Context
	}{
		{"other account", userctx.WithSessionID(database.WithUserID(context.Background(), "bob"), "second")},
		{"different session of same account", userctx.WithSessionID(alice, "second")},
		{"signed out", context.Background()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var events int
			m := NewManager(nil, nil, func(event string, _ any) {
				if event == "mcp:config_changed" {
					events++
				}
			})
			t.Cleanup(m.CloseAll)
			current := alice
			m.SetAuthContextProvider(func() context.Context { return current })
			const old = "plugin-retiring"
			m.servers[old] = &ServerStatus{ID: "installed", Slug: old, Status: StatusDisconnected, Config: ServerConfig{UserID: "alice", Slug: old}}
			m.SetRepository(packagedListRepo{list: func(context.Context) ([]ServerConfig, error) {
				current = tc.next
				return []ServerConfig{{UserID: "alice", Slug: "plugin-new", ID: "new"}}, nil
			}})
			if err := m.ReloadPackagedConfigs(alice); !errors.Is(err, database.ErrUserScopeRequired) {
				t.Fatalf("expected expired scope, got %v", err)
			}
			if len(m.servers) != 1 || m.servers[old] == nil || events != 0 {
				t.Fatalf("stale reload modified runtime: servers=%v, events=%d", m.servers, events)
			}
		})
	}
}

func TestPackagedRetirementBlocksWaitingConnect(t *testing.T) {
	const slug = "plugin-retiring"
	const id = "first"
	alice := database.WithUserID(context.Background(), "alice")
	inCleanup, release := make(chan struct{}), make(chan struct{})
	m := NewManager(nil, nil, func(event string, _ any) {
		if event == "mcp:server_disconnected" {
			close(inCleanup)
			<-release
		}
	})
	t.Cleanup(m.CloseAll)
	m.SetAuthContextProvider(func() context.Context { return alice })
	m.servers[slug] = &ServerStatus{ID: id, Slug: slug, Status: StatusConnected, Config: ServerConfig{ID: id, UserID: "alice", Slug: slug, Enabled: true}}
	m.connections[slug] = &serverConnection{}
	original := persistedLegacyConfig(m.servers[slug].Config)
	retired := make(chan error, 1)
	go func() {
		_, err := m.disconnectWithRecoveryForAttemptAndOwner(slug, false, true, 0, nil, nil, &original)
		retired <- err
	}()
	<-inCleanup
	wait := &contextDoneObserved{Context: alice, observed: make(chan struct{})}
	connected := make(chan error, 1)
	go func() { connected <- m.connectWithContextForRetry(wait, slug, 0, false) }()
	select {
	case <-wait.observed:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("a conexão não aguardou a limpeza do servidor")
	}
	close(release)
	if err := <-retired; err != nil {
		t.Fatal(err)
	}
	if err := <-connected; err == nil || !strings.Contains(err.Error(), "não encontrado") {
		t.Fatalf("a conexão pendente reencontrou o servidor retirado: %v", err)
	}
}

func TestPackagedReloadReconcilesReusedSlugByServerID(t *testing.T) {
	const slug = "plugin-same-slug"
	alice := database.WithUserID(context.Background(), "alice")
	for _, tc := range []struct {
		name, concurrentID, wantID string
	}{
		{"replaces the old database identity", "", "B"},
		{"keeps the new native identity", "C", "C"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := NewManager(nil, nil, nil)
			t.Cleanup(m.CloseAll)
			m.SetAuthContextProvider(func() context.Context { return alice })
			config := func(id string) ServerConfig {
				return ServerConfig{ID: id, UserID: "alice", Slug: slug, URL: "https://example.org/" + id}
			}
			setRuntime := func(id string) {
				m.mu.Lock()
				defer m.mu.Unlock()
				m.servers[slug] = &ServerStatus{ID: id, Slug: slug, Status: StatusDisconnected, Config: config(id)}
			}
			setRuntime("A")
			m.SetRepository(packagedListRepo{list: func(context.Context) ([]ServerConfig, error) {
				if tc.concurrentID != "" {
					setRuntime(tc.concurrentID)
				}
				return []ServerConfig{config("B")}, nil
			}})
			if err := m.ReloadPackagedConfigs(alice); err != nil {
				t.Fatal(err)
			}
			got := m.servers[slug]
			if got == nil || got.ID != tc.wantID || got.Config.URL != config(tc.wantID).URL {
				t.Fatalf("expected identity %s with its own endpoint; got %+v", tc.wantID, got)
			}
		})
	}
}

func TestPackagedReloadDiscardsReplacementDeletedDuringCleanup(t *testing.T) {
	const slug = "plugin-shared"
	alice := database.WithUserID(context.Background(), "alice")
	replacementExists := true
	m := NewManager(nil, nil, func(event string, _ any) {
		if event == "mcp:server_disconnected" {
			replacementExists = false
		}
	})
	t.Cleanup(m.CloseAll)
	m.SetAuthContextProvider(func() context.Context { return alice })
	a := ServerConfig{Slug: slug, ID: "A", UserID: "alice", Enabled: true}
	m.servers[slug] = &ServerStatus{ID: a.ID, Slug: slug, Config: a, Status: StatusConnected}
	m.connections[slug] = &serverConnection{}
	m.SetRepository(packagedListRepo{
		list: func(context.Context) ([]ServerConfig, error) {
			if replacementExists {
				return []ServerConfig{{Slug: slug, ID: "B", UserID: "alice"}}, nil
			}
			return nil, nil
		},
		log: func(entry *MCPServerLog) { t.Errorf("retirement attributed an event by slug: %+v", entry) },
	})
	if err := m.ReloadPackagedConfigs(alice); err != nil {
		t.Fatal(err)
	}
	if replacementExists || len(m.servers) != 0 {
		t.Fatalf("replacement resurrected: %+v", m.servers)
	}
}

func TestPackagedReloadPreservesConcurrentEditOfSameServer(t *testing.T) {
	const slug = "plugin-edited"
	alice := database.WithUserID(context.Background(), "alice")
	m := NewManager(nil, nil, nil)
	t.Cleanup(m.CloseAll)
	m.SetAuthContextProvider(func() context.Context { return alice })
	original := ServerConfig{Slug: slug, ID: "A", UserID: "alice", URL: "https://example.org/original"}
	m.servers[slug] = &ServerStatus{ID: original.ID, Slug: slug, Config: original, Status: StatusDisconnected}
	const newerURL = "https://example.org/current"
	m.SetRepository(packagedListRepo{list: func(context.Context) ([]ServerConfig, error) {
		stale := original
		stale.URL = "https://example.org/stale"
		m.mu.Lock()
		m.servers[slug].Config.URL = newerURL
		m.mu.Unlock()
		return []ServerConfig{stale}, nil
	}})
	if err := m.ReloadPackagedConfigs(alice); err != nil {
		t.Fatal(err)
	}
	if got := m.servers[slug]; got == nil || got.Config.URL != newerURL {
		t.Fatalf("concurrent edit lost: %+v", got)
	}
}

type missingRegistrationCatalog struct{ recordingUnavailableCatalog }

func (*missingRegistrationCatalog) MarkServerToolsUnavailable(context.Context, string, []string, string) (int, error) {
	return 0, gorm.ErrRecordNotFound
}

func TestPackagedRetirementAlreadyDetachedCatalog(t *testing.T) {
	var logs bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	m := NewManager(nil, nil, nil)
	m.SetCatalog(&missingRegistrationCatalog{})
	ctx := database.WithUserID(context.Background(), "owner")
	m.markServerToolsUnavailableByID(ctx, "removed", "server disconnected", true)
	if logs.Len() != 0 {
		t.Fatalf("unexpected diagnostic after removal: %s", logs.String())
	}
	m.markServerToolsUnavailableByID(ctx, "live", "server disconnected", false)
	if !strings.Contains(logs.String(), "level=ERROR") {
		t.Fatalf("missing diagnostic for live registration: %s", logs.String())
	}
}
