package agents

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"assistente/internal/filemanager"
)

// FileAgent é um agente inteligente para gerenciamento de arquivos
type FileAgent struct {
	BaseAgent
	storage          *filemanager.StorageManager  // Gerencia local + cloud providers
	security         *filemanager.SecurityValidator
	authorizedPaths  []filemanager.AuthorizedPath
	pendingDeletes   map[string]time.Time
	workingDirectory string
	defaultDirectory string
	gdrive           *filemanager.GoogleDriveProvider
	mu               sync.RWMutex
}

// NewFileAgent cria um novo FileAgent
func NewFileAgent(llmClient LLMClient, model string) *FileAgent {
	if model == "" {
		model = "gpt-4o-mini"
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "."
	}

	return &FileAgent{
		BaseAgent: BaseAgent{
			Name:        "file_manager",
			DisplayName: "File Manager",
			Description: "Gerencia arquivos no sistema local. Use para ler, escrever, buscar, editar e organizar arquivos e pastas. Pode navegar diretórios, ler conteúdo de arquivos de texto, criar e editar arquivos, e fazer buscas por nome ou conteúdo.",
			AgentType:   "internal",
			Model:       model,
			SystemPrompt: `Você é um especialista em gerenciamento de arquivos, tanto locais quanto na nuvem.

## INTERFACE UNIFICADA
As mesmas ferramentas funcionam para arquivos locais E na nuvem!
O sistema detecta automaticamente pelo formato do caminho:

**Caminhos locais Windows**: C:\docs\arquivo.txt, ./relativo/arquivo.txt
**Caminhos WSL (Linux no Windows)**: \\wsl$\Ubuntu\home\..., \\wsl.localhost\Ubuntu-24.04\...
**Google Drive**: gdrive://ID, https://docs.google.com/..., ou URLs do Drive

IMPORTANTE: Caminhos WSL (\\wsl$ e \\wsl.localhost) são PERMITIDOS! São sistemas Linux rodando no Windows.

## Suas capacidades:

### Navegação e Diretório de Trabalho:
- **get_working_directory**: Descobre o diretório de trabalho atual
- **set_working_directory**: Define um novo diretório de trabalho
- **folder_list**: Lista arquivos e pastas (local ou gdrive://)
- **folder_create**: Cria diretórios

### Leitura:
- **file_read**: Lê conteúdo de arquivos (local, .docx, .xlsx, .pdf, Google Docs...)
- **file_read_lines**: Lê range específico de linhas
- **file_info**: Obtém metadados (tamanho, tipo, datas)

### Escrita e Edição:
- **file_write**: Cria ou sobrescreve arquivos
- **file_append**: Adiciona conteúdo ao final
- **file_replace**: Substitui texto em arquivo

### Busca:
- **file_search_name**: Busca por nome (glob para local, query para cloud)
- **file_search_content**: Busca por conteúdo
- **file_grep**: Busca estruturada com contexto

### Exclusão:
- **file_delete**: Exclui arquivo (requer confirmação se não autorizado)

## Exemplos de uso:
- file_read("C:\docs\relatorio.docx") → Lê documento Word local
- file_read("\\wsl$\Ubuntu\home\user\projeto\main.go") → Lê arquivo Go no WSL
- file_read("\\wsl.localhost\Ubuntu-24.04\var\www\app.js") → Lê arquivo JS no WSL
- file_read("gdrive://1BxiMVs0XRA5...") → Lê documento do Google Drive
- file_read("https://docs.google.com/document/d/...") → Lê Google Doc via URL
- folder_list("\\wsl$\Ubuntu\home\user") → Lista pasta no WSL
- folder_list("gdrive://") → Lista raiz do Google Drive
- file_search_name("gdrive://", "*.pdf") → Busca PDFs no Drive

## REGRAS DE SEGURANÇA (OBRIGATÓRIAS):

### 🔴 Arquivos e Pastas de SISTEMA - TOTALMENTE PROIBIDOS:
NÃO É POSSÍVEL de NENHUMA forma ler, escrever, editar ou excluir arquivos em:
- C:\Windows, C:\Program Files, C:\ProgramData
- .ssh, .gnupg, .aws, .azure, .kube, .docker
- Arquivos com extensões: .dll, .sys, .exe, .bat, .cmd, .ps1, .reg, .msi

Se o usuário pedir para acessar esses arquivos, RECUSE educadamente.

### 🟡 Exclusão de Arquivos:
1. Em pastas NÃO autorizadas: SEMPRE peça confirmação explícita
2. Explique O QUE será excluído ANTES de pedir confirmação

## Diretório de Trabalho:
- Use get_working_directory para saber onde está
- Caminhos relativos são resolvidos a partir do diretório de trabalho atual
- Use set_working_directory para mudar de pasta

## Formato de resposta:
- Sempre retorne o caminho completo dos arquivos
- Mostre informações relevantes: tamanho, data, tipo, provider (local/gdrive)
- Para erros, explique claramente o que aconteceu`,
			Enabled: true,
			LLM:     llmClient,
		},
		storage:          filemanager.NewStorageManager(),
		security:         filemanager.NewSecurityValidator(nil),
		pendingDeletes:   make(map[string]time.Time),
		workingDirectory: homeDir,
		defaultDirectory: homeDir,
	}
}

