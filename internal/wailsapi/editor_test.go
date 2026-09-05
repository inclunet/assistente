package wailsapi

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"assistente/internal/apidto"
	"assistente/internal/configdir"
	"assistente/internal/core/ports"
	"assistente/internal/database"
	"assistente/internal/docextract"
)

const editorTestUserID = "01991f7c-1000-7000-8000-000000000001"

func writeEditorTestDOCX(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for name, content := range map[string]string{
		"[Content_Types].xml": `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"></Types>`,
		"word/document.xml":   `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:body></w:document>`,
	} {
		entry, createErr := zw.Create(name)
		if createErr != nil {
			t.Fatal(createErr)
		}
		if _, writeErr := entry.Write([]byte(content)); writeErr != nil {
			t.Fatal(writeErr)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

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
	AttachEditor(api, stubSession{ctx: database.WithUserID(context.Background(), editorTestUserID)}, EditorHooks{
		AppContext:    func() context.Context { return context.Background() },
		Dialog:        func() ports.SystemDialogPort { return nil },
		MarkSelfWrite: func(path string) func(bool) { return func(bool) {} },
		WatchFile:     func(path string) error { return nil },
		UnwatchFile:   func(path string) error { return nil },
	})
	return api
}

func attachEditorForUser(userID string) *Editor {
	api := NewEditor()
	AttachEditor(api, stubSession{ctx: database.WithUserID(context.Background(), userID)}, EditorHooks{
		AppContext:    func() context.Context { return context.Background() },
		Dialog:        func() ports.SystemDialogPort { return nil },
		MarkSelfWrite: func(path string) func(bool) { return func(bool) {} },
		WatchFile:     func(path string) error { return nil },
		UnwatchFile:   func(path string) error { return nil },
	})
	return api
}

func editorTestPaths(t *testing.T) editorUserPaths {
	t.Helper()
	paths, err := editorPathsForUser(editorTestUserID)
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

func TestEditorRejectsContextWithoutUserID(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)

	api := NewEditor()
	AttachEditor(api, stubSession{ctx: context.Background()}, EditorHooks{
		AppContext:    func() context.Context { return context.Background() },
		Dialog:        func() ports.SystemDialogPort { return &fakeDialog{} },
		MarkSelfWrite: func(path string) func(bool) { return func(bool) {} },
		WatchFile:     func(path string) error { return nil },
		UnwatchFile:   func(path string) error { return nil },
	})

	if _, err := api.EditorGetDraftPath("draft"); !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("draft sem userID: %v", err)
	}
	if _, err := api.EditorReadFile(filepath.Join(tempDir, "doc.md")); !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("arquivo sem userID: %v", err)
	}
	if err := api.EditorWatchFile(filepath.Join(tempDir, "doc.md")); !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("watch sem userID: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tempDir, ".assistente", "users")); !os.IsNotExist(err) {
		t.Fatalf("operação sem userID criou storage: %v", err)
	}
}

func TestEditorIsolatesDraftsAndStateBetweenUsers(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)

	userA := "01991f7c-1000-7000-8000-00000000000a"
	userB := "01991f7c-1000-7000-8000-00000000000b"
	apiA := attachEditorForUser(userA)
	apiB := attachEditorForUser(userB)

	if err := apiA.EditorWriteDraft("mesmo-id", "segredo A"); err != nil {
		t.Fatal(err)
	}
	if err := apiB.EditorWriteDraft("mesmo-id", "segredo B"); err != nil {
		t.Fatal(err)
	}
	if err := apiA.EditorSaveState(apidto.EditorState{FileModeByPath: map[string]string{"a.md": "rich"}}); err != nil {
		t.Fatal(err)
	}
	if err := apiB.EditorSaveState(apidto.EditorState{FileModeByPath: map[string]string{"b.md": "markdown"}}); err != nil {
		t.Fatal(err)
	}

	gotA, err := apiA.EditorReadDraft("mesmo-id")
	if err != nil || gotA != "segredo A" {
		t.Fatalf("draft A = %q, %v", gotA, err)
	}
	gotB, err := apiB.EditorReadDraft("mesmo-id")
	if err != nil || gotB != "segredo B" {
		t.Fatalf("draft B = %q, %v", gotB, err)
	}
	stateA, err := apiA.EditorLoadState()
	if err != nil || stateA.FileModeByPath["a.md"] != "rich" || stateA.FileModeByPath["b.md"] != "" {
		t.Fatalf("state A inesperado: %+v, %v", stateA, err)
	}
	stateB, err := apiB.EditorLoadState()
	if err != nil || stateB.FileModeByPath["b.md"] != "markdown" || stateB.FileModeByPath["a.md"] != "" {
		t.Fatalf("state B inesperado: %+v, %v", stateB, err)
	}

	pathA, _ := apiA.EditorGetDraftPath("mesmo-id")
	pathB, _ := apiB.EditorGetDraftPath("mesmo-id")
	if pathA == pathB || !strings.Contains(pathA, filepath.Join("users", userA)) || !strings.Contains(pathB, filepath.Join("users", userB)) {
		t.Fatalf("paths não isolados: A=%q B=%q", pathA, pathB)
	}
	if _, err := apiB.EditorReadFile(pathA); !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("usuário B abriu draft A como arquivo comum: %v", err)
	}
	if err := apiB.EditorWriteFile(pathA, "sobrescrito"); !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("usuário B escreveu draft A como arquivo comum: %v", err)
	}
	if err := apiB.EditorWatchFile(pathA); !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("usuário B observou draft A como arquivo comum: %v", err)
	}
}

