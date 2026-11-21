package updater

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func downloadForTest(ctx context.Context, u *Updater, url, kind string) ([]byte, error) {
	if kind == "installer" {
		path, err := u.downloadInstaller(ctx, url, 3)
		if err != nil {
			return nil, err
		}
		defer func() { _ = os.Remove(path) }()
		return os.ReadFile(path)
	}
	r, err := u.downloadBinary(ctx, url)
	if err != nil {
		return nil, err
	}
	f := r.(*os.File)
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = r.Close() }()
	return io.ReadAll(r)
}

func TestDownloadSlowBodyOutlivesMetadataTimeout(t *testing.T) {
	for _, kind := range []string{"installer", "binary"} {
		t.Run(kind, func(t *testing.T) {
			u := New("test", nil)
			u.httpClient.GetBaseClient().Timeout = 30 * time.Millisecond
			u.SetGitHubToken("test-token")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer test-token" {
					t.Error("token não preservado")
				}
				if r.URL.Path == "/redirect" {
					http.Redirect(w, r, "/asset", http.StatusFound)
					return
				}
				for _, part := range []string{"a", "b", "c"} {
					_, _ = io.WriteString(w, part)
					w.(http.Flusher).Flush()
					time.Sleep(60 * time.Millisecond)
				}
			}))
			defer server.Close()
			data, err := downloadForTest(context.Background(), u, server.URL+"/redirect", kind)
			if err != nil || string(data) != "abc" {
				t.Fatalf("download = %q, %v", data, err)
			}
			if u.httpClient.GetBaseClient().Timeout != 30*time.Millisecond {
				t.Fatal("download alterou cliente de metadados")
			}
		})
	}
}

func TestDownloadStopsAndRemovesPartialFile(t *testing.T) {
	for _, kind := range []string{"installer", "binary"} {
		for _, failure := range []string{"idle-header", "idle-body", "cancel", "total"} {
			t.Run(kind+"/"+failure, func(t *testing.T) {
				dir := t.TempDir()
				for _, key := range []string{"TMP", "TEMP", "TMPDIR"} {
					t.Setenv(key, dir)
				}
				u := New("test", nil)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				u.downloadIdle = 100 * time.Millisecond
				if failure == "total" {
					u.downloadIdle = time.Second
					u.downloadClient.GetBaseClient().Timeout = 100 * time.Millisecond
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if failure != "idle-header" {
						_, _ = io.WriteString(w, "a")
						w.(http.Flusher).Flush()
					}
					<-r.Context().Done()
				}))
				defer server.Close()
				if failure == "cancel" {
					u.progressCallback = func(n, _ int64, _ string) {
						if n > 0 {
							cancel()
						}
					}
				}
				_, err := downloadForTest(ctx, u, server.URL, kind)
				want := error(context.DeadlineExceeded)
				if failure == "cancel" {
					want = context.Canceled
				}
				if !errors.Is(err, want) {
					t.Fatalf("erro = %v; esperado %v", err, want)
				}
				if (failure == "idle-header" || failure == "idle-body") && !errors.Is(err, ErrDownloadIdle) {
					t.Fatalf("causa de inatividade perdida: %v", err)
				}
				files, err := os.ReadDir(dir)
				if err != nil || len(files) != 0 {
					t.Fatalf("arquivo parcial não removido: %v, %v", files, err)
				}
			})
		}
	}
}

func TestDownloadPolicy(t *testing.T) {
	u := New("test", nil)
	if u.httpClient.GetBaseClient().Timeout != 30*time.Second || u.downloadClient.GetBaseClient().Timeout != 15*time.Minute || u.downloadIdle != time.Minute {
		t.Fatal("limites de consulta e download incorretos")
	}
}
