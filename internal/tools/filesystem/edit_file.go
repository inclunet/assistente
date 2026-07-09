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

// QuestionnaireRequester abstrai o gerenciador de questionários para injeção de dependência.
type QuestionnaireRequester interface {
	RequestQuestionnaire(ctx context.Context, payload questionnaire.RequestPayload) (questionnaire.Response, error)
}

// editPolicy descreve o comportamento de confirmação da tool.
type editPolicy int

const (
	policyDirect          editPolicy = iota // edita direto, sem confirmação
	policyConfirmWithDiff                   // mostra diff e pede confirmação antes de editar
)

// EditFile realiza edições cirúrgicas em arquivos existentes usando substituição de texto.
// Similar ao str_replace: encontra uma string exata e substitui por outra.
// Quando invocada de uma aba de editor com o arquivo ativo, exibe confirmação com diff antes de editar.
type EditFile struct {
	workDir  string
	questMgr QuestionnaireRequester
	onWrite  FileWriteObserver
}

// FileWriteObserver é chamado imediatamente antes de uma tool escrever no disco.
// Retorna uma função opcional que recebe se a escrita foi concluída com sucesso.
type FileWriteObserver func(path string) func(committed bool)

// EditFileOption configura integrações opcionais da tool.
type EditFileOption func(*EditFile)

// WithEditFileWriteObserver registra um observador para escritas feitas pela tool.
func WithEditFileWriteObserver(observer FileWriteObserver) EditFileOption {
	return func(t *EditFile) {
		t.onWrite = observer
	}
}

// NewEditFile cria uma nova instância de EditFile.
func NewEditFile(workDir string, questMgr QuestionnaireRequester, opts ...EditFileOption) *EditFile {
	t := &EditFile{workDir: workDir, questMgr: questMgr}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	return t
}

func (t *EditFile) Name() string { return "edit_file" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *EditFile) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"}
}

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
		hint := ""
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
		occurrenceLines := findOccurrenceLines(content, a.OldString)
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

	// Resolve política de confirmação baseada no contexto de invocação
	policy := t.resolvePolicy(ctx, fullPath)

	if policy == policyConfirmWithDiff {
		if confirmed, toolResult := t.confirmWithDiff(ctx, a.Path, a.OldString, a.NewString); !confirmed {
			return toolResult, nil
		}
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

// resolvePolicy determina o comportamento de confirmação com base no contexto de invocação.
func (t *EditFile) resolvePolicy(ctx context.Context, fullPath string) editPolicy {
	inv, ok := invocationctx.Get(ctx)
	if !ok {
		return policyDirect
	}
	if inv.TabType == "editor" && inv.ActiveFilePath == fullPath {
		return policyConfirmWithDiff
	}
	return policyDirect
}

// confirmWithDiff exibe um questionário com o diff antes/depois e aguarda confirmação do usuário.
// Retorna (true, zero) se aprovado, ou (false, errorResult) se rejeitado ou em erro.
// Sem gerenciador de questionários (contextos não-UI: CLI/testes), aprova direto.
func (t *EditFile) confirmWithDiff(ctx context.Context, displayPath, oldString, newString string) (bool, tools.ToolResult) {
	title := "Confirmar edição"
	description := fmt.Sprintf("Revise a alteração em **%s** e clique em Aplicar para confirmar.", displayPath)
	return confirmEditWithDiff(ctx, t.questMgr, title, description, oldString, newString)
}

// confirmEditWithDiff exibe um questionário Antes/Depois (Aplicar/Rejeitar) e aguarda
// a confirmação do usuário. Compartilhado por edit_file e text_edit.
// Retorna (true, zero) se aprovado, ou (false, errorResult) se rejeitado ou em erro.
func confirmEditWithDiff(ctx context.Context, questMgr QuestionnaireRequester, title, description, before, after string) (bool, tools.ToolResult) {
	if questMgr == nil {
		// Sem gerenciador de questionários: edita direto (seguro para contextos não-UI)
		return true, tools.ToolResult{}
	}

	resp, err := questMgr.RequestQuestionnaire(ctx, questionnaire.RequestPayload{
		Title:       title,
		Description: description,
		Questions: []questionnaire.Question{
			{ID: "before", Type: "readonly_code", Prompt: "Antes", Content: before},
			{ID: "after", Type: "readonly_code", Prompt: "Depois", Content: after},
		},
		AllowCancel: true,
		SubmitLabel: "Aplicar",
		CancelLabel: "Rejeitar",
	})
	if err != nil {
		return false, tools.ToolResult{Content: fmt.Sprintf("Erro ao solicitar confirmação: %v", err), IsError: true}
	}
	if resp.Cancelled {
		return false, tools.ToolResult{Content: "Alteração rejeitada pelo usuário", IsError: true}
	}
	return true, tools.ToolResult{}
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
		lineNum := strings.Count(content[:absIdx], "\n") + 1
		lines = append(lines, lineNum)
		searchFrom = absIdx + 1
	}
	return lines
}
