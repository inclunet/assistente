package oauthflow

import (
	"context"
	"encoding/json"
	"errors"
	"golang.org/x/oauth2"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestDiscoveryCannotExpandPrivateTrust(t *testing.T) {
	for _, mode := range []string{"authorization-server", "resource", "redirect"} {
		t.Run(mode, func(t *testing.T) {
			var hits atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(500) }))
			defer target.Close()
			var origin string
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "redirect" {
					http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
					return
				}
				if mode == "resource" {
					_ = json.NewEncoder(w).Encode(map[string]any{"resource": target.URL})
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"resource": origin, "authorization_servers": []string{target.URL}})
			}))
			defer source.Close()
			origin = source.URL
			result := DiscoverOAuth(source.URL)
			if result.Found || hits.Load() != 0 {
				t.Fatalf("private trust expanded: found=%v hits=%d", result.Found, hits.Load())
			}
		})
	}
}

func TestClientGrantRefusesDNSChangeWithoutConsentInsideLease(t *testing.T) {
	ctx := WithNetworkOperation(context.Background())
	scope := ctx.Value(networkOperationKey{}).(*networkApprovals)
	ip := "10.0.0.1"
	policy := &discoveryNetwork{scope: scope, origin: "https://resource.example:443", ips: map[string]bool{}, trusted: map[string]map[string]bool{}, lookup: func(context.Context, string) ([]net.IPAddr, error) { return []net.IPAddr{{IP: net.ParseIP(ip)}}, nil }}
	ctx = context.WithValue(ctx, discoveryNetworkKey{}, policy)
	prompts := 0
	authorize := func(_ context.Context, d NetworkDestination) ([]net.IP, bool, error) {
		prompts++
		return d.IPs, true, nil
	}
	approved, err := PreflightOAuthEndpoint(ctx, "https://resource.example", "https://token.example/token", authorize)
	if err != nil || prompts != 1 {
		t.Fatal("initial preflight failed", err, prompts)
	}
	if approved.Value(discoveryNetworkKey{}) != policy {
		t.Fatal("preflight lost pinned policy")
	}
	ip = "10.0.0.2"
	_, err = RequestClientCredentialsToken(approved, ClientCredentialsConfig{Resource: "https://resource.example", TokenEndpoint: "https://token.example/token", ClientID: "client", ClientSecret: "secret"}, authorize)
	if !errors.Is(err, ErrNetworkAuthorization) || prompts != 1 {
		t.Fatal("DNS change prompted or reached grant", err, prompts)
	}
	// Only a new preflight outside the lease may ask about the new destination.
	if _, err := PreflightOAuthEndpoint(ctx, "https://resource.example", "https://token.example/token", authorize); err != nil || prompts != 2 {
		t.Fatal("new preflight cannot recover", err, prompts)
	}
	if _, err := RequestClientCredentialsToken(context.Background(), ClientCredentialsConfig{}, authorize); !errors.Is(err, ErrNetworkAuthorization) {
		t.Fatal("grant allowed without preflight", err)
	}
}

