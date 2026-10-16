package app

import (
	"assistente/internal/database"
	"assistente/internal/oauthflow"
	httpclient "assistente/internal/tools/http"
	"context"
	"net"
	"net/url"
)

// Reuses the network allowlist, category escalation rules and DecisionDialog.
// The OAuth core knows neither UI nor persistence and cannot grant its own trust.
func oauthNetworkAuthorizer(authorizer httpclient.NetworkAuthorizer) oauthflow.NetworkAuthorizer {
	return func(ctx context.Context, destination oauthflow.NetworkDestination) ([]net.IP, bool, error) {
		if _, err := database.RequireUserID(ctx); err != nil {
			return nil, false, err
		}
		target, err := url.Parse(destination.URL)
		if err != nil || target.Hostname() == "" || authorizer == nil {
			return nil, false, err
		}
		port := target.Port()
		explicit := port != ""
		if port == "" {
			if target.Scheme == "https" {
				port = "443"
			} else {
				port = "80"
			}
		}
		target.User = nil
		target.RawQuery = ""
		target.Fragment = ""
		target.ForceQuery = false
		return authorizer.Authorize(ctx, httpclient.BlockedDestination{
			Host: target.Hostname(), Port: port, PortExplicit: explicit, URL: target.String(), IPs: destination.IPs,
			Category: httpclient.MostSensitiveCategory(target.Hostname(), destination.IPs), Reason: "oauth_discovery_or_registration",
		})
	}
}
