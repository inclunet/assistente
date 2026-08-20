package docextract

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
)

func TestProjectionCacheHitsAndReturnsCopies(t *testing.T) {
	cache := NewProjectionCache(CacheConfig{MaxEntries: 2, MaxBytes: 1024})
	identity := FileIdentityFromStat(10, 20)
	var loads atomic.Int32
	load := func(context.Context) (*Result, error) {
		loads.Add(1)
		return &Result{Kind: KindPDF, Markdown: "conteúdo", Warnings: []string{"aviso"}}, nil
	}

	first, origin, err := cache.GetOrLoad(context.Background(), "a.pdf", identity, load)
	if err != nil || origin != OriginLoaded {
		t.Fatalf("primeira carga: origin=%v err=%v", origin, err)
	}
	first.Markdown = "alterado fora"
	first.Warnings[0] = "alterado fora"

	second, origin, err := cache.GetOrLoad(context.Background(), "a.pdf", identity, load)
	if err != nil || origin != OriginCached {
		t.Fatalf("segunda carga: origin=%v err=%v", origin, err)
	}
	if second.Markdown != "conteúdo" || second.Warnings[0] != "aviso" {
		t.Fatalf("cache expôs estado mutável: %+v", second)
	}
	if loads.Load() != 1 {
		t.Fatalf("loads=%d, quer 1", loads.Load())
	}
}

func TestProjectionCacheInvalidatesChangedIdentity(t *testing.T) {
	cache := NewProjectionCache(CacheConfig{MaxEntries: 2, MaxBytes: 1024})
	var loads atomic.Int32
	load := func(context.Context) (*Result, error) {
		n := loads.Add(1)
		return &Result{Kind: KindPDF, Markdown: string(rune('0' + n))}, nil
	}

	first, _, err := cache.GetOrLoad(context.Background(), "a.pdf", FileIdentityFromStat(10, 20), load)
	if err != nil {
		t.Fatal(err)
	}
	second, origin, err := cache.GetOrLoad(context.Background(), "a.pdf", FileIdentityFromStat(11, 21), load)
	if err != nil {
		t.Fatal(err)
	}
	if origin != OriginLoaded || first.Markdown == second.Markdown || loads.Load() != 2 {
		t.Fatalf(
			"identidade nova reutilizou cache: first=%q second=%q origin=%v loads=%d",
			first.Markdown, second.Markdown, origin, loads.Load(),
		)
	}
}

func TestProjectionCacheEvictsByEntriesAndBytes(t *testing.T) {
	cache := NewProjectionCache(CacheConfig{MaxEntries: 2, MaxBytes: 7})
	loads := map[string]int{}
	load := func(path, body string) func(context.Context) (*Result, error) {
		return func(context.Context) (*Result, error) {
			loads[path]++
			return &Result{Kind: KindPDF, Markdown: body}, nil
		}
	}

	for _, tc := range []struct {
		path string
		body string
	}{
		{"a", "aaa"},
		{"b", "bbb"},
		{"c", "ccc"},
	} {
		if _, _, err := cache.GetOrLoad(context.Background(), tc.path, FileIdentity{}, load(tc.path, tc.body)); err != nil {
			t.Fatal(err)
		}
	}
	if _, origin, err := cache.GetOrLoad(context.Background(), "a", FileIdentity{}, load("a", "aaa")); err != nil || origin != OriginLoaded {
		t.Fatalf("entrada mais antiga deveria ter sido removida: origin=%v err=%v", origin, err)
	}
	if loads["a"] != 2 {
		t.Fatalf("loads[a]=%d, quer 2", loads["a"])
	}
}

