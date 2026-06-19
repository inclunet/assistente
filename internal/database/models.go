package database

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	UserRoleAdmin = "admin"
	UserRoleUser  = "user"
)

// UUIDModel é o model base para entidades com PK UUIDv7.
// Substitui gorm.Model — gera ID automaticamente via BeforeCreate.
type UUIDModel struct {
	ID        string    `gorm:"type:text;primaryKey" json:"id"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// BeforeCreate gera UUIDv7 se ID estiver vazio.
func (u *UUIDModel) BeforeCreate(tx *gorm.DB) error {
	if u.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		u.ID = id.String()
	}
	return nil
}

// ==================== Users & Sessions ====================

// User representa uma conta local do Assistente.
type User struct {
	UUIDModel
	Username     string     `json:"username" gorm:"uniqueIndex;not null;size:64"`
	DisplayName  string     `json:"displayName"`
	PasswordHash string     `json:"-" gorm:"not null;type:text"`
	Role         string     `json:"role" gorm:"not null;default:'user';index"`
	IsActive     bool       `json:"isActive" gorm:"not null;default:true;index"`
	LastLoginAt  *time.Time `json:"lastLoginAt,omitempty"`
	Sessions     []Session  `json:"-" gorm:"foreignKey:UserID"`
}

// Session representa uma sessão local baseada em refresh token rotativo.
type Session struct {
	UUIDModel
	UserID           string     `json:"userId" gorm:"not null;index"`
	RefreshTokenHash string     `json:"-" gorm:"uniqueIndex;not null;type:text"`
	ExpiresAt        time.Time  `json:"expiresAt" gorm:"not null;index"`
	LastUsedAt       *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt        *time.Time `json:"revokedAt,omitempty" gorm:"index"`
	ClientLabel      string     `json:"clientLabel,omitempty"`
	User             *User      `json:"-" gorm:"foreignKey:UserID"`
}

// ==================== LLM Providers ====================

// LLMProvider armazena configuração de provedor LLM
type LLMProvider struct {
	ID                string `gorm:"primaryKey"`
	UserID            string `json:"userId,omitempty" gorm:"index"`
	Name              string `gorm:"not null"`
	Type              string `gorm:"not null"` // openai, claude, ollama, etc
	APIFormat         string // openai, anthropic, google (SDK/protocolo)
	BaseURL           string `gorm:"not null"`
	Model             string
	DefaultModel      string
	IsDefault         bool `gorm:"default:false"`
	Timeout           int
	CredentialPattern string
	AuthMode          string
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// ==================== Conversation & Messages ====================

// Kinds de conversa (AEP-0068). Conversas normais têm Kind="" (vazio);
// sub-conversas de sub-agentes têm Kind=ConversationKindSubagent.
const (
	ConversationKindSubagent = "subagent"
)

// Conversation representa uma conversa
type Conversation struct {
	UUIDModel
	UserID       string        `json:"userId,omitempty" gorm:"index"`
	Title        string        `json:"title"`
	Channel      string        `json:"channel,omitempty" gorm:"index"`    // Canal de origem: "signal", "telegram", "" (wails/local)
	ContactID    string        `json:"contact_id,omitempty" gorm:"index"` // ID do contato externo (UUID, phone, telegram ID)
	Messages     []ChatMessage `json:"messages,omitempty" gorm:"foreignKey:ConversationID"`
	MessageCount int           `json:"message_count" gorm:"-:migration;->"` // Campo calculado, não persiste no banco

	// Sub-agentes (AEP-0068): vínculo e filtragem de sub-conversas.
	//   - Kind="" → conversa normal; Kind="subagent" → sub-conversa de sub-agente.
	//   - ParentConversationID aponta para a conversa que originou o sub-agente.
	Kind                 string `json:"kind,omitempty" gorm:"index"`
	ParentConversationID string `json:"parentConversationId,omitempty" gorm:"index"`

	// LatestStatus é o status do run de sub-agente MAIS RECENTE desta conversa
	// (AEP-0068), preenchido só na listagem unificada via LEFT JOIN com
	// sub_agent_runs. Vazio para conversas comuns. Campo calculado, não persiste.
	LatestStatus string `json:"latestStatus,omitempty" gorm:"-:migration;->"`

	// Rolling Context: sumarização automática de mensagens antigas
	Summary               string `json:"summary,omitempty" gorm:"type:text"`                     // Resumo acumulativo da conversa
	SummaryUpToMessageID  string `json:"summary_up_to_message_id,omitempty"`                     // ID da última mensagem coberta pelo resumo
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
	UUIDModel
	ConversationID   string  `json:"conversationId" gorm:"index"`
	ParentID         *string `json:"parentId,omitempty" gorm:"index"` // ID da mensagem pai (define hierarquia)
	TurnID           *string `json:"turnId,omitempty" gorm:"index"`   // Agrupa mensagens de um turno (aponta para user message)
	Role             string  `json:"role"`                            // user, assistant, tool, system
	Content          string  `json:"content"`
	Reasoning        string  `json:"reasoning,omitempty"`                         // Reasoning/thinking do modelo (DeepSeek, Claude, o1, etc)
	Media            string  `json:"media,omitempty"`                             // JSON com mídias (imagens, áudio, etc) em base64
	Audio            string  `json:"audio,omitempty" gorm:"type:text"`            // Áudio da mensagem em base64 (recebido ou gerado via TTS)
	AudioMimeType    string  `json:"audioMimeType,omitempty"`                     // MIME do áudio: "audio/mpeg", "audio/aac", etc.
	ToolCalls        string  `json:"toolCalls,omitempty"`                         // JSON: [{"id":"call_x","type":"function","function":{...}}]
	ToolCallID       string  `json:"toolCallId,omitempty"`                        // Para role="tool": ID da chamada que este resultado responde
	PromptTokens     int     `json:"promptTokens,omitempty"`                      // Tokens de entrada
	CompletionTokens int     `json:"completionTokens,omitempty"`                  // Tokens de saída
	TotalTokens      int     `json:"totalTokens,omitempty"`                       // Total de tokens
	CacheReadTokens  int     `json:"cacheReadTokens,omitempty" gorm:"default:0"`  // Tokens de prompt lidos do cache
	CacheWriteTokens int     `json:"cacheWriteTokens,omitempty" gorm:"default:0"` // Tokens gravados/criados no cache
	CacheMissTokens  int     `json:"cacheMissTokens,omitempty" gorm:"default:0"`  // Tokens de prompt não atendidos pelo cache
	Model            string  `json:"model,omitempty"`                             // Modelo usado
	Source           string  `json:"source,omitempty"`                            // Origem da mensagem: "wails", "telegram", "signal", etc.
}

// ==================== Context Providers / Memory ====================

const (
	MemoryLoadPolicyCore        = "core"
	MemoryLoadPolicyPinned      = "pinned"
	MemoryLoadPolicyAuto        = "auto"
	MemoryLoadPolicyRetrievable = "retrievable"
	MemoryLoadPolicyArchived    = "archived"
)

const (
	MemoryKindUserPreference = "user_preference"
	MemoryKindIdentity       = "identity"
	MemoryKindProjectFact    = "project_fact"
	MemoryKindDecision       = "decision"
	MemoryKindConvention     = "convention"
	MemoryKindHistoricalNote = "historical_note"
	MemoryKindResolvedIssue  = "resolved_issue"
)

const (
	MemoryScopeGlobal       = "global"
	MemoryScopeUser         = "user"
	MemoryScopeWorkspace    = "workspace"
	MemoryScopeProject      = "project"
	MemoryScopeConversation = "conversation"
)

// MemoryRecord armazena uma unidade estruturada de memória do usuário.
// A política de carregamento controla se o record entra automaticamente no
// contexto ou se fica apenas recuperável por tool/busca.
type MemoryRecord struct {
	UUIDModel
	UserID             string `json:"userId,omitempty" gorm:"not null;index;index:idx_memory_user_policy_updated,priority:1"`
	Content            string `json:"content" gorm:"type:text;not null"`
	Summary            string `json:"summary,omitempty" gorm:"type:text"`
	LoadPolicy         string `json:"loadPolicy" gorm:"not null;default:'retrievable';index;index:idx_memory_user_policy_updated,priority:2"`
	ArchivedFromPolicy string `json:"archivedFromPolicy,omitempty" gorm:"index"`
	Kind               string `json:"kind" gorm:"not null;default:'historical_note';index"`
	Scope              string `json:"scope" gorm:"not null;default:'user';index"`
	ScopeRef           string `json:"scopeRef,omitempty" gorm:"index"`
	Tags               string `json:"tags,omitempty" gorm:"type:text"` // JSON array de strings
	Importance         int    `json:"importance" gorm:"not null;default:3;index"`
	Confidence         int    `json:"confidence" gorm:"not null;default:80"`

	SourceType string     `json:"sourceType,omitempty" gorm:"index"`
	SourceID   string     `json:"sourceId,omitempty" gorm:"index"`
	LastUsedAt *time.Time `json:"lastUsedAt,omitempty"`
	ExpiresAt  *time.Time `json:"expiresAt,omitempty" gorm:"index"`

	User *User `json:"-" gorm:"foreignKey:UserID"`
}

// ==================== Credenciais ====================

// CredentialEntry armazena credenciais por padrão de domínio (com campos sensíveis criptografados).
//
// Unicidade: índice `ux_credential_entries_user_pattern` em (user_id, pattern)
// criado pelo GORM AutoMigrate via tag `uniqueIndex`. O índice é full
// (não-parcial) porque o UPSERT do `credentials/db_store.go` usa
// `clause.OnConflict{Columns: [user_id, pattern]}`, que o SQLite só aceita
// contra índices unique sem cláusula `WHERE`.
//
// Patterns vazios (`pattern=”`) ficam sob o mesmo invariante de unicidade —
// na prática o app sempre grava patterns não-vazios (instance secrets têm
// nomes específicos como `internal-auth:refresh-token`). Bases legadas com
// duplicatas em (user_id, pattern) são deduplicadas em
// `dedupCredentialEntriesBeforeMigrate` antes do AutoMigrate aplicar o
// índice (review do AEP-0052, B31).
type CredentialEntry struct {
	UUIDModel
	UserID          string `json:"userId,omitempty" gorm:"index;uniqueIndex:ux_credential_entries_user_pattern"`
	Pattern         string `json:"pattern" gorm:"uniqueIndex:ux_credential_entries_user_pattern"`
	AuthType        string `json:"auth_type"`
	TokenEnc        string `json:"token_enc" gorm:"type:text"`
	Username        string `json:"username"`
	PasswordEnc     string `json:"password_enc" gorm:"type:text"`
	HeadersEnc      string `json:"headers_enc" gorm:"type:text"`
	ExpiresAt       int64  `json:"expires_at"`
	RefreshTokenEnc string `json:"refresh_token_enc" gorm:"type:text"`
	ClientIDEnc     string `json:"client_id_enc" gorm:"type:text"`
	ClientSecretEnc string `json:"client_secret_enc" gorm:"type:text"`
}

// CredentialKeyWrap armazena a DEK embrulhada com senha mestre ou recovery key.
//
// `DekID` é a `credentials.DEKIdentity(dek)` (hex 32 chars) da DEK
// efetivamente embrulhada em `WrappedDEK`. Permite detectar
// divergência entre keychain e wraps sem ter a senha mestre.
// Wraps pré-AEP-0061 vêm com `DekID == ""` e são repopulados pelo
// boot a partir da DEK do keychain (assumindo o keychain como
// fonte autoritativa naquele instante).
type CredentialKeyWrap struct {
	UUIDModel
	Kind         string `json:"kind" gorm:"uniqueIndex"` // master | recovery
	Salt         string `json:"salt" gorm:"type:text"`
	WrappedDEK   string `json:"wrapped_dek" gorm:"type:text"`
	ArgonTime    uint32 `json:"argon_time"`
	ArgonMemory  uint32 `json:"argon_memory"`
	ArgonThreads uint8  `json:"argon_threads"`
	DekID        string `json:"dek_id" gorm:"type:text;not null;default:''"`
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
	UUIDModel
	TaskListID         string    `json:"task_list_id" gorm:"uniqueIndex;not null;index"`
	Statuses           string    `json:"statuses" gorm:"type:text"`            // JSON array: [{"id":1,"order":0,"label":"A Fazer",...}]
	AllowedTransitions string    `json:"allowed_transitions" gorm:"type:text"` // JSON: {"1":[2,3],"2":[3]}
	InitialStatusID    int       `json:"initial_status_id" gorm:"default:1"`   // ID do status inicial para novas tasks
	TaskList           *TaskList `json:"task_list,omitempty" gorm:"foreignKey:TaskListID"`
}

// TaskList representa uma lista de tarefas
type TaskList struct {
	UUIDModel
	UserID            string `json:"userId,omitempty" gorm:"index"`
	Title             string `json:"title" gorm:"not null;index"`
	Slug              string `json:"slug,omitempty" gorm:"size:64"` // identificador estável portável (minúsculas); único quando não vazio
	Description       string `json:"description" gorm:"type:text"`
	PreferredViewMode string `json:"preferred_view_mode" gorm:"default:'list'"` // 'list' ou 'kanban'
	// ValidationPolicy: JSON opcional (TaskListValidationPolicy) — padrões para code de tasks e notas externas.
	ValidationPolicy string `json:"validation_policy,omitempty" gorm:"type:text"`
	// CustomActions: JSON opcional (TaskListCustomActions) — ações customizáveis por lista (AEP-0067).
	CustomActions string `json:"custom_actions,omitempty" gorm:"type:text"`
	// ConversationID: vínculo opcional com uma conversa (1 conversa : N tasklists).
	// Nullable; nil/ausente quando a lista não pertence a nenhuma conversa.
	ConversationID *string `json:"conversation_id,omitempty" gorm:"index"`

	// Relacionamentos
	Workflow *TaskListWorkflow `json:"workflow,omitempty" gorm:"foreignKey:TaskListID"`
	Tasks    []Task            `json:"tasks,omitempty" gorm:"foreignKey:TaskListID"`
}

// Task representa uma tarefa dentro de uma tasklist
// Suporta hierarquia via ParentID (subtasks) e status workflow via StatusID
type Task struct {
	UUIDModel
	TaskListID   string     `json:"task_list_id" gorm:"not null;index"`
	Title        string     `json:"title" gorm:"not null"`
	Description  string     `json:"description" gorm:"type:text"`
	Code         string     `json:"code,omitempty" gorm:"size:128;index"`
	Link         string     `json:"link,omitempty" gorm:"size:512"`
	StatusID     int        `json:"status_id" gorm:"not null;default:1;index"`
	ParentID     *string    `json:"parent_id,omitempty" gorm:"index"`
	Order        int        `json:"order" gorm:"default:0"`
	AssigneeName string     `json:"assignee_name,omitempty" gorm:"size:200"`
	AssigneeID   string     `json:"assignee_id,omitempty" gorm:"size:200"`
	CreatorName  string     `json:"creator_name,omitempty" gorm:"size:200"` // Quem criou/originou a task (nome de exibição)
	CreatorID    string     `json:"creator_id,omitempty" gorm:"size:200"`   // Identificador estável do criador (email, UUID, account ID externo)
	DueDate      *time.Time `json:"due_date,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	// ConversationID: vínculo opcional com uma conversa (1 conversa : N tasks).
	// Independente do vínculo da lista; nil/ausente quando não vinculada.
	ConversationID *string `json:"conversation_id,omitempty" gorm:"index"`

	// Relacionamentos
	TaskList *TaskList  `json:"task_list,omitempty" gorm:"foreignKey:TaskListID"`
	Parent   *Task      `json:"parent,omitempty" gorm:"foreignKey:ParentID"`
	Subtasks []Task     `json:"subtasks,omitempty" gorm:"foreignKey:ParentID"`
	Notes    []TaskNote `json:"notes,omitempty" gorm:"foreignKey:TaskID"`
}

