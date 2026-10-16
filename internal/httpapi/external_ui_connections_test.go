package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"assistente/internal/auth"

	"github.com/google/uuid"
)

func TestParseExternalUIContextQueryRequiresExactCanonicalParameters(t *testing.T) {
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	valid := url.Values{"connectionId": {id.String()}, "generation": {"12"}}
	connectionID, generation, ok := parseExternalUIContextQuery(valid)
	if !ok || connectionID != id.String() || generation != "12" {
		t.Fatalf("valid explicit query rejected: id=%q generation=%q ok=%v", connectionID, generation, ok)
	}
	cases := map[string]url.Values{
		"duplicate connection": {"connectionId": {id.String(), id.String()}, "generation": {"12"}},
		"duplicate generation": {"connectionId": {id.String()}, "generation": {"12", "13"}},
		"unknown parameter":    {"connectionId": {id.String()}, "generation": {"12"}, "extra": {"x"}},
		"missing parameter":    {"connectionId": {id.String()}},
		"noncanonical UUID":    {"connectionId": {"{" + id.String() + "}"}, "generation": {"12"}},
		"UUID v4":              {"connectionId": {uuid.NewString()}, "generation": {"12"}},
		"zero generation":      {"connectionId": {id.String()}, "generation": {"0"}},
		"leading zero":         {"connectionId": {id.String()}, "generation": {"012"}},
		"overflow generation":  {"connectionId": {id.String()}, "generation": {"18446744073709551616"}},
	}
	for name, query := range cases {
		t.Run(name, func(t *testing.T) {
			if _, _, ok := parseExternalUIContextQuery(query); ok {
				t.Fatalf("accepted invalid query: %#v", query)
			}
		})
	}
	if _, _, ok := parseExternalUIContextRawQuery("connectionId=" + id.String() + "&generation=1&bad=%zz"); ok {
		t.Fatal("accepted malformed percent-encoding")
	}
	if _, _, ok := parseExternalUIContextRawQuery(strings.Repeat("x", 129)); ok {
		t.Fatal("accepted oversized raw query")
	}
}

type externalUIQueryPort struct{ reads int }

func (*externalUIQueryPort) ConsumeInvitation(context.Context, string, auth.ExternalCommandPrincipal) (ExternalUIConnectionContext, error) {
	return ExternalUIConnectionContext{}, ErrExternalUIConnectionDenied
}

func (p *externalUIQueryPort) ReadConnection(context.Context, auth.ExternalCommandPrincipal, string, string) (ExternalUIConnectionContext, error) {
	p.reads++
	return ExternalUIConnectionContext{}, ErrExternalUIConnectionDenied
}

func (*externalUIQueryPort) RevokePrincipal(context.Context, string, string) {}

func TestExternalUIContextHTTPRejectsDuplicateAndUnknownQueryBeforeRead(t *testing.T) {
	f := newHTTPCommandFixture(t, false)
	port := &externalUIQueryPort{}
	f.server.externalUIConnections = port
	id, err := uuid.NewV7()
	if err != nil {
		t.Fatal(err)
	}
	cases := []string{
		"?connectionId=" + id.String() + "&connectionId=" + id.String() + "&generation=1",
		"?connectionId=" + id.String() + "&generation=1&extra=x",
		"?connectionId=" + id.String() + "&generation=01",
		"?connectionId=" + id.String(),
	}
	for _, query := range cases {
		req := httptest.NewRequest(http.MethodGet, "/auth/external/ui-connections/context"+query, nil)
		req.Header.Set("Authorization", "Bearer "+httpCommandToken)
		rec := httptest.NewRecorder()
		f.server.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("query %q returned %d: %s", query, rec.Code, rec.Body.String())
		}
	}
	if port.reads != 0 {
		t.Fatalf("strict query rejection reached connection store %d times", port.reads)
	}

}