func TestProjectionCacheCoalescesConcurrentLoads(t *testing.T) {
	cache := NewProjectionCache(CacheConfig{MaxEntries: 2, MaxBytes: 1024})
	started := make(chan struct{})
	release := make(chan struct{})
	var loads atomic.Int32
	load := func(context.Context) (*Result, error) {
		if loads.Add(1) == 1 {
			close(started)
		}
		<-release
		return &Result{Kind: KindPDF, Markdown: "ok"}, nil
	}

	const readers = 8
	var wg sync.WaitGroup
	errs := make(chan error, readers)
	var loaded, reused atomic.Int32
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, origin, err := cache.GetOrLoad(context.Background(), "a.pdf", FileIdentity{}, load)
			switch {
			case err != nil:
				errs <- err
			case result.Markdown != "ok":
				errs <- errors.New("resultado inesperado")
			case origin == OriginLoaded:
				loaded.Add(1)
			default:
				reused.Add(1)
			}
		}()
	}
	<-started
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
	if loads.Load() != 1 {
		t.Fatalf("loads=%d, quer 1", loads.Load())
	}
	// Só quem disparou a extração pode se declarar OriginLoaded; os demais
	// pegaram carona (ou acharam a entrada pronta) e não custaram extração.
	if loaded.Load() != 1 || reused.Load() != readers-1 {
		t.Fatalf("loaded=%d reused=%d, quer 1 e %d", loaded.Load(), reused.Load(), readers-1)
	}
}

func TestProjectionCacheWaiterCanCancel(t *testing.T) {
	cache := NewProjectionCache(CacheConfig{MaxEntries: 2, MaxBytes: 1024})
	started := make(chan struct{})
	release := make(chan struct{})
	load := func(context.Context) (*Result, error) {
		close(started)
		<-release
		return &Result{Kind: KindPDF, Markdown: "ok"}, nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _, _ = cache.GetOrLoad(context.Background(), "a.pdf", FileIdentity{}, load)
	}()
	<-started

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := cache.GetOrLoad(ctx, "a.pdf", FileIdentity{}, load)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v, quer Canceled", err)
	}
	close(release)
	<-done
}

func TestProjectionCacheOwnerCancellationDoesNotFailActiveWaiter(t *testing.T) {
	cache := NewProjectionCache(CacheConfig{MaxEntries: 2, MaxBytes: 1024})
	started := make(chan struct{})
	release := make(chan struct{})
	var loads atomic.Int32
	load := func(ctx context.Context) (*Result, error) {
		loads.Add(1)
		close(started)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return &Result{Kind: KindPDF, Markdown: "ok"}, nil
		}
	}

	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	ownerDone := make(chan error, 1)
	go func() {
		_, _, err := cache.GetOrLoad(ownerCtx, "a.pdf", FileIdentity{}, load)
		ownerDone <- err
	}()
	<-started

	waiterDone := make(chan error, 1)
	go func() {
		result, _, err := cache.GetOrLoad(context.Background(), "a.pdf", FileIdentity{}, load)
		if err == nil && result.Markdown != "ok" {
			err = errors.New("resultado inesperado")
		}
		waiterDone <- err
	}()

	// O teste está no mesmo pacote para confirmar deterministicamente que o
	// segundo interessado entrou no voo antes de cancelar o dono.
	for {
		cache.mu.Lock()
		waiters := 0
		for _, flight := range cache.flights {
			waiters = flight.waiters
		}
		cache.mu.Unlock()
		if waiters == 2 {
			break
		}
		runtime.Gosched()
	}

	cancelOwner()
	if err := <-ownerDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("dono: err=%v, quer Canceled", err)
	}
	close(release)
	if err := <-waiterDone; err != nil {
		t.Fatalf("waiter ativo falhou: %v", err)
	}
	if loads.Load() != 1 {
		t.Fatalf("loads=%d, quer 1", loads.Load())
	}
}

func TestProjectionCacheNewCallerDoesNotJoinAbandonedFlight(t *testing.T) {
	cache := NewProjectionCache(CacheConfig{MaxEntries: 2, MaxBytes: 1024})
	firstStarted := make(chan struct{})
	var loads atomic.Int32
	load := func(ctx context.Context) (*Result, error) {
		if loads.Add(1) == 1 {
			close(firstStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return &Result{Kind: KindPDF, Markdown: "novo"}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, _, err := cache.GetOrLoad(ctx, "a.pdf", FileIdentity{}, load)
		firstDone <- err
	}()
	<-firstStarted
	cancel()
	if err := <-firstDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("primeiro err=%v", err)
	}

	result, _, err := cache.GetOrLoad(context.Background(), "a.pdf", FileIdentity{}, load)
	if err != nil || result.Markdown != "novo" {
		t.Fatalf("novo chamador herdou voo cancelado: result=%+v err=%v", result, err)
	}
	if loads.Load() != 2 {
		t.Fatalf("loads=%d, quer 2", loads.Load())
	}
}
