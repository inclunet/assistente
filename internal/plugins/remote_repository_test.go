package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandsecurity"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/plugincatalog"
	"assistente/internal/pluginpackage"
	"assistente/internal/sqlite"
	httpclient "assistente/internal/tools/http"
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
	for name, body := range map[string]string{".github-main/.claude-plugin/marketplace.json": `{"name":"demo","owner":{"name":"publisher"},"plugins":[{"name":"sample","source":"./plugins/sample"}]}`, ".github-main/plugins/sample/.git/HEAD": "fixture", ".github-main/plugins/sample/plugin.json": `{"$schema":"` + pluginpackage.ManifestSchema + `","name":"sample"}`} {
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
	var conditional atomic.Int32
	var redirect atomic.Bool
	var reject atomic.Bool
	var block atomic.Bool
	started := make(chan struct{})
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Header.Get("Authorization") != "Bearer test-secret" {
			t.Error("missing expected vault authorization")
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if reject.Load() {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		if block.Load() {
			close(started)
			<-r.Context().Done()
			return
		}
		if redirect.Load() {
			http.Redirect(w, r, target.URL, http.StatusFound)
			return
		}
		if !changed.Load() {
			w.Header().Set("ETag", `"r1"`)
			w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")
			if r.Header.Get("If-None-Match") == `"r1"` && r.Header.Get("If-Modified-Since") != "" {
				conditional.Add(1)
				w.WriteHeader(http.StatusNotModified)
				return
			}
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
	store := NewInventory(db).WithRemoteClient(func(ref string) *httpclient.Client {
		if ref != u.Host {
			t.Fatalf("unexpected reference %q", ref)
		}
		client := RepositoryHTTPClient(manager, ref, nil)
		client.GetBaseClient().Transport.(*credentials.CredentialTransport).Base = server.Client().Transport
		return client
	}, nil)
	ctx := database.WithUserID(context.Background(), "owner")
	admit := commandsecurity.Admission(func(_ *gorm.DB, fn func() error) error { return fn() })
	other := database.WithUserID(context.Background(), "other")
	t.Run("formato escolhido precisa constar da leitura final", func(t *testing.T) {
		root := t.TempDir()
		dir := filepath.Join(root, ".cursor-plugin")
		if err := os.Mkdir(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "marketplace.json"), []byte(`{"name":"store","owner":{"name":"author"},"plugins":[]}`), 0600); err != nil {
			t.Fatal(err)
		}
		if err := store.saveRepository(other, root, "", "", plugincatalog.Claude, admit); err == nil || err.Error() != "PLUGIN_REPOSITORY_INDEX_PATH" {
			t.Fatal(err)
		}
		rows, err := store.Repositories(other)
		if err != nil || len(rows) != 0 {
			t.Fatal(rows, err)
		}
	})
	credential := &credentials.AuthConfig{Source: "static", Type: "bearer", Token: "test-secret"}
	if err := manager.RegisterPatternWithContext(other, u.Host, credential); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRemoteRepository(ctx, server.URL+"/archive.zip", true, admit); err == nil || requests.Load() != 0 {
		t.Fatal("foreign credential or anonymous fallback was used", err)
	}
	if err := manager.RegisterPatternWithContext(ctx, u.Host, &credentials.AuthConfig{Type: "oauth2", Source: "static", ClientID: "fixture"}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRemoteRepository(ctx, server.URL+"/archive.zip", true, admit); err == nil || err.Error() != "PLUGIN_REPOSITORY_CREDENTIAL" || requests.Load() != 0 {
		t.Fatal("incompatible vault entry reached the repository", err)
	}
	if err := manager.RegisterPatternWithContext(ctx, u.Host, credential); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveRemoteRepository(ctx, server.URL+"/archive.zip", true, admit); err != nil {
		t.Fatal(err)
	}
	rows, err := store.Repositories(ctx)
	if err != nil || len(rows) != 1 {
		t.Fatal(rows, err)
	}
	row := rows[0]
	if err := store.SaveRepository(ctx, "", row.ID, row.Revision, admit); err != nil || conditional.Load() != 1 {
		t.Fatalf("expected cached index on 304, got %v (%d)", err, conditional.Load())
	}
	refreshed, err := store.Repositories(ctx)
	if err != nil || len(refreshed) != 1 || len(refreshed[0].Indices) != 1 || refreshed[0].Revision != row.Revision {
		t.Fatal(refreshed, err)
	}

	snap, err := store.InspectRepositoryEntry(ctx, row.ID, row.Revision, plugincatalog.Claude, "sample")
	if err != nil || snap.Manifest.Name != "sample" || snap.DiscoveryRepositoryID != row.ID {
		t.Fatal(snap, err)
	}
	if _, ok := snap.Files[".git/HEAD"]; ok {
		t.Fatal("archive retained repository metadata")
	}
	changed.Store(true)
	inspect := func() (*pluginpackage.LocalSnapshot, error) {
		return store.InspectRepositoryEntry(ctx, row.ID, row.Revision, plugincatalog.Claude, "sample")
	}
	if _, err := store.InstallSource(ctx, snap.Origin, snap.Digest, "", nil, admit, inspect); !errors.Is(err, ErrPackageChanged) {
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
	if _, err := store.InstallSource(ctx, snap.Origin, snap.Digest, "", nil, admit, inspect); err != nil {
		t.Fatal(err)
	}
	reject.Store(true)
	if _, err := inspect(); err == nil || err.Error() != "PLUGIN_REPOSITORY_CREDENTIAL" {
		t.Fatal("missing vault diagnostic", err)
	}
	reject.Store(false)
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
	block.Store(true)
	requestCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := store.InspectRepositoryEntry(requestCtx, row.ID, row.Revision, plugincatalog.Claude, "sample")
		result <- err
	}()
	select {
	case <-started:
	case <-requestCtx.Done():
		t.Fatal("preview never started")
	}
	if err := store.RemoveRepository(ctx, row.ID, row.Revision, admit); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err == nil || requestCtx.Err() != nil {
		t.Fatal("removing the repository did not cancel the preview", err)
	}
}

type storeRoundTripFunc func(*http.Request) (*http.Response, error)

func (f storeRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestRedirectedZIPDoesNotReuseAnotherResourcesValidator(t *testing.T) {
	var archive bytes.Buffer
	writer := zip.NewWriter(&archive)
	if _, err := writer.Create("data"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			http.Redirect(w, r, "/new.zip", http.StatusFound)
			return
		}
		if r.Header.Get("If-None-Match") != "" || r.Header.Get("If-Modified-Since") != "" {
			t.Error("validator forwarded")
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", `"v1"`)
		_, _ = w.Write(archive.Bytes())
	}))
	defer server.Close()
	inventory := NewInventory(nil).WithRemoteClient(func(string) *httpclient.Client {
		client := RepositoryHTTPClient(nil, "", nil)
		client.GetBaseClient().Transport = server.Client().Transport
		return client
	}, nil)
	cache := repositoryValidators{etag: `"v1"`, root: "old"}
	snap, digest, err := inventory.download(database.WithUserID(context.Background(), "owner"), server.URL+"/latest", "", false, &cache)
	if err != nil || snap == nil || digest == "old" || cache.etag != "" {
		t.Fatal(digest, cache, err)
	}
}

func TestRemoteStoreDefaultPortPersistsAcrossOperations(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "port.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	conn, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := db.AutoMigrate(&database.PluginRepository{}); err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	archive := zip.NewWriter(&body)
	w, err := archive.Create(".claude-plugin/marketplace.json")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, `{"name":"demo","owner":{"name":"publisher"},"plugins":[]}`); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	var ports []string
	makeInventory := func() *Inventory {
		return NewInventory(db).WithRemoteClient(func(ref string) *httpclient.Client {
			client := RepositoryHTTPClient(nil, ref, nil)
			client.GetBaseClient().Transport = storeRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				ports = append(ports, req.URL.Port())
				return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body.Bytes())), Header: make(http.Header)}, nil
			})
			return client
		}, nil)
	}
	ctx := database.WithUserID(context.Background(), "owner")
	admit := commandsecurity.Admission(func(_ *gorm.DB, fn func() error) error { return fn() })
	store := makeInventory()
	for _, address := range []string{"https://store.example", "https://store.example:0443", "https://store.example:443", "https://store.example/"} {
		if err := store.SaveRemoteRepository(ctx, address, false, admit); err != nil {
			t.Fatal(err)
		}
	}
	store = makeInventory()
	rows, err := store.Repositories(ctx)
	if err != nil || len(rows) != 1 || rows[0].Origin != "https://store.example/" {
		t.Fatal(rows, err)
	}
	if err := store.SaveRepository(ctx, "", rows[0].ID, rows[0].Revision, admit); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ports, []string{"", "443", "443", "", "443"}) {
		t.Fatalf("dispatched ports = %q", ports)
	}
}

