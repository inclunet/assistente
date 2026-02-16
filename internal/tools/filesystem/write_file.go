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

// WriteFile cria ou sobrescreve um arquivo no disco.
// Cria diretórios intermediários automaticamente se necessário.
type WriteFile struct {
	workDir string
}

// NewWriteFile cria uma nova instância de WriteFile.
func NewWriteFile(workDir string) *WriteFile {
	return &WriteFile{workDir: workDir}
}

func (t *WriteFile) Name() string { return "write_file" }

func (t *WriteFile) Description() string {
	return "Creates or overwrites a file with full content (no partial edits). Creates intermediate directories. Use for new files or full rewrites; for small edits, use edit_file."
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

	// Valida segurança
	if err := validatePath(fullPath, t.workDir); err != nil {
		return tools.ToolResult{Content: err.Error(), IsError: true}, nil
	}

	// Bloqueia escrita em extensões sensíveis do sistema
	if isSensitiveFile(fullPath) {
		return tools.ToolResult{
			Content: fmt.Sprintf("Não é permitido escrever em arquivos de sistema/configuração sensíveis: %s", a.Path),
			IsError: true,
		}, nil
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

	// Cria diretórios intermediários se necessário
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao criar diretórios: %v", err),
			IsError: true,
		}, nil
	}

	// Escreve o arquivo
	if err := os.WriteFile(fullPath, []byte(a.Content), 0644); err != nil {
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao escrever arquivo: %v", err),
			IsError: true,
		}, nil
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

// isSensitiveFile verifica se o caminho é um arquivo sensível que não deve ser modificado.
func isSensitiveFile(path string) bool {
	name := strings.ToLower(filepath.Base(path))

	// Arquivos de ambiente com secrets
	sensitiveNames := map[string]bool{
		".env":         true,
		".env.local":   true,
		".env.prod":    true,
		".env.production": true,
		"id_rsa":       true,
		"id_ed25519":   true,
		"known_hosts":  true,
		"authorized_keys": true,
	}

	if sensitiveNames[name] {
		return true
	}

	// Extensões de certificados e chaves
	ext := strings.ToLower(filepath.Ext(path))
	sensitiveExts := map[string]bool{
		".pem": true,
		".key": true,
		".crt": true,
		".cer": true,
		".p12": true,
		".pfx": true,
	}

	return sensitiveExts[ext]
}
