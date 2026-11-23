package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cliadapter "assistente/adapters/cli"
	"assistente/internal/apidto"
	"assistente/internal/app"
	"assistente/internal/auth"
	"assistente/internal/configdir"
	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/profiles"
	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"
)

// Permite mudar o estado entre duas linhas lidas pelo scanner do REPL.
type nextREPLInput func([]byte) (int, error)

func (read nextREPLInput) Read(buf []byte) (int, error) { return read(buf) }

// Exercita o PersistentPreRunE real de dois comandos em processos lógicos
// distintos da instância que fez login. Apenas o keyring do SO é simulado.
func TestConversationCLIUsesSavedSessionAcrossInstances(t *testing.T) {
	// MockInit troca o keyring do processo inteiro. O subprocesso mantém o
	// mock e os singletons de configuração/CLI isolados dos demais testes.
	const fixtureEnv = "ASSISTENTE_TEST_CLI_SAVED_SESSION"
	if os.Getenv(fixtureEnv) != "1" {
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		child := exec.CommandContext(t.Context(), exe, "-test.run=^TestConversationCLIUsesSavedSessionAcrossInstances$", "-test.timeout=80s")
		child.Env = append(os.Environ(), fixtureEnv+"=1")
		if out, err := child.CombinedOutput(); err != nil {
			t.Fatalf("fixture CLI: %v\n%s", err, out)
		}
		return
	}
	home := t.TempDir()
	t.Chdir(home)
	for _, key := range []string{"HOME", "USERPROFILE", "APPDATA", "LOCALAPPDATA", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME"} {
		t.Setenv(key, home)
	}
	keyring.MockInit()
	configdir.ResetForTests()
	t.Cleanup(func() { _ = database.Close() })
	cacheDir := filepath.Join(configdir.GetHomeDir(), "llm-catalog")
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		t.Fatal(err)
	}
	const cache = `{"fetched_at":"2020-01-01T00:00:00Z","document":{"schema_version":1,"catalog_version":"fixture","updated_at":"2020-01-01T00:00:00Z","servers":[],"models":[{"id":"vendor/fixture","vendor":"Vendor","name":"Fixture","status":"active","capabilities":{},"parameters":{"temperature":{"support":"supported","default":0.42}}}],"exposures":[]}}`
	if err := os.WriteFile(filepath.Join(cacheDir, "catalog.json"), []byte(cache), 0600); err != nil {
		t.Fatal(err)
	}
	var completions atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer fixture-key" {
			http.Error(w, "credencial ausente", http.StatusUnauthorized)
			return
		}
		var payload struct {
			Temperature *float64 `json:"temperature"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil || payload.Temperature == nil || *payload.Temperature != 0.42 {
			t.Errorf("default do catálogo no envio: %v, %v", payload.Temperature, err)
			http.Error(w, "temperatura inesperada", http.StatusBadRequest)
			return
		}
		completions.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"fixture","choices":[{"index":0,"delta":{"content":"resposta de prova"},"finish_reason":"stop"}]}

data: [DONE]

`)
	}))
	defer server.Close()
	var conversationID, profileSlug, disposableID string
	var foreign database.Conversation
	var previousCredential database.CredentialEntry
	var activeRun database.SubAgentRun
	var pendingJob database.Job
	// Usa a implementação de Login do desktop, sem iniciar serviços autônomos na fixture.
	loginApp := app.NewCommandCLIApp()
	initialize := func() {
		if err := loginApp.StartupWithAdapters(context.Background(), cliadapter.NewEmitterAdapter(), cliadapter.WindowAdapter{}, cliadapter.DialogAdapter{}); err != nil {
			t.Fatal(err)
		}
		defer loginApp.Shutdown()
		if _, err := loginApp.SetupVault("test-password"); err != nil {
			t.Fatal(err)
		}
		account := app.CreateAdminRequest{Username: "fixture", Password: "test-password"}
		admin, err := loginApp.CreateAdminUser(app.CreateAdminRequest{Username: "admin", Password: "other-password"})
		if err != nil {
			t.Fatal(err)
		}
		foreign = database.Conversation{UserID: admin.ID, Title: "conversa privada do admin"}
		if err := database.DB().Create(&foreign).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := auth.NewIdentityService(database.DB()).CreateLocalUser(context.Background(), auth.CreateUserParams{Username: account.Username, Password: account.Password, Admin: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := loginApp.Login(app.LoginRequest{Username: admin.Username, Password: "other-password"}); err != nil {
			t.Fatal(err)
		}
		if err := database.DB().First(&previousCredential, "pattern = ? AND user_id = ''", credentials.InstanceSecretAuthRefreshToken).Error; err != nil {
			t.Fatal(err)
		}
		staleMirror, err := credentials.LoadAuthRefreshTokenFromKeychain()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := loginApp.Login(app.LoginRequest{Username: account.Username, Password: account.Password, ClientLabel: "desktop"}); err != nil {
			t.Fatal(err)
		}
		// Simula login em outra conta com sucesso no banco, mas sem atualizar
		// o espelho do SO. Ambas as sessões são válidas.
		if err := credentials.SaveAuthRefreshTokenToKeychain(staleMirror); err != nil {
			t.Fatal(err)
		}
		cli := asCLI(loginApp)
		conv, err := cli.EnsureConversation("conversa de prova")
		if err != nil {
			t.Fatal(err)
		}
		conversationID = conv.ID
		owner, err := loginApp.GetAuthUser()
		if err != nil {
			t.Fatal(err)
		}
		disposable := database.Conversation{UserID: owner.UserID, Title: "descartável"}
		if err := database.DB().Create(&disposable).Error; err != nil {
			t.Fatal(err)
		}
		disposableID = disposable.ID
		if _, err := cli.CreateLLMProvider(apidto.CreateLLMProviderRequest{ID: "fixture", Name: "Fixture", Type: "openai", AuthMode: "required", APIKey: "fixture-key", APIFormat: "openai", BaseURL: server.URL + "/v1", DefaultModel: "fixture"}); err != nil {
			t.Fatal(err)
		}
		// Cadastro preexistente do desktop. O teste começa com o vínculo privado
		// já salvo; o comportamento sob prova é sua resolução no envio pela CLI.
		if err := database.DB().Model(&database.LLMProvider{}).Where("id = ?", "fixture").Updates(map[string]any{"model_aliases": `{"fixture":"vendor/fixture"}`, "api_format": "", "is_default": false}).Error; err != nil {
			t.Fatal(err)
		}
		profileSlug, err = cli.CreateProfile(profiles.Profile{Name: "Revisor teste", Chat: profiles.ChatConfig{LLMProvider: "fixture", Model: "fixture", ResponseTimeout: 30, EnabledSkills: []string{}}})
		if err != nil {
			t.Fatal(err)
		}

	}
	initialize()
	var session database.Session
	if err := database.DB().First(&session, "client_label = ?", "desktop").Error; err != nil {
		t.Fatal(err)
	}
	activeRun = database.SubAgentRun{UserID: session.UserID, ChildConversationID: conversationID, Status: database.SubAgentRunStatusRunning}
	if err := database.DB().Create(&activeRun).Error; err != nil {
		t.Fatal(err)
	}
	// Uma passagem de Start do scheduler reconcilia jobs sem grant antes de
	// registrar seus gatilhos. A CLI não deve alterar os jobs do outro processo.
	var tool database.ToolCatalog
	if err := database.DB().First(&tool, "name = ?", "subagent").Error; err != nil {
		t.Fatal(err)
	}
	pendingJob = database.Job{UserID: session.UserID, Name: "Fixture", Slug: "fixture", ToolCatalogID: tool.ID, ToolName: tool.Name, Enabled: true, Inputs: "{}"}
	if err := database.DB().Create(&pendingJob).Error; err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	run := func(cmd *cobra.Command, check func(error)) {
		t.Helper()
		defer func() {
			cleanupRootApp()
			var run database.SubAgentRun
			if err := database.DB().First(&run, "id = ?", activeRun.ID).Error; err != nil || run.Status != database.SubAgentRunStatusRunning {
				t.Errorf("subagente do desktop = %q, %v", run.Status, err)
			}
			if err := database.Close(); err != nil {
				t.Error(err)
			}
		}()
		check(rootCmd.PersistentPreRunE(cmd, nil))
	}
	run(historyListCmd, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
		cli := asCLI(rootApp)
		var list bytes.Buffer
		if err := runHistoryList(cli, &list, "", 10); err != nil || (!strings.Contains(list.String(), "conversa de prova") || strings.Contains(list.String(), "privada do admin")) {
			t.Fatalf("listagem: %q, %v", list.String(), err)
		}

	})
	run(historyShowCmd, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
		if err := historyShowCmd.RunE(historyShowCmd, []string{conversationID}); err != nil {
			t.Fatal(err)
		}
		if err := historyShowCmd.RunE(historyShowCmd, []string{foreign.ID}); err == nil {
			t.Fatal("exibiu histórico de outra conta")
		}
	})
	run(historyDeleteCmd, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
		if err := historyDeleteCmd.RunE(historyDeleteCmd, []string{disposableID}); err != nil {
			t.Fatal(err)
		}
		if err := historyDeleteCmd.RunE(historyDeleteCmd, []string{foreign.ID}); err == nil {
			t.Fatal("aceitou exclusão em outra conta")
		}
		var row database.Conversation
		if err := database.DB().First(&row, "id = ?", foreign.ID).Error; err != nil {
			t.Fatal(err)
		}
		var count int64
		if err := database.DB().Model(&database.Conversation{}).Where("id = ?", disposableID).Count(&count).Error; err != nil || count != 0 {
			t.Fatalf("exclusão da conversa própria: %d, %v", count, err)
		}
	})
	run(chatCmd, func(err error) {
		if err != nil {
			t.Fatal(err)
		}
		user, err := rootApp.GetAuthUser()
		if err != nil || user.UserID != session.UserID || user.SessionID != session.ID {
			t.Fatalf("identidade do chat: %+v %v", user, err)
		}
		previousID, previousProfile := chatConversationID, chatProfile
		defer func() { chatConversationID, chatProfile = previousID, previousProfile }()
		chatConversationID, chatProfile = conversationID, profileSlug
		if err := chatCmd.RunE(chatCmd, []string{"Mensagem de prova"}); err != nil {
			t.Fatal(err)
		}
		if completions.Load() == 0 {
			t.Fatal("o provedor da conta não recebeu a mensagem")
		}
		var stored database.LLMProvider
		if err := database.DB().First(&stored, "id = ?", "fixture").Error; err != nil || stored.APIFormat != "" || stored.IsDefault {
			t.Fatalf("chat alterou o provedor compartilhado: formato=%q padrão=%v erro=%v", stored.APIFormat, stored.IsDefault, err)
		}
		var history bytes.Buffer
		if err := runHistoryShow(asCLI(rootApp), &history, conversationID); err != nil || !strings.Contains(history.String(), "resposta de prova") {
			t.Fatalf("conversa restaurada: %q, %v", history.String(), err)
		}
		var job database.Job
		if err := database.DB().First(&job, "id = ?", pendingJob.ID).Error; err != nil || !job.Enabled {
			t.Fatalf("job reconciliado pela CLI, enabled=%v, err=%v", job.Enabled, err)
		}
		var current database.Session
		if err := database.DB().First(&current, "id = ?", session.ID).Error; err != nil {
			t.Fatal(err)
		}
		if current.RefreshTokenHash != session.RefreshTokenHash || !current.ExpiresAt.Equal(session.ExpiresAt) {
			t.Fatal("a CLI alterou a sessão do desktop")
		}
		// Um login concorrente altera a linha de instância. A sessão deste
		// REPL continua válida no banco; a conversa não deve migrar para a outra.
		var savedCredential database.CredentialEntry
		if err := database.DB().First(&savedCredential, "id = ?", previousCredential.ID).Error; err != nil {
			t.Fatal(err)
		}
		activeTokenEnc := savedCredential.TokenEnc
		completionsBefore := completions.Load()
		changedInput := nextREPLInput(func(buf []byte) (int, error) {
			if err := database.DB().Model(&savedCredential).UpdateColumn("token_enc", previousCredential.TokenEnc).Error; err != nil {
				return 0, err
			}
			return copy(buf, "segunda mensagem\n"), io.EOF
		})
		input := io.MultiReader(strings.NewReader("primeira mensagem\n"), changedInput)
		var diagnostic bytes.Buffer
		if err := runREPL(asCLI(rootApp), cliEmitter, input, &diagnostic); err != nil {
			t.Fatal(err)
		}
		if completions.Load() != completionsBefore+1 || !strings.Contains(diagnostic.String(), "reabra a CLI") {
			t.Fatalf("troca de sessão entre turnos: %q, requisições=%d", diagnostic.String(), completions.Load()-completionsBefore)
		}
		if _, err := asCLI(rootApp).GetConversation(conversationID); !errors.Is(err, auth.ErrUnauthenticatedLocalSession) {
			t.Fatalf("troca de conta durante REPL: %v", err)
		}
		if err := database.DB().Model(&savedCredential).UpdateColumn("token_enc", activeTokenEnc).Error; err != nil {
			t.Fatal(err)
		}
		userRow := database.DB().Model(&database.User{}).Where("id = ?", session.UserID)
		if err := userRow.UpdateColumn("role", database.UserRoleUser).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := asCLI(rootApp).GetConversations(); !errors.Is(err, auth.ErrUnauthenticatedLocalSession) {
			t.Fatalf("rebaixamento de admin: %v", err)
		}
		if err := userRow.UpdateColumn("role", database.UserRoleAdmin).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.DB().Model(&current).Update("revoked_at", time.Now()).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := asCLI(rootApp).GetConversations(); !errors.Is(err, auth.ErrSessionRevoked) {
			t.Fatalf("acesso após logout no desktop: %v; esperado ErrSessionRevoked", err)
		}
	})
	run(historyListCmd, func(err error) {
		if !errors.Is(err, auth.ErrSessionRevoked) {
			t.Fatalf("sessão revogada com espelho de outra conta: %v", err)
		}
		current := session
		if err := database.DB().Model(&current).Update("revoked_at", nil).Error; err != nil {
			t.Fatal(err)
		}
		if err := database.DB().Model(&current).Update("expires_at", time.Now().Add(-time.Hour)).Error; err != nil {
			t.Fatal(err)
		}
	})
	run(chatCmd, func(err error) {
		if !errors.Is(err, auth.ErrSessionExpired) {
			t.Fatalf("sessão vencida: %v", err)
		}
	})
	run(historyListCmd, func(err error) {
		if !errors.Is(err, auth.ErrSessionExpired) {
			t.Fatalf("histórico com sessão vencida: %v", err)
		}
		if _, err := app.AuthenticatedContext(rootApp); err == nil {
			t.Fatal("histórico acessível sem autorização")
		}
		if err := database.DB().Model(&database.CredentialEntry{}).Where("pattern = ?", credentials.InstanceSecretAuthRefreshToken).Update("token_enc", "fixture-corrompido").Error; err != nil {
			t.Fatal(err)
		}
	})
	run(historyListCmd, func(err error) {
		if err == nil || errors.Is(err, auth.ErrInvalidRefreshToken) || !strings.Contains(err.Error(), "cofre") || strings.Contains(err.Error(), "fixture-corrompido") {
			t.Fatalf("leitura de segredo ilegível: %v", err)
		}
		if err := database.DB().Where("pattern = ?", credentials.InstanceSecretAuthRefreshToken).Delete(&database.CredentialEntry{}).Error; err != nil {
			t.Fatal(err)
		}
	})
	run(historyListCmd, func(err error) {
		if !errors.Is(err, auth.ErrInvalidRefreshToken) {
			t.Fatalf("histórico sem sessão: %v", err)
		}
	})
}