// SetAuthorizedPaths configura as pastas autorizadas
func (a *FileAgent) SetAuthorizedPaths(paths []filemanager.AuthorizedPath) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.authorizedPaths = paths
	a.security.SetAuthorizedPaths(paths)
}

// SetGoogleTokenProvider configura a função para obter token OAuth do Google
// Esta função é chamada pelo App quando uma conexão OAuth do Google está disponível
func (a *FileAgent) SetGoogleTokenProvider(tokenProvider func() (string, error)) {
	a.mu.Lock()
	defer a.mu.Unlock()
	
	// Cria ou atualiza o provider do Google Drive
	a.gdrive = filemanager.NewGoogleDriveProvider(tokenProvider)
	
	if tokenProvider != nil {
		// Registra o provider no StorageManager
		a.storage.RegisterProvider(a.gdrive)
	}
}

// IsGoogleDocsEnabled verifica se o suporte a Google Docs está habilitado
func (a *FileAgent) IsGoogleDocsEnabled() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.gdrive != nil && a.gdrive.IsAvailable()
}

// Execute recebe uma tarefa em linguagem natural e usa o LLM para decidir como resolver
func (a *FileAgent) Execute(ctx context.Context, task string) (string, error) {
	if a.LLM == nil {
		return "", fmt.Errorf("LLM client não configurado para o agente %s", a.Name)
	}

	fmt.Printf("📁 [File Agent] Recebeu tarefa: %s\n", task)

	// Inclui o diretório de trabalho atual no contexto
	enhancedTask := fmt.Sprintf("[Diretório de trabalho atual: %s]\n\n%s", a.GetWorkingDirectory(), task)

	// Usa o método com saver se disponível
	var result string
	var err error
	if a.MessageSaver != nil {
		result, err = a.LLM.ChatWithToolsAndSaver(
			ctx,
			a.Model,
			a.SystemPrompt,
			enhancedTask,
			a.GetTools(),
			a.ExecuteTool,
			a.Name,
			a.MessageSaver,
		)
	} else {
		result, err = a.LLM.ChatWithTools(
			ctx,
			a.Model,
			a.SystemPrompt,
			enhancedTask,
			a.GetTools(),
			a.ExecuteTool,
		)
	}

	if err != nil {
		return "", fmt.Errorf("erro no File Agent: %w", err)
	}

	fmt.Printf("✅ [File Agent] Resposta: %s\n", truncate(result, 100))
	return result, nil
}

// GetWorkingDirectory retorna o diretório de trabalho atual
func (a *FileAgent) GetWorkingDirectory() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.workingDirectory
}

// SetWorkingDirectory define um novo diretório de trabalho
func (a *FileAgent) SetWorkingDirectory(path string) error {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("caminho inválido: %w", err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("diretório não encontrado: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("o caminho não é um diretório: %s", absPath)
	}

	// Verifica se não é uma pasta protegida
	if err := a.security.ValidatePathForOperation(absPath, filemanager.OpRead); err != nil {
		return fmt.Errorf("não é possível usar pasta protegida como diretório de trabalho")
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	a.workingDirectory = absPath
	return nil
}

// ClearWorkingDirectory reseta para o diretório padrão
func (a *FileAgent) ClearWorkingDirectory() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.workingDirectory = a.defaultDirectory
}

// ResolvePath resolve um caminho relativo usando o diretório de trabalho
func (a *FileAgent) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return filepath.Join(a.workingDirectory, path)
}

