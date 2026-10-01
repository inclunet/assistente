package oauthflow

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDeviceGrantPollingAndScopes(t *testing.T) {
	for _, scope := range []string{"omitted", "limited"} {
		t.Run(scope, func(t *testing.T) {
			polls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Method != "POST" || r.Form.Get("client_id") != "client" || r.Form.Get("resource") != "https://audience.example/resource" {
					t.Errorf("invalid grant request: %s %v", r.Method, r.Form)
				}
				if r.URL.Path == "/device" {
					if r.Form.Get("scope") != "read write" {
						t.Error("missing requested scopes")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "secret-device", "user_code": "USER", "verification_uri": "http://" + r.Host + "/verify", "expires_in": 60})
					return
				}
				if r.Form.Get("device_code") != "secret-device" || r.Form.Get("grant_type") != "urn:ietf:params:oauth:grant-type:device_code" {
					t.Error("invalid device poll")
				}
				polls++
				if polls == 1 {
					w.WriteHeader(400)
					_, _ = w.Write([]byte(`{"error":"authorization_pending"}`))
					return
				}
				if polls == 2 {
					_, _ = w.Write([]byte(`{"error":"slow_down"}`))
					return
				}
				response := map[string]any{"access_token": "access", "refresh_token": "refresh", "token_type": "Bearer", "expires_in": 3600}
				if scope == "limited" {
					response["scope"] = "read"
				}
				_ = json.NewEncoder(w).Encode(response)
			}))
			defer srv.Close()
			var intervals []time.Duration
			presented := false
			result, err := authorizeDevice(context.Background(), DeviceGrantConfig{Resource: srv.URL, Audience: "https://audience.example/resource", ClientID: "client", DeviceEndpoint: srv.URL + "/device", TokenEndpoint: srv.URL + "/token", Scopes: []string{"read", "write"}}, func(_ context.Context, v DeviceVerification) error {
				presented = true
				if v.UserCode != "USER" || v.URL != srv.URL+"/verify" {
					t.Errorf("bad verification: %v", v)
				}
				return nil
			}, func(_ context.Context, d time.Duration) error {
				if !presented {
					t.Error("poll before presentation")
				}
				intervals = append(intervals, d)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			expectedScopes := []string{"read", "write"}
			if scope == "limited" {
				expectedScopes = []string{"read"}
			}
			if !reflect.DeepEqual(result.Scopes, expectedScopes) || result.Token.AccessToken != "access" || result.Token.RefreshToken != "refresh" || time.Until(result.Token.Expiry) < 59*time.Minute {
				t.Fatalf("invalid result: %+v", result)
			}
			if !reflect.DeepEqual(intervals, []time.Duration{5 * time.Second, 5 * time.Second, 10 * time.Second}) {
				t.Fatal(intervals)
			}
		})
	}
}

func TestDeviceGrantErrorsAreTerminalAndSanitized(t *testing.T) {
	for _, tc := range []struct {
		name, body, code string
		status           int
	}{
		{"refused", `{"error":"access_denied","error_description":"SECRET"}`, "access_denied", 400},
		{"expired", `{"error":"expired_token"}`, "expired_token", 400},
		{"registration", `{"error":"unauthorized_client"}`, "unauthorized_client", 400},
		{"unknown", `{"error":"SECRET"}`, "", 400},
		{"malformed", `SECRET`, "", 200},
		{"oversized", strings.Repeat("SECRET", 12000), "", 400},
		{"empty", `{"token_type":"Bearer"}`, "", 200},
		{"type", `{"access_token":"SECRET","token_type":"MAC"}`, "", 200},
		{"http", `{"access_token":"SECRET","token_type":"Bearer"}`, "", 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			polls := 0
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/device" {
					_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "SECRET", "user_code": "USER", "verification_uri": "http://" + r.Host + "/verify", "expires_in": 60})
					return
				}
				polls++
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()
			_, err := authorizeDevice(context.Background(), DeviceGrantConfig{Resource: srv.URL, ClientID: "c", DeviceEndpoint: srv.URL + "/device", TokenEndpoint: srv.URL + "/token"}, func(context.Context, DeviceVerification) error { return nil }, func(context.Context, time.Duration) error { return nil })
			if !errors.Is(err, ErrDeviceGrant) || DeviceGrantErrorCode(err) != tc.code || strings.Contains(err.Error(), "SECRET") || polls != 1 {
				t.Fatalf("err=%v polls=%d", err, polls)
			}
		})
	}
}
func TestDeviceGrantExpiresDuringPresentationAndCancelsPolling(t *testing.T) {
	for _, mode := range []string{"expires", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			var polls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/device" {
					_ = json.NewEncoder(w).Encode(map[string]any{"device_code": "d", "user_code": "u", "verification_uri": "http://" + r.Host + "/verify", "expires_in": 1})
					return
				}
				polls.Add(1)
			}))
			defer srv.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			_, err := AuthorizeDevice(ctx, DeviceGrantConfig{Resource: srv.URL, ClientID: "c", DeviceEndpoint: srv.URL + "/device", TokenEndpoint: srv.URL + "/token"}, func(ctx context.Context, _ DeviceVerification) error {
				if mode == "cancel" {
					cancel()
				} else {
					<-ctx.Done()
				}
				return nil
			})
			want := context.DeadlineExceeded
			if mode == "cancel" {
				want = context.Canceled
			}
			if !errors.Is(err, want) || polls.Load() != 0 {
				t.Fatalf("err=%v polls=%d", err, polls.Load())
			}
		})
	}
}
func TestDeviceGrantInitialErrorPreservesSafeCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"unauthorized_client","error_description":"SECRET"}`))
	}))
	defer srv.Close()
	_, err := AuthorizeDevice(context.Background(), DeviceGrantConfig{Resource: srv.URL, ClientID: "c", DeviceEndpoint: srv.URL}, func(context.Context, DeviceVerification) error { t.Fatal("unexpected presentation"); return nil })
	if DeviceGrantErrorCode(err) != "unauthorized_client" || strings.Contains(err.Error(), "SECRET") {
		t.Fatal(err)
	}
}

func TestDeviceRequestTimeoutDoesNotClaimGrantExpired(t *testing.T) {
	if err := safeDeviceTransportError(context.Background(), context.DeadlineExceeded); !errors.Is(err, ErrTransient) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("request timeout classified as grant expiry: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()
	if err := safeDeviceTransportError(ctx, context.DeadlineExceeded); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("flow expiry lost: %v", err)
	}
}
