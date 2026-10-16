package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/commandexecution"
	"assistente/internal/commandui"
	"assistente/internal/database"
	"assistente/internal/tools/filesystem"
	"assistente/internal/workspace"
	"github.com/google/uuid"
)

type editorFileCommandFlow struct {
	a           *App
	ctx         context.Context
	reservation commandui.Reservation
	handoff     commandui.Handoff
	operation   string
	path        string
	token       string
}

func beginEditorFileCommand(t *testing.T, commandID string, tab workspace.Tab) editorFileCommandFlow {
	t.Helper()
	a := readyCommandProduct(t)
	if err := a.workspaceMgr.AddTab(tab); err != nil {
		t.Fatal(err)
	}
	if err := a.workspaceMgr.SetActiveTab(tab.ID); err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), a.commandProduct.Load().principal.UserID)
	r := beginUICommand(t, a, commandID)
	h := takeUICommandFor(t, a, r.Ticket, commandID)
	return editorFileCommandFlow{a: a, ctx: ctx, reservation: r, handoff: h}
}

func prepareEditorFileCommand(t *testing.T, f *editorFileCommandFlow, content, path string, opened *apidto.EditorOpenResult) {
	t.Helper()
	operation, target, err := f.a.editorCommandTarget(f.ctx, f.reservation.Ticket, f.handoff.HandoffID)
	if err != nil {
		t.Fatal(err)
	}
	if path == "" {
		path = target
	}
	prepareEditorFileCommandAt(t, f, operation, content, path, opened)
}

func prepareEditorFileCommandAt(t *testing.T, f *editorFileCommandFlow, operation, content, path string, opened *apidto.EditorOpenResult) {
	t.Helper()
	version, err := filesystem.CaptureFileVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	token, err := f.a.editorCommandPrepare(f.ctx, apidto.EditorCommandPrepareRequest{
		Ticket: f.reservation.Ticket, HandoffID: f.handoff.HandoffID, Content: content,
	}, operation, path, version, opened)
	if err != nil {
		t.Fatal(err)
	}
	f.operation, f.path, f.token = operation, path, token
}

func commitEditorFileCommand(t *testing.T, f *editorFileCommandFlow, confirm bool, write func(context.Context, string, string, filesystem.FileVersion) error) (*apidto.EditorCommandResult, error) {
	t.Helper()
	return f.a.editorCommandCommit(f.ctx, apidto.EditorCommandCommitRequest{
		Ticket: f.reservation.Ticket, HandoffID: f.handoff.HandoffID, Token: f.token, ConfirmOverwrite: confirm,
	}, write)
}

