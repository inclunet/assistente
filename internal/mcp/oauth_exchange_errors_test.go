package mcp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"assistente/internal/oauthflow"
)

func TestPKCEExchangeReturnsSafeActionableErrors(t *testing.T) {
	for _, tc := range []struct {
		code string
		want error
	}{
		{"invalid_client", oauthflow.ErrClientConfiguration},
		{"invalid_scope", oauthflow.ErrPermission},
	} {
		t.Run(tc.code, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				_, _ = fmt.Fprintf(w, `{"error":%q,"error_description":"SECRET"}`, tc.code)
			}))
			defer server.Close()
			previous := browserOpen
			defer func() { browserOpen = previous }()
			browserOpen = func(raw string) error {
				u, err := url.Parse(raw)
				if err != nil {
					return err
				}
				q := u.Query()
				response, err := http.Get(q.Get("redirect_uri") + "?code=code&state=" + url.QueryEscape(q.Get("state")))
				if err == nil {
					_ = response.Body.Close()
				}
				return err
			}
			protocol := oauthProtocol{cfg: ServerConfig{URL: server.URL, OAuth2ClientID: "client", OAuth2AuthURL: server.URL + "/authorize", OAuth2TokenURL: server.URL + "/token", OAuth2CallbackHost: "127.0.0.1"}, resolvedClientID: "client", clientAuthMethod: "none"}
			err := protocol.authorizePKCE(context.Background())
			if !errors.Is(err, tc.want) || strings.Contains(err.Error(), "SECRET") {
				t.Fatalf("unexpected diagnostic: %v", err)
			}
		})
	}
}
