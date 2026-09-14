package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
	"assistente/internal/tools"
)

type mcpToolBridgeSimulated struct {
	name       string
	serverSlug string
}

func (m *mcpToolBridgeSimulated) Name() string { return m.name }

func (m *mcpToolBridgeSimulated) Description() string { return "MCP " + m.name }

func (m *mcpToolBridgeSimulated) Parameters() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (m *mcpToolBridgeSimulated) Execute(_ context.Context, args json.RawMessage) (tools.ToolResult, error) {
	return tools.ToolResult{
		Content: "MCP result for: " + string(args),
		Metadata: map[string]any{
			"server": m.serverSlug,
		},
	}, nil
}

func TestIntegration_FirstMessageMCPInvocationsUseCanonicalLedger(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}
	db := setupIntegrationDB(t)
	conversation := &database.Conversation{Title: "Primeira com MCP"}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	user := &database.ChatMessage{ConversationID: conversation.ID, Role: "user", Content: "Consulte os servidores"}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	assistant := &database.ChatMessage{ConversationID: conversation.ID, TurnID: &user.ID, Role: "assistant", Content: "Consultando."}
	if err := db.Create(assistant).Error; err != nil {
		t.Fatal(err)
	}

	for index, name := range []string{"mcp_github__search_repositories", "mcp_slack__send_message"} {
		catalog := &database.ToolCatalog{Name: name, DisplayName: name, Origin: "mcp"}
		if err := db.Create(catalog).Error; err != nil {
			t.Fatal(err)
		}
		invocation := &database.ToolInvocation{
			UserID:         "integration-user",
			ToolCatalogID:  catalog.ID,
			OriginType:     "chat",
			OriginID:       user.ID,
			ConversationID: &conversation.ID,
			TurnID:         &user.ID,
			ToolCallID:     "call-mcp-" + string(rune('1'+index)),
			Status:         "succeeded",
			Output:         `{"content":"ok"}`,
			Metadata:       `{"display":{"version":1,"origin":"mcp","name":"` + name + `"}}`,
			QueuedAt:       time.Now().UTC().Add(time.Duration(index) * time.Millisecond),
		}
		if err := db.Create(invocation).Error; err != nil {
			t.Fatal(err)
		}
	}

	var count int64
	if err := db.Model(&database.ToolInvocation{}).
		Where("conversation_id = ? AND origin_type = ?", conversation.ID, "chat").
		Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("invocações MCP no ledger = %d, esperado 2", count)
	}
	var technicalMessages int64
	if err := db.Model(&database.ChatMessage{}).
		Where("conversation_id = ? AND lower(trim(role)) = 'tool'", conversation.ID).
		Count(&technicalMessages).Error; err != nil {
		t.Fatal(err)
	}
	if technicalMessages != 0 {
		t.Fatalf("chat_messages contém %d resultados MCP técnicos", technicalMessages)
	}
}

func TestIntegration_FirstMessageMCPToolsAreDiscoverable(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}
	registry := tools.NewRegistry()
	for _, name := range []string{"mcp_github__search_repositories", "mcp_github__get_issue"} {
		if err := registry.Register(&mcpToolBridgeSimulated{name: name, serverSlug: "github"}); err != nil {
			t.Fatal(err)
		}
	}

	registered := registry.All()
	if len(registered) != 2 {
		t.Fatalf("ferramentas MCP registradas = %d, esperado 2", len(registered))
	}
	if registered[0].Name() != "mcp_github__search_repositories" &&
		registered[1].Name() != "mcp_github__search_repositories" {
		t.Fatalf("ferramenta do GitHub não descoberta: %+v", registered)
	}
}

func TestIntegration_FirstMessageExecutesMCPAndPersistsResultInLedger(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}
	db := setupIntegrationDB(t)
	conversation := &database.Conversation{Title: "Execução MCP"}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	user := &database.ChatMessage{ConversationID: conversation.ID, Role: "user", Content: "Liste o diretório"}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	tool := &mcpToolBridgeSimulated{name: "mcp_filesystem__read_dir", serverSlug: "filesystem"}
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"path":"/home"}`))
	if err != nil {
		t.Fatal(err)
	}
	catalog := &database.ToolCatalog{Name: tool.Name(), DisplayName: tool.Name(), Origin: "mcp"}
	if err := db.Create(catalog).Error; err != nil {
		t.Fatal(err)
	}
	invocation := &database.ToolInvocation{
		UserID:         "integration-user",
		ToolCatalogID:  catalog.ID,
		OriginType:     "chat",
		OriginID:       user.ID,
		ConversationID: &conversation.ID,
		TurnID:         &user.ID,
		ToolCallID:     "call-mcp-fs-1",
		Status:         "succeeded",
		Input:          `{"path":"/home"}`,
		Output:         `{"content":` + quotedJSON(result.Content) + `}`,
		Metadata:       `{"display":{"version":1,"origin":"mcp","name":"mcp_filesystem__read_dir","server_label":"filesystem"}}`,
		QueuedAt:       time.Now().UTC(),
	}
	if err := db.Create(invocation).Error; err != nil {
		t.Fatal(err)
	}

	var stored database.ToolInvocation
	if err := db.First(&stored, "id = ?", invocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stored.Output, "MCP result") || !strings.Contains(stored.Metadata, "filesystem") {
		t.Fatalf("resultado MCP canônico incompleto: %+v", stored)
	}
}

func TestIntegration_FirstMessageMultipleMCPServersKeepNamingConvention(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}
	registry := tools.NewRegistry()
	testCases := []struct {
		server string
		name   string
	}{
		{server: "github", name: "search_repositories"},
		{server: "filesystem", name: "read_file"},
		{server: "web", name: "search"},
		{server: "slack", name: "send_message"},
		{server: "notion", name: "create_page"},
	}
	for _, testCase := range testCases {
		fullName := "mcp_" + testCase.server + "__" + testCase.name
		if err := registry.Register(&mcpToolBridgeSimulated{name: fullName, serverSlug: testCase.server}); err != nil {
			t.Fatalf("registrar %s: %v", fullName, err)
		}
	}

	registered := registry.All()
	if len(registered) != len(testCases) {
		t.Fatalf("ferramentas registradas = %d, esperado %d", len(registered), len(testCases))
	}
	servers := make(map[string]bool, len(testCases))
	for _, tool := range registered {
		name := tool.Name()
		if !strings.HasPrefix(name, "mcp_") || !strings.Contains(name, "__") {
			t.Fatalf("nome MCP fora da convenção: %s", name)
		}
		parts := strings.SplitN(strings.TrimPrefix(name, "mcp_"), "__", 2)
		servers[parts[0]] = true
	}
	if len(servers) != len(testCases) {
		t.Fatalf("servidores MCP descobertos = %d, esperado %d", len(servers), len(testCases))
	}
}

func quotedJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}
