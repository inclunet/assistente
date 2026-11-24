package mcp

import (
	"context"
	"errors"
	"testing"

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
