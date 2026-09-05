package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"assistente/internal/tools"
)

// MakeDirectory cria um diretório no disco (com suporte a parents via MkdirAll).
type MakeDirectory struct {
	workDir string
}

func NewMakeDirectory(workDir string) *MakeDirectory {
	return &MakeDirectory{workDir: workDir}
}

func (t *MakeDirectory) Name() string { return "make_directory" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *MakeDirectory) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"}
}

func (t *MakeDirectory) Description() string {
	return "Create a directory, including missing parent directories by default. Use when an empty directory is required before later file operations. Do not use before write_file solely to create parents, because write_file already does that, and do not use to create files. With parents=false, creation fails if the immediate parent is missing. Risk: write."
}

func (t *MakeDirectory) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {"type": "string", "description": "Directory to create, absolute or relative to the working directory."},
			"parents": {"type": "boolean", "description": "Whether to create missing parent directories; defaults to true. Set false only when the parent must already exist."}
		},
		"required": ["path"],
		"additionalProperties": false
	}`)
}

type makeDirArgs struct {
	Path    string `json:"path"`
	Parents *bool  `json:"parents,omitempty"`
}

func (t *MakeDirectory) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a makeDirArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}
	p := strings.TrimSpace(a.Path)
	if p == "" {
		return tools.ToolResult{Content: "Parâmetro 'path' é obrigatório", IsError: true}, nil
	}
	parents := true
	if a.Parents != nil {
		parents = *a.Parents
	}

	fullPath, err := resolveFilePath(p, t.workDir)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}
	if err := validatePathWithPolicy(ctx, fullPath, t.workDir, ToolPolicy(), "mkdir"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	if info, err := os.Stat(fullPath); err == nil {
		if info.IsDir() {
			return tools.ToolResult{Content: fmt.Sprintf("Já existe: %s", p)}, nil
		}
		return tools.ToolResult{Content: fmt.Sprintf("Já existe um arquivo em '%s'", p), IsError: true}, nil
	}

	if parents {
		if err := EnsureDirWithPolicy(fullPath, 0o755, ToolPolicy()); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Erro ao criar diretório: %v", err), IsError: true}, nil
		}
	} else {
		if err := os.Mkdir(fullPath, 0o755); err != nil {
			return tools.ToolResult{Content: fmt.Sprintf("Erro ao criar diretório: %v", err), IsError: true}, nil
		}
	}

	return tools.ToolResult{Content: fmt.Sprintf("Criado diretório: %s", p), Metadata: map[string]any{"path": p, "parents": parents}}, nil
}
