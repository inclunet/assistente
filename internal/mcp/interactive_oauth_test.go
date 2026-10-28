package mcp

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/oauthflow"
)

func TestInteractiveOAuthRecoversBeforeResourceRequestAndDeduplicates(t *testing.T) {
	for _, scenario := range []string{"success", "denied", "insufficient_scopes"} {
		t.Run(scenario, func(t *testing.T) {
			denied := scenario == "denied"
			m, repo, ctx := managedFixture(t)
			sqlDB, err := repo.db.DB()
			if err != nil {
				t.Fatal(err)
			}
			sqlDB.SetMaxOpenConns(1)
			m.SetOAuthNetworkAuthorizer(func(_ context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
				return d.IPs, true, nil
			})
			var tokens, resources, browsers atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					http.NotFound(w, r)
					return
				}
				if r.URL.Path == "/token" {
					tokens.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"access_token":"ACCESS","refresh_token":"REFRESH","token_type":"Bearer","expires_in":3600}`)
					return
				}
				resources.Add(1)
				if r.Header.Get("Authorization") != "Bearer ACCESS" {
					t.Error("resource received invalid credential")
				}
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()
			if err := m.SaveConfig("interactive", managedConfig(server.URL)); err != nil {
				t.Fatal(err)
			}
			cfg, store, record := loadManaged(t, m, ctx, "interactive")
			expectedError := oauthflow.ErrReauthorize
			if scenario == "insufficient_scopes" {
				record.State = "connected"
				record.RequestedScopes = []string{"read"}
				record.Tokens = oauthflow.Tokens{Access: "OLD", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}
				record.Revision++
				if err := store.CompareAndSwap(ctx, record, record.Revision-1); err != nil {
					t.Fatal(err)
				}
				expectedError = oauthflow.ErrPermission
			}
			opened, release := make(chan struct{}), make(chan struct{})
			originalBrowser := browserOpen
			defer func() { browserOpen = originalBrowser }()
			browserOpen = func(raw string) error {
				if browsers.Add(1) == 1 {
					close(opened)
				}
				select {
				case <-release:
				case <-time.After(5 * time.Second):
					return errors.New("consent test timed out")
				}
				u, _ := url.Parse(raw)
				query := u.Query()
				callback := query.Get("redirect_uri") + "?state=" + url.QueryEscape(query.Get("state")) + "&code=CODE"
				if denied {
					callback = query.Get("redirect_uri") + "?state=" + url.QueryEscape(query.Get("state")) + "&error=access_denied"
				}
				response, err := http.Get(callback)
				if err != nil {
					return err
				}
				return response.Body.Close()
			}
			client := m.buildAuthHTTPClient(ctx, cfg.Slug, cfg)
			call := func(callCtx context.Context) error {
				req, err := http.NewRequestWithContext(callCtx, http.MethodPost, cfg.URL, nil)
				if err != nil {
					return err
				}
				resp, err := client.Do(req)
				if resp != nil {
					_ = resp.Body.Close()
				}
				return err
			}
			if err := call(ctx); !errors.Is(err, expectedError) || browsers.Load() != 0 || resources.Load() != 0 {
				t.Fatalf("background consent or request: %v", err)
			}
			turn := WithInteractiveOAuth(ctx)
			first, second := make(chan error, 1), make(chan error, 1)
			go func() { first <- call(turn) }()
			select {
			case <-opened:
			case <-time.After(5 * time.Second):
				t.Fatal("consent did not start")
			}
			go func() { second <- call(turn) }()
			close(release)
			for _, done := range []chan error{first, second} {
				select {
				case err := <-done:
					if denied && (err == nil || !strings.Contains(err.Error(), "oauth_consent_declined")) {
						t.Fatalf("expected declined consent: %v", err)
					}
					if !denied && err != nil {
						t.Fatal(err)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("concurrent authorization deadlocked")
				}
			}
			if browsers.Load() != 1 {
				t.Fatalf("browser opens=%d", browsers.Load())
			}
			if denied {
				if err := call(turn); err == nil || browsers.Load() != 1 || resources.Load() != 0 || tokens.Load() != 0 {
					t.Fatalf("denial retried: %v", err)
				}
			} else if tokens.Load() != 1 || resources.Load() != 2 {
				t.Fatalf("token/resource calls %d/%d", tokens.Load(), resources.Load())
			}
		})
	}
}

func TestInteractiveOAuthDoesNotReplayConcurrentUnauthorizedRequests(t *testing.T) {
	for _, sharedTurn := range []bool{true, false} {
		t.Run(map[bool]string{true: "same_turn", false: "different_turns"}[sharedTurn], func(t *testing.T) {
			m, repo, ctx := managedFixture(t)
			sqlDB, err := repo.db.DB()
			if err != nil {
				t.Fatal(err)
			}
			sqlDB.SetMaxOpenConns(1)
			m.SetOAuthNetworkAuthorizer(func(_ context.Context, d oauthflow.NetworkDestination) ([]net.IP, bool, error) {
				return d.IPs, true, nil
			})
			var resources, browsers atomic.Int32
			bothSent, firstCompleted := make(chan struct{}), make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					http.NotFound(w, r)
					return
				}
				if r.URL.Path == "/token" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"access_token":"NEW","token_type":"Bearer","expires_in":3600}`)
					return
				}
				n := resources.Add(1)
				if n == 2 {
					close(bothSent)
				}
				wait := bothSent
				if n == 2 {
					wait = firstCompleted
				}
				select {
				case <-wait:
				case <-time.After(5 * time.Second):
					t.Error("request synchronization timed out")
				}
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer server.Close()
			if err := m.SaveConfig("concurrent", managedConfig(server.URL)); err != nil {
				t.Fatal(err)
			}
			seedManagedRuntime(t, m, ctx, "concurrent", "OLD", "", time.Now().Add(time.Hour))
			cfg, _, _ := loadManaged(t, m, ctx, "concurrent")
			oldBrowser := browserOpen
			defer func() { browserOpen = oldBrowser }()
			browserOpen = func(raw string) error {
				browsers.Add(1)
				u, _ := url.Parse(raw)
				q := u.Query()
				response, err := http.Get(q.Get("redirect_uri") + "?state=" + url.QueryEscape(q.Get("state")) + "&code=CODE")
				if err != nil {
					return err
				}
				return response.Body.Close()
			}
			turn := WithInteractiveOAuth(ctx)
			client := m.buildAuthHTTPClient(ctx, cfg.Slug, cfg)
			done := make(chan error, 2)
			for range 2 {
				callCtx := turn
				if !sharedTurn {
					callCtx = WithInteractiveOAuth(ctx)
				}
				go func() {
					req, _ := http.NewRequestWithContext(callCtx, http.MethodPost, cfg.URL, strings.NewReader("operation"))
					response, err := client.Do(req)
					if response != nil {
						_ = response.Body.Close()
					}
					done <- err
				}()
			}
			for i := 0; i < 2; i++ {
				select {
				case err := <-done:
					if i == 0 {
						close(firstCompleted)
					}
					if err == nil || !strings.Contains(err.Error(), "oauth_request_not_replayable") {
						t.Fatalf("operation replayed or wrong error: %v", err)
					}
				case <-time.After(10 * time.Second):
					t.Fatal("requests did not finish")
				}
			}
			if resources.Load() != 2 || browsers.Load() != 1 {
				t.Fatalf("resource calls=%d browser opens=%d", resources.Load(), browsers.Load())
			}

		})
	}
}

