package mcp

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"assistente/internal/credentials"
	"assistente/internal/database"
)

// Bearer/Basic retain their existing hostname bindings, but source resolution
// and command rejection/cache handling belong to the shared CredentialTransport.
func (m *Manager) credentialHTTPClient(cfg ServerConfig) *http.Client {
	owner, _ := database.UserIDFromContext(m.credentialContext())
	transport := &mcpCredentialTransport{manager: m, owner: owner, resource: cfg.URL, expectedType: string(cfg.AuthType), base: newMCPTransport()}
	return &http.Client{Transport: transport, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 || !sameCredentialOrigin(cfg.URL, req.URL) {
			return credentials.ErrCredentialResolution
		}
		return nil
	}}
}

type mcpCredentialTransport struct {
	manager      *Manager
	owner        string
	resource     string
	expectedType string
	base         http.RoundTripper
}

func sameCredentialOrigin(resource string, target *url.URL) bool {
	origin, err := url.Parse(resource)
	return err == nil && target != nil && origin.User == nil && target.User == nil &&
		(origin.Scheme == "https" || origin.Scheme == "http") &&
		strings.EqualFold(origin.Scheme, target.Scheme) && strings.EqualFold(origin.Hostname(), target.Hostname()) &&
		credentialOriginPort(origin) == credentialOriginPort(target)
}

func credentialOriginPort(u *url.URL) string {
	if port := u.Port(); port != "" {
		return port
	}
	if strings.EqualFold(u.Scheme, "https") {
		return "443"
	}
	return "80"
}

func (t *mcpCredentialTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	fail := func() (*http.Response, error) {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, fmt.Errorf("%w: MCP", credentials.ErrCredentialResolution)
	}
	current, _ := database.UserIDFromContext(t.manager.credentialContext())
	if current != t.owner || t.manager.credMgr == nil || !sameCredentialOrigin(t.resource, req.URL) {
		return fail()
	}
	ctx := req.Context()
	if caller, ok := database.UserIDFromContext(ctx); ok && caller != t.owner {
		return fail()
	}
	if t.owner != "" {
		ctx = database.WithUserID(ctx, t.owner)
	}
	pattern, err := t.manager.credMgr.PatternForURLWithContext(ctx, t.resource)
	if err != nil || pattern == "" {
		return fail()
	}
	// Do not attach configuration snapshots or materialized secrets to the client.
	// All requests, including retries and SSE endpoint requests, resolve centrally.
	transport := credentials.NewCredentialTransport(t.manager.credMgr, pattern)
	transport.Base = t.base
	transport.ExpectedType = t.expectedType
	return transport.RoundTrip(req.Clone(ctx))
}

func (t *mcpCredentialTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
