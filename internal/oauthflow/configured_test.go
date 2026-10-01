package oauthflow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func configuredRecord(resource string) Record {
	return Record{Version: 1, Revision: 1, ID: "configured", UserID: "owner", ConsumerID: "server", Integration: "mcp", GrantType: "authorization_code", State: "connected", Resource: resource, Audience: resource, Client: ClientRegistration{ID: "client", AuthMethod: "none"}, Endpoints: Endpoints{Token: resource + "/token"}, Tokens: Tokens{Access: "old", Refresh: "refresh", Type: "Bearer", ExpiresAt: time.Now().Add(-time.Hour)}}
}

func TestConfiguredRefreshCoordinatesServiceInstances(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "new", "refresh_token": "rotated", "token_type": "Bearer", "expires_in": 3600})
	}))
	defer srv.Close()
	r := configuredRecord(srv.URL)
	store := &memoryStore{r: r}
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s, err := NewConfigured(r, nil)
			if err != nil {
				t.Error(err)
				return
			}
			got, err := s.Resolve(WithNetworkOperation(context.Background()), store, r.ID, r.Resource, "")
			if err != nil || got.Tokens.Access != "new" {
				t.Errorf("resolve: %v %v", got.State, err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("refresh requests=%d", calls.Load())
	}
	current, _ := store.Load(context.Background(), r.ID)
	if current.Tokens.Refresh != "rotated" || current.RefreshPending {
		t.Fatalf("rotation not committed: %v", current.State)
	}
}

func TestConfiguredAuthorizationFailurePreservesPreviousAndLateResultsCannotRestore(t *testing.T) {
	for _, mode := range []string{"refused", "invalidate", "save_failure"} {
		t.Run(mode, func(t *testing.T) {
			r := configuredRecord("https://resource.example")
			store := &memoryStore{r: r}
			s, _ := NewConfigured(r, nil)
			_, err := s.AuthorizeUsing(context.Background(), store, r.ID, func(ctx context.Context, candidate Record) (Record, error) {
				if mode == "refused" {
					return candidate, errors.New("refused")
				}
				if mode == "invalidate" {
					if err := s.Invalidate(ctx, store, r.ID); !errors.Is(err, ErrConflict) {
						t.Fatalf("active lease: %v", err)
					}
					invalid := candidate
					invalid.State = "disconnected"
					invalid.Tokens = Tokens{}
					invalid.Revision++
					if err := store.CompareAndSwap(ctx, invalid, candidate.Revision); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "save_failure" {
					store.failRevision = candidate.Revision + 1
				}
				candidate.Tokens.Access = "new"
				return candidate, nil
			})
			if err == nil {
				t.Fatal("expected failure")
			}
			got, _ := store.Load(context.Background(), r.ID)
			if mode == "invalidate" {
				if got.State != "disconnected" || got.Tokens.Access != "" {
					t.Fatal("restored invalidated authorization")
				}
			} else if got.Tokens.Access != "old" {
				t.Fatal("previous grant lost")
			}
		})
	}
}

func TestConfiguredClientCredentialsRetriesWithoutRotatingRefresh(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Has("scope") {
			t.Error("empty scope must be omitted")
		}
		if r.Form.Get("client_secret") != "secret" || r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("refresh_token") != "" {
			t.Error("incorrect client authentication")
		}
		if calls.Add(1) == 1 {
			w.WriteHeader(503)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "new", "token_type": "Bearer", "expires_in": 3600, "refresh_token": "must-not-use"})
	}))
	defer srv.Close()
	r := configuredRecord(srv.URL)
	r.GrantType = "client_credentials"
	r.State = "pending"
	r.Tokens = Tokens{}
	r.Client.Secret = "secret"
	r.Client.AuthMethod = "client_secret_post"
	store := &memoryStore{r: r}
	s, _ := NewConfigured(r, nil)
	if _, err := s.Resolve(context.Background(), store, r.ID, r.Resource, ""); err == nil {
		t.Fatal("expected transient failure")
	}
	got, err := s.Resolve(context.Background(), store, r.ID, r.Resource, "")
	if err != nil || got.Tokens.Access != "new" || got.Tokens.Refresh != "" || got.RefreshPending {
		t.Fatalf("retry: %v", err)
	}
	if _, err = s.Resolve(context.Background(), store, r.ID, r.Resource, ""); err != nil || calls.Load() != 2 {
		t.Fatalf("cache: %v calls=%d", err, calls.Load())
	}
}

