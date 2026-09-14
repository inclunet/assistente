package integration

import (
	"context"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
)

func TestIntegration_FirstMessagePersistsToolOnlyInLedger(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}
	db := setupIntegrationDB(t)

	conversation := &database.Conversation{Title: "Primeira com ferramenta"}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	user := &database.ChatMessage{ConversationID: conversation.ID, Role: "user", Content: "Leia config.json"}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	assistant := &database.ChatMessage{
		ConversationID: conversation.ID,
		TurnID:         &user.ID,
		Role:           "assistant",
		Content:        "Vou ler o arquivo.",
	}
	if err := db.Create(assistant).Error; err != nil {
		t.Fatal(err)
	}
	catalog := &database.ToolCatalog{Name: "read_file", DisplayName: "Ler arquivo", Origin: "builtin"}
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
		ToolCallID:     "call-read-1",
		Status:         "succeeded",
		Input:          `{"path":"config.json"}`,
		Output:         `{"content":"resultado"}`,
		Metadata:       `{"display":{"version":1,"name":"read_file"}}`,
		QueuedAt:       time.Now().UTC(),
	}
	if err := db.Create(invocation).Error; err != nil {
		t.Fatal(err)
	}

	var messages []database.ChatMessage
	if err := db.Where("conversation_id = ?", conversation.ID).Order("created_at").Find(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("chat_messages = %d, esperado somente user+assistant", len(messages))
	}
	for _, message := range messages {
		if strings.EqualFold(strings.TrimSpace(message.Role), "tool") {
			t.Fatalf("chat_messages contém role=tool: %+v", message)
		}
	}
	var stored database.ToolInvocation
	if err := db.First(&stored, "id = ?", invocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.ToolCallID != "call-read-1" || !strings.Contains(stored.Input, "config.json") ||
		!strings.Contains(stored.Output, "resultado") {
		t.Fatalf("invocação canônica incompleta: %+v", stored)
	}
}

func TestIntegration_ChatMessagesRejectsToolRoleAtRepositoryBoundary(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}
	db := setupIntegrationDB(t)
	conversation := &database.Conversation{UserID: "integration-user", Title: "Restrição de mensagens"}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), "integration-user")
	_, err := database.CreateMessageWithContext(ctx, database.MessageOptions{
		ConversationID: conversation.ID,
		Role:           " Tool ",
		Content:        "resultado técnico",
	})
	if err == nil {
		t.Fatal("schema aceitou role=tool em chat_messages")
	}
}

func TestIntegration_FirstMessagePersistsMultipleToolsInLedger(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}
	db := setupIntegrationDB(t)
	conversation := &database.Conversation{Title: "Múltiplas ferramentas"}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	user := &database.ChatMessage{ConversationID: conversation.ID, Role: "user", Content: "Leia dois arquivos"}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	catalog := &database.ToolCatalog{Name: "read_file", DisplayName: "Ler arquivo", Origin: "builtin"}
	if err := db.Create(catalog).Error; err != nil {
		t.Fatal(err)
	}
	for index, path := range []string{"config.json", "main.go"} {
		invocation := &database.ToolInvocation{
			UserID:         "integration-user",
			ToolCatalogID:  catalog.ID,
			OriginType:     "chat",
			OriginID:       user.ID,
			ConversationID: &conversation.ID,
			TurnID:         &user.ID,
			ToolCallID:     "call-read-" + string(rune('1'+index)),
			Status:         "succeeded",
			Input:          `{"path":"` + path + `"}`,
			Output:         `{"content":"ok"}`,
			QueuedAt:       time.Now().UTC().Add(time.Duration(index) * time.Millisecond),
		}
		if err := db.Create(invocation).Error; err != nil {
			t.Fatal(err)
		}
	}

	var invocations []database.ToolInvocation
	if err := db.Where("conversation_id = ?", conversation.ID).Order("queued_at").Find(&invocations).Error; err != nil {
		t.Fatal(err)
	}
	if len(invocations) != 2 ||
		!strings.Contains(invocations[0].Input, "config.json") ||
		!strings.Contains(invocations[1].Input, "main.go") {
		t.Fatalf("invocações múltiplas incompletas: %+v", invocations)
	}
}

