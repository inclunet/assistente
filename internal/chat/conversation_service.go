package chat

import (
	"assistente/internal/events"
)

// ConversationService orquestra consultas e modificações em conversas e mensagens.
// Ele delega a persistência para repositórios e emite eventos de domínio para o frontend.
type ConversationService struct {
	cr      ConversationRepository
	mr      MessageRepository
	emitter events.Emitter
}

func NewConversationService(cr ConversationRepository, mr MessageRepository, em events.Emitter) *ConversationService {
	return &ConversationService{cr: cr, mr: mr, emitter: em}
}
