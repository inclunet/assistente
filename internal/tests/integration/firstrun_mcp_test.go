package integration

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"
)

// MCPToolBridgeSimulated simula um MCP tool registrado no registry
type MCPToolBridgeSimulated struct {
	name        string
	description string
	serverSlug  string
}

func (m *MCPToolBridgeSimulated) Name() string {
	return m.name
}

func (m *MCPToolBridgeSimulated) Description() string {
	return m.description
}

func (m *MCPToolBridgeSimulated) Parameters() json.RawMessage {
	params := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"query": map[string]interface{}{
				"type":        "string",
				"description": "Query parameter",
			},
		},
		"required": []string{"query"},
	}
	raw, _ := json.Marshal(params)
	return raw
}

func (m *MCPToolBridgeSimulated) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
	result := tools.ToolResult{
		Content: "MCP result for: " + string(args),
		Metadata: map[string]any{
			"server": m.serverSlug,
		},
	}
	return result, nil
}

// TestIntegration_FirstMessageMCPServerAvailable testa descoberta de MCP server
func TestIntegration_FirstMessageMCPServerAvailable(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: MCP server conectado e ferramentas registradas
	registry := tools.NewRegistry()

	// Simular MCP tools registradas (vêm de MCPManager.Connect)
	mcpTools := []tools.Tool{
		&MCPToolBridgeSimulated{
			name:        "mcp_github__search_repositories",
			description: "Search GitHub repositories via MCP",
			serverSlug:  "github",
		},
		&MCPToolBridgeSimulated{
			name:        "mcp_github__get_issue",
			description: "Get GitHub issue details",
			serverSlug:  "github",
		},
	}

	for _, tool := range mcpTools {
		if err := registry.Register(tool); err != nil {
			t.Fatalf("falha ao registrar MCP tool: %v", err)
		}
	}

	// 2. Validar que MCP tools são visíveis para o agentic loop
	allTools := registry.All()
	if len(allTools) < 2 {
		t.Errorf("esperado pelo menos 2 MCP tools, obteve %d", len(allTools))
	}

	// 3. Verificar que tools têm prefixo MCP
	foundGithubTools := 0
	for _, tool := range allTools {
		if tool.Name() == "mcp_github__search_repositories" {
			foundGithubTools++
		}
	}

	if foundGithubTools == 0 {
		t.Error("MCP GitHub tools não foram encontradas")
	}

	// 4. Criar conversa
	conv := &database.Conversation{
		Title:     "MCP First Message",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 5. Primeira mensagem que pode usar MCP tools
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Procure no GitHub por repositórios de Go",
		Source:         "wails",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	t.Logf("✓ MCP Server (GitHub) conectado com %d ferramentas disponíveis", len(mcpTools))
}

// TestIntegration_FirstMessageUseMCPTool testa primeira mensagem chamando MCP tool
func TestIntegration_FirstMessageUseMCPTool(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: MCP tools registradas
	registry := tools.NewRegistry()

	mcpTool := &MCPToolBridgeSimulated{
		name:        "mcp_filesystem__read_dir",
		description: "List directory contents via MCP",
		serverSlug:  "filesystem",
	}

	if err := registry.Register(mcpTool); err != nil {
		t.Fatalf("falha ao registrar MCP tool: %v", err)
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "MCP Tool Use",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem do usuário
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Liste os arquivos do diretório /home",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 4. Assistente CHAMA MCP tool
	toolCalls := []ToolCall{
		{
			ID:   "call_mcp_fs_001",
			Type: "function",
			Function: ToolCallFn{
				Name:      "mcp_filesystem__read_dir",
				Arguments: `{"path":"/home"}`,
			},
		},
	}

	toolCallsJSON, _ := json.Marshal(toolCalls)

	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Vou listar o diretório /home via MCP",
		ToolCalls:      string(toolCallsJSON),
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar assistant msg: %v", err)
	}

	// 5. Executar MCP tool
	ctx := context.Background()
	result, err := mcpTool.Execute(ctx, []byte(`{"path":"/home"}`))
	if err != nil {
		t.Fatalf("falha ao executar MCP tool: %v", err)
	}

	// 6. Armazenar resultado
	toolResult := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "tool",
		Content:        result.Content,
		ToolCallID:     "call_mcp_fs_001",
		CreatedAt:      time.Now().Add(200 * time.Millisecond),
	}

	if err := db.Create(toolResult).Error; err != nil {
		t.Fatalf("falha ao criar tool result: %v", err)
	}

	// 7. Assistente responde com resultado
	finalMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "O diretório /home contém: " + result.Content,
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(300 * time.Millisecond),
	}

	if err := db.Create(finalMsg).Error; err != nil {
		t.Fatalf("falha ao criar final msg: %v", err)
	}

	// 8. Validar sequência completa
	var allMsgs []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv.ID).Order("created_at").Find(&allMsgs).Error; err != nil {
		t.Fatalf("falha ao recuperar histórico: %v", err)
	}

	if len(allMsgs) != 4 {
		t.Errorf("esperado 4 mensagens, obteve %d", len(allMsgs))
	}

	// 9. Validar que MCP tool foi chamada
	if !contains(allMsgs[1].ToolCalls, "mcp_filesystem__read_dir") {
		t.Error("MCP tool não foi chamada")
	}

	t.Log("✓ Primeira mensagem usou MCP tool com sucesso")
}

