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
	storage          *filemanager.StorageManager // Gerencia local + cloud providers
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
			Name:         "file_manager",
			DisplayName:  "File Manager",
			Description:  fileAgentDescription(),
			AgentType:    "internal",
			Model:        model,
			SystemPrompt: fileAgentSystemPrompt(),
			Enabled:      true,
			LLM:          llmClient,
		},
		storage:          filemanager.NewStorageManager(),
		security:         filemanager.NewSecurityValidator(nil),
		pendingDeletes:   make(map[string]time.Time),
		workingDirectory: homeDir,
		defaultDirectory: homeDir,
	}
}

// fileAgentDescription retorna a descrição para delegação do orquestrador
func fileAgentDescription() string {
	return NewDelegationDescription("File Manager", "Manages files on local filesystem, WSL, and Google Drive.").
		Capabilities(
			"Read files: text, documents (.docx, .xlsx, .pdf), Google Docs/Sheets",
			"Write/edit files: create, overwrite, append, replace text",
			"Search: by filename (glob patterns) or by content (grep-like)",
			"Navigate: list directories, get file info, change working directory",
			"Delete files (with confirmation for non-authorized folders)",
		).
		DelegateWhen(
			"Read, write, edit, or delete files",
			"Search files by name or content",
			"List directory contents or navigate folders",
			"Access Google Drive documents or local files",
			"Get file metadata (size, type, dates)",
			"User mentions file paths, folders, or documents",
		).
		DontDelegateWhen(
			"User asks general questions about file formats (answer directly)",
			"Task involves only explaining how files work",
			"No file operation is actually needed",
		).
		Build()
}

// fileAgentSystemPrompt returns the reduced system prompt (specific instructions moved to tools)
func fileAgentSystemPrompt() string {
	return `You are a file management specialist. Use the available tools to help users manage files.

PATH FORMATS (auto-detected):
- Local Windows: C:\path\file.txt or ./relative/path
- WSL paths: \\wsl$\Ubuntu\... or \\wsl.localhost\...  (ALLOWED - Linux on Windows)
- Google Drive: gdrive://ID or https://docs.google.com/...

SECURITY (enforced by tools):
- System folders (C:\Windows, Program Files) are blocked
- Sensitive folders (.ssh, .aws, .docker) are blocked
- Dangerous extensions (.exe, .dll, .bat) are blocked
- Deletions in non-authorized folders require confirmation

RESPONSE FORMAT:
- Always return full resolved paths
- Include relevant metadata (size, type, dates)
- Explain errors clearly

=== OTHER KNOWLEDGE SOURCES ===
If user is looking for documented information (not files), suggest checking:
- FAQ Manager: Procedures, guides, and technical documentation
- Memory Manager: User preferences and personal context
- Custom Agents (HTTP/MCP): The user may have configured additional agents for:
  * Internal knowledge bases (wikis, documentation portals)
  * Search engines or internal search portals
  * Internal systems with relevant data (CRMs, ERPs, databases)
  * External APIs with useful information

When responding without finding information, mention that other agents may be available.`
}

