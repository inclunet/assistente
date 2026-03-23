package database

import (
	"time"
)

// ==================== LLM Providers ====================

// LLMProvider armazena configuração de provedor LLM
type LLMProvider struct {
	ID                string `gorm:"primaryKey"`
	Name              string `gorm:"not null"`
	Type              string `gorm:"not null"` // openai, claude, ollama, etc
	BaseURL           string `gorm:"not null"`
	Model             string
	DefaultModel      string
	IsDefault         bool `gorm:"default:false"`
	Timeout           int
	CredentialPattern string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

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
	MessageCount int           `json:"message_count" gorm:"-:migration;->"` // Campo calculado, não persiste no banco

	// Rolling Context: sumarização automática de mensagens antigas
	Summary               string `json:"summary,omitempty" gorm:"type:text"`                     // Resumo acumulativo da conversa
	SummaryUpToMessageID  uint   `json:"summary_up_to_message_id,omitempty" gorm:"default:0"`    // ID da última mensagem coberta pelo resumo
	SummarizingInProgress bool   `json:"summarizing_in_progress,omitempty" gorm:"default:false"` // Evita sumarizações concorrentes
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
	ParentID         *uint     `json:"parentId,omitempty" gorm:"index"` // ID da mensagem pai (define hierarquia)
	TurnID           *uint     `json:"turnId,omitempty" gorm:"index"`   // Agrupa mensagens de um turno (aponta para user message)
	Role             string    `json:"role"`                            // user, assistant, tool, system
	Content          string    `json:"content"`
	Reasoning        string    `json:"reasoning,omitempty"`              // Reasoning/thinking do modelo (DeepSeek, Claude, o1, etc)
	Media            string    `json:"media,omitempty"`                  // JSON com mídias (imagens, áudio, etc) em base64
	Audio            string    `json:"audio,omitempty" gorm:"type:text"` // Áudio da mensagem em base64 (recebido ou gerado via TTS)
	AudioMimeType    string    `json:"audioMimeType,omitempty"`          // MIME do áudio: "audio/mpeg", "audio/aac", etc.
	ToolCalls        string    `json:"toolCalls,omitempty"`              // JSON: [{"id":"call_x","type":"function","function":{...}}]
	ToolCallID       string    `json:"toolCallId,omitempty"`             // Para role="tool": ID da chamada que este resultado responde
	PromptTokens     int       `json:"promptTokens,omitempty"`           // Tokens de entrada
	CompletionTokens int       `json:"completionTokens,omitempty"`       // Tokens de saída
	TotalTokens      int       `json:"totalTokens,omitempty"`            // Total de tokens
	Model            string    `json:"model,omitempty"`                  // Modelo usado
	Source           string    `json:"source,omitempty"`                 // Origem da mensagem: "wails", "telegram", "signal", etc.
	CreatedAt        time.Time `json:"createdAt"`
}

// ==================== Credenciais ====================

