package mcp

/*
MCP HTTP/SSE Client - Model Context Protocol over HTTP

Implementa o transporte HTTP/SSE do protocolo MCP:
- Requests enviados via HTTP POST para /message
- Respostas recebidas via SSE (Server-Sent Events) ou HTTP direto
- Suporte a autenticação via Bearer token ou API Key

Referência: https://spec.modelcontextprotocol.io/specification/basic/transports/#http-with-sse
*/

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"assistente/internal/llm"
)

// HTTPClient gerencia a comunicação com um servidor MCP via HTTP/SSE
type HTTPClient struct {
	// Configuração
	BaseURL   string            // URL base do servidor MCP (ex: https://mcp.example.com)
	Headers   map[string]string // Headers adicionais (auth, etc)
	AuthType  string            // none, bearer, api_key
	AuthValue string            // Token ou API Key

	// HTTP Client
	client *http.Client

	// Estado
	requestID  atomic.Int64
	serverInfo *ServerInfo
	tools      []Tool

	// SSE
	sseEndpoint string // Endpoint SSE (recebido no initialize)
	sseCancel   context.CancelFunc
	sseEvents   chan *sseEvent
	responses   map[int64]chan *JSONRPCResponse
	mu          sync.RWMutex

	ctx    context.Context
	cancel context.CancelFunc
}

// sseEvent representa um evento SSE
type sseEvent struct {
	Event string
	Data  string
	ID    string
}

// HTTPConfig contém a configuração para criar um HTTPClient
type HTTPConfig struct {
	BaseURL   string
	AuthType  string // none, bearer, api_key
	AuthValue string
	Headers   map[string]string
	Timeout   time.Duration
}

