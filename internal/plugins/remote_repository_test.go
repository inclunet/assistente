package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"sync/atomic"
	"testing"

	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/plugincatalog"
	"assistente/internal/pluginpackage"
	"assistente/internal/sqlite"
	"gorm.io/gorm"
)

func TestRemoteStoreRetainsReviewedArchiveAndVaultOrigin(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "remote.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.AutoMigrate(&database.PluginRepository{}, &database.PluginInstallation{}, &database.PluginRevision{}); err != nil {
		t.Fatal(err)
	}
	var payload bytes.Buffer
	zw := zip.NewWriter(&payload)
	for name, body := range map[string]string{"project-main/.claude-plugin/marketplace.json": `{"name":"demo","owner":{"name":"publisher"},"plugins":[{"name":"sample","source":"./plugins/sample"}]}`, "project-main/plugins/sample/plugin.json": `{"$schema":"` + pluginpackage.ManifestSchema + `","name":"sample"}`} {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	var contacts atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { contacts.Add(1) }))
	defer target.Close()
	var changed atomic.Bool
	var requests atomic.Int32
	var redirect atomic.Bool
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("missing expected vault authorization")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if redirect.Load() {
			http.Redirect(w, r, target.URL, http.StatusFound)
			return
		}
		if changed.Load() {
			// ZIP comments can change the archive revision while preserving index entries.
			body := bytes.Clone(payload.Bytes())
			body[len(body)-2] = 1
			_, _ = w.Write(append(body, 'x'))
			return
		}
		_, _ = w.Write(payload.Bytes())
	}))
	defer server.Close()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	manager := credentials.NewManager(nil)
	store := NewInventory(db).WithRemoteClient(func(ref string) *http.Client {
		if ref != u.Host {
			t.Fatalf("unexpected reference %q", ref)
		}
		client := RepositoryHTTPClient(manager, ref)
		client.Transport.(*credentials.CredentialTransport).Base = server.Client().Transport
		return client
	})
	ctx := database.WithUserID(context.Background(), "owner")
	admit := commandsecurity.Admission(func(fn func() error) error { return fn() })
	other := database.WithUserID(context.Background(), "other")
	credential := &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "test-secret"}
	if err := manager.RegisterPatternWithContext(other, u.Host, credential); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRemoteRepository(ctx, server.URL+"/archive.zip", true, "", "", admit); err == nil || requests.Load() != 0 {
		t.Fatal("foreign credential or anonymous fallback was used", err)
	}
	if err := manager.RegisterPatternWithContext(ctx, u.Host, credential); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRemoteRepository(ctx, server.URL+"/archive.zip", true, "", "", admit); err != nil {
		t.Fatal(err)
	}
	rows, err := store.Repositories(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	row := rows[0]
	snap, err := store.InspectRepositoryEntry(ctx, row.ID, row.Revision, plugincatalog.Claude, "sample")
	if err != nil || snap.Manifest.Name != "sample" || snap.DiscoveryRepositoryID != row.ID {
		t.Fatal(snap, err)
	}
	changed.Store(true)
	inspect := func() (*pluginpackage.LocalSnapshot, error) {
		return store.InspectRepositoryEntry(ctx, row.ID, row.Revision, plugincatalog.Claude, "sample")
	}
	if _, err := store.InstallSource(ctx, snap.Origin, snap.Digest, "", admit, inspect); !errors.Is(err, ErrPackageChanged) {
		t.Fatal("changed archive accepted", err)
	}
	if err := store.SaveRepository(ctx, "", row.ID, row.Revision, admit); err != nil {
		t.Fatal(err)
	}
	rows, err = store.Repositories(ctx)
	if err != nil || len(rows) != 1 || rows[0].Revision == row.Revision {
		t.Fatal(rows, err)
	}
	row = rows[0]
	snap, err = inspect()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.InstallSource(ctx, snap.Origin, snap.Digest, "", admit, inspect); err != nil {
		t.Fatal(err)
	}
	redirect.Store(true)
	if err := store.SaveRepository(ctx, "", row.ID, row.Revision, admit); err == nil {
		t.Fatal("cross-origin redirect accepted")
	}
	if contacts.Load() != 0 {
		t.Fatal("contacted redirect destination")
	}
	rows, err = store.Repositories(ctx)
	if err != nil || len(rows) != 1 || rows[0].Revision != row.Revision {
		t.Fatal(rows, err)
	}
}
