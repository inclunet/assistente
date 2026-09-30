package credentials

import (
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOAuthTransportReusesRotatesAndRestrictsDestination(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	calls, refreshes := 0, 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/token":
			refreshes++
			_, _ = fmt.Fprint(w, `{"access_token":"fresh","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`)
		case "/v1/responses":
			calls++
			if r.Header.Get("Authorization") == "Bearer old" {
				w.WriteHeader(401)
				return
			}
			if r.Header.Get("Authorization") != "Bearer fresh" {
				t.Error("wrong authorization")
			}
			_, _ = fmt.Fprint(w, "ok")
		default:
			t.Errorf("unauthorized destination %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	integration := oauthflow.Integration{ID: "test", Issuer: server.URL, Resource: server.URL + "/v1", Endpoints: oauthflow.Endpoints{Token: server.URL + "/token"}, RequiredScopes: []string{"invoke"}, Routes: []oauthflow.Route{{Method: "POST", Path: "/responses"}}}
	service := oauthflow.New(integration)
	mgr := NewManagerWithStore(bytes.Repeat([]byte{2}, 32), NewDBStore(), true)
	mgr.SetOAuthService(service)
	ctx := database.WithUserID(context.Background(), "owner")
	store, _ := mgr.OAuthStore(ctx)
	record, _ := service.Pending("transport", "owner", "test")
	record.State = "connected"
	record.GrantedScopes = []string{"invoke"}
	record.Client.ID = "client"
	record.Tokens = oauthflow.Tokens{Access: "old", Refresh: "original", Type: "Bearer"}
	if err := store.Create(ctx, record); err != nil {
		t.Fatal(err)
	}
	client := NewHTTPClientWithAuthMode(mgr, "oauth:transport", AuthRequired, 0)
	for range 2 {
		req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/v1/responses", strings.NewReader(`{}`))
		response, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if string(body) != "ok" {
			t.Fatalf("body=%s", body)
		}
	}
	if calls != 3 || refreshes != 1 {
		t.Fatalf("calls=%d refreshes=%d", calls, refreshes)
	}
	for _, path := range []string{"/v1/audio/speech", "/v1/../other"} {
		req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+path, nil)
		if resp, err := client.Do(req); err == nil {
			_ = resp.Body.Close()
			t.Fatal("unapproved route")
		}
	}
	req, _ := http.NewRequestWithContext(ctx, "POST", "https://example.invalid/v1/responses", nil)
	if resp, err := client.Do(req); err == nil {
		_ = resp.Body.Close()
		t.Fatal("cross-origin authorization")
	}
	if calls != 3 || refreshes != 1 {
		t.Fatal("unexpected calls")
	}
}

func TestOAuthDisconnectCancelsStreamingBody(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	i := oauthflow.Integration{ID: "test", Issuer: server.URL, Resource: server.URL + "/v1", RequiredScopes: []string{"invoke"}, Routes: []oauthflow.Route{{Method: "POST", Path: "/responses"}}}
	service := oauthflow.New(i)
	mgr := NewManagerWithStore(bytes.Repeat([]byte{3}, 32), NewDBStore(), true)
	mgr.SetOAuthService(service)
	ctx := database.WithUserID(context.Background(), "owner")
	store, _ := mgr.OAuthStore(ctx)
	r, _ := service.Pending("stream", "owner", "test")
	r.State = "connected"
	r.GrantedScopes = []string{"invoke"}
	r.Tokens = oauthflow.Tokens{Access: "access", Type: "Bearer"}
	if err := store.Create(ctx, r); err != nil {
		t.Fatal(err)
	}
	client := NewHTTPClientWithAuthMode(mgr, "oauth:stream", AuthRequired, 0)
	req, _ := http.NewRequestWithContext(ctx, "POST", server.URL+"/v1/responses", nil)
	response, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	done := make(chan error, 1)
	go func() { _, err := io.ReadAll(response.Body); done <- err }()
	if _, err = service.Disconnect(ctx, store, "stream"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("body was not canceled")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("stream survived disconnect")
	}
}
