package oauthflow

import (
	"context"
	"encoding/json"
	"errors"
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