// CredentialEntry armazena credenciais por padrão de domínio (com campos sensíveis criptografados).
type CredentialEntry struct {
	ID              uint      `json:"id" gorm:"primaryKey"`
	Pattern         string    `json:"pattern" gorm:"uniqueIndex"`
	AuthType        string    `json:"auth_type"`
	TokenEnc        string    `json:"token_enc" gorm:"type:text"`
	Username        string    `json:"username"`
	PasswordEnc     string    `json:"password_enc" gorm:"type:text"`
	HeadersEnc      string    `json:"headers_enc" gorm:"type:text"`
	ExpiresAt       int64     `json:"expires_at"`
	RefreshTokenEnc string    `json:"refresh_token_enc" gorm:"type:text"`
	ClientIDEnc     string    `json:"client_id_enc" gorm:"type:text"`
	ClientSecretEnc string    `json:"client_secret_enc" gorm:"type:text"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// CredentialKeyWrap armazena a DEK embrulhada com senha mestre ou recovery key.
type CredentialKeyWrap struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	Kind         string    `json:"kind" gorm:"uniqueIndex"` // master | recovery
	Salt         string    `json:"salt" gorm:"type:text"`
	WrappedDEK   string    `json:"wrapped_dek" gorm:"type:text"`
	ArgonTime    uint32    `json:"argon_time"`
	ArgonMemory  uint32    `json:"argon_memory"`
	ArgonThreads uint8     `json:"argon_threads"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// ==================== Task List Manager ====================

// TaskListWorkflowStatus representa um status no workflow de uma tasklist
// Armazenado como JSON dentro de TaskListWorkflow.Statuses
type TaskListWorkflowStatus struct {
	ID    int    `json:"id"`    // Identificador numérico do status (imutável, usado em transitions)
	Order int    `json:"order"` // Ordem de exibição (pode ser reordenado)
	Label string `json:"label"` // Nome do status (imutável se a tasklist tiver tasks)
	Color string `json:"color"` // Cor da tag (mutável, padrão: --accent)
	Icon  string `json:"icon"`  // Ícone (padrão: ⌛)
}

// TaskListWorkflowTransitions define transições permitidas
// Armazenado como JSON dentro de TaskListWorkflow.AllowedTransitions
// Exemplo: {"1": [2, 3], "2": [3]} = status 1 → 2 ou 3, status 2 → 3
type TaskListWorkflowTransitions map[int][]int

// TaskListWorkflow define o workflow (statuses e transições permitidas) de uma tasklist
type TaskListWorkflow struct {
	ID                 uint      `json:"id" gorm:"primaryKey"`
	TaskListID         uint      `json:"task_list_id" gorm:"uniqueIndex;not null;index"`
	Statuses           string    `json:"statuses" gorm:"type:text"`            // JSON array: [{"id":1,"order":0,"label":"A Fazer",...}]
	AllowedTransitions string    `json:"allowed_transitions" gorm:"type:text"` // JSON: {"1":[2,3],"2":[3]}
	InitialStatusID    int       `json:"initial_status_id" gorm:"default:1"`   // ID do status inicial para novas tasks
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
	TaskList           *TaskList `json:"task_list,omitempty" gorm:"foreignKey:TaskListID"`
}

// TaskList representa uma lista de tarefas
type TaskList struct {
	ID                uint      `json:"id" gorm:"primaryKey"`
	Title             string    `json:"title" gorm:"not null;index"`
	Description       string    `json:"description" gorm:"type:text"`
	PreferredViewMode string    `json:"preferred_view_mode" gorm:"default:'list'"` // 'list' ou 'kanban'
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`

	// Relacionamentos
	Workflow *TaskListWorkflow `json:"workflow,omitempty" gorm:"foreignKey:TaskListID"`
	Tasks    []Task            `json:"tasks,omitempty" gorm:"foreignKey:TaskListID"`
}

// Task representa uma tarefa dentro de uma tasklist
// Suporta hierarquia via ParentID (subtasks) e status workflow via StatusID
type Task struct {
	ID          uint       `json:"id" gorm:"primaryKey"`
	TaskListID  uint       `json:"task_list_id" gorm:"not null;index"`
	Title       string     `json:"title" gorm:"not null"`
	Description string     `json:"description" gorm:"type:text"`
	StatusID    int        `json:"status_id" gorm:"not null;default:1;index"` // ID do status (int para imutabilidade)
	ParentID    *uint      `json:"parent_id,omitempty" gorm:"index"`          // ID da task pai (para subtasks/hierarquia)
	Order       int        `json:"order" gorm:"default:0"`                    // Ordem dentro do status/parent
	DueDate     *time.Time `json:"due_date,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`

	// Relacionamentos
	TaskList *TaskList `json:"task_list,omitempty" gorm:"foreignKey:TaskListID"`
	Parent   *Task     `json:"parent,omitempty" gorm:"foreignKey:ParentID"`
	Subtasks []Task    `json:"subtasks,omitempty" gorm:"foreignKey:ParentID"`
}

