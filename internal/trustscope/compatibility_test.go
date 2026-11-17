package trustscope_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/database"
	"assistente/internal/fstrust"
	"assistente/internal/nettrust"
	"assistente/internal/testutil/authorizationfixture"
	"assistente/internal/tools/invocationctx"
	"assistente/internal/trustscope"
	"gorm.io/gorm"
)

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestCapturedIdentityRejectsChangedUser(t *testing.T) {
	dir := t.TempDir()
	m := nettrust.NewManager(authorizationfixture.Options(t, dir, ""))
	ctx := invocationctx.With(authorizationfixture.Context(), invocationctx.InvocationContext{ConversationID: "conversation"})
	ctx = m.Capture(ctx)
	if err := m.Add(ctx, nettrust.AllowlistEntry{Host: "private.internal", Scope: nettrust.ScopeSession}); err != nil {
		t.Fatal(err)
	}
	for _, user := range []string{"another-user", ""} {
		changed := database.WithUserID(ctx, user)
		if result := m.Match(changed, "private.internal", "443"); result.Err == nil || result.Allowed {
			t.Fatalf("concessão vazou: %+v", result)
		}
		if err := m.Add(changed, nettrust.AllowlistEntry{Host: "persistent.internal", Scope: nettrust.ScopeGlobal}); err == nil {
			t.Fatal("gravou com identidade divergente")
		}
	}
}

