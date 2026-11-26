package plugins

import (
	"archive/zip"
	"assistente/internal/database"
	httpclient "assistente/internal/tools/http"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/pluginpackage"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/memory"
)

func TestGitStoreReadsAdvertisedCommitOverApplicationHTTP(t *testing.T) {
	storage := memory.NewStorage()
	var hashes []plumbing.Hash
	store := func(o *plumbing.MemoryObject) plumbing.Hash {
		t.Helper()
		h, err := storage.SetEncodedObject(o)
		if err != nil {
			t.Fatal(err)
		}
		hashes = append(hashes, h)
		return h
	}
	blob := new(plumbing.MemoryObject)
	blob.SetType(plumbing.BlobObject)
	if _, err := blob.Write([]byte(`{"$schema":"` + pluginpackage.ManifestSchema + `","name":"sample"}`)); err != nil {
		t.Fatal(err)
	}
	tree := object.Tree{Entries: []object.TreeEntry{{Name: "plugin.json", Mode: filemode.Regular, Hash: store(blob)}}}
	treeObj := new(plumbing.MemoryObject)
	if err := tree.Encode(treeObj); err != nil {
		t.Fatal(err)
	}
	author := object.Signature{Name: "test", Email: "test@example.com", When: time.Unix(1, 0)}
	commit := object.Commit{TreeHash: store(treeObj), Author: author, Committer: author, Message: "fixture"}
	commitObj := new(plumbing.MemoryObject)
	if err := commit.Encode(commitObj); err != nil {
		t.Fatal(err)
	}
	head := store(commitObj)
	var pack bytes.Buffer
	if _, err := packfile.NewEncoder(&pack, storage, false).Encode(hashes, 0); err != nil {
		t.Fatal(err)
	}
	pkt := func(s string) string { return fmt.Sprintf("%04x%s", len(s)+4, s) }
	var packs atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/team/store.git/info/refs":
			if r.Method != "GET" || r.URL.Query().Get("service") != "git-upload-pack" {
				t.Error(r.Method, r.URL)
			}
			w.Header().Set("Content-Type", "application/x-git-upload-pack-advertisement")
			_, _ = io.WriteString(w, pkt("# service=git-upload-pack\n")+"0000"+pkt(head.String()+" HEAD\x00shallow symref=HEAD:refs/heads/main\n")+pkt(head.String()+" refs/heads/main\n")+"0000")
		case "/team/store.git/git-upload-pack":
			packs.Add(1)
			data, err := io.ReadAll(r.Body)
			if err != nil || r.Method != "POST" || !strings.Contains(string(data), "want "+head.String()) || !strings.Contains(string(data), "deepen 1") {
				t.Error(r.Method, string(data), err)
			}
			w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
			_, _ = io.WriteString(w, pkt("shallow "+head.String()+"\n")+"0000"+pkt("NAK\n"))
			_, _ = w.Write(pack.Bytes())
		default:
			t.Error(r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := RepositoryHTTPClient(nil, "", nil)
	client.GetBaseClient().Transport = server.Client().Transport
	archive, revision, err := readGitRepository(context.Background(), client, server.URL+"/team/store.git", "")
	if err != nil {
		t.Fatal(err)
	}
	if revision != gitRevisionPrefix+head.String() {
		t.Fatal(revision)
	}
	unchanged, hash, err := readGitRepository(context.Background(), client, server.URL+"/team/store.git", revision)
	if err != nil || unchanged != nil || hash != revision || packs.Load() != 1 {
		t.Fatal(hash, err, packs.Load())
	}
	snap, err := archive.Package(server.URL, ".")
	if err != nil || snap.Manifest.Name != "sample" {
		t.Fatal(snap, err)
	}
}

func TestGitStoreRejectsOtherContainersBeforeDecoding(t *testing.T) {
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := checkGitPack(context.Background(), data.Bytes()); err == nil {
		t.Fatal("accepted non-pack")
	}
}

func TestGitStoreBoundsZIPDirectory(t *testing.T) {
	var output bytes.Buffer
	writer := zip.NewWriter(&gitArchiveWriter{out: &output, remaining: 64})
	if _, err := writer.CreateHeader(&zip.FileHeader{Name: "directory/with/a/long/name/repeated-in-the-central-directory"}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); !errors.Is(err, pluginpackage.ErrTooLarge) {
		t.Fatal(err)
	}
	if output.Len() > 64 {
		t.Fatal(output.Len())
	}
}
func TestGitStoreClassifiesHTTPAuthenticationFailure(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusUnauthorized) }))
	defer server.Close()
	i := NewInventory(nil).WithRemoteClient(func(string) *httpclient.Client {
		client := RepositoryHTTPClient(nil, "", nil)
		client.GetBaseClient().Transport = server.Client().Transport
		return client
	}, nil)
	_, _, err := i.download(database.WithUserID(context.Background(), "owner"), server.URL, "", true, &repositoryValidators{})
	if err == nil || err.Error() != "PLUGIN_REPOSITORY_CREDENTIAL" {
		t.Fatal(err)
	}
}

func TestRemoteCatalogPrefixUsesActualGitRoot(t *testing.T) {
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	if _, err := writer.Create("release/.cursor-plugin/marketplace.json"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	archive, err := pluginpackage.OpenArchive(data.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if prefix := remoteCatalogPrefix(archive, "git:revision"); prefix != "" {
		t.Fatal(prefix)
	}
	if prefix := remoteCatalogPrefix(archive, "digest"); prefix != "release/" {
		t.Fatal(prefix)
	}
}
