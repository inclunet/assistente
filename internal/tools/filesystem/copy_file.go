package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"assistente/internal/tools"
)

// CopyFile copia um arquivo no disco.
// Exige read na origem e write no destino (quando invocado por skill).
type CopyFile struct {
	workDir string
}

func NewCopyFile(workDir string) *CopyFile {
	return &CopyFile{workDir: workDir}
}

func (t *CopyFile) Name() string { return "copy_file" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *CopyFile) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"}
}

func (t *CopyFile) Description() string {
	return "Copy one existing file to a second path while preserving the source. Use for file duplication or backup. Do not use for directories, renaming or relocating the source (use move_file), or changing file contents. The destination must not exist unless overwrite is explicitly true; copying large files incurs proportional I/O and overwriting can destroy a file. Risk: write."
}

func (t *CopyFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"from": {"type": "string", "description": "Existing source file to copy, absolute or relative to the working directory."},
			"to": {"type": "string", "description": "Destination file path, absolute or relative to the working directory; this tool does not copy directories."},
			"overwrite": {"type": "boolean", "description": "Whether to replace an existing destination file; defaults to false. Set true only when replacement is intentional."}
		},
		"required": ["from", "to"],
		"additionalProperties": false
	}`)
}

type copyFileArgs struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Overwrite *bool  `json:"overwrite,omitempty"`
}

func (t *CopyFile) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a copyFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	from := strings.TrimSpace(a.From)
	to := strings.TrimSpace(a.To)
	if from == "" {
		return tools.ToolResult{Content: "Parâmetro 'from' é obrigatório", IsError: true}, nil
	}
	if to == "" {
		return tools.ToolResult{Content: "Parâmetro 'to' é obrigatório", IsError: true}, nil
	}

	overwrite := false
	if a.Overwrite != nil {
		overwrite = *a.Overwrite
	}

	fullFrom, err := resolveFilePath(from, t.workDir)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	fullTo, err := resolveFilePath(to, t.workDir)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	if err := validatePathWithPolicy(ctx, fullFrom, t.workDir, ToolPolicy(), "copy_from"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	if err := validatePathWithPolicy(ctx, fullTo, t.workDir, ToolPolicy(), "copy_to"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	bytes, err := CopyFileWithPolicy(fullFrom, fullTo, overwrite, ToolPolicy())
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao copiar arquivo: %v", err), IsError: true}, nil
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("Copiado: %s -> %s", from, to),
		Metadata: map[string]any{
			"from":      from,
			"to":        to,
			"overwrite": overwrite,
			"bytes":     bytes,
		},
	}, nil
}
