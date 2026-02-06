package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"assistente/internal/agents"
	"assistente/internal/llm"
)

// ==================== Generic Tool Definitions ====================

// getGenericTools retorna as tools genéricas registradas diretamente no orquestrador.
// Estas tools são executadas sem delegação para sub-LLMs, eliminando latência extra.
func (a *App) getGenericTools() []llm.Tool {
	return []llm.Tool{
		// === File Tools (roteiam para FileAgent sem sub-LLM) ===
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "file_read",
				Description: "Read the contents of a file. Supports local paths, WSL paths, and Google Drive (gdrive://). For large files, use file_read_lines instead.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Path to the file (absolute or relative to working directory)",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "file_read_lines",
				Description: "Read specific lines from a file. Useful for large files.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Path to the file",
						},
						"start_line": map[string]interface{}{
							"type":        "integer",
							"description": "Starting line (1-indexed)",
						},
						"end_line": map[string]interface{}{
							"type":        "integer",
							"description": "Ending line (default: start_line + 20)",
						},
					},
					"required": []string{"path", "start_line"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "file_write",
				Description: "Write content to a file. Creates the file and intermediate directories if they don't exist.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Path to the file",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Complete content to write",
						},
						"create_dirs": map[string]interface{}{
							"type":        "boolean",
							"description": "Create intermediate directories (default: true)",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "file_append",
				Description: "Append content to the end of a file.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Path to the file",
						},
						"content": map[string]interface{}{
							"type":        "string",
							"description": "Content to append",
						},
					},
					"required": []string{"path", "content"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "file_replace",
				Description: "Find and replace text in a file.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Path to the file",
						},
						"old_text": map[string]interface{}{
							"type":        "string",
							"description": "Exact text to find",
						},
						"new_text": map[string]interface{}{
							"type":        "string",
							"description": "Replacement text",
						},
						"replace_all": map[string]interface{}{
							"type":        "boolean",
							"description": "Replace all occurrences (default: false)",
						},
					},
					"required": []string{"path", "old_text", "new_text"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "file_delete",
				Description: "Delete a file.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Path to the file to delete",
						},
						"confirm": map[string]interface{}{
							"type":        "boolean",
							"description": "Confirm deletion for non-authorized folders (default: false)",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "folder_list",
				Description: "List files and directories in a folder.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Directory path (default: current working directory)",
						},
						"show_hidden": map[string]interface{}{
							"type":        "boolean",
							"description": "Include hidden files (default: false)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "folder_create",
				Description: "Create a new directory (including intermediate directories).",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Directory path to create",
						},
					},
					"required": []string{"path"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "file_search_content",
				Description: "Search for text content within files in a directory.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"directory": map[string]interface{}{
							"type":        "string",
							"description": "Base directory to search",
						},
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Text to search for",
						},
						"file_pattern": map[string]interface{}{
							"type":        "string",
							"description": "Filter by file type (e.g., *.go, *.ts)",
						},
						"case_sensitive": map[string]interface{}{
							"type":        "boolean",
							"description": "Case-sensitive matching (default: false)",
						},
						"max_results": map[string]interface{}{
							"type":        "integer",
							"description": "Maximum files (default: 50)",
						},
					},
					"required": []string{"directory", "query"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "file_grep",
				Description: "Search for text or regex patterns in files, returning matching lines with context.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"directory": map[string]interface{}{
							"type":        "string",
							"description": "Base directory to search",
						},
						"query": map[string]interface{}{
							"type":        "string",
							"description": "Search term or regex pattern",
						},
						"is_regex": map[string]interface{}{
							"type":        "boolean",
							"description": "Treat query as regex (default: false)",
						},
						"file_pattern": map[string]interface{}{
							"type":        "string",
							"description": "Filter by file type (default: all text files)",
						},
						"context_lines": map[string]interface{}{
							"type":        "integer",
							"description": "Context lines before/after match (default: 2)",
						},
						"max_files": map[string]interface{}{
							"type":        "integer",
							"description": "Maximum files to search (default: 50)",
						},
					},
					"required": []string{"directory", "query"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "file_search_name",
				Description: "Search for files by name pattern (glob) in a directory.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"directory": map[string]interface{}{
							"type":        "string",
							"description": "Base directory to search",
						},
						"pattern": map[string]interface{}{
							"type":        "string",
							"description": "Glob pattern (e.g., *.txt, **/*.go)",
						},
						"recursive": map[string]interface{}{
							"type":        "boolean",
							"description": "Search subdirectories (default: true)",
						},
						"max_results": map[string]interface{}{
							"type":        "integer",
							"description": "Maximum results (default: 100)",
						},
					},
					"required": []string{"directory", "pattern"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "get_working_directory",
				Description: "Get the current working directory for file operations.",
				Parameters: map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "set_working_directory",
				Description: "Set the working directory for file operations.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"path": map[string]interface{}{
							"type":        "string",
							"description": "Path to set as working directory",
						},
					},
					"required": []string{"path"},
				},
			},
		},

		// === Web Tools ===
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "web_navigate",
				Description: "Navigate to a URL in the browser. Opens the page and returns basic info.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"url": map[string]interface{}{
							"type":        "string",
							"description": "URL to navigate to",
						},
						"wait_for": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector to wait for (default: body)",
						},
						"timeout": map[string]interface{}{
							"type":        "integer",
							"description": "Timeout in seconds (default: 30)",
						},
					},
					"required": []string{"url"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "web_read",
				Description: "Read the content of the current page in the browser.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector to extract from (default: auto-detect)",
						},
						"include_links": map[string]interface{}{
							"type":        "boolean",
							"description": "Include links (default: false)",
						},
						"max_length": map[string]interface{}{
							"type":        "integer",
							"description": "Max content length (default: 50000)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "web_screenshot",
				Description: "Take a screenshot of the current page.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector of element to capture (default: full viewport)",
						},
						"full_page": map[string]interface{}{
							"type":        "boolean",
							"description": "Capture full page with scrolling (default: false)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "web_click",
				Description: "Click an element on the current page.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector of element to click",
						},
						"text": map[string]interface{}{
							"type":        "string",
							"description": "Text of element to click (alternative to selector)",
						},
						"wait_navigation": map[string]interface{}{
							"type":        "boolean",
							"description": "Wait for navigation after click (default: true)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "web_type",
				Description: "Type text into an input field on the current page.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"selector": map[string]interface{}{
							"type":        "string",
							"description": "CSS selector of input field",
						},
						"text": map[string]interface{}{
							"type":        "string",
							"description": "Text to type",
						},
						"clear_first": map[string]interface{}{
							"type":        "boolean",
							"description": "Clear field before typing (default: true)",
						},
						"submit": map[string]interface{}{
							"type":        "boolean",
							"description": "Press Enter after typing (default: false)",
						},
					},
					"required": []string{"selector", "text"},
				},
			},
		},

		// === Image Tool ===
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "image_generate",
				Description: "Generate an image using DALL-E based on a text prompt.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"prompt": map[string]interface{}{
							"type":        "string",
							"description": "Detailed description of the image to generate",
						},
						"size": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"1024x1024", "1024x1792", "1792x1024"},
							"description": "Image dimensions (default: 1024x1024)",
						},
						"quality": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"standard", "hd"},
							"description": "Image quality (default: standard)",
						},
						"style": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"vivid", "natural"},
							"description": "Visual style (default: vivid)",
						},
					},
					"required": []string{"prompt"},
				},
			},
		},

		// === Chat/Conversation Management Tools (ex-ChatManager Agent) ===
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "new_conversation",
				Description: "Create a new conversation and open it in a new tab.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"title": map[string]interface{}{
							"type":        "string",
							"description": "Title for the new conversation (optional)",
						},
					},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "rename_conversation",
				Description: "Rename a conversation.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"conversation_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID of the conversation (optional, defaults to current)",
						},
						"new_title": map[string]interface{}{
							"type":        "string",
							"description": "New title for the conversation",
						},
					},
					"required": []string{"new_title"},
				},
			},
		},
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "switch_to_tab",
				Description: "Switch to a specific open tab.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"tab_id": map[string]interface{}{
							"type":        "integer",
							"description": "ID of the tab to switch to",
						},
					},
					"required": []string{"tab_id"},
				},
			},
		},

		// === HTTP Request Tool ===
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "http_request",
				Description: "Make an HTTP request to any URL. Use for REST API calls, fetching web content, etc.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"method": map[string]interface{}{
							"type":        "string",
							"enum":        []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
							"description": "HTTP method",
						},
						"url": map[string]interface{}{
							"type":        "string",
							"description": "Full URL to request",
						},
						"headers": map[string]interface{}{
							"type":        "object",
							"description": "HTTP headers as key-value pairs",
						},
						"body": map[string]interface{}{
							"type":        "string",
							"description": "Request body (for POST, PUT, PATCH)",
						},
					},
					"required": []string{"method", "url"},
				},
			},
		},

		// === Shell Execute Tool ===
		{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        "shell_execute",
				Description: "Execute a shell command. Use for running scripts, CLI tools, git commands, etc. Commands run in the current working directory.",
				Parameters: map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"command": map[string]interface{}{
							"type":        "string",
							"description": "Shell command to execute",
						},
						"working_directory": map[string]interface{}{
							"type":        "string",
							"description": "Working directory for the command (optional, defaults to file agent working directory)",
						},
						"timeout_seconds": map[string]interface{}{
							"type":        "integer",
							"description": "Timeout in seconds (default: 30, max: 300)",
						},
					},
					"required": []string{"command"},
				},
			},
		},
	}
}

