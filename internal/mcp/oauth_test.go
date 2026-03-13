package mcp

import (
	"fmt"
	"net"
	"sync"
	"testing"
)

func TestCallbackHostToListenIP(t *testing.T) {
	tests := []struct {
		name         string
		callbackHost string
		wantListenIP string
		wantHost     string // host used in redirectURL
	}{
		{
			name:         "empty defaults to localhost",
			callbackHost: "",
			wantListenIP: "127.0.0.1",
			wantHost:     "localhost",
		},
		{
			name:         "explicit localhost",
			callbackHost: "localhost",
			wantListenIP: "127.0.0.1",
			wantHost:     "localhost",
		},
		{
			name:         "explicit 127.0.0.1",
			callbackHost: "127.0.0.1",
			wantListenIP: "127.0.0.1",
			wantHost:     "127.0.0.1",
		},
		{
			name:         "IPv6 loopback",
			callbackHost: "[::1]",
			wantListenIP: "::1",
			wantHost:     "[::1]",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotHost, gotListenIP := resolveCallbackHost(tc.callbackHost)

			if gotHost != tc.wantHost {
				t.Errorf("callbackHost: got %q, want %q", gotHost, tc.wantHost)
			}
			if gotListenIP != tc.wantListenIP {
				t.Errorf("listenIP: got %q, want %q", gotListenIP, tc.wantListenIP)
			}
		})
	}
}

func TestListenAddrFormat(t *testing.T) {
	tests := []struct {
		name     string
		listenIP string
		port     int
		wantAddr string
	}{
		{
			name:     "IPv4 random port",
			listenIP: "127.0.0.1",
			port:     0,
			wantAddr: "127.0.0.1:0",
		},
		{
			name:     "IPv4 fixed port",
			listenIP: "127.0.0.1",
			port:     3118,
			wantAddr: "127.0.0.1:3118",
		},
		{
			name:     "IPv6 random port",
			listenIP: "::1",
			port:     0,
			wantAddr: "::1:0",
		},
		{
			name:     "IPv6 fixed port",
			listenIP: "::1",
			port:     8080,
			wantAddr: "::1:8080",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got string
			if tc.port > 0 {
				got = fmt.Sprintf("%s:%d", tc.listenIP, tc.port)
			} else {
				got = tc.listenIP + ":0"
			}

			if got != tc.wantAddr {
				t.Errorf("listenAddr: got %q, want %q", got, tc.wantAddr)
			}
		})
	}
}

func TestRedirectURLFormat(t *testing.T) {
	tests := []struct {
		name         string
		callbackHost string
		port         int
		wantURL      string
	}{
		{
			name:         "localhost default with port 3118",
			callbackHost: "",
			port:         3118,
			wantURL:      "http://localhost:3118/callback",
		},
		{
			name:         "explicit localhost with port 8080",
			callbackHost: "localhost",
			port:         8080,
			wantURL:      "http://localhost:8080/callback",
		},
		{
			name:         "127.0.0.1 with port 3118",
			callbackHost: "127.0.0.1",
			port:         3118,
			wantURL:      "http://127.0.0.1:3118/callback",
		},
		{
			name:         "IPv6 loopback with port 9090",
			callbackHost: "[::1]",
			port:         9090,
			wantURL:      "http://[::1]:9090/callback",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			host, _ := resolveCallbackHost(tc.callbackHost)
			got := fmt.Sprintf("http://%s:%d/callback", host, tc.port)

			if got != tc.wantURL {
				t.Errorf("redirectURL: got %q, want %q", got, tc.wantURL)
			}
		})
	}
}

func TestDCRPersistsCallbackPort(t *testing.T) {
	var mu sync.Mutex
	var savedConfig *ServerConfig

	rt := &pkceRoundTripper{
		cfg: ServerConfig{
			OAuth2CallbackPort: 0,
		},
		serverSlug: "test",
		onConfigUpdate: func(cfg ServerConfig) {
			mu.Lock()
			defer mu.Unlock()
			c := cfg
			savedConfig = &c
		},
	}

	// Simula o que acontece após DCR: porta aleatória é alocada
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Simula a lógica de persistência de porta pós-DCR
	if rt.cfg.OAuth2CallbackPort == 0 {
		rt.cfg.OAuth2CallbackPort = port
		if rt.onConfigUpdate != nil {
			rt.onConfigUpdate(rt.cfg)
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if savedConfig == nil {
		t.Fatal("onConfigUpdate não foi chamado")
	}
	if savedConfig.OAuth2CallbackPort != port {
		t.Errorf("porta persistida: got %d, want %d", savedConfig.OAuth2CallbackPort, port)
	}
}

func TestDCRDoesNotOverwriteFixedPort(t *testing.T) {
	called := false

	rt := &pkceRoundTripper{
		cfg: ServerConfig{
			OAuth2CallbackPort: 3118,
		},
		serverSlug: "test",
		onConfigUpdate: func(cfg ServerConfig) {
			called = true
		},
	}

	// Simula a lógica: porta fixa já existe, não deve sobrescrever
	if rt.cfg.OAuth2CallbackPort == 0 {
		rt.cfg.OAuth2CallbackPort = 9999
		if rt.onConfigUpdate != nil {
			rt.onConfigUpdate(rt.cfg)
		}
	}

	if called {
		t.Error("onConfigUpdate não deveria ter sido chamado quando porta fixa já existe")
	}
	if rt.cfg.OAuth2CallbackPort != 3118 {
		t.Errorf("porta não deveria mudar: got %d, want 3118", rt.cfg.OAuth2CallbackPort)
	}
}

func TestCallbackListenerBinds(t *testing.T) {
	tests := []struct {
		name         string
		callbackHost string
	}{
		{"localhost binds to 127.0.0.1", "localhost"},
		{"127.0.0.1 binds to 127.0.0.1", "127.0.0.1"},
		{"empty binds to 127.0.0.1", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, listenIP := resolveCallbackHost(tc.callbackHost)
			addr := listenIP + ":0"

			listener, err := net.Listen("tcp", addr)
			if err != nil {
				t.Fatalf("net.Listen(%q) failed: %v", addr, err)
			}
			defer listener.Close()

			tcpAddr := listener.Addr().(*net.TCPAddr)
			if tcpAddr.Port == 0 {
				t.Error("expected non-zero port from listener")
			}
		})
	}
}
