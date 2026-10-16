package credentials

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"assistente/internal/database"
)

type commandCacheFixture struct {
	m                  *Manager
	ctx                context.Context
	config             *AuthConfig
	value, calls, hold string
}

func newCommandCacheFixture(t *testing.T) commandCacheFixture {
	t.Helper()
	dir := t.TempDir()
	f := commandCacheFixture{m: NewManager(nil), ctx: database.WithUserID(context.Background(), "cache-user"), value: filepath.Join(dir, "value"), calls: filepath.Join(dir, "calls"), hold: filepath.Join(dir, "hold")}
	t.Setenv("ASSISTENTE_CREDENTIAL_TEST_HELPER", "1")
	t.Setenv("CREDENTIAL_CACHE_VALUE", f.value)
	t.Setenv("CREDENTIAL_CACHE_CALLS", f.calls)
	t.Setenv("CREDENTIAL_CACHE_HOLD", f.hold)
	f.config = &AuthConfig{Source: "command", Type: "bearer", SourceConfig: commandTestConfig(t, "cache")}
	f.setValue(t, "first")
	if err := f.m.RegisterPatternWithContext(f.ctx, "cache.example", f.config); err != nil {
		t.Fatal(err)
	}
	return f
}
func (f commandCacheFixture) setValue(t *testing.T, value string) {
	t.Helper()
	if err := os.WriteFile(f.value, []byte(value), 0600); err != nil {
		t.Fatal(err)
	}
}
func (f commandCacheFixture) count() int {
	b, _ := os.ReadFile(f.calls)
	return strings.Count(string(b), "call\n")
}
func (f commandCacheFixture) get(t *testing.T) *AuthConfig {
	t.Helper()
	a, e := f.m.getByPatternWithContext(f.ctx, "cache.example", true)
	if e != nil || a == nil {
		t.Fatalf("resolve: %v", e)
	}
	return a
}
func waitCacheCalls(t *testing.T, f commandCacheFixture, count int) {
	t.Helper()
	end := time.Now().Add(10 * time.Second)
	for f.count() < count && time.Now().Before(end) {
		time.Sleep(10 * time.Millisecond)
	}
	if f.count() < count {
		t.Fatal("helper não iniciou")
	}
}

func TestCommandCacheConcurrentAndMetadata(t *testing.T) {
	f := newCommandCacheFixture(t)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, e := f.m.getByPatternWithContext(f.ctx, "cache.example", true)
			if e != nil || a == nil || a.Token != "first" {
				t.Errorf("resolve concorrente: %v", e)
			}
		}()
	}
	wg.Wait()
	if f.count() != 1 {
		t.Fatalf("execuções: %d", f.count())
	}
	f.get(t).Token = "changed-by-caller"
	if f.get(t).Token != "first" {
		t.Fatal("resultado compartilhado mutável")
	}
	listed, e := f.m.ListCredentialsWithContext(f.ctx)
	if e != nil || len(listed) != 1 || listed[0].Auth.Token != "" {
		t.Fatal("cache vazou para metadados")
	}
	f.m.mu.RLock()
	cached := f.m.credentials[0].command.encrypted.Token
	f.m.mu.RUnlock()
	if cached == "first" || cached == "" {
		t.Fatal("cache não cifrado")
	}
	cancelled, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, e = f.m.getByPatternWithContext(cancelled, "cache.example", true); !errors.Is(e, context.Canceled) {
		t.Fatalf("hit ignorou cancelamento: %v", e)
	}
}

