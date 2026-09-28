package llm

import (
	"net/http"
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
			if rejected := sameOriginProviderRedirect(request(tc.target), via) != nil; rejected != tc.rejected {
				t.Fatalf("rejected=%v", rejected)
			}
		})
	}
}
