package oauthintegrations

import (
	"context"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"assistente/internal/oauthflow"
	"golang.org/x/oauth2"
)

type slackTransportFunc func(*http.Request) (*http.Response, error)

func (f slackTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type slackStore struct{ record oauthflow.Record }

func (s *slackStore) Create(_ context.Context, r oauthflow.Record) error {
	s.record = r
	return nil
}

func (s *slackStore) Load(context.Context, string) (oauthflow.Record, error) { return s.record, nil }
func (s *slackStore) CompareAndSwap(_ context.Context, r oauthflow.Record, revision uint64) error {
	if s.record.Revision != revision {
		return oauthflow.ErrConflict
	}
	s.record = r
	return nil
}

func TestSlackMCPAuthorizationAndRefresh(t *testing.T) {
	for _, method := range []string{"none", "client_secret_post"} {
		t.Run(method, func(t *testing.T) {
			ctx := context.Background()
			r := oauthflow.Record{Version: 1, Revision: 1, ID: "grant", UserID: "owner", ConsumerID: "slack", Integration: "mcp", GrantType: "authorization_code", State: "pending", Resource: "https://mcp.slack.com/mcp", Audience: "https://mcp.slack.com", RequestedScopes: []string{"chat:write", "users:read"}, Client: oauthflow.ClientRegistration{ID: "client", AuthMethod: method}, Endpoints: oauthflow.Endpoints{Token: "https://slack.com/api/oauth.v2.user.access"}}
			if method != "none" {
				r.Client.Secret = "secret"
			}
			calls := 0
			client := MCPHTTPClient(&http.Client{Transport: slackTransportFunc(func(req *http.Request) (*http.Response, error) {
				calls++
				if err := req.ParseForm(); err != nil {
					t.Fatal(err)
				}
				if req.Form.Get("client_secret") != r.Client.Secret || req.Form.Get("client_id") != r.Client.ID {
					t.Error("client method changed")
				}
				body := `{"ok":true,"access_token":"first","refresh_token":"refresh","token_type":"user","expires_in":3600,"authed_user":{"scope":"chat:write,users:read"}}`
				if calls > 1 {
					if req.Form.Get("grant_type") != "refresh_token" || req.Form.Get("refresh_token") != "refresh" {
						t.Error("wrong refresh request")
					}
					body = `{"ok":true,"access_token":"second","refresh_token":"rotated","token_type":"user","expires_in":3600,"scope":"chat:write,users:read"}`
				}
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body))}, nil
			})}, r.Resource)
			service, err := oauthflow.NewConfigured(r, nil)
			if err != nil {
				t.Fatal(err)
			}
			service.HTTP, service.BeforeRefresh = client, nil
			store := &slackStore{record: r}
			_, err = service.AuthorizeUsing(ctx, store, r.ID, func(ctx context.Context, candidate oauthflow.Record) (oauthflow.Record, error) {
				cfg := oauth2.Config{ClientID: r.Client.ID, ClientSecret: r.Client.Secret, Endpoint: oauth2.Endpoint{TokenURL: r.Endpoints.Token, AuthStyle: oauth2.AuthStyleInParams}}
				token, err := cfg.Exchange(context.WithValue(ctx, oauth2.HTTPClient, client), "code", oauth2.VerifierOption("verifier"))
				if err != nil {
					return candidate, err
				}
				if token.Type() != "Bearer" {
					return candidate, oauthflow.ErrReauthorize
				}
				candidate.Tokens = oauthflow.Tokens{Access: token.AccessToken, Refresh: token.RefreshToken, Type: token.Type(), ExpiresAt: token.Expiry}
				candidate.GrantedScopes = strings.Fields(token.Extra("scope").(string))
				return candidate, nil
			})
			if err != nil || store.record.State != "connected" || !reflect.DeepEqual(store.record.GrantedScopes, r.RequestedScopes) {
				t.Fatalf("authorization failed: %v", err)
			}
			got, err := service.Resolve(ctx, store, r.ID, r.Resource, "first")
			if err != nil || got.Tokens.Access != "second" || got.Tokens.Refresh != "rotated" || got.RefreshPending || calls != 2 {
				t.Fatalf("refresh failed: %v calls=%d", err, calls)
			}
		})
	}
}

func TestSlackMCPResponseBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, resource, endpoint, body string
		wantError                      bool
		status                         int
	}{
		{"bot", "https://mcp.slack.com/mcp", "https://slack.com/api/oauth.v2.user.access", `{"ok":true,"token_type":"bot","access_token":"SECRET"}`, true, 0},
		{"malformed", "https://mcp.slack.com/mcp", "https://slack.com/api/oauth.v2.user.access", `bad SECRET`, true, 0},
		{"null_scope", "https://mcp.slack.com/mcp", "https://slack.com/api/oauth.v2.user.access", `{"ok":true,"token_type":"user","scope":null}`, true, 0},
		{"failure_with_token", "https://mcp.slack.com/mcp", "https://slack.com/api/oauth.v2.user.access", `{"ok":false,"error":"invalid_refresh_token","access_token":"SECRET","token_type":"user"}`, false, 400},
		{"unknown_error", "https://mcp.slack.com/mcp", "https://slack.com/api/oauth.v2.user.access", `{"ok":false,"error":"SECRET"}`, false, 400},
		{"other_resource", "https://other.example/mcp", "https://slack.com/api/oauth.v2.user.access", `{"token_type":"user"}`, false, 200},
		{"other_endpoint", "https://mcp.slack.com/mcp", "https://slack.com.evil.test/api/oauth.v2.user.access", `{"token_type":"user"}`, false, 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := &http.Client{Timeout: time.Second, Transport: slackTransportFunc(func(*http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}
			client := MCPHTTPClient(original, tc.resource)
			if client.Timeout != original.Timeout {
				t.Fatal("timeout changed")
			}
			response, err := client.Post(tc.endpoint, "application/x-www-form-urlencoded", strings.NewReader(""))
			if tc.wantError {
				if !errors.Is(err, oauthflow.ErrReauthorize) || strings.Contains(err.Error(), "SECRET") {
					t.Fatalf("unsafe error: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			body, _ := io.ReadAll(response.Body)
			if response.StatusCode != tc.status {
				t.Fatal("wrong status")
			}
			if tc.status == 400 && strings.Contains(string(body), "SECRET") {
				t.Fatal("secret in failure")
			}
			if tc.status == 200 && string(body) != tc.body {
				t.Fatal("unrelated provider changed")
			}
		})
	}
}

func TestSlackMCPRefreshRejectsInvalidGrantsAndPreservesOmittedMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		want       error
	}{
		{"omitted_scope_refresh", `{"ok":true,"access_token":"next","token_type":"user"}`, nil},
		{"reduced_scope", `{"ok":true,"access_token":"next","token_type":"user","scope":"read"}`, oauthflow.ErrPermission},
		{"empty_scope", `{"ok":true,"access_token":"next","token_type":"user","authed_user":{"scope":""}}`, oauthflow.ErrPermission},
		{"revoked", `{"ok":false,"error":"invalid_refresh_token","access_token":"must-not-use","token_type":"user"}`, oauthflow.ErrReauthorize},
		{"missing_access", `{"ok":true,"token_type":"user"}`, oauthflow.ErrReauthorize},
		{"missing_ok", `{"access_token":"must-not-use","token_type":"user"}`, oauthflow.ErrReauthorize},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := oauthflow.Record{Version: 1, Revision: 1, ID: "grant", UserID: "owner", ConsumerID: "slack", Integration: "mcp", GrantType: "authorization_code", State: "connected", Resource: "https://mcp.slack.com/mcp", RequestedScopes: []string{"read", "write"}, GrantedScopes: []string{"read", "write"}, Client: oauthflow.ClientRegistration{ID: "client", AuthMethod: "none"}, Endpoints: oauthflow.Endpoints{Token: "https://slack.com/api/oauth.v2.user.access"}, Tokens: oauthflow.Tokens{Access: "old", Refresh: "refresh", Type: "Bearer", ExpiresAt: time.Now().Add(-time.Hour)}}
			service, err := oauthflow.NewConfigured(r, nil)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			service.BeforeRefresh = nil
			service.HTTP = MCPHTTPClient(&http.Client{Transport: slackTransportFunc(func(*http.Request) (*http.Response, error) {
				calls++
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})}, r.Resource)
			store := &slackStore{record: r}
			got, err := service.Resolve(context.Background(), store, r.ID, r.Resource, "")
			if !errors.Is(err, tc.want) || calls != 1 {
				t.Fatalf("resolve: %v calls=%d", err, calls)
			}
			if tc.want == nil && (got.Tokens.Refresh != "refresh" || !reflect.DeepEqual(got.GrantedScopes, r.GrantedScopes)) {
				t.Fatal("omitted metadata lost")
			}
			if tc.want != nil && got.Tokens.Access != "" {
				t.Fatal("invalid grant returned")
			}
		})
	}
}
