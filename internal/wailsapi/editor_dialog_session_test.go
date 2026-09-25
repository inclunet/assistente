package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/configdir"
	"assistente/internal/core/ports"
	"assistente/internal/database"
)

var errEditorDialogSessionTestStale = errors.New("dialog session stale")

func TestEditorOpenDialogDoesNotPublishContentAfterSessionChangesDuringRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "document.md")
	if err := os.WriteFile(path, []byte("private document"), 0600); err != nil {
		t.Fatal(err)
	}
	validations := 0
	api := editorDialogSessionTestAPI(t, &editorDialogPathPort{openPath: path}, func(context.Context) (func() error, error) {
		return func() error {
			validations++
			if validations > 1 {
				return errEditorDialogSessionTestStale
			}
			return nil
		}, nil
	})
	result, err := api.EditorOpenFile(apidto.FileDialogLabels{})
	if result != nil || !errors.Is(err, errEditorDialogSessionTestStale) || validations != 2 {
		t.Fatalf("late read published: result=%v error=%v validations=%d", result, err, validations)
	}
}

func TestEditorDialogRequiresSessionValidator(t *testing.T) {
	for _, capture := range []func(context.Context) (func() error, error){nil, func(context.Context) (func() error, error) { return nil, nil }} {
		api := editorDialogSessionTestAPI(t, &editorDialogPathPort{}, capture)
		if _, err := api.EditorOpenFile(apidto.FileDialogLabels{}); !errors.Is(err, ErrEditorNotWired) {
			t.Fatalf("open sem prova: %v", err)
		}
		if _, err := api.EditorSaveFileDialog("test.md", apidto.FileDialogLabels{}); !errors.Is(err, ErrEditorNotWired) {
			t.Fatalf("save sem prova: %v", err)
		}
	}
}

type editorDialogPathPort struct {
	openPath string
	savePath string
}

func (p *editorDialogPathPort) OpenFileDialog(ports.OpenFileOptions) (string, error) {
	return p.openPath, nil
}

func (p *editorDialogPathPort) SaveFileDialog(ports.SaveFileOptions) (string, error) {
	return p.savePath, nil
}

func editorDialogSessionTestAPI(t *testing.T, dialog ports.SystemDialogPort, capture func(context.Context) (func() error, error)) *Editor {
	t.Helper()
	tempDir := t.TempDir()
	t.Setenv("HOME", tempDir)
	t.Setenv("USERPROFILE", tempDir)
	configdir.ResetForTests()
	t.Cleanup(configdir.ResetForTests)
	api := NewEditor()
	AttachEditor(api, stubSession{ctx: database.WithUserID(context.Background(), editorTestUserID)}, EditorHooks{
		AppContext:           func() context.Context { return context.Background() },
		Dialog:               func() ports.SystemDialogPort { return dialog },
		CaptureDialogSession: capture,
		MarkSelfWrite:        func(string) func(bool) { return func(bool) {} },
		WatchFile:            func(string) error { return nil },
		UnwatchFile:          func(string) error { return nil },
	})
	return api
}

func TestEditorOpenDialogValidaSessaoDepoisDoDialogAntesDeLer(t *testing.T) {
	var validations int
	api := editorDialogSessionTestAPI(t, &editorDialogPathPort{
		openPath: filepath.Join(t.TempDir(), "nao-deve-ser-lido.md"),
	}, func(context.Context) (func() error, error) {
		return func() error {
			validations++
			return errEditorDialogSessionTestStale
		}, nil
	})

	result, err := api.EditorOpenFile(apidto.FileDialogLabels{})
	if !errors.Is(err, errEditorDialogSessionTestStale) || result != nil {
		t.Fatalf("open stale: result=%#v err=%v", result, err)
	}
	if validations != 1 {
		t.Fatalf("validações=%d, want 1", validations)
	}
}

func TestEditorSaveDialogNaoRetornaPathDepoisDeInvalidarSessao(t *testing.T) {
	api := editorDialogSessionTestAPI(t, &editorDialogPathPort{savePath: "C:/novo.md"}, func(context.Context) (func() error, error) {
		return func() error { return errEditorDialogSessionTestStale }, nil
	})

	path, err := api.EditorSaveFileDialog("novo.md", apidto.FileDialogLabels{})
	if !errors.Is(err, errEditorDialogSessionTestStale) || path != "" {
		t.Fatalf("save stale: path=%q err=%v", path, err)
	}
}

func TestEditorDialogCancelamentoNaoValidaDepois(t *testing.T) {
	var validations int
	dialogErr := errors.New("cancelado")
	api := editorDialogSessionTestAPI(t, dialogPortError{err: dialogErr}, func(context.Context) (func() error, error) {
		return func() error { validations++; return nil }, nil
	})

	if _, err := api.EditorSaveFileDialog("novo.md", apidto.FileDialogLabels{}); !errors.Is(err, dialogErr) {
		t.Fatalf("cancelamento: %v", err)
	}
	if validations != 0 {
		t.Fatalf("validação executada após cancelamento: %d", validations)
	}
}

type dialogPortError struct{ err error }

func (d dialogPortError) OpenFileDialog(ports.OpenFileOptions) (string, error) { return "", d.err }
func (d dialogPortError) SaveFileDialog(ports.SaveFileOptions) (string, error) { return "", d.err }