func TestConfiguredFailedRotationCannotReuseRefreshAfterRestart(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "new", "refresh_token": "rotated", "token_type": "Bearer"})
	}))
	defer srv.Close()
	r := configuredRecord(srv.URL)
	store := &memoryStore{r: r, failRevision: 3}
	s, _ := NewConfigured(r, nil)
	if _, err := s.Resolve(context.Background(), store, r.ID, r.Resource, ""); err == nil {
		t.Fatal("reported unpersisted grant")
	}
	store.failRevision = 0
	store.r.RefreshUntil = time.Time{}
	restarted, _ := NewConfigured(store.r, nil)
	if _, err := restarted.Resolve(context.Background(), store, r.ID, r.Resource, ""); !errors.Is(err, ErrReauthorize) {
		t.Fatalf("restart: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatal("reused rotating token")
	}
}

func TestConfiguredBasicAuthenticationOmitsBodyClientID(t *testing.T) {
	for _, grant := range []string{"authorization_code", "client_credentials"} {
		t.Run(grant, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_ = r.ParseForm()
				user, secret, ok := r.BasicAuth()
				if !ok || user != "client" || secret != "secret" || r.Form.Get("client_id") != "" || r.Form.Get("client_secret") != "" {
					t.Error("duplicated or missing authentication")
					w.WriteHeader(400)
					return
				}
				expected := "refresh_token"
				if grant == "client_credentials" {
					expected = grant
				}
				if r.Form.Get("grant_type") != expected {
					t.Error("wrong grant")
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "NEW", "token_type": "Bearer", "expires_in": 3600})
			}))
			defer srv.Close()
			r := configuredRecord(srv.URL)
			r.GrantType = grant
			r.Client.Secret = "secret"
			r.Client.AuthMethod = "client_secret_basic"
			if grant == "client_credentials" {
				r.State = "pending"
				r.Tokens = Tokens{}
			}
			store := &memoryStore{r: r}
			service, _ := NewConfigured(r, nil)
			got, err := service.Resolve(WithNetworkOperation(context.Background()), store, r.ID, r.Resource, "")
			if err != nil || got.Tokens.Access != "NEW" {
				t.Fatalf("Basic grant failed: %v", err)
			}
		})
	}
}

func TestConfiguredRejectsChangedConsumerGrantAndScopes(t *testing.T) {
	for _, field := range []string{"grant", "scopes", "consumer"} {
		t.Run(field, func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(500) }))
			defer srv.Close()
			r := configuredRecord(srv.URL)
			r.GrantType = "client_credentials"
			r.RequestedScopes = []string{"read"}
			r.GrantedScopes = []string{"read"}
			r.Client.Secret = "secret"
			r.Client.AuthMethod = "client_secret_post"
			service, err := NewConfigured(r, nil)
			if err != nil {
				t.Fatal(err)
			}
			switch field {
			case "grant":
				r.GrantType = "authorization_code"
			case "scopes":
				r.RequestedScopes[0] = "write"
			case "consumer":
				r.ConsumerID = "another"
			}
			r.Revision++
			store := &memoryStore{r: r}
			if _, err = service.Resolve(WithNetworkOperation(context.Background()), store, r.ID, r.Resource, ""); !errors.Is(err, ErrResource) {
				t.Fatalf("stale config accepted: %v", err)
			}
			persisted, _ := store.Load(context.Background(), r.ID)
			if calls.Load() != 0 || persisted.Revision != r.Revision || persisted.RefreshPending {
				t.Fatal("stale service issued or persisted grant")
			}
		})
	}
}

func TestConfiguredClientGrantWaitsForScopeCorrection(t *testing.T) {
	for _, invalidScope := range []bool{false, true} {
		t.Run(fmt.Sprint(invalidScope), func(t *testing.T) {
			var calls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if invalidScope && calls.Load() == 1 {
					w.WriteHeader(http.StatusBadRequest)
					_, _ = w.Write([]byte(`{"error":"invalid_scope","error_description":"SECRET"}`))
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "TOKEN", "token_type": "Bearer", "scope": "read"})
			}))
			defer srv.Close()
			r := configuredRecord(srv.URL)
			r.GrantType = "client_credentials"
			r.State = "pending"
			r.Tokens = Tokens{}
			r.RequestedScopes = []string{"read", "write"}
			r.Client.Secret = "secret"
			r.Client.AuthMethod = "client_secret_post"
			store := &memoryStore{r: r}
			for range 3 {
				current, _ := store.Load(context.Background(), r.ID)
				service, _ := NewConfigured(current, nil)
				if _, err := service.Resolve(WithNetworkOperation(context.Background()), store, r.ID, r.Resource, ""); !errors.Is(err, ErrPermission) {
					t.Fatalf("permission not reported: %v", err)
				}
			}
			if calls.Load() != 1 {
				t.Fatal("permission failure repeated grants")
			}
			edited, _ := store.Load(context.Background(), r.ID)
			edited.RequestedScopes = []string{"read"}
			edited.State = "pending"
			edited.Tokens = Tokens{}
			edited.Revision++
			if err := store.CompareAndSwap(context.Background(), edited, edited.Revision-1); err != nil {
				t.Fatal(err)
			}
			service, _ := NewConfigured(edited, nil)
			got, err := service.Resolve(WithNetworkOperation(context.Background()), store, r.ID, r.Resource, "")
			if err != nil || got.State != "connected" || calls.Load() != 2 {
				t.Fatalf("scope correction did not recover: %v", err)
			}

		})
	}
}

