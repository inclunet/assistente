package plugins

import (
	"assistente/internal/database"
	"assistente/internal/pluginpackage"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"gorm.io/gorm"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
)

type countedShell struct {
	calls      atomic.Int32
	onDispatch func() (tools.ToolResult, error)
}

func (*countedShell) Name() string                { return "run_command" }
func (*countedShell) Description() string         { return "Contagem de despachos" }
func (*countedShell) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (s *countedShell) Execute(context.Context, json.RawMessage) (tools.ToolResult, error) {
	s.calls.Add(1)
	if s.onDispatch != nil {
		return s.onDispatch()
	}
	return tools.ToolResult{Content: "confirmação recusada", Failure: &tools.ToolFailure{Code: "shell_command_refused", Kind: tools.ErrorKindAuthorization}}, nil
}

// A variante Windows roda exclusivamente no windows-installer com PowerShell 5.1 real.
func TestInstalledHookDispatch(t *testing.T) {
	if runtime.GOOS == "windows" && os.Getenv("ASSISTENTE_CI_WINDOWS_HOOKS") != "1" {
		t.Skip("PowerShell 5.1 real no windows-installer")
	}
	tmp := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", tmp)
	t.Setenv("LOCALAPPDATA", tmp)
	db := managedJobTestDB(t)
	inventory := NewInventory(db)
	ctx := database.WithUserID(t.Context(), "owner")
	admit := func(_ *gorm.DB, commit func() error) error { return commit() }
	dir := t.TempDir()
	ext := ".sh"
	script := "read -r event || :\ncase \"$event\" in *status2*) exit 2 ;; *deny*) echo '{\"permission\":\"deny\"}' ;; *allow*) echo '{\"permission\":\"allow\"}' ;; *) exit 9 ;; esac\n"
	if runtime.GOOS == "windows" {
		ext = ".ps1"
		script = `$event = $input | ConvertFrom-Json
if ($event.tool_name -ne 'Shell') {exit 9}
if ($event.tool_input.command -eq 'status2') {exit 2}
if ($event.tool_input.command -eq 'deny') { '{"permission":"deny"}' } else { '{"permission":"allow"}' }`
	}
	write := func(path, value string) {
		t.Helper()
		target := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(".cursor-plugin/plugin.json", `{"name":"example","version":"1"}`)
	write("hooks/hooks.json", fmt.Sprintf(`{"version":1,"hooks":{"preToolUse":[{"matcher":"Shell","command":"./hooks/check%s","failClosed":true}]}}`, ext))
	write("hooks/check"+ext, script)
	snap, err := pluginpackage.InspectLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	row, err := inventory.InstallLocal(ctx, snap.Origin, snap.Digest, "")
	if err != nil {
		t.Fatal(err)
	}
	registry := tools.NewRegistry()
	shell := new(countedShell)
	registry.MustRegister(shell)
	cfg := tools.DefaultExecutorConfig()
	cfg.BeforeTool = inventory.BeforeTool
	executor := tools.NewExecutor(registry, cfg)
	check := func(ctx context.Context, w, command, want string) {
		t.Helper()
		before := shell.calls.Load()
		r := executor.ExecuteOne(invocationctx.WithVerifiedWorkspaceID(ctx, w), tools.ToolCall{ID: "call", Function: tools.FunctionCall{Name: "run_command", Arguments: fmt.Sprintf(`{"command":%q}`, command)}})
		calls := int32(1)
		if want == "plugin_hook_blocked" {
			calls = 0
		}
		if r.ErrorCode != want || r.Retryable || shell.calls.Load()-before != calls {
			t.Fatalf("workspace %q cmd %q: %+v chamadas %d", w, command, r, shell.calls.Load()-before)
		}
	}
	decide := func(choice string) {
		t.Helper()
		if err := inventory.DecideToolHook(ctx, row.ID, row.Digest, cursorPreToolKey, "0", "w", choice, admit); err != nil {
			t.Fatal(err)
		}
	}
	check(ctx, "w", "deny", "shell_command_refused")
	decide("workspace")
	var logs bytes.Buffer
	prior := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prior) })
	write("hooks/check"+ext, "invalid") // O despacho usa a cópia instalada.
	check(ctx, "w", "deny", "plugin_hook_blocked")
	if msg := logs.String(); !strings.Contains(msg, row.ID) || !strings.Contains(msg, "reason=veto") || !strings.Contains(msg, cursorPreToolKey) {
		t.Fatalf("origem do veto: %q", msg)
	}
	logs.Reset()
	check(ctx, "w", "status2", "plugin_hook_blocked")
	if !strings.Contains(logs.String(), "reason=veto") {
		t.Fatalf("exit 2: %s", logs.String())
	}
	check(invocationctx.WithShellCommandMetadata(ctx, "protegido"), "w", "allow", "plugin_hook_blocked")
	check(ctx, "w", "allow", "shell_command_refused")
	check(ctx, "other", "deny", "shell_command_refused")
	check(ctx, "", "deny", "shell_command_refused")
	check(database.WithUserID(t.Context(), "other"), "w", "deny", "shell_command_refused")
	decide("global")
	decide("denyWorkspace")
	check(ctx, "w", "deny", "shell_command_refused")
	check(ctx, "future", "deny", "plugin_hook_blocked")
	if err := inventory.ChangeInstallation(ctx, row.ID, row.Digest, "disable", admit); err != nil {
		t.Fatal(err)
	}
	check(ctx, "future", "deny", "shell_command_refused")
	if err := inventory.DecideToolHook(ctx, row.ID, row.Digest, cursorPreToolKey, "0", "w", "global", admit); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("aceite obsoleto: %v", err)
	}
}

