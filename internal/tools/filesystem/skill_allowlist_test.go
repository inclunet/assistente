package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/tools"
)

func TestSkillFilesystemAllowlist_ReadFile_AllowsAndDenies(t *testing.T) {
	workDir := t.TempDir()

	allowedDir := filepath.Join(workDir, "allowed")
	secretDir := filepath.Join(allowedDir, "secret")
	notAllowedDir := filepath.Join(workDir, "notallowed")

	if err := os.MkdirAll(secretDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(notAllowedDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	allowedFile := filepath.Join(allowedDir, "a.txt")
	secretFile := filepath.Join(secretDir, "b.txt")
	notAllowedFile := filepath.Join(notAllowedDir, "c.txt")

	if err := os.WriteFile(allowedFile, []byte("ok"), 0o644); err != nil {
		t.Fatalf("write allowed: %v", err)
	}
	if err := os.WriteFile(secretFile, []byte("no"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	if err := os.WriteFile(notAllowedFile, []byte("no"), 0o644); err != nil {
		t.Fatalf("write notallowed: %v", err)
	}

	ec := tools.ExecutionContext{
		InvokedSkillSlug: "test-skill",
		Filesystem: &tools.FilesystemScope{
			Read:  []string{filepath.Join(allowedDir, "**")},
			Write: []string{filepath.Join(allowedDir, "**")},
			Deny:  []string{filepath.Join(secretDir, "**")},
		},
	}
	ctx := tools.WithExecutionContext(context.Background(), ec)

	tool := NewReadFile(workDir)

	// Allowed
	args, _ := json.Marshal(map[string]any{"path": "allowed/a.txt"})
	res, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("execute allowed returned err: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected allowed read, got error: %s", res.Content)
	}

	// Denied by denylist
	args, _ = json.Marshal(map[string]any{"path": "allowed/secret/b.txt"})
	res, err = tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("execute secret returned err: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected secret read to be denied")
	}

	// Denied by allowlist
	args, _ = json.Marshal(map[string]any{"path": "notallowed/c.txt"})
	res, err = tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("execute notallowed returned err: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected notallowed read to be denied")
	}
}

func TestSkillFilesystemAllowlist_ListAndSearch_DoNotLeakDeniedChildren(t *testing.T) {
	workDir := t.TempDir()

	allowedDir := filepath.Join(workDir, "allowed")
	secretDir := filepath.Join(allowedDir, "secret")
	if err := os.MkdirAll(secretDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(allowedDir, "a.txt"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write allowed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(secretDir, "b.txt"), []byte("no"), 0o644); err != nil {
		t.Fatalf("write secret: %v", err)
	}

	ec := tools.ExecutionContext{
		InvokedSkillSlug: "test-skill",
		Filesystem: &tools.FilesystemScope{
			Read:  []string{filepath.Join(allowedDir, "**")},
			Write: []string{filepath.Join(allowedDir, "**")},
			Deny:  []string{filepath.Join(secretDir, "**")},
		},
	}
	ctx := tools.WithExecutionContext(context.Background(), ec)

	// list_directory: não deve listar o diretório/arquivo negado
	listTool := NewListDirectory(workDir)
	args, _ := json.Marshal(map[string]any{"path": "allowed", "recursive": true, "max_depth": 5})
	res, err := listTool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("list execute returned err: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected list ok, got error: %s", res.Content)
	}
	if strings.Contains(res.Content, "secret") {
		t.Fatalf("expected denied child to be omitted from listing")
	}

	// search_files: não deve retornar o arquivo negado
	searchTool := NewSearchFiles(workDir)
	args, _ = json.Marshal(map[string]any{"pattern": "**/*.txt", "path": "allowed", "max_results": 50})
	res, err = searchTool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("search execute returned err: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected search ok, got error: %s", res.Content)
	}
	if strings.Contains(res.Content, "secret") || strings.Contains(res.Content, "b.txt") {
		t.Fatalf("expected denied file to be omitted from search results")
	}
}
