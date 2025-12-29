package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"assistente/internal/mcp"
)

// MCPExecutionMode define como o agente executa tools MCP
type MCPExecutionMode string

const (
	// MCPModeConvert converte tools MCP para formato OpenAI (compatível com qualquer modelo)
	MCPModeConvert MCPExecutionMode = "convert"

	// MCPModeNative passa tools MCP no formato nativo (para modelos que suportam MCP)
	MCPModeNative MCPExecutionMode = "native"

	// MCPModePassthrough não usa LLM intermediário - passa a tarefa direto para o servidor MCP
	// Útil quando o servidor MCP já tem um LLM embutido
	MCPModePassthrough MCPExecutionMode = "passthrough"
)

// MCPTransportType define o tipo de transporte MCP
type MCPTransportType string

const (
	// MCPTransportStdio usa processo local via stdin/stdout
	MCPTransportStdio MCPTransportType = "stdio"

	// MCPTransportHTTP usa HTTP/SSE para servidor remoto
	MCPTransportHTTP MCPTransportType = "http"
)

// MCPAgent é um agente que se conecta a um servidor MCP
type MCPAgent struct {
	BaseAgent

	// Configuração do servidor MCP (para stdio)
	ServerCommand string   `json:"server_command"` // Comando para iniciar o servidor
	ServerArgs    []string `json:"server_args"`    // Argumentos do comando
	ServerEnv     []string `json:"server_env"`     // Variáveis de ambiente

	// Configuração HTTP (para http)
	ServerURL   string            `json:"server_url"`   // URL do servidor MCP remoto
	AuthType    string            `json:"auth_type"`    // none, bearer, api_key
	AuthValue   string            `json:"auth_value"`   // Token ou API Key
	HTTPHeaders map[string]string `json:"http_headers"` // Headers customizados

	// Configuração comum
	TransportType MCPTransportType `json:"transport_type"` // stdio ou http
	ExecutionMode MCPExecutionMode `json:"execution_mode"` // Modo de execução

	// Cliente MCP (interface comum)
	transport mcp.Transport
}