func TestOAuthPreflightBoundsDNSWithoutBoundingConsent(t *testing.T) {
	target, _ := url.Parse("https://private.example/token")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var lookupContext context.Context
	policy := &discoveryNetwork{lookup: func(ctx context.Context, _ string) ([]net.IPAddr, error) {
		lookupContext = ctx
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("DNS has no bounded deadline")
		}
		return []net.IPAddr{{IP: net.ParseIP("10.0.0.1")}}, nil
	}}
	if err := policy.checkPreflightDestination(ctx, target); !errors.Is(err, ErrNetworkAuthorization) {
		t.Fatalf("expected consent request: %v", err)
	}
	if !errors.Is(lookupContext.Err(), context.Canceled) {
		t.Fatal("DNS scope was not released")
	}
	if ctx.Err() != nil {
		t.Fatal("DNS cancellation affected consent context")
	}
	if _, ok := ctx.Deadline(); ok {
		t.Fatal("consent acquired DNS deadline")
	}
	policy.lookup = func(ctx context.Context, _ string) ([]net.IPAddr, error) {
		cancel()
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if err := policy.checkPreflightDestination(ctx, target); !errors.Is(err, context.Canceled) {
		t.Fatalf("caller cancellation lost: %v", err)
	}
}
func TestDiscoverySocketRejectsPrivateDNSResult(t *testing.T) {
	target, _ := url.Parse("https://public.example/metadata")
	policy := &discoveryNetwork{origin: networkOrigin(target), ips: map[string]bool{"8.8.8.8": true}}
	transport := discoveryTransport(policy, target)
	defer transport.CloseIdleConnections()
	for _, address := range []string{"127.0.0.1:443", "10.0.0.1:443", "169.254.169.254:443", "100.64.0.1:443", "[::1]:443", "[::ffff:127.0.0.1]:443", "0.0.0.0:443", "224.0.0.1:443"} {
		conn, err := transport.DialContext(context.Background(), "tcp", address)
		if conn != nil {
			_ = conn.Close()
			t.Fatalf("connected to %s", address)
		}
		if !errors.Is(err, ErrNetworkAuthorization) {
			t.Fatalf("unguarded address %s: %v", address, err)
		}
	}
}
func TestDiscoveredMetadataRejectsUnsafeEndpointsAndIssuer(t *testing.T) {
	ctx := context.WithValue(context.Background(), discoveryNetworkKey{}, &discoveryNetwork{origin: "https://mcp.example:443", lookup: func(_ context.Context, host string) ([]net.IPAddr, error) {
		ip := net.ParseIP(host)
		if ip == nil {
			if host == "localhost" {
				ip = net.ParseIP("127.0.0.1")
			} else {
				ip = net.ParseIP("8.8.8.8")
			}
		}
		return []net.IPAddr{{IP: ip}}, nil
	}})
	for _, endpoint := range []string{"file:///secret", "javascript:alert(1)", "http://public.example/token", "https://user:secret@example.com/token", "https://example.com/token#secret", "https://127.0.0.1/token", "https://10.0.0.1/token", "https://169.254.169.254/token", "https://localhost/token"} {
		for _, field := range []string{"token", "authorization", "device", "registration"} {
			metadata := &authServerMetadata{Issuer: "https://issuer.example", TokenEndpoint: "https://tokens.example/token"}
			switch field {
			case "token":
				metadata.TokenEndpoint = endpoint
			case "authorization":
				metadata.AuthorizationEndpoint = endpoint
			case "device":
				metadata.DeviceAuthorizationEndpoint = endpoint
			case "registration":
				metadata.RegistrationEndpoint = endpoint
			}
			if validDiscoveredMetadata(ctx, metadata, []string{metadata.Issuer}) {
				t.Fatalf("accepted %s %s", field, endpoint)
			}
		}
	}
	metadata := &authServerMetadata{Issuer: "https://issuer.example", TokenEndpoint: "https://tokens.example/token"}
	if validDiscoveredMetadata(ctx, metadata, []string{"https://issuer.example/other"}) {
		t.Fatal("accepted mismatched issuer")
	}
	if !validDiscoveredMetadata(ctx, metadata, []string{metadata.Issuer}) {
		t.Fatal("rejected public cross-origin endpoint")
	}
}
func TestDynamicRegistrationCannotExpandPrivateTrust(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		_, _ = w.Write([]byte(`{"client_id":"wrong"}`))
	}))
	defer target.Close()
	_, err := RegisterDynamicClient(context.Background(), "https://192.0.2.1/mcp", target.URL, RegistrationRequest{})
	if !errors.Is(err, ErrRegistration) || hits.Load() != 0 {
		t.Fatalf("DCR expanded private trust: %v hits=%d", err, hits.Load())
	}
}
func TestDiscoveryRootIssuerFallback(t *testing.T) {
	var origin string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/oauth-authorization-server" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": origin, "token_endpoint": origin + "/token"})
	}))
	defer server.Close()
	origin = server.URL
	if result := DiscoverOAuth(server.URL + "/deep/mcp"); !result.Found {
		t.Fatalf("root issuer fallback failed: %+v", result)
	}
}

