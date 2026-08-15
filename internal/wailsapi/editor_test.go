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
	if _, err := api.EditorOpenFile(); !errors.Is(err, ErrEditorNotWired) {
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
	if _, err := api.EditorSaveFileDialog("x"); !errors.Is(err, ErrEditorNotWired) {
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
			_, err := api.EditorOpenFile()
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
			_, err := api.EditorSaveFileDialog("x")
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
