package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrConversationIDRequired = errors.New("conversation ID required")

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
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}

	var result *Conversation
	err = withSQLiteImmediateTransaction(ctx, r.db, "conversations.recycle_or_create", func(tx *gorm.DB) error {
		var candidate Conversation
		err := ScopeByUser(ctx, tx.WithContext(ctx), "user_id").
			Where("channel = '' AND contact_id = '' AND (kind = '' OR kind IS NULL)").
			Where("id NOT IN (?)",
				tx.WithContext(ctx).Model(&ChatMessage{}).Select("DISTINCT conversation_id"),
			).
			Order("created_at ASC").
			First(&candidate).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			result = &Conversation{Title: title, UserID: userID}
			return tx.WithContext(ctx).Create(result).Error
		}
		if err != nil {
			return err
		}

		now := time.Now()
		candidate.Title = title
		candidate.Summary = ""
		candidate.SummaryUpToMessageID = ""
		candidate.SummarizingInProgress = false
		candidate.CreatedAt = now
		candidate.UpdatedAt = now
		candidate.UserID = userID
		if err := tx.WithContext(ctx).Save(&candidate).Error; err != nil {
			return err
		}
		result = &candidate
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
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

const DefaultConversationPageLimit = 100
const maxConversationIDLookupLimit = 500
const maxConversationPageLimit = maxConversationIDLookupLimit

const conversationListCorrelatedSelect = `conversations.*,
	(SELECT COUNT(*) FROM chat_messages WHERE chat_messages.conversation_id = conversations.id) as message_count,
	COALESCE((
		SELECT status
		FROM sub_agent_runs
		WHERE user_id = ?
			AND child_conversation_id = conversations.id
			AND conversations.kind = 'subagent'
		ORDER BY turn_index DESC, created_at DESC, id DESC
		LIMIT 1
	), '') as latest_status`

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
		limit = DefaultConversationPageLimit
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
	// usuário. A listagem paginada calcula agregados por conversa retornada para
	// evitar varrer chat_messages/sub_agent_runs inteiros a cada page load.
	query := ScopeByUser(ctx, db.WithContext(ctx).Table("conversations"), "conversations.user_id")
	if paginated {
		query = query.Select(conversationListCorrelatedSelect, userID)
	} else {
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
		) as latest_run ON latest_run.child_conversation_id = conversations.id AND conversations.kind = 'subagent'`, userID)
	}
	query = query.Order("conversations.updated_at DESC, conversations.id DESC")
	if paginated {
		if limit > maxConversationPageLimit {
			limit = maxConversationPageLimit
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
		Select(conversationListCorrelatedSelect, userID).
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
	if strings.TrimSpace(id) == "" {
		return nil, ErrConversationIDRequired
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

// UpdateConversationAgentWorkDirWithContext grava o diretório em que o agente
// de código desta conversa trabalha (AEP-0084 D5). Vazio volta a conversa para
// o workspace ativo, que é o padrão.
func UpdateConversationAgentWorkDirWithContext(ctx context.Context, id, workDir string) error {
	return NewConversationRepository(db).UpdateConversationAgentWorkDirWithContext(ctx, id, workDir)
}

func (r *ConversationRepository) UpdateConversationAgentWorkDirWithContext(ctx context.Context, id, workDir string) error {
	db := r.db
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	// A conversa é conferida antes: sem isso, um identificador de outra pessoa
	// devolveria sucesso sem ter gravado nada, e a tela mostraria o diretório
	// que ninguém guardou.
	if _, err := r.GetConversationInfoWithContext(ctx, id); err != nil {
		return err
	}
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&Conversation{}), "user_id").Where("id = ?", id).Updates(map[string]interface{}{
		"agent_work_dir": workDir,
		"updated_at":     time.Now(),
	}).Error
}

const conversationDeleteBatchSize = 400

// DeleteConversationWithContext usa o mesmo pipeline transacional da exclusão
// em lote, evitando diferenças entre a ação unitária e a seleção múltipla.
func DeleteConversationWithContext(ctx context.Context, id string) error {
	return NewConversationRepository(db).DeleteConversationWithContext(ctx, id)
}

func (r *ConversationRepository) DeleteConversationWithContext(ctx context.Context, id string) error {
	_, err := r.DeleteConversationsWithContext(ctx, []string{id})
	return err
}

// ValidateOwnedConversationIDsWithContext normaliza e valida previamente um
// lote sem mutá-lo. A borda de domínio usa esta etapa antes de cancelar estado
// efêmero; a transação de delete repete a validação sob BEGIN IMMEDIATE.
func ValidateOwnedConversationIDsWithContext(ctx context.Context, ids []string) ([]string, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	normalized, err := normalizeConversationIDs(ids)
	if err != nil {
		return nil, err
	}
	releaseMaintenance, err := acquireSQLiteMaintenance(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseMaintenance()
	if err := WithSQLiteBusyRetry(ctx, "conversations.validate_batch", func() error {
		return validateOwnedConversationsTx(ctx, db.WithContext(ctx), normalized)
	}); err != nil {
		return nil, err
	}
	return normalized, nil
}

// DeleteConversationsWithContext normaliza e remove uma ou mais conversas em
// uma única transação BEGIN IMMEDIATE. A posse de TODOS os IDs é validada antes
// da primeira mutação; ID inexistente ou de outro usuário produz o mesmo erro e
// causa rollback integral (AEP-0052).
func DeleteConversationsWithContext(ctx context.Context, ids []string) ([]string, error) {
	return NewConversationRepository(db).DeleteConversationsWithContext(ctx, ids)
}

func (r *ConversationRepository) DeleteConversationsWithContext(ctx context.Context, ids []string) ([]string, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	normalized, err := normalizeConversationIDs(ids)
	if err != nil {
		return nil, err
	}

	releaseMaintenance, err := acquireSQLiteMaintenance(ctx)
	if err != nil {
		return nil, err
	}
	defer releaseMaintenance()

	err = withSQLiteImmediateTransaction(ctx, r.db, "conversations.delete_batch", func(tx *gorm.DB) error {
		if err := validateOwnedConversationsTx(ctx, tx, normalized); err != nil {
			return err
		}
		return deleteOwnedConversationsTx(ctx, tx, normalized)
	})
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func normalizeConversationIDs(ids []string) ([]string, error) {
	normalized := make([]string, 0, len(ids))
	seen := make(map[string]struct{}, len(ids))
	for _, rawID := range ids {
		id := strings.TrimSpace(rawID)
		if id == "" {
			return nil, ErrConversationIDRequired
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	if len(normalized) == 0 {
		return nil, ErrConversationIDRequired
	}
	return normalized, nil
}

func forConversationIDBatches(ids []string, fn func([]string) error) error {
	for start := 0; start < len(ids); start += conversationDeleteBatchSize {
		end := start + conversationDeleteBatchSize
		if end > len(ids) {
			end = len(ids)
		}
		if err := fn(ids[start:end]); err != nil {
			return err
		}
	}
	return nil
}

func validateOwnedConversationsTx(ctx context.Context, tx *gorm.DB, ids []string) error {
	found := make(map[string]struct{}, len(ids))
	err := forConversationIDBatches(ids, func(batch []string) error {
		var ownedIDs []string
		if err := ScopeByUser(ctx, tx.WithContext(ctx).Model(&Conversation{}), "user_id").
			Where("id IN ?", batch).
			Pluck("id", &ownedIDs).Error; err != nil {
			return err
		}
		for _, id := range ownedIDs {
			found[id] = struct{}{}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if len(found) != len(ids) {
		// Não identifica qual ID falhou: inexistente e pertencente a outra conta
		// são indistinguíveis para impedir inferência cross-user.
		return gorm.ErrRecordNotFound
	}
	return nil
}

// ValidateConversationOwnerTx valida um alvo de escrita relacionado a uma
// conversa usando o mesmo executor/transação da mutação. O erro não inclui o
// ID, para não distinguir conversa inexistente de conversa de outro usuário.
func ValidateConversationOwnerTx(ctx context.Context, tx *gorm.DB, conversationID, userID string) error {
	var count int64
	if err := tx.WithContext(ctx).Model(&Conversation{}).
		Where("id = ? AND user_id = ?", strings.TrimSpace(conversationID), strings.TrimSpace(userID)).
		Count(&count).Error; err != nil {
		return err
	}
	if count != 1 {
		return ErrConversationDeleted
	}
	return nil
}

func deleteOwnedConversationsTx(ctx context.Context, tx *gorm.DB, ids []string) error {
	if err := deleteChatToolInvocationsForConversationsTx(ctx, tx, ids); err != nil {
		return fmt.Errorf("erro ao excluir invocações das conversas: %w", err)
	}
	if tx.Migrator().HasTable(&ChannelResponsePending{}) {
		if err := forConversationIDBatches(ids, func(batch []string) error {
			return tx.WithContext(ctx).
				Where("conversation_id IN ?", batch).
				Delete(&ChannelResponsePending{}).Error
		}); err != nil {
			return fmt.Errorf("erro ao excluir respostas pendentes das conversas: %w", err)
		}
	}
	if tx.Migrator().HasTable(&ACPSession{}) {
		if err := forConversationIDBatches(ids, func(batch []string) error {
			return tx.WithContext(ctx).
				Where("conversation_id IN ?", batch).
				Delete(&ACPSession{}).Error
		}); err != nil {
			return fmt.Errorf("erro ao excluir sessões ACP das conversas: %w", err)
		}
	}
	if tx.Migrator().HasTable(&ChannelContactConversation{}) {
		if err := forConversationIDBatches(ids, func(batch []string) error {
			return tx.WithContext(ctx).
				Where("conversation_id IN ?", batch).
				Delete(&ChannelContactConversation{}).Error
		}); err != nil {
			return fmt.Errorf("erro ao excluir associações de canal das conversas: %w", err)
		}
	}
	if tx.Migrator().HasTable(&SubAgentRun{}) {
		if err := forConversationIDBatches(ids, func(batch []string) error {
			return tx.WithContext(ctx).
				Where("child_conversation_id IN ?", batch).
				Delete(&SubAgentRun{}).Error
		}); err != nil {
			return fmt.Errorf("erro ao excluir runs de subagente das conversas: %w", err)
		}
		if err := forConversationIDBatches(ids, func(batch []string) error {
			return tx.WithContext(ctx).Model(&SubAgentRun{}).
				Where("parent_conversation_id IN ?", batch).
				Updates(map[string]any{"parent_conversation_id": "", "parent_turn_id": ""}).Error
		}); err != nil {
			return fmt.Errorf("erro ao desvincular runs filhos das conversas: %w", err)
		}
	}
	if tx.Migrator().HasTable(&TaskList{}) {
		if err := forConversationIDBatches(ids, func(batch []string) error {
			return tx.WithContext(ctx).Model(&TaskList{}).
				Where("conversation_id IN ?", batch).
				Update("conversation_id", nil).Error
		}); err != nil {
			return fmt.Errorf("erro ao desvincular listas de tarefas das conversas: %w", err)
		}
	}
	if tx.Migrator().HasTable(&Task{}) {
		if err := forConversationIDBatches(ids, func(batch []string) error {
			return tx.WithContext(ctx).Model(&Task{}).
				Where("conversation_id IN ?", batch).
				Update("conversation_id", nil).Error
		}); err != nil {
			return fmt.Errorf("erro ao desvincular tarefas das conversas: %w", err)
		}
	}
	if tx.Migrator().HasTable(&MemoryRecord{}) {
		if err := forConversationIDBatches(ids, func(batch []string) error {
			return tx.WithContext(ctx).
				Where("scope = ? AND scope_ref IN ?", MemoryScopeConversation, batch).
				Delete(&MemoryRecord{}).Error
		}); err != nil {
			return fmt.Errorf("erro ao excluir memórias das conversas: %w", err)
		}
	}
	if tx.Migrator().HasTable(&TagAssignment{}) {
		if err := forConversationIDBatches(ids, func(batch []string) error {
			return tx.WithContext(ctx).
				Where("resource_type = ? AND resource_id IN ?", "conversation", batch).
				Delete(&TagAssignment{}).Error
		}); err != nil {
			return fmt.Errorf("erro ao excluir tags das conversas: %w", err)
		}
	}
	if err := forConversationIDBatches(ids, func(batch []string) error {
		return tx.WithContext(ctx).Model(&Conversation{}).
			Where("parent_conversation_id IN ?", batch).
			Update("parent_conversation_id", "").Error
	}); err != nil {
		return fmt.Errorf("erro ao desvincular sub-conversas: %w", err)
	}
	if err := forConversationIDBatches(ids, func(batch []string) error {
		messageIDs := tx.WithContext(ctx).Model(&ChatMessage{}).
			Select("chat_messages.id").
			Where("chat_messages.conversation_id IN ?", batch)
		return tx.WithContext(ctx).Where("id IN (?)", messageIDs).Delete(&ChatMessage{}).Error
	}); err != nil {
		return fmt.Errorf("erro ao excluir mensagens das conversas: %w", err)
	}

	var deleted int64
	if err := forConversationIDBatches(ids, func(batch []string) error {
		result := ScopeByUser(ctx, tx.WithContext(ctx), "user_id").
			Where("id IN ?", batch).
			Delete(&Conversation{})
		deleted += result.RowsAffected
		return result.Error
	}); err != nil {
		return fmt.Errorf("erro ao excluir conversas: %w", err)
	}
	if deleted != int64(len(ids)) {
		return gorm.ErrRecordNotFound
	}
	return nil
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
	return deleteChatToolInvocationsForConversationsTx(ctx, exec, []string{conversationID})
}

func deleteChatToolInvocationsForConversationsTx(ctx context.Context, exec *gorm.DB, conversationIDs []string) error {
	if !exec.Migrator().HasTable(&ToolInvocation{}) {
		return nil
	}

	return forConversationIDBatches(conversationIDs, func(batch []string) error {
		messageIDs := exec.WithContext(ctx).Model(&ChatMessage{}).
			Select("chat_messages.id").
			Where("chat_messages.conversation_id IN ?", batch)
		turnIDs := exec.WithContext(ctx).Model(&ChatMessage{}).
			Select("chat_messages.turn_id").
			Where("chat_messages.conversation_id IN ? AND chat_messages.turn_id IS NOT NULL AND chat_messages.turn_id <> ''", batch)
		return exec.WithContext(ctx).
			Where("origin_type = ? AND (origin_id IN (?) OR origin_id IN (?))", "chat", messageIDs, turnIDs).
			Delete(&ToolInvocation{}).Error
	})
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
