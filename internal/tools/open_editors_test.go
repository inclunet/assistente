package tools

import (
	"context"
	"runtime"
	"testing"
)

func TestWithOpenEditorPaths_RoundTrip(t *testing.T) {
	paths := []string{"/home/user/doc.txt", "/tmp/notes.md"}
	ctx := WithOpenEditorPaths(context.Background(), paths)

	got := GetOpenEditorPaths(ctx)
	if len(got) != 2 {
		t.Fatalf("expected 2 paths, got %d", len(got))
	}
	if got[0] != paths[0] || got[1] != paths[1] {
		t.Errorf("paths mismatch: got %v", got)
	}
}

func TestGetOpenEditorPaths_EmptyContext(t *testing.T) {
	got := GetOpenEditorPaths(context.Background())
	if got != nil {
		t.Errorf("expected nil for empty context, got %v", got)
	}
}

func TestIsOpenEditorFile_Match(t *testing.T) {
	paths := []string{"/home/user/doc.txt", "/tmp/notes.md"}
	ctx := WithOpenEditorPaths(context.Background(), paths)

	if !IsOpenEditorFile(ctx, "/home/user/doc.txt") {
		t.Error("expected match for /home/user/doc.txt")
	}
	if !IsOpenEditorFile(ctx, "/tmp/notes.md") {
		t.Error("expected match for /tmp/notes.md")
	}
	if IsOpenEditorFile(ctx, "/other/file.txt") {
		t.Error("should not match /other/file.txt")
	}
}

func TestIsOpenEditorFile_EmptyContext(t *testing.T) {
	if IsOpenEditorFile(context.Background(), "/any/file.txt") {
		t.Error("should not match with empty context")
	}
}

func TestIsOpenEditorFile_CaseInsensitiveWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("case insensitive test only on Windows")
	}
	paths := []string{`C:\Users\user\Doc.txt`}
	ctx := WithOpenEditorPaths(context.Background(), paths)

	if !IsOpenEditorFile(ctx, `C:\Users\user\doc.txt`) {
		t.Error("expected case-insensitive match on Windows")
	}
	if !IsOpenEditorFile(ctx, `C:\USERS\USER\DOC.TXT`) {
		t.Error("expected case-insensitive match on Windows (all caps)")
	}
}