// NewHTTPClient cria um novo cliente MCP HTTP/SSE
func NewHTTPClient(config HTTPConfig) *HTTPClient {
	ctx, cancel := context.WithCancel(context.Background())

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &HTTPClient{
		BaseURL:   strings.TrimSuffix(config.BaseURL, "/"),
		Headers:   config.Headers,
		AuthType:  config.AuthType,
		AuthValue: config.AuthValue,
		client:    llm.NewHTTPClientWithTimeout(timeout), // Usa pool compartilhado
		responses: make(map[int64]chan *JSONRPCResponse),
		sseEvents: make(chan *sseEvent, 100),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Connect conecta ao servidor MCP e faz o handshake
func (c *HTTPClient) Connect() error {
	// Faz o handshake inicial
	if err := c.initialize(); err != nil {
		return fmt.Errorf("erro no handshake MCP HTTP: %w", err)
	}

	// Inicia listener SSE se o servidor forneceu endpoint
	if c.sseEndpoint != "" {
		go c.listenSSE()
	}

	// Descobre as tools disponíveis
	if err := c.discoverTools(); err != nil {
		return fmt.Errorf("erro ao descobrir tools: %w", err)
	}

	return nil
}

// initialize faz o handshake com o servidor MCP
func (c *HTTPClient) initialize() error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"capabilities": map[string]interface{}{
			"tools": map[string]interface{}{},
		},
		"clientInfo": map[string]interface{}{
			"name":    "assistente-mcp-http-client",
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
		// Alguns servidores retornam endpoint SSE aqui
		Meta struct {
			SSEEndpoint string `json:"sseEndpoint,omitempty"`
		} `json:"_meta,omitempty"`
	}

	if err := json.Unmarshal(result, &initResult); err != nil {
		return fmt.Errorf("erro ao parsear resultado de initialize: %w", err)
	}

	c.serverInfo = &ServerInfo{
		Name:            initResult.ServerInfo.Name,
		Version:         initResult.ServerInfo.Version,
		ProtocolVersion: initResult.ProtocolVersion,
	}

	// Guarda endpoint SSE se fornecido
	if initResult.Meta.SSEEndpoint != "" {
		c.sseEndpoint = initResult.Meta.SSEEndpoint
	}

	// Envia notificação initialized
	c.notify("notifications/initialized", nil)

	return nil
}

// discoverTools descobre as ferramentas disponíveis no servidor
func (c *HTTPClient) discoverTools() error {
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
func (c *HTTPClient) GetTools() []Tool {
	return c.tools
}

// GetServerInfo retorna informações do servidor
func (c *HTTPClient) GetServerInfo() *ServerInfo {
	return c.serverInfo
}

// CallTool executa uma ferramenta no servidor MCP
func (c *HTTPClient) CallTool(name string, arguments map[string]interface{}) (*ToolResult, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": arguments,
	}

	result, err := c.call("tools/call", params)
	if err != nil {
		return nil, err
	}

	var toolResult ToolResult
	if err := json.Unmarshal(result, &toolResult); err != nil {
		return nil, fmt.Errorf("erro ao parsear resultado da tool: %w", err)
	}

	return &toolResult, nil
}

// call faz uma chamada JSON-RPC via HTTP e espera a resposta
func (c *HTTPClient) call(method string, params interface{}) (json.RawMessage, error) {
	id := c.requestID.Add(1)

	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      &id,
		Method:  method,
		Params:  params,
	}

	// Se temos SSE, registra canal para resposta
	if c.sseEndpoint != "" {
		respChan := make(chan *JSONRPCResponse, 1)
		c.mu.Lock()
		c.responses[id] = respChan
		c.mu.Unlock()

		defer func() {
			c.mu.Lock()
			delete(c.responses, id)
			c.mu.Unlock()
		}()

		// Envia request
		if err := c.sendHTTPRequest(req); err != nil {
			return nil, err
		}

		// Espera resposta via SSE
		select {
		case resp := <-respChan:
			if resp.Error != nil {
				return nil, fmt.Errorf("erro MCP [%d]: %s", resp.Error.Code, resp.Error.Message)
			}
			return resp.Result, nil
		case <-time.After(30 * time.Second):
			return nil, fmt.Errorf("timeout esperando resposta MCP")
		case <-c.ctx.Done():
			return nil, c.ctx.Err()
		}
	}

	// Sem SSE, faz request/response síncrono
	return c.sendSyncRequest(req)
}

// notify envia uma notificação JSON-RPC (sem esperar resposta)
func (c *HTTPClient) notify(method string, params interface{}) error {
	notification := JSONRPCNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}

	return c.sendHTTPRequest(notification)
}

// sendSyncRequest envia um request e espera resposta no mesmo HTTP response
func (c *HTTPClient) sendSyncRequest(req JSONRPCRequest) (json.RawMessage, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("erro ao serializar request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(c.ctx, "POST", c.BaseURL+"/message", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("erro ao criar request: %w", err)
	}

	c.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("erro ao enviar request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("servidor retornou status %d: %s", resp.StatusCode, string(body))
	}

	var rpcResp JSONRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return nil, fmt.Errorf("erro ao parsear resposta: %w", err)
	}

	if rpcResp.Error != nil {
		return nil, fmt.Errorf("erro MCP [%d]: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return rpcResp.Result, nil
}

// sendHTTPRequest envia uma mensagem via HTTP POST
func (c *HTTPClient) sendHTTPRequest(msg interface{}) error {
	body, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("erro ao serializar mensagem: %w", err)
	}

	req, err := http.NewRequestWithContext(c.ctx, "POST", c.BaseURL+"/message", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("erro ao criar request: %w", err)
	}

	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("erro ao enviar request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("servidor retornou status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// setHeaders configura os headers de autenticação e customizados
func (c *HTTPClient) setHeaders(req *http.Request) {
	// Headers customizados
	for k, v := range c.Headers {
		req.Header.Set(k, v)
	}

	// Autenticação
	switch c.AuthType {
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+c.AuthValue)
	case "api_key":
		req.Header.Set("X-API-Key", c.AuthValue)
	}
}

// listenSSE escuta eventos SSE do servidor
func (c *HTTPClient) listenSSE() {
	endpoint := c.sseEndpoint
	if !strings.HasPrefix(endpoint, "http") {
		endpoint = c.BaseURL + endpoint
	}

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		if err := c.connectSSE(endpoint); err != nil {
			fmt.Printf("Erro SSE, reconectando em 5s: %v\n", err)
			time.Sleep(5 * time.Second)
		}
	}
}