func TestEditorLegacyMigrationBelongsOnlyToFirstEligibleUser(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)

	legacyDir := legacyEditorDir()
	if err := os.MkdirAll(filepath.Join(legacyDir, "drafts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "drafts", "legado.md"), []byte("conteúdo legado"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "state.json"), []byte(`{"fileModeByPath":{"legado.md":"rich"}}`), 0600); err != nil {
		t.Fatal(err)
	}

	userA := "01991f7c-1000-7000-8000-00000000001a"
	userB := "01991f7c-1000-7000-8000-00000000001b"
	apiA := attachEditorForUser(userA)
	apiB := attachEditorForUser(userB)

	got, err := apiA.EditorReadDraft("legado")
	if err != nil || got != "conteúdo legado" {
		t.Fatalf("primeiro usuário não adotou draft: %q, %v", got, err)
	}
	state, err := apiA.EditorLoadState()
	if err != nil || state.FileModeByPath["legado.md"] != "rich" {
		t.Fatalf("primeiro usuário não adotou state: %+v, %v", state, err)
	}
	if _, err := apiB.EditorReadDraft("legado"); err == nil {
		t.Fatal("segundo usuário herdou draft legado")
	}
	if _, err := apiB.EditorReadFile(filepath.Join(legacyDir, "drafts", "legado.md")); !errors.Is(err, database.ErrUserScopeRequired) {
		t.Fatalf("segundo usuário abriu legado como arquivo comum: %v", err)
	}
	stateB, err := apiB.EditorLoadState()
	if err != nil || len(stateB.FileModeByPath) != 0 {
		t.Fatalf("segundo usuário herdou state: %+v, %v", stateB, err)
	}
	if data, err := os.ReadFile(filepath.Join(legacyDir, "drafts", "legado.md")); err != nil || string(data) != "conteúdo legado" {
		t.Fatalf("migração apagou/alterou legado: %q, %v", data, err)
	}

	var claim editorMigrationClaim
	data, err := os.ReadFile(editorMigrationClaimPath())
	if err != nil || json.Unmarshal(data, &claim) != nil {
		t.Fatalf("claim inválido: %s, %v", data, err)
	}
	if claim.UserID != userA {
		t.Fatalf("claim inesperado: %+v", claim)
	}
	pathsA, err := editorPathsForUser(userA)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(pathsA.root, ".legacy-migration-v1-complete")); err != nil {
		t.Fatalf("marcador de conclusão ausente: %v", err)
	}
}

func TestEditorLegacyMigrationIsIdempotentAndConcurrent(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)

	if err := os.MkdirAll(filepath.Join(legacyEditorDir(), "drafts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyEditorDir(), "drafts", "race.md"), []byte("legado"), 0600); err != nil {
		t.Fatal(err)
	}

	userA := "01991f7c-1000-7000-8000-00000000002a"
	userB := "01991f7c-1000-7000-8000-00000000002b"
	apis := []*Editor{attachEditorForUser(userA), attachEditorForUser(userB)}
	errs := make(chan error, 2)
	for _, api := range apis {
		go func(api *Editor) {
			_, err := api.EditorLoadState()
			errs <- err
		}(api)
	}
	for range apis {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}

	owners := 0
	var ownerAPI *Editor
	for _, api := range apis {
		if got, err := api.EditorReadDraft("race"); err == nil && got == "legado" {
			owners++
			ownerAPI = api
		}
	}
	if owners != 1 {
		t.Fatalf("draft legado teve %d donos, quer 1", owners)
	}
	if err := ownerAPI.EditorWriteDraft("race", "novo"); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerAPI.EditorLoadState(); err != nil {
		t.Fatal(err)
	}
	got, err := ownerAPI.EditorReadDraft("race")
	if err != nil || got != "novo" {
		t.Fatalf("rerun sobrescreveu dado novo: %q, %v", got, err)
	}
}

