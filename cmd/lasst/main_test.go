package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/configdir"
	"assistente/internal/llm"
	"assistente/internal/skills"
)

type testStreamer struct {
	calls int
	t     *testing.T
	mode  string
}

func (s *testStreamer) StreamChat(ctx context.Context, messages []llm.Message, _ llm.ChatParams, h llm.StreamHandler, defs ...llm.ToolDefinition) {
	s.t.Helper()
	s.calls++
	if s.mode == "truncated" {
		h.(llm.FinishReasonSink).OnFinishReason(llm.FinishInfo{Reason: llm.FinishReasonMaxTokens})
		h.OnToolCalls([]llm.ToolCall{{ID: "1", Function: llm.FunctionCall{Name: "read_file", Arguments: `{"path":"f.txt"}`}}}, "", llm.Usage{}, "")
		return
	}
	if s.mode == "forever" {
		h.OnToolCalls([]llm.ToolCall{{ID: "1", Type: "function", Function: llm.FunctionCall{Name: "read_file", Arguments: `{"path":"f.txt"}`}}}, "", llm.Usage{}, "")
		return
	}
	if s.calls == 1 {
		if len(defs) != 1 || defs[0].Function.Name != "read_file" {
			s.t.Fatalf("tools: %#v", defs)
		}
		if messages[0].Content != "Leia com cuidado" {
			s.t.Fatalf("skill: %#v", messages[0])
		}
		h.OnToolCalls([]llm.ToolCall{{ID: "call-1", Type: "function", Function: llm.FunctionCall{Name: "read_file", Arguments: `{"path":"f.txt"}`}}}, "", llm.Usage{}, "")
		return
	}
	last := messages[len(messages)-1]
	if last.Role != "tool" || last.ToolCallID != "call-1" || !strings.Contains(last.Content.(string), "conteúdo único") {
		s.t.Fatalf("resultado: %#v", last)
	}
	h.OnDone("resposta final", llm.Usage{}, "")
}
func TestRunUsesSkillAndReadFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("conteúdo único"), 0600); err != nil {
		t.Fatal(err)
	}
	_, body, err := skills.Parse("---\nname: review\ndescription: review\n---\nLeia com cuidado")
	if err != nil {
		t.Fatal(err)
	}
	s := &testStreamer{t: t}
	got, err := run(context.Background(), s, "modelo", body, "analise", dir)
	if err != nil || got != "resposta final" || s.calls != 2 {
		t.Fatalf("resposta=%q err=%v chamadas=%d", got, err, s.calls)
	}
}
func TestRunRejectsTruncatedToolCalls(t *testing.T) {
	s := &testStreamer{t: t, mode: "truncated"}
	_, err := run(context.Background(), s, "modelo", "skill", "prompt", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "incompleta") || s.calls != 1 {
		t.Fatalf("err=%v chamadas=%d", err, s.calls)
	}
}
func TestRunLimitsIterations(t *testing.T) {
	s := &testStreamer{t: t, mode: "forever"}
	_, err := run(context.Background(), s, "modelo", "skill", "prompt", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "limite") || s.calls != maxIterations {
		t.Fatalf("err=%v chamadas=%d", err, s.calls)
	}
}
func TestRunCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &testStreamer{t: t}
	_, err := run(ctx, s, "modelo", "skill", "prompt", t.TempDir())
	if err != context.Canceled || s.calls != 0 {
		t.Fatalf("err=%v chamadas=%d", err, s.calls)
	}
}

type pathStreamer struct {
	t              *testing.T
	path, expected string
	step           int
}

func (s *pathStreamer) StreamChat(_ context.Context, messages []llm.Message, _ llm.ChatParams, h llm.StreamHandler, _ ...llm.ToolDefinition) {
	s.t.Helper()
	s.step++
	if s.step == 1 {
		args, _ := json.Marshal(map[string]string{"path": s.path})
		h.OnToolCalls([]llm.ToolCall{{ID: "p", Type: "function", Function: llm.FunctionCall{Name: "read_file", Arguments: string(args)}}}, "", llm.Usage{}, "")
		return
	}
	result := messages[len(messages)-1].Content.(string)
	if !strings.Contains(result, s.expected) {
		s.t.Fatalf("resultado não contém %q: %s", s.expected, result)
	}
	h.OnDone("ok", llm.Usage{}, "")
}
func TestReadScopeRejectsOutsideWorkspace(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	t.Setenv("USERPROFILE", outside)
	t.Setenv("HOME", outside)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)
	if err := os.MkdirAll(filepath.Join(outside, ".assistente"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(outside, ".assistente", "secret.json")
	if err := os.WriteFile(path, []byte("segredo"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &pathStreamer{t: t, path: path, expected: "não permitido"}
	if _, err := run(context.Background(), s, "m", "skill", "prompt", root); err != nil {
		t.Fatal(err)
	}
}
func TestReadResultIncludesContinuation(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "long.txt"), []byte(strings.Repeat("linha\n", 2500)), 0600); err != nil {
		t.Fatal(err)
	}
	s := &pathStreamer{t: t, path: "long.txt", expected: "has_more"}
	if _, err := run(context.Background(), s, "m", "skill", "prompt", root); err != nil {
		t.Fatal(err)
	}
}
func TestRejectRestrictedSkill(t *testing.T) {
	t.Setenv("LASST_BASE_URL", "https://example.com")
	t.Setenv("LASST_MODEL", "modelo")
	t.Setenv("LASST_API_KEY", "key")
	path := filepath.Join(t.TempDir(), "SKILL.md")
	if err := os.WriteFile(path, []byte("---\nname: review\ntools:\n  denied: [read_file]\n---\nTeste"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := execute(path, "teste", io.Discard); err == nil || !strings.Contains(err.Error(), "não suportados") {
		t.Fatalf("err=%v", err)
	}
}
func TestReadRejectsGitCredentials(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("token"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &pathStreamer{t: t, path: ".git/config", expected: "denylist"}
	if _, err := run(context.Background(), s, "m", "skill", "prompt", root); err != nil {
		t.Fatal(err)
	}
}
func TestReadRejectsNestedGitCredentials(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sub", ".git")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config"), []byte("credencial-teste"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &pathStreamer{t: t, path: "sub/.git/config", expected: "denylist"}
	if _, err := run(context.Background(), s, "modelo", "skill", "prompt", root); err != nil {
		t.Fatal(err)
	}
}
func TestReadRejectsSymlinkToNestedGit(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "sub", ".git")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	secret := filepath.Join(dir, "config")
	if err := os.WriteFile(secret, []byte("credencial-teste"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, filepath.Join(root, "alias.txt")); err != nil {
		t.Skipf("symlink indisponível: %v", err)
	}
	s := &pathStreamer{t: t, path: "alias.txt", expected: "denylist"}
	if _, err := run(context.Background(), s, "modelo", "skill", "prompt", root); err != nil {
		t.Fatal(err)
	}
}
