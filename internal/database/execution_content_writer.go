package database

import "gorm.io/gorm"

// A proveniência é atribuída somente na inserção pelo writer de execução ou
// pelo importador. Save/Update do modelo de leitura nunca a reatribuem.
// Migrações históricas continuam escrevendo conteúdo sem proveniência.
func CreateChatMessageInTransaction(tx *gorm.DB, message *ChatMessage) error {
	audio, mime := message.Audio, message.AudioMimeType
	message.Audio, message.AudioMimeType = "", ""
	defer func() { message.Audio, message.AudioMimeType = audio, mime }()
	var err error
	if message.ChatExecutionID == nil {
		err = tx.Create(message).Error
	} else {
		err = tx.Table("chat_messages").Create(&struct {
			*ChatMessage
			ChatExecutionID *string
		}{message, message.ChatExecutionID}).Error
	}
	if err != nil || audio == "" {
		return err
	}
	origin := message.AudioOrigin
	if origin == "" {
		origin = AttachmentLegacyUnknown
	}
	return storeMessageAudio(tx, message.ID, audio, mime, origin)
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