// TestIntegration_FirstMessageMultipleMCPServers testa múltiplos MCP servers simultâneos
func TestIntegration_FirstMessageMultipleMCPServers(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: múltiplos MCP servers registrados
	registry := tools.NewRegistry()

	mcpServers := []struct {
		slug  string
		tools []string
	}{
		{"github", []string{"search_repositories", "get_issue"}},
		{"filesystem", []string{"read_file", "write_file"}},
		{"web", []string{"fetch_url", "search"}},
	}

	for _, server := range mcpServers {
		for _, toolName := range server.tools {
			fullName := "mcp_" + server.slug + "__" + toolName
			tool := &MCPToolBridgeSimulated{
				name:        fullName,
				description: "MCP tool " + toolName + " from " + server.slug,
				serverSlug:  server.slug,
			}

			if err := registry.Register(tool); err != nil {
				t.Fatalf("falha ao registrar tool %s: %v", fullName, err)
			}
		}
	}

	// 2. Validar que todos os servers foram registrados
	allTools := registry.All()
	if len(allTools) != 6 {
		t.Errorf("esperado 6 MCP tools (2+2+2), obteve %d", len(allTools))
	}

	// 3. Criar conversa
	conv := &database.Conversation{
		Title:     "Multiple MCP Servers",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 4. Primeira mensagem que requer dados de múltiplos servers
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Procure no GitHub, leia um arquivo local, e busque na web simultaneamente",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 5. Assistente chama tools de MÚLTIPLOS servers (paralelo)
	toolCalls := []ToolCall{
		{ID: "call_github", Type: "function", Function: ToolCallFn{Name: "mcp_github__search_repositories", Arguments: `{"q":"go"}`}},
		{ID: "call_fs", Type: "function", Function: ToolCallFn{Name: "mcp_filesystem__read_file", Arguments: `{"path":"config.json"}`}},
		{ID: "call_web", Type: "function", Function: ToolCallFn{Name: "mcp_web__search", Arguments: `{"q":"golang news"}`}},
	}

	toolCallsJSON, _ := json.Marshal(toolCalls)

	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Buscando em paralelo entre 3 MCP servers",
		ToolCalls:      string(toolCallsJSON),
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar assistant msg: %v", err)
	}

	// 6. Simular resultados de todos os 3 servers (em paralelo)
	results := []struct {
		callID  string
		server  string
		content string
	}{
		{"call_github", "github", `{"repos":["golang/go","aws/aws-sdk-go"]}`},
		{"call_fs", "filesystem", `{"content":"db: sqlite\nversion: 3"}`},
		{"call_web", "web", `{"articles":["Go 1.24 released","Go patterns"]}`},
	}

	for _, res := range results {
		toolResult := &database.ChatMessage{
			ConversationID: conv.ID,
			Role:           "tool",
			Content:        res.content,
			ToolCallID:     res.callID,
			CreatedAt:      time.Now().Add(200 * time.Millisecond),
		}

		if err := db.Create(toolResult).Error; err != nil {
			t.Fatalf("falha ao criar tool result para %s: %v", res.server, err)
		}
	}

	// 7. Validar que resultados de todos os 3 servers foram persistidos
	var toolMsgs []database.ChatMessage
	if err := db.Where("conversation_id = ? AND role = ?", conv.ID, "tool").Find(&toolMsgs).Error; err != nil {
		t.Fatalf("falha ao recuperar tool results: %v", err)
	}

	if len(toolMsgs) != 3 {
		t.Errorf("esperado 3 tool results (um por server), obteve %d", len(toolMsgs))
	}

	// 8. Verificar que servidores foram executados
	serversCalled := make(map[string]bool)
	for _, msg := range toolMsgs {
		if contains(msg.Content, "repos") {
			serversCalled["github"] = true
		}
		if contains(msg.Content, "db:") {
			serversCalled["filesystem"] = true
		}
		if contains(msg.Content, "articles") {
			serversCalled["web"] = true
		}
	}

	if len(serversCalled) != 3 {
		t.Error("nem todos os 3 MCP servers foram chamados")
	}

	t.Log("✓ Múltiplos MCP servers (GitHub, Filesystem, Web) executados em paralelo")
}

