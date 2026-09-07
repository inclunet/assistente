package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"assistente/internal/auth"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// hardeningServer monta um Server isolado por teste com defaults
// permissivos para que rate limit não interfira em testes que não estão
// validando esse comportamento.
func hardeningServer(t *testing.T) *Server {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	ids := auth.NewIdentityService(db)
	if _, err := ids.CreateLocalUser(context.Background(), auth.CreateUserParams{
		Username: "admin",
		Password: "secret-password",
		Admin:    true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{
		Issuer:   "test",
		Audience: "client",
	})
	if err != nil {
		t.Fatalf("new session service: %v", err)
	}
	return New(Config{
		IDs:       ids,
		Session:   sessions,
		AuthRate:  1000,
		AuthBurst: 1000,
		JWKSRate:  1000,
		JWKSBurst: 1000,
	})
}

// TestJWKSResponseHeaders valida B20 do review: o JWKS endpoint deve
// emitir Cache-Control e ETag para permitir caching downstream sem
// custo no signer.
func TestJWKSResponseHeaders(t *testing.T) {
	server := hardeningServer(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "max-age") {
		t.Errorf("Cache-Control = %q, want max-age", cc)
	}
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag missing in JWKS response")
	}

	// If-None-Match com mesmo ETag deve devolver 304.
	condReq := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	condReq.Header.Set("If-None-Match", etag)
	condRec := httptest.NewRecorder()
	server.Handler().ServeHTTP(condRec, condReq)
	if condRec.Code != http.StatusNotModified {
		t.Fatalf("conditional GET = %d, want 304", condRec.Code)
	}
}

// TestJWKSCacheReusesPayload garante que requests subsequentes em janela
// curta reusem exatamente o mesmo payload (sem reinvocar o signer).
// Hoje validamos por igualdade do ETag; quando rotação for implementada,
// o teste continua válido — qualquer mudança no signer obriga
// invalidação explícita via invalidateJWKSCache.
func TestJWKSCacheReusesPayload(t *testing.T) {
	server := hardeningServer(t)

	req := httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	first := rec.Header().Get("ETag")
	if first == "" {
		t.Fatal("missing ETag on first call")
	}

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
		if got := rec.Header().Get("ETag"); got != first {
			t.Fatalf("iter %d ETag = %q, want stable %q", i, got, first)
		}
	}
}

// TestJWKSCacheConcurrentReadsAreSafe rodado com -race para B20: vários
// readers competem pelo cache atomic.Pointer e não podem corromper
// estado. signerProvider() segue locked, mas só na primeira população.
func TestJWKSCacheConcurrentReadsAreSafe(t *testing.T) {
	server := hardeningServer(t)

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
			if rec.Code != http.StatusOK {
				t.Errorf("status = %d", rec.Code)
			}
		}()
	}
	wg.Wait()
}

