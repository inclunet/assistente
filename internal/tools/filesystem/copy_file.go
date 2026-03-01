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

func (t *CopyFile) Description() string {
	return "Copies a file on disk. Validates paths, respects skill filesystem scope, and blocks sensitive files. Fails if destination exists unless overwrite=true."
}

func (t *CopyFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"from": {"type": "string", "description": "Path de origem (absoluto ou relativo ao diretório de trabalho)"},
			"to": {"type": "string", "description": "Path de destino (absoluto ou relativo ao diretório de trabalho)"},
			"overwrite": {"type": "boolean", "description": "Se true, sobrescreve o destino se existir. Padrão: false"}
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

	if err := validatePathWithPolicy(ctx, fullFrom, t.workDir, ToolPolicy(), "read"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	if err := validatePathWithPolicy(ctx, fullTo, t.workDir, ToolPolicy(), "write"); err != nil {
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