func TestPersistentRevocationRejectsSessionInvalidatedAfterList(t *testing.T) {
	repo, _ := authorizationfixture.Open(t, t.TempDir())
	type entry struct{ Scope trustscope.Scope }
	var m *trustscope.Manager[entry, string]
	clear := false
	m = trustscope.NewManager(trustscope.Config[entry, string]{Domain: "network", Subdir: "test", Adapter: trustscope.Adapter[entry, string]{
		Scope: func(e entry) trustscope.Scope { return e.Scope }, Normalize: func(e entry) (entry, error) { return e, nil },
		Key: func(entry) string { return "private.internal" }, Identity: func(entry) string { return "private.internal" },
		Effect: func(entry) string { return "deny" }, Operation: func(entry) string { return "" },
		SameKey: func(entry, string) bool {
			if clear {
				m.ClearSession("conversation")
			}
			return true
		},
	}}, trustscope.Options{Repository: repo, HomeDir: func() string { return "" }, WorkDir: func() string { return "" }})
	ctx := invocationctx.With(authorizationfixture.Context(), invocationctx.InvocationContext{ConversationID: "conversation"})
	if err := m.Add(ctx, entry{Scope: trustscope.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	clear = true
	if err := m.Remove(ctx, trustscope.ScopeGlobal, "private.internal"); err == nil {
		t.Fatal("revogou negativa após limpar sessão")
	}
	rows, err := repo.List(ctx, "network", "global", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("negativa perdida: %+v %v", rows, err)
	}
}

func TestConsultaEmVooNaoRetornaSessaoInvalidada(t *testing.T) {
	dir := t.TempDir()
	repo, db := authorizationfixture.Open(t, dir)
	m := nettrust.NewManager(trustscope.Options{Repository: repo, HomeDir: func() string { return dir }, WorkDir: func() string { return "" }})
	ctx := invocationctx.With(authorizationfixture.Context(), invocationctx.InvocationContext{ConversationID: "conversation"})
	if err := m.Add(ctx, nettrust.AllowlistEntry{Host: "session.internal", Scope: nettrust.ScopeSession}); err != nil {
		t.Fatal(err)
	}
	if err := db.Callback().Query().Before("gorm:query").Register("test:clear-session", func(tx *gorm.DB) {
		if tx.Statement.Table == "authorization_rules" {
			m.ClearSession("conversation")
		}
	}); err != nil {
		t.Fatal(err)
	}
	if match := m.Match(ctx, "session.internal", "443"); match.Err == nil || match.Allowed {
		t.Fatalf("consulta publicou geração encerrada: %+v", match)
	}
}
func TestNetworkUpgradePreservesFileAndDoesNotResurrectRevocation(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "network-allowlist", "global.json")
	const original = `{"version":1,"entries":[{"host":"legacy.internal","scope":"global","category":"private-rfc1918","created_at":"2026-01-02T03:04:05Z"}]}`
	writeFixture(t, source, original)
	ctx := authorizationfixture.Context()
	options := authorizationfixture.Options(t, dir, dir)
	m := nettrust.NewManager(options)
	if d := m.Match(ctx, "legacy.internal", "443"); !d.Allowed || d.Err != nil {
		t.Fatalf("migration: %+v", d)
	}
	if err := m.Remove(ctx, nettrust.ScopeGlobal, "legacy.internal", ""); err != nil {
		t.Fatal(err)
	}
	m = nettrust.NewManager(options)
	if d := m.Match(ctx, "legacy.internal", "443"); d.Allowed || d.Err != nil {
		t.Fatalf("revoked rule resurrected: %+v", d)
	}
	if err := m.Add(ctx, nettrust.AllowlistEntry{Host: "new.internal", Scope: nettrust.ScopeGlobal}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(source)
	if err != nil || string(data) != original {
		t.Fatal("legacy backup was modified")
	}
	if d := m.Match(database.WithUserID(ctx, "another-user"), "new.internal", "443"); d.Allowed {
		t.Fatal("grant leaked to another user")
	}
}
func TestFilesystemUpgradeKeepsDenyAndOperations(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "restricted.txt")
	fixture := map[string]any{"version": 1, "entries": []fstrust.AllowlistEntry{{Path: target, Kind: fstrust.KindFile, Operation: "read", Effect: fstrust.EffectDeny, Scope: fstrust.ScopeGlobal}}}
	data, err := json.Marshal(fixture)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, filepath.Join(dir, "path-allowlist", "global.json"), string(data))
	m := fstrust.NewManager(authorizationfixture.Options(t, dir, dir))
	ctx := authorizationfixture.Context()
	if d := m.MatchDeny(ctx, target, "read"); !d.Matched || d.Err != nil {
		t.Fatalf("lost deny: %+v", d)
	}
	if d := m.MatchDeny(ctx, target, "write"); d.Matched {
		t.Fatal("expanded deny to another operation")
	}
}
func TestInvalidSourcesRemainPendingAndBlockOtherScopes(t *testing.T) {
	for _, content := range []string{"", `{broken`, `{"version":2,"entries":[]}`} {
		t.Run(content, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "network-allowlist", "global.json")
			writeFixture(t, source, content)
			options := authorizationfixture.Options(t, dir, dir)
			m := nettrust.NewManager(options)
			ctx := invocationctx.With(authorizationfixture.Context(), invocationctx.InvocationContext{ConversationID: "c", ProfileSlug: "p"})
			if d := m.Match(ctx, "blocked.internal", "443"); d.Allowed || d.Err == nil {
				t.Fatalf("invalid source accepted: %+v", d)
			}
			if err := m.Add(ctx, nettrust.AllowlistEntry{Host: "blocked.internal", Scope: nettrust.ScopeProfile}); err == nil {
				t.Fatal("mutation bypassed pending origin")
			}
			writeFixture(t, source, `{"version":1,"entries":[]}`)
			if err := m.Add(ctx, nettrust.AllowlistEntry{Host: "blocked.internal", Scope: nettrust.ScopeProfile}); err != nil {
				t.Fatalf("retry: %v", err)
			}
		})
	}
}
func TestConversationResetAndContextSwitchWhilePromptPending(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace-a")
	options := authorizationfixture.Options(t, dir, workspace)
	m := nettrust.NewManager(options)
	m.SetActiveProfileSlugFunc(func() string { return "profile-a" })
	m.SetWorkspaceDirFunc(func() string { return workspace })
	base := invocationctx.With(authorizationfixture.Context(), invocationctx.InvocationContext{ConversationID: "conversation"})
	captured := m.Capture(base)
	workspace = filepath.Join(dir, "workspace-b")
	m.SetActiveProfileSlugFunc(func() string { return "profile-b" })
	if err := m.Add(captured, nettrust.AllowlistEntry{Host: "a.internal", Scope: nettrust.ScopeWorkspace}); err != nil {
		t.Fatal(err)
	}
	if d := m.Match(base, "a.internal", "443"); d.Allowed {
		t.Fatal("grant followed active workspace")
	}
	m.ClearSession("conversation")
	if err := m.Add(captured, nettrust.AllowlistEntry{Host: "a.internal", Scope: nettrust.ScopeSession}); err == nil {
		t.Fatal("stale consent survived conversation reset")
	}
}
func TestMissingUserAndScopeIdentityFailClosed(t *testing.T) {
	dir := t.TempDir()
	m := nettrust.NewManager(authorizationfixture.Options(t, dir, dir))
	if d := m.Match(context.Background(), "a.internal", "443"); d.Err == nil {
		t.Fatal("unauthenticated lookup")
	}
	m.SetWorkspaceDirFunc(func() string { return "" })
	if err := m.Add(authorizationfixture.Context(), nettrust.AllowlistEntry{Host: "a.internal", Scope: nettrust.ScopeWorkspace}); err == nil {
		t.Fatal("workspace silently widened")
	}
	ctx := invocationctx.With(authorizationfixture.Context(), invocationctx.InvocationContext{ProfileSlug: "../escape"})
	if err := m.Add(ctx, nettrust.AllowlistEntry{Host: "a.internal", Scope: nettrust.ScopeProfile}); err == nil {
		t.Fatal("invalid legacy profile path accepted")
	}
}

func TestWorkspaceDaOperacaoNaoSegueInterface(t *testing.T) {
	dir := t.TempDir()
	m := nettrust.NewManager(authorizationfixture.Options(t, dir, filepath.Join(dir, "workspace-b", ".assistente")))
	operation := invocationctx.WithWorkspace(authorizationfixture.Context(), filepath.Join(dir, "workspace-a"))
	if err := m.Add(operation, nettrust.AllowlistEntry{Host: "origem.internal", Scope: nettrust.ScopeWorkspace}); err != nil {
		t.Fatal(err)
	}
	if match := m.Match(operation, "origem.internal", "443"); match.Err != nil || !match.Allowed {
		t.Fatalf("origem: %+v", match)
	}
	if match := m.Match(authorizationfixture.Context(), "origem.internal", "443"); match.Err != nil || match.Allowed {
		t.Fatalf("vazou para tela: %+v", match)
	}
	without := invocationctx.WithWorkspace(authorizationfixture.Context(), "")
	if err := m.Add(without, nettrust.AllowlistEntry{Host: "origem.internal", Scope: nettrust.ScopeWorkspace}); err == nil {
		t.Fatal("origem sem workspace herdou tela")
	}
}
