package mcp

import (
	"assistente/internal/credentials"
	"assistente/internal/oauthflow"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"testing"
	"time"
)

func TestDeviceDCRNeverRequiresCallbackPort(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = occupied.Close() }()
	var registrations, polls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(404)
			return
		}
		switch r.URL.Path {
		case "/register":
			var metadata map[string]any
			if err := json.NewDecoder(r.Body).Decode(&metadata); err != nil {
				t.Error(err)
			}
			if _, ok := metadata["redirect_uris"]; ok {
				t.Error("Device DCR registered callback")
			}
			if responseTypes, ok := metadata["response_types"].([]any); !ok || len(responseTypes) != 0 {
				t.Error("Device DCR must explicitly disable the default code response")
			}
			registrations.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"client_id":"registered"}`))
		case "/device":
			_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "device", "user_code": "user", "verification_uri": "http://" + r.Host + "/verify", "expires_in": 60, "interval": 1})
		case "/token":
			if polls.Add(1) == 1 {
				w.WriteHeader(400)
				_, _ = w.Write([]byte(`{"error":"unauthorized_client"}`))
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"access_token":"access","token_type":"Bearer","expires_in":3600}`))
		}
	}))
	defer srv.Close()
	previous := browserOpen
	browserOpen = func(string) error { return nil }
	defer func() { browserOpen = previous }()
	rt := &pkceRoundTripper{cfg: ServerConfig{URL: srv.URL, OAuth2RegistrationURL: srv.URL + "/register", OAuth2DeviceAuthURL: srv.URL + "/device", OAuth2TokenURL: srv.URL + "/token", OAuth2AuthURL: srv.URL + "/authorize", OAuth2CallbackPort: occupied.Addr().(*net.TCPAddr).Port}}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := rt.authorize(ctx); err != nil {
		t.Fatal(err)
	}
	if registrations.Load() != 2 || polls.Load() != 2 || rt.callback != nil || rt.tokenSource == nil {
		t.Fatalf("registration=%d polls=%d callback=%v", registrations.Load(), polls.Load(), rt.callback)
	}
	if rt.cfg.OAuth2CallbackPort != occupied.Addr().(*net.TCPAddr).Port {
		t.Fatal("Device changed callback configuration")
	}
}
func TestManualPKCEPortCollisionDoesNotOpenBrowser(t *testing.T) {
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = occupied.Close() }()
	previous := browserOpen
	browserOpen = func(string) error { t.Error("browser opened without reserved callback"); return nil }
	defer func() { browserOpen = previous }()
	rt := &pkceRoundTripper{cfg: ServerConfig{OAuth2ClientID: "manual", OAuth2CallbackPort: occupied.Addr().(*net.TCPAddr).Port}}
	if err := rt.authorizePKCE(context.Background()); !errors.Is(err, oauthflow.ErrCallbackPort) {
		t.Fatal(err)
	}
}
func TestDeviceRefusalNeverFallsBackToPKCE(t *testing.T) {
	for _, code := range []string{"access_denied", "expired_token"} {
		t.Run(code, func(t *testing.T) {
			var browserCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					w.WriteHeader(404)
					return
				}
				if r.URL.Path == "/device" {
					_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "d", "user_code": "u", "verification_uri": "http://" + r.Host + "/verify", "expires_in": 60, "interval": 1})
					return
				}
				w.WriteHeader(400)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
			}))
			defer srv.Close()
			previous := browserOpen
			browserOpen = func(string) error { browserCalls.Add(1); return nil }
			defer func() { browserOpen = previous }()
			rt := &pkceRoundTripper{cfg: ServerConfig{URL: srv.URL, OAuth2ClientID: "client", OAuth2DeviceAuthURL: srv.URL + "/device", OAuth2TokenURL: srv.URL + "/token", OAuth2AuthURL: srv.URL + "/authorize"}}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := rt.authorize(ctx); oauthflow.DeviceGrantErrorCode(err) != code {
				t.Fatal(err)
			}
			if browserCalls.Load() != 1 || rt.tokenSource != nil {
				t.Fatalf("browser=%d", browserCalls.Load())
			}
		})
	}
}

func TestPKCEFallbackRespectsClientRegistrationGrant(t *testing.T) {
	for _, mode := range []string{"manual", "device-only", "manual-replacement"} {
		t.Run(mode, func(t *testing.T) {
			var registrations atomic.Int32
			expectedClient := "existing"
			if mode == "device-only" {
				expectedClient = "pkce-client"
			}
			if mode == "manual-replacement" {
				expectedClient = "manual-new"
			}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != "POST" {
					w.WriteHeader(404)
					return
				}
				switch r.URL.Path {
				case "/device":
					w.WriteHeader(400)
					_, _ = w.Write([]byte(`{"error":"unsupported_grant_type"}`))
				case "/register":
					registrations.Add(1)
					if mode != "device-only" {
						t.Error("manual client re-registered")
						w.WriteHeader(403)
						return
					}
					var metadata oauthflow.RegistrationRequest
					if err := json.NewDecoder(r.Body).Decode(&metadata); err != nil {
						t.Error(err)
					}
					if len(metadata.RedirectURIs) != 1 {
						t.Error("PKCE callback missing")
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"client_id":"pkce-client"}`))
				case "/token":
					_ = r.ParseForm()
					client := r.Form.Get("client_id")
					if id, _, ok := r.BasicAuth(); ok {
						client = id
					}
					if client != expectedClient {
						t.Errorf("client=%s want=%s", client, expectedClient)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"access_token":"token","token_type":"Bearer","expires_in":3600}`))
				}
			}))
			defer srv.Close()
			manager := credentials.NewManager(nil)
			grant := ""
			if mode != "manual" {
				grant = "urn:ietf:params:oauth:grant-type:device_code"
			}
			if err := manager.RegisterPatternWithContext(context.Background(), clientCredPattern("test"), &credentials.AuthConfig{Source: "static", Type: "oauth2", ClientID: "existing", ClientGrantType: grant}); err != nil {
				t.Fatal(err)
			}
			cfg := ServerConfig{URL: srv.URL, OAuth2ClientID: "existing", OAuth2DeviceAuthURL: srv.URL + "/device", OAuth2RegistrationURL: srv.URL + "/register", OAuth2TokenURL: srv.URL + "/token", OAuth2AuthURL: srv.URL + "/authorize", OAuth2CallbackHost: "127.0.0.1"}
			if mode == "manual-replacement" {
				cfg.OAuth2ClientID = "manual-new"
			}
			rt := buildPKCERoundTripper(cfg, manager, nil, "test", nil, nil, nil, nil)
			previous := browserOpen
			browserOpen = func(raw string) error {
				u, err := url.Parse(raw)
				if err != nil {
					return err
				}
				resp, err := http.Get(u.Query().Get("redirect_uri") + "?code=ok&state=" + url.QueryEscape(u.Query().Get("state")))
				if err == nil {
					_ = resp.Body.Close()
				}
				return err
			}
			defer func() { browserOpen = previous }()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := rt.authorize(ctx); err != nil {
				t.Fatal(err)
			}
			want := int32(0)
			if mode == "device-only" {
				want = 1
			}
			if registrations.Load() != want {
				t.Fatalf("DCR=%d want=%d", registrations.Load(), want)
			}
			if rt.effectiveClientID() != expectedClient {
				t.Fatal("wrong client persisted")
			}
		})
	}
}
