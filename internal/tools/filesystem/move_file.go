package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"assistente/internal/tools"
)

// MoveFileTool move/renomeia um arquivo no disco.
// Renomear = mover dentro do mesmo diretório.
type MoveFileTool struct {
	workDir string
}

func NewMoveFile(workDir string) *MoveFileTool {
	return &MoveFileTool{workDir: workDir}
}

func (t *MoveFileTool) Name() string { return "move_file" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *MoveFileTool) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"}
}

func (t *MoveFileTool) Description() string {
	return "Move or rename one existing file. Use to change a file's path without keeping the source; renaming is a move within the same directory. Do not use for directories, to keep the source (use copy_file), or to change file contents. The destination must not exist unless overwrite is explicitly true; overwriting can destroy a file. Risk: write."
}

func (t *MoveFileTool) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"from": {"type": "string", "description": "Existing source file, absolute or relative to the working directory; it is removed after a successful move."},
			"to": {"type": "string", "description": "Destination file path, absolute or relative to the working directory; this tool does not move directories."},
			"overwrite": {"type": "boolean", "description": "Whether to replace an existing destination file; defaults to false. Set true only when replacement is intentional."}
		},
		"required": ["from", "to"],
		"additionalProperties": false
	}`)
}

type moveFileArgs struct {
	From      string `json:"from"`
	To        string `json:"to"`
	Overwrite *bool  `json:"overwrite,omitempty"`
}

func (t *MoveFileTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a moveFileArgs
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

	// Resolve paths
	fullFrom, err := resolveFilePath(from, t.workDir)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	fullTo, err := resolveFilePath(to, t.workDir)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Validate safety
	// Move usa operações específicas (move_from/move_to) para que a autorização
	// e o bloqueio de sensíveis sejam avaliados por ponta do movimento (AEP-0092).
	if err := validatePathWithPolicy(ctx, fullFrom, t.workDir, ToolPolicy(), "move_from"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	if err := validatePathWithPolicy(ctx, fullTo, t.workDir, ToolPolicy(), "move_to"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Do move
	if err := MoveFile(fullFrom, fullTo, overwrite); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao mover arquivo: %v", err), IsError: true}, nil
	}

	// Best-effort: bytes moved (stat dest)
	bytes := 0
	if info, statErr := os.Stat(fullTo); statErr == nil {
		bytes = int(info.Size())
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("Movido: %s -> %s", from, to),
		Metadata: map[string]any{
			"from":      from,
			"to":        to,
			"overwrite": overwrite,
			"bytes":     bytes,
		},
	}, nil
}
