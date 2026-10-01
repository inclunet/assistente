package mcp

import (
	"bytes"
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

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"assistente/internal/tools"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"
)

func managedFixture(t *testing.T) (*Manager, *DBRepository, context.Context) {
	t.Helper()
	repo, ctx, _ := setupRepositoryTest(t)
	if err := repo.db.AutoMigrate(&database.CredentialEntry{}); err != nil {
		t.Fatal(err)
	}
	mgr := NewManager(tools.NewRegistry(), credentials.NewManagerWithStore(bytes.Repeat([]byte{7}, 32), credentials.NewDBStore(), true), func(string, any) {})
	mgr.SetRepository(repo)
	mgr.SetAuthContextProvider(func() context.Context { return ctx })
	t.Cleanup(mgr.CloseAll)
	return mgr, repo, ctx
}
func managedConfig(resource string) ServerConfig {
	return ServerConfig{Name: "Managed", Transport: TransportStreamable, URL: resource, AuthType: AuthOAuth2PKCE, OAuthManaged: true, OAuth2ClientID: "client", OAuth2TokenURL: resource + "/token", OAuth2AuthURL: resource + "/authorize", Enabled: true}
}
func loadManaged(t *testing.T, m *Manager, ctx context.Context, slug string) (ServerConfig, oauthflow.Store, oauthflow.Record) {
	t.Helper()
	cfg, err := m.GetConfig(slug)
	if err != nil {
		t.Fatal(err)
	}
	store, r, _, err := m.managedOAuth(ctx, *cfg)
	if err != nil {
		t.Fatal(err)
	}
	return *cfg, store, r
}

