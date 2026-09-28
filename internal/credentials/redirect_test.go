package credentials

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestProviderRedirectPolicy(t *testing.T) {
	request := func(raw string) *http.Request {
		r, err := http.NewRequest("GET", raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	initial := request("https://provider.example/v1/models")
	for _, tc := range []struct {
		name, target string
		count        int
		rejected     bool
	}{
		{"same-origin", "https://provider.example/v2/models", 1, false},
		{"host", "https://other.example/models", 1, true},
		{"port", "https://provider.example:8443/models", 1, true},
		{"downgrade", "http://provider.example/models", 1, true},
		{"loop", "https://provider.example/models", 10, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			via := make([]*http.Request, tc.count)
			for i := range via {
				via[i] = initial
			}
			if rejected := SameOriginRedirect(request(tc.target), via) != nil; rejected != tc.rejected {
				t.Fatalf("rejected=%v", rejected)
			}
		})
	}
}

func TestCredentialClientsRejectCrossOriginRedirect(t *testing.T) {
	for _, kind := range []string{"generic", "auth-mode", "streaming"} {
		t.Run(kind, func(t *testing.T) {
			f := newCommandCacheFixture(t)
			var reached atomic.Int32
			destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reached.Add(1); w.WriteHeader(200) }))
			defer destination.Close()
			origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer first" {
					t.Error("credencial ausente na origem")
				}
				http.Redirect(w, r, destination.URL, http.StatusFound)
			}))
			defer origin.Close()
			var client *http.Client
			switch kind {
			case "generic":
				client = NewHTTPClient(f.m, "cache.example", 0)
			case "auth-mode":
				client = NewHTTPClientWithAuthMode(f.m, "cache.example", AuthRequired, 0)
			case "streaming":
				client = NewStreamingHTTPClientWithAuthMode(f.m, "cache.example", AuthRequired)
			}
			defer client.CloseIdleConnections()
			req, _ := http.NewRequestWithContext(f.ctx, "GET", origin.URL, nil)
			response, err := client.Do(req)
			if response != nil {
				_ = response.Body.Close()
			}
			if err == nil || reached.Load() != 0 {
				t.Fatal("cliente encaminhou credencial para outra origem")
			}
		})
	}
}
