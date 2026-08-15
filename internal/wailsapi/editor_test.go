package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/configdir"
	"assistente/internal/core/ports"
)

func TestEditorNotWired(t *testing.T) {
	t.Parallel()
	api := NewEditor()
	if _, err := api.EditorGetDraftPath("x"); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorGetDraftPath: got %v", err)
	}
	if err := api.EditorWriteDraft("x", ""); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorWriteDraft: got %v", err)
	}
	if _, err := api.EditorReadDraft("x"); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorReadDraft: got %v", err)
	}
	if err := api.EditorDeleteDraft("x"); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorDeleteDraft: got %v", err)
	}
	if _, err := api.EditorLoadState(); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorLoadState: got %v", err)
	}
	if err := api.EditorSaveState(apidto.EditorState{}); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorSaveState: got %v", err)
	}
	if _, err := api.EditorOpenFile(apidto.FileDialogLabels{}); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorOpenFile: got %v", err)
	}
	if _, err := api.EditorReadFile("x"); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorReadFile: got %v", err)
	}
	if _, err := api.EditorGetFileInfo("x"); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorGetFileInfo: got %v", err)
	}
	if err := api.EditorWriteFile("x", ""); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorWriteFile: got %v", err)
	}
	if _, err := api.EditorRenameFile("a", "b"); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorRenameFile: got %v", err)
	}
	if _, err := api.EditorSaveFileDialog("x", apidto.FileDialogLabels{}); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorSaveFileDialog: got %v", err)
	}
	if err := api.EditorWatchFile("x"); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorWatchFile: got %v", err)
	}
	if err := api.EditorUnwatchFile("x"); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("EditorUnwatchFile: got %v", err)
	}
}

func TestEditorAttachNilHooksStillNotWired(t *testing.T) {
	t.Parallel()
	api := NewEditor()
	AttachEditor(api, stubSession{}, EditorHooks{})
	if _, err := api.EditorLoadState(); !errors.Is(err, ErrEditorNotWired) {
		t.Fatalf("hooks vazios: got %v", err)
	}
}

func TestEditorUsesWithUserNotRequireAuth(t *testing.T) {
	t.Parallel()
	semAuth := errors.New("sessão não autenticada")
	api := NewEditor()
	AttachEditor(api, stubSession{err: semAuth}, EditorHooks{
		AppContext:    func() context.Context { return nil },
		Dialog:        func() ports.SystemDialogPort { return nil },
		MarkSelfWrite: func(path string) func(bool) { return nil },
		WatchFile:     func(path string) error { return nil },
		UnwatchFile:   func(path string) error { return nil },
	})

	casos := []struct {
		nome string
		fn   func() error
	}{
		{"EditorGetDraftPath", func() error {
			_, err := api.EditorGetDraftPath("x")
			return err
		}},
		{"EditorWriteDraft", func() error {
			return api.EditorWriteDraft("x", "")
		}},
		{"EditorReadDraft", func() error {
			_, err := api.EditorReadDraft("x")
			return err
		}},
		{"EditorDeleteDraft", func() error {
			return api.EditorDeleteDraft("x")
		}},
		{"EditorLoadState", func() error {
			_, err := api.EditorLoadState()
			return err
		}},
		{"EditorSaveState", func() error {
			return api.EditorSaveState(apidto.EditorState{})
		}},
		{"EditorOpenFile", func() error {
			_, err := api.EditorOpenFile(apidto.FileDialogLabels{})
			return err
		}},
		{"EditorReadFile", func() error {
			_, err := api.EditorReadFile("x")
			return err
		}},
		{"EditorGetFileInfo", func() error {
			_, err := api.EditorGetFileInfo("x")
			return err
		}},
		{"EditorWriteFile", func() error {
			return api.EditorWriteFile("x", "")
		}},
		{"EditorRenameFile", func() error {
			_, err := api.EditorRenameFile("a", "b")
			return err
		}},
		{"EditorSaveFileDialog", func() error {
			_, err := api.EditorSaveFileDialog("x", apidto.FileDialogLabels{})
			return err
		}},
		{"EditorWatchFile", func() error {
			return api.EditorWatchFile("x")
		}},
		{"EditorUnwatchFile", func() error {
			return api.EditorUnwatchFile("x")
		}},
	}
	for _, c := range casos {
		c := c
		t.Run(c.nome, func(t *testing.T) {
			t.Parallel()
			if err := c.fn(); !errors.Is(err, semAuth) {
				t.Fatalf("erro = %v, quer o da sessão", err)
			}
		})
	}
}

