package app

import (
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	httpclient "assistente/internal/tools/http"
	"context"
	"net"
	"testing"
)

type oauthNetworkSpy struct {
	calls       int
	destination httpclient.BlockedDestination
	user        string
}

func (s *oauthNetworkSpy) Authorize(ctx context.Context, d httpclient.BlockedDestination) ([]net.IP, bool, error) {
	s.calls++
	s.destination = d
	s.user, _ = database.RequireUserID(ctx)
	return d.IPs, true, nil
}
func TestOAuthNetworkReusesScopedAuthorizer(t *testing.T) {
	spy := &oauthNetworkSpy{}
	authorize := oauthNetworkAuthorizer(spy)
	d := oauthflow.NetworkDestination{URL: "https://intranet.example:8443/oauth?secret=redact#fragment", IPs: []net.IP{net.ParseIP("10.0.0.2"), net.ParseIP("169.254.169.254")}}
	if _, _, err := authorize(context.Background(), d); err == nil || spy.calls != 0 {
		t.Fatal("unauthenticated authorization reached engine")
	}
	ips, ok, err := authorize(database.WithUserID(context.Background(), "user-test"), d)
	if err != nil || !ok || len(ips) != 2 || spy.calls != 1 || spy.user != "user-test" {
		t.Fatalf("scope/decision lost: %v", err)
	}
	got := spy.destination
	if got.Host != "intranet.example" || got.Port != "8443" || !got.PortExplicit || got.URL != "https://intranet.example:8443/oauth" || got.Category != httpclient.CategoryMetadata {
		t.Fatalf("wrong network request: %+v", got)
	}
}
