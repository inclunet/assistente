package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"assistente/internal/questionnaire"
	"assistente/internal/tools"
	"assistente/internal/tools/invocationctx"
)

// TextEdit aplica uma substituição de texto no arquivo ativo do editor.
// É a tool preferida da superfície do editor (skill editor-texto): o modelo
// não informa caminho — o alvo é sempre o arquivo ativo descoberto via
// contexto de invocação (TabType == "editor" + ActiveFilePath). Sempre exibe
// um questionário de confirmação Antes/Depois (Aplicar/Rejeitar) antes de
// escrever. Fora do editor, orienta o modelo a usar edit_file.
type TextEdit struct {
	workDir  string
	questMgr QuestionnaireRequester
	onWrite  FileWriteObserver
}

// TextEditOption configura integrações opcionais da tool.
type TextEditOption func(*TextEdit)

// WithTextEditWriteObserver registra um observador para escritas feitas pela tool.
func WithTextEditWriteObserver(observer FileWriteObserver) TextEditOption {
	return func(t *TextEdit) {
		t.onWrite = observer
	}
}

// NewTextEdit cria uma nova instância de TextEdit.
func NewTextEdit(workDir string, questMgr QuestionnaireRequester, opts ...TextEditOption) *TextEdit {
	t := &TextEdit{workDir: workDir, questMgr: questMgr}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	return t
}

func (t *TextEdit) Name() string { return "text_edit" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *TextEdit) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write", Tags: []string{"editor"}}
}

func (t *TextEdit) Description() string {
	return "Replaces the selected text in the active editor text file after user confirmation (Apply/Reject). Refuses document formats (PDF, DOCX, etc.). Use 'original' with the exact selected text and 'replacement' with the final content. Only works from an editor tab with an open file; elsewhere use edit_file."
}

func (t *TextEdit) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"original": {
				"type": "string",
				"description": "Trecho exato do texto selecionado a substituir. Deve corresponder exatamente ao conteúdo do arquivo (incluindo indentação) e ser único; se não for, inclua mais contexto ao redor."
			},
			"replacement": {
				"type": "string",
				"description": "Conteúdo final que substituirá o trecho selecionado. Somente o texto final, sem explicações."
			},
			"format": {
				"type": "string",
				"enum": ["markdown", "plain"],
				"description": "Formato do conteúdo final. Padrão: markdown."
			},
			"notes": {
				"type": "string",
				"description": "Justificativa breve da alteração, quando útil."
			},
			"title": {
				"type": "string",
				"description": "Título opcional para o questionário de confirmação."
			},
			"description": {
				"type": "string",
				"description": "Descrição opcional para o questionário de confirmação."
			}
		},
		"required": ["original", "replacement"],
		"additionalProperties": false
	}`)
}

type textEditArgs struct {
	Original    string `json:"original"`
	Replacement string `json:"replacement"`
	Format      string `json:"format,omitempty"`
	Notes       string `json:"notes,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

