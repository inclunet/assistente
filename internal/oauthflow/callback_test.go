package oauthflow

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSharedCallbackRejectsInvalidRequestsWithoutConsuming(t *testing.T) {
	c, err := ReserveCallback(CallbackConfig{Host: "localhost", Path: "/callback", PortPolicy: "ephemeral"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Start("expected", CallbackPage{Success: "complete", Failure: "declined"}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path, query, host string
		status                    int
	}{
		{"POST", "/callback", "state=expected&code=ok", "", 404},
		{"GET", "/other", "state=expected&code=ok", "", 404},
		{"GET", "/call%62ack", "state=expected&code=ok", "", 404},
		{"GET", "/callback", "state=expected&code=ok", "attacker.example", 404},
		{"GET", "/callback", "state=wrong&error=access_denied", "", 400},
		{"GET", "/callback", "state=expected&state=expected&code=ok", "", 400},
		{"GET", "/callback", "state=expected&code=a&code=b", "", 400},
		{"GET", "/callback", "state=expected&code=a&error=denied", "", 400},
		{"GET", "/callback", "state=expected", "", 400},
		{"GET", "/callback", "state=expected&code=a&client_id=a&client_id=b", "", 400},
	} {
		target, _ := url.Parse(c.RedirectURI())
		target.Path = tc.path
		target.RawPath = ""
		target.RawQuery = tc.query
		raw := target.Scheme + "://" + target.Host + tc.path + "?" + tc.query
		req, _ := http.NewRequest(tc.method, raw, nil)
		if tc.host != "" {
			req.Host = tc.host
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != tc.status {
			t.Fatalf("%+v status=%d", tc, resp.StatusCode)
		}
	}
	resp, err := http.Get(c.RedirectURI() + "?state=expected&code=ok&client_id=issued")
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil || string(body) != "complete" || resp.ContentLength != 8 || resp.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("body=%q err=%v headers=%v", body, err, resp.Header)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	values, err := c.Wait(ctx)
	if err != nil || values.Get("code") != "ok" || values.Get("client_id") != "issued" {
		t.Fatalf("%v %v", values, err)
	}
	resp, err = http.Get(c.RedirectURI() + "?state=expected&code=replayed")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 409 {
		t.Fatal(resp.StatusCode)
	}
}
func TestSharedCallbackReservationAndCancellation(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", "[::1]"} {
		t.Run(host, func(t *testing.T) {
			c, err := ReserveCallback(CallbackConfig{Host: host, Path: "/callback", PortPolicy: "ephemeral"})
			if err != nil {
				if host == "[::1]" {
					t.Skipf("IPv6 unavailable: %v", err)
				}
				t.Fatal(err)
			}
			defer c.Close()
			u, _ := url.Parse(c.RedirectURI())
			if u.Host != host+":"+strconv.Itoa(c.Port()) {
				t.Fatal(u.Host)
			}
			collision, err := ReserveCallback(CallbackConfig{Host: host, Path: "/callback", PortPolicy: "fixed", Port: c.Port()})
			if collision != nil {
				collision.Close()
				t.Fatal("port stolen")
			}
			if !errors.Is(err, ErrCallbackPort) {
				t.Fatal(err)
			}
			if err := c.Start("state", CallbackPage{}); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := c.Wait(ctx); !errors.Is(err, context.Canceled) {
				t.Fatal(err)
			}
			c.Close()
			conn, err := net.DialTimeout("tcp", net.JoinHostPort(u.Hostname(), u.Port()), time.Second)
			if err == nil {
				_ = conn.Close()
				t.Fatal("listener left open")
			}
		})
	}
}
func TestSharedCallbackRejectsNonLoopbackAndInvalidPolicy(t *testing.T) {
	for _, cfg := range []CallbackConfig{
		{Host: "0.0.0.0", Path: "/callback", PortPolicy: "ephemeral"},
		{Host: "example.com", Path: "/callback", PortPolicy: "ephemeral"},
		{Host: "127.0.0.1", Path: "/callback?x=1", PortPolicy: "ephemeral"},
		{Host: "127.0.0.1", Path: "/a/../callback", PortPolicy: "ephemeral"},
		{Host: "127.0.0.1", Path: "/callback", PortPolicy: "fixed"},
	} {
		c, err := ReserveCallback(cfg)
		if c != nil {
			c.Close()
			t.Fatal("invalid callback accepted")
		}
		if !errors.Is(err, ErrResource) {
			t.Fatal(err)
		}
	}
}

func TestHTMLCallbackUsesFreshNonceWithoutRelaxingPlaintext(t *testing.T) {
	nonces := map[string]bool{}
	for _, mode := range []string{"success", "refused", "plaintext"} {
		t.Run(mode, func(t *testing.T) {
			c, err := ReserveCallback(CallbackConfig{Host: "127.0.0.1", Path: "/callback", PortPolicy: "ephemeral"})
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			const template = `<style nonce="` + CallbackNoncePlaceholder + `">body{font-family:sans-serif}</style><script nonce="` + CallbackNoncePlaceholder + `">window.close()</script>`
			contentType := "text/html; charset=utf-8"
			if mode == "plaintext" {
				contentType = "text/plain; charset=utf-8"
			}
			if err := c.Start("state", CallbackPage{Success: template, Failure: template, ContentType: contentType, HTMLNonce: true}); err != nil {
				t.Fatal(err)
			}
			query := "?state=state&code=ok"
			if mode == "refused" {
				query = "?state=state&error=access_denied"
			}
			resp, err := http.Get(c.RedirectURI() + query)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = resp.Body.Close() }()
			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if resp.ContentLength != int64(len(body)) {
				t.Fatal("incorrect response length")
			}
			policy := resp.Header.Get("Content-Security-Policy")
			if mode == "plaintext" {
				if policy != "default-src 'none'" || string(body) != template {
					t.Fatal("plaintext policy changed")
				}
				return
			}
			prefix := "default-src 'none'; style-src 'nonce-"
			if !strings.HasPrefix(policy, prefix) {
				t.Fatal(policy)
			}
			nonce, _, ok := strings.Cut(strings.TrimPrefix(policy, prefix), "'")
			if !ok || len(nonce) < 32 || nonces[nonce] {
				t.Fatal("invalid or reused nonce")
			}
			nonces[nonce] = true
			if policy != prefix+nonce+"'; script-src 'nonce-"+nonce+"'" {
				t.Fatal("unrestricted policy")
			}
			if strings.Contains(string(body), CallbackNoncePlaceholder) || strings.Count(string(body), `nonce="`+nonce+`"`) != 2 {
				t.Fatal("page nonce does not match policy")
			}
		})
	}
}
