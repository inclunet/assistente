package app

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"assistente/internal/commandcli"
	"assistente/internal/commandexecution"
	"assistente/internal/commandledger"
	"assistente/internal/commandruntime"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/questionnaire"
	"github.com/google/uuid"
)

func TestCommandCLIListsFullCatalogAndDescribesUnavailableWorkspaceCommand(t *testing.T) {
	a := readyCommandProduct(t)
	cli, err := NewCommandCLI(a)
	if err != nil {
		t.Fatalf("NewCommandCLI: %v", err)
	}

	items, err := cli.List(context.Background(), "en")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got := len(items); got != 150 {
		t.Fatalf("catálogo CLI = %d itens, esperado 150", got)
	}

	description, err := cli.Describe(context.Background(), commandProductWorkspaceListID, "en")
	if err != nil {
		t.Fatalf("Describe workspace.list: %v", err)
	}
	payload := mustJSONMap(t, description)
	if payload["id"] != commandProductWorkspaceListID {
		t.Fatalf("Describe retornou ID inesperado: %#v", payload["id"])
	}
	if executable, ok := jsonBool(payload, "executable"); !ok || executable {
		t.Fatalf("workspace.list deveria estar indisponível na CLI: %#v", payload)
	}
	if reason, ok := jsonString(payload, "unavailable_reason"); !ok || strings.TrimSpace(reason) == "" {
		t.Fatalf("workspace.list deveria expor motivo estável de indisponibilidade: %#v", payload)
	}
}

func TestCommandCLIRejectsVisualWorkspaceAndLayerCommandsWithoutQuestionnaireOrEffect(t *testing.T) {
	a := readyCommandProduct(t)
	var questionnaires atomic.Int32
	a.questionnaireMgr = questionnaire.NewManager(func(event string, _ any) {
		if event == questionnaire.EventQuestionnaire {
			questionnaires.Add(1)
		}
	})
	cli, err := NewCommandCLI(a)
	if err != nil {
		t.Fatalf("NewCommandCLI: %v", err)
	}

	requests := []struct {
		id   string
		args json.RawMessage
	}{
		{commandProductShortcutsShowID, json.RawMessage(`{}`)},
		{commandProductWorkspaceListID, json.RawMessage(`{}`)},
		{commandLayerActivateID, json.RawMessage(`{"scope":"global","rule_id":"rule","duration_seconds":0}`)},
	}
	for _, request := range requests {
		t.Run(request.id, func(t *testing.T) {
			result, err := cli.Execute(context.Background(), commandcli.Request{
				CommandID: request.id,
				Arguments: request.args,
			})
			if err == nil {
				t.Fatalf("comando CLI indisponível retornou nil error: %+v", result)
			}
			if result.Status != string(commandledger.Denied) {
				t.Fatalf("comando indisponível sem status denied: %+v err=%v", result, err)
			}
			var statusErr *commandcli.StatusError
			if !errors.As(err, &statusErr) {
				t.Fatalf("recusa não retornou StatusError: %T %v", err, err)
			}
			if statusErr.Status != commandledger.Denied {
				t.Fatalf("StatusError com status inesperado: %+v", statusErr)
			}
			requireUUIDv7(t, result.RequestID)
			if len(result.Output) != 0 {
				t.Fatalf("recusa devolveu output: %s", result.Output)
			}
		})
	}
	if got := questionnaires.Load(); got != 0 {
		t.Fatalf("CLI abriu questionário para comando indisponível: %d", got)
	}
}

