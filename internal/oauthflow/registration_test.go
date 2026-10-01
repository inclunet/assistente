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

func TestDynamicRegistrationPreservesMetadata(t *testing.T) {
	metadata := RegistrationRequest{RedirectURIs: []string{"http://localhost:8989/exact/callback"}, ClientName: "Assistente", GrantTypes: []string{"authorization_code", "refresh_token", "urn:ietf:params:oauth:grant-type:device_code"}, ResponseTypes: []string{"code"}, TokenEndpointAuthMethod: "none", Scope: "read offline_access"}
	received := make(chan RegistrationRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Error("invalid registration request")
		}
		var got RegistrationRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		received <- got
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"client_id":"issued","client_secret":"sensitive"}`))
	}))
	defer server.Close()
	result, err := RegisterDynamicClient(context.Background(), server.URL, metadata)
	if err != nil || result.ClientID != "issued" || result.ClientSecret != "sensitive" {
		t.Fatalf("registration failed: %v", err)
	}
	if got := <-received; !reflect.DeepEqual(got, metadata) {
		t.Fatalf("metadata changed: %+v", got)
	}
}

func TestDynamicRegistrationRejectsUnsafeResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"error", 400, `{"error":"sensitive-token"}`},
		{"invalid-json", 200, `sensitive-token`},
		{"missing-client", 200, `{"client_secret":"sensitive-token"}`},
		{"empty-client", 200, `{"client_id":" "}`},
		{"oversize", 200, `{"client_id":"issued","padding":"` + strings.Repeat("x", registrationBodyLimit) + `"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			result, err := RegisterDynamicClient(context.Background(), server.URL+"?secret=sensitive-token", RegistrationRequest{})
			if result != nil || !errors.Is(err, ErrRegistration) {
				t.Fatalf("accepted invalid registration: %v", err)
			}
			if strings.Contains(err.Error(), "sensitive-token") || strings.Contains(err.Error(), server.URL) {
				t.Fatal("leaked remote diagnostics")
			}
		})
	}
}

func TestDynamicRegistrationRejectsRedirect(t *testing.T) {
	var calls atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		_, _ = w.Write([]byte(`{"client_id":"wrong"}`))
	}))
	defer target.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer server.Close()
	if _, err := RegisterDynamicClient(context.Background(), server.URL, RegistrationRequest{}); !errors.Is(err, ErrRegistration) {
		t.Fatalf("redirect accepted: %v", err)
	}
	if calls.Load() != 0 {
		t.Fatal("followed registration redirect")
	}
}

func TestDynamicRegistrationCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	defer close(release)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := RegisterDynamicClient(ctx, server.URL, RegistrationRequest{}); done <- err }()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("registration did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("lost cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("registration ignored cancellation")
	}
}

func TestDynamicRegistrationRejectsInvalidEndpoint(t *testing.T) {
	for _, endpoint := range []string{"file:///secret", "https://user:secret@example.com/register", "https://example.com/register#fragment", "https:///register"} {
		if _, err := RegisterDynamicClient(context.Background(), endpoint, RegistrationRequest{}); !errors.Is(err, ErrRegistration) {
			t.Fatalf("accepted endpoint: %v", err)
		}
	}
}
