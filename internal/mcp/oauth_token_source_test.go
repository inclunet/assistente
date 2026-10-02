package mcp

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/oauthflow"
	"golang.org/x/oauth2"
)

func TestStoredAndClientCredentialsRefreshApprovalIsPerOperation(t *testing.T) {
	for _, grant := range []string{"pkce", "client-credentials"} {
		t.Run(grant, func(t *testing.T) {
			var prompts, requests, issued atomic.Int32
			var refreshes []string
			var refreshesMu sync.Mutex
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "application/json")
				if r.Header.Get("Authorization") != "" {
					w.WriteHeader(401)
					_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
					return
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
					return
				}
				refreshesMu.Lock()
				refreshes = append(refreshes, r.Form.Get("refresh_token"))
				refreshesMu.Unlock()
				n := issued.Add(1)
				expires := 3600
				access := "second"
				if n == 1 {
					expires = 1
					access = "first"
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": access, "refresh_token": "rotated", "token_type": "Bearer", "expires_in": expires})
			}))
			defer server.Close()
			authorize := func(context.Context, oauthflow.NetworkDestination) ([]net.IP, bool, error) {
				prompts.Add(1)
				return []net.IP{net.ParseIP("127.0.0.1")}, true, nil
			}
			ctx := oauthflow.WithNetworkAuthorizer(context.Background(), authorize)
			cfg := ServerConfig{URL: "https://192.0.2.1/mcp", OAuth2ClientID: "client", OAuth2TokenURL: server.URL}
			var source oauth2.TokenSource
			if grant == "pkce" {
				m := newTestManagerWithEmit(func(string, any) {})
				defer m.CloseAll()
				m.SetOAuthNetworkAuthorizer(authorize)
				storeUserToken(t, m, "srv", "expired", "seed", time.Now().Add(-time.Hour).Unix())
				source = m.buildPKCERoundTripperForServer(ctx, "srv", cfg).tokenSource
			} else {
				source = buildClientCredentialsTokenSource(ctx, cfg, "secret", authorize)
			}
			first, err := source.Token()
			if err != nil || first == nil || first.AccessToken != "first" {
				t.Fatalf("first refresh: %v", err)
			}
			var wg sync.WaitGroup
			results := make(chan *oauth2.Token, 8)
			errs := make(chan error, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() { defer wg.Done(); token, err := source.Token(); results <- token; errs <- err }()
			}
			wg.Wait()
			close(results)
			close(errs)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			for token := range results {
				if token == nil || token.AccessToken != "second" {
					t.Fatalf("concurrent refresh: %+v", token)
				}
			}
			if _, err := source.Token(); err != nil {
				t.Fatal(err)
			}
			if prompts.Load() != 2 || issued.Load() != 2 || requests.Load() != 3 {
				t.Fatalf("prompts=%d issued=%d HTTP attempts=%d", prompts.Load(), issued.Load(), requests.Load())
			}
			refreshesMu.Lock()
			defer refreshesMu.Unlock()
			if grant == "pkce" && (len(refreshes) != 2 || refreshes[0] != "seed" || refreshes[1] != "rotated") {
				t.Fatalf("refresh rotation lost: %v", refreshes)
			}
		})
	}
}
