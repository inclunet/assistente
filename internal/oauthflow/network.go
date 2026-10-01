package oauthflow

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"syscall"
	"time"

	"assistente/internal/networkpolicy"
)

var ErrNetworkAuthorization = errors.New("oauth_discovery_destination_blocked")

// NetworkDestination contains only sanitized display data. The application adapter
// delegates decisions and persistence to the existing network trust engine.
type NetworkDestination struct {
	URL string
	IPs []net.IP
}
type NetworkAuthorizer func(context.Context, NetworkDestination) ([]net.IP, bool, error)
type networkAuthorizerKey struct{}

// WithNetworkAuthorizer scopes refusals to this operation/connection, including
// retries performed by HTTP/OAuth SDKs. A new explicit connection gets a new scope.
func WithNetworkAuthorizer(ctx context.Context, authorize NetworkAuthorizer) context.Context {
	if ctx.Value(networkAuthorizerKey{}) != nil {
		return ctx
	}
	return context.WithValue(ctx, networkAuthorizerKey{}, &networkAuthorization{authorize: authorize})
}

type networkAuthorization struct {
	mu        sync.Mutex
	authorize NetworkAuthorizer
	denied    error
}

func NetworkAuthorizationError(ctx context.Context) error {
	state, _ := ctx.Value(networkAuthorizerKey{}).(*networkAuthorization)
	if state == nil {
		return nil
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.denied
}
func (state *networkAuthorization) decide(ctx context.Context, d NetworkDestination) ([]net.IP, bool, error) {
	state.mu.Lock()
	denied := state.denied
	state.mu.Unlock()
	if denied != nil {
		return nil, false, denied
	}
	if state.authorize == nil {
		return nil, false, nil
	}
	ips, ok, err := state.authorize(ctx, d)
	if !ok || err != nil || len(ips) == 0 {
		state.mu.Lock()
		state.denied = errors.Join(ErrNetworkAuthorization, err)
		state.mu.Unlock()
	}
	return ips, ok, err
}

type discoveryNetworkKey struct{}
type discoveryNetwork struct {
	mu            sync.Mutex
	origin        string
	ips           map[string]bool
	trusted       map[string]map[string]bool
	pending       *NetworkDestination
	pendingOrigin string
	lookup        func(context.Context, string) ([]net.IPAddr, error)
}
type blockedOAuthIP struct{ ip net.IP }

func (e *blockedOAuthIP) Error() string { return ErrNetworkAuthorization.Error() }
func (e *blockedOAuthIP) Unwrap() error { return ErrNetworkAuthorization }

func endpointURL(raw string) (*url.URL, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || u.Opaque != "" || (u.Scheme != "https" && u.Scheme != "http") {
		return nil, ErrNetworkAuthorization
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	ip := net.ParseIP(host)
	if u.Scheme == "http" && host != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, ErrNetworkAuthorization
	}
	return u, nil
}
func networkOrigin(u *url.URL) string {
	port := u.Port()
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	return u.Scheme + "://" + net.JoinHostPort(strings.ToLower(strings.TrimSuffix(u.Hostname(), ".")), port)
}

func withDiscoveryNetwork(ctx context.Context, raw string) context.Context {
	if ctx.Value(discoveryNetworkKey{}) != nil {
		return ctx
	}
	p := &discoveryNetwork{ips: map[string]bool{}, trusted: map[string]map[string]bool{}, lookup: net.DefaultResolver.LookupIPAddr}
	u, err := url.Parse(raw)
	if err == nil && u.Hostname() != "" && u.User == nil && (u.Scheme == "https" || u.Scheme == "http") {
		p.origin = networkOrigin(u)
		lookupCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		ips, lookupErr := p.lookup(lookupCtx, u.Hostname())
		cancel()
		if lookupErr == nil {
			for _, ip := range ips {
				p.ips[ip.IP.String()] = true
			}
		}
	}
	return context.WithValue(ctx, discoveryNetworkKey{}, p)
}

// Authorization is outside HTTP/discovery deadlines, but retains caller cancellation
// and identity. Only socket/preflight refusals trigger replay, never HTTP failures.
func runNetworkOperation(ctx context.Context, resource string, run func(context.Context)) error {
	ctx = withDiscoveryNetwork(ctx, resource)
	p := ctx.Value(discoveryNetworkKey{}).(*discoveryNetwork)
	state, _ := ctx.Value(networkAuthorizerKey{}).(*networkAuthorization)
	for attempt := 0; attempt <= 8; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		p.mu.Lock()
		p.pending = nil
		p.mu.Unlock()
		run(ctx)
		p.mu.Lock()
		pending, origin := p.pending, p.pendingOrigin
		p.mu.Unlock()
		if pending == nil {
			return nil
		}
		if state == nil || state.authorize == nil || attempt == 8 {
			return ErrNetworkAuthorization
		}
		ips, ok, err := state.decide(ctx, *pending)
		if err != nil {
			return errors.Join(ErrNetworkAuthorization, err)
		}
		if !ok || len(ips) == 0 {
			return ErrNetworkAuthorization
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		allowed := map[string]bool{}
		for _, ip := range ips {
			if ip != nil {
				allowed[ip.String()] = true
			}
		}
		p.mu.Lock()
		p.trusted[origin] = allowed
		p.mu.Unlock()
	}
	return ErrNetworkAuthorization
}
func (p *discoveryNetwork) blocked(u *url.URL, ips []net.IP) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.pending == nil {
		p.pending = &NetworkDestination{URL: sanitizeURLHint(u.String()), IPs: ips}
		p.pendingOrigin = networkOrigin(u)
	}
	return ErrNetworkAuthorization
}
func (p *discoveryNetwork) permits(u *url.URL, ip net.IP) bool {
	if ip == nil {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	origin := networkOrigin(u)
	return (origin == p.origin && p.ips[ip.String()]) || p.trusted[origin][ip.String()] || !networkpolicy.IsBlockedIP(ip)
}
func (p *discoveryNetwork) checkDestination(ctx context.Context, u *url.URL) error {
	if u == nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ErrNetworkAuthorization
	}
	if networkOrigin(u) != p.origin {
		if _, err := endpointURL(u.String()); err != nil {
			return err
		}
	}
	resolved, err := p.lookup(ctx, u.Hostname())
	if err != nil {
		return err
	}
	ips := make([]net.IP, 0, len(resolved))
	blocked := false
	for _, address := range resolved {
		ips = append(ips, address.IP)
		if !p.permits(u, address.IP) {
			blocked = true
		}
	}
	if len(ips) == 0 {
		return ErrNetworkAuthorization
	}
	if blocked {
		return p.blocked(u, ips)
	}
	return nil
}

