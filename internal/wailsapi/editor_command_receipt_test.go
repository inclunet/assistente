package wailsapi

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"assistente/internal/apidto"
	"assistente/internal/database"
	"assistente/internal/tools/filesystem"
)

func TestEditorCommandPrepareFirstSaveAndCopyRequireOverwriteReceipt(t *testing.T) {
	for _, operation := range []string{"save", "save_copy"} {
		t.Run(operation, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "chosen.md")
			if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
				t.Fatal(err)
			}
			api := editorDialogSessionTestAPI(t, &editorDialogPathPort{savePath: path}, func(context.Context) (func() error, error) { return func() error { return nil }, nil })
			api.hooks.CommandTarget = func(context.Context, string, string) (string, string, error) { return operation, "", nil }
			var prepared bool
			api.hooks.CommandPrepare = func(ctx context.Context, req apidto.EditorCommandPrepareRequest, op, resolved string, version filesystem.FileVersion, opened *apidto.EditorOpenResult) (string, error) {
				prepared = true
				if op != operation || req.Content != "replacement" || !version.Exists() || opened != nil {
					t.Fatal("preparation lost operation/content/receipt")
				}
				if err := version.Validate(resolved); err != nil {
					t.Fatal(err)
				}
				return "receipt", nil
			}
			result, err := api.EditorPrepareCommand(apidto.EditorCommandPrepareRequest{Ticket: "ticket", HandoffID: "handoff", Content: "replacement"})
			if err != nil || result == nil || result.Cancelled || !result.RequiresOverwrite || result.Token != "receipt" || !prepared {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			got, err := os.ReadFile(path)
			if err != nil || string(got) != "original" {
				t.Fatalf("prepare wrote content=%q err=%v", got, err)
			}
		})
	}
}

func TestEditorCommandPrepareOpenReturnsContentOnlyToBackendPreparation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "chosen.md")
	if err := os.WriteFile(path, []byte("private content"), 0600); err != nil {
		t.Fatal(err)
	}
	api := editorDialogSessionTestAPI(t, &editorDialogPathPort{openPath: path}, func(context.Context) (func() error, error) { return func() error { return nil }, nil })
	api.hooks.CommandTarget = func(context.Context, string, string) (string, string, error) { return "open", "", nil }
	api.hooks.CommandPrepare = func(_ context.Context, _ apidto.EditorCommandPrepareRequest, op, path string, v filesystem.FileVersion, opened *apidto.EditorOpenResult) (string, error) {
		if op != "open" || opened == nil || opened.Content != "private content" || v.Validate(path) != nil {
			t.Fatal("missing validated open content")
		}
		return "opaque", nil
	}
	result, err := api.EditorPrepareCommand(apidto.EditorCommandPrepareRequest{Ticket: "ticket", HandoffID: "handoff"})
	if err != nil || result == nil || result.Token != "opaque" || result.RequiresOverwrite {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestEditorCommandPrepareCancelDoesNotCreateReceipt(t *testing.T) {
	api := editorDialogSessionTestAPI(t, &editorDialogPathPort{}, func(context.Context) (func() error, error) { return func() error { return nil }, nil })
	api.hooks.CommandTarget = func(context.Context, string, string) (string, string, error) { return "open", "", nil }
	api.hooks.CommandPrepare = func(context.Context, apidto.EditorCommandPrepareRequest, string, string, filesystem.FileVersion, *apidto.EditorOpenResult) (string, error) {
		t.Fatal("cancel prepared a receipt")
		return "", nil
	}
	result, err := api.EditorPrepareCommand(apidto.EditorCommandPrepareRequest{Ticket: "ticket", HandoffID: "handoff"})
	if err != nil || result == nil || !result.Cancelled || result.Token != "" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
}

func TestEditorCommandWriteRefusesChangedVersionAndReadOnlyFormat(t *testing.T) {
	api := editorDialogSessionTestAPI(t, &editorDialogPathPort{}, func(context.Context) (func() error, error) { return func() error { return nil }, nil })
	ctx := database.WithUserID(context.Background(), editorTestUserID)
	path := filepath.Join(t.TempDir(), "target.md")
	if err := os.WriteFile(path, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := filesystem.CaptureFileVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("external"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := api.writeEditorCommandFile(ctx, path, "overwrite", v); !errors.Is(err, filesystem.ErrFileVersionChanged) {
		t.Fatalf("stale receipt=%v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "external" {
		t.Fatalf("content=%q err=%v", got, err)
	}
	readonly := filepath.Join(t.TempDir(), "target.pdf")
	if err := os.WriteFile(readonly, []byte("%PDF-1.7"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err = filesystem.CaptureFileVersion(readonly)
	if err != nil {
		t.Fatal(err)
	}
	if err := api.writeEditorCommandFile(ctx, readonly, "overwrite", v); err == nil {
		t.Fatal("readonly format overwritten")
	}
}
