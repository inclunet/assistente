package mcp

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/credentials"
	"assistente/internal/database"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// setupMCPCredsTestDB inicializa um DB SQLite em memória com o schema
// de credenciais para testes locais ao package mcp (o helper análogo
// no package credentials é interno e não exportável).
func setupMCPCredsTestDB(t *testing.T) {
	t.Helper()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(&database.CredentialEntry{}, &database.CredentialKeyWrap{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	database.SetDB(db)
	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		database.SetDB(nil)
	})
}

// TestLoadConfigs_DoesNotAutoConnect prova o contrato do AEP-0061
// (fase MCP): `LoadConfigs` apenas popula o estado em memória, NUNCA
// dispara conexão. A versão antiga fazia `go m.Connect(slug)` para
// cada server enabled+autoconnect dentro do próprio LoadConfigs, e
// como LoadConfigs roda no startup pré-login, todos os servidores
// OAuth perdiam o token em memória, caíam no fallback "sem token" e
// abriam o navegador em paralelo.
//
// Auto-connect agora é responsabilidade exclusiva de
// `AutoConnectAll`, chamado por `reloadUserScopedRuntime` pós-login.
func TestLoadConfigs_DoesNotAutoConnect(t *testing.T) {
	m := newTestManagerWithTempDir(t)

	cfg := []byte(`{
		"name": "Test Server",
		"transport": "streamable_http",
		"url": "http://127.0.0.1:0/mcp",
		"enabled": true,
		"auto_connect": true
	}`)
	if err := m.resolver.Create("test-server.json", cfg); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if err := m.LoadConfigs(); err != nil {
		t.Fatalf("LoadConfigs: %v", err)
	}

	m.mu.RLock()
	_, registered := m.servers["test-server"]
	connectionCount := len(m.connections)
	m.mu.RUnlock()

	if !registered {
		t.Fatal("LoadConfigs deveria popular m.servers com o config carregado")
	}
	if connectionCount != 0 {
		t.Fatalf("LoadConfigs NÃO pode iniciar conexão; m.connections=%d (esperado 0). Regressão AEP-0061.", connectionCount)
	}
}

// TestAutoConnectAll_RespectsCancelledContext: AutoConnectAll é serial
// e cancela imediatamente quando o ctx morre. Sem isso, login com
// timeout curto (10s no reloadUserScopedRuntime) congelaria
// indefinidamente em servidor MCP travado.
func TestAutoConnectAll_RespectsCancelledContext(t *testing.T) {
	m := newTestManagerWithTempDir(t)

	for _, slug := range []string{"a", "b", "c"} {
		m.servers[slug] = &ServerStatus{
			Slug: slug,
			Config: ServerConfig{
				Enabled:     true,
				AutoConnect: true,
				Transport:   TransportStreamable,
				URL:         "http://127.0.0.1:1/mcp",
			},
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		m.AutoConnectAll(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("AutoConnectAll não respeitou ctx cancelado em 2s")
	}

	m.mu.RLock()
	count := len(m.connections)
	m.mu.RUnlock()
	if count != 0 {
		t.Fatalf("AutoConnectAll com ctx cancelado não deveria conectar nada; got %d", count)
	}
}

// TestLoadUserTokens_RespectsUserScopedCtx documenta o segundo fix
// do AEP-0061 (fase MCP): `loadUserTokens(ctx, ...)` precisa do ctx
// user-scoped vigente. Sem isso, mesmo POS-Login (com a credencial
// user-scoped no banco e em memória), o filtro anti-leak do Manager
// (`GetByPatternWithContext` ignora user-scoped quando userID==""
// no ctx) bloqueava qualquer leitura — exatamente o que reproduzia o
// "credencial gerenciada não resolvida" toda vez que o app reabria.
func TestLoadUserTokens_RespectsUserScopedCtx(t *testing.T) {
	setupMCPCredsTestDB(t)

	credMgr := credentials.NewManagerWithStoreAndPersistence(
		[]byte("test-key-exactly-32-bytes-long!!"),
		credentials.NewDBStore(),
		true,
	)
	userCtx := database.WithUserID(context.Background(), "user-1")
	if err := credMgr.RegisterPatternWithContext(userCtx, userTokensPattern("test"), &credentials.AuthConfig{
		Type:       "oauth2",
		Token:      "user-1-access-token",
		RefreshURL: "user-1-refresh",
		ExpiresAt:  time.Now().Add(1 * time.Hour).Unix(),
	}); err != nil {
		t.Fatalf("seed token: %v", err)
	}

	if got := loadUserTokens(userCtx, credMgr, "test"); got == nil || got.AccessToken != "user-1-access-token" {
		t.Fatalf("loadUserTokens(userCtx) não achou token user-scoped; got %+v", got)
	}

	if got := loadUserTokens(context.Background(), credMgr, "test"); got != nil {
		t.Fatalf("loadUserTokens(bgCtx) NÃO deveria achar token user-scoped (filtro anti-leak); got %+v", got)
	}

	otherCtx := database.WithUserID(context.Background(), "user-2")
	if got := loadUserTokens(otherCtx, credMgr, "test"); got != nil {
		t.Fatalf("loadUserTokens(otherUserCtx) NÃO deveria achar token de outro user; got %+v", got)
	}
}

// TestOAuthFlowArbiter_SerializesConcurrentCallers garante que o
// arbiter global do package serializa flows interativos entre
// servidores diferentes. Era a causa do sintoma "todos os MCPs abrem
// browser ao mesmo tempo no startup": antes, dois `authorize()`
// concorrentes faziam `browser.OpenURL` em paralelo.
//
// O teste usa o mutex diretamente para evitar montar todo o flow
// OAuth real (que dependeria de discovery, callback server, etc.); o
// contrato sob teste é "enquanto um caller segura o lock, nenhum
// outro entra".
func TestOAuthFlowArbiter_SerializesConcurrentCallers(t *testing.T) {
	const concurrency = 5
	var (
		active    int32
		maxActive int32
		wg        sync.WaitGroup
	)

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			oauthFlowArbiter.Lock()
			defer oauthFlowArbiter.Unlock()
			now := atomic.AddInt32(&active, 1)
			for {
				cur := atomic.LoadInt32(&maxActive)
				if now <= cur || atomic.CompareAndSwapInt32(&maxActive, cur, now) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			atomic.AddInt32(&active, -1)
		}()
	}
	wg.Wait()

	if max := atomic.LoadInt32(&maxActive); max != 1 {
		t.Fatalf("oauthFlowArbiter falhou em serializar: max simultâneo = %d (esperado 1)", max)
	}
}

// TestDisconnectAll_KeepsManagerUsable garante que `DisconnectAll`
// (o caminho do logout/troca de user) não derruba o Manager — só
// fecha conexões. Diferente de `CloseAll` que invalida o ctx base e
// é shutdown definitivo.
func TestDisconnectAll_KeepsManagerUsable(t *testing.T) {
	m := newTestManagerWithTempDir(t)
	m.servers["x"] = &ServerStatus{Slug: "x", Config: ServerConfig{Enabled: true}}

	m.DisconnectAll()

	if err := m.ctx.Err(); err != nil {
		t.Fatalf("DisconnectAll cancelou o ctx do Manager (deveria ser shutdown-only do CloseAll): %v", err)
	}
	if _, ok := m.servers["x"]; !ok {
		t.Fatal("DisconnectAll não pode descartar o registro de servers; só fecha conexões")
	}
}
