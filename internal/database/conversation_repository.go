package database

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

// ConversationRepository encapsula a persistencia de conversas e busca com um *gorm.DB injetado.
type ConversationRepository struct {
	db *gorm.DB
}

// NewConversationRepository cria um ConversationRepository com o *gorm.DB injetado.
func NewConversationRepository(database *gorm.DB) *ConversationRepository {
	return &ConversationRepository{db: database}
}

// ==================== Conversation ====================

// CreateConversationWithContext cria uma nova conversa pertencente ao usuário
// do contexto. Falha fechado com ErrUserScopeRequired se o ctx não carregar
// userID — uma conversa sem dono não pode existir no modelo AEP-0052
// (canais legados usam FindOrCreateChannelConversationWithContext +
// WithBootstrap explícito).
func CreateConversationWithContext(ctx context.Context, title, model string) (*Conversation, error) {
	return NewConversationRepository(db).CreateConversationWithContext(ctx, title, model)
}

func (r *ConversationRepository) CreateConversationWithContext(ctx context.Context, title, model string) (*Conversation, error) {
	db := r.db
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	conv := &Conversation{
		Title:  title,
		UserID: userID,
	}

	if err := db.WithContext(ctx).Create(conv).Error; err != nil {
		return nil, err
	}
	return conv, nil
}

// CreateSubAgentConversationWithContext cria uma sub-conversa de sub-agente
// (AEP-0068) pertencente ao usuário do contexto, marcada com
// Kind=ConversationKindSubagent e vinculada à conversa-pai. Falha fechado com
// ErrUserScopeRequired se o ctx não carregar userID (AEP-0052) — uma
// sub-conversa sem dono não pode existir.
func CreateSubAgentConversationWithContext(ctx context.Context, title, parentConversationID string) (*Conversation, error) {
	return NewConversationRepository(db).CreateSubAgentConversationWithContext(ctx, title, parentConversationID)
}

func (r *ConversationRepository) CreateSubAgentConversationWithContext(ctx context.Context, title, parentConversationID string) (*Conversation, error) {
	db := r.db
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	conv := &Conversation{
		Title:                title,
		UserID:               userID,
		Kind:                 ConversationKindSubagent,
		ParentConversationID: parentConversationID,
	}
	if err := db.WithContext(ctx).Create(conv).Error; err != nil {
		return nil, err
	}
	return conv, nil
}

// RecycleOrCreateConversationWithContext busca uma conversa vazia (0 mensagens,
// sem canal, não vinculada a nenhuma tab aberta) do usuário do contexto e a
// recicla, resetando título e timestamps. Se não encontrar candidata, cria uma
// nova. Evita acumular registros órfãos no banco.
func RecycleOrCreateConversationWithContext(ctx context.Context, title string) (*Conversation, error) {
	return NewConversationRepository(db).RecycleOrCreateConversationWithContext(ctx, title)
}

func (r *ConversationRepository) RecycleOrCreateConversationWithContext(ctx context.Context, title string) (*Conversation, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var candidate Conversation
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").
		Where("channel = '' AND contact_id = '' AND (kind = '' OR kind IS NULL)").
		Where("id NOT IN (?)",
			db.WithContext(ctx).Model(&ChatMessage{}).Select("DISTINCT conversation_id"),
		).
		Order("created_at ASC").
		First(&candidate).Error

	if err == nil {
		now := time.Now()
		candidate.Title = title
		candidate.Summary = ""
		candidate.SummaryUpToMessageID = ""
		candidate.SummarizingInProgress = false
		candidate.CreatedAt = now
		candidate.UpdatedAt = now
		if userID, ok := UserIDFromContext(ctx); ok {
			candidate.UserID = userID
		}
		if err := db.WithContext(ctx).Save(&candidate).Error; err != nil {
			return nil, err
		}
		return &candidate, nil
	}

	return r.CreateConversationWithContext(ctx, title, "")
}