func TestCommandCLIRevalidatesCallerSessionRoleAndHostSecurity(t *testing.T) {
	for _, scenario := range []string{"caller_revoked", "session_changed", "role_deactivated", "os_locked", "vault_locked"} {
		t.Run(scenario, func(t *testing.T) {
			a := readyCommandProduct(t)
			cli, err := NewCommandCLI(a)
			if err != nil {
				t.Fatalf("NewCommandCLI: %v", err)
			}
			ctx := context.Background()
			principal := a.commandProduct.Load().principal

			switch scenario {
			case "caller_revoked":
				if err := database.DB().Model(&database.Session{}).Where("id = ?", principal.SessionID).Update("revoked_at", time.Now().UTC()).Error; err != nil {
					t.Fatal(err)
				}
			case "session_changed":
				var user database.User
				if err := database.DB().First(&user, "id = ?", principal.UserID).Error; err != nil {
					t.Fatal(err)
				}
				pair, err := a.sessionSvc.IssueSession(ctx, &user, "command-cli-test-session-change")
				if err != nil {
					t.Fatal(err)
				}
				a.setCurrentAuthUser(&AuthUser{UserID: user.ID, SessionID: pair.SessionID, Role: user.Role})
			case "role_deactivated":
				if err := database.DB().Model(&database.User{}).Where("id = ?", principal.UserID).Update("role", "disabled").Error; err != nil {
					t.Fatal(err)
				}
			case "os_locked":
				if err := a.commandHost.SetOSSessionState(ctx, true, true); err != nil {
					t.Fatal(err)
				}
			case "vault_locked":
				if err := a.commandHost.SetVaultUnlocked(ctx, false); err != nil {
					t.Fatal(err)
				}
			}

			if _, err := cli.List(ctx, "en"); err == nil {
				t.Fatal("List aceitou caller inseguro")
			}
			if _, err := cli.Describe(ctx, commandProductWorkspaceListID, "en"); err == nil {
				t.Fatal("Describe aceitou caller inseguro")
			}
			statusID := uuid.Must(uuid.NewV7()).String()
			status, err := cli.Status(ctx, statusID)
			if err == nil {
				t.Fatalf("Status aceitou caller inseguro: %+v", status)
			}

			result, err := cli.Execute(ctx, commandcli.Request{
				CommandID: commandProductWorkspaceListID,
				Arguments: json.RawMessage(`{}`),
			})
			if err == nil {
				t.Fatalf("caller inseguro não foi recusado: %+v", result)
			}
			if result.Status == string(commandledger.Succeeded) {
				t.Fatalf("caller inseguro executou: %+v err=%v", result, err)
			}
			if result.RequestID == "" {
				t.Fatalf("recusa sem request ID: %+v err=%v", result, err)
			}
		})
	}
}

func TestCommandCLIStatusDoesNotExposePaletteInvocation(t *testing.T) {
	a := readyCommandProduct(t)
	cli, err := NewCommandCLI(a)
	if err != nil {
		t.Fatalf("NewCommandCLI: %v", err)
	}

	palette, err := a.ExecutePaletteCommand(commandProductWorkspaceListID, json.RawMessage(`{}`))
	if err != nil || palette.Status != string(commandledger.Succeeded) {
		t.Fatalf("execução real da palette: %+v err=%v", palette, err)
	}
	status, err := cli.Status(context.Background(), palette.InvocationID)
	if err == nil {
		t.Fatalf("Status CLI expôs invocação da palette: %+v", status)
	}
	if !errors.Is(err, commandexecution.ErrDenied) {
		t.Fatalf("Status CLI deveria retornar ErrDenied para invocação da palette: %v", err)
	}
	if status.RequestID != palette.InvocationID {
		t.Fatalf("Status CLI perdeu o ID consultado: %+v", status)
	}
	if status.Status != "" {
		t.Fatalf("Status CLI vazou status da palette: %+v", status)
	}
	if status.ResultSummary != nil {
		t.Fatalf("Status CLI vazou result_summary da palette: %+v", status)
	}
	if status.ErrorCode != nil {
		t.Fatalf("Status CLI vazou error_code da palette: %+v", status)
	}
	if len(status.Output) != 0 {
		t.Fatalf("Status CLI não deve devolver output vivo da palette: %s", status.Output)
	}
}