// connectSSE conecta ao endpoint SSE e processa eventos
func (c *HTTPClient) connectSSE(endpoint string) error {
	req, err := http.NewRequestWithContext(c.ctx, "GET", endpoint, nil)
	if err != nil {
		return err
	}

	c.setHeaders(req)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("Connection", "keep-alive")

	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SSE retornou status %d", resp.StatusCode)
	}

	reader := bufio.NewReader(resp.Body)
	var event sseEvent

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		line = strings.TrimSpace(line)

		if line == "" {
			// Fim do evento
			if event.Data != "" {
				c.processSSEEvent(&event)
			}
			event = sseEvent{}
			continue
		}

		if strings.HasPrefix(line, "event:") {
			event.Event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			event.Data = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		} else if strings.HasPrefix(line, "id:") {
			event.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
		}
	}
}

// processSSEEvent processa um evento SSE recebido
func (c *HTTPClient) processSSEEvent(event *sseEvent) {
	// Tenta parsear como JSON-RPC response
	var resp JSONRPCResponse
	if err := json.Unmarshal([]byte(event.Data), &resp); err != nil {
		return
	}

	// Se é uma notificação (sem ID), ignora por enquanto
	if resp.ID == nil {
		return
	}

	// Roteia para o canal correto
	c.mu.RLock()
	ch, ok := c.responses[*resp.ID]
	c.mu.RUnlock()

	if ok {
		select {
		case ch <- &resp:
		default:
		}
	}
}

// Close encerra o cliente MCP HTTP
func (c *HTTPClient) Close() error {
	c.cancel()
	return nil
}

// IsConnected verifica se está conectado
func (c *HTTPClient) IsConnected() bool {
	return c.serverInfo != nil
}

// ==================== Resources Implementation ====================

// ListResources retorna os recursos disponíveis no servidor
func (c *HTTPClient) ListResources() ([]Resource, error) {
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

	return result.Resources, nil
}

// ListResourceTemplates retorna os templates de recursos
func (c *HTTPClient) ListResourceTemplates() ([]ResourceTemplate, error) {
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

	return result.ResourceTemplates, nil
}

// ReadResource lê o conteúdo de um recurso
func (c *HTTPClient) ReadResource(uri string) (*ResourceContents, error) {
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

	return &result.Contents[0], nil
}

// ==================== Prompts Implementation ====================

// ListPrompts retorna os prompts disponíveis
func (c *HTTPClient) ListPrompts() ([]Prompt, error) {
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

	return result.Prompts, nil
}

// GetPrompt obtém um prompt expandido com argumentos
func (c *HTTPClient) GetPrompt(name string, arguments map[string]string) (*PromptResult, error) {
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

	return &result, nil
}

// ==================== Sampling Implementation ====================

// CreateMessage solicita ao servidor que crie uma mensagem via LLM
func (c *HTTPClient) CreateMessage(request *SamplingRequest) (*SamplingResult, error) {
	resp, err := c.call("sampling/createMessage", request)
	if err != nil {
		return nil, fmt.Errorf("erro ao criar mensagem: %w", err)
	}

	var result SamplingResult
	if err := json.Unmarshal(resp, &result); err != nil {
		return nil, fmt.Errorf("erro ao parsear resultado: %w", err)
	}

	return &result, nil
}

// Verifica que HTTPClient implementa Transport
var _ Transport = (*HTTPClient)(nil)