func TestManagedOAuthOneEncryptedEntryAndAtomicConsumer(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	cfg := managedConfig("https://resource.example")
	if err := m.SaveConfigWithOAuthSecret("new", cfg, "CLIENT-SECRET"); err != nil {
		t.Fatal(err)
	}
	projected, _, r := loadManaged(t, m, ctx, "new")
	if projected.OAuth2ClientID != "client" || r.Client.Secret != "CLIENT-SECRET" {
		t.Fatal("configuration lost")
	}
	var rows []database.CredentialEntry
	if err := repo.db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Source != "oauth" || strings.Contains(rows[0].OAuthEnc, "CLIENT-SECRET") {
		t.Fatal("not one encrypted entry")
	}
	stored, err := repo.GetServer(ctx, "new")
	if err != nil {
		t.Fatal(err)
	}
	if stored.OAuth2ClientID != "" || stored.OAuth2TokenURL != "" || stored.OAuthAuthorizationID != r.ID {
		t.Fatal("duplicated OAuth configuration")
	}
	other := database.WithUserID(context.Background(), "user-b")
	if _, err = m.resolveManagedOAuth(other, *stored, ""); err == nil {
		t.Fatal("cross-user resolution")
	}
	forged := *stored
	forged.URL = "https://other.example"
	if _, err = m.resolveManagedOAuth(ctx, forged, ""); err == nil {
		t.Fatal("cross-resource resolution")
	}
	if err := repo.db.Callback().Create().Before("gorm:create").Register("reject_managed_consumer", func(tx *gorm.DB) {
		if tx.Statement.Table == "mcp_servers" {
			_ = tx.AddError(errors.New("consumer write failed"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = repo.db.Callback().Create().Remove("reject_managed_consumer") }()
	if err := m.SaveConfigWithOAuthSecret("fail", cfg, "FAILED-SECRET"); err == nil {
		t.Fatal("expected atomic create failure")
	}
	var count int64
	repo.db.Model(&database.CredentialEntry{}).Count(&count)
	if count != 1 {
		t.Fatal("orphan authorization after failed create")
	}
	if _, ok := m.servers["fail"]; ok {
		t.Fatal("published failed consumer")
	}
}

func TestManagedOAuthLegacyIsNotMigratedOrUsedAsFallback(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	legacy := managedConfig("https://resource.example")
	legacy.OAuthManaged = false
	if err := m.SaveConfig("legacy", legacy); err != nil {
		t.Fatal(err)
	}
	legacy.OAuthManaged = true
	if err := m.SaveConfig("legacy", legacy); err == nil {
		t.Fatal("implicit migration")
	}
	if err := m.SaveConfig("new", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ := loadManaged(t, m, ctx, "new")
	if err := m.credMgr.RegisterPatternWithContext(ctx, userTokensPattern("new"), &credentials.AuthConfig{Source: "static", Type: "oauth2", Token: "LEGACY"}); err != nil {
		t.Fatal(err)
	}
	tok, ok := m.resolveNativeAuthToken(ctx, nativeMCPCandidate{slug: "new", name: "new", url: cfg.URL, authType: cfg.AuthType, managedConfig: cfg})
	if ok || tok != "" {
		t.Fatal("managed authorization fell back to legacy token")
	}
	old, err := repo.GetServer(ctx, "legacy")
	if err != nil || old.OAuthAuthorizationID != "" {
		t.Fatal("legacy changed")
	}
}

func TestManagedOAuthPKCEDCRPersistsCallbackAndRefreshAfterRestart(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	var tokenCalls atomic.Int32
	var redirect string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/register":
			var body oauthflow.RegistrationRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.RedirectURIs) != 1 {
				t.Error("missing exact callback")
				w.WriteHeader(400)
				return
			}
			redirect = body.RedirectURIs[0]
			_ = json.NewEncoder(w).Encode(map[string]string{"client_id": "registered"})
		case "/token":
			_ = r.ParseForm()
			if r.Header.Get("Authorization") != "" || r.Form.Get("client_id") != "registered" {
				t.Error("public client used Basic or omitted client_id")
			}
			n := tokenCalls.Add(1)
			if n == 1 && r.Form.Get("redirect_uri") != redirect {
				t.Error("callback differs from registration")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "ACCESS-" + r.Form.Get("grant_type"), "refresh_token": "REFRESH", "token_type": "Bearer", "expires_in": 3600})
		case "/authorize":
			w.WriteHeader(200)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cfg := managedConfig(srv.URL)
	cfg.OAuth2ClientID = ""
	cfg.OAuth2TokenAuthMethod = "client_secret_basic"
	cfg.OAuth2RegistrationURL = srv.URL + "/register"
	if err := m.SaveConfig("new", cfg); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ = loadManaged(t, m, ctx, "new")
	oldBrowser := browserOpen
	defer func() { browserOpen = oldBrowser }()
	browserOpen = func(raw string) error {
		u, _ := url.Parse(raw)
		q := u.Query()
		callback := q.Get("redirect_uri") + "?state=" + url.QueryEscape(q.Get("state")) + "&code=CODE"
		resp, err := http.Get(callback)
		if err != nil {
			return err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return nil
	}
	if err := m.authorizeManagedOAuth(ctx, "new", cfg); err != nil {
		t.Fatal(err)
	}
	cfg, store, r := loadManaged(t, m, ctx, "new")
	if r.Client.ID != "registered" || r.Tokens.Refresh != "REFRESH" || r.Callback.Port == 0 || r.State != "connected" {
		t.Fatalf("grant not persisted: %s", r.State)
	}
	r.Tokens.ExpiresAt = time.Now().Add(-time.Hour)
	r.Revision++
	if err := store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
		t.Fatal(err)
	}
	restarted := NewManager(tools.NewRegistry(), credentials.NewManagerWithStore(bytes.Repeat([]byte{7}, 32), credentials.NewDBStore(), true), func(string, any) {})
	restarted.SetRepository(repo)
	restarted.SetAuthContextProvider(func() context.Context { return ctx })
	defer restarted.CloseAll()
	if err := restarted.LoadConfigs(); err != nil {
		t.Fatal(err)
	}
	tok, ok := restarted.resolveNativeAuthToken(ctx, nativeMCPCandidate{slug: "new", name: "new", managedConfig: cfg})
	if !ok || tok != "ACCESS-refresh_token" || tokenCalls.Load() != 2 {
		t.Fatalf("restart/native refresh failed: ok=%v calls=%d", ok, tokenCalls.Load())
	}
	var count int64
	repo.db.Model(&database.CredentialEntry{}).Count(&count)
	if count != 1 {
		t.Fatal("legacy credential pair was created")
	}
}

func TestManagedOAuthDeviceAndStartupNeverOpenBrowserImplicitly(t *testing.T) {
	m, _, ctx := managedFixture(t)
	var opened atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/.well-known/oauth-protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": "http://" + r.Host + "/canonical", "authorization_servers": []string{"http://" + r.Host}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": "http://" + r.Host, "authorization_endpoint": "http://" + r.Host + "/authorize", "token_endpoint": "http://" + r.Host + "/token", "device_authorization_endpoint": "http://" + r.Host + "/device"})
		case "/device":
			_ = r.ParseForm()
			if r.Form.Get("resource") != "http://"+r.Host+"/canonical" {
				t.Error("device ignored canonical audience")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "DEVICE", "user_code": "USER", "verification_uri": "http://" + r.Host + "/verify", "expires_in": 60, "interval": 1})
		case "/token":
			_ = r.ParseForm()
			if r.Form.Get("resource") != "http://"+r.Host+"/canonical" {
				t.Error("token ignored canonical audience")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "device-token", "token_type": "Bearer", "expires_in": 3600})
		case "/authorize", "/verify":
			w.WriteHeader(200)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	cfg := managedConfig(srv.URL)
	cfg.OAuth2DeviceAuthURL = srv.URL + "/device"
	cfg.AutoConnect = true
	if err := m.SaveConfig("new", cfg); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ = loadManaged(t, m, ctx, "new")
	oldBrowser := browserOpen
	defer func() { browserOpen = oldBrowser }()
	browserOpen = func(string) error { opened.Add(1); return nil }
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if _, err := m.managedHTTPClient(ctx, cfg).Do(req); err == nil {
		t.Fatal("pending grant unexpectedly connected")
	}
	if opened.Load() != 0 {
		t.Fatal("silent resolution opened browser")
	}
	if err := m.authorizeManagedOAuth(ctx, "new", cfg); err != nil {
		t.Fatal(err)
	}
	projected, _, r := loadManaged(t, m, ctx, "new")
	if r.Tokens.Access != "device-token" || opened.Load() != 1 || r.Audience != srv.URL+"/canonical" {
		t.Fatal("device authorization not committed")
	}
	projected.Name = "Renamed"
	if err := m.SaveConfig("new", projected); err != nil {
		t.Fatal(err)
	}
	_, _, edited := loadManaged(t, m, ctx, "new")
	if edited.Tokens.Access != "device-token" || edited.Endpoints.Device != r.Endpoints.Device {
		t.Fatal("name edit discarded authorization")
	}

}

func TestManagedOAuthClientCredentialsAndDeleteFenceTransport(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	var tokens, requests atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			tokens.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "CC", "token_type": "Bearer", "expires_in": 3600})
			return
		}
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer CC" {
			t.Error("missing bearer")
		}
		w.WriteHeader(200)
	}))
	defer srv.Close()
	cfg := managedConfig(srv.URL)
	cfg.AuthType = AuthOAuth2ClientCredentials
	if err := m.SaveConfig("cc", cfg); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveServerAuth("cc", string(cfg.AuthType), "", "", "", "secret"); err != nil {
		t.Fatal(err)
	}
	cfg, _, _ = loadManaged(t, m, ctx, "cc")
	client := m.managedHTTPClient(ctx, cfg)
	for range 2 {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	if tokens.Load() != 1 {
		t.Fatal("client grant was not cached")
	}
	if err := m.DeleteConfig("cc"); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	if resp, err := client.Do(req); err == nil {
		_ = resp.Body.Close()
		t.Fatal("old transport survived deletion")
	}
	var count int64
	repo.db.Model(&database.CredentialEntry{}).Count(&count)
	if count != 0 || requests.Load() != 2 {
		t.Fatal("deletion did not remove/fence authorization")
	}
}

