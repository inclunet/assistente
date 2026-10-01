package oauthflow

import (
	"context"
	"errors"
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

type resourceTestRT func(*http.Request) (*http.Response, error)

func (f resourceTestRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestResourceOriginCheckedBeforeAuthentication(t *testing.T) {
	for _, target := range []string{"http://remote.example/mcp", "https://elsewhere.example/mcp", "https://mcp.example:444/mcp", "https://user:secret@mcp.example/mcp", "https://mcp.example/mcp#fragment"} {
		t.Run(target, func(t *testing.T) {
			calls := 0
			client := NewResourceHTTPClient("https://mcp.example/mcp", resourceTestRT(func(*http.Request) (*http.Response, error) { calls++; return nil, nil }))
			_, err := client.Get(target)
			if !errors.Is(err, ErrResourceDestination) || calls != 0 {
				t.Fatalf("unauthorized destination reached authentication: calls=%d err=%v", calls, err)
			}
		})
	}
	for _, origin := range []string{"http://remote.example/mcp", "https://user:secret@mcp.example/mcp"} {
		client := NewResourceHTTPClient(origin, resourceTestRT(func(*http.Request) (*http.Response, error) {
			t.Fatal("invalid configured origin reached authentication")
			return nil, nil
		}))
		if _, err := client.Get(origin); !errors.Is(err, ErrResourceDestination) {
			t.Fatal(err)
		}
	}
}

func TestResourceCorporateConsentAndStreaming(t *testing.T) {
	for _, allow := range []bool{true, false} {
		t.Run(map[bool]string{true: "approve", false: "deny"}[allow], func(t *testing.T) {
			var hits, prompts atomic.Int32
			release := make(chan struct{})
			defer close(release)
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("missing bearer")
				}
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, "data: first\n\n")
				w.(http.Flusher).Flush()
				select {
				case <-release:
				case <-r.Context().Done():
				}
			}))
			defer server.Close()
			previous := http.DefaultTransport
			http.DefaultTransport = server.Client().Transport
			defer func() { http.DefaultTransport = previous }()
			target, _ := url.Parse(server.URL)
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			ctx = WithNetworkOperation(ctx)
			// Simulate an origin that initially resolved publicly and now resolves to
			// a private corporate endpoint. Trust must come from the existing authorizer.
			policy := &discoveryNetwork{scope: ctx.Value(networkOperationKey{}).(*networkApprovals), origin: networkOrigin(target), ips: map[string]bool{"8.8.8.8": true}, trusted: map[string]map[string]bool{}, lookup: func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
			}}
			ctx = context.WithValue(ctx, discoveryNetworkKey{}, policy)
			transport := NewResourceTransport(server.URL, func(ctx context.Context, d NetworkDestination) ([]net.IP, bool, error) {
				prompts.Add(1)
				if d.URL != server.URL+"/stream" || len(d.IPs) != 1 {
					t.Errorf("wrong destination: %+v", d)
				}
				return d.IPs, allow, nil
			}).(*authorizedOAuthTransport)
			// Resource streams must ignore the token-response deadline even if set.
			transport.timeout = time.Millisecond
			client := NewResourceHTTPClient(server.URL, transport)
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/stream", nil)
			req.Header.Set("Accept", "text/event-stream")
			req.Header.Set("Authorization", "Bearer test-token")
			response, err := client.Do(req)
			if !allow {
				if !errors.Is(err, ErrNetworkAuthorization) || hits.Load() != 0 || prompts.Load() != 1 {
					t.Fatalf("denial ignored: err=%v hits=%d prompts=%d", err, hits.Load(), prompts.Load())
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = response.Body.Close() }()
			first := make([]byte, len("data: first\n\n"))
			if _, err := io.ReadFull(response.Body, first); err != nil {
				t.Fatal(err)
			}
			time.Sleep(20 * time.Millisecond)
			if ctx.Err() != nil || prompts.Load() != 1 || hits.Load() != 1 {
				t.Fatal("stream/consent failed")
			}
			cancel()
			if _, err := response.Body.Read(make([]byte, 1)); !errors.Is(err, context.Canceled) {
				t.Fatalf("stream did not retain caller cancellation: %v", err)
			}
		})
	}
}

func TestResourceRedirectPolicy(t *testing.T) {
	client := NewResourceHTTPClient("https://mcp.example/path", nil)
	for _, target := range []string{"http://mcp.example/path", "https://mcp.example:444/path", "https://internal.example/path"} {
		req, _ := http.NewRequest(http.MethodGet, target, nil)
		if !errors.Is(client.CheckRedirect(req, nil), ErrResourceDestination) {
			t.Fatalf("redirect allowed: %s", target)
		}
	}
	req, _ := http.NewRequest(http.MethodGet, "https://MCP.example:443/other", nil)
	if err := client.CheckRedirect(req, nil); err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(req, make([]*http.Request, 10)); err == nil {
		t.Fatal("redirect loop allowed")
	}
}

func TestResourceSocketCannotBypassApproval(t *testing.T) {
	target, _ := url.Parse("https://mcp.example/resource")
	policy := &discoveryNetwork{origin: networkOrigin(target), ips: map[string]bool{"8.8.8.8": true}}
	transport := discoveryTransport(policy, target)
	defer transport.CloseIdleConnections()
	for _, addr := range []string{"127.0.0.1:443", "10.0.0.1:443", "169.254.169.254:443"} {
		conn, err := transport.DialContext(context.Background(), "tcp", addr)
		if conn != nil {
			_ = conn.Close()
			t.Fatal("unapproved socket connected")
		}
		if !errors.Is(err, ErrNetworkAuthorization) {
			t.Fatal(err)
		}
	}
}

func TestResourceRejectsWithoutReadingOrLeakingBody(t *testing.T) {
	body := &resourceClosingBody{Reader: strings.NewReader("secret")}
	req, _ := http.NewRequest(http.MethodPost, "https://other.example/private?secret=hidden", body)
	client := NewResourceHTTPClient("https://mcp.example", nil)
	_, err := client.Transport.RoundTrip(req)
	if !body.closed || err == nil || strings.Contains(err.Error(), "hidden") {
		t.Fatalf("body or error unsafe: closed=%v err=%v", body.closed, err)
	}
}

type resourceClosingBody struct {
	io.Reader
	closed bool
}

func (b *resourceClosingBody) Close() error { b.closed = true; return nil }

func TestHTTPExceptionRequiresActualLoopback(t *testing.T) {
	target, _ := url.Parse("http://localhost/resource")
	for _, address := range []string{"10.0.0.1", "8.8.8.8"} {
		t.Run(address, func(t *testing.T) {
			ip := net.ParseIP(address)
			policy := &discoveryNetwork{origin: networkOrigin(target), ips: map[string]bool{address: true}, trusted: map[string]map[string]bool{networkOrigin(target): {address: true}}, lookup: func(context.Context, string) ([]net.IPAddr, error) { return []net.IPAddr{{IP: ip}}, nil }}
			if err := policy.checkDestination(context.Background(), target); !errors.Is(err, ErrNetworkAuthorization) {
				t.Fatalf("HTTP reached non-loopback DNS: %v", err)
			}
			transport := discoveryTransport(policy, target)
			defer transport.CloseIdleConnections()
			conn, err := transport.DialContext(context.Background(), "tcp", net.JoinHostPort(address, "80"))
			if conn != nil {
				_ = conn.Close()
				t.Fatal("HTTP reached non-loopback socket")
			}
			if !errors.Is(err, ErrNetworkAuthorization) {
				t.Fatal(err)
			}
		})
	}
}