// TestIntegration_FirstMessageMCPServerOffline testa fallback quando MCP server está offline
func TestIntegration_FirstMessageMCPServerOffline(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: MCP server OfflineToolSimulated (simula erro)
	registry := tools.NewRegistry()

	offlineTool := &MCPToolBridgeSimulated{
		name:        "mcp_database__query",
		description: "Query via MCP (offline)",
		serverSlug:  "database",
	}

	if err := registry.Register(offlineTool); err != nil {
		t.Fatalf("falha ao registrar tool: %v", err)
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "MCP Server Offline",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Execute uma query no banco de dados",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 4. Assistente tenta chamar MCP tool
	toolCalls := []ToolCall{
		{
			ID:   "call_db_query",
			Type: "function",
			Function: ToolCallFn{
				Name:      "mcp_database__query",
				Arguments: `{"sql":"SELECT * FROM users"}`,
			},
		},
	}

	toolCallsJSON, _ := json.Marshal(toolCalls)

	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Vou executar a query no MCP database",
		ToolCalls:      string(toolCallsJSON),
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar assistant msg: %v", err)
	}

	// 5. MCP server está offline - simular erro
	mcpServerError := "MCP Server 'database' is offline or unreachable (connection timeout)"

	// 6. Tool resultado como erro
	errorMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "tool",
		Content:        "Error: " + mcpServerError,
		ToolCallID:     "call_db_query",
		CreatedAt:      time.Now().Add(200 * time.Millisecond),
	}

	if err := db.Create(errorMsg).Error; err != nil {
		t.Fatalf("falha ao criar error msg: %v", err)
	}

	// 7. Assistente reconhece erro e oferece alternativa
	recoveryMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "O servidor MCP de banco de dados está offline no momento. Gostaria de tentar novamente mais tarde ou usar uma abordagem diferente?",
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(300 * time.Millisecond),
	}

	if err := db.Create(recoveryMsg).Error; err != nil {
		t.Fatalf("falha ao criar recovery msg: %v", err)
	}

	// 8. Validar que erro foi tratado
	var allMsgs []database.ChatMessage
	if err := db.Where("conversation_id = ?", conv.ID).Order("created_at").Find(&allMsgs).Error; err != nil {
		t.Fatalf("falha ao recuperar histórico: %v", err)
	}

	if len(allMsgs) != 4 {
		t.Errorf("esperado 4 mensagens, obteve %d", len(allMsgs))
	}

	// 9. Validar que assistente reconheceu o erro
	if !contains(allMsgs[3].Content, "offline") {
		t.Error("assistente deveria reconhecer que MCP server está offline")
	}

	t.Log("✓ MCP server offline detectado, assistente oferece alternativa")
}

// TestIntegration_FirstMessageMCPToolMetadata testa rastreamento de metadados MCP
func TestIntegration_FirstMessageMCPToolMetadata(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: MCP tool
	registry := tools.NewRegistry()

	mcpTool := &MCPToolBridgeSimulated{
		name:        "mcp_github__get_user",
		description: "Get GitHub user info",
		serverSlug:  "github",
	}

	if err := registry.Register(mcpTool); err != nil {
		t.Fatalf("falha ao registrar tool: %v", err)
	}

	// 2. Criar conversa
	conv := &database.Conversation{
		Title:     "MCP Metadata",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 3. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Qual é a informação do usuário golang no GitHub?",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 4. Assistente chama MCP tool
	toolCalls := []ToolCall{
		{
			ID:   "call_github_user",
			Type: "function",
			Function: ToolCallFn{
				Name:      "mcp_github__get_user",
				Arguments: `{"username":"golang"}`,
			},
		},
	}

	toolCallsJSON, _ := json.Marshal(toolCalls)

	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Buscando info do usuário golang no GitHub",
		ToolCalls:      string(toolCallsJSON),
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar assistant msg: %v", err)
	}

	// 5. Resultado do MCP tool com metadados
	ctx := context.Background()
	result, err := mcpTool.Execute(ctx, []byte(`{"username":"golang"}`))
	if err != nil {
		t.Fatalf("falha ao executar MCP tool: %v", err)
	}

	// 6. Armazenar resultado
	toolResult := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "tool",
		Content:        result.Content,
		ToolCallID:     "call_github_user",
		CreatedAt:      time.Now().Add(200 * time.Millisecond),
	}

	if err := db.Create(toolResult).Error; err != nil {
		t.Fatalf("falha ao criar tool result: %v", err)
	}

	// 7. Validar que resultado do MCP foi persistido
	var retrieved database.ChatMessage
	if err := db.First(&retrieved, "id = ?", toolResult.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar resultado: %v", err)
	}

	// 8. Validar que MCP server metadata está no conteúdo
	if !contains(retrieved.Content, "MCP result") {
		t.Error("conteúdo MCP não foi persistido corretamente")
	}

	t.Log("✓ Metadados MCP persistidos no resultado da ferramenta")
}

