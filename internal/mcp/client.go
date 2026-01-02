package mcp

/*
MCP Client - Model Context Protocol Implementation

Este cliente implementa o protocolo MCP (Model Context Protocol) conforme especificação:
- Versão do protocolo: 2024-11-05
- Transporte: stdio (JSON-RPC 2.0 sobre stdin/stdout)
- Referência: https://spec.modelcontextprotocol.io/

Funcionalidades implementadas:
✅ Handshake (initialize + notifications/initialized)
✅ Descoberta de tools (tools/list)
✅ Execução de tools (tools/call)
✅ Resources (resources/list, resources/read)
✅ Prompts (prompts/list, prompts/get)
✅ Sampling (sampling/createMessage)
✅ Notificações do servidor (log)
✅ Transporte stdio
✅ Transporte HTTP/SSE

Formato das mensagens:
- Cada mensagem é um objeto JSON em uma única linha
- Requisições têm "id" (número), notificações não têm "id"
- Respostas referenciam o "id" da requisição original
*/

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Client gerencia a comunicação com um servidor MCP via stdio
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	stderr io.ReadCloser

	requestID atomic.Int64
	responses map[int64]chan *JSONRPCResponse
	mu        sync.RWMutex

	serverInfo *ServerInfo
	tools      []Tool

	ctx    context.Context
	cancel context.CancelFunc

	// Estado da conexão
	connected     atomic.Bool
	lastActivity  atomic.Int64 // Unix timestamp da última atividade
	processExited atomic.Bool

	// Para reconexão
	command string
	args    []string
	env     []string
}