func TestCommandCacheIsolationAndInvalidation(t *testing.T) {
	f := newCommandCacheFixture(t)
	old := f.get(t)
	other := database.WithUserID(context.Background(), "other-user")
	f.setValue(t, "second")
	if e := f.m.RegisterPatternWithContext(other, "cache.example", f.config); e != nil {
		t.Fatal(e)
	}
	a, e := f.m.getByPatternWithContext(other, "cache.example", true)
	if e != nil || a.Token != "second" {
		t.Fatal("isolamento")
	}
	if f.get(t).Token != "first" {
		t.Fatal("alteração de outro usuário invalidou cache")
	}
	if _, e := f.m.getByPatternWithContext(context.Background(), "cache.example", true); e != nil {
		t.Fatal(e)
	}
	if !f.m.rejectCommandCredential(old) {
		t.Fatal("401 não invalidou")
	}
	if f.get(t).Token != "second" {
		t.Fatal("não renovou")
	}
	f.setValue(t, "third")
	f.m.rejectCommandCredential(old) // resposta atrasada, já houve renovação
	if f.get(t).Token != "second" {
		t.Fatal("401 atrasado apagou valor renovado")
	}
	if e := f.m.RegisterPatternWithContext(f.ctx, "cache.example", f.config); e != nil {
		t.Fatal(e)
	}
	if f.get(t).Token != "third" {
		t.Fatal("edição não invalidou")
	}
	f.m.ClearCommandCache()
	f.setValue(t, "fourth")
	if f.get(t).Token != "fourth" {
		t.Fatal("sessão não invalidou")
	}
	if e := f.m.DeletePattern(f.ctx, "cache.example"); e != nil {
		t.Fatal(e)
	}
	a, e = f.m.getByPatternWithContext(f.ctx, "cache.example", true)
	if e != nil || a != nil {
		t.Fatal("delete não invalidou")
	}
	f.m.Reset(nil, false)
	a, e = f.m.getByPatternWithContext(other, "cache.example", true)
	if e != nil || a != nil {
		t.Fatal("reset não invalidou")
	}
}

func TestCommandCacheCancellationAndReplacement(t *testing.T) {
	f := newCommandCacheFixture(t)
	if e := os.WriteFile(f.hold, []byte("hold"), 0600); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := f.m.getByPatternWithContext(f.ctx, "cache.example", true); done <- e }()
	waitCacheCalls(t, f, 1)
	waiter, cancel := context.WithCancel(f.ctx)
	cancel()
	if _, e := f.m.getByPatternWithContext(waiter, "cache.example", true); !errors.Is(e, context.Canceled) {
		t.Fatal("waiter não cancelado")
	}
	if e := f.m.RegisterPatternWithContext(f.ctx, "cache.example", f.config); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if !errors.Is(e, errCommandCredentialChanged) {
			t.Fatalf("resultado obsoleto: %v", e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("comando antigo não cancelado")
	}
	if e := os.Remove(f.hold); e != nil {
		t.Fatal(e)
	}
	f.setValue(t, "new")
	if f.get(t).Token != "new" || f.count() != 2 {
		t.Fatal("resultado antigo reaproveitado")
	}
}

func TestCommandCacheDoesNotCacheFailures(t *testing.T) {
	f := newCommandCacheFixture(t)
	f.setValue(t, "")
	if _, e := f.m.getByPatternWithContext(f.ctx, "cache.example", true); e == nil {
		t.Fatal("esperava falha")
	}
	f.setValue(t, "fixed")
	if f.get(t).Token != "fixed" || f.count() != 2 {
		t.Fatal("falha foi cacheada")
	}
}

func TestCommandTransportRefresh(t *testing.T) {
	for _, status := range []int{401, 400, 403, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			f := newCommandCacheFixture(t)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				body, e := io.ReadAll(r.Body)
				if e != nil || string(body) != "payload" {
					t.Error("corpo de retry diferente")
				}
				if calls == 1 {
					f.setValue(t, "second")
					w.WriteHeader(status)
					return
				}
				if r.Header.Get("Authorization") != "Bearer second" {
					t.Error("retry não renovado")
				}
				w.WriteHeader(200)
			}))
			defer server.Close()
			tr := NewCredentialTransport(f.m, "cache.example")
			req, _ := http.NewRequestWithContext(f.ctx, "POST", server.URL, strings.NewReader("payload"))
			response, e := tr.RoundTrip(req)
			if e != nil {
				t.Fatal(e)
			}
			_ = response.Body.Close()
			wantCalls := 1
			wantStatus := status
			if status == 401 {
				wantCalls = 2
				wantStatus = 200
			}
			if calls != wantCalls || f.count() != wantCalls || response.StatusCode != wantStatus {
				t.Fatalf("HTTP=%d comandos=%d status=%d", calls, f.count(), response.StatusCode)
			}
			if req.Header.Get("Authorization") != "" {
				t.Fatal("request original alterado")
			}
		})
	}
}