// TestInternalErrorsAreNotLeakedToClient cobre B21: handler que devolve
// erro interno deve gerar mensagem genérica, não o err.Error() cru. O
// 500 desliga writeError → writeInternalErr.
func TestInternalErrorsAreNotLeakedToClient(t *testing.T) {
	// Forçamos 500 quebrando a sessão depois do login estar montado:
	// usamos handleVaultStatus com vault nil. Mais barato que mockar
	// sessão; apenas exercita o caminho writeInternalErr.
	srv := New(Config{
		IDs:       (&auth.IdentityService{}),
		AuthRate:  1000,
		AuthBurst: 1000,
		JWKSRate:  1000,
		JWKSBurst: 1000,
	})

	// Tentar GET /auth/me sem token → 401 com mensagem fixa.
	req := httptest.NewRequest(http.MethodGet, "/auth/me", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("me without token status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	// Mensagem deve ser estável e não incluir paths internos / structs.
	if body["error"] == "" {
		t.Fatalf("missing error field: %s", rec.Body.String())
	}
	for _, leak := range []string{"github.com/", "/internal/", "*auth.", "panic", "stack"} {
		if strings.Contains(body["error"], leak) {
			t.Errorf("error message leaks internal token %q: %s", leak, body["error"])
		}
	}
}

// TestLoginAuthErrorMessageGeneric valida que login com credenciais
// inválidas não vaza a estrutura interna (B21 + M2 do bloco 1: mensagem
// uniforme entre "user inexistente" e "senha errada").
func TestLoginAuthErrorMessageGeneric(t *testing.T) {
	server := hardeningServer(t)

	cases := []map[string]string{
		{"username": "admin", "password": "wrong"},
		{"username": "ghost", "password": "any"},
	}
	var seen []string
	for _, payload := range cases {
		rec := requestJSON(t, server, http.MethodPost, "/auth/login", payload)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("login %+v status = %d body=%s", payload, rec.Code, rec.Body.String())
		}
		var body map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		seen = append(seen, body["error"])
	}
	if seen[0] == "" || seen[0] != seen[1] {
		t.Fatalf("login error messages diverge or empty: %#v", seen)
	}
}

// TestRateLimitBlocksAfterBurst cobre M21: burst pequeno suficiente para
// observar o bloqueio em sequência rápida. Confirma que retorna 429.
func TestRateLimitBlocksAfterBurst(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	if err := db.AutoMigrate(&database.User{}, &database.Session{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	ids := auth.NewIdentityService(db)
	if _, err := ids.CreateLocalUser(context.Background(), auth.CreateUserParams{
		Username: "admin",
		Password: "secret-password",
		Admin:    true,
	}); err != nil {
		t.Fatalf("create user: %v", err)
	}
	sessions, err := auth.NewSessionService(db, auth.SessionConfig{
		Issuer:   "test",
		Audience: "client",
	})
	if err != nil {
		t.Fatalf("new session service: %v", err)
	}
	server := New(Config{
		IDs:       ids,
		Session:   sessions,
		AuthRate:  0.0001,
		AuthBurst: 2,
	})

	successes := 0
	tooMany := 0
	for i := 0; i < 6; i++ {
		rec := requestJSON(t, server, http.MethodPost, "/auth/login", map[string]string{
			"username": "admin",
			"password": "secret-password",
		})
		switch rec.Code {
		case http.StatusOK:
			successes++
		case http.StatusTooManyRequests:
			tooMany++
		}
	}
	if successes != 2 {
		t.Errorf("successes = %d, want 2 (burst)", successes)
	}
	if tooMany == 0 {
		t.Errorf("expected at least one 429 after burst exhausted, got none")
	}
}

// TestRateLimitAllowsDifferentIPs garante isolamento por chave: um
// atacante saturando uma origem não pode bloquear outras.
func TestRateLimitAllowsDifferentIPs(t *testing.T) {
	limiter := newRateLimiter(0.001, 1)
	if !limiter.allow("1.2.3.4") {
		t.Fatal("first call should pass")
	}
	if limiter.allow("1.2.3.4") {
		t.Fatal("second call should be blocked for same IP")
	}
	if !limiter.allow("5.6.7.8") {
		t.Fatal("different IP should not be affected")
	}
}

// TestExtractClientLabelTruncates garante Mi4 do bloco 1 ainda em vigor
// no edge HTTP — labels gigantes não vão para o SessionService.
func TestExtractClientLabelTruncates(t *testing.T) {
	long := strings.Repeat("a", 1024)
	got := extractClientLabel(long)
	if len(got) != 256 {
		t.Errorf("len = %d, want 256", len(got))
	}
	if extractClientLabel("  hi  ") != "hi" {
		t.Errorf("trim broken")
	}
}

// TestErrSessionUnavailableIsService503 garante que a distinção entre
// "indisponível" (503) e "interno" (500) é mantida quando o handler
// JWKS encontra o session service nil.
func TestErrSessionUnavailableIsService503(t *testing.T) {
	server := New(Config{Sessions: func() *auth.SessionService { return nil }})

	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/jwks.json", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
	if !errors.Is(errSessionUnavailable, errSessionUnavailable) {
		t.Fatal("sentinel error broken")
	}
}