type repositoryNetworkDecision struct {
	allow bool
	calls int
}

func (decision *repositoryNetworkDecision) Authorize(_ context.Context, dest httpclient.BlockedDestination) ([]net.IP, bool, error) {
	decision.calls++
	return dest.IPs, decision.allow, nil
}

func TestRemoteStoreNetworkAndCredentialChoice(t *testing.T) {
	var contacts atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contacts.Add(1)
		if r.Header.Get("Authorization") != "" {
			t.Error("public store received credentials")
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	ctx := database.WithUserID(context.Background(), "owner")
	manager := credentials.NewManager(nil)
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                        string
		private, allow              bool
		wantContacts, wantDecisions int32
	}{
		{"private_without_credential", true, true, 0, 0},
		{"public_network_denied", false, false, 0, 1},
		{"public_network_approved", false, true, 1, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := contacts.Load()
			ref := ""
			if tc.private {
				ref = u.Host
			} else if err := manager.RegisterPatternWithContext(ctx, u.Host, &credentials.AuthConfig{Type: "bearer", Source: "static", Token: "fixture"}); err != nil {
				t.Fatal(err)
			}
			decision := &repositoryNetworkDecision{allow: tc.allow}
			client := RepositoryHTTPClient(manager, ref, decision)
			if !tc.private {
				base := client.GetBaseClient().Transport.(*http.Transport)
				base.TLSClientConfig = server.Client().Transport.(*http.Transport).TLSClientConfig.Clone()
			}
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Do(ctx, req)
			if resp != nil {
				_ = resp.Body.Close()
			}
			if tc.private && !errors.Is(err, credentials.ErrCredentialResolution) || (err == nil) != (tc.wantContacts > 0) || contacts.Load()-before != tc.wantContacts || int32(decision.calls) != tc.wantDecisions {
				t.Fatalf("contacts=%d decisions=%d error=%v", contacts.Load()-before, decision.calls, err)
			}
		})
	}
}