func TestManagedOAuthEditsRefuseLiveLeasesAndRollbackConsumerFailure(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	if err := m.SaveConfig("new", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}
	cfg, store, r := loadManaged(t, m, ctx, "new")
	r.State = "connected"
	r.Tokens = oauthflow.Tokens{Access: "OLD", Type: "Bearer"}
	r.AuthorizationAttempt = "active"
	r.AuthorizationUntil = time.Now().Add(time.Minute)
	r.Revision++
	if err := store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
		t.Fatal(err)
	}
	cfg.Name = "Changed"
	if err := m.SaveConfig("new", cfg); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("edit active lease: %v", err)
	}
	if err := m.DeleteConfig("new"); !errors.Is(err, oauthflow.ErrConflict) {
		t.Fatalf("delete active lease: %v", err)
	}
	r.AuthorizationAttempt = ""
	r.AuthorizationUntil = time.Time{}
	r.Revision++
	if err := store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Callback().Update().Before("gorm:update").Register("reject_consumer_edit", func(tx *gorm.DB) {
		if tx.Statement.Table == "mcp_servers" {
			_ = tx.AddError(errors.New("consumer edit failed"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = repo.db.Callback().Update().Remove("reject_consumer_edit") }()
	cfg.OAuth2Scopes = []string{"expanded"}
	if err := m.SaveConfig("new", cfg); err == nil {
		t.Fatal("expected atomic edit failure")
	}
	_, _, after := loadManaged(t, m, ctx, "new")
	if after.Revision != r.Revision || after.Tokens.Access != "OLD" || len(after.RequestedScopes) != 0 {
		t.Fatal("failed consumer edit changed envelope")
	}
}

type managedTestRoundTrip func(*http.Request) (*http.Response, error)

func (f managedTestRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type managedTrackedBody struct {
	io.Reader
	closed bool
}

func (b *managedTrackedBody) Close() error { b.closed = true; return nil }
func TestManagedOAuthReplayFailureCloses401And403DoesNotRefresh(t *testing.T) {
	for _, status := range []int{401, 403} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			m, _, ctx := managedFixture(t)
			var tokens atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tokens.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "NEW", "refresh_token": "ROTATED", "token_type": "Bearer", "expires_in": 3600})
			}))
			defer srv.Close()
			if err := m.SaveConfig("new", managedConfig(srv.URL)); err != nil {
				t.Fatal(err)
			}
			cfg, store, r := loadManaged(t, m, ctx, "new")
			r.State = "connected"
			r.Tokens = oauthflow.Tokens{Access: "OLD", Refresh: "REFRESH", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}
			r.Revision++
			if err := store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
				t.Fatal(err)
			}
			body := &managedTrackedBody{Reader: strings.NewReader("rejected")}
			transport := &managedOAuthTransport{manager: m, cfg: cfg, store: store, ctx: ctx, base: managedTestRoundTrip(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: status, Body: body, Header: make(http.Header)}, nil
			})}
			req, _ := http.NewRequestWithContext(ctx, http.MethodPost, srv.URL, strings.NewReader("payload"))
			req.GetBody = func() (io.ReadCloser, error) { return nil, errors.New("cannot recreate body") }
			resp, err := transport.RoundTrip(req)
			if status == 401 {
				if err == nil || !body.closed || tokens.Load() != 1 {
					t.Fatalf("401 leaked body or did not renew: %v %v %d", err, body.closed, tokens.Load())
				}
			} else {
				if err != nil || resp.StatusCode != 403 || tokens.Load() != 0 {
					t.Fatalf("403 refreshed: %v", err)
				}
				_ = resp.Body.Close()
			}
		})
	}
}

