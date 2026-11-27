package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"assistente/internal/authorization"
	"assistente/internal/logging"
	"assistente/internal/pluginpackage"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
	"gorm.io/gorm"
)

const cursorPreToolKey = "hooks/hooks.json#preToolUse"

const cursorPostToolKey = "hooks/hooks.json#postToolUse"

var errHookVeto = errors.New("veto do hook")
var errHookOutput = errors.New("resposta do hook incompatível")

var hookPath = regexp.MustCompile(`^[A-Za-z0-9_./-]+$`)

// O primeiro adaptador aceita um script relativo da revisão para a ferramenta Shell.
// Campos adicionais podem alterar o filtro, prazo ou permissões e exigem outro adaptador.
func toolHookScript(snapshot *pluginpackage.LocalSnapshot, key string) (string, bool) {
	if snapshot.Manifest.Format != pluginpackage.Cursor || len(snapshot.Manifest.Unmapped) != 0 {
		return "", false
	}
	for name := range snapshot.Files {
		if name == ".cursor/hooks.json" || name == "hooks.json" || strings.HasPrefix(name, ".cursor/rules/") || strings.HasPrefix(name, "rules/") {
			return "", false
		}
	}
	var config struct {
		Version int                        `json:"version"`
		Hooks   map[string]json.RawMessage `json:"hooks"`
	}
	doc := snapshot.Files["hooks/hooks.json"]
	if len(doc) > 64<<10 || !strictHookJSON(doc, &config) || config.Version != 1 || len(config.Hooks) != 1 {
		return "", false
	}
	if key != cursorPreToolKey && key != cursorPostToolKey {
		return "", false
	}
	event := strings.TrimPrefix(key, "hooks/hooks.json#")
	var handlers []struct {
		Command    string `json:"command"`
		Matcher    string `json:"matcher"`
		FailClosed *bool  `json:"failClosed"`
	}
	if !strictHookJSON(config.Hooks[event], &handlers) || len(handlers) != 1 || handlers[0].Matcher != "Shell" {
		return "", false
	}
	if key == cursorPreToolKey && (handlers[0].FailClosed == nil || !*handlers[0].FailClosed) || key == cursorPostToolKey && handlers[0].FailClosed != nil {
		return "", false
	}
	entry, ok := strings.CutPrefix(handlers[0].Command, "./")
	if !ok || !fs.ValidPath(entry) || !hookPath.MatchString(entry) {
		return "", false
	}
	ext := ".sh"
	if runtime.GOOS == "windows" {
		ext = ".ps1"
	}
	_, exists := snapshot.Files[entry]
	return entry, exists && filepath.Ext(entry) == ext
}

func strictHookJSON(raw []byte, target any) bool {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target) == nil && decoder.Decode(new(any)) == io.EOF
}

// BeforeTool revalida cada decisão depois de carregar a revisão e antes de iniciar o script.
func (i *Inventory) BeforeTool(ctx context.Context, call tools.ToolCall) error {
	return i.runToolHooks(ctx, call, cursorPreToolKey, "", 0)
}
func (i *Inventory) AfterTool(ctx context.Context, call tools.ToolCall, result tools.ToolResult, duration int64) error {
	code, known := result.Metadata["exitCode"].(int)
	if !known || code != 0 || ctx.Err() != nil {
		return nil
	}
	return i.runToolHooks(ctx, call, cursorPostToolKey, result.Content, duration)
}
func (i *Inventory) runToolHooks(ctx context.Context, call tools.ToolCall, key, output string, duration int64) error {
	workspace := invocationctx.VerifiedWorkspaceID(ctx)
	if workspace == "" || call.Function.Name != "run_command" {
		return nil
	}
	rules := authorization.NewRepository(func() *gorm.DB { return i.db })
	rows, err := rules.ListScopes(ctx, authorization.PluginHookDomain, map[string]string{"global": "", "workspace": workspace})
	if err != nil {
		return err
	}
	var failures error
	seen := map[string]bool{}
	for _, row := range rows {
		if row.Effect != "allow" || seen[row.SelectorKey] {
			continue
		}
		seen[row.SelectorKey] = true
		var hook authorization.PluginHook
		if err := json.Unmarshal([]byte(row.Selector), &hook); err != nil {
			return err
		}
		if hook.Component != key {
			continue
		}
		ok, err := rules.PluginHookAllowed(ctx, hook)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := i.runGrantedHook(ctx, call, hook, rules, output, duration); err != nil {
			logging.Warnf(ctx, "plugin_hook", "installation=%s component=%s tool_call_id=%s reason=%s", hook.InstallationID, hook.Component, call.ID, hookFailureReason(err))
			if key == cursorPreToolKey || ctx.Err() != nil {
				return err
			}
			failures = errors.Join(failures, err)
		}
	}
	return failures
}