// GetDelegationDescription retorna descrição otimizada para o orquestrador decidir delegação
func (a *FileAgent) GetDelegationDescription() string {
	return a.Description
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
	// Descrição comum para parâmetros de path
	pathDesc := NewParamDescription("File or directory path").
		Formats(
			"absolute (C:\\docs\\file.txt)",
			"relative (./file.txt) - resolved from working directory",
			"WSL (\\\\wsl$\\Ubuntu\\... or \\\\wsl.localhost\\...)",
			"Google Drive (gdrive://ID or https://docs.google.com/...)",
		).Build()

	pathDescLocal := NewParamDescription("File or directory path (local only)").
		Formats(
			"absolute (C:\\docs\\file.txt)",
			"relative (./file.txt)",
			"WSL (\\\\wsl$\\Ubuntu\\...)",
		).Build()

	return []Tool{
		// ===== NAVIGATION =====
		{
			Type: "function",
			Function: ToolFunction{
				Name: "get_working_directory",
				Description: NewToolDescription("Returns the current working directory where relative paths are resolved from.").
					WhenToUse(
						"Before using relative paths to know the base location",
						"To verify current location before navigation",
					).
					Returns("JSON with working_directory (string) and is_default (boolean)").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{}, nil),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "set_working_directory",
				Description: NewToolDescription("Changes the working directory. All relative paths will resolve from this location.").
					WhenToUse(
						"Before working with multiple files in the same folder",
						"To simplify paths when user specifies a project folder",
					).
					WhenNotToUse(
						"For one-off file operations (just use full path)",
					).
					Returns("JSON with success, previous_directory, new_directory").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"path": JSONSchemaString(pathDescLocal),
					},
					[]string{"path"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "folder_list",
				Description: NewToolDescription("Lists files and folders in a directory with metadata (size, type, dates).").
					WhenToUse(
						"User wants to see directory contents",
						"Need to find files before reading them",
						"Exploring folder structure",
					).
					WhenNotToUse(
						"Looking for specific file by name - use file_search_name instead",
						"Looking for files containing specific text - use file_search_content",
					).
					Returns("JSON with path, entries (array of file info), count").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"path": JSONSchemaString(
							NewParamDescription("Directory to list").
								Formats("absolute", "relative", "WSL", "gdrive://").
								Examples(".", "C:\\Users", "gdrive://", "\\\\wsl$\\Ubuntu\\home").
								Default("current working directory").
								Build(),
						),
						"show_hidden": JSONSchemaBool("Include hidden files (starting with dot). Default: false"),
					},
					[]string{"path"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "folder_create",
				Description: NewToolDescription("Creates a new directory, including any intermediate directories needed.").
					WhenToUse("Creating new folder structure", "Before writing file to non-existent directory").
					Returns("JSON with success, path, message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"path": JSONSchemaString(pathDescLocal),
					},
					[]string{"path"},
				),
			},
		},

		// ===== READING =====
		{
			Type: "function",
			Function: ToolFunction{
				Name: "file_read",
				Description: NewToolDescription("Reads complete file content. Supports text files, Office documents (.docx, .xlsx, .pdf), and Google Docs.").
					WhenToUse(
						"Reading text files (.txt, .json, .go, .md, .yaml, .xml, etc.)",
						"Reading Office documents (.docx, .xlsx, .pdf)",
						"Reading Google Docs/Sheets via URL or gdrive:// prefix",
						"File is reasonably sized (< 10MB or < 10000 lines)",
					).
					WhenNotToUse(
						"Binary files (.exe, .dll, images) - use file_info to check type first",
						"Very large files (> 10000 lines) - use file_read_lines with specific range",
						"Just need file metadata - use file_info instead",
					).
					Notes(
						"For spreadsheets, returns 'sheets' array with sheet names and data",
						"For documents with links, returns 'links' array",
					).
					Returns("JSON with path, content, encoding, line_count. May include sheets, links, metadata for documents.").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"path": JSONSchemaString(pathDesc),
					},
					[]string{"path"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "file_read_lines",
				Description: NewToolDescription("Reads a specific range of lines from a file. Efficient for large files or inspecting search results.").
					WhenToUse(
						"File is very large (> 10000 lines)",
						"Only need to see specific section (e.g., around line from search result)",
						"Paginating through large file",
					).
					WhenNotToUse(
						"File is small enough to read entirely",
						"Need to search content - use file_grep or file_search_content first",
					).
					Returns("JSON with path, start_line, end_line, total_lines, content (array with line numbers), raw_text").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"path":       JSONSchemaString(pathDescLocal),
						"start_line": JSONSchemaInt("Starting line number (1-indexed, first line is 1)"),
						"end_line": map[string]interface{}{
							"type":        "integer",
							"description": "Ending line number (1-indexed, inclusive). Default: start_line + 20",
						},
					},
					[]string{"path", "start_line"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "file_info",
				Description: NewToolDescription("Gets detailed metadata about a file or folder without reading content.").
					WhenToUse(
						"Need file size, type, or dates before reading",
						"Checking if file exists",
						"Determining file type before deciding how to process",
						"Getting folder statistics",
					).
					WhenNotToUse(
						"Need actual file content - use file_read",
					).
					Returns("JSON with name, path, size, is_dir, mod_time, type, permissions").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"path": JSONSchemaString(pathDesc),
					},
					[]string{"path"},
				),
			},
		},

		// ===== WRITING =====
		{
			Type: "function",
			Function: ToolFunction{
				Name: "file_write",
				Description: NewToolDescription("Creates a new file or completely overwrites existing file with provided content.").
					WhenToUse(
						"Creating a new file",
						"Replacing entire file content",
						"Writing generated content to file",
					).
					WhenNotToUse(
						"Adding to existing file - use file_append instead",
						"Replacing specific text - use file_replace instead",
						"File is in protected/system folder (will be blocked)",
					).
					Notes(
						"Creates intermediate directories by default",
						"WARNING: Overwrites existing content without confirmation",
					).
					Returns("JSON with success, path, size, message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"path":        JSONSchemaString(pathDescLocal),
						"content":     JSONSchemaString("Complete content to write to the file"),
						"create_dirs": JSONSchemaBool("Create intermediate directories if they don't exist. Default: true"),
					},
					[]string{"path", "content"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "file_append",
				Description: NewToolDescription("Adds content to the end of an existing file, or creates file if it doesn't exist.").
					WhenToUse(
						"Adding entries to log files",
						"Appending items to lists",
						"Adding content without reading the whole file first",
					).
					WhenNotToUse(
						"Need to replace specific text - use file_replace",
						"Need to insert at specific position - use file_read + file_write",
					).
					Returns("JSON with success, path, bytes_written, message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"path":    JSONSchemaString(pathDescLocal),
						"content": JSONSchemaString("Content to append to end of file"),
					},
					[]string{"path", "content"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "file_replace",
				Description: NewToolDescription("Finds and replaces text within a file. Can replace first occurrence or all occurrences.").
					WhenToUse(
						"Replacing specific text string in file",
						"Updating configuration values",
						"Renaming variables or strings",
						"Making targeted edits without rewriting entire file",
					).
					WhenNotToUse(
						"old_text doesn't exist in file (check with file_grep first)",
						"Complex edits requiring context - read file first",
					).
					Notes(
						"Returns count of replacements made",
						"If text not found, returns success:false with message",
					).
					Returns("JSON with success, path, replacements (count), message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"path":        JSONSchemaString(pathDescLocal),
						"old_text":    JSONSchemaString("Exact text to find and replace"),
						"new_text":    JSONSchemaString("Replacement text (can be empty to delete)"),
						"replace_all": JSONSchemaBool("Replace ALL occurrences (true) or just first (false). Default: false"),
					},
					[]string{"path", "old_text", "new_text"},
				),
			},
		},

		// ===== SEARCH =====
		{
			Type: "function",
			Function: ToolFunction{
				Name: "file_search_name",
				Description: NewToolDescription("Searches for files by name using glob patterns. Fast filename-based search.").
					WhenToUse(
						"Finding files by extension (*.txt, *.go, *.json)",
						"Finding files with specific naming pattern (report*.pdf)",
						"Locating files when you know part of the name",
						"Finding all files of a certain type in directory tree",
					).
					WhenNotToUse(
						"Looking for files containing specific text - use file_search_content",
						"Need line numbers and context - use file_grep",
					).
					Notes(
						"For Google Drive, pattern becomes a search query",
						"Use ** for recursive matching (e.g., **/*.go)",
					).
					Returns("JSON with directory, pattern, results (array of file paths), count").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"directory": JSONSchemaString(
							NewParamDescription("Base directory to search from").
								Examples(".", "C:\\Projects", "gdrive://").
								Build(),
						),
						"pattern": JSONSchemaString(
							NewParamDescription("Glob pattern for matching filenames").
								Examples("*.txt", "**/*.go", "report*.pdf", "config.*").
								Build(),
						),
						"recursive":   JSONSchemaBool("Search in subdirectories. Default: true"),
						"max_results": JSONSchemaInt("Maximum results to return. Default: 100"),
					},
					[]string{"directory", "pattern"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "file_search_content",
				Description: NewToolDescription("Searches for files containing specific text. Returns list of matching files.").
					WhenToUse(
						"Finding which files contain a specific string",
						"Locating files by content when filename is unknown",
						"Quick content search across many files",
					).
					WhenNotToUse(
						"Need exact line numbers and context - use file_grep instead",
						"Searching by filename - use file_search_name",
						"Complex regex patterns - use file_grep with is_regex=true",
					).
					Returns("JSON with directory, query, results (array of {path, matches}), count").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"directory": JSONSchemaString(
							NewParamDescription("Base directory to search").Build(),
						),
						"query": JSONSchemaString(
							NewParamDescription("Text to search for inside files").
								Constraints("case-insensitive by default", "plain text, not regex").
								Build(),
						),
						"file_pattern":   JSONSchemaString("Filter by file type (e.g., *.go, *.txt). Default: all text files"),
						"case_sensitive": JSONSchemaBool("Case-sensitive matching. Default: false"),
						"max_results":    JSONSchemaInt("Maximum files to return. Default: 50"),
					},
					[]string{"directory", "query"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "file_grep",
				Description: NewToolDescription("Structured search returning exact locations: file path, line number, column, and surrounding context. Like grep/ripgrep.").
					WhenToUse(
						"Need exact line numbers to use with file_read_lines",
						"Need surrounding context to understand matches",
						"Planning to edit found occurrences with file_replace",
						"Using regex patterns for complex matching",
						"Want detailed results for code navigation",
					).
					WhenNotToUse(
						"Just need list of files containing text - use file_search_content (faster)",
						"Searching by filename - use file_search_name",
					).
					Notes(
						"Returns context_before and context_after for each match",
						"Supports regex when is_regex=true",
						"Results grouped by file with all matches per file",
					).
					Returns("JSON with query, directory, total_files, total_matches, results (array of {file, matches[]})").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"directory": JSONSchemaString(
							NewParamDescription("Base directory to search").Build(),
						),
						"query": JSONSchemaString(
							NewParamDescription("Search term or regex pattern").
								Examples("TODO", "func.*Handler", "import.*fmt").
								Build(),
						),
						"is_regex":      JSONSchemaBool("Treat query as regular expression. Default: false"),
						"file_pattern":  JSONSchemaString("Filter by file type (e.g., *.go). Default: all text files"),
						"context_lines": JSONSchemaInt("Lines of context before and after match. Default: 2"),
						"max_files":     JSONSchemaInt("Maximum files to search. Default: 50"),
					},
					[]string{"directory", "query"},
				),
			},
		},

		// ===== DELETE =====
		{
			Type: "function",
			Function: ToolFunction{
				Name: "file_delete",
				Description: NewToolDescription("Deletes a file. Requires explicit confirmation unless folder is pre-authorized.").
					WhenToUse(
						"User explicitly requests file deletion",
						"Cleaning up temporary files",
						"Removing generated files",
					).
					WhenNotToUse(
						"Deleting directories - not supported (only files)",
						"System or protected files - will be blocked",
					).
					Notes(
						"Returns requires_confirmation:true if folder not authorized",
						"Call again with confirm:true to proceed with deletion",
						"Use authorize_folder to pre-authorize folders for deletion",
					).
					Returns("JSON with success, path, message OR requires_confirmation, path, message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"path":    JSONSchemaString(pathDescLocal),
						"confirm": JSONSchemaBool("Explicit confirmation for deletion in non-authorized folders"),
					},
					[]string{"path"},
				),
			},
		},

		// ===== AUTHORIZATION =====
		{
			Type: "function",
			Function: ToolFunction{
				Name: "authorize_folder",
				Description: NewToolDescription("Pre-authorizes a folder for file deletion without per-file confirmation.").
					WhenToUse(
						"Setting up a workspace folder for frequent file operations",
						"User trusts a temporary/output directory",
						"Before batch deletion operations",
					).
					WhenNotToUse(
						"Authorizing system folders - will be blocked",
						"One-time deletion - just use confirm:true on file_delete",
					).
					Returns("JSON with success, path, recursive, message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"path":      JSONSchemaString(pathDescLocal),
						"recursive": JSONSchemaBool("Apply to subdirectories too. Default: true"),
					},
					[]string{"path"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "revoke_folder_authorization",
				Description: NewToolDescription("Removes deletion authorization from a folder. Deletions will require confirmation again.").
					WhenToUse(
						"Finishing work on a temporary folder",
						"Increasing safety after sensitive operations",
					).
					Returns("JSON with success, path, message").
					Build(),
				Parameters: JSONSchemaObject(
					map[string]interface{}{
						"path": JSONSchemaString(pathDescLocal),
					},
					[]string{"path"},
				),
			},
		},
		{
			Type: "function",
			Function: ToolFunction{
				Name: "list_authorized_folders",
				Description: NewToolDescription("Lists all folders currently authorized for deletion without confirmation.").
					WhenToUse(
						"Checking current authorization status",
						"Before cleanup operations",
					).
					Returns("JSON with count, folders (array of {path, allow_delete, allow_write, recursive, created_at})").
					Build(),
				Parameters: JSONSchemaObject(map[string]interface{}{}, nil),
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