// FindOrCreateChannelConversationWithContext localiza ou cria uma conversa de
// canal pertencente ao usuário do contexto. Mensagens vindas de canais
// externos (WhatsApp/Telegram/etc.) precisam ser associadas ao usuário dono
// do canal — o caller deve injetar esse userID no contexto via WithUserID
// (gateway carrega ChannelConfig.OwnerUserID e propaga; ver
// internal/messaging/gateway.go).
//
// SECURITY: bootstrap-tolerant — esta é a única função de banco do AEP-0052
// que aceita ctx sem userID, e mesmo assim só quando o caller marca
// explicitamente com WithBootstrap. Esse caminho é necessário para configs
// de canal pré-AEP-0052 (ChannelConfig.OwnerUserID == ""): o gateway aceita
// receber a mensagem, mas marca o ctx com WithBootstrap antes de chamar.
// A conversa nasce órfã (user_id="") e fica invisível até AdoptLegacyData
// a atribuir ao primeiro usuário, e o gateway pode logar/notificar.
//
// Sem userID e sem WithBootstrap, retorna ErrUserScopeRequired — bug do
// caller, não fall-through silencioso.
func FindOrCreateChannelConversationWithContext(ctx context.Context, channel, contactID, contactName string) (*Conversation, bool, error) {
	return NewConversationRepository(db).FindOrCreateChannelConversationWithContext(ctx, channel, contactID, contactName)
}

func (r *ConversationRepository) FindOrCreateChannelConversationWithContext(ctx context.Context, channel, contactID, contactName string) (*Conversation, bool, error) {
	db := r.db
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return nil, false, err
	}
	var conv Conversation
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").
		Where("channel = ? AND contact_id = ?", channel, contactID).
		First(&conv).Error
	if err == nil {
		return &conv, false, nil
	}

	title := contactName
	if title == "" {
		title = contactID
	}
	userID, _ := UserIDFromContext(ctx)
	conv = Conversation{
		Title:     title,
		Channel:   channel,
		ContactID: contactID,
		UserID:    userID,
	}
	if err := db.WithContext(ctx).Create(&conv).Error; err != nil {
		return nil, false, err
	}
	return &conv, true, nil
}

// GetConversationsWithContext retorna as conversas do usuário do contexto,
// ordenadas pela última atualização. Falha fechado com ErrUserScopeRequired
// se o ctx não carregar userID — listar conversas sem escopo retornaria
// dados de todos os usuários.
func GetConversationsWithContext(ctx context.Context) ([]Conversation, error) {
	return NewConversationRepository(db).GetConversationsWithContext(ctx)
}

func (r *ConversationRepository) GetConversationsWithContext(ctx context.Context) ([]Conversation, error) {
	result, err := r.GetConversationsPageWithContext(ctx, 0, 0)
	if err != nil {
		return nil, err
	}
	return result.Conversations, nil
}

type ConversationListResult struct {
	Conversations []Conversation `json:"conversations"`
	Total         int64          `json:"total"`
}

const defaultConversationPageLimit = 100
const maxConversationIDLookupLimit = 500

func GetConversationsPageWithContext(ctx context.Context, limit, offset int) (ConversationListResult, error) {
	return NewConversationRepository(db).GetConversationsPageWithContext(ctx, limit, offset)
}

func GetConversationsByIDsWithContext(ctx context.Context, ids []string) ([]Conversation, error) {
	return NewConversationRepository(db).GetConversationsByIDsWithContext(ctx, ids)
}