func TestWithoutInteractiveOAuthMasksInheritedConsent(t *testing.T) {
	m, _, ctx := managedFixture(t)
	if err := m.SaveConfig("background", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ := loadManaged(t, m, ctx, "background")
	interactive := WithInteractiveOAuth(ctx)
	background := WithoutInteractiveOAuth(context.WithoutCancel(interactive))
	if _, ok := background.Value(interactiveOAuthKey{}).(*interactiveOAuthScope); ok {
		t.Fatal("background inherited consent")
	}
	if err := m.recoverInteractiveOAuth(background, cfg); !errors.Is(err, oauthflow.ErrReauthorize) {
		t.Fatal(err)
	}
	if _, ok := interactive.Value(interactiveOAuthKey{}).(*interactiveOAuthScope); !ok {
		t.Fatal("parent scope lost")
	}
}

func TestConnectWithContextPreservesExistingConnection(t *testing.T) {
	for _, access := range []string{"VALID", ""} {
		t.Run("access_"+access, func(t *testing.T) {
			m, _, ctx := managedFixture(t)
			if err := m.SaveConfig("connected", managedConfig("https://resource.example")); err != nil {
				t.Fatal(err)
			}
			seedManagedRuntime(t, m, ctx, "connected", access, "", time.Now().Add(time.Hour))
			conn := &serverConnection{}
			m.connections["connected"] = conn
			defer delete(m.connections, "connected")
			if err := m.ConnectWithContext(ctx, "connected"); err != nil {
				t.Fatal(err)
			}
			turn := WithInteractiveOAuth(ctx)
			if err := m.ConnectWithContext(turn, "connected"); err != nil {
				t.Fatal(err)
			}
			cfg, _, _ := loadManaged(t, m, ctx, "connected")
			if turn.Value(interactiveOAuthKey{}).(*interactiveOAuthScope).hasAttempt(cfg) {
				t.Fatal("connect opened unnecessary consent")
			}
			if m.connections["connected"] != conn {
				t.Fatal("existing connection replaced")
			}
		})
	}
}

func TestNativeOAuthIgnoresServersOutsideSelectedTools(t *testing.T) {
	m, _, ctx := managedFixture(t)
	if err := m.SaveConfig("unselected", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}
	m.servers["unselected"].Status = StatusConnected
	m.servers["unselected"].Tools = []MCPToolInfo{{Name: "query", FullName: "query"}}
	turn := WithInteractiveOAuth(ctx)
	for _, enabled := range [][]string{{}, {"other"}} {
		if got := m.GetEligibleNativeMCPServers(turn, enabled); len(got) != 0 {
			t.Fatal("unselected server included")
		}
	}
	cfg, _, _ := loadManaged(t, m, ctx, "unselected")
	scope := turn.Value(interactiveOAuthKey{}).(*interactiveOAuthScope)
	if scope.hasAttempt(cfg) || m.servers["unselected"].NeedsReauth {
		t.Fatal("unselected server resolved authorization")
	}
}
