package controllers

import (
	"context"
	"strings"
	"testing"
	"time"

	"assistente/internal/nettrust"
)

func TestNetTrustGetNetworkAllowlistNilManager(t *testing.T) {
	t.Parallel()
	c := NewNetTrustController(NetTrustControllerConfig{})
	if got := c.GetNetworkAllowlist(context.Background()); got != nil {
		t.Fatalf("want nil, got %#v", got)
	}
}

func TestNetTrustRemoveRequiresPersistentScope(t *testing.T) {
	t.Parallel()
	c := NewNetTrustController(NetTrustControllerConfig{
		NetTrustMgr: nettrust.NewManager(),
	})
	err := c.RemoveNetworkAllowlistEntry(context.Background(), "session", "example.local", "")
	if err == nil || !strings.Contains(err.Error(), "escopo inválido") {
		t.Fatalf("want escopo inválido, got %v", err)
	}
}

func TestNetTrustRemoveNilManager(t *testing.T) {
	t.Parallel()
	c := NewNetTrustController(NetTrustControllerConfig{})
	err := c.RemoveNetworkAllowlistEntry(context.Background(), "global", "example.local", "")
	if err == nil || !strings.Contains(err.Error(), "não inicializado") {
		t.Fatalf("want gerenciador não inicializado, got %v", err)
	}
}

func TestNetTrustGetNetworkAllowlistMapsEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mgr := nettrust.NewManagerWithDirs(dir, dir)
	created := time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	entry := nettrust.AllowlistEntry{
		Host:        "api.internal.example",
		Port:        "8443",
		Scope:       nettrust.ScopeGlobal,
		Category:    "private",
		ResolvedIPs: []string{"10.0.0.1"},
		CreatedBy:   "user",
		CreatedAt:   created,
		Reason:      "teste",
	}
	if err := mgr.Add(context.Background(), entry); err != nil {
		t.Fatalf("Add: %v", err)
	}

	c := NewNetTrustController(NetTrustControllerConfig{NetTrustMgr: mgr})
	views := c.GetNetworkAllowlist(context.Background())
	if len(views) != 1 {
		t.Fatalf("want 1 entrada, got %d", len(views))
	}
	v := views[0]
	if v.Host != entry.Host || v.Port != entry.Port || v.Scope != string(entry.Scope) {
		t.Fatalf("view incompleta: %#v", v)
	}
	if v.CreatedAt == "" || v.Reason != "teste" {
		t.Fatalf("metadados: %#v", v)
	}
}