func (r *ConversationRepository) GetConversationsPageWithContext(ctx context.Context, limit, offset int) (ConversationListResult, error) {
	db := r.db
	userID, err := RequireUserID(ctx)
	if err != nil {
		return ConversationListResult{}, err
	}
	var conversations []Conversation
	var total int64
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 && offset > 0 {
		limit = defaultConversationPageLimit
	}
	paginated := limit > 0
	if paginated {
		countQuery := ScopeByUser(ctx, db.WithContext(ctx).Table("conversations"), "conversations.user_id")
		if err := countQuery.Count(&total).Error; err != nil {
			return ConversationListResult{}, err
		}
	}

	// Listagem unificada (AEP-0068): inclui conversas comuns E sub-conversas de
	// sub-agentes (kind=subagent) — são a mesma entidade do ponto de vista do
	// usuário. Dois LEFT JOINs em uma única query (evita N+1):
	//   - msg_counts: contagem de mensagens por conversa.
	//   - latest_run: status do run de sub-agente MAIS RECENTE (mesmo critério
	//     canônico turn_index DESC, created_at DESC, id DESC), vazio quando a
	//     conversa não é de sub-agente. Escopado por user_id (AEP-0052).
	//
	// O JOIN de latest_run é condicionado a conversations.kind='subagent': não há
	// FK garantindo que SubAgentRun.ChildConversationID aponte para uma conversa
	// kind=subagent, então sem essa condição um run órfão poderia popular
	// latest_status numa conversa comum e vazar o dado para o cliente.
	query := ScopeByUser(ctx, db.WithContext(ctx).Table("conversations"), "conversations.user_id")
	query = query.
		Select("conversations.*, COALESCE(msg_counts.count, 0) as message_count, COALESCE(latest_run.status, '') as latest_status").
		Joins("LEFT JOIN (SELECT conversation_id, COUNT(*) as count FROM chat_messages GROUP BY conversation_id) as msg_counts ON msg_counts.conversation_id = conversations.id").
		Joins(`LEFT JOIN (
			SELECT child_conversation_id, status FROM (
				SELECT child_conversation_id, status,
				       ROW_NUMBER() OVER (PARTITION BY child_conversation_id ORDER BY turn_index DESC, created_at DESC, id DESC) AS rn
				FROM sub_agent_runs
				WHERE user_id = ?
			) WHERE rn = 1
		) as latest_run ON latest_run.child_conversation_id = conversations.id AND conversations.kind = 'subagent'`, userID).
		Order("conversations.updated_at DESC, conversations.id DESC")
	if paginated {
		if limit > 500 {
			limit = 500
		}
		query = query.Limit(limit).Offset(offset)
	}
	err = query.Find(&conversations).Error

	if err != nil {
		return ConversationListResult{}, err
	}
	if !paginated {
		total = int64(len(conversations))
	}

	return ConversationListResult{Conversations: conversations, Total: total}, nil
}

func (r *ConversationRepository) GetConversationsByIDsWithContext(ctx context.Context, ids []string) ([]Conversation, error) {
	db := r.db
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	cleanIDs := make([]string, 0, len(ids))
	seen := map[string]bool{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		cleanIDs = append(cleanIDs, id)
	}
	if len(cleanIDs) == 0 {
		return []Conversation{}, nil
	}
	if len(cleanIDs) > maxConversationIDLookupLimit {
		cleanIDs = cleanIDs[:maxConversationIDLookupLimit]
	}

	var conversations []Conversation
	query := ScopeByUser(ctx, db.WithContext(ctx).Table("conversations"), "conversations.user_id")
	err = query.
		Select("conversations.*, COALESCE(msg_counts.count, 0) as message_count, COALESCE(latest_run.status, '') as latest_status").
		Joins("LEFT JOIN (SELECT conversation_id, COUNT(*) as count FROM chat_messages GROUP BY conversation_id) as msg_counts ON msg_counts.conversation_id = conversations.id").
		Joins(`LEFT JOIN (
			SELECT child_conversation_id, status FROM (
				SELECT child_conversation_id, status,
				       ROW_NUMBER() OVER (PARTITION BY child_conversation_id ORDER BY turn_index DESC, created_at DESC, id DESC) AS rn
				FROM sub_agent_runs
				WHERE user_id = ?
			) WHERE rn = 1
		) as latest_run ON latest_run.child_conversation_id = conversations.id AND conversations.kind = 'subagent'`, userID).
		Where("conversations.id IN ?", cleanIDs).
		Order("conversations.updated_at DESC, conversations.id DESC").
		Find(&conversations).Error
	if err != nil {
		return nil, err
	}
	return conversations, nil
}

