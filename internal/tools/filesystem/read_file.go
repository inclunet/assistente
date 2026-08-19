package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"assistente/internal/docextract"
	"assistente/internal/tools"
)

// ReadFile lê o conteúdo de um arquivo no disco.
// Suporta offset e limit para ler arquivos grandes parcialmente.
// Documentos V1 (AEP-0093) são projetados para Markdown.
type ReadFile struct {
	// workDir é o diretório base para caminhos relativos
	workDir string
}

// NewReadFile cria uma nova instância de ReadFile.
// workDir define o diretório base para resolução de caminhos relativos.
func NewReadFile(workDir string) *ReadFile {
	return &ReadFile{workDir: workDir}
}

func (t *ReadFile) Name() string { return "read_file" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *ReadFile) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "filesystem", Class: "read_context", Package: "coding_readonly", Risk: "read"}
}

func (t *ReadFile) Description() string {
	return "Reads a file and returns line-numbered content. For documents (PDF, DOCX, XLSX, PPTX, ODT/ODS/ODP, CSV, RTF, EPUB) returns a Markdown projection (not the original file); document extraction is capped at 32 MiB of input, while plain text/code has no such cap. Use offset (1-indexed; negative counts from end) and limit (number of lines of the returned text/projection). Without offset/limit, returns the whole result."
}

func (t *ReadFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo (absoluto ou relativo ao diretório de trabalho)"
			},
			"offset": {
				"type": "integer",
				"description": "Linha inicial (1-indexed) do texto/projeção. Se negativo, conta do final."
			},
			"limit": {
				"type": "integer",
				"description": "Número máximo de linhas a retornar. Sem limit, retorna tudo a partir do offset."
			}
		},
		"required": ["path"],
		"additionalProperties": false
	}`)
}

// readFileArgs são os argumentos parseados de read_file
type readFileArgs struct {
	Path   string `json:"path"`
	Offset *int   `json:"offset,omitempty"`
	Limit  *int   `json:"limit,omitempty"`
}

func (t *ReadFile) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a readFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	if a.Path == "" {
		return tools.ToolResult{Content: "Parâmetro 'path' é obrigatório", IsError: true}, nil
	}

	// Resolve caminho
	fullPath, err := t.resolvePath(a.Path)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Valida segurança do caminho (toolcalling estrito)
	if err := validatePathWithPolicy(ctx, fullPath, t.workDir, ToolPolicy(), "read"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Verifica se existe e é um arquivo
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.ToolResult{Content: fmt.Sprintf("Arquivo não encontrado: %s", a.Path), IsError: true}, nil
		}
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao acessar arquivo: %v", err), IsError: true}, nil
	}
	if info.IsDir() {
		return tools.ToolResult{Content: fmt.Sprintf("'%s' é um diretório, não um arquivo. Use list_directory.", a.Path), IsError: true}, nil
	}

	if msg, rejected := rejectOversizedDocument(fullPath, a.Path, info.Size()); rejected {
		return tools.ToolResult{Content: msg, IsError: true}, nil
	}

	// Lê o arquivo
	data, err := ReadFileBytes(fullPath)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao ler arquivo: %v", err), IsError: true}, nil
	}

	extracted, err := docextract.Extract(data, a.Path)
	if err != nil {
		return tools.ToolResult{Content: documentReadError(err), IsError: true}, nil
	}

	var content string
	var meta map[string]any
	if docextract.IsDocument(extracted.Kind) {
		body := docextract.FormatProjectionHeader(extracted) + extracted.Markdown
		content = body
		meta = map[string]any{
			"projection": true,
			"format":     string(extracted.Kind),
			"size_bytes": len(data),
		}
		if extracted.Pages > 0 {
			meta["pages"] = extracted.Pages
		}
	} else {
		content = extracted.Markdown
		meta = map[string]any{
			"size_bytes": len(data),
		}
	}

	lines := strings.Split(content, "\n")
	totalLines := len(lines)

	// Aplica offset e limit se fornecidos
	if a.Offset != nil || a.Limit != nil {
		offset := 0
		if a.Offset != nil {
			offset = *a.Offset
		}

		// Offset negativo conta do final
		if offset < 0 {
			offset = totalLines + offset
			if offset < 0 {
				offset = 0
			}
		} else if offset > 0 {
			// 1-indexed para 0-indexed
			offset = offset - 1
		}

		if offset >= totalLines {
			return tools.ToolResult{
				Content: fmt.Sprintf("Offset %d excede o número de linhas (%d)", *a.Offset, totalLines),
				IsError: true,
			}, nil
		}

		end := totalLines
		if a.Limit != nil && *a.Limit > 0 {
			end = offset + *a.Limit
			if end > totalLines {
				end = totalLines
			}
		}

		// Numera as linhas para contexto
		var numbered []string
		for i := offset; i < end; i++ {
			numbered = append(numbered, fmt.Sprintf("%6d|%s", i+1, lines[i]))
		}

		header := fmt.Sprintf("Arquivo: %s (linhas %d-%d de %d)\n", a.Path, offset+1, end, totalLines)
		meta["total_lines"] = totalLines
		meta["offset"] = offset + 1
		meta["limit"] = end - offset
		return tools.ToolResult{
			Content:  header + strings.Join(numbered, "\n"),
			Metadata: meta,
		}, nil
	}

	// Retorno completo com numeração de linhas
	var numbered []string
	for i, line := range lines {
		numbered = append(numbered, fmt.Sprintf("%6d|%s", i+1, line))
	}

	header := fmt.Sprintf("Arquivo: %s (%d linhas, %d bytes)\n", a.Path, totalLines, len(data))
	meta["total_lines"] = totalLines
	return tools.ToolResult{
		Content:  header + strings.Join(numbered, "\n"),
		Metadata: meta,
	}, nil
}

// resolvePath converte caminho relativo para absoluto usando workDir
func (t *ReadFile) resolvePath(path string) (string, error) {
	return resolveFilePath(path, t.workDir)
}