// GetTools retorna as tools disponíveis do FileAgent
func (a *FileAgent) GetTools() []Tool {
	return []Tool{
		// Navegação
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "get_working_directory",
				Description: "Retorna o diretório de trabalho atual. Use para saber onde o agente está operando.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "set_working_directory",
				Description: "Define um novo diretório de trabalho. Caminhos relativos serão resolvidos a partir deste diretório.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Caminho do novo diretório de trabalho",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "folder_list",
				Description: "Lista arquivos e pastas em um diretório",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Caminho do diretório (relativo ou absoluto). Use '.' para diretório atual.",
						},
						"show_hidden": map[string]interface{}{
							"type":        "boolean",
							"description": "Incluir arquivos ocultos. Default: false",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "folder_create",
				Description: "Cria um novo diretório, incluindo diretórios intermediários",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Caminho do diretório a criar",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		// Leitura
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "file_read",
				Description: "Lê o conteúdo completo de um arquivo de texto",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Caminho do arquivo",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "file_read_lines",
				Description: "Lê um range específico de linhas de um arquivo. Útil para detalhar resultados de busca.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Caminho do arquivo",
						},
						"start_line": map[string]interface{}{
							"type":        "integer",
							"description": "Linha inicial (1-indexed)",
						},
						"end_line": map[string]interface{}{
							"type":        "integer",
							"description": "Linha final (1-indexed). Default: start_line + 20",
						},
					},
					"required": []string{"path", "start_line"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "file_info",
				Description: "Obtém informações detalhadas sobre um arquivo ou pasta",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Caminho do arquivo ou pasta",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		// Escrita
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "file_write",
				Description: "Cria ou sobrescreve um arquivo com o conteúdo fornecido",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Caminho do arquivo",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Conteúdo a ser escrito",
						},
						"create_dirs": map[string]interface{}{
							"type":        "boolean",
							"description": "Criar diretórios intermediários se não existirem. Default: true",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "file_append",
				Description: "Adiciona conteúdo ao final de um arquivo",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Caminho do arquivo",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Conteúdo a ser adicionado",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "file_replace",
				Description: "Substitui texto em um arquivo",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Caminho do arquivo",
						},
						"old_text": map[string]interface{}{
							"type":        "string",
							"description": "Texto a ser substituído",
						},
						"new_text": map[string]interface{}{
							"type":        "string",
							"description": "Novo texto",
						},
						"replace_all": map[string]interface{}{
							"type":        "boolean",
							"description": "Substituir todas as ocorrências. Default: false",
						},
					},
					"required": []string{"path", "old_text", "new_text"},
				},
			},
		},
		// Busca
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "file_search_name",
				Description: "Busca arquivos por nome usando padrões glob (ex: *.txt, report*.pdf)",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"directory": map[string]interface{}{
							"type":        "string",
							"description": "Diretório base para busca",
						},
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "Padrão de busca glob (ex: *.txt, **/*.go)",
						},
						"recursive": map[string]interface{}{
							"type":        "boolean",
							"description": "Buscar em subdiretórios. Default: true",
						},
						"max_results": map[string]interface{}{
							"type":        "integer",
							"description": "Limite de resultados. Default: 100",
						},
					},
					"required": []string{"directory", "pattern"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "file_search_content",
				Description: "Busca arquivos que contêm determinado texto",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"directory": map[string]interface{}{
							"type":        "string",
							"description": "Diretório base para busca",
						},
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Texto a buscar",
						},
						"file_pattern": map[string]interface{}{
							"type":        "string",
							"description": "Filtrar por tipo de arquivo (ex: *.go, *.txt). Default: todos",
						},
						"case_sensitive": map[string]interface{}{
							"type":        "boolean",
							"description": "Busca sensível a maiúsculas. Default: false",
						},
						"max_results": map[string]interface{}{
							"type":        "integer",
							"description": "Limite de resultados. Default: 50",
						},
					},
					"required": []string{"directory", "query"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "file_grep",
				Description: "Busca estruturada em arquivos retornando path, linha, coluna e contexto. Ideal para encontrar e depois editar.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"directory": map[string]interface{}{
							"type":        "string",
							"description": "Diretório base para busca",
						},
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Termo ou expressão a buscar",
						},
						"is_regex": map[string]interface{}{
							"type":        "boolean",
							"description": "Tratar query como expressão regular. Default: false",
						},
						"file_pattern": map[string]interface{}{
							"type":        "string",
							"description": "Filtrar por tipo de arquivo (ex: *.go). Default: todos",
						},
						"context_lines": map[string]interface{}{
							"type":        "integer",
							"description": "Linhas de contexto antes e depois. Default: 2",
						},
						"max_files": map[string]interface{}{
							"type":        "integer",
							"description": "Limite de arquivos. Default: 50",
						},
					},
					"required": []string{"directory", "query"},
				},
			},
		},
		// Exclusão
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "file_delete",
				Description: "Exclui um arquivo. Requer confirmação se a pasta não estiver autorizada.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Caminho do arquivo a excluir",
						},
						"confirm": map[string]interface{}{
							"type":        "boolean",
							"description": "Confirmação explícita da exclusão",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		// Autorização de pastas
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "authorize_folder",
				Description: "Autoriza uma pasta para operações de exclusão sem confirmação. Útil para pastas de trabalho temporárias.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Caminho da pasta a autorizar",
						},
						"recursive": map[string]interface{}{
							"type":        "boolean",
							"description": "Aplicar autorização a subpastas também. Default: true",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "revoke_folder_authorization",
				Description: "Remove autorização de uma pasta. Exclusões passarão a exigir confirmação novamente.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Caminho da pasta para revogar autorização",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name:        "list_authorized_folders",
				Description: "Lista todas as pastas autorizadas para operações de exclusão.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
	}
}