// Every connection checks the actual post-DNS address. No environment proxies or
// connection pools are shared across trust decisions.
func discoveryTransport(p *discoveryNetwork, target *url.URL) *http.Transport {
	base, ok := http.DefaultTransport.(*http.Transport)
	transport := &http.Transport{TLSHandshakeTimeout: 5 * time.Second}
	if ok {
		transport = base.Clone()
	}
	transport.Proxy = nil
	transport.DisableKeepAlives = true
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		dialer := net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}
		dialer.Control = func(_, actual string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(actual)
			if err != nil {
				return ErrNetworkAuthorization
			}
			ip := net.ParseIP(host)
			if !p.permits(target, ip) {
				return &blockedOAuthIP{ip: ip}
			}
			return nil
		}
		return dialer.DialContext(ctx, network, address)
	}
	return transport
}

type discoveryRoundTripper struct{ policy *discoveryNetwork }

func (r discoveryRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	if err := r.policy.checkDestination(request.Context(), request.URL); err != nil {
		if request.Body != nil {
			_ = request.Body.Close()
		}
		return nil, err
	}
	transport := discoveryTransport(r.policy, request.URL)
	response, err := transport.RoundTrip(request)
	var blocked *blockedOAuthIP
	if errors.As(err, &blocked) {
		return nil, r.policy.blocked(request.URL, []net.IP{blocked.ip})
	}
	return response, err
}
func validProtectedResource(ctx context.Context, configured, resource string) bool {
	original, e1 := url.Parse(configured)
	discovered, e2 := url.Parse(resource)
	if e1 != nil || e2 != nil || discovered.Hostname() == "" || discovered.User != nil || discovered.Fragment != "" {
		return false
	}
	// Legacy resource aliases may canonicalize the path, but cannot change origin.
	return networkOrigin(original) == networkOrigin(discovered)
}
func validDiscoveredMetadata(ctx context.Context, metadata *authServerMetadata, expectedIssuers []string) bool {
	if !validAuthServerMetadata(metadata) || !slices.Contains(expectedIssuers, metadata.Issuer) {
		return false
	}
	policy, ok := ctx.Value(discoveryNetworkKey{}).(*discoveryNetwork)
	if !ok {
		return false
	}
	issuer, err := endpointURL(metadata.Issuer)
	if err != nil || issuer.RawQuery != "" || policy.checkDestination(ctx, issuer) != nil {
		return false
	}
	for _, endpoint := range []string{metadata.AuthorizationEndpoint, metadata.TokenEndpoint, metadata.RegistrationEndpoint, metadata.DeviceAuthorizationEndpoint} {
		if endpoint == "" {
			continue
		}
		u, err := endpointURL(endpoint)
		if err != nil || policy.checkDestination(ctx, u) != nil {
			return false
		}
	}
	return true
}