func TestCommandEditorFileSaveFirstPathBytesAndLedger(t *testing.T) {
	path := filepath.Join(t.TempDir(), "first.md")
	f := beginEditorFileCommand(t, commandEditorFileSaveID, workspace.Tab{ID: uuid.NewString(), Type: workspace.TabTypeEditor, Title: "Sem título"})
	prepareEditorFileCommand(t, &f, "conteúdo novo", path, nil)
	writes := 0
	result, err := commitEditorFileCommand(t, &f, true, func(_ context.Context, gotPath, content string, _ filesystem.FileVersion) error {
		writes++
		return os.WriteFile(gotPath, []byte(content), 0o644)
	})
	if err != nil || result == nil || result.Status != "succeeded" || !result.Written || writes != 1 {
		t.Fatalf("save first: result=%+v writes=%d err=%v", result, writes, err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "conteúdo novo" {
		t.Fatalf("bytes gravados=%q err=%v", got, err)
	}
	active := f.a.workspaceMgr.Active().ActiveTab()
	if active == nil || active.State["filePath"] != path || active.Title != filepath.Base(path) {
		t.Fatalf("metadata do primeiro save: %+v", active)
	}
	if _, err := commitEditorFileCommand(t, &f, true, func(_ context.Context, _, _ string, _ filesystem.FileVersion) error {
		writes++
		return nil
	}); err == nil || writes != 1 {
		t.Fatalf("duplicate save escreveu novamente: writes=%d err=%v", writes, err)
	}
	assertEditorFileLedger(t, f.reservation.InvocationID, "conteúdo novo", path)
}

func TestCommandEditorFileOpenReturnsTransientReadAndPersistsMetadataOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "open.md")
	if err := os.WriteFile(path, []byte("não deve ir para o ledger"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := beginEditorFileCommand(t, commandEditorFileOpenID, workspace.Tab{ID: uuid.NewString(), Type: workspace.TabTypeEditor})
	opened := &apidto.EditorOpenResult{Path: path, Content: "não deve ir para o ledger"}
	prepareEditorFileCommand(t, &f, "", path, opened)
	writes := 0
	result, err := commitEditorFileCommand(t, &f, false, func(context.Context, string, string, filesystem.FileVersion) error {
		writes++
		return nil
	})
	if err != nil || result == nil || result.Status != "succeeded" || result.Written || writes != 0 || result.Opened == nil || result.Opened.Content != opened.Content {
		t.Fatalf("open: result=%+v writes=%d err=%v", result, writes, err)
	}
	active := f.a.workspaceMgr.Active().ActiveTab()
	if active == nil || active.State["filePath"] != path {
		t.Fatalf("metadata open ausente: %+v", active)
	}
	assertEditorFileLedger(t, f.reservation.InvocationID, opened.Content, path)
}

func TestCommandEditorFileCommitRejectsWrongReceiptStaleWorkspaceAndAuth(t *testing.T) {
	t.Run("wrong receipt", func(t *testing.T) {
		f := beginEditorFileCommand(t, commandEditorFileSaveID, workspace.Tab{ID: uuid.NewString(), Type: workspace.TabTypeEditor})
		prepareEditorFileCommand(t, &f, "bytes", filepath.Join(t.TempDir(), "wrong.md"), nil)
		writes := 0
		result, err := f.a.editorCommandCommit(f.ctx, apidto.EditorCommandCommitRequest{Ticket: f.reservation.Ticket, HandoffID: f.handoff.HandoffID, Token: "wrong-token", ConfirmOverwrite: true}, func(context.Context, string, string, filesystem.FileVersion) error { writes++; return nil })
		if err == nil || result != nil || writes != 0 {
			t.Fatalf("receipt forjado aceito: result=%+v writes=%d err=%v", result, writes, err)
		}
	})
	t.Run("stale workspace", func(t *testing.T) {
		f := beginEditorFileCommand(t, commandEditorFileSaveID, workspace.Tab{ID: uuid.NewString(), Type: workspace.TabTypeEditor})
		prepareEditorFileCommand(t, &f, "bytes", filepath.Join(t.TempDir(), "stale.md"), nil)
		other := uuid.NewString()
		if err := f.a.workspaceMgr.AddTab(workspace.Tab{ID: other, Type: workspace.TabTypeEditor}); err != nil {
			t.Fatal(err)
		}
		if err := f.a.workspaceMgr.SetActiveTab(other); err != nil {
			t.Fatal(err)
		}
		writes := 0
		_, err := commitEditorFileCommand(t, &f, true, func(context.Context, string, string, filesystem.FileVersion) error { writes++; return nil })
		if err == nil || writes != 0 {
			t.Fatalf("stale aceito: writes=%d err=%v", writes, err)
		}
	})
	t.Run("auth context", func(t *testing.T) {
		f := beginEditorFileCommand(t, commandEditorFileSaveID, workspace.Tab{ID: uuid.NewString(), Type: workspace.TabTypeEditor})
		prepareEditorFileCommand(t, &f, "bytes", filepath.Join(t.TempDir(), "auth.md"), nil)
		foreign := database.WithUserID(context.Background(), uuid.NewString())
		_, err := f.a.editorCommandCommit(foreign, apidto.EditorCommandCommitRequest{Ticket: f.reservation.Ticket, HandoffID: f.handoff.HandoffID, Token: f.token, ConfirmOverwrite: true}, func(context.Context, string, string, filesystem.FileVersion) error { return nil })
		if !errors.Is(err, commandexecution.ErrDenied) {
			t.Fatalf("contexto estranho: %v", err)
		}
	})
}

func TestCommandEditorFileFirstSaveExistingRequiresOverwriteAndDuplicateDoesNotWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "existing.md")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := beginEditorFileCommand(t, commandEditorFileSaveID, workspace.Tab{ID: uuid.NewString(), Type: workspace.TabTypeEditor})
	prepareEditorFileCommand(t, &f, "new", path, nil)
	writes := 0
	write := func(context.Context, string, string, filesystem.FileVersion) error { writes++; return nil }
	result, err := commitEditorFileCommand(t, &f, false, write)
	if !errors.Is(err, commandui.ErrCancelled) || result == nil || result.Status != "cancelled" || writes != 0 {
		t.Fatalf("overwrite sem decisão: result=%+v writes=%d err=%v", result, writes, err)
	}
	// A decisão cancelada consome o ticket; não há segunda escrita possível.
	if _, err := commitEditorFileCommand(t, &f, true, write); err == nil || writes != 0 {
		t.Fatalf("segunda tentativa escreveu: writes=%d err=%v", writes, err)
	}
}

