package controllers

import (
	"assistente/internal/testutil/authorizationfixture"

	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/fstrust"
)

func TestFSTrustAddPathAllowlistEntryDeny(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mgr := fstrust.NewManager(authorizationfixture.Options(t, dir, dir))
	c := NewFSTrustController(FSTrustControllerConfig{FSTrustMgr: mgr})
	path := filepath.Join(dir, "bloqueado.txt")

	if err := c.AddPathAllowlistEntry(authorizationfixture.Context(), path, "file", "read", "deny", "global", "teste"); err != nil {
		t.Fatalf("AddPathAllowlistEntry deny: %v", err)
	}
	if err := c.AddPathAllowlistEntry(authorizationfixture.Context(), path, "file", "read", "deny", "session", ""); err == nil {
		t.Fatal("session não deveria ser aceito para regra persistente")
	}

	views, err := mgr.List(authorizationfixture.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Effect != "deny" {
		t.Fatalf("want 1 deny, got %#v", views)
	}
}

func TestFSTrustAddPathAllowlistEntryAllow(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mgr := fstrust.NewManager(authorizationfixture.Options(t, dir, dir))
	c := NewFSTrustController(FSTrustControllerConfig{FSTrustMgr: mgr})
	path := filepath.Join(dir, "permitido.txt")

	if err := c.AddPathAllowlistEntry(authorizationfixture.Context(), path, "file", " ReAd ", "allow", "global", " docs "); err != nil {
		t.Fatalf("AddPathAllowlistEntry allow: %v", err)
	}
	if err := c.AddPathAllowlistEntry(authorizationfixture.Context(), path, "file", "read", "allow", "session", ""); err == nil {
		t.Fatal("session não deveria ser aceito pela gestão persistente")
	}
	if err := c.AddPathAllowlistEntry(authorizationfixture.Context(), path, "file", "read", "maybe", "global", ""); err == nil {
		t.Fatal("efeito desconhecido não deveria ser aceito")
	}
	views, err := mgr.List(authorizationfixture.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 || views[0].Effect != "allow" || views[0].Operation != "read" || views[0].Reason != "docs" {
		t.Fatalf("want 1 allow normalizado, got %#v", views)
	}
}

func TestFSTrustAddPathAllowlistEntryTrims(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mgr := fstrust.NewManager(authorizationfixture.Options(t, dir, dir))
	c := NewFSTrustController(FSTrustControllerConfig{FSTrustMgr: mgr})
	want := filepath.Join(dir, "bloqueado.txt")
	// Path com espaços nas bordas e um segmento ".." que precisa ser colapsado
	// para casar o que o MatchDeny usa em tempo de acesso.
	raw := "  " + filepath.Join(dir, "sub", "..", "bloqueado.txt") + "  "

	if err := c.AddPathAllowlistEntry(authorizationfixture.Context(), raw, "file", "  ReAd  ", "deny", "global", "  motivo  "); err != nil {
		t.Fatalf("AddPathAllowlistEntry: %v", err)
	}

	views, err := mgr.List(authorizationfixture.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("want 1 entrada, got %d", len(views))
	}
	v := views[0]
	if v.Path != strings.TrimSpace(v.Path) || fstrust.NormalizePath(v.Path) != fstrust.NormalizePath(want) {
		t.Fatalf("path não resolvido/normalizado: %q (quer %q)", v.Path, want)
	}
	if v.Operation != "read" {
		t.Fatalf("operation não normalizada: %q", v.Operation)
	}
	if v.Reason != "motivo" {
		t.Fatalf("reason não normalizado: %q", v.Reason)
	}
}

func TestFSTrustAddPathAllowlistEntryExpandsHome(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skipf("home indisponível: %v", err)
	}
	dir := t.TempDir()
	mgr := fstrust.NewManager(authorizationfixture.Options(t, dir, dir))
	c := NewFSTrustController(FSTrustControllerConfig{FSTrustMgr: mgr})

	if err := c.AddPathAllowlistEntry(authorizationfixture.Context(), "~/segredo.env", "file", "read", "deny", "global", ""); err != nil {
		t.Fatalf("AddPathAllowlistEntry: %v", err)
	}

	views, err := mgr.List(authorizationfixture.Context())
	if err != nil {
		t.Fatal(err)
	}
	if len(views) != 1 {
		t.Fatalf("want 1 entrada, got %d", len(views))
	}
	want := fstrust.NormalizePath(filepath.Join(home, "segredo.env"))
	if fstrust.NormalizePath(views[0].Path) != want {
		t.Fatalf("~ não expandido: %q (quer %q)", views[0].Path, want)
	}
	if strings.Contains(views[0].Path, "~") {
		t.Fatalf("path ainda contém ~: %q", views[0].Path)
	}
}

func TestFSTrustAddPathAllowlistEntryRejectsRelative(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	mgr := fstrust.NewManager(authorizationfixture.Options(t, dir, dir))
	c := NewFSTrustController(FSTrustControllerConfig{FSTrustMgr: mgr})

	if err := c.AddPathAllowlistEntry(authorizationfixture.Context(), "relativo/segredo.env", "file", "read", "deny", "global", ""); err == nil {
		t.Fatal("path relativo deveria ser rejeitado")
	}
	if entries, err := mgr.List(authorizationfixture.Context()); err != nil || len(entries) != 0 {
		t.Fatal("nada deveria ter sido persistido para path relativo")
	}
}