func TestCommandTransportRetryLimits(t *testing.T) {
	for _, mode := range []string{"same-token", "second-401", "non-replayable", "refresh-failure"} {
		t.Run(mode, func(t *testing.T) {
			f := newCommandCacheFixture(t)
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				switch mode {
				case "second-401":
					f.setValue(t, "second")
				case "refresh-failure":
					f.setValue(t, "")
				}
				w.WriteHeader(401)
			}))
			defer server.Close()
			req, _ := http.NewRequestWithContext(f.ctx, "POST", server.URL, strings.NewReader("payload"))
			if mode == "non-replayable" {
				req.GetBody = nil
			}
			response, e := NewCredentialTransport(f.m, "cache.example").RoundTrip(req)
			if mode == "refresh-failure" {
				if !errors.Is(e, ErrCredentialResolution) || response != nil {
					t.Fatal("falha de renovação não propagada como credencial")
				}
			} else {
				if e != nil || response.StatusCode != 401 {
					t.Fatalf("response=%v err=%v", response, e)
				}
				_ = response.Body.Close()
			}
			want := 1
			if mode == "second-401" {
				want = 2
			}
			if calls != want {
				t.Fatalf("requests=%d", calls)
			}
			commandCalls := 2
			if mode == "non-replayable" {
				commandCalls = 1
			}
			if f.count() != commandCalls {
				t.Fatalf("comandos=%d", f.count())
			}
		})
	}
}

func TestCommandDirectConsumersRemainUncached(t *testing.T) {
	f := newCommandCacheFixture(t)
	a, e := f.m.GetByPatternWithContext(f.ctx, "cache.example")
	if e != nil || a.Token != "first" {
		t.Fatal(e)
	}
	f.setValue(t, "second")
	a, e = f.m.ResolveForURLWithContext(f.ctx, "https://cache.example")
	if e != nil || a.Token != "second" {
		t.Fatal("consumidor direto recebeu cache obsoleto")
	}
	if f.count() != 2 {
		t.Fatal("consumidores diretos não executaram comando")
	}
}

func TestCommandCacheWaitingCancellation(t *testing.T) {
	f := newCommandCacheFixture(t)
	if e := os.WriteFile(f.hold, []byte("hold"), 0600); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := f.m.getByPatternWithContext(f.ctx, "cache.example", true); done <- e }()
	waitCacheCalls(t, f, 1)
	ctx, cancel := context.WithTimeout(f.ctx, 100*time.Millisecond)
	defer cancel()
	if _, e := f.m.getByPatternWithContext(ctx, "cache.example", true); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("waiter: %v", e)
	}
	if e := os.Remove(f.hold); e != nil {
		t.Fatal(e)
	}
	select {
	case e := <-done:
		if e != nil {
			t.Fatal(e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("leader cancelado pelo waiter")
	}
	if f.get(t).Token != "first" || f.count() != 1 {
		t.Fatal("waiter causou reexecução")
	}
}

func TestCommandCacheSessionDiscardsInflight(t *testing.T) {
	f := newCommandCacheFixture(t)
	if e := os.WriteFile(f.hold, []byte("hold"), 0600); e != nil {
		t.Fatal(e)
	}
	done := make(chan error, 1)
	go func() { _, e := f.m.getByPatternWithContext(f.ctx, "cache.example", true); done <- e }()
	waitCacheCalls(t, f, 1)
	f.m.ClearCommandCache()
	select {
	case e := <-done:
		if !errors.Is(e, errCommandCredentialChanged) {
			t.Fatal(e)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("sessão não cancelou execução")
	}
	if e := os.Remove(f.hold); e != nil {
		t.Fatal(e)
	}
	f.setValue(t, "new-session")
	if f.get(t).Token != "new-session" || f.count() != 2 {
		t.Fatal("cache de outra sessão")
	}
}

type commandCacheRoundTripFunc func(*http.Request) (*http.Response, error)

func (f commandCacheRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestCommandTransportNetworkErrorDoesNotRenew(t *testing.T) {
	f := newCommandCacheFixture(t)
	tr := NewCredentialTransport(f.m, "cache.example")
	sentinel := errors.New("network failure")
	tr.Base = commandCacheRoundTripFunc(func(*http.Request) (*http.Response, error) { return nil, sentinel })
	req, _ := http.NewRequestWithContext(f.ctx, "GET", "https://cache.example", nil)
	if _, e := tr.RoundTrip(req); !errors.Is(e, sentinel) {
		t.Fatal(e)
	}
	f.setValue(t, "changed")
	if f.get(t).Token != "first" || f.count() != 1 {
		t.Fatal("rede invalidou cache")
	}
}

func TestCommandTransportConcurrentUnauthorizedRenewsOnce(t *testing.T) {
	f := newCommandCacheFixture(t)
	const concurrency = 8
	var mu sync.Mutex
	rejected := 0
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer first" {
			mu.Lock()
			rejected++
			if rejected == concurrency {
				f.setValue(t, "second")
				close(ready)
			}
			mu.Unlock()
			select {
			case <-ready:
			case <-r.Context().Done():
				return
			}
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("Authorization") != "Bearer second" {
			t.Error("token inesperado")
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	tr := NewCredentialTransport(f.m, "cache.example")
	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			response, e := tr.RoundTrip(req)
			if e != nil {
				t.Error(e)
				return
			}
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != 200 {
				t.Errorf("status=%d", response.StatusCode)
			}
		}()
	}
	wg.Wait()
	if f.count() != 2 {
		t.Fatalf("rajada de401 executou comando %d vezes", f.count())
	}
}

func TestCommandTransportConcurrentUnchangedTokenRenewsOnce(t *testing.T) {
	f := newCommandCacheFixture(t)
	const concurrency = 8
	var mu sync.Mutex
	rejected := 0
	ready := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer first" {
			mu.Lock()
			rejected++
			if rejected == concurrency {
				// Mantém o token rejeitado para todos os consumidores.
				close(ready)
			}
			mu.Unlock()
			select {
			case <-ready:
			case <-r.Context().Done():
				return
			}
			w.WriteHeader(401)
			return
		}
		if r.Header.Get("Authorization") != "Bearer second" {
			t.Error("token inesperado")
		}
		w.WriteHeader(200)
	}))
	defer server.Close()
	tr := NewCredentialTransport(f.m, "cache.example")
	ctx, cancel := context.WithTimeout(f.ctx, 20*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
			response, e := tr.RoundTrip(req)
			if e != nil {
				t.Error(e)
				return
			}
			defer func() { _ = response.Body.Close() }()
			if response.StatusCode != 401 {
				t.Errorf("status=%d", response.StatusCode)
			}
		}()
	}
	wg.Wait()
	if f.count() != 2 {
		t.Fatalf("rajada de401 executou comando %d vezes", f.count())
	}
	// Uma chamada posterior pode recuperar quando o comando passar a emitir token novo.
	f.setValue(t, "second")
	req, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	response, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != 200 || f.count() != 3 {
		t.Fatalf("recuperação: status=%d comandos=%d", response.StatusCode, f.count())
	}

}

