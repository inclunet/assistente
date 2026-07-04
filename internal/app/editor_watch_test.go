package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestEditorAssistedWriteMarkerConsumedOnce(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	app := &App{}

	norm, err := normalizeWatchPath(filePath)
	if err != nil {
		t.Fatalf("normalizeWatchPath: %v", err)
	}
	normDir, err := normalizeWatchPath(filepath.Dir(filePath))
	if err != nil {
		t.Fatalf("normalizeWatchPath dir: %v", err)
	}
	app.editorDirWatches = map[string]*editorDirWatch{
		normDir: {
			files:    map[string]int{norm: 1},
			lastEmit: map[string]time.Time{},
		},
	}
	app.editorAssistedWriteByPath = map[string]editorAssistedWrite{}

	commit := app.markEditorAssistedWrite(filePath)
	if commit == nil {
		t.Fatal("expected commit function")
	}
	if err := os.WriteFile(filePath, []byte("depois"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	commit(true)

	origin, ok := app.consumeEditorAssistedWrite(norm)
	if !ok {
		t.Fatal("expected assisted write marker")
	}
	if origin != "assistant_tool" {
		t.Fatalf("origin = %q, want assistant_tool", origin)
	}

	if origin, ok := app.consumeEditorAssistedWrite(norm); ok {
		t.Fatalf("marker should be consumed once, got origin %q", origin)
	}
}

func TestEditorAssistedWriteMarkerCanBeCancelled(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	app := &App{}

	norm, err := normalizeWatchPath(filePath)
	if err != nil {
		t.Fatalf("normalizeWatchPath: %v", err)
	}
	normDir, err := normalizeWatchPath(filepath.Dir(filePath))
	if err != nil {
		t.Fatalf("normalizeWatchPath dir: %v", err)
	}
	app.editorDirWatches = map[string]*editorDirWatch{
		normDir: {
			files:    map[string]int{norm: 1},
			lastEmit: map[string]time.Time{},
		},
	}
	app.editorAssistedWriteByPath = map[string]editorAssistedWrite{}

	commit := app.markEditorAssistedWrite(filePath)
	if commit == nil {
		t.Fatal("expected commit function")
	}
	commit(false)

	if origin, ok := app.consumeEditorAssistedWrite(norm); ok {
		t.Fatalf("cancelled marker should not be consumed, got origin %q", origin)
	}
}

func TestEditorAssistedWriteMarkerRejectsDifferentFileState(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	app := &App{}

	norm, err := normalizeWatchPath(filePath)
	if err != nil {
		t.Fatalf("normalizeWatchPath: %v", err)
	}
	normDir, err := normalizeWatchPath(filepath.Dir(filePath))
	if err != nil {
		t.Fatalf("normalizeWatchPath dir: %v", err)
	}
	app.editorDirWatches = map[string]*editorDirWatch{
		normDir: {
			files:    map[string]int{norm: 1},
			lastEmit: map[string]time.Time{},
		},
	}
	app.editorAssistedWriteByPath = map[string]editorAssistedWrite{}

	commit := app.markEditorAssistedWrite(filePath)
	if commit == nil {
		t.Fatal("expected commit function")
	}
	if err := os.WriteFile(filePath, []byte("tool content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	commit(true)
	if err := os.WriteFile(filePath, []byte("external content"), 0644); err != nil {
		t.Fatalf("overwrite file: %v", err)
	}

	if origin, ok := app.consumeEditorAssistedWrite(norm); ok {
		t.Fatalf("changed file state should not be assisted, got origin %q", origin)
	}
}

func TestEditorAssistedWriteMarkerWaitsForCommit(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	app := &App{}

	norm, err := normalizeWatchPath(filePath)
	if err != nil {
		t.Fatalf("normalizeWatchPath: %v", err)
	}
	normDir, err := normalizeWatchPath(filepath.Dir(filePath))
	if err != nil {
		t.Fatalf("normalizeWatchPath dir: %v", err)
	}
	app.editorDirWatches = map[string]*editorDirWatch{
		normDir: {
			files:    map[string]int{norm: 1},
			lastEmit: map[string]time.Time{},
		},
	}
	app.editorAssistedWriteByPath = map[string]editorAssistedWrite{}

	commit := app.markEditorAssistedWrite(filePath)
	if commit == nil {
		t.Fatal("expected commit function")
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		time.Sleep(20 * time.Millisecond)
		if err := os.WriteFile(filePath, []byte("tool content"), 0644); err != nil {
			t.Errorf("write file: %v", err)
			return
		}
		commit(true)
	}()

	origin, ok := app.consumeEditorAssistedWrite(norm)
	<-done
	if !ok {
		t.Fatal("expected assisted marker after delayed commit")
	}
	if origin != "assistant_tool" {
		t.Fatalf("origin = %q, want assistant_tool", origin)
	}
}