func TestEditorLegacyMigrationRecoversIncompleteCopy(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)

	userID := "01991f7c-1000-7000-8000-00000000003a"
	paths, err := editorPathsForUser(userID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(legacyEditorDir(), "drafts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyEditorDir(), "drafts", "crash.md"), []byte("conteúdo completo"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := writeJSONPrivate(editorMigrationClaimPath(), editorMigrationClaim{
		Version:   editorMigrationVersion,
		UserID:    userID,
		ClaimedAt: time.Now().UnixMilli(),
	}, true); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(paths.draftDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(paths.draftDir, "crash.md"), []byte("parcial"), 0600); err != nil {
		t.Fatal(err)
	}

	api := attachEditorForUser(userID)
	got, err := api.EditorReadDraft("crash")
	if err != nil || got != "conteúdo completo" {
		t.Fatalf("retomada não reparou cópia parcial: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(paths.root, ".legacy-migration-v1-complete")); err != nil {
		t.Fatalf("retomada não concluiu migração: %v", err)
	}
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
	statePath := editorTestPaths(t).state
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

	statePath := editorTestPaths(t).state
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

func TestEditorReadFileProjectsDocumentForReadOnlyView(t *testing.T) {
	api := setupEditorAPITest(t)
	path := filepath.Join(t.TempDir(), "manual.docx")
	writeEditorTestDOCX(t, path, "Texto para leitura")

	result, err := api.EditorReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Projected || !result.ReadOnly || result.Format != "docx" {
		t.Fatalf("resultado documental inesperado: %+v", result)
	}
	if !strings.Contains(result.Content, "Texto para leitura") {
		t.Fatalf("projeção não contém texto: %q", result.Content)
	}
	if strings.Contains(result.Content, "projeção Markdown") || strings.Contains(result.Content, "Origem:") {
		t.Fatalf("metadados vazaram para o conteúdo: %q", result.Content)
	}
}

// Reabrir um documento grande não pode custar a leitura inteira de novo: o
// corpo do arquivo é corrompido preservando magic, tamanho e mtime, então uma
// reextração falharia e só o cache devolve a projeção original.
func TestEditorReadFileReusesProjectionWithoutRereadingDocument(t *testing.T) {
	api := setupEditorAPITest(t)
	path := filepath.Join(t.TempDir(), "manual.docx")
	writeEditorTestDOCX(t, path, "Texto para leitura")

	first, err := api.EditorReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	corrupted := append([]byte(nil), original...)
	for i := 4; i < len(corrupted); i++ {
		corrupted[i] = 0
	}
	if err := os.WriteFile(path, corrupted, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	second, err := api.EditorReadFile(path)
	if err != nil {
		t.Fatalf("releitura falhou em vez de usar o cache: %v", err)
	}
	if second.Content != first.Content {
		t.Fatalf("conteúdo divergiu do cache: %q", second.Content)
	}
}

func TestEditorWarningCodeDistinguishesPDFWithoutText(t *testing.T) {
	if got := editorWarningCode(&docextract.Result{
		Kind:     docextract.KindPDF,
		Warnings: []string{"sem texto"},
	}); got != "no_extractable_text" {
		t.Fatalf("code=%q", got)
	}
	if got := editorWarningCode(&docextract.Result{
		Kind:     docextract.KindEPUB,
		Markdown: "# Parcial",
		Warnings: []string{"item omitido"},
	}); got != "partial_extraction" {
		t.Fatalf("code=%q", got)
	}
}

func TestEditorWriteFileRejectsDocument(t *testing.T) {
	api := setupEditorAPITest(t)
	path := filepath.Join(t.TempDir(), "manual.docx")
	writeEditorTestDOCX(t, path, "Original")

	if err := api.EditorWriteFile(path, "# substituição"); err == nil {
		t.Fatal("EditorWriteFile deveria rejeitar documento opaco")
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
	AttachEditor(api, stubSession{ctx: database.WithUserID(context.Background(), editorTestUserID)}, EditorHooks{
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
	if !strings.Contains(fake.lastOpen.Filters[0].Pattern, "*.docx") ||
		!strings.Contains(fake.lastOpen.Filters[0].Pattern, "*.pdf") {
		t.Fatalf("filtro de abertura não inclui documentos: %q", fake.lastOpen.Filters[0].Pattern)
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
	if fake.lastOpen.Filters[0].DisplayName != "Documentos e texto" {
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
	if strings.Contains(fake.lastSave.Filters[0].Pattern, "*.docx") {
		t.Fatalf("filtro de salvamento não deve oferecer documentos: %q", fake.lastSave.Filters[0].Pattern)
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
	AttachEditor(api, stubSession{ctx: database.WithUserID(context.Background(), editorTestUserID)}, EditorHooks{
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
