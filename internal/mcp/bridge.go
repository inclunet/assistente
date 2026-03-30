package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"

	"assistente/internal/tools"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

// MCPToolBridge adapta uma tool MCP para a interface tools.Tool do nosso sistema.
// Cada instância representa uma tool de um servidor MCP específico.
type MCPToolBridge struct {
	serverSlug     string
	toolName       string // nome original da tool no servidor MCP
	fullName       string // nome registrado no registry (namespaced)
	description    string
	inputSchema    json.RawMessage
	session        *mcpsdk.ClientSession
	onSessionError func(slug string, err error) // notifica o Manager sobre erros de transporte/sessão
}

// NewMCPToolBridge cria um bridge para uma tool MCP.
func NewMCPToolBridge(serverSlug string, mcpTool *mcpsdk.Tool, session *mcpsdk.ClientSession) *MCPToolBridge {
	// Serializa o inputSchema para JSON
	var schema json.RawMessage
	if mcpTool.InputSchema != nil {
		data, err := json.Marshal(mcpTool.InputSchema)
		if err == nil {
			schema = data
		}
	}
	if schema == nil {
		schema = json.RawMessage(`{"type":"object","properties":{}}`)
	}

	return &MCPToolBridge{
		serverSlug:  serverSlug,
		toolName:    mcpTool.Name,
		fullName:    BuildToolName(serverSlug, mcpTool.Name),
		description: mcpTool.Description,
		inputSchema: schema,
		session:     session,
	}
}

// BuildToolName constrói o nome namespaced de uma tool MCP.
// Formato: "mcp_{serverSlug}__{toolName}"
func BuildToolName(serverSlug, toolName string) string {
	return fmt.Sprintf("mcp_%s__%s", serverSlug, toolName)
}

// ParseToolName extrai serverSlug e toolName de um nome namespaced.
// Retorna ("", "", false) se o nome não for um tool MCP válido.
func ParseToolName(fullName string) (serverSlug, toolName string, ok bool) {
	if !strings.HasPrefix(fullName, "mcp_") {
		return "", "", false
	}
	rest := fullName[4:] // remove "mcp_"
	parts := strings.SplitN(rest, "__", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// Name retorna o nome completo (namespaced) da tool.
func (b *MCPToolBridge) Name() string {
	return b.fullName
}

// Description retorna a descrição original da tool MCP.
func (b *MCPToolBridge) Description() string {
	return b.description
}

// Parameters retorna o JSON Schema dos parâmetros da tool MCP.
func (b *MCPToolBridge) Parameters() json.RawMessage {
	return b.inputSchema
}

// Execute chama a tool no servidor MCP via session.CallTool.
func (b *MCPToolBridge) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	var arguments map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return tools.ToolResult{
				Content: fmt.Sprintf("Erro ao parsear argumentos: %v", err),
				IsError: true,
			}, nil
		}
	}

	result, err := b.session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      b.toolName,
		Arguments: arguments,
	})
	if err != nil {
		if b.onSessionError != nil && isSessionOrTransportError(err) {
			go b.onSessionError(b.serverSlug, err)
		}
		return tools.ToolResult{
			Content: fmt.Sprintf("Erro ao chamar tool MCP '%s' no servidor '%s': %v", b.toolName, b.serverSlug, err),
			IsError: true,
		}, nil
	}

	content := extractTextContent(result)

	return tools.ToolResult{
		Content: content,
		IsError: result.IsError,
		Metadata: map[string]any{
			"mcpServer": b.serverSlug,
			"mcpTool":   b.toolName,
		},
	}, nil
}

// isSessionOrTransportError detecta erros que indicam que a sessão ou transporte
// estão quebrados e uma reconexão é necessária.
func isSessionOrTransportError(err error) bool {
	if err == nil {
		return false
	}

	// EOF / conexão fechada
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}

	// Erros de rede
	var netErr *net.OpError
	if errors.As(err, &netErr) {
		return true
	}

	// SessionExpiredError do nosso round tripper
	var sessErr *SessionExpiredError
	if errors.As(err, &sessErr) {
		return true
	}

	msg := strings.ToLower(err.Error())
	sessionIndicators := []string{
		"connection reset",
		"connection refused",
		"connection closed",
		"client is closing",
		"broken pipe",
		"session expired",
		"session not found",
		"use of closed network connection",
		"exceeded",
		"eof",
	}
	for _, indicator := range sessionIndicators {
		if strings.Contains(msg, indicator) {
			return true
		}
	}

	return false
}

// extractTextContent extrai o conteúdo textual de um CallToolResult.
// Concatena todos os TextContent separados por newline.
func extractTextContent(result *mcpsdk.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}

	var parts []string
	for _, content := range result.Content {
		switch c := content.(type) {
		case *mcpsdk.TextContent:
			parts = append(parts, c.Text)
		case *mcpsdk.ImageContent:
			parts = append(parts, fmt.Sprintf("[Imagem: %s, %d bytes]", c.MIMEType, len(c.Data)))
		case *mcpsdk.AudioContent:
			parts = append(parts, fmt.Sprintf("[Áudio: %s, %d bytes]", c.MIMEType, len(c.Data)))
		case *mcpsdk.EmbeddedResource:
			// Tenta extrair texto do recurso embutido
			if c.Resource != nil && c.Resource.Text != "" {
				parts = append(parts, c.Resource.Text)
			} else {
				parts = append(parts, "[Recurso embutido]")
			}
		default:
			// Tenta serializar como JSON como fallback
			data, err := json.Marshal(content)
			if err == nil {
				parts = append(parts, string(data))
			}
		}
	}

	return strings.Join(parts, "\n")
}

// ToMCPToolInfo converte informações do bridge para MCPToolInfo (formato frontend).
func (b *MCPToolBridge) ToMCPToolInfo() MCPToolInfo {
	return MCPToolInfo{
		Name:        b.toolName,
		FullName:    b.fullName,
		Description: b.description,
		Schema:      b.inputSchema,
		ServerSlug:  b.serverSlug,
	}
}

// Garante que MCPToolBridge implementa tools.Tool em tempo de compilação.
var _ tools.Tool = (*MCPToolBridge)(nil)