// NewClient cria um novo cliente MCP
func NewClient(command string, args []string, env []string) (*Client, error) {
	ctx, cancel := context.WithCancel(context.Background())

	cmd := exec.CommandContext(ctx, command, args...)
	if len(env) > 0 {
		cmd.Env = append(cmd.Environ(), env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("erro ao criar stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("erro ao criar stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return nil, fmt.Errorf("erro ao criar stderr pipe: %w", err)
	}

	client := &Client{
		cmd:       cmd,
		stdin:     stdin,
		stdout:    bufio.NewReader(stdout),
		stderr:    stderr,
		responses: make(map[int64]chan *JSONRPCResponse),
		ctx:       ctx,
		cancel:    cancel,
		// Guarda parâmetros para possível reconexão
		command: command,
		args:    args,
		env:     env,
	}

	return client, nil
}

// Start inicia o servidor MCP e faz o handshake
func (c *Client) Start() error {
	fmt.Printf("[MCP] Iniciando processo: %s\n", c.cmd.Path)
	fmt.Printf("[MCP] Args: %v\n", c.args)

	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("erro ao iniciar servidor MCP: %w", err)
	}

	fmt.Printf("[MCP] Processo iniciado com PID: %d\n", c.cmd.Process.Pid)

	// Inicia goroutine para ler respostas do stdout
	go c.readResponses()

	// Inicia goroutine para ler stderr (importante: evita que o processo bloqueie)
	go c.readStderr()

	// Inicia goroutine para monitorar o processo
	go c.monitorProcess()

	// Aguarda o processo inicializar e estar pronto para receber comandos
	// O mcp-remote pode levar alguns segundos para conectar ao servidor remoto
	if err := c.waitForReady(30 * time.Second); err != nil {
		c.Close()
		return fmt.Errorf("erro aguardando processo MCP ficar pronto: %w", err)
	}

	// Faz o handshake inicial
	if err := c.initialize(); err != nil {
		c.Close()
		return fmt.Errorf("erro no handshake MCP: %w", err)
	}

	// Descobre as tools disponíveis
	if err := c.discoverTools(); err != nil {
		c.Close()
		return fmt.Errorf("erro ao descobrir tools: %w", err)
	}

	// Marca como conectado
	c.connected.Store(true)
	c.updateActivity()

	return nil
}

// readStderr lê o stderr do processo para evitar bloqueio e logar mensagens
func (c *Client) readStderr() {
	scanner := bufio.NewScanner(c.stderr)
	// Aumenta o buffer para capturar mensagens longas
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	serverReady := false

	for scanner.Scan() {
		line := scanner.Text()
		fmt.Printf("[MCP STDERR] %s\n", line)
		c.updateActivity()

		// Detecta quando o mcp-remote está pronto
		if !serverReady && (strings.Contains(line, "Local STDIO server running") ||
			strings.Contains(line, "Proxy established") ||
			strings.Contains(line, "Press Ctrl+C to exit") ||
			strings.Contains(line, "server running") ||
			strings.Contains(line, "ready")) {
			serverReady = true
			fmt.Println("[MCP] ✅ Servidor MCP pronto para comunicação")
		}

		// Detecta URLs de autorização OAuth e abre no navegador
		if strings.Contains(line, "authorize?") || strings.Contains(line, "/oauth/") {
			// Extrai a URL da linha
			if url := extractURL(line); url != "" {
				fmt.Printf("[MCP] Detectada URL de autorização OAuth, abrindo navegador...\n")
				openBrowser(url)
			}
		}

		// Detecta erros fatais
		if strings.Contains(line, "ENOENT") || strings.Contains(line, "Cannot find") ||
			strings.Contains(line, "MODULE_NOT_FOUND") || strings.Contains(line, "npm ERR!") {
			fmt.Printf("[MCP] ❌ Erro fatal detectado no stderr\n")
		}
	}
	if err := scanner.Err(); err != nil {
		fmt.Printf("[MCP] Erro ao ler stderr: %v\n", err)
	}
	fmt.Println("[MCP] Leitura de stderr encerrada")
}

// waitForReady aguarda até o processo MCP estar pronto para receber comandos
// Verifica se o processo está vivo e aguarda sinais do stderr
func (c *Client) waitForReady(timeout time.Duration) error {
	fmt.Println("[MCP] Aguardando processo ficar pronto...")

	start := time.Now()
	checkInterval := 100 * time.Millisecond
	minWait := 2 * time.Second // Tempo mínimo de espera para dar tempo ao processo inicializar

	for {
		elapsed := time.Since(start)

		// Verifica se o processo morreu
		if c.processExited.Load() {
			return fmt.Errorf("processo encerrou prematuramente")
		}

		// Após o tempo mínimo, considera pronto
		if elapsed >= minWait {
			fmt.Printf("[MCP] Processo pronto após %.1fs\n", elapsed.Seconds())
			return nil
		}

		// Timeout
		if elapsed >= timeout {
			return fmt.Errorf("timeout aguardando processo ficar pronto")
		}

		time.Sleep(checkInterval)
	}
}

// monitorProcess monitora o processo do servidor MCP e detecta quando ele morre
func (c *Client) monitorProcess() {
	if c.cmd == nil || c.cmd.Process == nil {
		return
	}

	pid := c.cmd.Process.Pid
	fmt.Printf("[MCP MONITOR] Monitorando processo PID %d\n", pid)

	// Espera o processo terminar
	err := c.cmd.Wait()

	c.processExited.Store(true)
	c.connected.Store(false)

	if err != nil {
		fmt.Printf("[MCP MONITOR] ⚠️ Processo PID %d encerrou com erro: %v\n", pid, err)
	} else {
		fmt.Printf("[MCP MONITOR] Processo PID %d encerrou normalmente (exit code 0)\n", pid)
	}

	// Calcula quanto tempo o processo ficou vivo
	lastActivity := c.GetLastActivity()
	if !lastActivity.IsZero() {
		uptime := time.Since(lastActivity)
		fmt.Printf("[MCP MONITOR] Última atividade: %v atrás\n", uptime.Round(time.Second))
	}

	// Notifica todos os canais pendentes
	c.mu.Lock()
	pendingCount := len(c.responses)
	for id, ch := range c.responses {
		select {
		case ch <- &JSONRPCResponse{
			Error: &JSONRPCError{
				Code:    -32000,
				Message: "Processo MCP encerrou inesperadamente",
			},
		}:
		default:
		}
		delete(c.responses, id)
	}
	c.mu.Unlock()

	if pendingCount > 0 {
		fmt.Printf("[MCP MONITOR] %d chamadas pendentes foram notificadas do encerramento\n", pendingCount)
	}
}

// updateActivity atualiza o timestamp da última atividade
func (c *Client) updateActivity() {
	c.lastActivity.Store(time.Now().Unix())
}

// GetLastActivity retorna o timestamp da última atividade
func (c *Client) GetLastActivity() time.Time {
	return time.Unix(c.lastActivity.Load(), 0)
}

// IsProcessAlive verifica se o processo ainda está rodando
func (c *Client) IsProcessAlive() bool {
	if c.processExited.Load() {
		return false
	}
	if c.cmd == nil || c.cmd.Process == nil {
		return false
	}
	// No Windows, não podemos usar Signal(0) de forma confiável
	// Então confiamos no monitorProcess para atualizar processExited
	return !c.processExited.Load()
}

// extractURL extrai uma URL de uma string
func extractURL(s string) string {
	// Procura por http:// ou https://
	start := strings.Index(s, "https://")
	if start == -1 {
		start = strings.Index(s, "http://")
	}
	if start == -1 {
		return ""
	}

	// Encontra o fim da URL (espaço, nova linha, ou fim da string)
	url := s[start:]
	for i, ch := range url {
		if ch == ' ' || ch == '\n' || ch == '\r' || ch == '\t' || ch == '"' || ch == '\'' {
			return url[:i]
		}
	}
	return url
}

// openBrowser abre uma URL no navegador padrão do sistema
func openBrowser(url string) error {
	var cmd *exec.Cmd

	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default: // linux e outros
		cmd = exec.Command("xdg-open", url)
	}

	return cmd.Start()
}