func TestManagedOAuthTransportCannotSurviveVaultSession(t *testing.T) {
	m, _, ctx := managedFixture(t)
	if err := m.SaveConfig("new", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}
	cfg, store, r := loadManaged(t, m, ctx, "new")
	r.State = "connected"
	r.Tokens = oauthflow.Tokens{Access: "OLD", Type: "Bearer"}
	r.Revision++
	if err := store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
		t.Fatal(err)
	}
	var requests int
	transport := &managedOAuthTransport{manager: m, cfg: cfg, store: store, ctx: ctx, base: managedTestRoundTrip(func(*http.Request) (*http.Response, error) { requests++; return nil, errors.New("must not send") })}
	m.credMgr.ClearCommandCache() // Also invalidates the shared OAuth session epoch.
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, cfg.URL, nil)
	if _, err := transport.RoundTrip(req); err == nil || requests != 0 {
		t.Fatal("transport survived invalidated vault session")
	}
}

func TestManagedOAuthReservedSlugIsAtomic(t *testing.T) {
	m, repo, _ := managedFixture(t)
	if err := m.SaveConfig("native", managedConfig("https://resource.example")); err == nil {
		t.Fatal("reserved namespace accepted")
	}
	var count int64
	if err := repo.db.Model(&database.CredentialEntry{}).Count(&count).Error; err != nil || count != 0 {
		t.Fatalf("orphan authorization: %d %v", count, err)
	}
}