func (t *TextEdit) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a textEditArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	if a.Original == "" {
		return tools.ToolResult{Content: "Parâmetro 'original' é obrigatório e não pode ser vazio", IsError: true}, nil
	}
	if a.Original == a.Replacement {
		return tools.ToolResult{Content: "original e replacement são idênticos — nenhuma edição necessária", IsError: true}, nil
	}

	format := strings.TrimSpace(strings.ToLower(a.Format))
	if format == "" {
		format = "markdown"
	}
	if format != "markdown" && format != "plain" {
		return tools.ToolResult{Content: fmt.Sprintf("Parâmetro 'format' inválido: %q. Use 'markdown' ou 'plain'.", a.Format), IsError: true}, nil
	}

	// A tool é específica da superfície do editor: o alvo é sempre o arquivo
	// ativo da aba de editor que invocou o turno.
	inv, ok := invocationctx.Get(ctx)
	if !ok || inv.TabType != "editor" || strings.TrimSpace(inv.ActiveFilePath) == "" {
		return tools.ToolResult{
			Content: "text_edit só pode ser usada a partir de uma aba de editor com arquivo ativo. Para editar arquivos fora do editor, use edit_file com o parâmetro 'path'.",
			IsError: true,
		}, nil
	}
	fullPath := inv.ActiveFilePath

	// Valida segurança (toolcalling estrito)
	if err := validatePathWithPolicy(ctx, fullPath, t.workDir, ToolPolicy(), "edit"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Verifica se arquivo existe
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return tools.ToolResult{Content: fmt.Sprintf("Arquivo ativo não encontrado: %s", fullPath), IsError: true}, nil
		}
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao acessar arquivo: %v", err), IsError: true}, nil
	}
	if info.IsDir() {
		return tools.ToolResult{Content: fmt.Sprintf("'%s' é um diretório, não um arquivo", fullPath), IsError: true}, nil
	}

	// Lê conteúdo atual
	data, err := ReadFileBytes(fullPath)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao ler arquivo: %v", err), IsError: true}, nil
	}
	if msg, ok := rejectDocumentWrite(data, fullPath); ok {
		return tools.ToolResult{Content: msg, IsError: true}, nil
	}
	content := string(data)

	// Conta ocorrências — 'original' deve ser único no arquivo. Usa
	// findOccurrenceLines (que avança 1 byte por match) para também contar
	// ocorrências sobrepostas, que strings.Count ignoraria.
	occurrenceLines := findOccurrenceLines(content, a.Original)
	count := len(occurrenceLines)

	if count == 0 {
		hint := ""
		trimmedOriginal := strings.TrimSpace(a.Original)
		if trimmedOriginal != a.Original && strings.Contains(content, trimmedOriginal) {
			hint = " Dica: o texto foi encontrado com indentação diferente. Verifique espaços/tabs."
		}
		return tools.ToolResult{
			Content: fmt.Sprintf("'original' não encontrado no arquivo ativo '%s'.%s Garanta que 'original' corresponde exatamente ao texto selecionado, incluindo indentação e quebras de linha.", fullPath, hint),
			IsError: true,
		}, nil
	}

	if count > 1 {
		return tools.ToolResult{
			Content: fmt.Sprintf(
				"'original' encontrado %d vezes no arquivo ativo '%s' (linhas: %v). "+
					"Deve ser único para evitar substituições indesejadas. "+
					"Inclua mais contexto ao redor da seleção em 'original' (e o mesmo contexto em 'replacement').",
				count, fullPath, occurrenceLines,
			),
			IsError: true,
		}, nil
	}

	// Confirmação Antes/Depois (Aplicar/Rejeitar) — sempre, pois a tool é da
	// superfície do editor. Título e descrição que o modelo escreve são
	// conteúdo: vão como texto puro, sem chave de tradução (AEP-0085 D6).
	title := editConfirmTitle()
	if doModelo := strings.TrimSpace(a.Title); doModelo != "" {
		title = questionnaire.Plain(doModelo)
	}
	description := editConfirmDescription(
		strings.TrimSpace(a.Description),
		fullPath,
		strings.TrimSpace(a.Notes),
	)
	if confirmed, toolResult := confirmEditWithDiff(ctx, t.questMgr, title, description, a.Original, a.Replacement); !confirmed {
		return toolResult, nil
	}

	// Relê o arquivo após a confirmação: o disco pode ter mudado enquanto o
	// usuário revisava (outra aba, autosave tardio, processo externo). Aplicar
	// a substituição sobre o snapshot antigo descartaria essas alterações.
	freshData, err := ReadFileBytes(fullPath)
	if err != nil {
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao reler arquivo após confirmação: %v", err), IsError: true}, nil
	}
	if fresh := string(freshData); fresh != content {
		freshCount := len(findOccurrenceLines(fresh, a.Original))
		if freshCount != 1 {
			return tools.ToolResult{
				Content: fmt.Sprintf(
					"O arquivo '%s' foi modificado durante a revisão e 'original' agora ocorre %d vez(es). "+
						"Nenhuma alteração foi aplicada. Releia o conteúdo atual e tente novamente.",
					fullPath, freshCount,
				),
				IsError: true,
			}, nil
		}
		content = fresh
	}

	// Realiza a substituição (única ocorrência garantida acima)
	newContent := strings.Replace(content, a.Original, a.Replacement, 1)

	// Escreve o arquivo modificado
	var cancelWriteMarker func(bool)
	if t.onWrite != nil {
		cancelWriteMarker = t.onWrite(fullPath)
	}
	if err := WriteFileBytes(fullPath, []byte(newContent), info.Mode()); err != nil {
		if cancelWriteMarker != nil {
			cancelWriteMarker(false)
		}
		return tools.ToolResult{Content: fmt.Sprintf("Erro ao escrever arquivo: %v", err), IsError: true}, nil
	}
	if cancelWriteMarker != nil {
		cancelWriteMarker(true)
	}

	// Calcula diff resumido
	oldLines := strings.Count(a.Original, "\n") + 1
	newLines := strings.Count(a.Replacement, "\n") + 1
	lineDiff := newLines - oldLines

	diffInfo := ""
	if lineDiff > 0 {
		diffInfo = fmt.Sprintf(", +%d linhas", lineDiff)
	} else if lineDiff < 0 {
		diffInfo = fmt.Sprintf(", %d linhas", lineDiff)
	}

	totalLines := strings.Count(newContent, "\n") + 1

	return tools.ToolResult{
		Content: fmt.Sprintf("Edição aplicada em: %s — 1 substituição%s (total: %d linhas)", fullPath, diffInfo, totalLines),
		Metadata: map[string]any{
			"path":         fullPath,
			"format":       format,
			"replacements": 1,
			"line_diff":    lineDiff,
			"total_lines":  totalLines,
		},
	}, nil
}