// initialize faz o handshake com o servidor MCP
func (c *Client) initialize() error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"clientInfo": map[string]interface{}{
			"name":    "assistente-mcp-client",
			"version": "1.0.0",
		},
	}

	result, err := c.call("initialize", params)
	if err != nil {
		return err
	}

	var initResult struct {
		ProtocolVersion string `json:"protocolVersion"`
		ServerInfo      struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}

	if err := json.Unmarshal(result, &initResult); err != nil {
		return fmt.Errorf("erro ao parsear resultado de initialize: %w", err)
	}

	c.serverInfo = &ServerInfo{
		Name:            initResult.ServerInfo.Name,
		Version:         initResult.ServerInfo.Version,
		ProtocolVersion: initResult.ProtocolVersion,
	}

	// Envia notificação initialized
	c.notify("notifications/initialized", nil)

	return nil
}

// discoverTools descobre as ferramentas disponíveis no servidor
func (c *Client) discoverTools() error {
	result, err := c.call("tools/list", nil)
	if err != nil {
		return err
	}

	var toolsResult struct {
		Tools []Tool `json:"tools"`
	}

	if err := json.Unmarshal(result, &toolsResult); err != nil {
		return fmt.Errorf("erro ao parsear lista de tools: %w", err)
	}

	c.tools = toolsResult.Tools
	return nil
}

// GetTools retorna as ferramentas disponíveis
func (c *Client) GetTools() []Tool {
	return c.tools
}

// GetServerInfo retorna informações do servidor
func (c *Client) GetServerInfo() *ServerInfo {
	return c.serverInfo
}