func TestDiscoveryPrivateRedirectUsesConsent(t *testing.T) {
	for _, approve := range []bool{false, true} {
		t.Run(map[bool]string{false: "deny", true: "approve"}[approve], func(t *testing.T) {
			var hits, prompts atomic.Int32
			var origin, targetURL string
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				_ = json.NewEncoder(w).Encode(map[string]any{"issuer": origin, "token_endpoint": targetURL + "/token"})
			}))
			defer target.Close()
			targetURL = target.URL
			source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/.well-known/oauth-authorization-server" {
					http.NotFound(w, r)
					return
				}
				http.Redirect(w, r, target.URL+"/metadata", http.StatusTemporaryRedirect)
			}))
			defer source.Close()
			origin = source.URL
			ctx := WithNetworkAuthorizer(context.Background(), func(ctx context.Context, d NetworkDestination) ([]net.IP, bool, error) {
				prompts.Add(1)
				if _, bounded := ctx.Deadline(); bounded {
					t.Error("network deadline leaked into human decision")
				}
				if d.URL != target.URL+"/metadata" {
					t.Errorf("wrong destination %s", d.URL)
				}
				return d.IPs, approve, nil
			})
			result := DiscoverOAuthContext(ctx, source.URL)
			if result.Found != approve || prompts.Load() != 1 || (!approve && hits.Load() != 0) {
				t.Fatalf("found=%v prompts=%d hits=%d", result.Found, prompts.Load(), hits.Load())
			}
		})
	}
}
func TestDCRApprovalTransmitsExactlyOnePOST(t *testing.T) {
	var posts, prompts atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		_, _ = w.Write([]byte(`{"client_id":"issued"}`))
	}))
	defer target.Close()
	ctx := WithNetworkAuthorizer(context.Background(), func(ctx context.Context, d NetworkDestination) ([]net.IP, bool, error) {
		prompts.Add(1)
		if posts.Load() != 0 {
			t.Error("POST sent before approval")
		}
		if _, bounded := ctx.Deadline(); bounded {
			t.Error("DCR timeout applies to human decision")
		}
		return d.IPs, true, nil
	})
	result, err := RegisterDynamicClient(ctx, "https://192.0.2.1/mcp", target.URL, RegistrationRequest{})
	if err != nil || result.ClientID != "issued" || posts.Load() != 1 || prompts.Load() != 1 {
		t.Fatalf("err=%v posts=%d prompts=%d", err, posts.Load(), prompts.Load())
	}
}
func TestDCRDoesNotRetryRemoteFailureAfterConsent(t *testing.T) {
	var posts atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { posts.Add(1); w.WriteHeader(500) }))
	defer target.Close()
	ctx := WithNetworkAuthorizer(context.Background(), func(_ context.Context, d NetworkDestination) ([]net.IP, bool, error) { return d.IPs, true, nil })
	_, err := RegisterDynamicClient(ctx, "https://192.0.2.1/mcp", target.URL, RegistrationRequest{})
	if !errors.Is(err, ErrRegistration) || posts.Load() != 1 {
		t.Fatalf("ambiguous registration retried: %v posts=%d", err, posts.Load())
	}
}
func TestNetworkConsentCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	ctx = WithNetworkAuthorizer(ctx, func(ctx context.Context, _ NetworkDestination) ([]net.IP, bool, error) {
		close(started)
		<-ctx.Done()
		return nil, false, ctx.Err()
	})
	done := make(chan error, 1)
	go func() {
		_, err := RegisterDynamicClient(ctx, "https://192.0.2.1/mcp", "http://127.0.0.1:1/register", RegistrationRequest{})
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("consent did not start")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("lost cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("consent did not cancel")
	}
}

func TestOAuthSDKDoesNotRepeatDeniedConsent(t *testing.T) {
	var prompts, posts atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { posts.Add(1); w.WriteHeader(500) }))
	defer target.Close()
	client := NewNetworkHTTPClient("https://192.0.2.1", func(context.Context, NetworkDestination) ([]net.IP, bool, error) {
		prompts.Add(1)
		return nil, false, nil
	}, time.Second)
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, client)
	cfg := oauth2.Config{ClientID: "client", ClientSecret: "secret", Endpoint: oauth2.Endpoint{TokenURL: target.URL}}
	_, err := cfg.Exchange(ctx, "code")
	if !errors.Is(err, ErrNetworkAuthorization) || prompts.Load() != 1 || posts.Load() != 0 {
		t.Fatalf("SDK retried consent: %v prompts=%d posts=%d", err, prompts.Load(), posts.Load())
	}
}
func TestOAuthHTTPClientBoundsResponseBodyRead(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()
	client := NewNetworkHTTPClient(server.URL, nil, 50*time.Millisecond)
	response, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	_, err = io.ReadAll(response.Body)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("response read not bounded: %v", err)
	}
}