// GetConversationWithContext retorna uma conversa do usuário do contexto com
// suas mensagens. Deprecated em favor de GetConversationInfoWithContext +
// GetMessagesWithContext (lazy loading), mas mantida para callers que ainda
// precisam do payload completo.
func GetConversationWithContext(ctx context.Context, id string) (*Conversation, error) {
	return NewConversationRepository(db).GetConversationWithContext(ctx, id)
}

func (r *ConversationRepository) GetConversationWithContext(ctx context.Context, id string) (*Conversation, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var conv Conversation
	query := ScopeByUser(ctx, db.WithContext(ctx), "user_id")
	err := query.Preload("Messages", func(db *gorm.DB) *gorm.DB {
		return db.Order("created_at ASC")
	}).First(&conv, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// GetConversationInfoWithContext retorna apenas metadados da conversa
// pertencente ao usuário do contexto. Falha fechado sem userID — sem isso
// um caller distraído lendo conv por ID veria dados de qualquer usuário
// (ScopeByUser fail-open + First por id = vazamento silencioso).
func GetConversationInfoWithContext(ctx context.Context, id string) (*Conversation, error) {
	return NewConversationRepository(db).GetConversationInfoWithContext(ctx, id)
}

func (r *ConversationRepository) GetConversationInfoWithContext(ctx context.Context, id string) (*Conversation, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var conv Conversation
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").First(&conv, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &conv, nil
}

// UpdateConversationWithContext atualiza título da conversa do usuário do contexto.
func UpdateConversationWithContext(ctx context.Context, id string, title, model string) error {
	return NewConversationRepository(db).UpdateConversationWithContext(ctx, id, title, model)
}

func (r *ConversationRepository) UpdateConversationWithContext(ctx context.Context, id string, title, model string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	updates := map[string]interface{}{
		"title":      title,
		"updated_at": time.Now(),
	}

	return ScopeByUser(ctx, db.WithContext(ctx).Model(&Conversation{}), "user_id").Where("id = ?", id).Updates(updates).Error
}

// UpdateConversationChannelWithContext atualiza o canal e contato vinculados
// a uma conversa do usuário do contexto. Passar channel="" e contactID=""
// desvincula a conversa do canal.
func UpdateConversationChannelWithContext(ctx context.Context, id string, channel, contactID string) error {
	return NewConversationRepository(db).UpdateConversationChannelWithContext(ctx, id, channel, contactID)
}

func (r *ConversationRepository) UpdateConversationChannelWithContext(ctx context.Context, id string, channel, contactID string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&Conversation{}), "user_id").Where("id = ?", id).Updates(map[string]interface{}{
		"channel":    channel,
		"contact_id": contactID,
		"updated_at": time.Now(),
	}).Error
}

// DeleteConversationWithContext deleta uma conversa do usuário do contexto e
// suas mensagens.
func DeleteConversationWithContext(ctx context.Context, id string) error {
	return NewConversationRepository(db).DeleteConversationWithContext(ctx, id)
}

func (r *ConversationRepository) DeleteConversationWithContext(ctx context.Context, id string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	if _, err := r.GetConversationInfoWithContext(ctx, id); err != nil {
		return err
	}
	if err := deleteChatToolInvocationsForConversation(ctx, db, id); err != nil {
		return err
	}
	if err := db.WithContext(ctx).Where("conversation_id = ?", id).Delete(&ChatMessage{}).Error; err != nil {
		return err
	}
	return ScopeByUser(ctx, db.WithContext(ctx), "user_id").Where("id = ?", id).Delete(&Conversation{}).Error
}