// CanHandle verifica se o agente pode executar a tool
func (a *FileAgent) CanHandle(toolName string) bool {
	return strings.HasPrefix(toolName, "file_") ||
		strings.HasPrefix(toolName, "folder_") ||
		strings.HasPrefix(toolName, "get_working_directory") ||
		strings.HasPrefix(toolName, "set_working_directory") ||
		strings.HasPrefix(toolName, "authorize_folder") ||
		strings.HasPrefix(toolName, "revoke_folder_authorization") ||
		strings.HasPrefix(toolName, "list_authorized_folders")
}

// ExecuteTool executa uma tool do FileAgent
func (a *FileAgent) ExecuteTool(toolCall ToolCall) (string, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("erro ao parsear argumentos: %v", err)
	}

	switch toolCall.Function.Name {
	// Navegação
	case "get_working_directory":
		return a.executeGetWorkingDirectory()
	case "set_working_directory":
		return a.executeSetWorkingDirectory(args)
	case "folder_list":
		return a.executeFolderList(args)
	case "folder_create":
		return a.executeFolderCreate(args)

	// Leitura
	case "file_read":
		return a.executeFileRead(args)
	case "file_read_lines":
		return a.executeFileReadLines(args)
	case "file_info":
		return a.executeFileInfo(args)

	// Escrita
	case "file_write":
		return a.executeFileWrite(args)
	case "file_append":
		return a.executeFileAppend(args)
	case "file_replace":
		return a.executeFileReplace(args)

	// Busca
	case "file_search_name":
		return a.executeFileSearchName(args)
	case "file_search_content":
		return a.executeFileSearchContent(args)
	case "file_grep":
		return a.executeFileGrep(args)

	// Exclusão
	case "file_delete":
		return a.executeFileDelete(args)

	// Autorização
	case "authorize_folder":
		return a.executeAuthorizeFolder(args)
	case "revoke_folder_authorization":
		return a.executeRevokeFolderAuthorization(args)
	case "list_authorized_folders":
		return a.executeListAuthorizedFolders()

	default:
		return "", fmt.Errorf("tool desconhecida: %s", toolCall.Function.Name)
	}
}

// ===== Implementação das Tools =====