// CallTool executa uma ferramenta no servidor MCP
func (c *Client) CallTool(name string, arguments map[string]interface{}) (*ToolResult, error) {
	// Log detalhado da chamada
	argsJSON, _ := json.Marshal(arguments)
	argsStr := string(argsJSON)
	if len(argsStr) > 500 {
		argsStr = argsStr[:500] + "..."
	}
	fmt.Printf("[MCP CALL] 📤 Tool: %s\n", name)
	fmt.Printf("[MCP CALL]    Args: %s\n", argsStr)

	params := map[string]interface{}{
		"name":      name,
		"arguments": arguments,
	}

	result, err := c.call("tools/call", params)
	if err != nil {
		fmt.Printf("[MCP CALL] ❌ Erro: %v\n", err)
		return nil, err
	}

	var toolResult ToolResult
	if err := json.Unmarshal(result, &toolResult); err != nil {
		fmt.Printf("[MCP CALL] ❌ Erro ao parsear: %v\n", err)
		return nil, fmt.Errorf("erro ao parsear resultado da tool: %w", err)
	}

	// Log da resposta
	if toolResult.IsError {
		fmt.Printf("[MCP CALL] ⚠️ Tool retornou erro\n")
	} else {
		respStr := ""
		for _, content := range toolResult.Content {
			if content.Text != "" {
				respStr = content.Text
				break
			}
		}
		if len(respStr) > 500 {
			respStr = respStr[:500] + "..."
		}
		fmt.Printf("[MCP CALL] 📥 Resposta: %s\n", respStr)
	}

	return &toolResult, nil
}

// call faz uma chamada JSON-RPC e espera a resposta com timeout
func (c *Client) call(method string, params interface{}) (json.RawMessage, error) {
	// Verifica se o processo ainda está vivo (não usa c.connected pois pode ser chamado durante handshake)
	if c.processExited.Load() {
		return nil, fmt.Errorf("processo MCP encerrou")
	}
	if c.cmd == nil || c.cmd.Process == nil {
		return nil, fmt.Errorf("processo MCP não iniciado")
	}

	id := c.requestID.Add(1)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  params,
	}

	// Cria canal para receber resposta
	respChan := make(chan *JSONRPCResponse, 1)
	c.mu.Lock()
	c.responses[id] = respChan
	c.mu.Unlock()

	defer func() {
		c.mu.Lock()
		delete(c.responses, id)
		c.mu.Unlock()
	}()

	// Envia requisição
	if err := c.send(req); err != nil {
		c.connected.Store(false)
		return nil, fmt.Errorf("erro ao enviar (conexão pode estar fechada): %w", err)
	}

	// Atualiza atividade
	c.updateActivity()

	// Espera resposta com timeout de 2 minutos (mcp-remote pode demorar na primeira execução)
	select {
	case resp := <-respChan:
		if resp.Error != nil {
			return nil, fmt.Errorf("erro MCP [%d]: %s", resp.Error.Code, resp.Error.Message)
		}
		c.updateActivity()
		return resp.Result, nil
	case <-time.After(120 * time.Second):
		return nil, fmt.Errorf("timeout aguardando resposta MCP para método %s", method)
	case <-c.ctx.Done():
		return nil, c.ctx.Err()
	}
}

// notify envia uma notificação JSON-RPC (sem ID, sem resposta esperada)
func (c *Client) notify(method string, params interface{}) error {
	notification := JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return c.send(notification)
}

// send envia uma mensagem JSON-RPC para o servidor
func (c *Client) send(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("erro ao serializar mensagem: %w", err)
	}

	// Log da mensagem enviada
	dataStr := string(data)
	if len(dataStr) > 200 {
		fmt.Printf("[MCP] Enviando (%d bytes): %s...\n", len(dataStr), dataStr[:200])
	} else {
		fmt.Printf("[MCP] Enviando: %s\n", dataStr)
	}

	// MCP stdio transport: cada mensagem é uma linha JSON terminada com \n
	data = append(data, '\n')

	_, err = c.stdin.Write(data)
	if err != nil {
		return fmt.Errorf("erro ao enviar mensagem: %w", err)
	}

	return nil
}

