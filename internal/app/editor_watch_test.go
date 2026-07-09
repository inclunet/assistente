package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"assistente/internal/configdir"
)

// A marcação NÃO é consumida no primeiro match: no Windows uma única gravação
// pode gerar múltiplos eventos Write, e todos devem ser atribuídos à mesma
// escrita enquanto size+mtime continuarem batendo (até o TTL expirar).
func TestEditorAssistedWriteMarkerSurvivesRepeatedEventsWhileFileMatches(t *testing.T) {
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

	for i := 0; i < 3; i++ {
		origin, ok := app.resolveEditorSelfWrite(norm)
		if !ok {
			t.Fatalf("event %d: expected marker to stay alive while file matches", i+1)
		}
		if origin != "assistant_tool" {
			t.Fatalf("event %d: origin = %q, want assistant_tool", i+1, origin)
		}
	}

	// Uma mudança externa real invalida a marcação para os próximos eventos.
	if err := os.WriteFile(filePath, []byte("conteudo externo diferente"), 0644); err != nil {
		t.Fatalf("overwrite file: %v", err)
	}
	if origin, ok := app.resolveEditorSelfWrite(norm); ok {
		t.Fatalf("marker should be invalidated after external change, got origin %q", origin)
	}
}

func TestEditorAssistedWriteMarkerDoesNotRequireActiveWatchAtWriteStart(t *testing.T) {
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

	commit := app.markEditorAssistedWrite(filePath)
	if commit == nil {
		t.Fatal("expected commit function")
	}
	if err := os.WriteFile(filePath, []byte("depois"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	commit(true)

	app.editorWatchMu.Lock()
	app.editorDirWatches = map[string]*editorDirWatch{
		normDir: {
			files:    map[string]int{norm: 1},
			lastEmit: map[string]time.Time{},
		},
	}
	app.editorWatchMu.Unlock()

	origin, ok := app.resolveEditorSelfWrite(norm)
	if !ok {
		t.Fatal("expected assisted write marker registered before watch")
	}
	if origin != "assistant_tool" {
		t.Fatalf("origin = %q, want assistant_tool", origin)
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

	if origin, ok := app.resolveEditorSelfWrite(norm); ok {
		t.Fatalf("cancelled marker should not be consumed, got origin %q", origin)
	}
}

func TestEditorAssistedWriteMarkerCommitDoesNotRecreateClearedMarker(t *testing.T) {
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
	app.clearEditorAssistedWrite(norm)
	if err := os.WriteFile(filePath, []byte("tool content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	commit(true)

	if origin, ok := app.resolveEditorSelfWrite(norm); ok {
		t.Fatalf("cleared marker should not be recreated, got origin %q", origin)
	}
}

func TestEditorAssistedWriteMarkerCommitDoesNotStampNewerGeneration(t *testing.T) {
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

	oldCommit := app.markEditorAssistedWrite(filePath)
	if oldCommit == nil {
		t.Fatal("expected old commit function")
	}
	newCommit := app.markEditorAssistedWrite(filePath)
	if newCommit == nil {
		t.Fatal("expected new commit function")
	}
	if err := os.WriteFile(filePath, []byte("tool content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	oldCommit(true)

	app.editorWatchMu.Lock()
	write := app.editorAssistedWriteByPath[norm]
	app.editorWatchMu.Unlock()
	if write.committed {
		t.Fatal("old commit should not stamp newer marker generation")
	}

	newCommit(true)
	origin, ok := app.resolveEditorSelfWrite(norm)
	if !ok {
		t.Fatal("expected assisted marker from newer generation")
	}
	if origin != "assistant_tool" {
		t.Fatalf("origin = %q, want assistant_tool", origin)
	}
}

func TestEditorAssistedWriteMarkerCancelDoesNotDeleteNewerGeneration(t *testing.T) {
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

	oldCommit := app.markEditorAssistedWrite(filePath)
	if oldCommit == nil {
		t.Fatal("expected old commit function")
	}
	newCommit := app.markEditorAssistedWrite(filePath)
	if newCommit == nil {
		t.Fatal("expected new commit function")
	}
	oldCommit(false)

	app.editorWatchMu.Lock()
	_, stillPresent := app.editorAssistedWriteByPath[norm]
	app.editorWatchMu.Unlock()
	if !stillPresent {
		t.Fatal("old cancel should not delete newer marker generation")
	}

	if err := os.WriteFile(filePath, []byte("tool content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	newCommit(true)
	if origin, ok := app.resolveEditorSelfWrite(norm); !ok || origin != "assistant_tool" {
		t.Fatalf("expected newer assisted marker, got origin=%q ok=%v", origin, ok)
	}
}

func TestEditorAssistedWriteMarkerStatErrorDoesNotDeleteNewerGeneration(t *testing.T) {
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

	oldCommit := app.markEditorAssistedWrite(filePath)
	if oldCommit == nil {
		t.Fatal("expected old commit function")
	}
	newCommit := app.markEditorAssistedWrite(filePath)
	if newCommit == nil {
		t.Fatal("expected new commit function")
	}
	oldCommit(true)

	app.editorWatchMu.Lock()
	_, stillPresent := app.editorAssistedWriteByPath[norm]
	app.editorWatchMu.Unlock()
	if !stillPresent {
		t.Fatal("old stat error should not delete newer marker generation")
	}

	if err := os.WriteFile(filePath, []byte("tool content"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	newCommit(true)
	if origin, ok := app.resolveEditorSelfWrite(norm); !ok || origin != "assistant_tool" {
		t.Fatalf("expected newer assisted marker, got origin=%q ok=%v", origin, ok)
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

	if origin, ok := app.resolveEditorSelfWrite(norm); ok {
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

	origin, ok := app.resolveEditorSelfWrite(norm)
	<-done
	if !ok {
		t.Fatal("expected assisted marker after delayed commit")
	}
	if origin != "assistant_tool" {
		t.Fatalf("origin = %q, want assistant_tool", origin)
	}
}

func TestEditorAssistedWriteMarkerUncommittedReturnsWithoutConsuming(t *testing.T) {
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

	start := time.Now()
	if origin, ok := app.resolveEditorSelfWrite(norm); ok {
		t.Fatalf("uncommitted marker should not be consumed, got origin %q", origin)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("consume waited too long: %s", elapsed)
	}

	app.editorWatchMu.Lock()
	_, stillPresent := app.editorAssistedWriteByPath[norm]
	app.editorWatchMu.Unlock()
	if !stillPresent {
		t.Fatal("uncommitted marker should remain for a later event")
	}

	commit(false)
}

func TestEditorWriteFileMarksSelfWriteWithEditorUIOrigin(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	app := &App{}

	norm, err := normalizeWatchPath(filePath)
	if err != nil {
		t.Fatalf("normalizeWatchPath: %v", err)
	}

	if err := app.EditorWriteFile(filePath, "conteudo salvo pelo editor"); err != nil {
		t.Fatalf("EditorWriteFile: %v", err)
	}

	origin, ok := app.resolveEditorSelfWrite(norm)
	if !ok {
		t.Fatal("expected self-write marker after EditorWriteFile")
	}
	if origin != "editor_ui" {
		t.Fatalf("origin = %q, want editor_ui", origin)
	}

	// Eventos duplicados da mesma gravação continuam resolvendo como self-write.
	if origin, ok := app.resolveEditorSelfWrite(norm); !ok || origin != "editor_ui" {
		t.Fatalf("expected marker alive for duplicated event, got origin=%q ok=%v", origin, ok)
	}
}

func TestEditorWriteFileMarkerInvalidatedByExternalChange(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	app := &App{}

	norm, err := normalizeWatchPath(filePath)
	if err != nil {
		t.Fatalf("normalizeWatchPath: %v", err)
	}

	if err := app.EditorWriteFile(filePath, "conteudo salvo pelo editor"); err != nil {
		t.Fatalf("EditorWriteFile: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("conteudo escrito por outro programa"), 0644); err != nil {
		t.Fatalf("overwrite file: %v", err)
	}

	if origin, ok := app.resolveEditorSelfWrite(norm); ok {
		t.Fatalf("external change should not resolve as self-write, got origin %q", origin)
	}
}

func TestExternalWriteWithoutMarkerIsNotSelfWrite(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "doc.md")
	app := &App{}

	norm, err := normalizeWatchPath(filePath)
	if err != nil {
		t.Fatalf("normalizeWatchPath: %v", err)
	}

	if err := os.WriteFile(filePath, []byte("escrita externa"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	if origin, ok := app.resolveEditorSelfWrite(norm); ok {
		t.Fatalf("external write should have no marker, got origin %q", origin)
	}
}

func TestEditorWriteDraftMarksSelfWriteWithEditorUIOrigin(t *testing.T) {
	tempHome := t.TempDir()
	t.Setenv("HOME", tempHome)
	t.Setenv("USERPROFILE", tempHome)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)

	app := &App{}
	if err := app.EditorWriteDraft("draft-teste", "conteudo do draft"); err != nil {
		t.Fatalf("EditorWriteDraft: %v", err)
	}

	p, err := draftPath("draft-teste")
	if err != nil {
		t.Fatalf("draftPath: %v", err)
	}
	norm, err := normalizeWatchPath(p)
	if err != nil {
		t.Fatalf("normalizeWatchPath: %v", err)
	}

	origin, ok := app.resolveEditorSelfWrite(norm)
	if !ok {
		t.Fatal("expected self-write marker after EditorWriteDraft")
	}
	if origin != "editor_ui" {
		t.Fatalf("origin = %q, want editor_ui", origin)
	}
}
