package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"assistente/internal/tools"
)

// DeleteFile remove um arquivo no disco.
// Por segurança, não remove diretórios.
type DeleteFile struct {
	workDir string
}

func NewDeleteFile(workDir string) *DeleteFile {
	return &DeleteFile{workDir: workDir}
}

func (t *DeleteFile) Name() string { return "delete_file" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *DeleteFile) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "destructive"}
}

func (t *DeleteFile) Description() string {
	return "Permanently delete one file. Use only when the requested outcome requires removing that specific file. Do not use for directories, cleanup speculation, or content changes; prefer editing or moving when deletion is unnecessary. The operation is destructive and does not provide recovery. Risk: destructive."
}

func (t *DeleteFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Existing file to delete permanently, absolute or relative to the working directory; directories are rejected."},
			"missing_ok": {"type": "boolean", "description": "Whether an absent file counts as success; defaults to false. Use true for idempotent cleanup of an explicitly named file."}
		},
		"required": ["path"],
		"additionalProperties": false
	}`)
}

type deleteFileArgs struct {
	Path      string `json:"path"`
	MissingOk *bool  `json:"missing_ok,omitempty"`
}

func (t *DeleteFile) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a deleteFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}
	p := strings.TrimSpace(a.Path)
	if p == "" {
		return tools.ToolResult{Content: "Parâmetro 'path' é obrigatório", IsError: true}, nil
	}
	missingOk := false
	if a.MissingOk != nil {
		missingOk = *a.MissingOk
	}

	fullPath, err := resolveFilePath(p, t.workDir)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	if err := validatePathWithPolicy(ctx, fullPath, t.workDir, ToolPolicy(), "delete"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			if missingOk {
				return tools.ToolResult{Content: fmt.Sprintf("Nada para remover (não existe): %s", p)}, nil
			}
			return tools.ToolResult{Content: fmt.Sprintf("Arquivo não encontrado: %s", p), IsError: true}, nil
		}
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao acessar arquivo: %v", err), IsError: true}, nil
	}
	if info.IsDir() {
		return tools.ToolResult{Content: fmt.Sprintf("'%s' é um diretório. delete_file remove apenas arquivos.", p), IsError: true}, nil
	}

	bytes := info.Size()
	if err := RemoveFileWithPolicy(fullPath, ToolPolicy()); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao remover arquivo: %v", err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("Removido: %s", p),
		Metadata: map[string]any{"path": p, "bytes": bytes},
	}, nil
}