func TestCommandCLIExecuteGeneratesUUIDOnErrorAndRejectsFirstRequestID(t *testing.T) {
	a := readyCommandProduct(t)
	cli, err := NewCommandCLI(a)
	if err != nil {
		t.Fatalf("NewCommandCLI: %v", err)
	}

	requested := uuid.Must(uuid.NewV7()).String()
	result, err := cli.Execute(context.Background(), commandcli.Request{
		CommandID: commandProductWorkspaceListID,
		Arguments: json.RawMessage(`{}`),
		RequestID: requested,
	})
	if !errors.Is(err, commandcli.ErrRequestIDOnExecute) {
		t.Fatalf("request ID na primeira tentativa deveria ser recusado: result=%+v err=%v", result, err)
	}
	if result.RequestID != "" {
		t.Fatalf("recusa estrutural não deveria fabricar resultado parcial: %+v", result)
	}

	result, err = cli.Execute(context.Background(), commandcli.Request{CommandID: "command.unknown", Arguments: json.RawMessage(`{}`)})
	if err == nil {
		t.Fatalf("comando desconhecido não foi recusado: %+v", result)
	}
	if result.Status != string(commandledger.Denied) {
		t.Fatalf("comando desconhecido sem status denied: %+v err=%v", result, err)
	}
	requireUUIDv7(t, result.RequestID)
}

func TestCommandCLIRetryUnknownRequestIsRejected(t *testing.T) {
	a := readyCommandProduct(t)
	cli, err := NewCommandCLI(a)
	if err != nil {
		t.Fatalf("NewCommandCLI: %v", err)
	}
	unknown := uuid.Must(uuid.NewV7()).String()
	result, err := cli.Retry(context.Background(), commandcli.Request{
		CommandID: commandProductWorkspaceListID,
		Arguments: json.RawMessage(`{}`),
		RequestID: unknown,
	})
	if err == nil && result.Status == string(commandledger.Succeeded) {
		t.Fatalf("retry de request desconhecido foi aceito: %+v", result)
	}
	if result.RequestID != unknown {
		t.Fatalf("retry perdeu o request ID consultado: %+v", result)
	}
}

func TestCommandCLIInitGlobalHotkeysWithoutGUIIsSafe(t *testing.T) {
	a := &App{}
	a.initGlobalHotkeys()
	if a.globalHotkeyOwnership != nil {
		t.Fatal("initGlobalHotkeys sem GUI inicializou ownership nativo")
	}
	if got := a.GetGlobalCommandOwnership(); len(got.Combinations) != 0 {
		t.Fatalf("ownership inesperado sem GUI: %+v", got)
	}
}

func TestCommandCLIMountWithLiveContextWithoutGUIDoesNotStartDeck(t *testing.T) {
	a, _ := appLifecycleProductMountFixture(t)
	runCtx, cancel := context.WithCancel(context.Background())
	a.ctx = runCtx
	a.cancel = cancel
	// A fixture já publicou um snapshot controlado do SO. Impedir o monitor
	// nativo evita que este teste de montagem CLI transforme o cancel vivo em
	// observação real de hotkeys/lock da máquina do agente.
	a.commandOSStarted = true
	t.Cleanup(cancel)

	if err := a.credMgr.RegisterInstanceSecret("internal-auth:command-request-hmac:v1", base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{42}, 32))); err != nil {
		t.Fatal(err)
	}
	if err := a.ensureCommandLifecycleMountedForCurrentUser(runCtx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ShutdownCommandLifecycle(runCtx, a)
		_ = a.drainCommandExecutors(runCtx)
		_ = a.shutdownCommandBridgeIfConfigured(runCtx)
	})
	if err := a.commandHost.SetOSSessionState(runCtx, true, false); err != nil {
		t.Fatal(err)
	}
	if err := a.rebuildCommandLifecyclePersistedConfiguration(runCtx); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapCommandLifecycle(runCtx, a); err != nil {
		t.Fatal(err)
	}

	p := a.commandProduct.Load()
	if p == nil {
		t.Fatal("produto de comandos não montado")
	}
	p.mu.Lock()
	deckCancel, deckDriver := p.deckCancel, p.deckDriver
	p.mu.Unlock()
	if deckCancel != nil || deckDriver != nil {
		t.Fatalf("montagem sem GUI iniciou Deck: cancel=%v driver=%T", deckCancel != nil, deckDriver)
	}
}

