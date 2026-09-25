package wailsapi

import (
	"context"
	"fmt"
	"os"
	"strings"

	"assistente/internal/apidto"
	"assistente/internal/core/ports"
	"assistente/internal/docextract"
	"assistente/internal/tools/filesystem"
)

// EditorPrepareCommand captura o alvo, abre o diálogo e prepara o receipt.
// A autoridade do alvo e do comando permanece no App; este bind somente
// resolve o path, lê a seleção e entrega a observação efêmera ao hook.
func (api *Editor) EditorPrepareCommand(request apidto.EditorCommandPrepareRequest) (*apidto.EditorCommandPreparation, error) {
	session, hooks, err := api.deps()
	if err != nil {
		return nil, err
	}
	if hooks.CommandTarget == nil || hooks.CommandPrepare == nil {
		return nil, ErrEditorNotWired
	}
	return WithUser(session, func(ctx context.Context) (*apidto.EditorCommandPreparation, error) {
		if err := requireEditorUser(ctx); err != nil {
			return nil, err
		}
		operation, targetPath, err := hooks.CommandTarget(ctx, request.Ticket, request.HandoffID)
		if err != nil {
			return nil, err
		}
		operation = strings.TrimSpace(operation)
		if operation != "open" && operation != "save" && operation != "save_copy" {
			return nil, fmt.Errorf("operação de arquivo inválida")
		}

		validateDialog, err := api.captureDialogSession(ctx)
		if err != nil {
			return nil, err
		}
		var chosen string
		switch operation {
		case "open":
			dialog := hooks.Dialog()
			if dialog == nil {
				return nil, fmt.Errorf("app não inicializado")
			}
			chosen, err = dialog.OpenFileDialog(ports.OpenFileOptions{Title: orDefault(request.Labels.Title, "Abrir arquivo"), Filters: dialogFilters(request.Labels, true)})
		case "save_copy":
			dialog := hooks.Dialog()
			if dialog == nil {
				return nil, fmt.Errorf("app não inicializado")
			}
			name := orDefault(request.SuggestedFilename, orDefault(request.Labels.DefaultFilename, "documento.md"))
			chosen, err = dialog.SaveFileDialog(ports.SaveFileOptions{Title: orDefault(request.Labels.Title, "Salvar arquivo"), DefaultFilename: name, Filters: dialogFilters(request.Labels, false)})
		case "save":
			chosen = targetPath
			if strings.TrimSpace(chosen) == "" {
				dialog := hooks.Dialog()
				if dialog == nil {
					return nil, fmt.Errorf("app não inicializado")
				}
				name := orDefault(request.SuggestedFilename, orDefault(request.Labels.DefaultFilename, "documento.md"))
				chosen, err = dialog.SaveFileDialog(ports.SaveFileOptions{Title: orDefault(request.Labels.Title, "Salvar arquivo"), DefaultFilename: name, Filters: dialogFilters(request.Labels, false)})
			}
		}
		if err != nil {
			return nil, err
		}
		if err := validateDialog(); err != nil {
			return nil, err
		}
		if strings.TrimSpace(chosen) == "" {
			return &apidto.EditorCommandPreparation{Cancelled: true}, nil
		}
		resolved, err := api.resolveUserFilePath(ctx, chosen)
		if err != nil {
			return nil, err
		}

		version, err := filesystem.CaptureFileVersion(resolved)
		if err != nil {
			return nil, err
		}
		var opened *apidto.EditorOpenResult
		if operation == "open" {
			opened, err = api.readDocument(ctx, resolved)
			if err != nil {
				return nil, err
			}
			if err := validateDialog(); err != nil {
				return nil, err
			}
			if err := version.Validate(resolved); err != nil {
				return nil, err
			}
		}
		token, err := hooks.CommandPrepare(ctx, request, operation, resolved, version, opened)
		if err != nil {
			return nil, err
		}
		return &apidto.EditorCommandPreparation{
			Token: token, Path: resolved,
			RequiresOverwrite: version.Exists() && (operation == "save_copy" || (operation == "save" && strings.TrimSpace(targetPath) == "")),
		}, nil
	})
}

// EditorCommitCommand delega o commit real ao App. O hook executa a escrita
// dentro do broker, após revalidar ticket, principal, snapshot e FileVersion.
func (api *Editor) EditorCommitCommand(request apidto.EditorCommandCommitRequest) (*apidto.EditorCommandResult, error) {
	session, hooks, err := api.deps()
	if err != nil {
		return nil, err
	}
	if hooks.CommandCommit == nil {
		return nil, ErrEditorNotWired
	}
	return WithUser(session, func(ctx context.Context) (*apidto.EditorCommandResult, error) {
		if err := requireEditorUser(ctx); err != nil {
			return nil, err
		}
		return hooks.CommandCommit(ctx, request, api.writeEditorCommandFile)
	})
}

func (api *Editor) writeEditorCommandFile(ctx context.Context, path, content string, expected filesystem.FileVersion) error {
	if ctx == nil {
		return context.Canceled
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	resolved, err := api.resolveUserFilePath(ctx, path)
	if err != nil || resolved != path {
		if err != nil {
			return err
		}
		return fmt.Errorf("path resolvido diverge do receipt")
	}
	if existing, readErr := readEditorFilePrefix(resolved); readErr == nil {
		if err := docextract.CheckWritable(existing, resolved); err != nil {
			return err
		}
	} else if !os.IsNotExist(readErr) {
		return fmt.Errorf("não foi possível classificar o arquivo antes de salvar: %w", readErr)
	}
	if err := docextract.CheckWritableString(content, resolved); err != nil {
		return err
	}
	perm := os.FileMode(0644)
	if info, statErr := os.Stat(resolved); statErr == nil {
		perm = info.Mode().Perm()
	}
	_, hooks, err := api.deps()
	if err != nil {
		return err
	}
	commit := hooks.MarkSelfWrite(resolved)
	if err := filesystem.WriteFileBytesReplacingVersion(resolved, []byte(content), perm, expected); err != nil {
		if commit != nil {
			commit(false)
		}
		return fmt.Errorf("falha ao salvar arquivo: %w", err)
	}
	if commit != nil {
		commit(true)
	}
	return nil
}