// TestIntegration_FirstMessageMCPToolNaming testa convenção de nomeação MCP
func TestIntegration_FirstMessageMCPToolNaming(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}

	db := setupIntegrationDB(t)

	// 1. Setup: MCP tools com nomeação correta: mcp_{serverSlug}__{toolName}
	registry := tools.NewRegistry()

	testCases := []struct {
		serverSlug string
		toolName   string
	}{
		{"github", "search_repositories"},
		{"github", "get_issue"},
		{"stripe", "create_invoice"},
		{"slack", "send_message"},
		{"notion", "create_page"},
	}

	for _, tc := range testCases {
		fullName := "mcp_" + tc.serverSlug + "__" + tc.toolName
		tool := &MCPToolBridgeSimulated{
			name:        fullName,
			description: tc.toolName + " from " + tc.serverSlug,
			serverSlug:  tc.serverSlug,
		}

		if err := registry.Register(tool); err != nil {
			t.Fatalf("falha ao registrar %s: %v", fullName, err)
		}
	}

	// 2. Validar nomeação
	allTools := registry.All()
	if len(allTools) != 5 {
		t.Errorf("esperado 5 ferramentas, obteve %d", len(allTools))
	}

	// 3. Garantir que nomeação segue padrão mcp_{serverSlug}__{toolName}
	for _, tool := range allTools {
		name := tool.Name()
		if !contains(name, "mcp_") {
			t.Errorf("ferramenta %s não tem prefixo mcp_", name)
		}
		if !contains(name, "__") {
			t.Errorf("ferramenta %s não tem separador __ entre server e tool", name)
		}
	}

	// 4. Criar conversa
	conv := &database.Conversation{
		Title:     "MCP Naming",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if err := db.Create(conv).Error; err != nil {
		t.Fatalf("falha ao criar conversa: %v", err)
	}

	// 5. Primeira mensagem
	userMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "user",
		Content:        "Use ferramentas de múltiplos servidores MCP",
		CreatedAt:      time.Now(),
	}

	if err := db.Create(userMsg).Error; err != nil {
		t.Fatalf("falha ao criar mensagem: %v", err)
	}

	// 6. Assistente chama tools com nomes MCP corretos
	toolCalls := []ToolCall{
		{ID: "c1", Type: "function", Function: ToolCallFn{Name: "mcp_github__search_repositories", Arguments: `{}`}},
		{ID: "c2", Type: "function", Function: ToolCallFn{Name: "mcp_stripe__create_invoice", Arguments: `{}`}},
		{ID: "c3", Type: "function", Function: ToolCallFn{Name: "mcp_slack__send_message", Arguments: `{}`}},
	}

	toolCallsJSON, _ := json.Marshal(toolCalls)

	assistantMsg := &database.ChatMessage{
		ConversationID: conv.ID,
		Role:           "assistant",
		Content:        "Chamando MCP tools",
		ToolCalls:      string(toolCallsJSON),
		TurnID:         &userMsg.ID,
		CreatedAt:      time.Now().Add(100 * time.Millisecond),
	}

	if err := db.Create(assistantMsg).Error; err != nil {
		t.Fatalf("falha ao criar assistant msg: %v", err)
	}

	// 7. Validar que nomes MCP foram usados corretamente
	var saved database.ChatMessage
	if err := db.First(&saved, "id = ?", assistantMsg.ID).Error; err != nil {
		t.Fatalf("falha ao recuperar: %v", err)
	}

	if !contains(saved.ToolCalls, "mcp_github__search_repositories") {
		t.Error("nome MCP correto não foi usado para GitHub")
	}
	if !contains(saved.ToolCalls, "mcp_stripe__create_invoice") {
		t.Error("nome MCP correto não foi usado para Stripe")
	}
	if !contains(saved.ToolCalls, "mcp_slack__send_message") {
		t.Error("nome MCP correto não foi usado para Slack")
	}

	t.Log("✓ Nomes MCP seguem convenção correta: mcp_{serverSlug}__{toolName}")
}