// MCPAgentConfig contém a configuração para criar um MCPAgent
type MCPAgentConfig struct {
	Name         string `json:"name"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	Model        string `json:"model"`
	SystemPrompt string `json:"system_prompt"`

	// Configuração stdio
	ServerCommand string   `json:"server_command"`
	ServerArgs    []string `json:"server_args"`
	ServerEnv     []string `json:"server_env"`

	// Configuração HTTP
	ServerURL   string            `json:"server_url"`
	AuthType    string            `json:"auth_type"`
	AuthValue   string            `json:"auth_value"`
	HTTPHeaders map[string]string `json:"http_headers"`

	// Configuração comum
	TransportType MCPTransportType `json:"transport_type"` // stdio, http
	ExecutionMode MCPExecutionMode `json:"execution_mode"` // convert, native, passthrough
}

// NewMCPAgent cria um novo MCPAgent
func NewMCPAgent(config MCPAgentConfig, llmClient LLMClient) *MCPAgent {
	if config.Model == "" {
		config.Model = "gpt-4o-mini"
	}

	if config.ExecutionMode == "" {
		config.ExecutionMode = MCPModeConvert // Modo padrão para compatibilidade
	}

	if config.TransportType == "" {
		config.TransportType = MCPTransportStdio // Padrão para stdio
	}

	if config.SystemPrompt == "" {
		config.SystemPrompt = fmt.Sprintf(`Você é um agente especializado que usa o servidor MCP "%s".

Analise a tarefa recebida e use as ferramentas disponíveis para completá-la.
Seja preciso ao passar parâmetros para as ferramentas.
Retorne uma resposta clara e formatada para o usuário.`, config.DisplayName)
	}

	return &MCPAgent{
		BaseAgent: BaseAgent{
			Name:         config.Name,
			DisplayName:  config.DisplayName,
			Description:  config.Description,
			AgentType:    "mcp",
			Model:        config.Model,
			SystemPrompt: config.SystemPrompt,
			Enabled:      true,
			LLM:          llmClient,
		},
		ServerCommand: config.ServerCommand,
		ServerArgs:    config.ServerArgs,
		ServerEnv:     config.ServerEnv,
		ServerURL:     config.ServerURL,
		AuthType:      config.AuthType,
		AuthValue:     config.AuthValue,
		HTTPHeaders:   config.HTTPHeaders,
		TransportType: config.TransportType,
		ExecutionMode: config.ExecutionMode,
	}
}

// Connect conecta ao servidor MCP usando o transporte configurado
// Tenta múltiplas vezes antes de falhar (útil para mcp-remote que pode precisar de tempo)
func (a *MCPAgent) Connect() error {
	return a.ConnectWithRetry(3, 2*time.Second)
}

// ConnectWithRetry tenta conectar com retry e delay
func (a *MCPAgent) ConnectWithRetry(maxRetries int, delay time.Duration) error {
	if a.transport != nil && a.transport.IsConnected() {
		return nil // Já conectado
	}

	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		if attempt > 1 {
			fmt.Printf("[MCP Agent %s] Tentativa %d/%d de conexão...\n", a.Name, attempt, maxRetries)
			time.Sleep(delay)
		}

		var transport mcp.Transport

		switch a.TransportType {
		case MCPTransportHTTP:
			// Cria cliente HTTP/SSE
			httpClient := mcp.NewHTTPClient(mcp.HTTPConfig{
				BaseURL:   a.ServerURL,
				AuthType:  a.AuthType,
				AuthValue: a.AuthValue,
				Headers:   a.HTTPHeaders,
			})
			transport = httpClient

		default: // MCPTransportStdio
			// Cria cliente stdio
			stdioClient, createErr := mcp.NewClient(a.ServerCommand, a.ServerArgs, a.ServerEnv)
			if createErr != nil {
				lastErr = fmt.Errorf("erro ao criar cliente MCP stdio: %w", createErr)
				continue
			}
			transport = stdioClient
		}

		// Tenta conectar
		if err := transport.Connect(); err != nil {
			lastErr = fmt.Errorf("erro ao conectar ao servidor MCP: %w", err)
			// Fecha o transporte se falhou
			transport.Close()
			continue
		}

		// Sucesso!
		a.transport = transport
		return nil
	}

	return fmt.Errorf("falha após %d tentativas: %w", maxRetries, lastErr)
}

// Disconnect desconecta do servidor MCP
func (a *MCPAgent) Disconnect() error {
	if a.transport != nil {
		err := a.transport.Close()
		a.transport = nil
		return err
	}
	return nil
}

// IsConnected verifica se está conectado ao servidor MCP
func (a *MCPAgent) IsConnected() bool {
	return a.transport != nil && a.transport.IsConnected()
}

// GetMCPTools retorna as ferramentas do servidor MCP
func (a *MCPAgent) GetMCPTools() []mcp.Tool {
	if a.transport == nil {
		return nil
	}
	return a.transport.GetTools()
}

// GetServerInfo retorna informações do servidor MCP
func (a *MCPAgent) GetServerInfo() *mcp.ServerInfo {
	if a.transport == nil {
		return nil
	}
	return a.transport.GetServerInfo()
}

// GetTransport retorna o transporte MCP para acesso direto
func (a *MCPAgent) GetTransport() mcp.Transport {
	return a.transport
}

// GetTools retorna as tools no formato esperado pelo LLM
func (a *MCPAgent) GetTools() []Tool {
	if a.transport == nil {
		return nil
	}

	mcpTools := a.transport.GetTools()
	tools := make([]Tool, len(mcpTools))

	for i, t := range mcpTools {
		tools[i] = Tool{
			Type: "function",
			Function: ToolFunction{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.InputSchema,
			},
		}
	}

	return tools
}

// Execute executa uma tarefa usando o LLM do agente e as tools MCP
func (a *MCPAgent) Execute(ctx context.Context, task string) (string, error) {
	// Conecta ou reconecta ao servidor MCP se necessário
	if err := a.ensureConnected(); err != nil {
		return "", err
	}

	// Escolhe o modo de execução
	switch a.ExecutionMode {
	case MCPModePassthrough:
		return a.executePassthrough(ctx, task)
	case MCPModeNative:
		return a.executeNative(ctx, task)
	default: // MCPModeConvert
		return a.executeConvert(ctx, task)
	}
}

// ensureConnected garante que estamos conectados, reconectando se necessário
func (a *MCPAgent) ensureConnected() error {
	// Se já está conectado, retorna
	if a.IsConnected() {
		return nil
	}

	// Se tinha um transporte antigo, tenta reconectar se suportar
	if a.transport != nil {
		fmt.Printf("[MCP Agent %s] Conexão perdida, tentando reconectar...\n", a.Name)

		// Verifica se o transporte suporta reconexão
		if reconnectable, ok := a.transport.(mcp.Reconnectable); ok {
			if err := reconnectable.Reconnect(); err != nil {
				fmt.Printf("[MCP Agent %s] Falha ao reconectar: %v\n", a.Name, err)
				a.transport.Close()
				a.transport = nil
				// Tenta conexão completa
				return a.Connect()
			}
			fmt.Printf("[MCP Agent %s] Reconectado com sucesso!\n", a.Name)
			return nil
		}

		// Transporte não suporta reconexão, fecha e cria novo
		a.transport.Close()
		a.transport = nil
	}

	// Tenta conectar
	return a.Connect()
}

// Ping verifica se a conexão está ativa
func (a *MCPAgent) Ping() error {
	if a.transport == nil {
		return fmt.Errorf("não conectado")
	}

	if reconnectable, ok := a.transport.(MCPReconnectable); ok {
		return reconnectable.Ping()
	}

	// Para transportes que não suportam ping, verifica IsConnected
	if !a.IsConnected() {
		return fmt.Errorf("não conectado")
	}
	return nil
}

// executeConvert usa conversão de tools MCP para formato OpenAI
func (a *MCPAgent) executeConvert(ctx context.Context, task string) (string, error) {
	if a.LLM == nil {
		return "", fmt.Errorf("LLM client não configurado para o agente %s", a.Name)
	}

	tools := a.GetTools()
	if len(tools) == 0 {
		return "", fmt.Errorf("nenhuma ferramenta disponível no servidor MCP")
	}

	// Gera system prompt com informações das tools
	systemPrompt := a.generateSystemPrompt()

	// A interface LLMClient.ChatWithTools já gerencia o loop de tool calls internamente
	// Passamos o executor de tools que usa o transporte MCP
	return a.LLM.ChatWithTools(ctx, a.Model, systemPrompt, task, tools, a.executeTool)
}

// executeNative passa tools MCP no formato nativo para modelos que suportam
// Útil para Claude e outros modelos com suporte nativo a MCP
func (a *MCPAgent) executeNative(ctx context.Context, task string) (string, error) {
	if a.LLM == nil {
		return "", fmt.Errorf("LLM client não configurado para o agente %s", a.Name)
	}

	mcpTools := a.GetMCPTools()
	if len(mcpTools) == 0 {
		return "", fmt.Errorf("nenhuma ferramenta disponível no servidor MCP")
	}

	// Verifica se o LLM suporta modo nativo MCP
	nativeLLM, ok := a.LLM.(MCPNativeLLMClient)
	if !ok {
		// Fallback para modo convert se LLM não suporta nativo
		fmt.Printf("  ⚠️ LLM não suporta MCP nativo, usando modo convert\n")
		return a.executeConvert(ctx, task)
	}

	// Gera system prompt
	systemPrompt := a.generateSystemPrompt()

	// Usa interface nativa do LLM
	return nativeLLM.ChatWithMCPTools(ctx, a.Model, systemPrompt, task, mcpTools, func(name string, args map[string]interface{}) (string, error) {
		result, err := a.transport.CallTool(name, args)
		if err != nil {
			return "", err
		}
		return a.formatMCPResult(result), nil
	})
}

// executePassthrough envia a tarefa direto para o servidor MCP
// Útil quando o servidor MCP já tem um LLM embutido (ex: Claude Desktop)
func (a *MCPAgent) executePassthrough(ctx context.Context, task string) (string, error) {
	// Verifica se há uma tool especial para processar tarefas
	mcpTools := a.GetMCPTools()

	// Procura por uma tool que aceite a tarefa como input
	// Normalmente seria algo como "process_task" ou "chat"
	for _, tool := range mcpTools {
		if tool.Name == "process_task" || tool.Name == "chat" || tool.Name == "query" {
			result, err := a.transport.CallTool(tool.Name, map[string]interface{}{
				"task":    task,
				"message": task,
				"query":   task,
			})
			if err != nil {
				return "", err
			}
			return a.formatMCPResult(result), nil
		}
	}

	// Se não encontrou tool específica, tenta usar a primeira tool disponível
	// passando a tarefa como único argumento
	if len(mcpTools) > 0 {
		// Fallback para modo convert se não encontrou tool de passthrough
		fmt.Printf("  ⚠️ Nenhuma tool de passthrough encontrada, usando modo convert\n")
		return a.executeConvert(ctx, task)
	}

	return "", fmt.Errorf("nenhuma ferramenta disponível para passthrough")
}

// MaxToolResponseChars define o limite máximo de caracteres para respostas de tools
// 50.000 caracteres ~ 12.500 tokens (margem segura para contexto de 128k)
const MaxToolResponseChars = 50000

// authErrorPatterns são padrões específicos que indicam erros de autenticação/sessão expirada
// Usamos padrões mais específicos para evitar falsos positivos em respostas normais
var authErrorPatterns = []struct {
	pattern    string
	mustBeJSON bool // Se true, só detecta em respostas curtas que parecem JSON de erro
}{
	{`"code":401`, false},
	{`"code": 401`, false},
	{`"message":"Unauthorized"`, false},
	{`"message": "Unauthorized"`, false},
	{`Authentication failed`, false},
	{`authentication failed`, false},
	{`accessibleResources.filter is not a function`, false}, // Bug específico do mcp-remote
	{`session expired`, false},
	{`Session expired`, false},
	{`token expired`, false},
	{`Token expired`, false},
	{`invalid_grant`, false},
	{`access_denied`, false},
}

// isAuthenticationError verifica se o texto indica erro de autenticação ou sessão expirada
// Usa padrões mais específicos para evitar falsos positivos
func isAuthenticationError(text string) bool {
	// Respostas muito grandes (> 5KB) provavelmente são dados válidos, não erros
	if len(text) > 5000 {
		return false
	}

	for _, p := range authErrorPatterns {
		if strings.Contains(text, p.pattern) {
			return true
		}
	}
	return false
}

// isToolResultAuthError verifica se um ToolResult indica erro de autenticação
// Só considera erro de auth se o resultado for um erro (IsError) ou resposta curta com padrão de erro
func (a *MCPAgent) isToolResultAuthError(result *mcp.ToolResult) bool {
	if result == nil {
		return false
	}

	// Se o MCP explicitamente marcou como erro, verifica padrões
	if result.IsError {
		for _, content := range result.Content {
			if content.Type == "text" && isAuthenticationError(content.Text) {
				return true
			}
		}
	}

	// Para respostas não marcadas como erro, só verifica se for curta (< 1KB)
	// para evitar falsos positivos em JSONs grandes válidos
	for _, content := range result.Content {
		if content.Type == "text" && len(content.Text) < 1000 && isAuthenticationError(content.Text) {
			return true
		}
	}

	return false
}

// forceReconnect força uma reconexão completa, fechando o transporte atual
func (a *MCPAgent) forceReconnect() error {
	fmt.Printf("[MCP Agent %s] 🔄 Forçando reconexão completa (sessão expirada detectada)...\n", a.Name)

	// Fecha o transporte atual
	if a.transport != nil {
		a.transport.Close()
		a.transport = nil
	}

	// Aguarda um momento para garantir que recursos foram liberados
	time.Sleep(500 * time.Millisecond)

	// Tenta reconectar com mais tentativas para OAuth
	return a.ConnectWithRetry(3, 3*time.Second)
}

// formatMCPResult formata o resultado de uma chamada MCP com truncamento automático
func (a *MCPAgent) formatMCPResult(result *mcp.ToolResult) string {
	var sb strings.Builder
	for _, content := range result.Content {
		switch content.Type {
		case "text":
			sb.WriteString(content.Text)
		case "image":
			sb.WriteString(fmt.Sprintf("[Imagem: %s]", content.MimeType))
		case "resource":
			sb.WriteString(fmt.Sprintf("[Recurso: %s]", content.MimeType))
		}
		sb.WriteString("\n")
	}

	text := strings.TrimSpace(sb.String())

	// Trunca se exceder o limite para evitar estouro de contexto do LLM
	if len(text) > MaxToolResponseChars {
		truncated := text[:MaxToolResponseChars]

		// Tenta cortar em uma quebra de linha para não cortar no meio de uma palavra
		lastNewline := strings.LastIndex(truncated, "\n")
		if lastNewline > MaxToolResponseChars*3/4 { // Só corta se não perder muito
			truncated = truncated[:lastNewline]
		}

		originalLen := len(text)
		truncatedLen := len(truncated)
		fmt.Printf("[MCP Agent %s] ⚠️ Resposta truncada: %d → %d caracteres (%.1f%% removido)\n",
			a.Name, originalLen, truncatedLen, float64(originalLen-truncatedLen)/float64(originalLen)*100)

		return truncated + fmt.Sprintf("\n\n[... RESPOSTA TRUNCADA: exibindo %d de %d caracteres. Para detalhes completos, acesse diretamente o recurso.]", truncatedLen, originalLen)
	}

	return text
}

// executeTool executa uma ferramenta MCP a partir de um ToolCall
// Inclui detecção automática de erros de autenticação e reconexão transparente
func (a *MCPAgent) executeTool(toolCall ToolCall) (string, error) {
	// Verifica e tenta reconectar se necessário
	if err := a.ensureConnected(); err != nil {
		return "", fmt.Errorf("não foi possível conectar ao MCP: %w", err)
	}

	// Parse dos argumentos
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("erro ao parsear argumentos: %w", err)
	}

	// Função auxiliar para executar a chamada
	executeCall := func() (*mcp.ToolResult, error) {
		return a.transport.CallTool(toolCall.Function.Name, args)
	}

	// Primeira tentativa
	result, err := executeCall()

	// Verifica se houve erro de transporte
	if err != nil {
		fmt.Printf("[MCP Agent %s] ⚠️ Erro na chamada: %v\n", a.Name, err)

		// Verifica se o erro indica problema de autenticação
		if isAuthenticationError(err.Error()) {
			if reconnErr := a.forceReconnect(); reconnErr == nil {
				result, err = executeCall()
				if err == nil && !a.isToolResultAuthError(result) {
					fmt.Printf("[MCP Agent %s] ✅ Chamada bem-sucedida após reconexão\n", a.Name)
					return a.formatMCPResult(result), nil
				}
			}
		} else {
			// Erro não é de auth, tenta reconexão simples
			if reconnectable, ok := a.transport.(mcp.Reconnectable); ok {
				if reconnErr := reconnectable.Reconnect(); reconnErr == nil {
					result, err = executeCall()
					if err == nil {
						fmt.Printf("[MCP Agent %s] ✅ Chamada bem-sucedida após reconexão\n", a.Name)
						return a.formatMCPResult(result), nil
					}
				}
			}
		}

		return "", fmt.Errorf("erro ao chamar tool %s: %w", toolCall.Function.Name, err)
	}

	// Chamada foi bem sucedida, mas verifica se o resultado indica erro de autenticação
	// (alguns servidores MCP retornam erro de auth dentro do resultado, não como erro de transporte)
	if a.isToolResultAuthError(result) {
		fmt.Printf("[MCP Agent %s] ⚠️ Resposta indica erro de autenticação, tentando reconectar...\n", a.Name)

		// Força reconexão completa para renovar sessão OAuth
		if reconnErr := a.forceReconnect(); reconnErr == nil {
			// Tenta novamente após reconexão
			result, err = executeCall()
			if err == nil && !a.isToolResultAuthError(result) {
				fmt.Printf("[MCP Agent %s] ✅ Sessão renovada, chamada bem-sucedida!\n", a.Name)
				return a.formatMCPResult(result), nil
			}
		}

		// Se ainda falhou, retorna o resultado original (com o erro de auth)
		// para que o LLM possa informar o usuário
		fmt.Printf("[MCP Agent %s] ❌ Não foi possível renovar a sessão\n", a.Name)
	}

	return a.formatMCPResult(result), nil
}

// generateSystemPrompt gera o system prompt com informações das tools
func (a *MCPAgent) generateSystemPrompt() string {
	var sb strings.Builder
	sb.WriteString(a.SystemPrompt)
	sb.WriteString("\n\nFerramentas disponíveis:\n")

	tools := a.GetMCPTools()
	for _, tool := range tools {
		sb.WriteString(fmt.Sprintf("- %s: %s\n", tool.Name, tool.Description))
	}

	sb.WriteString("\nUse as ferramentas conforme necessário para completar a tarefa.")

	return sb.String()
}

// CanHandle verifica se o agente pode executar uma determinada tool
func (a *MCPAgent) CanHandle(toolName string) bool {
	tools := a.GetMCPTools()
	for _, t := range tools {
		if t.Name == toolName {
			return true
		}
	}
	return false
}

// ExecuteTool executa uma tool call (interface Agent)
func (a *MCPAgent) ExecuteTool(toolCall ToolCall) (string, error) {
	return a.executeTool(toolCall)
}