func TestEditorUsesWithUserNotRequireAuthSource(t *testing.T) {
	t.Parallel()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	src, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "editor.go"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(src)
	if strings.Contains(body, "requireAuthenticatedContext(") {
		t.Fatal("editor.go não deve chamar requireAuthenticatedContext(; use WithUser")
	}
	if !strings.Contains(body, "WithUser(session,") {
		t.Fatal("editor.go deve chamar WithUser(session,")
	}
}

func setupEditorAPITest(t *testing.T) *Editor {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)

	api := NewEditor()
	AttachEditor(api, stubSession{ctx: context.Background()}, EditorHooks{
		AppContext:    func() context.Context { return context.Background() },
		Dialog:        func() ports.SystemDialogPort { return nil },
		MarkSelfWrite: func(path string) func(bool) { return func(bool) {} },
		WatchFile:     func(path string) error { return nil },
		UnwatchFile:   func(path string) error { return nil },
	})
	return api
}

func TestEditorPrivateFilesUse0600(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissões POSIX não se aplicam no Windows")
	}
	api := setupEditorAPITest(t)

	if err := api.EditorWriteDraft("draft-privado", "conteudo sensivel"); err != nil {
		t.Fatalf("EditorWriteDraft: %v", err)
	}
	draftPath, err := api.EditorGetDraftPath("draft-privado")
	if err != nil {
		t.Fatalf("EditorGetDraftPath: %v", err)
	}
	info, err := os.Stat(draftPath)
	if err != nil {
		t.Fatalf("stat draft: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("draft perm = %04o, quer 0600", got)
	}
	draftDirInfo, err := os.Stat(filepath.Dir(draftPath))
	if err != nil {
		t.Fatalf("stat draft dir: %v", err)
	}
	if got := draftDirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("draft dir perm = %04o, quer 0700", got)
	}

	if err := api.EditorSaveState(apidto.EditorState{FileModeByPath: map[string]string{}}); err != nil {
		t.Fatalf("EditorSaveState: %v", err)
	}
	statePath := editorStatePath()
	stateInfo, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("stat state: %v", err)
	}
	if got := stateInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("state.json perm = %04o, quer 0600", got)
	}
	stateDirInfo, err := os.Stat(filepath.Dir(statePath))
	if err != nil {
		t.Fatalf("stat editor dir: %v", err)
	}
	if got := stateDirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("editor dir perm = %04o, quer 0700", got)
	}
}

func TestEditorPrivateFilesTightenLegacyModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissões POSIX não se aplicam no Windows")
	}
	api := setupEditorAPITest(t)

	draftPath, err := api.EditorGetDraftPath("legado")
	if err != nil {
		t.Fatalf("EditorGetDraftPath: %v", err)
	}
	draftDir := filepath.Dir(draftPath)
	editorDir := filepath.Dir(draftDir)
	if err := os.MkdirAll(draftDir, 0755); err != nil {
		t.Fatalf("mkdir legado: %v", err)
	}
	if err := os.Chmod(editorDir, 0755); err != nil {
		t.Fatalf("chmod editor legado: %v", err)
	}
	if err := os.Chmod(draftDir, 0755); err != nil {
		t.Fatalf("chmod drafts legado: %v", err)
	}
	if err := os.WriteFile(draftPath, []byte("antigo"), 0644); err != nil {
		t.Fatalf("seed draft: %v", err)
	}
	if err := api.EditorWriteDraft("legado", "novo"); err != nil {
		t.Fatalf("EditorWriteDraft: %v", err)
	}
	info, err := os.Stat(draftPath)
	if err != nil {
		t.Fatalf("stat draft: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("draft legado perm = %04o, quer 0600 após rewrite", got)
	}
	dirInfo, err := os.Stat(draftDir)
	if err != nil {
		t.Fatalf("stat draft dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("draft dir legado perm = %04o, quer 0700 após rewrite", got)
	}
	editorInfo, err := os.Stat(editorDir)
	if err != nil {
		t.Fatalf("stat editor dir: %v", err)
	}
	if got := editorInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("editor dir legado perm = %04o, quer 0700 após rewrite de draft", got)
	}

	statePath := editorStatePath()
	if err := os.MkdirAll(filepath.Dir(statePath), 0755); err != nil {
		t.Fatalf("mkdir state legado: %v", err)
	}
	if err := os.WriteFile(statePath, []byte(`{}`), 0644); err != nil {
		t.Fatalf("seed state: %v", err)
	}
	if err := api.EditorSaveState(apidto.EditorState{}); err != nil {
		t.Fatalf("EditorSaveState: %v", err)
	}
	stateInfo, err := os.Stat(statePath)
	if err != nil {
		t.Fatalf("stat state: %v", err)
	}
	if got := stateInfo.Mode().Perm(); got != 0600 {
		t.Fatalf("state legado perm = %04o, quer 0600 após rewrite", got)
	}
	stateDirInfo, err := os.Stat(filepath.Dir(statePath))
	if err != nil {
		t.Fatalf("stat editor dir: %v", err)
	}
	if got := stateDirInfo.Mode().Perm(); got != 0700 {
		t.Fatalf("editor dir legado perm = %04o, quer 0700 após rewrite", got)
	}
}

func TestEditorWriteFilePreservesExistingMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permissões POSIX não se aplicam no Windows")
	}
	api := setupEditorAPITest(t)

	path := filepath.Join(t.TempDir(), "secreto.md")
	if err := os.WriteFile(path, []byte("antes"), 0600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := api.EditorWriteFile(path, "depois"); err != nil {
		t.Fatalf("EditorWriteFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0600 {
		t.Fatalf("perm = %04o, quer preservar 0600", got)
	}
}

// fakeDialog captura as opções passadas ao SystemDialogPort nos testes.
type fakeDialog struct {
	lastOpen ports.OpenFileOptions
	lastSave ports.SaveFileOptions
}

func (f *fakeDialog) OpenFileDialog(opts ports.OpenFileOptions) (string, error) {
	f.lastOpen = opts
	return "", nil
}

func (f *fakeDialog) SaveFileDialog(opts ports.SaveFileOptions) (string, error) {
	f.lastSave = opts
	return "", nil
}

func setupEditorAPIWithDialog(t *testing.T, dialog ports.SystemDialogPort) *Editor {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)

	api := NewEditor()
	AttachEditor(api, stubSession{ctx: context.Background()}, EditorHooks{
		AppContext:    func() context.Context { return context.Background() },
		Dialog:        func() ports.SystemDialogPort { return dialog },
		MarkSelfWrite: func(path string) func(bool) { return func(bool) {} },
		WatchFile:     func(path string) error { return nil },
		UnwatchFile:   func(path string) error { return nil },
	})
	return api
}

func TestEditorOpenFilePassesFrontendLabels(t *testing.T) {
	fake := &fakeDialog{}
	api := setupEditorAPIWithDialog(t, fake)

	labels := apidto.FileDialogLabels{
		Title:          "Open file",
		MarkdownFilter: "Markdown files",
		AllFilesFilter: "All files",
	}
	if _, err := api.EditorOpenFile(labels); err != nil {
		t.Fatalf("EditorOpenFile: %v", err)
	}
	if fake.lastOpen.Title != "Open file" {
		t.Fatalf("Title = %q, quer Open file", fake.lastOpen.Title)
	}
	if len(fake.lastOpen.Filters) != 2 {
		t.Fatalf("filters = %d, quer 2", len(fake.lastOpen.Filters))
	}
	if fake.lastOpen.Filters[0].DisplayName != "Markdown files" {
		t.Fatalf("markdown filter = %q", fake.lastOpen.Filters[0].DisplayName)
	}
	if fake.lastOpen.Filters[1].DisplayName != "All files" {
		t.Fatalf("all filter = %q", fake.lastOpen.Filters[1].DisplayName)
	}
}

func TestEditorOpenFileAppliesFallbackWhenLabelsEmpty(t *testing.T) {
	fake := &fakeDialog{}
	api := setupEditorAPIWithDialog(t, fake)

	if _, err := api.EditorOpenFile(apidto.FileDialogLabels{}); err != nil {
		t.Fatalf("EditorOpenFile: %v", err)
	}
	if fake.lastOpen.Title != "Abrir arquivo" {
		t.Fatalf("Title = %q, quer fallback pt-BR", fake.lastOpen.Title)
	}
	if len(fake.lastOpen.Filters) != 2 {
		t.Fatalf("filters = %d, quer 2", len(fake.lastOpen.Filters))
	}
	if fake.lastOpen.Filters[0].DisplayName != "Markdown" {
		t.Fatalf("markdown filter = %q", fake.lastOpen.Filters[0].DisplayName)
	}
	if fake.lastOpen.Filters[1].DisplayName != "Todos os arquivos" {
		t.Fatalf("all filter = %q", fake.lastOpen.Filters[1].DisplayName)
	}
}