func TestManagedOAuthDisconnectCancelsRefreshPreflight(t *testing.T) {
	m, _, ctx := managedFixture(t)
	started := make(chan struct{})
	var resources atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			close(started)
			select {
			case <-r.Context().Done():
			case <-time.After(10 * time.Second):
			}
			return
		}
		resources.Add(1)
	}))
	defer srv.Close()
	if err := m.SaveConfig("cancel", managedConfig(srv.URL)); err != nil {
		t.Fatal(err)
	}
	_, store, record := loadManaged(t, m, ctx, "cancel")
	record.State = "connected"
	record.Tokens = oauthflow.Tokens{Access: "OLD", Refresh: "REFRESH", Type: "Bearer", ExpiresAt: time.Now().Add(-time.Hour)}
	record.Revision++
	if err := store.CompareAndSwap(ctx, record, record.Revision-1); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- m.Connect("cancel") }()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("refresh did not start")
	}
	_ = m.Disconnect("cancel")
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("connect survived cancellation")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("refresh was not canceled")
	}
	if resources.Load() != 0 {
		t.Fatal("resource connected after cancellation")
	}
}

func TestManagedOAuthSSEFallbackPreservesAuthorization(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	probe := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
	}))
	defer probe.Close()
	if err := m.SaveConfig("polling", managedConfig(probe.URL)); err != nil {
		t.Fatal(err)
	}
	_, store, record := loadManaged(t, m, ctx, "polling")
	record.State = "connected"
	record.Tokens = oauthflow.Tokens{Access: "VALID", Refresh: "REFRESH", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}
	record.Revision++
	if err := store.CompareAndSwap(ctx, record, record.Revision-1); err != nil {
		t.Fatal(err)
	}
	server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "fallback-test", Version: "1"}, nil)
	var calls atomic.Int32
	m.transportFactory = func(context.Context, string, ServerConfig) (mcpsdk.Transport, error) {
		if calls.Add(1) == 1 {
			return &delayedErrorTransport{err: errors.New("standalone SSE request failed")}, nil
		}
		clientTransport, serverTransport := mcpsdk.NewInMemoryTransports()
		session, err := server.Connect(m.ctx, serverTransport, nil)
		if err != nil {
			return nil, err
		}
		t.Cleanup(func() { _ = session.Close() })
		return clientTransport, nil
	}
	if err := m.Connect("polling"); err != nil {
		t.Fatal(err)
	}
	defer m.CloseAll()
	latest, err := store.Load(ctx, record.ID)
	if err != nil || latest.Revision != record.Revision || (latest.Tokens.Access != record.Tokens.Access || latest.Tokens.Refresh != record.Tokens.Refresh || latest.Tokens.Type != record.Tokens.Type || !latest.Tokens.ExpiresAt.Equal(record.Tokens.ExpiresAt)) {
		t.Fatalf("fallback changed authorization: %v", err)
	}
	cfg, err := repo.GetServer(ctx, "polling")
	if err != nil || !cfg.DisableSSE || calls.Load() != 2 {
		t.Fatalf("fallback not persisted: calls=%d err=%v", calls.Load(), err)
	}
}