// ==================== Generic Tool Execution ====================

// executeGenericTool tenta executar uma tool genérica.
// Retorna (result, handled, error). Se handled=false, a tool não é genérica.
func (a *App) executeGenericTool(toolCall llm.ToolCall) (string, bool, error) {
	toolName := toolCall.Function.Name

	// Tools que roteiam para agentes existentes (sem sub-LLM)
	fileToolNames := map[string]bool{
		"file_read": true, "file_read_lines": true, "file_write": true,
		"file_append": true, "file_replace": true, "file_delete": true,
		"folder_list": true, "folder_create": true,
		"file_search_content": true, "file_grep": true, "file_search_name": true,
		"get_working_directory": true, "set_working_directory": true,
	}

	webToolNames := map[string]bool{
		"web_navigate": true, "web_read": true, "web_screenshot": true,
		"web_click": true, "web_type": true,
	}

	imageToolNames := map[string]bool{
		"generate_image": true, "image_generate": true,
	}

	chatManagerToolNames := map[string]bool{
		"new_conversation": true, "rename_conversation": true,
		"switch_to_tab": true,
	}

	// File tools → FileAgent.ExecuteTool (sem sub-LLM)
	if fileToolNames[toolName] {
		result, err := a.routeToAgentTool("file_manager", toolCall)
		return result, true, err
	}

	// Web tools → WebAgent.ExecuteTool (sem sub-LLM)
	if webToolNames[toolName] {
		result, err := a.routeToAgentTool("web", toolCall)
		return result, true, err
	}

	// Image tools → ImageAgent.ExecuteTool
	if imageToolNames[toolName] {
		// Remap image_generate → generate_image (nome interno do agente)
		if toolName == "image_generate" {
			toolCall.Function.Name = "generate_image"
		}
		result, err := a.routeToAgentTool("image", toolCall)
		return result, true, err
	}

	// Chat Manager tools → ChatManagerAgent.ExecuteTool
	if chatManagerToolNames[toolName] {
		result, err := a.routeToAgentTool("chat_manager", toolCall)
		return result, true, err
	}

	// HTTP Request — implementação standalone
	if toolName == "http_request" {
		result, err := a.executeHTTPRequest(toolCall)
		return result, true, err
	}

	// Shell Execute — implementação standalone
	if toolName == "shell_execute" {
		result, err := a.executeShellCommand(toolCall)
		return result, true, err
	}

	return "", false, nil
}

