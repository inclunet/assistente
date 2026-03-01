package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"assistente/internal/tools"
)

// EditFile realiza edições cirúrgicas em arquivos existentes usando substituição de texto.
// Similar ao str_replace: encontra uma string exata e substitui por outra.
// Isso é mais preciso que reescrever o arquivo inteiro com write_file.
type EditFile struct {
	workDir string
}

// NewEditFile cria uma nova instância de EditFile.
func NewEditFile(workDir string) *EditFile {
	return &EditFile{workDir: workDir}
}

func (t *EditFile) Name() string { return "edit_file" }

func (t *EditFile) Description() string {
	return "Edits an existing file by replacing an exact string (old_string) with another (new_string). old_string should be unique (include context/indentation). If multiple occurrences exist, it fails unless replace_all=true."
}

func (t *EditFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo a editar (absoluto ou relativo ao diretório de trabalho)"
			},
			"old_string": {
				"type": "string",
				"description": "Texto exato a ser encontrado e substituído. Deve incluir contexto suficiente para ser único no arquivo."
			},
			"new_string": {
				"type": "string",
				"description": "Texto que substituirá old_string."
			},
			"replace_all": {
				"type": "boolean",
				"description": "Se true, substitui TODAS as ocorrências de old_string. Padrão: false (substitui apenas a primeira ocorrência, falhando se houver mais de uma)."
			}
		},
		"required": ["path", "old_string", "new_string"],
		"additionalProperties": false
	}`)
}

type editFileArgs struct {
	Path       string `json:"path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll *bool  `json:"replace_all,omitempty"`
}

func (t *EditFile) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a editFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	if a.Path == "" {
		return tools.ToolResult{Content: "Parâmetro 'path' é obrigatório", IsError: true}, nil
	}
	if a.OldString == "" {
		return tools.ToolResult{Content: "Parâmetro 'old_string' é obrigatório e não pode ser vazio", IsError: true}, nil
	}
	if a.OldString == a.NewString {
		return tools.ToolResult{Content: "old_string e new_string são idênticos — nenhuma edição necessária", IsError: true}, nil
	}

	replaceAll := false
	if a.ReplaceAll != nil {
		replaceAll = *a.ReplaceAll
	}

	// Resolve caminho
	fullPath, err := t.resolvePath(a.Path)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Valida segurança (toolcalling estrito)
	if err := validatePathWithPolicy(ctx, fullPath, t.workDir, ToolPolicy(), "edit"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Verifica se arquivo existe
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.ToolResult{Content: fmt.Sprintf("Arquivo não encontrado: %s", a.Path), IsError: true}, nil
		}
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao acessar arquivo: %v", err), IsError: true}, nil
	}
	if info.IsDir() {
		return tools.ToolResult{Content: fmt.Sprintf("'%s' é um diretório, não um arquivo", a.Path), IsError: true}, nil
	}

	// Lê conteúdo atual
	data, err := ReadFileBytes(fullPath)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao ler arquivo: %v", err), IsError: true}, nil
	}
	content := string(data)

	// Conta ocorrências
	count := strings.Count(content, a.OldString)

	if count == 0 {
		// Tenta fornecer diagnóstico útil
		hint := ""
		// Verifica se é problema de whitespace/indentação
		trimmedOld := strings.TrimSpace(a.OldString)
		if trimmedOld != a.OldString && strings.Contains(content, trimmedOld) {
			hint = " Dica: o texto foi encontrado com indentação diferente. Verifique espaços/tabs."
		}
		return tools.ToolResult{
			Content: fmt.Sprintf("old_string não encontrada no arquivo '%s'.%s Garanta que old_string corresponde exatamente ao texto no arquivo, incluindo indentação.", a.Path, hint),
			IsError: true,
		}, nil
	}

	if count > 1 && !replaceAll {
		// Encontra as linhas onde old_string aparece para ajudar o LLM
		lines := strings.Split(content, "\n")
		var occurrenceLines []int
		searchIdx := 0
		for i, line := range lines {
			if strings.Contains(line, a.OldString) || (searchIdx < len(content) && strings.Contains(content[searchIdx:], a.OldString)) {
				// Verifica se esta linha faz parte de alguma ocorrência
				if strings.Contains(strings.Join(lines[max(0, i-2):min(len(lines), i+3)], "\n"), a.OldString) {
					occurrenceLines = append(occurrenceLines, i+1)
				}
			}
		}
		// Método mais preciso: busca as posições no conteúdo
		occurrenceLines = findOccurrenceLines(content, a.OldString)

		return tools.ToolResult{
			Content: fmt.Sprintf(
				"old_string encontrada %d vezes no arquivo '%s' (linhas: %v). "+
					"Deve ser única para evitar substituições indesejadas. "+
					"Inclua mais contexto em old_string ou use replace_all=true.",
				count, a.Path, occurrenceLines,
			),
			IsError: true,
		}, nil
	}

	// Realiza a substituição
	var newContent string
	replacements := 0
	if replaceAll {
		newContent = strings.ReplaceAll(content, a.OldString, a.NewString)
		replacements = count
	} else {
		newContent = strings.Replace(content, a.OldString, a.NewString, 1)
		replacements = 1
	}

	// Escreve o arquivo modificado
	if err := WriteFileBytes(fullPath, []byte(newContent), info.Mode()); err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao escrever arquivo: %v", err), IsError: true}, nil
	}
	// Calcula diff resumido
	oldLines := strings.Count(a.OldString, "\n") + 1
	newLines := strings.Count(a.NewString, "\n") + 1
	lineDiff := newLines - oldLines

	diffInfo := ""
	if lineDiff > 0 {
		diffInfo = fmt.Sprintf(", +%d linhas", lineDiff)
	} else if lineDiff < 0 {
		diffInfo = fmt.Sprintf(", %d linhas", lineDiff)
	}

	totalLines := strings.Count(newContent, "\n") + 1

	return tools.ToolResult{
		Content: fmt.Sprintf("Editado: %s — %d substituição(ões)%s (total: %d linhas)",
			a.Path, replacements, diffInfo, totalLines),
		Metadata: map[string]any{
			"path":         a.Path,
			"replacements": replacements,
			"line_diff":    lineDiff,
			"total_lines":  totalLines,
		},
	}, nil
}

func (t *EditFile) resolvePath(path string) (string, error) {
	return resolveFilePath(path, t.workDir)
}

// findOccurrenceLines retorna os números das linhas onde old_string começa.
func findOccurrenceLines(content, oldString string) []int {
	var lines []int
	searchFrom := 0
	for {
		idx := strings.Index(content[searchFrom:], oldString)
		if idx == -1 {
			break
		}
		absIdx := searchFrom + idx
		// Conta newlines até esta posição para determinar a linha
		lineNum := strings.Count(content[:absIdx], "\n") + 1
		lines = append(lines, lineNum)
		searchFrom = absIdx + 1
	}
	return lines
}
