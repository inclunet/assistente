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
	return "Reads a file and returns line-numbered content. Text files (code, Markdown, HTML, JSON, CSV, RTF, ...) are returned verbatim, exactly as stored on disk. Only opaque documents the model cannot read as-is (PDF, DOCX, XLSX, PPTX, ODT/ODS/ODP, EPUB) are converted to a Markdown projection (not the original file), with extraction capped at 32 MiB of input; text has no such cap. Set document_mode to \"markdown\" to also convert text formats that have a projection (e.g. a CSV rendered as a Markdown table). Use offset (1-indexed; negative counts from end) and limit (number of lines of the returned content). Without offset/limit, returns the whole result."
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
				"description": "Linha inicial (1-indexed) do conteúdo retornado. Se negativo, conta do final."
			},
			"limit": {
				"type": "integer",
				"description": "Número máximo de linhas a retornar. Sem limit, retorna tudo a partir do offset."
			},
			"document_mode": {
				"type": "string",
				"enum": ["auto", "markdown"],
				"description": "auto (padrão): só documento opaco (PDF/DOCX/XLSX/PPTX/ODF/EPUB) vira Markdown; arquivo de texto volta como está. markdown: converte também formatos textuais com projeção, como CSV em tabela."
			}
		},
		"required": ["path"],
		"additionalProperties": false
	}`)
}

// readFileArgs são os argumentos parseados de read_file
type readFileArgs struct {
	Path         string `json:"path"`
	Offset       *int   `json:"offset,omitempty"`
	Limit        *int   `json:"limit,omitempty"`
	DocumentMode string `json:"document_mode,omitempty"`
}

// parseDocumentMode valida o modo pedido. Modo desconhecido é erro em vez de
// virar auto: silenciar o engano faria o chamador achar que pediu conversão e
// receber o texto cru sem aviso.
func parseDocumentMode(raw string) (docextract.Mode, error) {
	switch raw {
	case "", string(docextract.ModeAuto):
		return docextract.ModeAuto, nil
	case string(docextract.ModeMarkdown):
		return docextract.ModeMarkdown, nil
	case "ocr":
		return "", fmt.Errorf("document_mode %q ainda não está disponível (previsto para a Fase 3 do AEP-0093)", raw)
	default:
		return "", fmt.Errorf("document_mode inválido: %q (use \"auto\" ou \"markdown\")", raw)
	}
}

func (t *ReadFile) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a readFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	if a.Path == "" {
		return tools.ToolResult{Content: "Parâmetro 'path' é obrigatório", IsError: true}, nil
	}

	mode, err := parseDocumentMode(a.DocumentMode)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
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

	if msg, rejected := rejectOversizedDocument(fullPath, a.Path, info.Size(), mode); rejected {
		return tools.ToolResult{Content: msg, IsError: true}, nil
	}

	if res, handled := readTextSliceStreaming(fullPath, a.Path, info.Size(), a.Offset, a.Limit, mode); handled {
		return res, nil
	}

	// Lê o arquivo
	data, err := ReadFileBytes(fullPath)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao ler arquivo: %v", err), IsError: true}, nil
	}

	extracted, err := docextract.ExtractMode(data, a.Path, mode)
	if err != nil {
		return tools.ToolResult{Content: documentReadError(err), IsError: true}, nil
	}

	var content string
	var meta map[string]any
	if extracted.Projected {
		content = docextract.FormatProjectionHeader(extracted) + extracted.Markdown
		meta = map[string]any{
			"projection": true,
			"format":     string(extracted.Kind),
			"size_bytes": int64(len(data)),
		}
		if extracted.Pages > 0 {
			meta["pages"] = extracted.Pages
		}
	} else {
		content = extracted.Markdown
		meta = map[string]any{
			"size_bytes": int64(len(data)),
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