func TestCommandEditorFileSaveCopyPreservesDocumentMetadata(t *testing.T) {
	source := filepath.Join(t.TempDir(), "document.md")
	copyPath := filepath.Join(t.TempDir(), "document-copy.md")
	if err := os.WriteFile(source, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}
	tabID := uuid.NewString()
	f := beginEditorFileCommand(t, commandEditorFileSaveCopyID, workspace.Tab{
		ID: tabID, Type: workspace.TabTypeEditor, Title: "Documento",
		State: map[string]any{"filePath": source, "displayMode": "rich"},
	})
	prepareEditorFileCommand(t, &f, "cópia", copyPath, nil)
	writes := 0
	result, err := commitEditorFileCommand(t, &f, true, func(_ context.Context, path, content string, _ filesystem.FileVersion) error {
		writes++
		return os.WriteFile(path, []byte(content), 0o644)
	})
	if err != nil || result == nil || result.Status != "succeeded" || !result.Written || writes != 1 {
		t.Fatalf("save copy: result=%+v writes=%d err=%v", result, writes, err)
	}
	got, err := os.ReadFile(copyPath)
	if err != nil || string(got) != "cópia" {
		t.Fatalf("bytes da cópia=%q err=%v", got, err)
	}
	active := f.a.workspaceMgr.Active().ActiveTab()
	if active == nil || active.State["filePath"] != source || active.State["displayMode"] != "rich" || active.Title != "Documento" {
		t.Fatalf("save copy alterou metadata do documento: %+v", active)
	}
}

func TestCommandEditorFileSaveRejectsStaleFileBeforeWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "stale.md")
	if err := os.WriteFile(path, []byte("antes"), 0o644); err != nil {
		t.Fatal(err)
	}
	f := beginEditorFileCommand(t, commandEditorFileSaveID, workspace.Tab{
		ID: uuid.NewString(), Type: workspace.TabTypeEditor, State: map[string]any{"filePath": path},
	})
	prepareEditorFileCommand(t, &f, "novo", "", nil)
	if err := os.WriteFile(path, []byte("mudou fora do editor"), 0o644); err != nil {
		t.Fatal(err)
	}
	writes := 0
	_, err := commitEditorFileCommand(t, &f, true, func(context.Context, string, string, filesystem.FileVersion) error {
		writes++
		return nil
	})
	if err == nil || writes != 0 {
		t.Fatalf("arquivo stale aceito: writes=%d err=%v", writes, err)
	}
	got, readErr := os.ReadFile(path)
	if readErr != nil || string(got) != "mudou fora do editor" {
		t.Fatalf("arquivo stale foi sobrescrito: %q err=%v", got, readErr)
	}
}

func TestCommandEditorFileTargetRejectsNonEditorTab(t *testing.T) {
	a := contextChatFixture(t, workspace.TabTypeChat)
	ctx := database.WithUserID(context.Background(), a.commandProduct.Load().principal.UserID)
	r := beginUICommand(t, a, commandEditorFileSaveID)
	h := takeUICommandFor(t, a, r.Ticket, commandEditorFileSaveID)
	if _, _, err := a.editorCommandTarget(ctx, r.Ticket, h.HandoffID); err == nil {
		t.Fatal("Begin/Target de arquivo aceitou aba não-editor")
	}
}