func TestManagedOAuthRenamePreservesDiscoveredAudience(t *testing.T) {
	m, _, ctx := managedFixture(t)
	if err := m.SaveConfig("audience", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}
	cfg, store, record := loadManaged(t, m, ctx, "audience")
	record.Audience = "https://canonical.example/resource"
	record.State = "connected"
	record.Tokens = oauthflow.Tokens{Access: "VALID", Type: "Bearer"}
	record.Revision++
	if err := store.CompareAndSwap(ctx, record, record.Revision-1); err != nil {
		t.Fatal(err)
	}
	cfg.Name = "Renamed"
	if err := m.SaveConfig("audience", cfg); err != nil {
		t.Fatal(err)
	}
	latest, err := store.Load(ctx, record.ID)
	if err != nil || latest.Audience != record.Audience || latest.Tokens.Access != "VALID" || latest.State != "connected" {
		t.Fatalf("rename invalidated discovered resource: %v", err)
	}
}

func TestManagedOAuthDetachFailurePreservesAuthorization(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(map[bool]string{false: "detach", true: "delete"}[remove], func(t *testing.T) {
			m, repo, ctx := managedFixture(t)
			if err := m.SaveConfig("atomic", managedConfig("https://resource.example")); err != nil {
				t.Fatal(err)
			}
			cfg, store, record := loadManaged(t, m, ctx, "atomic")
			record.State = "connected"
			record.Tokens = oauthflow.Tokens{Access: "VALID", Refresh: "REFRESH", Type: "Bearer"}
			record.Revision++
			if err := store.CompareAndSwap(ctx, record, record.Revision-1); err != nil {
				t.Fatal(err)
			}
			reject := func(tx *gorm.DB) {
				if tx.Statement.Table == "mcp_servers" {
					_ = tx.AddError(errors.New("consumer failed"))
				}
			}
			if remove {
				if err := repo.db.Callback().Delete().Before("gorm:delete").Register("reject_detach", reject); err != nil {
					t.Fatal(err)
				}
				defer func() { _ = repo.db.Callback().Delete().Remove("reject_detach") }()
			} else {
				if err := repo.db.Callback().Update().Before("gorm:update").Register("reject_detach", reject); err != nil {
					t.Fatal(err)
				}
				defer func() { _ = repo.db.Callback().Update().Remove("reject_detach") }()
			}
			var err error
			if remove {
				err = m.DeleteConfig("atomic")
			} else {
				cfg.AuthType = AuthNone
				err = m.SaveConfig("atomic", cfg)
			}
			if err == nil {
				t.Fatal("consumer failure ignored")
			}
			latest, err := store.Load(ctx, record.ID)
			if err != nil || latest.Revision != record.Revision || latest.Tokens.Access != "VALID" || latest.Tokens.Refresh != "REFRESH" || latest.State != "connected" {
				t.Fatalf("failed detach destroyed authorization: %v", err)
			}
			persisted, err := repo.GetServer(ctx, "atomic")
			if err != nil || persisted.OAuthAuthorizationID != record.ID || persisted.AuthType != AuthOAuth2PKCE {
				t.Fatalf("consumer changed after rollback: %v", err)
			}
		})
	}
}