func TestNetworkApprovalReusedOnlyForApprovedOriginAndIPs(t *testing.T) {
	calls := 0
	ctx := WithNetworkAuthorizer(context.Background(), func(_ context.Context, d NetworkDestination) ([]net.IP, bool, error) {
		calls++
		return d.IPs, true, nil
	})
	ctx = WithNetworkOperation(ctx)
	state := ctx.Value(networkAuthorizerKey{}).(*networkAuthorization)
	for _, raw := range []string{"https://internal.example/token", "https://internal.example/next", "https://internal.example:444/token"} {
		_, ok, err := state.decide(ctx, NetworkDestination{URL: raw, IPs: []net.IP{net.ParseIP("10.0.0.1")}})
		if !ok || err != nil {
			t.Fatalf("decision: %v", err)
		}
	}
	if calls != 2 {
		t.Fatalf("expected one decision per origin, got %d", calls)
	}
	_, _, _ = state.decide(ctx, NetworkDestination{URL: "https://internal.example/token", IPs: []net.IP{net.ParseIP("10.0.0.2")}})
	if calls != 3 {
		t.Fatal("DNS change reused unapproved IP")
	}
	ctx = WithNetworkOperation(ctx)
	_, _, _ = state.decide(ctx, NetworkDestination{URL: "https://internal.example/token", IPs: []net.IP{net.ParseIP("10.0.0.1")}})
	if calls != 4 {
		t.Fatal("approval escaped operation")
	}
}

func TestOAuthPollingReusesConsentAcrossHTTPClients(t *testing.T) {
	var prompts, hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.WriteHeader(http.StatusOK) }))
	defer server.Close()
	authorize := func(_ context.Context, d NetworkDestination) ([]net.IP, bool, error) {
		prompts.Add(1)
		return d.IPs, true, nil
	}
	ctx := WithNetworkOperation(WithNetworkAuthorizer(context.Background(), authorize))
	for i := 0; i < 3; i++ {
		client := NewNetworkHTTPClient("https://192.0.2.1/mcp", authorize, time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/token", strings.NewReader("grant_type=device_code"))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if prompts.Load() != 1 || hits.Load() != 3 {
		t.Fatalf("prompts=%d requests=%d", prompts.Load(), hits.Load())
	}
}

func TestDiscoveryInitialOriginCannotBypassTLS(t *testing.T) {
	for _, raw := range []string{"http://public.example/mcp", "http://10.0.0.1/mcp"} {
		t.Run(raw, func(t *testing.T) {
			u, err := url.Parse(raw)
			if err != nil {
				t.Fatal(err)
			}
			policy := &discoveryNetwork{origin: networkOrigin(u), lookup: func(context.Context, string) ([]net.IPAddr, error) {
				t.Fatal("insecure origin reached DNS")
				return nil, nil
			}}
			if err := policy.checkDestination(context.Background(), u); !errors.Is(err, ErrNetworkAuthorization) {
				t.Fatalf("insecure explicit origin accepted: %v", err)
			}
		})
	}
	for _, raw := range []string{"http://localhost/mcp", "http://127.0.0.1/mcp", "http://[::1]/mcp"} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatal(err)
		}
		ip := net.ParseIP("127.0.0.1")
		policy := &discoveryNetwork{origin: networkOrigin(u), ips: map[string]bool{ip.String(): true}, lookup: func(context.Context, string) ([]net.IPAddr, error) { return []net.IPAddr{{IP: ip}}, nil }}
		if err := policy.checkDestination(context.Background(), u); err != nil {
			t.Fatalf("local explicit origin rejected: %v", err)
		}
	}
}

func TestPublicDiscoveryRejectsRemoteHTTPBeforeDNS(t *testing.T) {
	var lookups atomic.Int32
	original := net.DefaultResolver
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(context.Context, string, string) (net.Conn, error) {
		lookups.Add(1)
		return nil, errors.New("unexpected DNS")
	}}
	t.Cleanup(func() { net.DefaultResolver = original })
	raw := "http://oauth-insecure.example/mcp"
	if result := DiscoverOAuthContext(context.Background(), raw); result.Found {
		t.Fatal("insecure discovery succeeded")
	}
	if _, err := DiscoverEndpoints(context.Background(), raw); err == nil {
		t.Fatal("insecure runtime discovery succeeded")
	}
	if lookups.Load() != 0 {
		t.Fatalf("insecure discovery performed %d DNS attempts", lookups.Load())
	}
}