func TestIntegration_FirstMessageToolFailurePersistsInLedger(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}
	db := setupIntegrationDB(t)
	conversation := &database.Conversation{Title: "Falha de ferramenta"}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	user := &database.ChatMessage{ConversationID: conversation.ID, Role: "user", Content: "Leia arquivo ausente"}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	catalog := &database.ToolCatalog{Name: "read_file", DisplayName: "Ler arquivo", Origin: "builtin"}
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
		ToolCallID:     "call-read-error",
		Status:         "failed",
		Input:          `{"path":"/ausente"}`,
		Output:         `{"content":"file not found","is_error":true}`,
		ErrorKind:      "tool_error",
		ErrorMessage:   "file not found",
		QueuedAt:       time.Now().UTC(),
	}
	if err := db.Create(invocation).Error; err != nil {
		t.Fatal(err)
	}

	var stored database.ToolInvocation
	if err := db.First(&stored, "id = ?", invocation.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Status != "failed" || stored.ErrorMessage == "" || !strings.Contains(stored.Output, "is_error") {
		t.Fatalf("falha canônica incompleta: %+v", stored)
	}
}

func TestIntegration_FirstMessageToolResultAndFinalAnswerRemainSeparated(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}
	db := setupIntegrationDB(t)
	conversation := &database.Conversation{Title: "Resultado incorporado"}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	user := &database.ChatMessage{ConversationID: conversation.ID, Role: "user", Content: "Quanto é 2 + 2?"}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	intermediate := &database.ChatMessage{ConversationID: conversation.ID, TurnID: &user.ID, Role: "assistant", Content: "Vou calcular."}
	final := &database.ChatMessage{ConversationID: conversation.ID, TurnID: &user.ID, Role: "assistant", Content: "A resposta é 4."}
	if err := db.Create(intermediate).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(final).Error; err != nil {
		t.Fatal(err)
	}
	catalog := &database.ToolCatalog{Name: "calculator", DisplayName: "Calculadora", Origin: "builtin"}
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
		ToolCallID:     "call-calc-1",
		Status:         "succeeded",
		Input:          `{"expression":"2+2"}`,
		Output:         `{"content":"4"}`,
		QueuedAt:       time.Now().UTC(),
	}
	if err := db.Create(invocation).Error; err != nil {
		t.Fatal(err)
	}

	var messages []database.ChatMessage
	if err := db.Where("conversation_id = ?", conversation.ID).Order("created_at").Find(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 || !strings.Contains(messages[2].Content, "4") {
		t.Fatalf("histórico conversacional inesperado: %+v", messages)
	}
	var technicalMessages int64
	if err := db.Model(&database.ChatMessage{}).Where("conversation_id = ? AND role = ?", conversation.ID, "tool").Count(&technicalMessages).Error; err != nil {
		t.Fatal(err)
	}
	if technicalMessages != 0 {
		t.Fatalf("resultado técnico duplicado em chat_messages: %d", technicalMessages)
	}
}

func TestIntegration_FirstMessageToolTokenUsageStaysOnAssistantMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("Utilizando -short, pulando teste de integração")
	}
	db := setupIntegrationDB(t)
	conversation := &database.Conversation{Title: "Tokens com ferramenta"}
	if err := db.Create(conversation).Error; err != nil {
		t.Fatal(err)
	}
	user := &database.ChatMessage{ConversationID: conversation.ID, Role: "user", Content: "Busque um dado"}
	if err := db.Create(user).Error; err != nil {
		t.Fatal(err)
	}
	first := &database.ChatMessage{
		ConversationID: conversation.ID, TurnID: &user.ID, Role: "assistant", Content: "Buscando.",
		PromptTokens: 120, CompletionTokens: 25, TotalTokens: 145,
	}
	second := &database.ChatMessage{
		ConversationID: conversation.ID, TurnID: &user.ID, Role: "assistant", Content: "Resultado final.",
		PromptTokens: 280, CompletionTokens: 18, TotalTokens: 298,
	}
	if err := db.Create(first).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(second).Error; err != nil {
		t.Fatal(err)
	}

	var assistants []database.ChatMessage
	if err := db.Where("conversation_id = ? AND role = ?", conversation.ID, "assistant").Order("created_at").Find(&assistants).Error; err != nil {
		t.Fatal(err)
	}
	if len(assistants) != 2 || assistants[0].TotalTokens != 145 || assistants[1].TotalTokens != 298 ||
		assistants[1].PromptTokens <= assistants[0].PromptTokens {
		t.Fatalf("tokens por iteração não preservados: %+v", assistants)
	}
}