func TestManagedOAuthAuthInfoAfterRemovingSecret(t *testing.T) {
	m, _, ctx := managedFixture(t)
	if err := m.SaveConfig("info", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}
	if _, has, err := m.GetServerAuthInfo("info"); err != nil || has {
		t.Fatalf("public ID shown as secret: %v", err)
	}
	if err := m.SaveServerAuth("info", string(AuthOAuth2PKCE), "", "", "", "SECRET"); err != nil {
		t.Fatal(err)
	}
	if _, has, err := m.GetServerAuthInfo("info"); err != nil || !has {
		t.Fatalf("secret not shown: %v", err)
	}
	if err := m.DeleteServerAuth("info"); err != nil {
		t.Fatal(err)
	}
	if _, has, err := m.GetServerAuthInfo("info"); err != nil || has {
		t.Fatalf("removed secret shown configured: %v", err)
	}
	_, _, record := loadManaged(t, m, ctx, "info")
	if record.Client.ID != "client" {
		t.Fatal("public registration was lost")
	}
}

func TestManagedOAuthRemoveSecretFailureIsAtomic(t *testing.T) {
	m, repo, ctx := managedFixture(t)
	if err := m.SaveConfigWithOAuthSecret("clear", managedConfig("https://resource.example"), "SECRET"); err != nil {
		t.Fatal(err)
	}
	_, store, record := loadManaged(t, m, ctx, "clear")
	record.State = "connected"
	record.Tokens = oauthflow.Tokens{Access: "VALID", Type: "Bearer"}
	record.Revision++
	if err := store.CompareAndSwap(ctx, record, record.Revision-1); err != nil {
		t.Fatal(err)
	}
	if err := repo.db.Callback().Update().Before("gorm:update").Register("reject_clear", func(tx *gorm.DB) {
		if tx.Statement.Table == "credential_entries" {
			_ = tx.AddError(errors.New("write failed"))
		}
	}); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = repo.db.Callback().Update().Remove("reject_clear") }()
	if err := m.DeleteServerAuth("clear"); err == nil {
		t.Fatal("write failure ignored")
	}
	latest, err := store.Load(ctx, record.ID)
	if err != nil || latest.Revision != record.Revision || latest.Tokens.Access != "VALID" || latest.Client.Secret != "SECRET" || latest.State != "connected" {
		t.Fatalf("partial clear: %v", err)
	}
}

func TestManagedOAuthPublishesWorkspaceRoots(t *testing.T) {
	m, _, _ := managedFixture(t)
	if err := m.SetWorkspaceRoots([]Root{{URI: "file:///first", Name: "first"}}); err != nil {
		t.Fatal(err)
	}
	if err := m.SaveConfig("roots", managedConfig("https://resource.example")); err != nil {
		t.Fatal(err)
	}
	assertRoot := func(slug, uri string) {
		t.Helper()
		m.mu.RLock()
		defer m.mu.RUnlock()
		status := m.servers[slug]
		if status == nil || len(status.Roots) != 1 || status.Roots[0].URI != uri {
			t.Fatalf("missing roots for %s", slug)
		}
	}
	assertRoot("roots", "file:///first")
	if err := m.SetWorkspaceRoots([]Root{{URI: "file:///second", Name: "second"}}); err != nil {
		t.Fatal(err)
	}
	cfg, err := m.GetConfig("roots")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Name = "Updated"
	if err = m.SaveConfig("roots", *cfg); err != nil {
		t.Fatal(err)
	}
	assertRoot("roots", "file:///second")
	copySlug, err := m.DuplicateConfig("roots")
	if err != nil {
		t.Fatal(err)
	}
	assertRoot(copySlug, "file:///second")
}