func TestConfiguredRegistrationCheckpointPreservesGrantAndFencesLateWrites(t *testing.T) {
	r := configuredRecord("https://resource.example")
	store := &memoryStore{r: r}
	service, _ := NewConfigured(r, nil)
	var late func(Record) (Record, error)
	_, err := service.AuthorizeUsingCheckpoint(context.Background(), store, r.ID, func(ctx context.Context, candidate Record, checkpoint func(Record) (Record, error)) (Record, error) {
		late = checkpoint
		candidate.Client.ID, candidate.Client.Method = "registered", "dcr"
		candidate.Tokens.Access = "must-not-replace"
		saved, err := checkpoint(candidate)
		if err != nil || saved.Tokens.Access != "old" {
			t.Fatalf("checkpoint replaced grant: %v", err)
		}
		return Record{}, errors.New("consent denied")
	})
	if err == nil {
		t.Fatal("expected failure")
	}
	saved, _ := store.Load(context.Background(), r.ID)
	if saved.PendingRegistration == nil || saved.PendingRegistration.Client.ID != "registered" || saved.Client.ID != r.Client.ID || saved.Tokens.Access != "old" || saved.AuthorizationActive() {
		t.Fatal("checkpoint or previous grant lost")
	}
	if _, err := late(saved); err == nil {
		t.Fatal("late checkpoint accepted")
	}
}

func TestConfiguredRefusedCandidateKeepsOriginalRefreshBinding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_ = req.ParseForm()
		if req.Form.Get("client_id") != "client" || req.Form.Get("refresh_token") != "refresh" {
			t.Error("refresh used candidate client")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "renewed", "token_type": "Bearer", "expires_in": 3600})
	}))
	defer srv.Close()
	r := configuredRecord(srv.URL)
	store := &memoryStore{r: r}
	service, _ := NewConfigured(r, nil)
	_, err := service.AuthorizeUsingCheckpoint(context.Background(), store, r.ID, func(ctx context.Context, candidate Record, checkpoint func(Record) (Record, error)) (Record, error) {
		candidate.Client.ID, candidate.Client.Method = "different-client", "dcr"
		if _, err := checkpoint(candidate); err != nil {
			t.Fatal(err)
		}
		return Record{}, errors.New("consent denied")
	})
	if err == nil {
		t.Fatal("expected refused consent")
	}
	saved, _ := store.Load(context.Background(), r.ID)
	service, _ = NewConfigured(saved, nil)
	renewed, err := service.Resolve(WithNetworkOperation(context.Background()), store, r.ID, r.Resource, "")
	if err != nil || renewed.Tokens.Access != "renewed" {
		t.Fatalf("original grant lost: %v", err)
	}
	_, err = service.AuthorizeUsingCheckpoint(context.Background(), store, r.ID, func(ctx context.Context, candidate Record, _ func(Record) (Record, error)) (Record, error) {
		if candidate.Client.ID != "different-client" {
			t.Fatal("candidate was not reused")
		}
		candidate.Tokens = Tokens{Access: "candidate-token", Type: "Bearer"}
		return candidate, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	saved, _ = store.Load(context.Background(), r.ID)
	if saved.PendingRegistration != nil || saved.Client.ID != "different-client" || saved.Tokens.Access != "candidate-token" {
		t.Fatal("candidate was not promoted atomically")
	}
}

func TestConfiguredClientGrantReportsConfigurationErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
	}))
	defer srv.Close()
	for _, missing := range []string{"id", "secret", "endpoint", "rejected", "removed"} {
		t.Run(missing, func(t *testing.T) {
			r := configuredRecord(srv.URL)
			r.GrantType, r.State = "client_credentials", "pending"
			r.Client.Secret = "secret"
			switch missing {
			case "id":
				r.Client.ID = ""
			case "secret":
				r.Client.Secret = ""
			case "endpoint":
				r.Endpoints.Token = ""
			}
			store := &memoryStore{r: r}
			service, err := NewConfigured(r, nil)
			if err != nil {
				t.Fatal(err)
			}
			if missing == "removed" {
				if err := service.InvalidateAndClearClientSecret(context.Background(), store, r.ID); err != nil {
					t.Fatal(err)
				}
			}
			before := calls.Load()
			_, err = service.Resolve(WithNetworkOperation(context.Background()), store, r.ID, r.Resource, "")
			if !errors.Is(err, ErrClientConfiguration) || errors.Is(err, ErrReauthorize) {
				t.Fatalf("wrong guidance: %v", err)
			}
			if missing != "rejected" && calls.Load() != before {
				t.Fatal("incomplete configuration sent request")
			}
		})
	}
}
