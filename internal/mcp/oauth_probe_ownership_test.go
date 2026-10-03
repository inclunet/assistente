package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/oauthflow"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

func TestLegacyOAuthDoesNotReplayUnavailableBody(t *testing.T) {
	for _, brokenFactory := range []bool{false, true} {
		t.Run(map[bool]string{false: "absent", true: "failure"}[brokenFactory], func(t *testing.T) {
			var resources atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`)
					return
				}
				resources.Add(1)
				body, _ := io.ReadAll(r.Body)
				if string(body) != "original-body" {
					t.Error("original body lost")
				}
				w.WriteHeader(http.StatusUnauthorized)
			}))
			defer srv.Close()
			m, ctx, cfg := managedTokenTestFixture(t, srv.URL, "old", "refresh", time.Now().Add(time.Hour))
			client := m.managedHTTPClient(ctx, cfg)
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, io.NopCloser(strings.NewReader("original-body")))
			if brokenFactory {
				req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("private failure") }
			}
			response, err := client.Do(req)
			if response != nil || err == nil || !strings.Contains(err.Error(), "oauth_request_not_replayable") || strings.Contains(err.Error(), "private failure") || resources.Load() != 1 {
				t.Fatalf("unsafe replay: requests=%d err=%v", resources.Load(), err)
			}
		})
	}
}

func TestManagedConnectStopsAfterRefreshPersistenceFailure(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	var tokenCalls, resourceCalls, browsers atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			tokenCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"fresh","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`)
			return
		}
		resourceCalls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	cfg := managedConfig(srv.URL + "/mcp")
	cfg.OAuth2TokenAuthMethod = "none"
	cfg.OAuth2TokenURL = srv.URL + "/token"
	if err := m.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	seedManagedRuntime(t, m, ctx, "legacy", "old", "old-refresh", time.Now().Add(-time.Hour))
	if err := repo.db.Callback().Update().Before("gorm:update").Register("reject_probe_save", func(tx *gorm.DB) {
		if fields, ok := tx.Statement.Dest.(map[string]any); ok && fields["oauth_enc"] != nil && tokenCalls.Load() > 0 {
			_ = tx.AddError(errors.New("disk full"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.db.Callback().Update().Remove("reject_probe_save") })
	oldBrowser := browserOpen
	browserOpen = func(string) error { browsers.Add(1); return nil }
	defer func() { browserOpen = oldBrowser }()
	if err := m.Connect("legacy"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("lost terminal error: %v", err)
	}
	if tokenCalls.Load() != 1 || resourceCalls.Load() != 0 || browsers.Load() != 0 {
		t.Fatalf("continued after failure: tokens=%d resource=%d browser=%d", tokenCalls.Load(), resourceCalls.Load(), browsers.Load())
	}
	_, _, record := loadManaged(t, m, ctx, "legacy")
	if !record.RefreshPending || record.Tokens.Access != "old" {
		t.Fatal("failed rotation was published or lost its recovery barrier")
	}
}

func TestManagedPollingDCRPersistsCallbackForReauthorization(t *testing.T) {
	t.Run("probe", func(t *testing.T) { testLegacyPollingDCR(t, false) })
	t.Run("handshake", func(t *testing.T) { testLegacyPollingDCR(t, true) })
}

func testLegacyPollingDCR(t *testing.T, handshakeFallback bool) {
	m, repo, ctx := managedFixture(t)
	var registrations, authorizations atomic.Int32
	var redirect atomic.Value
	redirect.Store("")
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "legacy-polling", Version: "1"}, nil)
	handler := mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/register":
			registrations.Add(1)
			var metadata oauthflow.RegistrationRequest
			if err := json.NewDecoder(r.Body).Decode(&metadata); err != nil {
				t.Error(err)
				w.WriteHeader(400)
				return
			}
			if len(metadata.RedirectURIs) != 1 {
				t.Error("missing callback")
				w.WriteHeader(400)
				return
			}
			redirect.Store(metadata.RedirectURIs[0])
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"client_id":"registered"}`)
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("redirect_uri") != redirect.Load().(string) {
				t.Error("callback changed after registration")
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"access","token_type":"Bearer","expires_in":3600}`)
		case "/authorize":
			w.WriteHeader(http.StatusOK)
		case "/mcp":
			if r.Method == http.MethodGet {
				if handshakeFallback {
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(http.StatusOK)
					return
				}
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			if r.Header.Get("Authorization") != "Bearer access" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			handler.ServeHTTP(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	defer m.CloseAll()
	if handshakeFallback {
		var attempts atomic.Int32
		m.transportFactory = func(connectCtx context.Context, slug string, config ServerConfig) (mcpsdk.Transport, error) {
			if attempts.Add(1) == 1 {
				return &delayedErrorTransport{err: errors.New("standalone SSE request failed")}, nil
			}
			projected, err := m.GetConfig(slug)
			if err != nil || projected.OAuth2CallbackPort == 0 || projected.OAuth2ClientID != "registered" || !config.DisableSSE {
				t.Error("fallback lost the latest OAuth metadata or polling preference")
			}
			clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
			session, err := server.Connect(m.ctx, serverTransport, nil)
			if err != nil {
				return nil, err
			}
			t.Cleanup(func() { _ = session.Close() })
			return clientTransport, nil
		}
	}
	cfg := managedConfig(srv.URL + "/mcp")
	cfg.OAuth2ClientID, cfg.OAuth2TokenAuthMethod = "", "none"
	cfg.OAuth2TokenURL, cfg.OAuth2AuthURL, cfg.OAuth2RegistrationURL = srv.URL+"/token", srv.URL+"/authorize", srv.URL+"/register"
	cfg.OAuth2CallbackHost = "127.0.0.1"
	if err := m.SaveConfig("legacy", cfg); err != nil {
		t.Fatal(err)
	}
	oldBrowser := browserOpen
	defer func() { browserOpen = oldBrowser }()
	browserOpen = func(raw string) error {
		authorizations.Add(1)
		u, err := url.Parse(raw)
		if err != nil {
			return err
		}
		q := u.Query()
		response, err := http.Get(q.Get("redirect_uri") + "?state=" + url.QueryEscape(q.Get("state")) + "&code=code")
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, response.Body)
		return response.Body.Close()
	}
	if err := m.Connect("legacy"); err != nil {
		t.Fatal(err)
	}
	persisted, err := repo.GetServer(ctx, "legacy")
	if err != nil || persisted.OAuth2CallbackPort != 0 || persisted.OAuthAuthorizationID == "" || persisted.DisableSSE != handshakeFallback {
		t.Fatalf("lost callback or persisted transient probe flag: %v", err)
	}
	_, _, record := loadManaged(t, m, ctx, "legacy")
	if record.Callback.Port == 0 || record.Client.ID != "registered" {
		t.Fatal("registration not stored in composed authorization")
	}
	if err := m.ReauthorizeServer(ctx, "legacy"); err != nil {
		t.Fatal(err)
	}
	if registrations.Load() != 1 || authorizations.Load() != 2 {
		t.Fatalf("registration=%d authorization=%d", registrations.Load(), authorizations.Load())
	}
}