func TestCommandEditorFileKeyboardResetAfterTakeAllowedButEpochChangeDenied(t *testing.T) {
	t.Run("reset após take", func(t *testing.T) {
		a := readyCommandProduct(t)
		tabID := uuid.NewString()
		if err := a.workspaceMgr.AddTab(workspace.Tab{ID: tabID, Type: workspace.TabTypeEditor}); err != nil {
			t.Fatal(err)
		}
		if err := a.workspaceMgr.SetActiveTab(tabID); err != nil {
			t.Fatal(err)
		}
		view, err := a.GetLocalCommandKeyboardMap()
		if err != nil {
			t.Fatal(err)
		}
		var shortcut LocalCommandShortcut
		for _, binding := range view.Bindings {
			if binding.CommandID == commandEditorFileSaveID && binding.Handler == "contextual" {
				shortcut = binding.Shortcut
				break
			}
		}
		if shortcut.Code == "" {
			t.Fatalf("Ctrl+S contextual não publicado: %+v", view)
		}
		reservation, err := a.BeginLocalCommandUIKey(view.Generation, shortcut, false)
		if err != nil {
			t.Fatal(err)
		}
		handoff, err := a.TakeUICommand(reservation.Ticket)
		if err != nil {
			t.Fatal(err)
		}
		f := editorFileCommandFlow{a: a, ctx: database.WithUserID(context.Background(), a.commandProduct.Load().principal.UserID), reservation: *reservation, handoff: handoff}
		operation, target, err := a.editorCommandTarget(f.ctx, f.reservation.Ticket, f.handoff.HandoffID)
		if err != nil {
			t.Fatal(err)
		}
		a.ResetLocalCommandKeyboard(view.Generation)
		prepareEditorFileCommandAt(t, &f, operation, "keyboard bytes", filepath.Join(t.TempDir(), "keyboard.md"), nil)
		if target != "" {
			t.Fatalf("Target Ctrl+S deveria permitir destino escolhido: %q", target)
		}
		writes := 0
		result, err := commitEditorFileCommand(t, &f, true, func(_ context.Context, gotPath, content string, _ filesystem.FileVersion) error {
			writes++
			return os.WriteFile(gotPath, []byte(content), 0o644)
		})
		if err != nil || result == nil || result.Status != "succeeded" || writes != 1 {
			t.Fatalf("keyboard após reset: result=%+v writes=%d err=%v", result, writes, err)
		}
	})

	t.Run("epoch change", func(t *testing.T) {
		f := beginEditorFileCommand(t, commandEditorFileSaveID, workspace.Tab{ID: uuid.NewString(), Type: workspace.TabTypeEditor})
		prepareEditorFileCommand(t, &f, "bytes", filepath.Join(t.TempDir(), "epoch.md"), nil)
		if err := f.a.commandHost.SetVaultUnlocked(f.ctx, false); err != nil {
			t.Fatal(err)
		}
		writes := 0
		_, err := commitEditorFileCommand(t, &f, true, func(context.Context, string, string, filesystem.FileVersion) error { writes++; return nil })
		if err == nil || writes != 0 {
			t.Fatalf("epoch alterado aceitou: writes=%d err=%v", writes, err)
		}
	})
}

func assertEditorFileLedger(t *testing.T, invocationID, secret, path string) {
	t.Helper()
	var count int64
	if err := database.DB().Table("command_invocations").Where("invocation_id = ?", invocationID).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("ledger count=%d", count)
	}
	var rows []struct {
		ArgumentsSummary string
		ResultSummary    *string
	}
	if err := database.DB().Table("command_invocations").Select("arguments_summary, result_summary").Where("invocation_id = ?", invocationID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	serialized := ""
	for _, row := range rows {
		serialized += row.ArgumentsSummary + " "
		if row.ResultSummary != nil {
			serialized += *row.ResultSummary
		}
	}
	if strings.Contains(serialized, secret) || strings.Contains(serialized, path) {
		t.Fatalf("conteúdo/path persistido no ledger: %q", serialized)
	}
}
