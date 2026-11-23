package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
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
		completions.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, `data: {"id":"chatcmpl-test","object":"chat.completion.chunk","created":1,"model":"fixture","choices":[{"index":0,"delta":{"content":"resposta de prova"},"finish_reason":"stop"}]}

data: [DONE]

`)
	}))
	defer server.Close()
	var conversationID, profileSlug string
	var activeRun database.SubAgentRun
	var pendingJob database.Job
	desktop := app.NewCommandCLIApp()
	initialize := func() {
		if err := desktop.StartupWithAdapters(context.Background(), cliadapter.NewEmitterAdapter(), cliadapter.WindowAdapter{}, cliadapter.DialogAdapter{}); err != nil {
			t.Fatal(err)
		}
		defer desktop.Shutdown()
		if _, err := desktop.SetupVault("test-password"); err != nil {
			t.Fatal(err)
		}
		account := app.CreateAdminRequest{Username: "fixture", Password: "test-password"}
		admin, err := desktop.CreateAdminUser(app.CreateAdminRequest{Username: "admin", Password: "other-password"})
		if err != nil {
			t.Fatal(err)
		}
		if err := database.DB().Create(&database.Conversation{UserID: admin.ID, Title: "conversa privada do admin"}).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := auth.NewIdentityService(database.DB()).CreateLocalUser(context.Background(), auth.CreateUserParams{Username: account.Username, Password: account.Password}); err != nil {
			t.Fatal(err)
		}
		if _, err := desktop.Login(app.LoginRequest{Username: account.Username, Password: account.Password, ClientLabel: "desktop"}); err != nil {
			t.Fatal(err)
		}
		cli := asCLI(desktop)
		conv, err := cli.EnsureConversation("conversa de prova")
		if err != nil {
			t.Fatal(err)
		}
		conversationID = conv.ID
		if _, err := cli.CreateLLMProvider(apidto.CreateLLMProviderRequest{ID: "fixture", Name: "Fixture", Type: "openai", AuthMode: "required", APIKey: "fixture-key", APIFormat: "openai", BaseURL: server.URL + "/v1", DefaultModel: "fixture"}); err != nil {
			t.Fatal(err)
		}
		profileSlug, err = cli.CreateProfile(profiles.Profile{Name: "Revisor teste", Chat: profiles.ChatConfig{LLMProvider: "fixture", Model: "fixture", ResponseTimeout: 30, EnabledSkills: []string{}}})
		if err != nil {
			t.Fatal(err)
		}

	}
	initialize()
	var session database.Session
	if err := database.DB().First(&session).Error; err != nil {
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
		var history bytes.Buffer
		if err := runHistoryShow(asCLI(rootApp), &history, conversationID); err != nil || !strings.Contains(history.String(), "resposta de prova") {
			t.Fatalf("conversa restaurada: %q, %v", history.String(), err)
		}
		var run database.SubAgentRun
		if err := database.DB().First(&run, "id = ?", activeRun.ID).Error; err != nil || run.Status != database.SubAgentRunStatusRunning {
			t.Fatalf("subagente do desktop = %q, %v", run.Status, err)
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
		if err := database.DB().Model(&current).Update("revoked_at", time.Now()).Error; err != nil {
			t.Fatal(err)
		}
		if _, err := asCLI(rootApp).GetConversations(); err == nil {
			t.Fatal("a CLI manteve acesso após logout no desktop")
		}
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
		if err := credentials.DeleteAuthRefreshTokenFromKeychain(); err != nil {
			t.Fatal(err)
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