type commandCacheCloseTransport struct{ closed bool }

func (t *commandCacheCloseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("unused")
}
func (t *commandCacheCloseTransport) CloseIdleConnections() { t.closed = true }
func TestCommandTransportForwardsCloseIdleConnections(t *testing.T) {
	base := &commandCacheCloseTransport{}
	client := &http.Client{Transport: &CredentialTransport{Base: base}}
	client.CloseIdleConnections()
	if !base.closed {
		t.Fatal("conexões ociosas não foram fechadas")
	}
}

func TestCommandCacheReloadPreservesOnlyIdenticalStoredEntry(t *testing.T) {
	setupScopedCredentialStoreTestDB(t)
	f := newCommandCacheFixture(t)
	store := NewDBStore()
	f.m = NewManagerWithStore(f.m.encKey, store, true)
	if err := f.m.RegisterPatternWithContext(f.ctx, "cache.example", f.config); err != nil {
		t.Fatal(err)
	}
	first := f.get(t)
	if err := f.m.LoadUserCredentials(f.ctx, "cache-user"); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t); got.commandEntry != first.commandEntry || f.count() != 1 {
		t.Fatal("recarga idêntica perdeu cache")
	}
	// Uma edição externa persistida exige descartar o cache, mesmo mantendo o ID.
	other := NewManagerWithStore(f.m.encKey, store, true)
	if err := other.RegisterPatternWithContext(f.ctx, "cache.example", f.config); err != nil {
		t.Fatal(err)
	}
	f.setValue(t, "second")
	if err := f.m.LoadUserCredentials(f.ctx, "cache-user"); err != nil {
		t.Fatal(err)
	}
	if got := f.get(t); got.Token != "second" || got.commandEntry == first.commandEntry || f.count() != 2 {
		t.Fatal("recarga alterada preservou cache antigo")
	}
}
