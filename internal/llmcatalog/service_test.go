package llmcatalog

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type doerFunc func(context.Context, *http.Request) (*http.Response, error)

func (fn doerFunc) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	return fn(ctx, req)
}

type closeErrorBody struct{ io.Reader }

func (closeErrorBody) Close() error { return errors.New("falha ao fechar body") }

func TestRefreshValidatesAndPersistsThenOfflineLoadsCache(t *testing.T) {
	dir := t.TempDir()
	service := New(Config{
		Dir: dir,
		HTTP: doerFunc(func(context.Context, *http.Request) (*http.Response, error) {
			return response(http.StatusOK, minimalCatalog, http.Header{"Content-Type": []string{"application/json"}, "ETag": []string{`"catalog-v1"`}}), nil
		}),
	})
	got := service.Refresh(context.Background())
	if got.Document.CatalogVersion != "v1" || got.FromCache || got.FetchedAt.IsZero() || got.Refreshing {
		t.Fatalf("snapshot após refresh = %#v", got)
	}

	offline := New(Config{Dir: dir, HTTP: doerFunc(func(context.Context, *http.Request) (*http.Response, error) {
		return nil, errors.New("offline")
	})})
	offline.Start(context.Background())
	defer offline.Close()
	loaded := offline.Snapshot()
	if loaded.Document.CatalogVersion != "v1" || !loaded.FromCache {
		t.Fatalf("cache offline = %#v", loaded)
	}
}

func TestRefreshKeepsSuccessfulCatalogWhenBodyCloseFails(t *testing.T) {
	service := New(Config{Dir: t.TempDir(), HTTP: doerFunc(func(context.Context, *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       closeErrorBody{Reader: strings.NewReader(minimalCatalog)},
		}, nil
	})})

	got := service.Refresh(context.Background())
	if got.Document.CatalogVersion != "v1" || got.ReasonCode != "" || got.FromCache {
		t.Fatalf("falha de limpeza não deve invalidar catálogo publicado: %#v", got)
	}
}

func TestInvalidRefreshPreservesLastValidDocument(t *testing.T) {
	var calls atomic.Int32
	service := New(Config{Dir: t.TempDir(), HTTP: doerFunc(func(context.Context, *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return response(http.StatusOK, minimalCatalog, http.Header{"Content-Type": []string{"application/json"}}), nil
		}
		return response(http.StatusOK, `{"schema_version":99}`, http.Header{"Content-Type": []string{"application/json"}}), nil
	})})
	if got := service.Refresh(context.Background()); got.Document.CatalogVersion != "v1" {
		t.Fatalf("primeiro refresh = %#v", got)
	}
	got := service.Refresh(context.Background())
	if got.Document.CatalogVersion != "v1" || got.ReasonCode != "malformed_catalog" {
		t.Fatalf("refresh inválido deve preservar cache válido: %#v", got)
	}
}

func TestRefreshDoesNotPublishCatalogWhenCacheWriteFails(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	service := New(Config{Dir: dir, HTTP: doerFunc(func(context.Context, *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return response(http.StatusOK, minimalCatalog, http.Header{"Content-Type": []string{"application/json"}}), nil
		}
		updated := strings.Replace(minimalCatalog, `"catalog_version":"v1"`, `"catalog_version":"v2"`, 1)
		return response(http.StatusOK, updated, http.Header{"Content-Type": []string{"application/json"}}), nil
	})})
	first := service.Refresh(context.Background())
	cachePath := filepath.Join(dir, cacheFileName)
	if err := os.Remove(cachePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cachePath, 0o700); err != nil {
		t.Fatal(err)
	}

	got := service.Refresh(context.Background())
	if got.Document.CatalogVersion != first.Document.CatalogVersion || !got.FetchedAt.Equal(first.FetchedAt) || got.ReasonCode != "cache_write_failed" {
		t.Fatalf("catálogo não persistido foi publicado: antes=%#v depois=%#v", first, got)
	}
}

func TestNotModifiedDoesNotPublishFreshnessWhenCacheWriteFails(t *testing.T) {
	dir := t.TempDir()
	var calls atomic.Int32
	service := New(Config{Dir: dir, HTTP: doerFunc(func(context.Context, *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return response(http.StatusOK, minimalCatalog, http.Header{"Content-Type": []string{"application/json"}, "ETag": []string{`"catalog-v1"`}}), nil
		}
		return response(http.StatusNotModified, "", http.Header{"ETag": []string{`"catalog-v1"`}}), nil
	})})
	first := service.Refresh(context.Background())
	cachePath := filepath.Join(dir, cacheFileName)
	if err := os.Remove(cachePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(cachePath, 0o700); err != nil {
		t.Fatal(err)
	}

	got := service.Refresh(context.Background())
	if got.Document.CatalogVersion != first.Document.CatalogVersion || !got.FetchedAt.Equal(first.FetchedAt) || got.FromCache != first.FromCache || got.ReasonCode != "cache_write_failed" {
		t.Fatalf("frescor não persistido foi publicado: antes=%#v depois=%#v", first, got)
	}
}

func TestRefreshUsesConditionalRequestAndRetainsDocumentOnNotModified(t *testing.T) {
	var calls atomic.Int32
	service := New(Config{Dir: t.TempDir(), HTTP: doerFunc(func(_ context.Context, req *http.Request) (*http.Response, error) {
		if calls.Add(1) == 1 {
			return response(http.StatusOK, minimalCatalog, http.Header{
				"Content-Type": []string{"application/json"},
				"ETag":         []string{`"catalog-v1"`},
			}), nil
		}
		if req.Header.Get("If-None-Match") != `"catalog-v1"` {
			t.Errorf("If-None-Match = %q", req.Header.Get("If-None-Match"))
		}
		return response(http.StatusNotModified, "", http.Header{"ETag": []string{`"catalog-v1"`}}), nil
	})})
	base := time.Date(2026, 10, 17, 12, 0, 0, 0, time.UTC)
	var ticks atomic.Int64
	service.now = func() time.Time { return base.Add(time.Duration(ticks.Add(1)) * time.Second) }
	first := service.Refresh(context.Background())
	second := service.Refresh(context.Background())
	if second.Document.CatalogVersion != first.Document.CatalogVersion || !second.FromCache || !second.FetchedAt.After(first.FetchedAt) {
		t.Fatalf("snapshot após 304 = %#v; anterior = %#v", second, first)
	}
}

func TestStartReturnsWhileBackgroundFetchIsPending(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	service := New(Config{Dir: t.TempDir(), HTTP: doerFunc(func(ctx context.Context, _ *http.Request) (*http.Response, error) {
		close(started)
		select {
		case <-release:
			return response(http.StatusOK, minimalCatalog, http.Header{"Content-Type": []string{"application/json"}}), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})})
	start := time.Now()
	service.Start(ctx)
	if time.Since(start) > time.Second {
		t.Fatal("Start bloqueou esperando a rede")
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("busca em background não começou")
	}
	cancel()
	close(release)
	service.Close()
}

func response(status int, body string, headers http.Header) *http.Response {
	canonical := make(http.Header, len(headers))
	for name, values := range headers {
		for _, value := range values {
			canonical.Add(name, value)
		}
	}
	return &http.Response{StatusCode: status, Header: canonical, Body: io.NopCloser(strings.NewReader(body))}
}
