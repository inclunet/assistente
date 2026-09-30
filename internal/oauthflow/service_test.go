package oauthflow

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

type memoryStore struct {
	mu           sync.Mutex
	r            Record
	failRevision uint64
}

func (m *memoryStore) Load(ctx context.Context, id string) (Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	if m.r.ID != id {
		return Record{}, ErrConflict
	}
	return m.r, nil
}
func (m *memoryStore) Create(_ context.Context, r Record) error { m.r = r; return nil }
func (m *memoryStore) CompareAndSwap(ctx context.Context, r Record, revision uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if m.failRevision == r.Revision {
		return errors.New("disk full")
	}
	if revision != m.r.Revision {
		return ErrConflict
	}
	m.r = r
	return nil
}
func fixture(t *testing.T, handler http.HandlerFunc) (*Service, *memoryStore, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	i := Integration{ID: "fixture", Issuer: server.URL, Resource: server.URL + "/v1", Endpoints: Endpoints{Token: server.URL + "/token", Authorization: server.URL + "/authorize", JWKS: server.URL + "/jwks", Revocation: server.URL + "/revoke"}, Scopes: []string{"openid", "invoke"}, RequiredScopes: []string{"invoke"}, Callback: CallbackConfig{Host: "127.0.0.1", Path: "/auth/callback", PortPolicy: "ephemeral"}, InitialClientID: "register", RegistrationMethod: "extension", CallbackClientID: func(r Record, v url.Values) (string, error) { return v.Get("client_id"), nil }}
	service := New(i)
	record, _ := service.Pending("authorization", "owner", "fixture")
	record.Client.ID = "client"
	record.State = "connected"
	record.Subject = "subject"
	record.GrantedScopes = []string{"invoke"}
	record.Tokens = Tokens{Access: "old-access", Refresh: "old-refresh", Type: "Bearer", ExpiresAt: time.Now().Add(-time.Hour)}
	return service, &memoryStore{r: record}, server
}
func TestRefreshSerializedAndRotationPersisted(t *testing.T) {
	var calls atomic.Int32
	s, store, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = r.ParseForm()
		if r.Form.Get("refresh_token") != "old-refresh" || r.Form.Has("scope") {
			t.Error("wrong refresh request")
		}
		_, _ = fmt.Fprint(w, `{"access_token":"new-access","refresh_token":"new-refresh","token_type":"Bearer","expires_in":3600}`)
	})
	resource := store.r.Resource
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.Resolve(context.Background(), store, "authorization", resource, "")
			if err != nil || r.Tokens.Access != "new-access" {
				t.Errorf("resolve: %v", err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 || store.r.Tokens.Refresh != "new-refresh" || store.r.RefreshPending {
		t.Fatalf("refresh state: calls=%d", calls.Load())
	}
}
func TestRefreshCrashSafetyAndNoImplicitRetry(t *testing.T) {
	for _, failure := range []string{"before", "after", "network"} {
		t.Run(failure, func(t *testing.T) {
			calls := 0
			s, store, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if failure == "network" {
					w.WriteHeader(503)
					return
				}
				_, _ = fmt.Fprint(w, `{"access_token":"new","refresh_token":"rotated","token_type":"Bearer","expires_in":3600}`)
			})
			if failure == "before" {
				store.failRevision = 2
			}
			if failure == "after" {
				store.failRevision = 3
			}
			_, err := s.Resolve(context.Background(), store, store.r.ID, store.r.Resource, "")
			if err == nil {
				t.Fatal("expected failure")
			}
			expected := 1
			if failure == "before" {
				expected = 0
			}
			if calls != expected {
				t.Fatalf("calls=%d", calls)
			}
			if failure != "before" {
				restarted := New(s.integrations["fixture"])
				_, err = restarted.Resolve(context.Background(), store, store.r.ID, store.r.Resource, "")
				if !errors.Is(err, ErrReauthorize) || calls != 1 {
					t.Fatal("reused ambiguous refresh")
				}
			}
		})
	}
}
func TestRefreshLostScopePersistsRotationButDoesNotExposeToken(t *testing.T) {
	s, store, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"access_token":"new","refresh_token":"rotated","token_type":"Bearer","scope":"openid","expires_in":3600}`)
	})
	r, err := s.Resolve(context.Background(), store, store.r.ID, store.r.Resource, "")
	if !errors.Is(err, ErrPermission) || r.Tokens.Access != "" || store.r.Tokens.Refresh != "rotated" || store.r.State != "permission_required" {
		t.Fatalf("scope loss: %v", err)
	}
}
func TestExpiryAndResourceRules(t *testing.T) {
	s, store, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected renewal") })
	store.r.Tokens.ExpiresAt = time.Time{}
	if _, err := s.Resolve(context.Background(), store, store.r.ID, store.r.Resource, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(context.Background(), store, store.r.ID, "https://other.invalid", ""); !errors.Is(err, ErrResource) {
		t.Fatal(err)
	}
	store.r.Tokens.ExpiresAt = time.Now().Add(-time.Minute)
	store.r.Tokens.EarliestRefreshAt = time.Now().Add(time.Hour)
	if _, err := s.Resolve(context.Background(), store, store.r.ID, store.r.Resource, ""); !errors.Is(err, ErrTransient) {
		t.Fatal(err)
	}
}
func TestAuthorizePKCECallbackAndValidatedIdentity(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	var nonce, issuer, redirect, challenge string
	var store *memoryStore
	s, m, server := fixture(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/jwks" {
			_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "test", Algorithm: "RS256", Use: "sig"}}})
			return
		}
		if r.URL.Path != "/token" {
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(404)
			return
		}
		_ = r.ParseForm()
		if r.Form.Get("client_id") != "issued" || r.Form.Get("redirect_uri") != redirect || r.Form.Get("code") != "code" || r.Form.Get("code_verifier") == "" {
			t.Error("invalid exchange")
		}
		if store.r.Client.ID != "issued" {
			t.Error("client not durably retained before exchange")
		}
		signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "test"))
		signed, _ := jwt.Signed(signer).Claims(jwt.Claims{Issuer: issuer, Subject: "subject", Audience: jwt.Audience{"issued"}, Expiry: jwt.NewNumericDate(time.Now().Add(time.Hour))}).Claims(map[string]any{"nonce": nonce, "email": "user@example.test"}).Serialize()
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "access", "refresh_token": "refresh", "token_type": "Bearer", "id_token": signed, "scope": "openid invoke", "expires_in": 3600})
	})
	store = m
	issuer = server.URL
	store.r.Client.ID = ""
	store.r.Tokens = Tokens{}
	store.r.State = "pending"
	summary, err := s.Authorize(context.Background(), store, store.r.ID, "host", func(raw string) error {
		u, _ := url.Parse(raw)
		q := u.Query()
		nonce = q.Get("nonce")
		redirect = q.Get("redirect_uri")
		challenge = q.Get("code_challenge")
		if q.Get("client_id") != "register" || q.Get("code_challenge_method") != "S256" || challenge == "" {
			t.Error("invalid authorization parameters")
		}
		bad, _ := http.Get(redirect + "?state=invalid&code=code")
		if bad.StatusCode != 400 {
			t.Error("invalid state accepted")
		}
		_ = bad.Body.Close()
		v := url.Values{"state": {q.Get("state")}, "code": {"code"}, "client_id": {"issued"}}
		good, e := http.Get(redirect + "?" + v.Encode())
		if e != nil {
			return e
		}
		_ = good.Body.Close()
		return nil
	}, "Return to app")
	if err != nil || summary.State != "connected" || summary.Email != "user@example.test" {
		t.Fatalf("authorization: %+v %v", summary, err)
	}
	if strings.Contains(fmt.Sprintf("%+v", summary), "refresh") {
		t.Fatal("summary leaked token")
	}
}
func TestAuthorizationCancellationClosesListener(t *testing.T) {
	s, store, _ := fixture(t, func(http.ResponseWriter, *http.Request) { t.Error("exchange on canceled authorization") })
	ctx, cancel := context.WithCancel(context.Background())
	var redirect string
	_, err := s.Authorize(ctx, store, store.r.ID, "host", func(raw string) error {
		u, _ := url.Parse(raw)
		redirect = u.Query().Get("redirect_uri")
		cancel()
		return nil
	}, "Return")
	if !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	client := &http.Client{Timeout: time.Second}
	if resp, err := client.Get(redirect); err == nil {
		_ = resp.Body.Close()
		t.Fatal("callback still listening")
	}
}

func TestRefreshConfirmedInvalidGrantClearsTokens(t *testing.T) {
	for _, code := range []string{"invalid_grant", "invalid_refresh_token", "token_expired", "refresh_token_expired", "refresh_token_invalidated", "refresh_token_reused"} {
		t.Run(code, func(t *testing.T) {
			calls := 0
			s, store, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": code, "error_description": "secret-refresh"})
			})
			store.r.Tokens.ID = "validated-identity"
			_, err := s.Resolve(context.Background(), store, store.r.ID, store.r.Resource, "")
			if !errors.Is(err, ErrReauthorize) || strings.Contains(err.Error(), "secret-refresh") {
				t.Fatalf("unexpected error: %v", err)
			}
			if store.r.Tokens != (Tokens{ID: "validated-identity"}) || store.r.RefreshPending || store.r.State != "reauthorization_required" || store.r.Client.ID != "client" {
				t.Fatal("invalid grant was not cleared while retaining registration")
			}
			_, _ = s.Resolve(context.Background(), store, store.r.ID, store.r.Resource, "")
			if calls != 1 {
				t.Fatal("retried rejected grant")
			}
		})
	}
}

func TestCallbackBodyDeliveredBeforeFastAuthorizationFailure(t *testing.T) {
	const completion = "Retorne à aplicação"
	for attempt := range 20 {
		s, store, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadRequest) })
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		result := make(chan string, 1)
		_, err := s.Authorize(ctx, store, store.r.ID, "host", func(raw string) error {
			u, _ := url.Parse(raw)
			q := u.Query()
			go func() {
				values := url.Values{"state": {q.Get("state")}, "code": {"code"}, "client_id": {"client"}}
				if attempt%2 == 0 {
					values.Del("code")
					values.Set("error", "access_denied")
				}
				response, err := http.Get(q.Get("redirect_uri") + "?" + values.Encode())
				if err != nil {
					result <- err.Error()
					return
				}
				body, err := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if err != nil {
					result <- err.Error()
					return
				}
				if response.ContentLength != int64(len(completion)) {
					result <- "incomplete framing"
					return
				}
				result <- string(body)
			}()
			return nil
		}, completion)
		if err == nil {
			t.Fatal("expected token failure")
		}
		select {
		case body := <-result:
			if body != completion {
				t.Fatalf("callback truncated: %q", body)
			}
		case <-ctx.Done():
			t.Fatal("callback not delivered")
		}
		cancel()
	}
}

func TestDisconnectRetainsOnlyIdentityHint(t *testing.T) {
	s, store, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	store.r.Tokens.ID = "validated-identity"
	revoked, err := s.Disconnect(context.Background(), store, store.r.ID)
	if err != nil || !revoked {
		t.Fatalf("disconnect: %v", err)
	}
	if store.r.Tokens != (Tokens{ID: "validated-identity"}) || store.r.State != "disconnected" {
		t.Fatal("access retained or identity hint lost")
	}
	if _, err = s.Resolve(context.Background(), store, store.r.ID, store.r.Resource, ""); !errors.Is(err, ErrReauthorize) {
		t.Fatalf("disconnected grant used: %v", err)
	}
}

func TestDisconnectAccessOnlyRevocation(t *testing.T) {
	for _, status := range []int{200, 400, 503, 0} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			calls := 0
			s, store, _ := fixture(t, func(w http.ResponseWriter, r *http.Request) {
				calls++
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Form.Get("token") != "access-only" || r.Form.Get("token_type_hint") != "access_token" {
					t.Error("wrong revocation token")
				}
				w.WriteHeader(status)
			})
			store.r.Tokens = Tokens{Access: "access-only", ID: "validated-identity"}
			if status == 0 {
				store.r.Tokens.Access = ""
			}
			confirmed, err := s.Disconnect(context.Background(), store, store.r.ID)
			if err != nil || confirmed != (status == 200 || status == 0) {
				t.Fatalf("confirmation: %v %v", confirmed, err)
			}
			expectedCalls := 1
			if status == 0 {
				expectedCalls = 0
			}
			if status == 503 {
				expectedCalls = 2
			}
			if calls != expectedCalls || store.r.Tokens != (Tokens{ID: "validated-identity"}) || store.r.State != "disconnected" {
				t.Fatal("revocation lifecycle", calls)
			}
		})
	}
}
