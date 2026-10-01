package oauthflow

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

var ErrCallbackPort = errors.New("oauth_callback_port_unavailable")

// CallbackPage belongs to the consumer, including localization and presentation.
type CallbackPage struct{ Success, Failure, ContentType string }

// LoopbackCallback reserves the exact redirect URI before DCR or authorization.
// A single owner starts, waits and closes it; requests are consumed at most once.
type LoopbackCallback struct {
	listener net.Listener
	redirect *url.URL
	results  chan url.Values
	server   *http.Server
}

func ReserveCallback(cfg CallbackConfig) (*LoopbackCallback, error) {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	host = strings.Trim(host, "[]")
	bind, network := host, "tcp4"
	switch host {
	case "localhost":
		bind = "127.0.0.1"
	case "127.0.0.1":
	case "::1":
		network = "tcp6"
	default:
		return nil, ErrResource
	}
	if !strings.HasPrefix(cfg.Path, "/") || path.Clean(cfg.Path) != cfg.Path || strings.ContainsAny(cfg.Path, "?#%\\") {
		return nil, ErrResource
	}
	port := cfg.Port
	if cfg.PortPolicy == "ephemeral" {
		port = 0
	} else if cfg.PortPolicy != "fixed" || port < 1 || port > 65535 {
		return nil, ErrResource
	}
	listener, err := net.Listen(network, net.JoinHostPort(bind, strconv.Itoa(port)))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrCallbackPort, err)
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port
	redirect := &url.URL{Scheme: "http", Host: net.JoinHostPort(host, strconv.Itoa(actualPort)), Path: cfg.Path}
	return &LoopbackCallback{listener: listener, redirect: redirect, results: make(chan url.Values, 1)}, nil
}
func (c *LoopbackCallback) RedirectURI() string { return c.redirect.String() }
func (c *LoopbackCallback) Port() int           { return c.listener.Addr().(*net.TCPAddr).Port }
func (c *LoopbackCallback) Close() {
	if c.server != nil {
		_ = c.server.Close()
	}
	_ = c.listener.Close()
}
func (c *LoopbackCallback) Start(state string, page CallbackPage) error {
	if state == "" || c.server != nil {
		return ErrResource
	}
	var consumed atomic.Bool
	c.server = &http.Server{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192, Handler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'none'")
		if req.Method != http.MethodGet || req.Host != c.redirect.Host || req.URL.EscapedPath() != c.redirect.EscapedPath() {
			http.Error(w, "", http.StatusNotFound)
			return
		}
		v, err := url.ParseQuery(req.URL.RawQuery)
		if err != nil || len(v["state"]) != 1 || subtle.ConstantTimeCompare([]byte(v.Get("state")), []byte(state)) != 1 {
			http.Error(w, "", http.StatusBadRequest)
			return
		}
		for _, key := range []string{"code", "error", "client_id"} {
			if len(v[key]) > 1 {
				http.Error(w, "", http.StatusBadRequest)
				return
			}
		}
		if (v.Get("code") == "") == (v.Get("error") == "") {
			http.Error(w, "", http.StatusBadRequest)
			return
		}
		if !consumed.CompareAndSwap(false, true) {
			http.Error(w, "", http.StatusConflict)
			return
		}
		body := page.Success
		if v.Get("error") != "" {
			body = page.Failure
		}
		contentType := page.ContentType
		if contentType == "" {
			contentType = "text/plain; charset=utf-8"
		}
		w.Header().Set("Content-Type", contentType)
		w.Header().Set("Content-Length", strconv.Itoa(len(body)))
		_, _ = w.Write([]byte(body))
		_ = http.NewResponseController(w).Flush()
		select {
		case c.results <- v:
		default:
		}
	})}
	go func() { _ = c.server.Serve(c.listener) }()
	return nil
}
func (c *LoopbackCallback) Wait(ctx context.Context) (url.Values, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-c.results:
		return result, nil
	}
}