func hookFailureReason(err error) string {
	switch {
	case errors.Is(err, errHookVeto):
		return "veto"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, errHookOutput):
		return "invalid_output"
	case errors.Is(err, ErrComponentUnavailable):
		return "component_unavailable"
	default:
		return "execution_failed"
	}
}

func (i *Inventory) runGrantedHook(ctx context.Context, call tools.ToolCall, hook authorization.PluginHook, rules *authorization.Repository, output string, duration int64) error {
	if _, protected := invocationctx.LookupShellCommandMetadata(ctx); protected {
		return ErrComponentUnavailable
	}
	snap, err := i.installedSnapshot(ctx, hook.InstallationID, hook.Digest)
	if err != nil {
		return err
	}
	entry, ok := toolHookScript(snap, hook.Component)
	if !ok {
		return ErrComponentUnavailable
	}
	event := map[string]any{"hook_event_name": strings.TrimPrefix(hook.Component, "hooks/hooks.json#"), "tool_name": "Shell", "tool_input": json.RawMessage(call.Function.Arguments), "tool_use_id": call.ID}
	if hook.Component == cursorPostToolKey {
		payload, err := json.Marshal(map[string]string{"content": output})
		if err != nil {
			return err
		}
		event["tool_output"] = string(payload)
		event["duration"] = duration
	}
	input, err := json.Marshal(event)
	if err != nil {
		return err
	}
	return runToolScript(ctx, snap, entry, hook.Component, input, func() (bool, error) { return rules.PluginHookAllowed(ctx, hook) })
}

func runToolScript(ctx context.Context, snap *pluginpackage.LocalSnapshot, entry, key string, input []byte, allowed func() (bool, error)) (err error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	parent := filepath.Join(cache, "assistente", "plugin-hooks")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	dir, err := os.MkdirTemp(parent, "revision-")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, os.RemoveAll(dir)) }()
	root, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	for name, data := range snap.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		local, err := filepath.Localize(name)
		if err != nil {
			return err
		}
		if err := root.MkdirAll(filepath.Dir(local), 0700); err != nil {
			return err
		}
		f, err := root.OpenFile(local, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err != nil {
			return err
		}
		_, writeErr := f.Write(data)
		if err := errors.Join(writeErr, f.Close()); err != nil {
			return err
		}
	}
	timeout, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd, err := preToolCommand(timeout, filepath.Join(dir, filepath.FromSlash(entry)))
	if err != nil {
		return err
	}
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "CURSOR_PLUGIN_ROOT="+dir)
	cmd.Stdin = bytes.NewReader(input)
	var stdout hookOutput
	cmd.Stdout = &stdout
	cmd.WaitDelay = time.Second
	ok, err := allowed()
	if err != nil || !ok {
		return err
	}
	if err := cmd.Run(); err != nil {
		if timeout.Err() != nil {
			return timeout.Err()
		}
		var exit *exec.ExitError
		if key == cursorPreToolKey && errors.As(err, &exit) && exit.ExitCode() == 2 {
			return errHookVeto
		}
		return fmt.Errorf("hook terminou com falha: %w", err)
	}
	if key == cursorPostToolKey {
		raw := bytes.TrimSpace(stdout.buffer.Bytes())
		var response map[string]json.RawMessage
		if stdout.overflow || len(raw) != 0 && (!strictHookJSON(raw, &response) || response == nil || len(response) != 0) {
			return errHookOutput
		}
		return nil
	}
	var decision struct {
		Permission   string `json:"permission"`
		UserMessage  string `json:"user_message"`
		AgentMessage string `json:"agent_message"`
	}
	if stdout.overflow || !strictHookJSON(stdout.buffer.Bytes(), &decision) {
		return errHookOutput
	}
	switch decision.Permission {
	case "allow":
		return nil
	case "deny":
		return errHookVeto
	default:
		return errHookOutput
	}
}

type hookOutput struct {
	buffer   bytes.Buffer
	overflow bool
}

func (o *hookOutput) Write(p []byte) (int, error) {
	n := len(p)
	remaining := 16384 - o.buffer.Len()
	if n > remaining {
		o.overflow = true
		p = p[:remaining]
	}
	_, _ = o.buffer.Write(p)
	return n, nil
}