func TestEditorSaveFileDialogPassesFrontendLabels(t *testing.T) {
	fake := &fakeDialog{}
	api := setupEditorAPIWithDialog(t, fake)

	labels := apidto.FileDialogLabels{
		Title:           "Save file",
		MarkdownFilter:  "Markdown files",
		AllFilesFilter:  "All files",
		DefaultFilename: "notes.md",
	}
	if _, err := api.EditorSaveFileDialog("", labels); err != nil {
		t.Fatalf("EditorSaveFileDialog: %v", err)
	}
	if fake.lastSave.Title != "Save file" {
		t.Fatalf("Title = %q, quer Save file", fake.lastSave.Title)
	}
	if fake.lastSave.DefaultFilename != "notes.md" {
		t.Fatalf("DefaultFilename = %q, quer notes.md", fake.lastSave.DefaultFilename)
	}
	if len(fake.lastSave.Filters) != 2 {
		t.Fatalf("filters = %d, quer 2", len(fake.lastSave.Filters))
	}
	if fake.lastSave.Filters[0].DisplayName != "Markdown files" {
		t.Fatalf("markdown filter = %q", fake.lastSave.Filters[0].DisplayName)
	}
	if fake.lastSave.Filters[1].DisplayName != "All files" {
		t.Fatalf("all filter = %q", fake.lastSave.Filters[1].DisplayName)
	}
}

func TestEditorSaveFileDialogSuggestedFilenameTakesPrecedence(t *testing.T) {
	fake := &fakeDialog{}
	api := setupEditorAPIWithDialog(t, fake)

	labels := apidto.FileDialogLabels{DefaultFilename: "from-labels.md"}
	if _, err := api.EditorSaveFileDialog("suggested.md", labels); err != nil {
		t.Fatalf("EditorSaveFileDialog: %v", err)
	}
	if fake.lastSave.DefaultFilename != "suggested.md" {
		t.Fatalf("DefaultFilename = %q, quer suggested.md", fake.lastSave.DefaultFilename)
	}
}

func TestEditorSaveFileDialogAppliesFallbackWhenLabelsEmpty(t *testing.T) {
	fake := &fakeDialog{}
	api := setupEditorAPIWithDialog(t, fake)

	if _, err := api.EditorSaveFileDialog("", apidto.FileDialogLabels{}); err != nil {
		t.Fatalf("EditorSaveFileDialog: %v", err)
	}
	if fake.lastSave.Title != "Salvar arquivo" {
		t.Fatalf("Title = %q, quer fallback pt-BR", fake.lastSave.Title)
	}
	if fake.lastSave.DefaultFilename != "documento.md" {
		t.Fatalf("DefaultFilename = %q, quer documento.md", fake.lastSave.DefaultFilename)
	}
	if len(fake.lastSave.Filters) != 2 {
		t.Fatalf("filters = %d, quer 2", len(fake.lastSave.Filters))
	}
	if fake.lastSave.Filters[0].DisplayName != "Markdown" {
		t.Fatalf("markdown filter = %q", fake.lastSave.Filters[0].DisplayName)
	}
	if fake.lastSave.Filters[1].DisplayName != "Todos os arquivos" {
		t.Fatalf("all filter = %q", fake.lastSave.Filters[1].DisplayName)
	}
}

func TestEditorWatchNormalizaPathNaBorda(t *testing.T) {
	var observados []string
	api := NewEditor()
	AttachEditor(api, stubSession{ctx: context.Background()}, EditorHooks{
		AppContext:    func() context.Context { return context.Background() },
		Dialog:        func() ports.SystemDialogPort { return nil },
		MarkSelfWrite: func(path string) func(bool) { return func(bool) {} },
		WatchFile: func(path string) error {
			observados = append(observados, path)
			return nil
		},
		UnwatchFile: func(path string) error {
			observados = append(observados, path)
			return nil
		},
	})

	if err := api.EditorWatchFile("  C:/tmp/doc.md  "); err != nil {
		t.Fatalf("EditorWatchFile: %v", err)
	}
	if err := api.EditorUnwatchFile("  C:/tmp/doc.md  "); err != nil {
		t.Fatalf("EditorUnwatchFile: %v", err)
	}
	for _, got := range observados {
		if got != "C:/tmp/doc.md" {
			t.Fatalf("hook recebeu %q, quer path sem espaços", got)
		}
	}

	if err := api.EditorWatchFile("   "); err == nil {
		t.Fatal("EditorWatchFile com path vazio deveria falhar na borda")
	}
	if err := api.EditorUnwatchFile("   "); err == nil {
		t.Fatal("EditorUnwatchFile com path vazio deveria falhar na borda")
	}
	if len(observados) != 2 {
		t.Fatalf("hooks chamados %d vezes, quer 2 (path vazio não chega ao watcher)", len(observados))
	}
}