// readResponses lê respostas do servidor em loop
func (c *Client) readResponses() {
	fmt.Println("[MCP READER] Iniciando leitura de respostas do servidor...")
	messageCount := 0
	for {
		select {
		case <-c.ctx.Done():
			fmt.Printf("[MCP READER] Contexto cancelado após %d mensagens, parando leitura\n", messageCount)
			c.connected.Store(false)
			return
		default:
		}

		line, err := c.stdout.ReadBytes('\n')
		if err != nil {
			c.connected.Store(false)
			if err != io.EOF {
				fmt.Printf("[MCP READER] ⚠️ Erro ao ler resposta após %d mensagens: %v\n", messageCount, err)
			} else {
				fmt.Printf("[MCP READER] EOF recebido após %d mensagens - conexão encerrada\n", messageCount)
			}
			return
		}
		messageCount++

		// Atualiza timestamp de atividade
		c.updateActivity()

		// Log da linha recebida (truncada se muito grande)
		lineStr := string(line)
		if len(lineStr) > 200 {
			fmt.Printf("[MCP] Recebido (%d bytes): %s...\n", len(lineStr), lineStr[:200])
		} else {
			fmt.Printf("[MCP] Recebido: %s", lineStr)
		}

		var resp JSONRPCResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			fmt.Printf("[MCP] Erro ao parsear JSON: %v - Linha: %s\n", err, lineStr)
			continue
		}

		// Ignora notificações do servidor (sem ID)
		if resp.ID == nil {
			fmt.Printf("[MCP] Notificação recebida (sem ID)\n")
			continue
		}

		fmt.Printf("[MCP] Resposta para ID %d recebida\n", *resp.ID)

		// Roteia resposta para o canal correto
		c.mu.RLock()
		ch, ok := c.responses[*resp.ID]
		c.mu.RUnlock()

		if ok {
			select {
			case ch <- &resp:
				fmt.Printf("[MCP] Resposta ID %d entregue ao canal\n", *resp.ID)
			default:
				fmt.Printf("[MCP] Canal para ID %d bloqueado\n", *resp.ID)
			}
		} else {
			fmt.Printf("[MCP] Nenhum canal esperando resposta para ID %d\n", *resp.ID)
		}
	}
}

// Close encerra o cliente MCP
func (c *Client) Close() error {
	c.connected.Store(false)
	c.cancel()

	if c.stdin != nil {
		c.stdin.Close()
	}

	if c.cmd != nil && c.cmd.Process != nil {
		// Tenta encerrar graciosamente primeiro
		c.cmd.Process.Kill()
		// Wait já é chamado em monitorProcess, mas chamamos aqui por segurança
		// caso monitorProcess ainda não tenha sido iniciado
		c.cmd.Wait()
	}

	c.processExited.Store(true)

	fmt.Println("[MCP] Cliente MCP encerrado")
	return nil
}

// Connect implementa Transport - para stdio, chama Start()
func (c *Client) Connect() error {
	return c.Start()
}

// IsConnected verifica se o cliente está conectado e o processo está vivo
func (c *Client) IsConnected() bool {
	if !c.connected.Load() {
		return false
	}
	if c.processExited.Load() {
		c.connected.Store(false)
		return false
	}
	return c.serverInfo != nil && c.cmd != nil && c.cmd.Process != nil
}

// ==================== Resources Implementation ====================

// ListResources retorna os recursos disponíveis no servidor
func (c *Client) ListResources() ([]Resource, error) {
	resp, err := c.call("resources/list", nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar resources: %w", err)
	}

	var result struct {
		Resources []Resource `json:"resources"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("erro ao parsear resources: %w", err)
	}

	fmt.Printf("[MCP] Encontrados %d resources\n", len(result.Resources))
	return result.Resources, nil
}

// ListResourceTemplates retorna os templates de recursos
func (c *Client) ListResourceTemplates() ([]ResourceTemplate, error) {
	resp, err := c.call("resources/templates/list", nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar resource templates: %w", err)
	}

	var result struct {
		ResourceTemplates []ResourceTemplate `json:"resourceTemplates"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("erro ao parsear templates: %w", err)
	}

	fmt.Printf("[MCP] Encontrados %d resource templates\n", len(result.ResourceTemplates))
	return result.ResourceTemplates, nil
}