func TestManagedOAuthSessionExpiryTriggersBridgeRecovery(t *testing.T) {
	for _, status := range []int{404, 410} {
		for _, retry := range []bool{false, true} {
			t.Run(http.StatusText(status)+map[bool]string{false: "/initial", true: "/retry"}[retry], func(t *testing.T) {
				m, _, ctx := managedFixture(t)
				var tokenCalls atomic.Int32
				srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					tokenCalls.Add(1)
					_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "NEW", "refresh_token": "ROTATED", "token_type": "Bearer", "expires_in": 3600})
				}))
				defer srv.Close()
				if err := m.SaveConfig("expired", managedConfig(srv.URL)); err != nil {
					t.Fatal(err)
				}
				cfg, store, record := loadManaged(t, m, ctx, "expired")
				record.State = "connected"
				record.Tokens = oauthflow.Tokens{Access: "OLD", Refresh: "REFRESH", Type: "Bearer", ExpiresAt: time.Now().Add(time.Hour)}
				record.Revision++
				if err := store.CompareAndSwap(ctx, record, record.Revision-1); err != nil {
					t.Fatal(err)
				}
				var bodies []*managedTrackedBody
				transport := &managedOAuthTransport{manager: m, cfg: cfg, store: store, ctx: ctx, base: managedTestRoundTrip(func(*http.Request) (*http.Response, error) {
					code := status
					if retry && len(bodies) == 0 {
						code = 401
					}
					body := &managedTrackedBody{Reader: strings.NewReader("expired")}
					bodies = append(bodies, body)
					return &http.Response{StatusCode: code, Body: body, Header: make(http.Header)}, nil
				})}
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
				resp, err := transport.RoundTrip(req)
				var expired *SessionExpiredError
				if resp != nil || !errors.As(err, &expired) || expired.StatusCode != status || !isSessionOrTransportError(err) {
					t.Fatalf("bridge recovery not signaled: %v", err)
				}
				for _, body := range bodies {
					if !body.closed {
						t.Fatal("session error leaked response")
					}
				}
				expected := int32(0)
				if retry {
					expected = 1
				}
				if tokenCalls.Load() != expected {
					t.Fatal("session expiry triggered OAuth renewal")
				}
			})
		}
	}
}

func TestManagedOAuthRegistrationMetadataInvalidatesDCR(t *testing.T) {
	edits := map[string]func(*ServerConfig){
		"callback":     func(c *ServerConfig) { c.OAuth2CallbackPort = 12345 },
		"scopes":       func(c *ServerConfig) { c.OAuth2Scopes = []string{"read", "write"} },
		"resource":     func(c *ServerConfig) { c.URL = "https://other.example" },
		"registration": func(c *ServerConfig) { c.OAuth2RegistrationURL = "https://resource.example/new-registration" },
		"token":        func(c *ServerConfig) { c.OAuth2TokenURL = "https://resource.example/new-token" },
		"rename":       func(c *ServerConfig) { c.Name = "Renamed" },
	}
	for _, method := range []string{"dcr", "manual"} {
		for name, edit := range edits {
			t.Run(method+"/"+name, func(t *testing.T) {
				m, _, ctx := managedFixture(t)
				cfg := managedConfig("https://resource.example")
				if err := m.SaveConfig("new", cfg); err != nil {
					t.Fatal(err)
				}
				cfg, store, r := loadManaged(t, m, ctx, "new")
				r.Client.Method = method
				r.Client.Secret = "registered-secret"
				r.Revision++
				if err := store.CompareAndSwap(ctx, r, r.Revision-1); err != nil {
					t.Fatal(err)
				}
				edit(&cfg)
				if err := m.SaveConfig("new", cfg); err != nil {
					t.Fatal(err)
				}
				_, _, got := loadManaged(t, m, ctx, "new")
				if method == "dcr" && name != "rename" {
					if got.Client.ID != "" || got.Client.Secret != "" {
						t.Fatal("stale DCR registration preserved")
					}
				} else if got.Client.ID != r.Client.ID {
					t.Fatal("unchanged or manual registration lost")
				}
			})
		}
	}
}
