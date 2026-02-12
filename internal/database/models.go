package database

import (
	"time"
)

// ==================== Conversation & Messages ====================

// Conversation representa uma conversa
type Conversation struct {
	ID           uint          `json:"id" gorm:"primaryKey"`
	Title        string        `json:"title"`
	Channel      string        `json:"channel,omitempty" gorm:"index"`    // Canal de origem: "signal", "telegram", "" (wails/local)
	ContactID    string        `json:"contact_id,omitempty" gorm:"index"` // ID do contato externo (UUID, phone, telegram ID)
	CreatedAt    time.Time     `json:"created_at"`
	UpdatedAt    time.Time     `json:"updated_at"`
	Messages     []ChatMessage `json:"messages,omitempty" gorm:"foreignKey:ConversationID"`
	MessageCount int           `json:"message_count" gorm:"-"` // Campo calculado, não persiste no banco
}

// ChatMessage representa uma mensagem na conversa
// A hierarquia é definida pelo ParentID:
//   - ParentID=null: mensagem de nível 0 (user/assistant principal)
//   - ParentID=ID_delegação: mensagem de nível 1 (agente respondendo ao orquestrador)
//   - ParentID=ID_agente_tool: mensagem de nível 2 (tool respondendo ao agente)
//
// Tool Calling:
//   - TurnID agrupa todas as mensagens de um turno (aponta para o user message que iniciou)
//   - ToolCalls (JSON) armazena as chamadas de ferramentas solicitadas pelo assistant
//   - ToolCallID vincula um resultado (role=tool) à chamada correspondente
type ChatMessage struct {
	ID               uint      `json:"id" gorm:"primaryKey"`
	ConversationID   uint      `json:"conversationId" gorm:"index"`
	ParentID         *uint     `json:"parentId,omitempty" gorm:"index"`     // ID da mensagem pai (define hierarquia)
	TurnID           *uint     `json:"turnId,omitempty" gorm:"index"`       // Agrupa mensagens de um turno (aponta para user message)
	Role             string    `json:"role"`                                // user, assistant, tool, system
	Content          string    `json:"content"`
	Reasoning        string    `json:"reasoning,omitempty"`                 // Reasoning/thinking do modelo (DeepSeek, Claude, o1, etc)
	Media            string    `json:"media,omitempty"`                     // JSON com mídias (imagens, áudio, etc) em base64
	ToolCalls        string    `json:"toolCalls,omitempty"`                 // JSON: [{"id":"call_x","type":"function","function":{...}}]
	ToolCallID       string    `json:"toolCallId,omitempty"`                // Para role="tool": ID da chamada que este resultado responde
	PromptTokens     int       `json:"promptTokens,omitempty"`             // Tokens de entrada
	CompletionTokens int       `json:"completionTokens,omitempty"`         // Tokens de saída
	TotalTokens      int       `json:"totalTokens,omitempty"`              // Total de tokens
	Model            string    `json:"model,omitempty"`                    // Modelo usado
	Source           string    `json:"source,omitempty"`                   // Origem da mensagem: "wails", "telegram", "signal", etc.
	CreatedAt        time.Time `json:"createdAt"`
}

// ==================== Chat Tabs ====================

// ChatTab representa uma aba de chat aberta na interface
type ChatTab struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	ConversationID *uint     `json:"conversation_id,omitempty" gorm:"index"`
	Title          string    `json:"title" gorm:"default:'Nova conversa'"`
	Icon           string    `json:"icon" gorm:"default:'💬'"`
	Position       int       `json:"position" gorm:"index;default:0"`
	IsActive       bool      `json:"is_active" gorm:"index;default:false"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	// Relacionamento
	Conversation *Conversation `json:"conversation,omitempty" gorm:"foreignKey:ConversationID"`
}
