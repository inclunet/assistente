package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"assistente/internal/tools"
)

// ReadFile lê o conteúdo de um arquivo no disco.
// Suporta offset e limit para ler arquivos grandes parcialmente.
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

func (t *ReadFile) Description() string {
	return "Lê o conteúdo de um arquivo. Suporta leitura parcial com offset (linha inicial) e limit (número de linhas). Sem offset/limit, retorna o arquivo inteiro."
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
				"description": "Linha inicial (1-indexed). Se negativo, conta do final do arquivo."
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

	// Valida segurança do caminho
	if err := validatePath(fullPath, t.workDir); err != nil {
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

	// Lê o arquivo
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao ler arquivo: %v", err), IsError: true}, nil
	}

	content := string(data)
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
		return tools.ToolResult{
			Content: header + strings.Join(numbered, "\n"),
			Metadata: map[string]any{
				"total_lines": totalLines,
				"offset":      offset + 1,
				"limit":       end - offset,
			},
		}, nil
	}

	// Retorno completo com numeração de linhas
	var numbered []string
	for i, line := range lines {
		numbered = append(numbered, fmt.Sprintf("%6d|%s", i+1, line))
	}

	header := fmt.Sprintf("Arquivo: %s (%d linhas, %d bytes)\n", a.Path, totalLines, len(data))
	return tools.ToolResult{
		Content: header + strings.Join(numbered, "\n"),
		Metadata: map[string]any{
			"total_lines": totalLines,
			"size_bytes":  len(data),
		},
	}, nil
}

// resolvePath converte caminho relativo para absoluto usando workDir
func (t *ReadFile) resolvePath(path string) (string, error) {
	if filepath.IsAbs(path) {
		return filepath.Clean(path), nil
	}
	return filepath.Abs(filepath.Join(t.workDir, path))
}
