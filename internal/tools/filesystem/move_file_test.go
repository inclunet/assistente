package filesystem

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMoveFileTool_RenamesFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	dst := filepath.Join(dir, "b.txt")

	if err := os.WriteFile(src, []byte("hello"), 0644); err != nil {
		t.Fatalf("write src: %v", err)
	}

	tool := NewMoveFile(dir)
	args := map[string]any{"from": "a.txt", "to": "b.txt"}
	b, _ := json.Marshal(args)

	res, err := tool.Execute(context.Background(), b)
	if err != nil {
		t.Fatalf("execute err: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", res.Content)
	}

	if _, err := os.Stat(dst); err != nil {
		t.Fatalf("dst not found: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Fatalf("src should be gone, got: %v", err)
	}
}

func TestMoveFileTool_FailsIfDestExistsByDefault(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	dst := filepath.Join(dir, "b.txt")

	_ = os.WriteFile(src, []byte("hello"), 0644)
	_ = os.WriteFile(dst, []byte("existing"), 0644)

	tool := NewMoveFile(dir)
	args := map[string]any{"from": "a.txt", "to": "b.txt"}
	b, _ := json.Marshal(args)

	res, err := tool.Execute(context.Background(), b)
	if err != nil {
		t.Fatalf("execute err: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected error, got success: %s", res.Content)
	}
}

func TestMoveFileTool_OverwritesWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.txt")
	dst := filepath.Join(dir, "b.txt")

	_ = os.WriteFile(src, []byte("hello"), 0644)
	_ = os.WriteFile(dst, []byte("existing"), 0644)

	tool := NewMoveFile(dir)
	args := map[string]any{"from": "a.txt", "to": "b.txt", "overwrite": true}
	b, _ := json.Marshal(args)

	res, err := tool.Execute(context.Background(), b)
	if err != nil {
		t.Fatalf("execute err: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %s", res.Content)
	}

	data, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected overwritten content, got: %q", string(data))
	}
}

func TestRenameFileSameDir_PreservesExtensionWhenMissing(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "a.md")
	_ = os.WriteFile(src, []byte("hi"), 0644)

	newPath, err := RenameFileSameDir(src, "b")
	if err != nil {
		t.Fatalf("rename err: %v", err)
	}
	if filepath.Base(newPath) != "b.md" {
		t.Fatalf("expected b.md, got %s", filepath.Base(newPath))
	}
}