// routeToAgentTool roteia uma tool call diretamente para o ExecuteTool de um agente,
// sem passar pelo sub-LLM do agente. Isso elimina a latência de delegação.
func (a *App) routeToAgentTool(agentName string, toolCall llm.ToolCall) (string, error) {
	if a.registry == nil {
		return "", fmt.Errorf("registry não inicializado")
	}

	agent := a.registry.Get(agentName)
	if agent == nil {
		return "", fmt.Errorf("agente %s não encontrado", agentName)
	}

	// Converte llm.ToolCall para agents.ToolCall
	agentToolCall := agents.ToolCall{
		ID:   toolCall.ID,
		Type: toolCall.Type,
		Function: agents.FunctionCall{
			Name:      toolCall.Function.Name,
			Arguments: toolCall.Function.Arguments,
		},
	}

	return agent.ExecuteTool(agentToolCall)
}

// ==================== Standalone Tool Implementations ====================

// executeHTTPRequest faz uma requisição HTTP genérica
func (a *App) executeHTTPRequest(toolCall llm.ToolCall) (string, error) {
	var args struct {
		Method  string            `json:"method"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}

	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("erro ao parsear argumentos: %v", err)
	}

	if args.URL == "" {
		return "", fmt.Errorf("URL não especificada")
	}
	if args.Method == "" {
		args.Method = "GET"
	}

	log.Printf("[http_request] %s %s", args.Method, args.URL)

	// Cria a requisição
	var bodyReader io.Reader
	if args.Body != "" {
		bodyReader = strings.NewReader(args.Body)
	}

	req, err := http.NewRequest(args.Method, args.URL, bodyReader)
	if err != nil {
		return "", fmt.Errorf("erro ao criar requisição: %v", err)
	}

	// Aplica headers
	for k, v := range args.Headers {
		req.Header.Set(k, v)
	}

	// Faz a requisição
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("erro na requisição: %v", err)
	}
	defer resp.Body.Close()

	// Lê a resposta (limitada a 100KB)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 100*1024))
	if err != nil {
		return "", fmt.Errorf("erro ao ler resposta: %v", err)
	}

	result := map[string]interface{}{
		"status":      resp.StatusCode,
		"status_text": resp.Status,
		"headers":     resp.Header,
		"body":        string(body),
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return string(resultJSON), nil
}

// executeShellCommand executa um comando no shell
func (a *App) executeShellCommand(toolCall llm.ToolCall) (string, error) {
	var args struct {
		Command          string `json:"command"`
		WorkingDirectory string `json:"working_directory"`
		TimeoutSeconds   int    `json:"timeout_seconds"`
	}

	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("erro ao parsear argumentos: %v", err)
	}

	if args.Command == "" {
		return "", fmt.Errorf("comando não especificado")
	}

	timeout := args.TimeoutSeconds
	if timeout <= 0 {
		timeout = 30
	}
	if timeout > 300 {
		timeout = 300
	}

	log.Printf("[shell_execute] %s (timeout: %ds)", args.Command, timeout)

	// Determina o shell baseado no OS
	var cmd *exec.Cmd
	cmd = exec.Command("cmd", "/C", args.Command)

	// Define working directory
	if args.WorkingDirectory != "" {
		cmd.Dir = args.WorkingDirectory
	} else {
		// Tenta usar o working directory do FileAgent
		if a.registry != nil {
			if agent := a.registry.Get("file_manager"); agent != nil {
				if fileAgent, ok := agent.(*agents.FileAgent); ok {
					cmd.Dir = fileAgent.GetWorkingDirectory()
				}
			}
		}
	}

	// Combina stdout e stderr
	output, err := cmd.CombinedOutput()

	result := map[string]interface{}{
		"command":  args.Command,
		"output":   string(output),
		"exit_code": 0,
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			result["exit_code"] = exitErr.ExitCode()
		} else {
			return "", fmt.Errorf("erro ao executar comando: %v", err)
		}
	}

	resultJSON, _ := json.MarshalIndent(result, "", "  ")
	return string(resultJSON), nil
}