func TestInstalledPostToolHook(t *testing.T) {
	if runtime.GOOS == "windows" && os.Getenv("ASSISTENTE_CI_WINDOWS_HOOKS") != "1" {
		t.Skip("PowerShell real apenas no CI")
	}
	dir := t.TempDir()
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	t.Setenv("LOCALAPPDATA", cache)
	ext := ".sh"
	script := `read -r event || :
case "$event" in *postToolUse*result-observed*) ;; *) exit 8 ;; esac
case "$event" in *wait-expiry*) exec sleep 30 ;; *bad-response*) echo '{"permission":"deny"}';; *) echo '{}' ;; esac`
	if runtime.GOOS == "windows" {
		ext = ".ps1"
		script = `$evt=$input | ConvertFrom-Json
if ($evt.hook_event_name -ne 'postToolUse' -or ($evt.tool_output | ConvertFrom-Json).content -ne 'result-observed') {exit 8}
if ($evt.tool_input.command -eq 'wait-expiry') {Start-Sleep -Seconds 30}
if ($evt.tool_input.command -eq 'bad-response') { '{"permission":"deny"}' } else { '{}' }`
	}
	for name, data := range map[string]string{".cursor-plugin/plugin.json": `{"name":"observer","version":"1"}`, "hooks/check" + ext: script, "hooks/hooks.json": fmt.Sprintf(`{"version":1,"hooks":{"postToolUse":[{"matcher":"Shell","command":"./hooks/check%s"}]}}`, ext)} {
		target := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx := invocationctx.WithVerifiedWorkspaceID(database.WithUserID(t.Context(), "owner"), "w")
	inv := NewInventory(managedJobTestDB(t))
	snap, err := pluginpackage.InspectLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	row, err := inv.InstallLocal(ctx, snap.Origin, snap.Digest, "")
	if err != nil {
		t.Fatal(err)
	}
	call := func(command string) tools.ToolCall {
		return tools.ToolCall{ID: "id", Function: tools.FunctionCall{Name: "run_command", Arguments: fmt.Sprintf(`{"command":%q}`, command)}}
	}
	admit := func(_ *gorm.DB, commit func() error) error { return commit() }
	var revoke bool
	exitCode := 0
	shell := &countedShell{onDispatch: func() (tools.ToolResult, error) {
		if revoke {
			if err := inv.ChangeInstallation(ctx, row.ID, row.Digest, "disable", admit); err != nil {
				return tools.ToolResult{}, err
			}
		}
		return tools.ToolResult{Content: "result-observed", Metadata: map[string]any{"exitCode": exitCode}}, nil
	}}
	reg := tools.NewRegistry()
	reg.MustRegister(shell)
	cfg := tools.DefaultExecutorConfig()
	cfg.BeforeTool, cfg.AfterTool = inv.BeforeTool, inv.AfterTool
	executor := tools.NewExecutor(reg, cfg)
	verify := func(origin context.Context, command string, warn bool) {
		t.Helper()
		before := shell.calls.Load()
		result := executor.ExecuteOne(origin, call(command))
		if shell.calls.Load() != before+1 || result.PostToolHookFailed != warn || result.Result.Content != "result-observed" || result.Error != nil || result.Retryable || result.Result.IsError {
			t.Fatalf("%s: %+v, despachos %d", command, result, shell.calls.Load()-before)
		}
	}
	verify(ctx, "bad-response", false)
	if err := inv.DecideToolHook(ctx, row.ID, row.Digest, cursorPostToolKey, "0", "w", "workspace", admit); err != nil {
		t.Fatal(err)
	}
	verify(ctx, "ok", false)
	verify(ctx, "bad-response", true)
	exitCode = 7
	verify(ctx, "bad-response", false)
	exitCode = 0
	verify(invocationctx.WithVerifiedWorkspaceID(ctx, "other"), "bad-response", false)
	verify(database.WithUserID(ctx, "another-owner"), "bad-response", false)
	// A regra de workspace roda antes da regra global. A segunda deve observar a
	// mesma execução tanto após uma resposta inválida quanto após um prazo vencido.
	receipt := filepath.Join(cache, "observed.txt")
	t.Setenv("HOOK_TEST_RECEIPT", receipt)
	record := `echo observed >> "$HOOK_TEST_RECEIPT"`
	if runtime.GOOS == "windows" {
		record = `[IO.File]::AppendAllText($env:HOOK_TEST_RECEIPT, "observed")`
	}
	if err := os.WriteFile(filepath.Join(dir, "hooks", "check"+ext), []byte(record), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".cursor-plugin", "plugin.json"), []byte(`{"name":"second","version":"1"}`), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := pluginpackage.InspectLocal(dir)
	if err != nil {
		t.Fatal(err)
	}
	other, err := inv.InstallLocal(ctx, second.Origin, second.Digest, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.DecideToolHook(ctx, other.ID, other.Digest, cursorPostToolKey, "0", "w", "global", admit); err != nil {
		t.Fatal(err)
	}
	for n, command := range []string{"bad-response", "wait-expiry"} {
		verify(ctx, command, true)
		data, err := os.ReadFile(receipt)
		if err != nil || strings.Count(string(data), "observed") != n+1 {
			t.Fatalf("segundo observador após %s: %q, %v", command, data, err)
		}
	}
	revoke = true
	verify(ctx, "bad-response", false)
}
