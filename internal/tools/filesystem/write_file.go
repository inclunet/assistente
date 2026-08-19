package filesystem

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"assistente/internal/tools"
)

// WriteFile cria ou sobrescreve um arquivo no disco.
// Cria diretórios intermediários automaticamente se necessário.
// Quando invocada de uma aba de editor com o arquivo ativo, exibe confirmação
// com previews Antes/Depois antes de gravar (mesma política do edit_file).
type WriteFile struct {
	workDir  string
	questMgr QuestionnaireRequester
	onWrite  FileWriteObserver
}

// WriteFileOption configura integrações opcionais da tool.
type WriteFileOption func(*WriteFile)

// WithWriteFileWriteObserver registra um observador para escritas feitas pela tool.
func WithWriteFileWriteObserver(observer FileWriteObserver) WriteFileOption {
	return func(t *WriteFile) {
		t.onWrite = observer
	}
}

// WithWriteFileQuestionnaire registra o gerenciador de questionários usado para
// pedir confirmação quando a tool sobrescreve o arquivo ativo de uma aba de editor.
func WithWriteFileQuestionnaire(questMgr QuestionnaireRequester) WriteFileOption {
	return func(t *WriteFile) {
		t.questMgr = questMgr
	}
}

// NewWriteFile cria uma nova instância de WriteFile.
func NewWriteFile(workDir string, opts ...WriteFileOption) *WriteFile {
	t := &WriteFile{workDir: workDir}
	for _, opt := range opts {
		if opt != nil {
			opt(t)
		}
	}
	return t
}

func (t *WriteFile) Name() string { return "write_file" }

// CatalogMetadata declara os metadados de catálogo da tool (AEP-0077, Fase 1).
func (t *WriteFile) CatalogMetadata() tools.CatalogMetadata {
	return tools.CatalogMetadata{Category: "filesystem", Class: "edit_files", Package: "coding_edit", Risk: "write"}
}

func (t *WriteFile) Description() string {
	return "Creates or overwrites a text file with full content (no partial edits). Creates intermediate directories. Refuses binary documents (PDF, DOCX/XLSX/PPTX, ODT/ODS/ODP, EPUB) — read them with read_file, which returns a Markdown projection; CSV and RTF remain writable as text. For small edits, use edit_file."
}

func (t *WriteFile) Parameters() json.RawMessage {
	return json.RawMessage(`{
		"type": "object",
		"properties": {
			"path": {
				"type": "string",
				"description": "Caminho do arquivo a criar/sobrescrever (absoluto ou relativo ao diretório de trabalho)"
			},
			"content": {
				"type": "string",
				"description": "Conteúdo completo do arquivo"
			}
		},
		"required": ["path", "content"],
		"additionalProperties": false
	}`)
}

type writeFileArgs struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

// Limite de segurança para tamanho do conteúdo escrito
const writeMaxContentSize = 5 * 1024 * 1024 // 5MB

func (t *WriteFile) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var a writeFileArgs
	if err := json.Unmarshal(args, &a); err != nil {
		return tools.ToolResult{Content: "Erro ao parsear argumentos: " + err.Error(), IsError: true}, nil
	}

	if a.Path == "" {
		return tools.ToolResult{Content: "Parâmetro 'path' é obrigatório", IsError: true}, nil
	}

	// Limite de tamanho
	if len(a.Content) > writeMaxContentSize {
		return tools.ToolResult{
			Content: fmt.Sprintf("Conteúdo muito grande (%d bytes). Máximo permitido: %d bytes", len(a.Content), writeMaxContentSize),
			IsError: true,
		}, nil
	}

	// Resolve caminho
	fullPath, err := t.resolvePath(a.Path)
	if err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Valida segurança (toolcalling estrito)
	if err := validatePathWithPolicy(ctx, fullPath, t.workDir, ToolPolicy(), "write"); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Verifica se o arquivo já existe (para relatório)
	existed := false
	var oldSize int64
	if info, err := os.Stat(fullPath); err == nil {
		existed = true
		oldSize = info.Size()
	}

	// Verifica se é diretório
	if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
		return tools.ToolResult{
			Content: fmt.Sprintf("'%s' é um diretório, não pode ser sobrescrito como arquivo", a.Path),
			IsError: true,
		}, nil
	}

	// AEP-0093: escrita só em texto — rejeita documento existente ou conteúdo de documento
	if existed {
		if msg, ok := rejectExistingDocument(fullPath, a.Path); ok {
			return tools.ToolResult{Content: msg, IsError: true}, nil
		}
	}
	if msg, ok := rejectDocumentWriteString(a.Content, a.Path); ok {
		return tools.ToolResult{Content: msg, IsError: true}, nil
	}

	// Resolve política de confirmação baseada no contexto de invocação (AEP-0032:
	// sobrescrever o arquivo ativo do editor exige revisão humana).
	if resolveEditPolicy(ctx, fullPath) == policyConfirmWithDiff {
		before := ""
		if existed {
			prefix, err := readFilePrefixForPreview(fullPath)
			if err != nil {
				// Sem o conteúdo atual não dá para o usuário revisar o que será
				// perdido — aborta em vez de mostrar um "Antes" vazio.
				return tools.ToolResult{
					Content: fmt.Sprintf("Erro ao ler conteúdo atual para confirmação: %v", err),
					IsError: true,
				}, nil
			}
			before = prefix
		}
		if confirmed, toolResult := confirmBeforeAfter(ctx, t.questMgr, overwriteConfirmTitle(),
			a.Path, truncateForPreview(before), truncateForPreview(a.Content)); !confirmed {
			return toolResult, nil
		}
	}

	// Escreve o arquivo (criando diretórios intermediários se necessário)
	var cancelWriteMarker func(bool)
	if t.onWrite != nil {
		cancelWriteMarker = t.onWrite(fullPath)
	}
	if err := WriteFileBytes(fullPath, []byte(a.Content), 0644); err != nil {
		if cancelWriteMarker != nil {
			cancelWriteMarker(false)
		}
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao escrever arquivo: %v", err),
			IsError: true,
		}, nil
	}
	if cancelWriteMarker != nil {
		cancelWriteMarker(true)
	}

	// Conta linhas
	lineCount := strings.Count(a.Content, "\n") + 1
	if a.Content == "" {
		lineCount = 0
	}

	// Relatório
	action := "Criado"
	details := ""
	if existed {
		action = "Sobrescrito"
		details = fmt.Sprintf(" (antes: %s)", formatSize(oldSize))
	}

	return tools.ToolResult{
		Content: fmt.Sprintf("%s: %s (%d linhas, %s)%s", action, a.Path, lineCount, formatSize(int64(len(a.Content))), details),
		Metadata: map[string]any{
			"path":      a.Path,
			"action":    strings.ToLower(action),
			"lines":     lineCount,
			"bytes":     len(a.Content),
			"overwrite": existed,
		},
	}, nil
}

func (t *WriteFile) resolvePath(path string) (string, error) {
	return resolveFilePath(path, t.workDir)
}