func (a *FileAgent) executeGetWorkingDirectory() (string, error) {
	result := map[string]interface{}{
		"working_directory": a.GetWorkingDirectory(),
		"is_default":        a.GetWorkingDirectory() == a.defaultDirectory,
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeSetWorkingDirectory(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return `{"error": "Caminho é obrigatório"}`, nil
	}

	resolvedPath := a.ResolvePath(path)
	previousDir := a.GetWorkingDirectory()

	if err := a.SetWorkingDirectory(resolvedPath); err != nil {
		return fmt.Sprintf(`{"error": "%s"}`, err.Error()), nil
	}

	result := map[string]interface{}{
		"success":            true,
		"previous_directory": previousDir,
		"new_directory":      a.GetWorkingDirectory(),
		"message":            "Diretório de trabalho alterado com sucesso",
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeFolderList(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}
	showHidden, _ := args["show_hidden"].(bool)

	// Detecta se é caminho local ou cloud
	var resolvedPath string
	if a.storage.IsCloudPath(path) {
		resolvedPath = path
	} else {
		resolvedPath = a.ResolvePath(path)
		// Validação de segurança apenas para arquivos locais
		if err := a.security.ValidatePathForOperation(resolvedPath, filemanager.OpList); err != nil {
			return fmt.Sprintf(`{"error": "Acesso negado: %s"}`, err.Error()), nil
		}
	}

	entries, err := a.storage.ListDirectory(context.Background(), resolvedPath, filemanager.ListOptions{
		ShowHidden: showHidden,
	})
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao listar diretório: %s"}`, err.Error()), nil
	}

	result := map[string]interface{}{
		"path":    resolvedPath,
		"entries": entries,
		"count":   len(entries),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeFolderCreate(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return `{"error": "Caminho é obrigatório"}`, nil
	}

	resolvedPath := a.ResolvePath(path)

	if err := a.security.ValidatePathForOperation(resolvedPath, filemanager.OpWrite); err != nil {
		return fmt.Sprintf(`{"error": "Acesso negado: %s"}`, err.Error()), nil
	}

	if err := os.MkdirAll(resolvedPath, 0755); err != nil {
		return fmt.Sprintf(`{"error": "Erro ao criar diretório: %s"}`, err.Error()), nil
	}

	result := map[string]interface{}{
		"success": true,
		"path":    resolvedPath,
		"message": "Diretório criado com sucesso",
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeFileRead(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return `{"error": "Caminho é obrigatório"}`, nil
	}

	// Detecta se é caminho local ou cloud e resolve adequadamente
	var resolvedPath string
	if a.storage.IsCloudPath(path) {
		resolvedPath = path // Cloud paths não precisam de resolução relativa
	} else {
		resolvedPath = a.ResolvePath(path)
		// Validação de segurança apenas para arquivos locais
		if err := a.security.ValidatePathForOperation(resolvedPath, filemanager.OpRead); err != nil {
			return fmt.Sprintf(`{"error": "Acesso negado: %s"}`, err.Error()), nil
		}
	}

	content, err := a.storage.ReadFile(context.Background(), resolvedPath, filemanager.ReadOptions{})
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao ler arquivo: %s"}`, err.Error()), nil
	}

	result := map[string]interface{}{
		"path":       resolvedPath,
		"content":    content.Text,
		"encoding":   content.Encoding,
		"line_count": content.LineCount,
	}
	
	// Adiciona dados extras para documentos estruturados
	if len(content.Sheets) > 0 {
		result["sheets"] = content.Sheets
	}
	if len(content.Slides) > 0 {
		result["slides"] = content.Slides
	}
	if len(content.Links) > 0 {
		result["links"] = content.Links
	}
	if content.Metadata != nil {
		result["metadata"] = content.Metadata
	}
	
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeFileReadLines(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return `{"error": "Caminho é obrigatório"}`, nil
	}

	startLine := 1
	if sl, ok := args["start_line"].(float64); ok {
		startLine = int(sl)
	}
	endLine := startLine + 20
	if el, ok := args["end_line"].(float64); ok {
		endLine = int(el)
	}

	resolvedPath := a.ResolvePath(path)

	if err := a.security.ValidatePathForOperation(resolvedPath, filemanager.OpRead); err != nil {
		return fmt.Sprintf(`{"error": "Acesso negado: %s"}`, err.Error()), nil
	}

	file, err := os.Open(resolvedPath)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao abrir arquivo: %s"}`, err.Error()), nil
	}
	defer file.Close()

	var lines []filemanager.LineInfo
	var rawText strings.Builder
	scanner := bufio.NewScanner(file)
	lineNum := 0
	totalLines := 0

	for scanner.Scan() {
		totalLines++
		lineNum++
		if lineNum >= startLine && lineNum <= endLine {
			text := scanner.Text()
			lines = append(lines, filemanager.LineInfo{Number: lineNum, Text: text})
			rawText.WriteString(text + "\n")
		}
	}

	result := filemanager.LinesResult{
		Path:       resolvedPath,
		StartLine:  startLine,
		EndLine:    endLine,
		TotalLines: totalLines,
		Content:    lines,
		RawText:    rawText.String(),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeFileInfo(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return `{"error": "Caminho é obrigatório"}`, nil
	}

	// Detecta se é caminho local ou cloud
	var resolvedPath string
	if a.storage.IsCloudPath(path) {
		resolvedPath = path
	} else {
		resolvedPath = a.ResolvePath(path)
		// Validação de segurança apenas para arquivos locais
		if err := a.security.ValidatePathForOperation(resolvedPath, filemanager.OpInfo); err != nil {
			return fmt.Sprintf(`{"error": "Acesso negado: %s"}`, err.Error()), nil
		}
	}

	info, err := a.storage.GetFileInfo(context.Background(), resolvedPath)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao obter informações: %s"}`, err.Error()), nil
	}

	jsonResult, _ := json.Marshal(info)
	return string(jsonResult), nil
}

func (a *FileAgent) executeFileWrite(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" || content == "" {
		return `{"error": "Caminho e conteúdo são obrigatórios"}`, nil
	}

	createDirs := true
	if cd, ok := args["create_dirs"].(bool); ok {
		createDirs = cd
	}

	resolvedPath := a.ResolvePath(path)

	if err := a.security.ValidatePathForOperation(resolvedPath, filemanager.OpWrite); err != nil {
		return fmt.Sprintf(`{"error": "Acesso negado: %s"}`, err.Error()), nil
	}

	// Cria diretórios se necessário
	if createDirs {
		dir := filepath.Dir(resolvedPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Sprintf(`{"error": "Erro ao criar diretórios: %s"}`, err.Error()), nil
		}
	}

	if err := os.WriteFile(resolvedPath, []byte(content), 0644); err != nil {
		return fmt.Sprintf(`{"error": "Erro ao escrever arquivo: %s"}`, err.Error()), nil
	}

	result := map[string]interface{}{
		"success": true,
		"path":    resolvedPath,
		"size":    len(content),
		"message": "Arquivo escrito com sucesso",
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeFileAppend(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	content, _ := args["content"].(string)
	if path == "" || content == "" {
		return `{"error": "Caminho e conteúdo são obrigatórios"}`, nil
	}

	resolvedPath := a.ResolvePath(path)

	if err := a.security.ValidatePathForOperation(resolvedPath, filemanager.OpWrite); err != nil {
		return fmt.Sprintf(`{"error": "Acesso negado: %s"}`, err.Error()), nil
	}

	file, err := os.OpenFile(resolvedPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao abrir arquivo: %s"}`, err.Error()), nil
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		return fmt.Sprintf(`{"error": "Erro ao escrever: %s"}`, err.Error()), nil
	}

	result := map[string]interface{}{
		"success":       true,
		"path":          resolvedPath,
		"bytes_written": len(content),
		"message":       "Conteúdo adicionado com sucesso",
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeFileReplace(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	oldText, _ := args["old_text"].(string)
	newText, _ := args["new_text"].(string)
	if path == "" || oldText == "" {
		return `{"error": "Caminho e texto antigo são obrigatórios"}`, nil
	}

	replaceAll, _ := args["replace_all"].(bool)
	resolvedPath := a.ResolvePath(path)

	if err := a.security.ValidatePathForOperation(resolvedPath, filemanager.OpWrite); err != nil {
		return fmt.Sprintf(`{"error": "Acesso negado: %s"}`, err.Error()), nil
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro ao ler arquivo: %s"}`, err.Error()), nil
	}

	content := string(data)
	var count int

	if replaceAll {
		count = strings.Count(content, oldText)
		content = strings.ReplaceAll(content, oldText, newText)
	} else {
		if strings.Contains(content, oldText) {
			content = strings.Replace(content, oldText, newText, 1)
			count = 1
		}
	}

	if count == 0 {
		return `{"success": false, "message": "Texto não encontrado no arquivo", "replacements": 0}`, nil
	}

	if err := os.WriteFile(resolvedPath, []byte(content), 0644); err != nil {
		return fmt.Sprintf(`{"error": "Erro ao escrever arquivo: %s"}`, err.Error()), nil
	}

	result := map[string]interface{}{
		"success":      true,
		"path":         resolvedPath,
		"replacements": count,
		"message":      fmt.Sprintf("%d substituição(ões) realizada(s)", count),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeFileSearchName(args map[string]interface{}) (string, error) {
	directory, _ := args["directory"].(string)
	pattern, _ := args["pattern"].(string)
	if directory == "" || pattern == "" {
		return `{"error": "Diretório e padrão são obrigatórios"}`, nil
	}

	maxResults := 100
	if mr, ok := args["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	// Detecta se é caminho local ou cloud
	var resolvedDir string
	if a.storage.IsCloudPath(directory) {
		resolvedDir = directory
	} else {
		resolvedDir = a.ResolvePath(directory)
		// Validação de segurança apenas para arquivos locais
		if err := a.security.ValidatePathForOperation(resolvedDir, filemanager.OpList); err != nil {
			return fmt.Sprintf(`{"error": "Acesso negado: %s"}`, err.Error()), nil
		}
	}

	files, err := a.storage.SearchByName(context.Background(), resolvedDir, pattern, filemanager.SearchOptions{
		MaxResults: maxResults,
	})
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro na busca: %s"}`, err.Error()), nil
	}

	result := map[string]interface{}{
		"directory": resolvedDir,
		"pattern":   pattern,
		"results":   files,
		"count":     len(files),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeFileSearchContent(args map[string]interface{}) (string, error) {
	directory, _ := args["directory"].(string)
	query, _ := args["query"].(string)
	if directory == "" || query == "" {
		return `{"error": "Diretório e query são obrigatórios"}`, nil
	}

	caseSensitive, _ := args["case_sensitive"].(bool)
	maxResults := 50
	if mr, ok := args["max_results"].(float64); ok {
		maxResults = int(mr)
	}

	// Detecta se é caminho local ou cloud
	var resolvedDir string
	if a.storage.IsCloudPath(directory) {
		resolvedDir = directory
	} else {
		resolvedDir = a.ResolvePath(directory)
		// Validação de segurança apenas para arquivos locais
		if err := a.security.ValidatePathForOperation(resolvedDir, filemanager.OpList); err != nil {
			return fmt.Sprintf(`{"error": "Acesso negado: %s"}`, err.Error()), nil
		}
	}

	results, err := a.storage.SearchByContent(context.Background(), resolvedDir, query, filemanager.SearchOptions{
		CaseSensitive: caseSensitive,
		MaxResults:    maxResults,
	})
	if err != nil {
		return fmt.Sprintf(`{"error": "Erro na busca: %s"}`, err.Error()), nil
	}

	result := map[string]interface{}{
		"directory": resolvedDir,
		"query":     query,
		"results":   results,
		"count":     len(results),
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeFileGrep(args map[string]interface{}) (string, error) {
	directory, _ := args["directory"].(string)
	query, _ := args["query"].(string)
	if directory == "" || query == "" {
		return `{"error": "Diretório e query são obrigatórios"}`, nil
	}

	isRegex, _ := args["is_regex"].(bool)
	filePattern, _ := args["file_pattern"].(string)
	if filePattern == "" {
		filePattern = "*"
	}
	contextLines := 2
	if cl, ok := args["context_lines"].(float64); ok {
		contextLines = int(cl)
	}
	maxFiles := 50
	if mf, ok := args["max_files"].(float64); ok {
		maxFiles = int(mf)
	}

	resolvedDir := a.ResolvePath(directory)

	if err := a.security.ValidatePathForOperation(resolvedDir, filemanager.OpList); err != nil {
		return fmt.Sprintf(`{"error": "Acesso negado: %s"}`, err.Error()), nil
	}

	var searchFunc func(string) (bool, []int)
	if isRegex {
		re, err := regexp.Compile(query)
		if err != nil {
			return fmt.Sprintf(`{"error": "Regex inválida: %s"}`, err.Error()), nil
		}
		searchFunc = func(line string) (bool, []int) {
			loc := re.FindStringIndex(line)
			if loc != nil {
				return true, loc
			}
			return false, nil
		}
	} else {
		lowerQuery := strings.ToLower(query)
		searchFunc = func(line string) (bool, []int) {
			idx := strings.Index(strings.ToLower(line), lowerQuery)
			if idx >= 0 {
				return true, []int{idx, idx + len(query)}
			}
			return false, nil
		}
	}

	var results []map[string]interface{}
	filesSearched := 0
	totalMatches := 0

	filepath.Walk(resolvedDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}

		if filesSearched >= maxFiles {
			return filepath.SkipAll
		}

		// Verifica padrão de arquivo
		if filePattern != "*" {
			matched, _ := filepath.Match(filePattern, info.Name())
			if !matched {
				return nil
			}
		}

		// Só busca em arquivos de texto
		if !filemanager.IsTextFile(path) {
			return nil
		}

		// Verifica segurança
		if err := a.security.ValidatePathForOperation(path, filemanager.OpRead); err != nil {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		lines := strings.Split(string(data), "\n")
		var fileMatches []map[string]interface{}

		for i, line := range lines {
			found, loc := searchFunc(line)
			if found {
				// Contexto antes
				var before []string
				for j := i - contextLines; j < i; j++ {
					if j >= 0 {
						before = append(before, lines[j])
					}
				}

				// Contexto depois
				var after []string
				for j := i + 1; j <= i+contextLines && j < len(lines); j++ {
					after = append(after, lines[j])
				}

				fileMatches = append(fileMatches, map[string]interface{}{
					"line_number":    i + 1,
					"column_start":   loc[0] + 1,
					"column_end":     loc[1] + 1,
					"line_content":   line,
					"context_before": before,
					"context_after":  after,
				})
				totalMatches++
			}
		}

		if len(fileMatches) > 0 {
			results = append(results, map[string]interface{}{
				"file": map[string]interface{}{
					"path": path,
					"name": info.Name(),
					"size": info.Size(),
				},
				"matches": fileMatches,
			})
			filesSearched++
		}

		return nil
	})

	result := map[string]interface{}{
		"query":         query,
		"directory":     resolvedDir,
		"total_files":   len(results),
		"total_matches": totalMatches,
		"results":       results,
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeFileDelete(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return `{"error": "Caminho é obrigatório"}`, nil
	}

	confirm, _ := args["confirm"].(bool)
	resolvedPath := a.ResolvePath(path)

	// Verifica segurança
	if err := a.security.ValidatePathForOperation(resolvedPath, filemanager.OpDelete); err != nil {
		if err == filemanager.ErrDeleteNotAllowed {
			// Pasta não autorizada - precisa de confirmação
			if !confirm {
				return fmt.Sprintf(`{"requires_confirmation": true, "path": "%s", "message": "Para excluir este arquivo, confirme a operação passando confirm=true"}`, resolvedPath), nil
			}
		} else {
			return fmt.Sprintf(`{"error": "Acesso negado: %s"}`, err.Error()), nil
		}
	}

	// Verifica se arquivo existe
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return fmt.Sprintf(`{"error": "Arquivo não encontrado: %s"}`, err.Error()), nil
	}

	// Não permite excluir diretórios por esta tool
	if info.IsDir() {
		return `{"error": "Use folder_delete para excluir diretórios"}`, nil
	}

	if err := os.Remove(resolvedPath); err != nil {
		return fmt.Sprintf(`{"error": "Erro ao excluir: %s"}`, err.Error()), nil
	}

	result := map[string]interface{}{
		"success": true,
		"path":    resolvedPath,
		"message": "Arquivo excluído com sucesso",
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

// ===== Tools de Autorização =====

// OnAuthorizationChange é um callback chamado quando pastas autorizadas mudam
// Deve ser configurado pelo app para persistir no banco de dados
var OnAuthorizationChange func(paths []filemanager.AuthorizedPath)

func (a *FileAgent) executeAuthorizeFolder(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return `{"error": "Caminho é obrigatório"}`, nil
	}

	recursive := true
	if r, ok := args["recursive"].(bool); ok {
		recursive = r
	}

	resolvedPath := a.ResolvePath(path)

	// Verifica se é um diretório válido
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return fmt.Sprintf(`{"error": "Pasta não encontrada: %s"}`, err.Error()), nil
	}
	if !info.IsDir() {
		return `{"error": "O caminho não é um diretório"}`, nil
	}

	// Verifica se não é uma pasta protegida
	if err := a.security.ValidatePathForOperation(resolvedPath, filemanager.OpRead); err != nil {
		return fmt.Sprintf(`{"error": "Não é possível autorizar pasta protegida: %s"}`, err.Error()), nil
	}

	// Verifica se já está autorizada
	a.mu.Lock()
	for _, ap := range a.authorizedPaths {
		if strings.EqualFold(ap.Path, resolvedPath) {
			a.mu.Unlock()
			return fmt.Sprintf(`{"already_authorized": true, "path": "%s", "message": "Pasta já estava autorizada"}`, resolvedPath), nil
		}
	}

	// Adiciona nova autorização
	newAuth := filemanager.AuthorizedPath{
		Path:        resolvedPath,
		AllowDelete: true,
		AllowWrite:  true,
		Recursive:   recursive,
		CreatedAt:   time.Now(),
	}
	a.authorizedPaths = append(a.authorizedPaths, newAuth)
	a.security.SetAuthorizedPaths(a.authorizedPaths)
	a.mu.Unlock()

	// Notifica callback para persistir
	if OnAuthorizationChange != nil {
		OnAuthorizationChange(a.authorizedPaths)
	}

	result := map[string]interface{}{
		"success":   true,
		"path":      resolvedPath,
		"recursive": recursive,
		"message":   "Pasta autorizada para exclusão sem confirmação",
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeRevokeFolderAuthorization(args map[string]interface{}) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return `{"error": "Caminho é obrigatório"}`, nil
	}

	resolvedPath := a.ResolvePath(path)

	a.mu.Lock()
	found := false
	newPaths := make([]filemanager.AuthorizedPath, 0)
	for _, ap := range a.authorizedPaths {
		if strings.EqualFold(ap.Path, resolvedPath) {
			found = true
			continue
		}
		newPaths = append(newPaths, ap)
	}

	if !found {
		a.mu.Unlock()
		return fmt.Sprintf(`{"not_found": true, "path": "%s", "message": "Pasta não estava autorizada"}`, resolvedPath), nil
	}

	a.authorizedPaths = newPaths
	a.security.SetAuthorizedPaths(a.authorizedPaths)
	a.mu.Unlock()

	// Notifica callback para persistir
	if OnAuthorizationChange != nil {
		OnAuthorizationChange(a.authorizedPaths)
	}

	result := map[string]interface{}{
		"success": true,
		"path":    resolvedPath,
		"message": "Autorização revogada. Exclusões nesta pasta exigirão confirmação.",
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

func (a *FileAgent) executeListAuthorizedFolders() (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	folders := make([]map[string]interface{}, 0)
	for _, ap := range a.authorizedPaths {
		folders = append(folders, map[string]interface{}{
			"path":         ap.Path,
			"allow_delete": ap.AllowDelete,
			"allow_write":  ap.AllowWrite,
			"recursive":    ap.Recursive,
			"created_at":   ap.CreatedAt.Format(time.RFC3339),
		})
	}

	result := map[string]interface{}{
		"count":   len(folders),
		"folders": folders,
	}
	jsonResult, _ := json.Marshal(result)
	return string(jsonResult), nil
}

// GetAuthorizedPaths retorna as pastas autorizadas (para persistência)
func (a *FileAgent) GetAuthorizedPaths() []filemanager.AuthorizedPath {
	a.mu.RLock()
	defer a.mu.RUnlock()
	result := make([]filemanager.AuthorizedPath, len(a.authorizedPaths))
	copy(result, a.authorizedPaths)
	return result
}