// TaskNoteType categoriza o tipo de nota/interação em uma task
type TaskNoteType int

const (
	TaskNoteInternal TaskNoteType = 1 // Nota interna (anotação do operador)
	TaskNoteCustomer TaskNoteType = 2 // Resposta/interação do cliente
	TaskNoteAgent    TaskNoteType = 3 // Ação do agente/operador
	TaskNoteSystem   TaskNoteType = 4 // Evento automático de sistema
)

// TaskNote representa uma nota ou interação associada a uma task
type TaskNote struct {
	UUIDModel
	UserID     string       `json:"userId,omitempty" gorm:"index"`
	TaskID     string       `json:"task_id" gorm:"not null;index"`
	Type       TaskNoteType `json:"type" gorm:"not null;default:1"`
	Content    string       `json:"content" gorm:"type:text;not null"`
	AuthorName string       `json:"author_name,omitempty" gorm:"size:200"` // Nome de exibição do autor da nota
	AuthorID   string       `json:"author_id,omitempty" gorm:"size:200"`   // Identificador estável do autor (email, UUID, account ID externo)
	// Origem externa (sync Jira/FSD/etc.): JSON usa "source"; coluna external_source evita ambiguidade com SQL reserved.
	ExternalSource    string     `json:"source,omitempty" gorm:"column:external_source;size:64"`
	ExternalID        string     `json:"external_id,omitempty" gorm:"size:256"`
	ExternalParentID  string     `json:"external_parent_id,omitempty" gorm:"size:256"`
	ExternalUpdatedAt *time.Time `json:"external_updated_at,omitempty"`

	Task *Task `json:"-" gorm:"foreignKey:TaskID"`
}

// UpsertTaskNoteByExternalParams descreve criação/atualização idempotente por (external_source, external_id).
type UpsertTaskNoteByExternalParams struct {
	TaskID            string
	Type              *TaskNoteType // obrigatório apenas na criação da nota
	Content           string
	AuthorName        string
	AuthorID          string
	ExternalSource    string
	ExternalID        string
	ExternalParentID  string
	ExternalUpdatedAt *time.Time
}
