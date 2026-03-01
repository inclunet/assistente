package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/tools"
)

func TestCopyFileTool_EnforcesSkillWriteOnDestination(t *testing.T) {
	workDir := t.TempDir()

	srcDir := filepath.Join(workDir, "allowed")
	dstDir := filepath.Join(workDir, "dst")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		t.Fatalf("mkdir dst: %v", err)
	}

	src := filepath.Join(srcDir, "a.txt")
	if err := os.WriteFile(src, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	// Skill permite ler de allowed/** mas NÃO permite escrever em dst/**
	ec := tools.ExecutionContext{
		InvokedSkillSlug: "test-skill",
		Filesystem: &tools.FilesystemScope{
			Read: []string{filepath.Join(srcDir, "**")},
			// Write vazio => deve negar qualquer escrita
			Write: nil,
			Deny:  nil,
		},
	}
	ctx := tools.WithExecutionContext(context.Background(), ec)

	tool := NewCopyFile(workDir)
	args, _ := json.Marshal(map[string]any{"from": "allowed/a.txt", "to": "dst/b.txt"})
	res, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("execute returned err: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected copy to be denied (no write scope), got success: %s", res.Content)
	}
}

func TestDeleteFileTool_EnforcesSkillWrite(t *testing.T) {
	workDir := t.TempDir()
	allowedDir := filepath.Join(workDir, "allowed")
	if err := os.MkdirAll(allowedDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	p := filepath.Join(allowedDir, "a.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	ec := tools.ExecutionContext{
		InvokedSkillSlug: "test-skill",
		Filesystem: &tools.FilesystemScope{Read: []string{filepath.Join(allowedDir, "**")}, Write: nil},
	}
	ctx := tools.WithExecutionContext(context.Background(), ec)

	tool := NewDeleteFile(workDir)
	args, _ := json.Marshal(map[string]any{"path": "allowed/a.txt"})
	res, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("execute returned err: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected delete to be denied (no write scope)")
	}
}

func TestMakeDirectoryTool_EnforcesSkillWrite(t *testing.T) {
	workDir := t.TempDir()

	ec := tools.ExecutionContext{InvokedSkillSlug: "test-skill", Filesystem: &tools.FilesystemScope{Write: nil}}
	ctx := tools.WithExecutionContext(context.Background(), ec)

	tool := NewMakeDirectory(workDir)
	args, _ := json.Marshal(map[string]any{"path": "newdir"})
	res, err := tool.Execute(ctx, args)
	if err != nil {
		t.Fatalf("execute returned err: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected mkdir to be denied (no write scope)")
	}
}
