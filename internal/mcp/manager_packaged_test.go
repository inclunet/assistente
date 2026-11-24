package mcp

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/userctx"
)

type packagedListRepo struct {
	Repository
	list func(context.Context) ([]ServerConfig, error)
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
	m.servers[slug] = &ServerStatus{ID: id, Slug: slug, Status: StatusConnected, Config: ServerConfig{UserID: "alice", Slug: slug, Enabled: true}}
	m.connections[slug] = &serverConnection{}
	retired := make(chan error, 1)
	go func() {
		_, err := m.disconnectWithRecoveryForAttemptAndOwner(slug, false, true, 0, nil, nil, id)
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