func deleteChatToolInvocationsForConversation(ctx context.Context, exec *gorm.DB, conversationID string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil
	}
	if _, err := NewConversationRepository(exec).GetConversationInfoWithContext(ctx, conversationID); err != nil {
		return err
	}
	return deleteChatToolInvocationsForConversationTx(ctx, exec, conversationID)
}

// deleteChatToolInvocationsForConversationTx remove as tool invocations de chat
// associadas ao histórico da conversa usando o executor `exec` fornecido (pode
// ser o `db` global OU uma transação `tx`). Recebe o executor explicitamente para
// poder participar de uma transação atômica — ver ClearConversationContentWithContext.
// O chamador é responsável por validar posse/escopo (RequireUserID +
// GetConversationInfoWithContext) ANTES; aqui só coletamos os ids e deletamos.
//
// IMPORTANTE: deve rodar ANTES do delete das mensagens (coleta os ids a partir
// das mensagens que ainda existem).
func deleteChatToolInvocationsForConversationTx(ctx context.Context, exec *gorm.DB, conversationID string) error {
	conversationID = strings.TrimSpace(conversationID)
	if conversationID == "" {
		return nil
	}
	if !exec.Migrator().HasTable(&ToolInvocation{}) {
		return nil
	}
	userID, _ := UserIDFromContext(ctx)

	// turn_id aponta para a user message; origin_id usa turn_id.
	var turnIDs []string
	if err := scopedMessageQuery(ctx, exec.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.turn_id IS NOT NULL AND chat_messages.turn_id <> ''", conversationID).
		Distinct().
		Pluck("chat_messages.turn_id", &turnIDs).Error; err != nil {
		return err
	}
	// Algumas mensagens podem ter origin_id igual ao próprio message id.
	var msgIDs []string
	if err := scopedMessageQuery(ctx, exec.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ?", conversationID).
		Pluck("chat_messages.id", &msgIDs).Error; err != nil {
		return err
	}

	ids := make([]string, 0, len(turnIDs)+len(msgIDs))
	ids = append(ids, turnIDs...)
	ids = append(ids, msgIDs...)
	if len(ids) == 0 {
		return nil
	}

	// Batch para evitar estourar limite de variáveis do SQLite.
	const batchSize = 400
	for start := 0; start < len(ids); start += batchSize {
		end := start + batchSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := exec.WithContext(ctx).
			Where("user_id = ? AND origin_type = ? AND origin_id IN ?", userID, "chat", ids[start:end]).
			Delete(&ToolInvocation{}).Error; err != nil {
			return err
		}
	}
	return nil
}

// ==================== Utilities ====================

// GenerateTitle gera um título baseado na primeira mensagem
func GenerateTitle(content string) string {
	if len(content) > 50 {
		return content[:50] + "..."
	}
	if len(content) == 0 {
		return "Nova conversa"
	}
	return content
}

// SearchConversationsWithContext busca conversas por título no escopo do
// usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
// Antes, ScopeByUser passava fail-open e devolvia conversas de todos os
// usuários — vetor crítico porque é alcançado pelo SearchConversationsTool
// exposto ao LLM (cross-user leak via prompt do agente).
func SearchConversationsWithContext(ctx context.Context, query string) ([]Conversation, error) {
	return NewConversationRepository(db).SearchConversationsWithContext(ctx, query)
}

func (r *ConversationRepository) SearchConversationsWithContext(ctx context.Context, query string) ([]Conversation, error) {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var conversations []Conversation
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return r.GetConversationsWithContext(ctx)
	}
	searchTerm := "%" + query + "%"
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").
		Where("LOWER(title) LIKE ?", searchTerm).
		Order("updated_at DESC").
		Find(&conversations).Error
	return conversations, err
}
