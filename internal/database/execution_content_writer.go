package database

import "gorm.io/gorm"

// A proveniência é atribuída somente na inserção pelo writer de execução ou
// pelo importador. Save/Update do modelo de leitura nunca a reatribuem.
// Migrações históricas continuam escrevendo conteúdo sem proveniência.
func CreateChatMessageInTransaction(tx *gorm.DB, message *ChatMessage) error {
	if message.ChatExecutionID == nil {
		return tx.Create(message).Error
	}
	return tx.Table("chat_messages").Create(&struct {
		*ChatMessage
		ChatExecutionID *string
	}{message, message.ChatExecutionID}).Error
}

func CreateToolInvocationInTransaction(tx *gorm.DB, invocation *ToolInvocation) error {
	if invocation.ChatExecutionID == nil {
		return tx.Create(invocation).Error
	}
	return tx.Table("tool_invocations").Create(&struct {
		*ToolInvocation
		ChatExecutionID *string
	}{invocation, invocation.ChatExecutionID}).Error
}
