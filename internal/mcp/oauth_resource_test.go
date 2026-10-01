package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/oauthflow"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

func resourceTestClient(t *testing.T, grant AuthType, resource, token string) *http.Client {
	t.Helper()
	m := newTestManagerWithEmit(func(string, any) {})
	t.Cleanup(m.CloseAll)
	storeUserToken(t, m, "resource", "test-token", "refresh", time.Now().Add(time.Hour).Unix())
	rt := &pkceRoundTripper{credMgr: m.credMgr, serverSlug: "resource"}
	rt.persistClientCreds("client", "secret")
	return m.buildAuthHTTPClient(context.Background(), "resource", ServerConfig{URL: resource, AuthType: grant, OAuth2ClientID: "client", OAuth2TokenURL: token})
}

func TestOAuthResourceRedirectsAndAudience(t *testing.T) {
	for _, grant := range []AuthType{AuthOAuth2PKCE, AuthOAuth2ClientCredentials} {
		t.Run(string(grant), func(t *testing.T) {
			var leaked, received, posts atomic.Int32
			other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { leaked.Add(1) }))
			defer other.Close()
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/token" {
					posts.Add(1)
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "token_type": "Bearer", "expires_in": 3600})
					return
				}
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("resource missing bearer")
				}
				switch r.URL.Path {
				case "/cross":
					http.Redirect(w, r, other.URL+"/private", http.StatusTemporaryRedirect)
				case "/same":
					http.Redirect(w, r, "/final", http.StatusTemporaryRedirect)
				default:
					received.Add(1)
					_, _ = io.WriteString(w, "ok")
				}
			}))
			defer server.Close()
			client := resourceTestClient(t, grant, server.URL, server.URL+"/token")
			response, err := client.Post(server.URL+"/same", "application/json", strings.NewReader("{}"))
			if err != nil {
				t.Fatal(err)
			}
			_ = response.Body.Close()
			if received.Load() != 1 {
				t.Fatal("same-origin redirect failed")
			}
			before := posts.Load()
			for _, target := range []string{server.URL + "/cross", other.URL + "/sse-endpoint", "http://remote.example/resource"} {
				response, err := client.Get(target)
				if response != nil {
					_ = response.Body.Close()
				}
				if !errors.Is(err, oauthflow.ErrResourceDestination) {
					t.Fatalf("unexpected result for %s: %v", target, err)
				}
			}
			if leaked.Load() != 0 || posts.Load() != before {
				t.Fatalf("reached unapproved resource or refreshed unnecessarily: leaked=%d posts=%d", leaked.Load(), posts.Load())
			}
		})
	}
}

func TestOAuthResourceRejectsConfiguredRemoteHTTPBeforeToken(t *testing.T) {
	for _, grant := range []AuthType{AuthOAuth2PKCE, AuthOAuth2ClientCredentials} {
		t.Run(string(grant), func(t *testing.T) {
			var calls atomic.Int32
			token := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
			defer token.Close()
			client := resourceTestClient(t, grant, "http://remote.example/mcp", token.URL)
			_, err := client.Get("http://remote.example/mcp")
			if !errors.Is(err, oauthflow.ErrResourceDestination) || calls.Load() != 0 {
				t.Fatalf("invalid resource reached token endpoint: %v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestOAuthResourcePreservesMCPStreaming(t *testing.T) {
	for _, grant := range []AuthType{AuthOAuth2PKCE, AuthOAuth2ClientCredentials} {
		for _, mode := range []string{"sse", "streamable"} {
			t.Run(string(grant)+"/"+mode, func(t *testing.T) {
				server := mcpsdk.NewServer(&mcpsdk.Implementation{Name: "oauth-resource", Version: "1"}, nil)
				var handler http.Handler
				if mode == "sse" {
					handler = mcpsdk.NewSSEHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
				} else {
					handler = mcpsdk.NewStreamableHTTPHandler(func(*http.Request) *mcpsdk.Server { return server }, nil)
				}
				httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/token" {
						w.Header().Set("Content-Type", "application/json")
						_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "test-token", "token_type": "Bearer", "expires_in": 3600})
						return
					}
					if r.Header.Get("Authorization") != "Bearer test-token" {
						t.Error("missing bearer")
						w.WriteHeader(401)
						return
					}
					handler.ServeHTTP(w, r)
				}))
				defer httpServer.Close()
				httpClient := resourceTestClient(t, grant, httpServer.URL, httpServer.URL+"/token")
				var transport mcpsdk.Transport
				if mode == "sse" {
					transport = &mcpsdk.SSEClientTransport{Endpoint: httpServer.URL, HTTPClient: httpClient}
				} else {
					transport = &mcpsdk.StreamableClientTransport{Endpoint: httpServer.URL, HTTPClient: httpClient}
				}
				client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "test", Version: "1"}, nil)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				session, err := client.Connect(ctx, transport, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer func() { _ = session.Close() }()
				if err := session.Ping(ctx, nil); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}