func TestRepositoryInternationalAuthority(t *testing.T) {
	mixed, err := remoteURL("https://Store.Example:0443")
	if err != nil || mixed.Host != "Store.Example:0443" || addressHost(mixed, true) != "store.example" {
		t.Fatal(mixed, err)
	}
	unicodeHost, err := remoteURL("https://İ.example")
	if err != nil {
		t.Fatal(err)
	}
	if unicodeHost.Host != "İ.example" || addressHost(unicodeHost, true) == "i.example" {
		t.Fatalf("changed Unicode reference or target: %s / %s", unicodeHost.Host, addressHost(unicodeHost, true))
	}
	for _, raw := range []string{"https://bücher.example:0443", "https://xn--bcher-kva.example/"} {
		u, err := remoteURL(raw)
		if err != nil || addressHost(u, true) != "xn--bcher-kva.example" {
			t.Fatal(raw, err)
		}
	}
}

func TestRepositoryIPv6Authority(t *testing.T) {
	for _, raw := range []string{"https://[2001:0db8:0:0:0:0:0:1]:0443", "https://[2001:db8::1]/"} {
		u, err := remoteURL(raw)
		if err != nil || addressHost(u, true) != "[2001:db8::1]" {
			t.Fatal(raw, err)
		}
	}
}

func TestArchiveETagRequiresOneRepresentation(t *testing.T) {
	for _, tag := range []string{`"v1"`, `W/"v2"`, `""`} {
		if !validArchiveETag(tag) {
			t.Fatal(tag)
		}
	}
	for _, tag := range []string{"*", "", `"a","b"`, "abc", `W/*`} {
		if validArchiveETag(tag) {
			t.Fatal(tag)
		}
	}
}
