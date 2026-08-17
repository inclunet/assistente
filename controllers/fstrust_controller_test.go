package controllers

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"assistente/internal/fstrust"
)

func TestFSTrustGetPathAllowlistNilManager(t *testing.T) {
	t.Parallel()
	c := NewFSTrustController(FSTrustControllerConfig{})
	if got := c.GetPathAllowlist(context.Background()); got != nil {
		t.Fatalf("want nil, got %#v", got)
	}
}

func TestFSTrustRemoveRequiresPersistentScope(t *testing.T) {
	t.Parallel()
	c := NewFSTrustController(FSTrustControllerConfig{
		FSTrustMgr: fstrust.NewManager(),
	})
	err := c.RemovePathAllowlistEntry(context.Background(), "session", "/tmp/a.txt", "file", "read", "allow")
	if err == nil || !strings.Contains(err.Error(), "escopo inválido") {
		t.Fatalf("want escopo inválido, got %v", err)
	}
}

func TestFSTrustRemoveNilManager(t *testing.T) {
	t.Parallel()
	c := NewFSTrustController(FSTrustControllerConfig{})
	err := c.RemovePathAllowlistEntry(context.Background(), "global", "/tmp/a.txt", "file", "read", "allow")
	if err == nil || !strings.Contains(err.Error(), "não inicializado") {
		t.Fatalf("want gerenciador não inicializado, got %v", err)
	}
}

func TestFSTrustRemoveInvalidKind(t *testing.T) {
	t.Parallel()
	c := NewFSTrustController(FSTrustControllerConfig{
		FSTrustMgr: fstrust.NewManager(),
	})
	err := c.RemovePathAllowlistEntry(context.Background(), "global", "/tmp/a.txt", "weird", "read", "allow")
	if err == nil || !strings.Contains(err.Error(), "kind inválido") {
		t.Fatalf("want kind inválido, got %v", err)
	}
}

func TestFSTrustAddPathDenyEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mgr := fstrust.NewManagerWithDirs(dir, dir)
	c := NewFSTrustController(FSTrustControllerConfig{FSTrustMgr: mgr})
	path := filepath.Join(dir, "bloqueado.txt")

	if err := c.AddPathDenyEntry(context.Background(), path, "file", "read", "global", "teste"); err != nil {
		t.Fatalf("AddPathDenyEntry: %v", err)
	}
	if err := c.AddPathDenyEntry(context.Background(), path, "file", "read", "session", ""); err == nil {
		t.Fatal("session não deveria ser aceito para denylist")
	}

	views := c.GetPathAllowlist(context.Background())
	if len(views) != 1 || views[0].Effect != "deny" {
		t.Fatalf("want 1 deny, got %#v", views)
	}
}

func TestFSTrustGetPathAllowlistMapsEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mgr := fstrust.NewManagerWithDirs(dir, dir)
	path := filepath.Join(dir, "docs", "a.txt")
	created := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	entry := fstrust.AllowlistEntry{
		Path:      path,
		Kind:      fstrust.KindFile,
		Operation: "read",
		Effect:    fstrust.EffectAllow,
		Scope:     fstrust.ScopeGlobal,
		CreatedBy: "user",
		CreatedAt: created,
		Reason:    "teste",
	}
	if err := mgr.Add(context.Background(), entry); err != nil {
		t.Fatalf("Add: %v", err)
	}

	c := NewFSTrustController(FSTrustControllerConfig{FSTrustMgr: mgr})
	views := c.GetPathAllowlist(context.Background())
	if len(views) != 1 {
		t.Fatalf("want 1 entrada, got %d", len(views))
	}
	v := views[0]
	if fstrust.NormalizePath(v.Path) != fstrust.NormalizePath(entry.Path) ||
		v.Kind != string(entry.Kind) ||
		v.Operation != entry.Operation ||
		v.Effect != "allow" ||
		v.Scope != string(entry.Scope) {
		t.Fatalf("view incompleta: %#v", v)
	}
	if v.CreatedAt == "" || v.Reason != "teste" {
		t.Fatalf("metadados: %#v", v)
	}
}