func TestCommandCLIWaitReadyCancelledContextReturnsContextCanceled(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := waitCommandCLIReady(ctx, a, p)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitCommandCLIReady com contexto cancelado = %v", err)
	}
	if !errors.Is(err, commandruntime.ErrNotReady) {
		t.Fatalf("waitCommandCLIReady não preservou ErrNotReady: %v", err)
	}
}

func TestCommandCLIWaitReadyLockedHostTimesOutWithoutPublishing(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	ctx := context.Background()
	if err := p.host.SetVaultUnlocked(ctx, false); err != nil {
		t.Fatal(err)
	}
	before, err := CommandLifecycleSnapshot(a)
	if err != nil {
		t.Fatal(err)
	}
	deadline, cancel := context.WithTimeout(ctx, 20*time.Millisecond)
	defer cancel()
	err = waitCommandCLIReady(deadline, a, p)
	if !errors.Is(err, commandruntime.ErrNotReady) {
		t.Fatalf("host bloqueado deveria retornar ErrNotReady: %v", err)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout do host bloqueado não foi preservado: %v", err)
	}
	after, err := CommandLifecycleSnapshot(a)
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("waitCommandCLIReady publicou/mutou lifecycle: antes=%+v depois=%+v", before, after)
	}
}

func TestCommandCLIWaitReadyReadyImmediatePasses(t *testing.T) {
	a := readyCommandProduct(t)
	p := a.commandProduct.Load()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := waitCommandCLIReady(ctx, a, p); err != nil {
		t.Fatalf("produto pronto não passou imediatamente: %v", err)
	}
}

func TestCommandCLIAppConstructorSelectsOnlyModeAndReloadPreservesProviderRegistry(t *testing.T) {
	cliApp := NewCommandCLIApp()
	if cliApp == nil || !cliApp.commandCLIOnly {
		t.Fatalf("NewCommandCLIApp não marcou commandCLIOnly: %#v", cliApp)
	}
	desktopApp := NewApp()
	if desktopApp == nil || desktopApp.commandCLIOnly {
		t.Fatalf("NewApp deveria manter modo desktop: %#v", desktopApp)
	}

	probe := &llm.ProviderConfig{
		ID:      "command-cli-reload-probe",
		Name:    "Command CLI reload probe",
		Type:    llm.ProviderOpenAI,
		BaseURL: "https://example.invalid/v1",
	}
	if err := cliApp.llmRegistry.Register(probe); err != nil {
		t.Fatalf("registrar provider de prova: %v", err)
	}
	_ = cliApp.reloadUserScopedRuntime()
	if cliApp.llmRegistry.Get(probe.ID) != probe {
		t.Fatal("reloadUserScopedRuntime do modo CLI limpou o registry LLM")
	}
}

func requireUUIDv7(t *testing.T, value string) {
	t.Helper()
	id, err := uuid.Parse(value)
	if err != nil || id.Version() != 7 || id.Variant() != uuid.RFC4122 || id.String() != value {
		t.Fatalf("request ID não é UUIDv7 canônico: %q err=%v", value, err)
	}
}

func mustJSONMap(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("serializar projeção CLI: %v", err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("decodificar projeção CLI: %v", err)
	}
	return decoded
}

func jsonBool(value map[string]any, key string) (bool, bool) {
	got, ok := value[key].(bool)
	return got, ok
}

func jsonString(value map[string]any, key string) (string, bool) {
	got, ok := value[key].(string)
	return got, ok
}