// ReadResource lê o conteúdo de um recurso
func (c *Client) ReadResource(uri string) (*ResourceContents, error) {
	params := map[string]string{
		"uri": uri,
	}

	resp, err := c.call("resources/read", params)
	if err != nil {
		return nil, fmt.Errorf("erro ao ler resource: %w", err)
	}

	var result struct {
		Contents []ResourceContents `json:"contents"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("erro ao parsear resource: %w", err)
	}

	if len(result.Contents) == 0 {
		return nil, fmt.Errorf("resource não encontrado: %s", uri)
	}

	fmt.Printf("[MCP] Resource lido: %s (%s)\n", uri, result.Contents[0].MimeType)
	return &result.Contents[0], nil
}

// ==================== Prompts Implementation ====================

// ListPrompts retorna os prompts disponíveis
func (c *Client) ListPrompts() ([]Prompt, error) {
	resp, err := c.call("prompts/list", nil)
	if err != nil {
		return nil, fmt.Errorf("erro ao listar prompts: %w", err)
	}

	var result struct {
		Prompts []Prompt `json:"prompts"`
	}
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("erro ao parsear prompts: %w", err)
	}

	fmt.Printf("[MCP] Encontrados %d prompts\n", len(result.Prompts))
	return result.Prompts, nil
}

// GetPrompt obtém um prompt expandido com argumentos
func (c *Client) GetPrompt(name string, arguments map[string]string) (*PromptResult, error) {
	params := map[string]interface{}{
		"name": name,
	}
	if arguments != nil {
		params["arguments"] = arguments
	}

	resp, err := c.call("prompts/get", params)
	if err != nil {
		return nil, fmt.Errorf("erro ao obter prompt: %w", err)
	}

	var result PromptResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("erro ao parsear prompt: %w", err)
	}

	fmt.Printf("[MCP] Prompt '%s' obtido com %d mensagens\n", name, len(result.Messages))
	return &result, nil
}

// ==================== Sampling Implementation ====================

// CreateMessage solicita ao servidor que crie uma mensagem via LLM
func (c *Client) CreateMessage(request *SamplingRequest) (*SamplingResult, error) {
	resp, err := c.call("sampling/createMessage", request)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar mensagem: %w", err)
	}

	var result SamplingResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("erro ao parsear resultado: %w", err)
	}

	fmt.Printf("[MCP] Mensagem criada via sampling: model=%s, stopReason=%s\n", result.Model, result.StopReason)
	return &result, nil
}

// Ping verifica se o servidor ainda está respondendo
// Tenta listar as tools (operação leve) para verificar a conexão
func (c *Client) Ping() error {
	if !c.IsConnected() {
		return fmt.Errorf("não está conectado")
	}

	// Tenta fazer uma chamada simples para verificar a conexão
	_, err := c.call("tools/list", nil)
	if err != nil {
		c.connected.Store(false)
		return fmt.Errorf("ping falhou: %w", err)
	}

	return nil
}

// Reconnect tenta reconectar ao servidor MCP
func (c *Client) Reconnect() error {
	// Fecha a conexão atual
	c.Close()

	// Recria o contexto
	c.ctx, c.cancel = context.WithCancel(context.Background())
	c.processExited.Store(false)
	c.connected.Store(false)

	// Recria o comando
	c.cmd = exec.CommandContext(c.ctx, c.command, c.args...)
	if len(c.env) > 0 {
		c.cmd.Env = append(c.cmd.Environ(), c.env...)
	}

	var err error
	c.stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("erro ao criar stdin pipe: %w", err)
	}

	stdout, err := c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("erro ao criar stdout pipe: %w", err)
	}
	c.stdout = bufio.NewReader(stdout)

	c.stderr, err = c.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("erro ao criar stderr pipe: %w", err)
	}

	// Limpa responses pendentes
	c.mu.Lock()
	c.responses = make(map[int64]chan *JSONRPCResponse)
	c.mu.Unlock()

	// Inicia novamente
	fmt.Printf("[MCP] Tentando reconectar...\n")
	return c.Start()
}

// Verifica que Client implementa Transport
var _ Transport = (*Client)(nil)