// NewNetworkHTTPClient applies the same consent and socket guard to subsequent
// OAuth requests. Redirects are never followed with codes, tokens or secrets.
func NewNetworkHTTPClient(resource string, authorize NetworkAuthorizer, timeout time.Duration) *http.Client {
	return &http.Client{Transport: &authorizedOAuthTransport{resource: resource, authorize: authorize, timeout: timeout}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

type authorizedOAuthTransport struct {
	mu        sync.Mutex
	denied    error
	resource  string
	authorize NetworkAuthorizer
	timeout   time.Duration
}
type cancelBody struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (b *cancelBody) Close() error { defer b.cancel(); return b.ReadCloser.Close() }
func (t *authorizedOAuthTransport) RoundTrip(request *http.Request) (result *http.Response, resultErr error) {
	t.mu.Lock()
	denied := t.denied
	t.mu.Unlock()
	if denied != nil {
		if request.Body != nil {
			_ = request.Body.Close()
		}
		return nil, denied
	}
	defer func() {
		if errors.Is(resultErr, ErrNetworkAuthorization) {
			t.mu.Lock()
			t.denied = resultErr
			t.mu.Unlock()
		}
	}()

	if _, err := endpointURL(request.URL.String()); err != nil {
		return nil, err
	}
	ctx := request.Context()
	if t.authorize != nil {
		ctx = WithNetworkAuthorizer(ctx, t.authorize)
	}
	var response *http.Response
	var responseErr error
	attempts := 0
	err := runNetworkOperation(ctx, t.resource, func(operationCtx context.Context) {
		callCtx, cancel := context.WithTimeout(operationCtx, t.timeout)
		clone := request.Clone(callCtx)
		if attempts > 0 && request.Body != nil {
			if request.GetBody == nil {
				cancel()
				responseErr = ErrNetworkAuthorization
				return
			}
			clone.Body, responseErr = request.GetBody()
			if responseErr != nil {
				cancel()
				return
			}
		}
		attempts++
		policy := operationCtx.Value(discoveryNetworkKey{}).(*discoveryNetwork)
		response, responseErr = (discoveryRoundTripper{policy: policy}).RoundTrip(clone)
		if responseErr != nil {
			cancel()
			return
		}
		response.Body = &cancelBody{ReadCloser: response.Body, cancel: cancel}
	})
	if err != nil {
		return nil, err
	}
	return response, responseErr
}
