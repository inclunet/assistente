package database

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"path/filepath"
	"strings"
	"time"

	"assistente/internal/configdir"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

// gormLogLevel controla o nível de log do GORM. Padrão: Warn.
// Use SetLogLevel(logger.Silent) para silenciar completamente (ex.: CLI sem --verbose).
var gormLogLevel = logger.Warn

// SetLogLevel define o nível de log do GORM antes de Init().
func SetLogLevel(level logger.LogLevel) {
	gormLogLevel = level
}

// ErrConversationDeleted é retornado quando se tenta salvar mensagem em conversa que foi deletada
// Os chamadores devem verificar esse erro e abortar o processamento graciosamente
var ErrConversationDeleted = errors.New("conversa foi deletada")

// ErrParentMessageDeleted é retornado quando se tenta criar mensagem com parentId que não existe mais
// Isso acontece quando a conversa foi limpa (clear) - as mensagens foram deletadas mas a conversa ainda existe
var ErrParentMessageDeleted = errors.New("mensagem pai foi deletada")

// DB retorna a instância do banco de dados
func DB() *gorm.DB {
	return db
}

// SetDB define a instância do banco de dados (usado em testes)
func SetDB(database *gorm.DB) {
	db = database
}

// Close fecha a conexão com o banco de dados
func Close() error {
	if db == nil {
		return nil
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	return sqlDB.Close()
}

// Init inicializa o banco de dados
// Resolve conversations.db nos 3 diretórios (exe > home > workdir).
// Se não existir em nenhum, cria em ~/.assistente/
func Init() error {
	rootResolver := configdir.NewResolver("")

	var dbPath string
	resolved, err := rootResolver.Resolve("conversations.db")
	if err != nil {
		// Não existe em nenhum diretório — criar no home
		if err := rootResolver.EnsureHomeDir(); err != nil {
			return err
		}
		dbPath = filepath.Join(configdir.GetHomeDir(), "conversations.db")
	} else {
		dbPath = resolved.Path
	}

	db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		Logger: logger.Default.LogMode(gormLogLevel),
	})
	if err != nil {
		return err
	}

	// Ativa modo WAL para melhor performance com arquivos grandes
	db.Exec("PRAGMA journal_mode=WAL")
	db.Exec("PRAGMA synchronous=NORMAL")

	// Migração: converter IDs INTEGER → UUIDv7 (AEP-0046)
	if err := migrateToUUIDv7(); err != nil {
		return fmt.Errorf("erro na migração UUIDv7: %w", err)
	}

	// Bases legadas pré-AEP-0052 podem ter (user_id, pattern) duplicado em
	// credential_entries; dedup antes do AutoMigrate criar o índice unique.
	dedupCredentialEntriesBeforeMigrate()

	// Auto migrate das tabelas persistidas no SQLite; perfis continuam
	// gerenciados via arquivos JSON em .assistente/profiles/.
	if err := db.AutoMigrate(
		&User{},
		&Session{},
		&Conversation{},
		&ChatMessage{},
		&CredentialEntry{},
		&CredentialKeyWrap{},
		&LLMProvider{},
		&TaskListWorkflow{},
		&TaskList{},
		&Task{},
		&TaskNote{},
		&MCPServer{},
		&MCPServerLog{},
		&ToolCatalog{},
		&Tag{},
		&TagAssignment{},
		&JobPipeline{},
		&Job{},
		&JobTrigger{},
		&JobRun{},
		&JobEvent{},
		&JobRunEvent{},
	); err != nil {
		return err
	}

	ensureTaskNoteExternalUniqueIndex()
	ensureTaskListSlugUniqueIndex()
	ensureChatMessageWindowIndex()
	ensureCredentialEntryUserPatternIndex()
	if err := ensureUsernameCaseInsensitive(); err != nil {
		return err
	}

	// Normalizar campos booleanos: SQLite armazena bool como INTEGER 0/1,
	// mas valores corrompidos (ex: 4) causam erro no GORM Scan.
	db.Exec(`UPDATE conversations SET summarizing_in_progress = CASE WHEN summarizing_in_progress > 0 THEN 1 ELSE 0 END WHERE summarizing_in_progress NOT IN (0, 1)`)

	if err := migrateRefreshURLToEnc(); err != nil {
		return err
	}

	// Inicializa FTS5 (full-text search) para busca em mensagens
	if err := initFTS5(); err != nil {
		return fmt.Errorf("erro ao inicializar FTS5: %w", err)
	}

	// Verifica se o índice FTS5 está desatualizado e precisa de rebuild
	sqlDB, err := db.DB()
	if err == nil {
		var ftsCount, msgCount int
		_ = sqlDB.QueryRow(`SELECT count(*) FROM chat_messages_fts`).Scan(&ftsCount)
		_ = sqlDB.QueryRow(`SELECT count(*) FROM chat_messages WHERE role IN ('user','assistant') AND content != ''`).Scan(&msgCount)
		if msgCount > 0 && ftsCount < msgCount {
			log.Printf("[Database] Índice FTS5 desatualizado (%d/%d), reconstruindo...", ftsCount, msgCount)
			if err := RebuildFTSIndex(context.Background()); err != nil {
				log.Printf("[Database] ERRO: falha ao reconstruir FTS5 — busca de histórico pode estar incompleta. Será retentado no próximo startup. Erro: %v", err)
			} else {
				log.Printf("[Database] Índice FTS5 reconstruído (%d mensagens)", msgCount)
			}
		}
	}

	return nil
}

// AdoptLegacyData vincula registros single-user existentes (user_id IS NULL
// ou user_id vazio) ao usuário ativo. A operação é idempotente.
//
// PONTOS DE CHAMADA (P0-4 do re-review da Fatia 1):
//   - Login (`app_auth.go` em `adoptLegacyDataForUser`): roda em TODO
//     login bem-sucedido. Idempotente após o primeiro: a partir do
//     segundo login do mesmo usuário, o WHERE não casa nada.
//   - RefreshAuth (`app_auth.go` em `adoptLegacyDataForUser`): roda em
//     TODO refresh bem-sucedido. Mesma idempotência.
//
// (`CreateAdminUser` por si só NÃO chama AdoptLegacyData — quem adota
// é o primeiro Login após a criação do admin, exatamente como descrito
// nos call sites acima.)
//
// SECURITY: instance-wide — varre TODAS as tabelas que carregam
// `user_id`. O WHERE é restrito a registros sem dono,
// portanto registros legitimamente atribuídos a outro usuário NÃO são
// re-atribuídos. Concretamente: User B logando depois de User A NÃO
// herda dados de A — o A já adotou tudo no primeiro login dele e o
// WHERE da B não casa mais nada.
//
// PREMISSA CRÍTICA: nenhum caminho produz registros órfãos (user_id
// vazio) DEPOIS do bootstrap. Se alguma migração futura (import legacy,
// fix de schema, restore de backup pré-AEP-0052) introduzir órfãos em
// runtime, o próximo login a executar AdoptLegacyData os atribuirá ao
// caller — possivelmente ao usuário errado. Validar essa premissa
// antes de qualquer mudança que produza órfãos em runtime.
func AdoptLegacyData(userID string) error {
	if db == nil {
		return errors.New("banco de dados não inicializado")
	}
	if strings.TrimSpace(userID) == "" {
		return errors.New("userID obrigatório")
	}

	return db.Transaction(func(tx *gorm.DB) error {
		tables := []string{
			"llm_providers",
			"conversations",
			"task_lists",
		}
		for _, table := range tables {
			if err := tx.Exec(
				fmt.Sprintf("UPDATE %s SET user_id = ? WHERE user_id IS NULL OR user_id = ''", table),
				userID,
			).Error; err != nil {
				return err
			}
		}
		// Antes do UPDATE genérico de credential_entries, removemos órfãs
		// (user_id IS NULL/'') cujo `pattern` JÁ está reivindicado pelo
		// userID corrente. Sem isso o UPDATE viola
		// `ux_credential_entries_user_pattern` (user_id, pattern) e a
		// transação inteira aborta, deixando o login do admin recém-criado
		// em estado inconsistente: o User existe no banco, mas a sessão
		// nunca completa e a próxima tentativa de CreateAdminUser bate em
		// "admin inicial já foi criado".
		//
		// O `dedupCredentialEntriesBeforeMigrate` que roda antes do
		// AutoMigrate só dedupa pares EXATAMENTE iguais em (user_id,
		// pattern), então não pega o cenário órfã+claimed do mesmo pattern.
		// A versão claimed é sempre canônica (foi escrita pelo user real,
		// possui chave wrap atualizada); a órfã é resíduo de boots antigos
		// e pode ser descartada sem perda de dados.
		if err := tx.Exec(
			`DELETE FROM credential_entries
			 WHERE (user_id IS NULL OR user_id = '')
			   AND pattern NOT LIKE 'internal-auth:%'
			   AND pattern NOT LIKE 'internal-tls:%'
			   AND EXISTS (
			     SELECT 1 FROM credential_entries claimed
			     WHERE claimed.pattern = credential_entries.pattern
			       AND claimed.user_id = ?
			   )`,
			userID,
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			"UPDATE credential_entries SET user_id = ? WHERE (user_id IS NULL OR user_id = '') AND pattern NOT LIKE 'internal-auth:%' AND pattern NOT LIKE 'internal-tls:%'",
			userID,
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`DELETE FROM credential_entries
			 WHERE (pattern LIKE 'internal-auth:%' OR pattern LIKE 'internal-tls:%')
			   AND user_id != ''
			   AND EXISTS (
			     SELECT 1 FROM credential_entries existing
			     WHERE existing.pattern = credential_entries.pattern
			       AND (existing.user_id IS NULL OR existing.user_id = '')
			   )`,
		).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`UPDATE credential_entries
			 SET user_id = ''
			 WHERE (pattern LIKE 'internal-auth:%' OR pattern LIKE 'internal-tls:%')
			   AND user_id != ''`,
		).Error; err != nil {
			return err
		}
		return nil
	})
}

// ==================== Conversation ====================

// CreateConversationWithContext cria uma nova conversa pertencente ao usuário
// do contexto. Falha fechado com ErrUserScopeRequired se o ctx não carregar
// userID — uma conversa sem dono não pode existir no modelo AEP-0052
// (canais legados usam FindOrCreateChannelConversationWithContext +
// WithBootstrap explícito).
func CreateConversationWithContext(ctx context.Context, title, model string) (*Conversation, error) {
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

// RecycleOrCreateConversationWithContext busca uma conversa vazia (0 mensagens,
// sem canal, não vinculada a nenhuma tab aberta) do usuário do contexto e a
// recicla, resetando título e timestamps. Se não encontrar candidata, cria uma
// nova. Evita acumular registros órfãos no banco.
func RecycleOrCreateConversationWithContext(ctx context.Context, title string) (*Conversation, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var candidate Conversation
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").
		Where("channel = '' AND contact_id = ''").
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

	return CreateConversationWithContext(ctx, title, "")
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
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var conversations []Conversation

	// Usa subquery para contar mensagens em uma única query (evita N+1)
	query := ScopeByUser(ctx, db.WithContext(ctx).Table("conversations"), "conversations.user_id")
	err := query.
		Select("conversations.*, COALESCE(msg_counts.count, 0) as message_count").
		Joins("LEFT JOIN (SELECT conversation_id, COUNT(*) as count FROM chat_messages GROUP BY conversation_id) as msg_counts ON msg_counts.conversation_id = conversations.id").
		Order("conversations.updated_at DESC").
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
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	if _, err := GetConversationInfoWithContext(ctx, id); err != nil {
		return err
	}
	if err := db.WithContext(ctx).Where("conversation_id = ?", id).Delete(&ChatMessage{}).Error; err != nil {
		return err
	}
	return ScopeByUser(ctx, db.WithContext(ctx), "user_id").Where("id = ?", id).Delete(&Conversation{}).Error
}

// ==================== ChatMessage ====================

// MessageOptions contém opções para criar uma mensagem
type MessageOptions struct {
	ConversationID   string
	ParentID         *string // ID da mensagem pai (define hierarquia)
	TurnID           *string // Agrupa mensagens de um turno (aponta para user message)
	Role             string  // user, assistant, tool, system
	Content          string
	Reasoning        string // Reasoning/thinking do modelo
	Media            string // JSON com mídias
	Audio            string // Áudio em base64 (recebido ou TTS)
	AudioMimeType    string // MIME do áudio
	ToolCalls        string // JSON: [{"id":"call_x","type":"function","function":{...}}]
	ToolCallID       string // Para role="tool": ID da chamada que este resultado responde
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
	Model            string
	Source           string // Origem da mensagem: "wails", "telegram", "signal", etc.
}

func scopedMessageQuery(ctx context.Context, base *gorm.DB) *gorm.DB {
	return ScopeByUser(ctx,
		base.WithContext(ctx).Joins("JOIN conversations ON conversations.id = chat_messages.conversation_id"),
		"conversations.user_id",
	)
}

// CreateMessageWithContext cria uma mensagem em uma conversa do usuário do
// contexto. Falha fechado com ErrUserScopeRequired sem userID — uma mensagem
// sem dono é cross-user leak garantido (a query de busca de conversa pai
// passa fail-open via ScopeByUser e qualquer conversa por ID seria
// candidata).
func CreateMessageWithContext(ctx context.Context, opts MessageOptions) (*ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var conv Conversation
	if err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").First(&conv, "id = ?", opts.ConversationID).Error; err != nil {
		return nil, fmt.Errorf("%w: conversa %s", ErrConversationDeleted, opts.ConversationID)
	}

	// Verifica se a mensagem pai existe (se parentId foi fornecido)
	if opts.ParentID != nil && *opts.ParentID != "" {
		var parentMsg ChatMessage
		if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).First(&parentMsg, "chat_messages.id = ?", *opts.ParentID).Error; err != nil {
			return nil, fmt.Errorf("%w: mensagem %s", ErrParentMessageDeleted, *opts.ParentID)
		}
		if parentMsg.ConversationID != opts.ConversationID {
			return nil, fmt.Errorf("%w: mensagem %s", ErrParentMessageDeleted, *opts.ParentID)
		}
	}

	msg := &ChatMessage{
		ConversationID:   opts.ConversationID,
		ParentID:         opts.ParentID,
		TurnID:           opts.TurnID,
		Role:             opts.Role,
		Content:          opts.Content,
		Reasoning:        opts.Reasoning,
		Media:            opts.Media,
		Audio:            opts.Audio,
		AudioMimeType:    opts.AudioMimeType,
		ToolCalls:        opts.ToolCalls,
		ToolCallID:       opts.ToolCallID,
		PromptTokens:     opts.PromptTokens,
		CompletionTokens: opts.CompletionTokens,
		TotalTokens:      opts.TotalTokens,
		Model:            opts.Model,
		Source:           opts.Source,
	}
	if err := db.WithContext(ctx).Create(msg).Error; err != nil {
		return nil, err
	}
	ScopeByUser(ctx, db.WithContext(ctx).Model(&Conversation{}), "user_id").
		Where("id = ?", opts.ConversationID).
		Update("updated_at", time.Now())
	return msg, nil
}

// AddMessageWithContext adiciona uma mensagem simples (sem parent - nível 0)
// para o usuário do contexto.
func AddMessageWithContext(ctx context.Context, conversationID string, role, content string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
	})
}

// AddMessageWithMediaWithContext adiciona uma mensagem com mídias (sem parent
// - nível 0) para o usuário do contexto.
func AddMessageWithMediaWithContext(ctx context.Context, conversationID string, role, content, media string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		Role:           role,
		Content:        content,
		Media:          media,
	})
}

// AddMessageWithTokensWithContext adiciona uma mensagem com informações de
// tokens para o usuário do contexto.
func AddMessageWithTokensWithContext(ctx context.Context, conversationID string, role, content string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID:   conversationID,
		Role:             role,
		Content:          content,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Model:            model,
	})
}

// AddMessageWithTokensAndMediaWithContext adiciona uma mensagem com mídias e
// informações de tokens para o usuário do contexto.
func AddMessageWithTokensAndMediaWithContext(ctx context.Context, conversationID string, role, content, media string, promptTokens, completionTokens, totalTokens int, model string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID:   conversationID,
		Role:             role,
		Content:          content,
		Media:            media,
		PromptTokens:     promptTokens,
		CompletionTokens: completionTokens,
		TotalTokens:      totalTokens,
		Model:            model,
	})
}

// GetMessageAudioWithContext retorna o áudio base64 e MIME de uma mensagem
// pertencente ao usuário do contexto. Retorna ("", "", nil) se a mensagem não
// tem áudio.
func GetMessageAudioWithContext(ctx context.Context, messageID string) (string, string, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return "", "", err
	}
	var msg ChatMessage
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Select("chat_messages.audio", "chat_messages.audio_mime_type").
		First(&msg, "chat_messages.id = ?", messageID).Error; err != nil {
		return "", "", err
	}
	return msg.Audio, msg.AudioMimeType, nil
}

// SaveMessageAudioWithContext salva áudio (base64) numa mensagem existente do
// usuário do contexto.
func SaveMessageAudioWithContext(ctx context.Context, messageID string, audioBase64 string, mimeType string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	messageIDs := scopedMessageQuery(ctx, db.Model(&ChatMessage{}).Select("chat_messages.id").Where("chat_messages.id = ?", messageID))
	return db.WithContext(ctx).Model(&ChatMessage{}).Where("id = ?", messageID).Where("id IN (?)", messageIDs).Updates(map[string]interface{}{
		"audio":           audioBase64,
		"audio_mime_type": mimeType,
	}).Error
}

// HasMessageAudioWithContext verifica se uma mensagem do usuário do contexto
// tem áudio salvo.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID retorna false sem tocar
// o banco — antes vazava existência de áudio em mensagens cross-user via
// scopedMessageQuery (fail-open).
func HasMessageAudioWithContext(ctx context.Context, messageID string) bool {
	if _, err := RequireUserID(ctx); err != nil {
		return false
	}
	var count int64
	scopedMessageQuery(ctx, db.Model(&ChatMessage{})).Where("chat_messages.id = ? AND chat_messages.audio != '' AND chat_messages.audio IS NOT NULL", messageID).Count(&count)
	return count > 0
}

// GetMessageContentWithContext retorna o conteúdo textual de uma mensagem
// pertencente ao usuário do contexto.
func GetMessageContentWithContext(ctx context.Context, messageID string) (string, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return "", err
	}
	var msg ChatMessage
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Select("chat_messages.content").
		First(&msg, "chat_messages.id = ?", messageID).Error; err != nil {
		return "", err
	}
	return msg.Content, nil
}

// GetMessageWithContext retorna a mensagem completa pelo ID, restrita ao
// usuário do contexto. Falha fechado sem userID — leitura por ID sem escopo
// retornaria mensagens de qualquer usuário (vetor de leak silencioso).
func GetMessageWithContext(ctx context.Context, messageID string) (*ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var msg ChatMessage
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).First(&msg, "chat_messages.id = ?", messageID).Error; err != nil {
		return nil, err
	}
	return &msg, nil
}

// AddToolMessageWithContext adiciona uma mensagem de role="tool" (resposta de
// tool ao orquestrador) para o usuário do contexto.
func AddToolMessageWithContext(ctx context.Context, conversationID string, content string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		Role:           "tool",
		Content:        content,
	})
}

// AddToolResultMessageWithContext adiciona uma mensagem de resultado de tool
// com TurnID e ToolCallID para o usuário do contexto. Usado pelo agentic loop
// para salvar o resultado de uma execução de ferramenta.
func AddToolResultMessageWithContext(ctx context.Context, conversationID string, turnID string, content, toolCallID string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		TurnID:         &turnID,
		Role:           "tool",
		Content:        content,
		ToolCallID:     toolCallID,
	})
}

// AddAssistantToolMessageWithContext adiciona uma mensagem do assistente que
// contém tool_calls para o usuário do contexto. Usada quando o LLM responde
// com texto + pedidos de ferramentas.
func AddAssistantToolMessageWithContext(ctx context.Context, conversationID string, turnID string, content, toolCalls, reasoning, model string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		TurnID:         &turnID,
		Role:           "assistant",
		Content:        content,
		ToolCalls:      toolCalls,
		Reasoning:      reasoning,
		Model:          model,
	})
}

// GetTurnMessagesWithContext retorna todas as mensagens de um turno (mesmo
// TurnID) pertencentes ao usuário do contexto, ordenadas por criação.
func GetTurnMessagesWithContext(ctx context.Context, turnID string) ([]ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var messages []ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.turn_id = ?", turnID).
		Order("chat_messages.created_at ASC, chat_messages.id ASC").
		Find(&messages).Error
	return messages, err
}

// GetMessagesByTurnIDWithContext retorna mensagens de um turno específico
// pertencentes ao usuário do contexto. Mantém o mesmo escopo de parent da
// janela para não misturar raiz e threads.
func GetMessagesByTurnIDWithContext(ctx context.Context, conversationID string, parentID *string, turnID string, limit int) ([]ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	if turnID == "" {
		return []ChatMessage{}, nil
	}
	query := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.turn_id = ?", conversationID, turnID)
	if parentID != nil {
		query = query.Where("chat_messages.parent_id = ?", *parentID)
	} else {
		query = query.Where("chat_messages.parent_id IS NULL")
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	var messages []ChatMessage
	err := query.Order("chat_messages.created_at ASC, chat_messages.id ASC").Find(&messages).Error
	return messages, err
}

// AddChildMessageWithContext adiciona uma mensagem filha (com ParentID
// definido) para o usuário do contexto.
func AddChildMessageWithContext(ctx context.Context, conversationID string, parentID string, role, content, model string) (*ChatMessage, error) {
	return CreateMessageWithContext(ctx, MessageOptions{
		ConversationID: conversationID,
		ParentID:       &parentID,
		Role:           role,
		Content:        content,
		Model:          model,
	})
}

// UpdateMessageContentWithContext atualiza o conteúdo e tokens de uma mensagem
// existente do usuário do contexto.
func UpdateMessageContentWithContext(ctx context.Context, messageID string, content string, promptTokens, completionTokens, totalTokens int, model string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	messageIDs := scopedMessageQuery(ctx, db.Model(&ChatMessage{}).Select("chat_messages.id").Where("chat_messages.id = ?", messageID))
	return db.WithContext(ctx).Model(&ChatMessage{}).Where("id = ?", messageID).Where("id IN (?)", messageIDs).Updates(map[string]interface{}{
		"content":           content,
		"prompt_tokens":     promptTokens,
		"completion_tokens": completionTokens,
		"total_tokens":      totalTokens,
		"model":             model,
	}).Error
}

// DeleteMessageWithContext exclui uma mensagem e todas as suas filhas
// (respostas) do usuário do contexto.
func DeleteMessageWithContext(ctx context.Context, messageID string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	var childIDs []string
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).Where("chat_messages.parent_id = ?", messageID).Pluck("chat_messages.id", &childIDs).Error; err != nil {
		return err
	}
	for _, childID := range childIDs {
		if err := DeleteMessageWithContext(ctx, childID); err != nil {
			return err
		}
	}
	messageIDs := scopedMessageQuery(ctx, db.Model(&ChatMessage{}).Select("chat_messages.id").Where("chat_messages.id = ?", messageID))
	return db.WithContext(ctx).Where("id IN (?)", messageIDs).Delete(&ChatMessage{}).Error
}

// DeleteAllMessagesWithContext remove todas as mensagens de uma conversa
// pertencente ao usuário do contexto.
func DeleteAllMessagesWithContext(ctx context.Context, conversationID string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	messageIDs := scopedMessageQuery(ctx, db.Model(&ChatMessage{}).Select("chat_messages.id").Where("chat_messages.conversation_id = ?", conversationID))
	return db.WithContext(ctx).Where("id IN (?)", messageIDs).Delete(&ChatMessage{}).Error
}

// ClearAllConversationsWithContext apaga mensagens e conversas pertencentes ao
// usuário do contexto. Falha fechado com ErrUserScopeRequired sem userID —
// não há caso legítimo de "limpar global"; AdoptLegacyData/RebuildFTSIndex
// têm assinaturas próprias para operações instance-wide. Antes do AEP-0052
// esta função apagava tudo de todos quando chamada sem ctx; o comportamento
// foi removido para não ser uma bomba-relógio assinada.
func ClearAllConversationsWithContext(ctx context.Context) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	messageIDs := scopedMessageQuery(ctx, db.WithContext(ctx).Model(&ChatMessage{}).Select("chat_messages.id"))
	if err := db.WithContext(ctx).Where("id IN (?)", messageIDs).Delete(&ChatMessage{}).Error; err != nil {
		return fmt.Errorf("erro ao limpar mensagens: %w", err)
	}
	if err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").Delete(&Conversation{}).Error; err != nil {
		return fmt.Errorf("erro ao limpar conversas: %w", err)
	}
	return nil
}

// GetMessagesWithContext retorna mensagens de uma conversa do usuário do
// contexto, com filtro opcional por parent.
func GetMessagesWithContext(ctx context.Context, conversationID string, parentID *string) ([]ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var messages []ChatMessage
	query := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).Order("chat_messages.created_at ASC, chat_messages.id ASC")

	if parentID != nil {
		query = query.Where("chat_messages.parent_id = ?", *parentID)
		if conversationID != "" {
			query = query.Where("chat_messages.conversation_id = ?", conversationID)
		}
	} else {
		if conversationID == "" {
			return nil, fmt.Errorf("conversationID é obrigatório para buscar mensagens raiz")
		}
		query = query.Where("chat_messages.conversation_id = ? AND chat_messages.parent_id IS NULL", conversationID)
	}

	err := query.Find(&messages).Error
	return messages, err
}

// GetRecentRootMessagesWithContext retorna as mensagens raiz mais recentes de
// uma conversa do usuário do contexto, preservando ordem cronológica no retorno.
func GetRecentRootMessagesWithContext(ctx context.Context, conversationID string, limit int) ([]ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	if conversationID == "" {
		return nil, fmt.Errorf("conversationID é obrigatório para buscar mensagens recentes")
	}
	if limit <= 0 {
		return []ChatMessage{}, nil
	}

	var messages []ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).Where("chat_messages.conversation_id = ? AND chat_messages.parent_id IS NULL", conversationID).
		Order("chat_messages.created_at DESC, chat_messages.id DESC").
		Limit(limit).
		Find(&messages).Error
	if err != nil {
		return nil, err
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

// GetRootMessagesBeforeWithContext retorna mensagens raiz anteriores a
// beforeID, do usuário do contexto, preservando ordem cronológica no retorno.
func GetRootMessagesBeforeWithContext(ctx context.Context, conversationID string, beforeID string, limit int) ([]ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	if conversationID == "" {
		return nil, fmt.Errorf("conversationID é obrigatório para buscar mensagens anteriores")
	}
	if beforeID == "" {
		return nil, fmt.Errorf("beforeID é obrigatório para buscar mensagens anteriores")
	}
	if limit <= 0 {
		return []ChatMessage{}, nil
	}

	var before ChatMessage
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Select("chat_messages.id", "chat_messages.created_at").
		First(&before, "chat_messages.id = ? AND chat_messages.conversation_id = ?", beforeID, conversationID).Error; err != nil {
		return nil, err
	}

	var messages []ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).Where(
		"chat_messages.conversation_id = ? AND chat_messages.parent_id IS NULL AND (chat_messages.created_at < ? OR (chat_messages.created_at = ? AND chat_messages.id < ?))",
		conversationID,
		before.CreatedAt,
		before.CreatedAt,
		before.ID,
	).
		Order("chat_messages.created_at DESC, chat_messages.id DESC").
		Limit(limit).
		Find(&messages).Error
	if err != nil {
		return nil, err
	}

	for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
		messages[i], messages[j] = messages[j], messages[i]
	}
	return messages, nil
}

type MessageWindowQuery struct {
	ConversationID  string
	ParentID        *string
	Anchor          string
	AnchorMessageID string
	Direction       string
	Limit           int
}

type MessageWindowResult struct {
	Items      []MessageWindowItem
	Messages   []ChatMessage
	TotalCount int
	StartIndex int
	EndIndex   int
	HasBefore  bool
	HasAfter   bool
}

type MessageWindowItem struct {
	Kind          string
	ID            string
	MessageID     string
	TurnID        string
	OriginalIndex int
	CreatedAt     time.Time
	FirstID       string
}

const (
	// MaxMessageWindowRows limita cada janela canônica de timeline para manter
	// consultas e renderização previsíveis em conversas longas. Ver AEP-0059.
	MaxMessageWindowRows = 240

	// MessageWindowItemKindMessage identifica uma mensagem raiz navegável sem consolidação.
	MessageWindowItemKindMessage = "message"
	// MessageWindowItemKindTurn identifica um turno consolidado por turn_id.
	MessageWindowItemKindTurn = "turn"

	messageWindowAnchorStart = "start"
	messageWindowAnchorEnd   = "end"

	messageWindowDirectionBefore = "before"
	messageWindowDirectionAfter  = "after"
	messageWindowDirectionAround = "around"
)

func normalizeMessageWindowLimit(limit int) int {
	if limit <= 0 {
		return 0
	}
	if limit > MaxMessageWindowRows {
		return MaxMessageWindowRows
	}
	return limit
}

func computeMessageWindowHasAfter(total int, startIndex int, endIndex int, itemCount int) bool {
	if total <= 0 {
		return false
	}
	if itemCount == 0 {
		return startIndex < total
	}
	return endIndex < total-1
}

func normalizeMessageWindowCursor(query MessageWindowQuery) (anchor string, direction string, err error) {
	anchor = strings.TrimSpace(query.Anchor)
	direction = strings.TrimSpace(query.Direction)
	if direction == "" {
		direction = messageWindowDirectionBefore
	}
	if anchor != "" && anchor != messageWindowAnchorStart && anchor != messageWindowAnchorEnd {
		return "", "", fmt.Errorf("anchor de janela de mensagens inválido: %s", query.Anchor)
	}
	if direction != messageWindowDirectionBefore &&
		direction != messageWindowDirectionAfter &&
		direction != messageWindowDirectionAround {
		return "", "", fmt.Errorf("direction de janela de mensagens inválido: %s", query.Direction)
	}
	if anchor != "" && query.AnchorMessageID != "" {
		return "", "", fmt.Errorf("anchor e anchorMessageId são mutuamente exclusivos")
	}
	if anchor == messageWindowAnchorStart && direction == messageWindowDirectionBefore {
		return "", "", fmt.Errorf("anchor=start não aceita direction=before")
	}
	if anchor == messageWindowAnchorEnd && direction == messageWindowDirectionAfter {
		return "", "", fmt.Errorf("anchor=end não aceita direction=after")
	}
	if direction == messageWindowDirectionAround && query.AnchorMessageID == "" {
		return "", "", fmt.Errorf("direction=around exige anchorMessageId")
	}
	return anchor, direction, nil
}

func messageScopeQuery(conversationID string, parentID *string) *gorm.DB {
	query := db.Model(&ChatMessage{}).Where("conversation_id = ?", conversationID)
	if parentID != nil {
		return query.Where("parent_id = ?", *parentID)
	}
	return query.Where("parent_id IS NULL")
}

func timelineItemCTE(parentID *string) string {
	parentPredicate := "parent_id IS NULL"
	if parentID != nil {
		parentPredicate = "parent_id = ?"
	}
	return fmt.Sprintf(`
WITH scoped AS (
	SELECT
		id,
		turn_id,
		created_at,
		CASE WHEN COALESCE(turn_id, '') <> '' THEN 'turn' ELSE 'message' END AS item_kind,
		CASE WHEN COALESCE(turn_id, '') <> '' THEN turn_id ELSE id END AS item_id,
		ROW_NUMBER() OVER (
			PARTITION BY
				CASE WHEN COALESCE(turn_id, '') <> '' THEN 'turn' ELSE 'message' END,
				CASE WHEN COALESCE(turn_id, '') <> '' THEN turn_id ELSE id END
			ORDER BY created_at ASC, id ASC
		) AS rn
	FROM chat_messages
	WHERE conversation_id = ? AND %s
),
timeline_items AS (
	SELECT
		item_kind AS kind,
		item_id AS id,
		id AS message_id,
		CASE WHEN item_kind = 'turn' THEN item_id ELSE '' END AS turn_id,
		created_at,
		id AS first_id
	FROM scoped
	WHERE rn = 1
)`, parentPredicate)
}

func timelineItemArgs(conversationID string, parentID *string) []interface{} {
	args := []interface{}{conversationID}
	if parentID != nil {
		args = append(args, *parentID)
	}
	return args
}

func countTimelineItems(conversationID string, parentID *string) (int, error) {
	var count int64
	sql := timelineItemCTE(parentID) + ` SELECT COUNT(*) FROM timeline_items`
	err := db.Raw(sql, timelineItemArgs(conversationID, parentID)...).Scan(&count).Error
	return int(count), err
}

func getAnchorTimelineItem(query MessageWindowQuery) (*MessageWindowItem, error) {
	sql := timelineItemCTE(query.ParentID) + `
SELECT ti.kind, ti.id, ti.message_id, ti.turn_id, ti.created_at, ti.first_id
FROM timeline_items ti
JOIN scoped s ON s.item_kind = ti.kind AND s.item_id = ti.id
WHERE s.id = ?
LIMIT 1`
	args := append(timelineItemArgs(query.ConversationID, query.ParentID), query.AnchorMessageID)
	var item MessageWindowItem
	if err := db.Raw(sql, args...).Scan(&item).Error; err != nil {
		return nil, err
	}
	if item.ID == "" {
		return nil, fmt.Errorf("anchorMessageId inválido: %s", query.AnchorMessageID)
	}
	return &item, nil
}

func countTimelineItemsBefore(conversationID string, parentID *string, anchor MessageWindowItem) (int, error) {
	var count int64
	sql := timelineItemCTE(parentID) + `
SELECT COUNT(*)
FROM timeline_items
WHERE created_at < ? OR (created_at = ? AND first_id < ?)`
	args := append(timelineItemArgs(conversationID, parentID), anchor.CreatedAt, anchor.CreatedAt, anchor.FirstID)
	err := db.Raw(sql, args...).Scan(&count).Error
	return int(count), err
}

func queryTimelineItems(conversationID string, parentID *string, where string, order string, limit int, extraArgs ...interface{}) ([]MessageWindowItem, error) {
	if limit <= 0 {
		return []MessageWindowItem{}, nil
	}
	sql := timelineItemCTE(parentID) + `
SELECT kind, id, message_id, turn_id, created_at, first_id
FROM timeline_items`
	args := timelineItemArgs(conversationID, parentID)
	if where != "" {
		sql += " WHERE " + where
		args = append(args, extraArgs...)
	}
	sql += " ORDER BY " + order + " LIMIT ?"
	args = append(args, limit)
	var items []MessageWindowItem
	if err := db.Raw(sql, args...).Scan(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func queryTimelineItemsAround(conversationID string, parentID *string, offset int, limit int) ([]MessageWindowItem, error) {
	if limit <= 0 {
		return []MessageWindowItem{}, nil
	}
	sql := timelineItemCTE(parentID) + `
SELECT kind, id, message_id, turn_id, created_at, first_id
FROM timeline_items
ORDER BY created_at ASC, first_id ASC
LIMIT ? OFFSET ?`
	args := append(timelineItemArgs(conversationID, parentID), limit, offset)
	var items []MessageWindowItem
	if err := db.Raw(sql, args...).Scan(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func reverseTimelineItems(items []MessageWindowItem) {
	for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
		items[i], items[j] = items[j], items[i]
	}
}

func fetchMessagesForTimelineItems(conversationID string, parentID *string, items []MessageWindowItem) ([]ChatMessage, error) {
	if len(items) == 0 {
		return []ChatMessage{}, nil
	}
	turnIDs := make([]string, 0)
	messageIDs := make([]string, 0)
	for _, item := range items {
		if item.Kind == MessageWindowItemKindTurn {
			turnIDs = append(turnIDs, item.TurnID)
		} else {
			messageIDs = append(messageIDs, item.MessageID)
		}
	}
	query := messageScopeQuery(conversationID, parentID)
	switch {
	case len(turnIDs) > 0 && len(messageIDs) > 0:
		query = query.Where("turn_id IN ? OR id IN ?", turnIDs, messageIDs)
	case len(turnIDs) > 0:
		query = query.Where("turn_id IN ?", turnIDs)
	default:
		query = query.Where("id IN ?", messageIDs)
	}
	var messages []ChatMessage
	if err := query.Order("created_at ASC, id ASC").Find(&messages).Error; err != nil {
		return nil, err
	}
	return messages, nil
}

// GetMessageWindowWithContext retorna uma fatia ordenada de itens de timeline
// para o usuário do contexto, acompanhada de metadados absolutos para
// renderização acessível e navegação incremental.
func GetMessageWindowWithContext(ctx context.Context, query MessageWindowQuery) (*MessageWindowResult, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	if query.ConversationID == "" {
		return nil, fmt.Errorf("conversationID é obrigatório para buscar janela de mensagens")
	}
	if _, err := GetConversationInfoWithContext(ctx, query.ConversationID); err != nil {
		return nil, err
	}
	if query.ParentID != nil {
		parent, err := GetMessageWithContext(ctx, *query.ParentID)
		if err != nil {
			return nil, err
		}
		if parent.ConversationID != query.ConversationID {
			return nil, fmt.Errorf("parentID não pertence à conversa solicitada")
		}
	}
	anchor, direction, err := normalizeMessageWindowCursor(query)
	if err != nil {
		return nil, err
	}

	limit := normalizeMessageWindowLimit(query.Limit)
	total, err := countTimelineItems(query.ConversationID, query.ParentID)
	if err != nil {
		return nil, err
	}
	if limit == 0 {
		return &MessageWindowResult{
			Items:      []MessageWindowItem{},
			Messages:   []ChatMessage{},
			TotalCount: total,
			StartIndex: 0,
			EndIndex:   -1,
			HasBefore:  false,
			HasAfter:   total > 0,
		}, nil
	}
	if total == 0 {
		return &MessageWindowResult{
			Items:      []MessageWindowItem{},
			Messages:   []ChatMessage{},
			TotalCount: 0,
			StartIndex: 0,
			EndIndex:   -1,
		}, nil
	}

	startIndex := 0
	windowLimit := min(limit, total)
	var items []MessageWindowItem

	if query.AnchorMessageID != "" {
		anchorItem, err := getAnchorTimelineItem(query)
		if err != nil {
			return nil, err
		}
		anchorIndex, err := countTimelineItemsBefore(query.ConversationID, query.ParentID, *anchorItem)
		if err != nil {
			return nil, err
		}
		switch direction {
		case messageWindowDirectionAfter:
			startIndex = anchorIndex + 1
			items, err = queryTimelineItems(
				query.ConversationID,
				query.ParentID,
				"created_at > ? OR (created_at = ? AND first_id > ?)",
				"created_at ASC, first_id ASC",
				windowLimit,
				anchorItem.CreatedAt,
				anchorItem.CreatedAt,
				anchorItem.FirstID,
			)
		case messageWindowDirectionAround:
			startIndex = anchorIndex - (limit / 2)
			if startIndex < 0 {
				startIndex = 0
			}
			if startIndex+windowLimit > total {
				startIndex = total - windowLimit
				if startIndex < 0 {
					startIndex = 0
				}
			}
			items, err = queryTimelineItemsAround(query.ConversationID, query.ParentID, startIndex, windowLimit)
		default:
			items, err = queryTimelineItems(
				query.ConversationID,
				query.ParentID,
				"created_at < ? OR (created_at = ? AND first_id < ?)",
				"created_at DESC, first_id DESC",
				windowLimit,
				anchorItem.CreatedAt,
				anchorItem.CreatedAt,
				anchorItem.FirstID,
			)
			reverseTimelineItems(items)
			startIndex = anchorIndex - len(items)
			if startIndex < 0 {
				startIndex = 0
			}
		}
		if err != nil {
			return nil, err
		}
	} else if anchor == messageWindowAnchorStart || direction == messageWindowDirectionAfter {
		startIndex = 0
		items, err = queryTimelineItems(query.ConversationID, query.ParentID, "", "created_at ASC, first_id ASC", windowLimit)
	} else {
		items, err = queryTimelineItems(query.ConversationID, query.ParentID, "", "created_at DESC, first_id DESC", windowLimit)
		reverseTimelineItems(items)
		startIndex = total - len(items)
	}
	if err != nil {
		return nil, err
	}
	for i := range items {
		items[i].OriginalIndex = startIndex + i
	}

	messages, err := fetchMessagesForTimelineItems(query.ConversationID, query.ParentID, items)
	if err != nil {
		return nil, err
	}
	endIndex := startIndex + len(items) - 1
	if len(items) == 0 {
		endIndex = -1
	}

	return &MessageWindowResult{
		Items:      items,
		Messages:   messages,
		TotalCount: total,
		StartIndex: startIndex,
		EndIndex:   endIndex,
		HasBefore:  startIndex > 0,
		HasAfter:   computeMessageWindowHasAfter(total, startIndex, endIndex, len(items)),
	}, nil
}

// GetAllConversationMessagesWithContext retorna todas as mensagens de uma
// conversa (incluindo filhas) pertencente ao usuário do contexto.
func GetAllConversationMessagesWithContext(ctx context.Context, conversationID string) ([]ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var messages []ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ?", conversationID).
		Order("chat_messages.created_at ASC, chat_messages.id ASC").
		Find(&messages).Error
	return messages, err
}

// CountChildrenWithContext retorna a contagem de filhos para cada mensagem do
// usuário do contexto.
func CountChildrenWithContext(ctx context.Context, messageIDs []string) (map[string]int, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	if len(messageIDs) == 0 {
		return make(map[string]int), nil
	}

	type countResult struct {
		ParentID string
		Count    int
	}

	var results []countResult
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Select("chat_messages.parent_id, COUNT(*) as count").
		Where("chat_messages.parent_id IN ?", messageIDs).
		Group("chat_messages.parent_id").
		Scan(&results).Error

	if err != nil {
		return nil, err
	}

	counts := make(map[string]int)
	for _, r := range results {
		counts[r.ParentID] = r.Count
	}

	return counts, nil
}

// GetMessageTreeWithContext retorna uma mensagem do usuário do contexto com
// todos os seus descendentes.
// GetMessageTreeWithContext retorna a mensagem raiz e todos os descendentes
// pertencentes ao usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID no ctx retorna
// ErrUserScopeRequired — sem isso, scopedMessageQuery passa fail-open via
// ScopeByUser e qualquer messageID poderia ser usado para enumerar
// estruturas de conversas alheias.
func GetMessageTreeWithContext(ctx context.Context, messageID string) (*ChatMessage, []ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, nil, err
	}
	var message ChatMessage
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		First(&message, "chat_messages.id = ?", messageID).Error; err != nil {
		return nil, nil, err
	}

	var descendants []ChatMessage
	if err := getDescendantsWithContext(ctx, messageID, &descendants); err != nil {
		return nil, nil, err
	}

	return &message, descendants, nil
}

func getDescendantsWithContext(ctx context.Context, parentID string, descendants *[]ChatMessage) error {
	var children []ChatMessage
	if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.parent_id = ?", parentID).
		Order("chat_messages.created_at ASC").
		Find(&children).Error; err != nil {
		return err
	}
	for _, child := range children {
		*descendants = append(*descendants, child)
		if err := getDescendantsWithContext(ctx, child.ID, descendants); err != nil {
			return err
		}
	}
	return nil
}

// GetConversationTokenStatsWithContext retorna estatísticas de tokens de uma
// conversa pertencente ao usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
func GetConversationTokenStatsWithContext(ctx context.Context, conversationID string) (map[string]int, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var result struct {
		TotalPromptTokens     int
		TotalCompletionTokens int
		TotalTokens           int
	}
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ?", conversationID).
		Select("SUM(chat_messages.prompt_tokens) as total_prompt_tokens, SUM(chat_messages.completion_tokens) as total_completion_tokens, SUM(chat_messages.total_tokens) as total_tokens").
		Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return map[string]int{
		"prompt_tokens":     result.TotalPromptTokens,
		"completion_tokens": result.TotalCompletionTokens,
		"total_tokens":      result.TotalTokens,
	}, nil
}

// GetAllTokenStatsWithContext retorna estatísticas de tokens de todas as
// conversas do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID retornaria estatísticas
// agregadas globalmente — vetor de inferência sobre uso da instância.
func GetAllTokenStatsWithContext(ctx context.Context) (map[string]int, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var result struct {
		TotalPromptTokens     int
		TotalCompletionTokens int
		TotalTokens           int
	}
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Select("SUM(chat_messages.prompt_tokens) as total_prompt_tokens, SUM(chat_messages.completion_tokens) as total_completion_tokens, SUM(chat_messages.total_tokens) as total_tokens").
		Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return map[string]int{
		"prompt_tokens":     result.TotalPromptTokens,
		"completion_tokens": result.TotalCompletionTokens,
		"total_tokens":      result.TotalTokens,
	}, nil
}

// TokenStats representa estatísticas detalhadas de tokens
type TokenStats struct {
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	MessageCount     int    `json:"message_count"`
	Model            string `json:"model,omitempty"`
}

// ToolUsageBreakdown detalha o uso de um tool específico
type ToolUsageBreakdown struct {
	ToolName              string `json:"tool_name"`
	CallCount             int    `json:"call_count"`
	TotalPromptTokens     int    `json:"total_prompt_tokens"`
	TotalCompletionTokens int    `json:"total_completion_tokens"`
	TotalTokens           int    `json:"total_tokens"`
}

// DetailedTokenStats fornece breakdown detalhado de tokens por categoria
type DetailedTokenStats struct {
	// Dados básicos da conversa
	PromptTokens     int    `json:"prompt_tokens"`
	CompletionTokens int    `json:"completion_tokens"`
	TotalTokens      int    `json:"total_tokens"`
	MessageCount     int    `json:"message_count"`
	Model            string `json:"model,omitempty"`

	// Breakdown de contexto (sistema + resumo + mensagens)
	SystemPromptEstimatedTokens int `json:"system_prompt_estimated_tokens"`
	SummaryTokens               int `json:"summary_tokens"`
	MessagesInContextCount      int `json:"messages_in_context_count"`
	MessagesInContextTokens     int `json:"messages_in_context_tokens"`
	MessagesOutOfContextCount   int `json:"messages_out_of_context_count"`
	MessagesOutOfContextTokens  int `json:"messages_out_of_context_tokens"`

	// Tool calling
	ToolsUsedCount int                  `json:"tools_used_count"`
	ToolBreakdown  []ToolUsageBreakdown `json:"tool_breakdown"`
}

// GetTurnTokenStatsWithContext retorna estatísticas de tokens para um turno
// específico do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
func GetTurnTokenStatsWithContext(ctx context.Context, conversationID string, turnID string) (*TokenStats, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var result struct {
		TotalPromptTokens     int
		TotalCompletionTokens int
		TotalTokens           int
		MessageCount          int
	}
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.turn_id = ?", conversationID, turnID).
		Select("SUM(prompt_tokens) as total_prompt_tokens, SUM(completion_tokens) as total_completion_tokens, SUM(total_tokens) as total_tokens, COUNT(*) as message_count").
		Scan(&result).Error
	if err != nil {
		return nil, err
	}
	return &TokenStats{
		PromptTokens:     result.TotalPromptTokens,
		CompletionTokens: result.TotalCompletionTokens,
		TotalTokens:      result.TotalTokens,
		MessageCount:     result.MessageCount,
	}, nil
}

// GetConversationDetailedTokenStatsWithContext retorna estatísticas detalhadas
// de tokens de uma conversa pertencente ao usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna
// ErrUserScopeRequired.
func GetConversationDetailedTokenStatsWithContext(ctx context.Context, conversationID string) (*TokenStats, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var result struct {
		TotalPromptTokens     int
		TotalCompletionTokens int
		TotalTokens           int
		MessageCount          int
	}
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ?", conversationID).
		Select("SUM(prompt_tokens) as total_prompt_tokens, SUM(completion_tokens) as total_completion_tokens, SUM(total_tokens) as total_tokens, COUNT(*) as message_count").
		Scan(&result).Error
	if err != nil {
		return nil, err
	}

	var mostUsedModel string
	scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.model != ''", conversationID).
		Select("chat_messages.model").
		Group("chat_messages.model").
		Order("COUNT(*) DESC").
		Limit(1).
		Scan(&mostUsedModel)

	return &TokenStats{
		PromptTokens:     result.TotalPromptTokens,
		CompletionTokens: result.TotalCompletionTokens,
		TotalTokens:      result.TotalTokens,
		MessageCount:     result.MessageCount,
		Model:            mostUsedModel,
	}, nil
}

// GetDetailedTokenStatsWithContext retorna agregação completa de tokens com
// breakdown por categoria, restrita ao usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052). A guarda explícita evita que callers
// distraídos descubram o número de tokens de qualquer conversa por ID — a
// rota natural via DBStore já enforça userID, mas chamadas diretas a esta
// função (em util/test/scripts) precisam falhar em vez de vazar.
func GetDetailedTokenStatsWithContext(ctx context.Context, conversationID string, summaryUpToMessageID string) (*DetailedTokenStats, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	basicStats, err := GetConversationDetailedTokenStatsWithContext(ctx, conversationID)
	if err != nil {
		return nil, err
	}

	// 2. Recuperar resumo (se houver)
	summaryTokens := 0
	summary, _, err := GetConversationSummaryWithContext(ctx, conversationID)
	if err == nil && summary != "" {
		// Estima tokens do resumo: ~1 token a cada 4 caracteres
		summaryTokens = (len(summary) + 3) / 4
	}

	// 3. Contar mensagens in-context vs out-of-context
	// Usa índice na lista ordenada por created_at (como HistoryLoader.Load)
	// em vez de comparação lexicográfica de IDs, evitando problemas com
	// UUIDs gerados no mesmo milissegundo.
	var messagesInContextCount, messagesOutOfContextCount int
	var messagesInContextTokens, messagesOutOfContextTokens int

	if summaryUpToMessageID != "" {
		var allMessages []ChatMessage
		if err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
			Where("chat_messages.conversation_id = ? AND chat_messages.parent_id IS NULL", conversationID).
			Order("chat_messages.created_at ASC").
			Select("chat_messages.id, chat_messages.total_tokens").
			Find(&allMessages).Error; err == nil {

			cutIdx := -1
			for i, m := range allMessages {
				if m.ID == summaryUpToMessageID {
					cutIdx = i
					break
				}
			}

			if cutIdx >= 0 {
				// Out of context: mensagens até cutIdx (inclusive)
				for _, m := range allMessages[:cutIdx+1] {
					messagesOutOfContextCount++
					messagesOutOfContextTokens += m.TotalTokens
				}
				// In context: mensagens após cutIdx
				for _, m := range allMessages[cutIdx+1:] {
					messagesInContextCount++
					messagesInContextTokens += m.TotalTokens
				}
			} else {
				// summaryUpToMessageID não encontrado: tratar tudo como in-context
				messagesInContextCount = basicStats.MessageCount
				messagesInContextTokens = basicStats.TotalTokens
			}
		}
	} else {
		// Se não há sumarização, todas são in-context
		messagesInContextCount = basicStats.MessageCount
		messagesInContextTokens = basicStats.TotalTokens
	}

	// 4. Breakdown de tool usage
	toolBreakdown, toolsUsedCount := getToolUsageBreakdownWithContext(ctx, conversationID)

	// Estima tokens do system prompt: ~1 token a cada 4 caracteres
	// O DefaultSystemPrompt tem ~500 caracteres, então ~125 tokens
	systemPromptEstimatedTokens := 125

	return &DetailedTokenStats{
		PromptTokens:                basicStats.PromptTokens,
		CompletionTokens:            basicStats.CompletionTokens,
		TotalTokens:                 basicStats.TotalTokens,
		MessageCount:                basicStats.MessageCount,
		Model:                       basicStats.Model,
		SystemPromptEstimatedTokens: systemPromptEstimatedTokens,
		SummaryTokens:               summaryTokens,
		MessagesInContextCount:      messagesInContextCount,
		MessagesInContextTokens:     messagesInContextTokens,
		MessagesOutOfContextCount:   messagesOutOfContextCount,
		MessagesOutOfContextTokens:  messagesOutOfContextTokens,
		ToolsUsedCount:              toolsUsedCount,
		ToolBreakdown:               toolBreakdown,
	}, nil
}

// getToolUsageBreakdown extrai informações de uso de tools das mensagens
func getToolUsageBreakdownWithContext(ctx context.Context, conversationID string) ([]ToolUsageBreakdown, int) {
	var messages []ChatMessage
	scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.tool_calls != '' AND chat_messages.tool_calls IS NOT NULL", conversationID).
		Select("chat_messages.tool_calls, chat_messages.prompt_tokens, chat_messages.completion_tokens").
		Find(&messages)

	// Map para agregar tool usage
	toolMap := make(map[string]*ToolUsageBreakdown)

	for _, msg := range messages {
		if msg.ToolCalls == "" {
			continue
		}

		// Parse JSON das tool calls
		var toolCalls []map[string]interface{}
		err := json.Unmarshal([]byte(msg.ToolCalls), &toolCalls)
		if err != nil {
			continue
		}

		for _, toolCall := range toolCalls {
			if funcData, ok := toolCall["function"].(map[string]interface{}); ok {
				if toolName, ok := funcData["name"].(string); ok {
					if _, exists := toolMap[toolName]; !exists {
						toolMap[toolName] = &ToolUsageBreakdown{
							ToolName: toolName,
						}
					}
					toolMap[toolName].CallCount++
					// Distribuir tokens igualmente entre tools usados nessa mensagem
					toolCount := len(toolCalls)
					if toolCount > 0 {
						toolMap[toolName].TotalPromptTokens += msg.PromptTokens / toolCount
						toolMap[toolName].TotalCompletionTokens += msg.CompletionTokens / toolCount
					}
				}
			}
		}
	}

	// Converter map para slice
	var result []ToolUsageBreakdown
	for _, breakdown := range toolMap {
		breakdown.TotalTokens = breakdown.TotalPromptTokens + breakdown.TotalCompletionTokens
		result = append(result, *breakdown)
	}

	return result, len(toolMap)
}

// GetContextWindowUsageWithContext calcula a porcentagem de uso da janela de
// contexto para o usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Embora delegue para
// GetConversationDetailedTokenStatsWithContext (que já tem gate), valida
// no topo para defesa em camadas — se o gate interno for relaxado por
// engano em refactor futuro, este nível continua fail-closed.
func GetContextWindowUsageWithContext(ctx context.Context, conversationID string, contextLimit int) (float64, int, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return 0, 0, err
	}
	stats, err := GetConversationDetailedTokenStatsWithContext(ctx, conversationID)
	if err != nil {
		return 0, 0, err
	}
	if contextLimit <= 0 {
		return 0, stats.TotalTokens, nil
	}
	percentage := (float64(stats.TotalTokens) / float64(contextLimit)) * 100
	return percentage, stats.TotalTokens, nil
}

// GetRecentMessagesTokenCountWithContext retorna o total de tokens das N
// mensagens mais recentes do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
func GetRecentMessagesTokenCountWithContext(ctx context.Context, conversationID string, messageLimit int) (int, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return 0, err
	}
	var totalTokens int
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ?", conversationID).
		Order("chat_messages.created_at DESC").
		Limit(messageLimit).
		Select("SUM(chat_messages.total_tokens)").
		Scan(&totalTokens).Error
	return totalTokens, err
}

// ==================== Rolling Context (Summary) ====================

// GetConversationSummaryWithContext retorna o resumo e o ID da última mensagem
// resumida de uma conversa do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna
// ErrUserScopeRequired — não cabe ler resumo cross-user.
func GetConversationSummaryWithContext(ctx context.Context, conversationID string) (summary string, upToMessageID string, err error) {
	if _, err := RequireUserID(ctx); err != nil {
		return "", "", err
	}
	var conv Conversation
	err = ScopeByUser(ctx, db.WithContext(ctx).Select("summary", "summary_up_to_message_id"), "user_id").First(&conv, "id = ?", conversationID).Error
	if err != nil {
		return "", "", err
	}
	return conv.Summary, conv.SummaryUpToMessageID, nil
}

// UpdateConversationSummaryWithContext atualiza o resumo de uma conversa do
// usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna
// ErrUserScopeRequired — escrita global de summary é vetor de poluição
// cross-user.
func UpdateConversationSummaryWithContext(ctx context.Context, conversationID string, summary string, upToMessageID string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&Conversation{}), "user_id").Where("id = ?", conversationID).Updates(map[string]interface{}{
		"summary":                  summary,
		"summary_up_to_message_id": upToMessageID,
		"summarizing_in_progress":  false,
	}).Error
}

// SetSummarizingInProgressWithContext marca se uma sumarização está em
// andamento para o usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna
// ErrUserScopeRequired.
func SetSummarizingInProgressWithContext(ctx context.Context, conversationID string, inProgress bool) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	return ScopeByUser(ctx, db.WithContext(ctx).Model(&Conversation{}), "user_id").Where("id = ?", conversationID).
		Update("summarizing_in_progress", inProgress).Error
}

// IsSummarizingInProgressWithContext verifica se há sumarização em andamento
// para o usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna
// ErrUserScopeRequired.
func IsSummarizingInProgressWithContext(ctx context.Context, conversationID string) (bool, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return false, err
	}
	var conv Conversation
	err := ScopeByUser(ctx, db.WithContext(ctx).Select("summarizing_in_progress"), "user_id").First(&conv, "id = ?", conversationID).Error
	if err != nil {
		return false, err
	}
	return conv.SummarizingInProgress, nil
}

// GetMessagesAfterIDWithContext retorna mensagens raiz de uma conversa do
// usuário do contexto, criadas após a mensagem afterID. Usa posição na lista
// ordenada por created_at em vez de comparação lexicográfica de IDs, evitando
// problemas com UUIDs gerados no mesmo milissegundo. Se afterID for vazio,
// retorna todas as mensagens raiz.
//
// SECURITY: fail-closed (AEP-0052). Sem userID no ctx retorna
// ErrUserScopeRequired.
func GetMessagesAfterIDWithContext(ctx context.Context, conversationID string, afterID string) ([]ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var messages []ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.parent_id IS NULL", conversationID).
		Order("chat_messages.created_at ASC").Find(&messages).Error
	if err != nil {
		return nil, err
	}
	if afterID == "" {
		return messages, nil
	}
	for i, m := range messages {
		if m.ID == afterID {
			return messages[i+1:], nil
		}
	}
	// afterID não encontrado: retorna todas
	return messages, nil
}

// GetMessagesBetweenIDsWithContext retorna mensagens raiz do usuário do
// contexto criadas após startAfterID até endID (inclusive). Usa posição na
// lista ordenada por created_at em vez de comparação lexicográfica de IDs.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
func GetMessagesBetweenIDsWithContext(ctx context.Context, conversationID string, startAfterID string, endID string) ([]ChatMessage, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var messages []ChatMessage
	err := scopedMessageQuery(ctx, db.Model(&ChatMessage{})).
		Where("chat_messages.conversation_id = ? AND chat_messages.parent_id IS NULL", conversationID).
		Order("chat_messages.created_at ASC").Find(&messages).Error
	if err != nil {
		return nil, err
	}
	startIdx := 0
	if startAfterID != "" {
		for i, m := range messages {
			if m.ID == startAfterID {
				startIdx = i + 1
				break
			}
		}
	}
	var result []ChatMessage
	for _, m := range messages[startIdx:] {
		result = append(result, m)
		if m.ID == endID {
			break
		}
	}
	return result, nil
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
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var conversations []Conversation
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return GetConversationsWithContext(ctx)
	}
	searchTerm := "%" + query + "%"
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").
		Where("LOWER(title) LIKE ?", searchTerm).
		Order("updated_at DESC").
		Find(&conversations).Error
	return conversations, err
}

// MessageSearchResult representa um resultado de busca no conteúdo de mensagens
type MessageSearchResult struct {
	ConversationID    string    `json:"conversation_id"`
	ConversationTitle string    `json:"conversation_title"`
	MessageID         string    `json:"message_id"`
	Role              string    `json:"role"`
	Snippet           string    `json:"snippet"`
	Rank              float64   `json:"rank"`
	CreatedAt         time.Time `json:"created_at"`
}

// initFTS5 cria a tabela FTS5 e triggers de sincronização.
// Idempotente — pode ser chamada múltiplas vezes sem efeito.
func initFTS5() error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("erro ao obter sql.DB: %w", err)
	}

	stmts := []string{
		// Tabela FTS5 virtual (content-sync externo via triggers)
		`CREATE VIRTUAL TABLE IF NOT EXISTS chat_messages_fts USING fts5(
			content,
			role UNINDEXED,
			conversation_id UNINDEXED,
			content='chat_messages',
			content_rowid='rowid',
			tokenize='unicode61 remove_diacritics 2'
		)`,

		// Trigger INSERT: indexa apenas user e assistant
		`CREATE TRIGGER IF NOT EXISTS chat_messages_fts_insert AFTER INSERT ON chat_messages
		WHEN NEW.role IN ('user', 'assistant')
		BEGIN
			INSERT INTO chat_messages_fts(rowid, content, role, conversation_id)
			VALUES (NEW.rowid, NEW.content, NEW.role, NEW.conversation_id);
		END`,

		// Trigger DELETE: remove do índice
		`CREATE TRIGGER IF NOT EXISTS chat_messages_fts_delete AFTER DELETE ON chat_messages
		WHEN OLD.role IN ('user', 'assistant')
		BEGIN
			INSERT INTO chat_messages_fts(chat_messages_fts, rowid, content, role, conversation_id)
			VALUES ('delete', OLD.rowid, OLD.content, OLD.role, OLD.conversation_id);
		END`,

		// Trigger UPDATE: atualiza no índice
		`CREATE TRIGGER IF NOT EXISTS chat_messages_fts_update AFTER UPDATE OF content ON chat_messages
		WHEN NEW.role IN ('user', 'assistant')
		BEGIN
			INSERT INTO chat_messages_fts(chat_messages_fts, rowid, content, role, conversation_id)
			VALUES ('delete', OLD.rowid, OLD.content, OLD.role, OLD.conversation_id);
			INSERT INTO chat_messages_fts(rowid, content, role, conversation_id)
			VALUES (NEW.rowid, NEW.content, NEW.role, NEW.conversation_id);
		END`,
	}

	for _, stmt := range stmts {
		if _, err := sqlDB.Exec(stmt); err != nil {
			return fmt.Errorf("erro FTS5 setup: %w\nSQL: %s", err, stmt)
		}
	}

	return nil
}

// RebuildFTSIndex reconstrói o índice FTS5 a partir das mensagens existentes.
// Limpa o índice e repovoa apenas com mensagens de user/assistant.
//
// Aceita ctx para permitir cancelamento via timeout/Cancel ao caller, mesmo
// que a operação seja instance-wide e não filtre por userID (Minor I do
// re-review do AEP-0052: simetria com o resto da API *WithContext).
//
// SECURITY: instance-wide — opera sobre o índice FTS global, sem filtro
// de userID. O entry point Wails (App.RebuildSearchIndex) exige sessão
// autenticada antes de chamar (ver internal/app/db.go), garantindo que
// nenhum disparo aconteça pré-login mesmo sendo uma operação de banco
// que ignora o escopo.
func RebuildFTSIndex(ctx context.Context) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	if _, err := sqlDB.ExecContext(ctx, `INSERT INTO chat_messages_fts(chat_messages_fts) VALUES('delete-all')`); err != nil {
		return fmt.Errorf("erro ao limpar FTS: %w", err)
	}

	if _, err := sqlDB.ExecContext(ctx, `
		INSERT INTO chat_messages_fts(rowid, content, role, conversation_id)
		SELECT rowid, content, role, conversation_id
		FROM chat_messages
		WHERE role IN ('user', 'assistant') AND content != ''
	`); err != nil {
		return fmt.Errorf("erro ao repopular FTS: %w", err)
	}

	return nil
}

// SearchMessageContentWithContext busca no conteúdo das mensagens das
// conversas do usuário do contexto usando FTS5 + BM25. query suporta sintaxe
// FTS5: palavras, "frases exatas", prefixo*, operadores OR/AND/NOT. Retorna
// até `limit` resultados ranqueados por relevância.
//
// SECURITY: fail-closed. Sem userID no ctx, retorna ErrUserScopeRequired —
// FTS5 indexa todas as conversas do banco e a junção com `conversations`
// só protege se filtrarmos por user_id obrigatoriamente. AEP-0052.
func SearchMessageContentWithContext(ctx context.Context, query string, limit int) ([]MessageSearchResult, error) {
	userID, err := RequireUserID(ctx)
	if err != nil {
		return nil, err
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	var results []MessageSearchResult

	baseSQL := `
		SELECT
			m.conversation_id,
			c.title AS conversation_title,
			m.id AS message_id,
			fts.role,
			snippet(chat_messages_fts, 0, '>>>', '<<<', '...', 48) AS snippet,
			bm25(chat_messages_fts) AS rank,
			m.created_at
		FROM chat_messages_fts fts
		JOIN chat_messages m ON m.rowid = fts.rowid
		JOIN conversations c ON c.id = m.conversation_id
		WHERE chat_messages_fts MATCH ?
		  AND c.user_id = ?
	`
	args := []interface{}{query, userID}
	baseSQL += `
		ORDER BY bm25(chat_messages_fts)
		LIMIT ?
	`
	args = append(args, limit)

	err = db.WithContext(ctx).Raw(baseSQL, args...).Scan(&results).Error

	if err != nil {
		if strings.Contains(err.Error(), "fts5: syntax error") || strings.Contains(err.Error(), "no such column") {
			escapedQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
			args[0] = escapedQuery
			err = db.WithContext(ctx).Raw(baseSQL, args...).Scan(&results).Error
			if err != nil {
				return nil, fmt.Errorf("erro na busca FTS5: %w", err)
			}
		} else {
			return nil, fmt.Errorf("erro na busca FTS5: %w", err)
		}
	}

	return results, nil
}

// ==================== LLM Providers ====================

// SaveLLMProviderWithContext salva ou atualiza um provedor associado ao
// usuário do contexto.
//
// SECURITY: fail-closed bootstrap-tolerant (AEP-0052 / B11). Aceita ctx com
// userID OU marcado por WithBootstrap (CLI setup, registro de credenciais
// via env). Sem nenhum dos dois, retorna ErrUserScopeRequired — antes era
// fail-open silencioso (provider.UserID ficava em branco e gravava órfão).
// Defesa em camadas: o caller providers.DBStore.Save também valida.
func SaveLLMProviderWithContext(ctx context.Context, provider *LLMProvider) error {
	if err := RequireUserIDOrBootstrap(ctx); err != nil {
		return err
	}
	if provider != nil && provider.UserID == "" {
		if userID, ok := UserIDFromContext(ctx); ok {
			provider.UserID = userID
		}
	}
	return db.WithContext(ctx).Save(provider).Error
}

// GetLLMProvidersWithContext retorna todos os provedores do usuário do
// contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
// Retornar lista global expõe IDs/credenciais (mesmo cifradas/refs) de
// todos os usuários da instância.
func GetLLMProvidersWithContext(ctx context.Context) ([]*LLMProvider, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var providers []*LLMProvider
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").Order("created_at ASC").Find(&providers).Error
	return providers, err
}

// GetLLMProviderWithContext busca um provedor por ID no escopo do usuário do
// contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
// Antes, ScopeByUser fail-open + First por ID = leitura cross-user de
// provedor alheio com todos os metadados.
func GetLLMProviderWithContext(ctx context.Context, id string) (*LLMProvider, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var provider LLMProvider
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").First(&provider, "id = ?", id).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// DeleteLLMProviderWithContext remove um provedor do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID = ErrUserScopeRequired.
// Sem isso, DELETE por ID puro apaga provedor de qualquer usuário.
func DeleteLLMProviderWithContext(ctx context.Context, id string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	return ScopeByUser(ctx, db.WithContext(ctx), "user_id").Delete(&LLMProvider{}, "id = ?", id).Error
}

// CountLLMProvidersWithContext retorna o número total de provedores do
// usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID retornaria contagem
// global — vetor de inferência sobre uso/dimensão da instância.
func CountLLMProvidersWithContext(ctx context.Context) (int64, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return 0, err
	}
	var count int64
	err := ScopeByUser(ctx, db.WithContext(ctx).Model(&LLMProvider{}), "user_id").Count(&count).Error
	return count, err
}

// SetDefaultProviderWithContext marca um provedor como default (e desmarca os
// demais) no escopo do usuário do contexto.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID, o reset is_default=false
// limparia o default de TODOS os usuários — operação destrutiva cross-user.
func SetDefaultProviderWithContext(ctx context.Context, id string) error {
	if _, err := RequireUserID(ctx); err != nil {
		return err
	}
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scoped := ScopeByUser(ctx, tx.Model(&LLMProvider{}), "user_id")
		if err := scoped.Where("is_default = ?", true).Update("is_default", false).Error; err != nil {
			return err
		}
		return ScopeByUser(ctx, tx.Model(&LLMProvider{}), "user_id").Where("id = ?", id).Update("is_default", true).Error
	})
}

// GetDefaultProviderWithContext retorna o provedor marcado como default no
// escopo do usuário do contexto, ou nil se nenhum.
//
// SECURITY: fail-closed (AEP-0052 / B11). Sem userID retornaria o primeiro
// default que aparecer no banco — vetor de leak de provider alheio.
func GetDefaultProviderWithContext(ctx context.Context) (*LLMProvider, error) {
	if _, err := RequireUserID(ctx); err != nil {
		return nil, err
	}
	var provider LLMProvider
	err := ScopeByUser(ctx, db.WithContext(ctx), "user_id").First(&provider, "is_default = ?", true).Error
	if err != nil {
		return nil, err
	}
	return &provider, nil
}

// ensureTaskNoteExternalUniqueIndex aplica índice único parcial em (external_source, external_id).
//
// Escolha de modelagem: chave única global por origem (sem task_id na unicidade), alinhada à
// preferência de produto e ao padrão “ID estável no sistema remoto”. O mesmo comentário Jira
// (por exemplo) deve mapear a no máximo uma TaskNote no app, impedindo duplicatas em re-syncs.
// Notas manuais permanecem fora do índice (WHERE ambos os campos não vazios).
//
// Se a mesma referência externa for associada a outra task local, UpsertTaskNoteByExternal
// retorna erro explícito em vez de duplicar linhas.
func ensureTaskNoteExternalUniqueIndex() {
	if db == nil {
		return
	}
	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS ux_task_notes_external_source_id ON task_notes (external_source, external_id) WHERE external_source <> '' AND external_id <> ''`)
}

func ensureChatMessageWindowIndex() {
	if db == nil {
		return
	}
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_chat_messages_window ON chat_messages (conversation_id, parent_id, created_at, id)`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_chat_messages_timeline_window ON chat_messages (conversation_id, parent_id, turn_id, created_at, id)`)
}

// dedupCredentialEntriesBeforeMigrate remove duplicatas em
// `(user_id, pattern)` em bases legadas antes que o AutoMigrate tente
// criar o índice unique. Mantém a entry mais recente (maior `updated_at`,
// ties por `id` UUIDv7 desc) por chave (user_id, pattern). É idempotente:
// se a tabela ainda não existe ou já está sem duplicatas, é noop.
//
// Roda **antes** do AutoMigrate porque o GORM cria o índice unique a
// partir da tag `uniqueIndex` no model, e bases pré-AEP-0052 podiam ter
// `pattern` repetido entre registros legacy sem dono — sem dedup prévio o
// AutoMigrate falha e o app não sobe (review do AEP-0052, Bloco 6, B31).
func dedupCredentialEntriesBeforeMigrate() {
	if db == nil {
		return
	}
	if !db.Migrator().HasTable("credential_entries") {
		return
	}
	if !legacyColumnExists("credential_entries", "user_id") {
		return
	}
	res := db.Exec(`
		DELETE FROM credential_entries
		WHERE pattern IS NOT NULL
		AND id NOT IN (
			SELECT id FROM credential_entries ce
			WHERE ce.id = (
			    SELECT inner_ce.id FROM credential_entries inner_ce
			    WHERE inner_ce.user_id = ce.user_id
			      AND inner_ce.pattern = ce.pattern
			    ORDER BY inner_ce.updated_at DESC, inner_ce.id DESC
			    LIMIT 1
			)
		)
	`)
	if res.Error != nil {
		log.Printf("[Database] AVISO: dedup de credential_entries falhou: %v", res.Error)
	} else if res.RowsAffected > 0 {
		log.Printf("[Database] dedup de credential_entries: %d duplicatas removidas (user_id, pattern)", res.RowsAffected)
	}
}

// ensureCredentialEntryUserPatternIndex limpa índices legados que possam
// existir em DBs antigos. O índice unique atual em (user_id, pattern) é
// criado pela tag `uniqueIndex:ux_credential_entries_user_pattern` no
// `CredentialEntry` model durante o AutoMigrate.
//
// Limitação aceita (review do AEP-0052, M42): o índice é full, não filtra
// pattern vazio. Patterns vazios também disputam unicidade. Isso é exigência
// do SQLite para que o UPSERT (`clause.OnConflict`) usado em
// `credentials/db_store.go` funcione — SQLite só aceita ON CONFLICT contra
// índices unique sem `WHERE`. Em prática o app sempre grava patterns
// não-vazios.
func ensureCredentialEntryUserPatternIndex() {
	if db == nil {
		return
	}

	db.Exec(`DROP INDEX IF EXISTS idx_credential_entries_pattern`)
}

// ensureUsernameCaseInsensitive normaliza usernames legados para lowercase e
// aplica defesa em DB contra registros case-variantes (Alice vs alice).
//
// Decisões (review do AEP-0052, Bloco 6, B34):
//
//   - **Normalização one-shot:** percorre `users` cujo `username` contém
//     maiúsculas e tenta `LOWER(username)`. Se há colisão (ex.: já existe
//     `alice` e tentamos baixar `Alice`), preserva o registro mais antigo
//     (menor `id` UUIDv7 ≈ criado primeiro) e desativa o duplicado em vez de
//     deletar — evita perda silenciosa de dados de um usuário real.
//   - **Defesa em DB:** cria `UNIQUE INDEX users_username_lower_unique ON
//     users(LOWER(username))` para impedir que INSERTs futuros com case
//     diferente coexistam (defense in depth — `IdentityService` já normaliza
//     no `Save`, mas migrações externas ou ferramentas administrativas podiam
//     burlar).
//   - **Compatibilidade:** mantém o índice unique padrão em `username` —
//     ambos coexistem (o em LOWER apenas adiciona invariante adicional).
func ensureUsernameCaseInsensitive() error {
	if db == nil {
		return nil
	}
	if !db.Migrator().HasTable(&User{}) {
		return nil
	}

	type userRow struct {
		ID       string
		Username string
	}
	var legacyMixedCase []userRow
	if err := db.Raw(`SELECT id, username FROM users WHERE username <> LOWER(username)`).Scan(&legacyMixedCase).Error; err != nil {
		return fmt.Errorf("scan legacy mixed-case usernames: %w", err)
	}

	for _, row := range legacyMixedCase {
		lower := strings.ToLower(row.Username)
		var conflictID string
		err := db.Raw(`SELECT id FROM users WHERE username = ? AND id <> ? LIMIT 1`, lower, row.ID).Scan(&conflictID).Error
		if err != nil {
			return fmt.Errorf("check username collision %q: %w", row.Username, err)
		}
		if conflictID != "" {
			loser := row.ID
			if conflictID < loser {
				loser = conflictID
			}
			suffix := loser
			if len(suffix) > 8 {
				suffix = suffix[:8]
			}
			deactivated := fmt.Sprintf("%s.legacy.%s", lower, suffix)
			if err := db.Exec(`UPDATE users SET username = ?, is_active = 0 WHERE id = ?`, deactivated, loser).Error; err != nil {
				return fmt.Errorf("deactivate legacy duplicate username %q: %w", row.Username, err)
			}
			log.Printf("[Database] AVISO: username legacy %q desativado por colisão case-insensitive (id=%s renomeado para %q)", row.Username, loser, deactivated)
			if loser == row.ID {
				continue
			}
		}
		if err := db.Exec(`UPDATE users SET username = ? WHERE id = ?`, lower, row.ID).Error; err != nil {
			return fmt.Errorf("normalize username %q: %w", row.Username, err)
		}
	}

	if len(legacyMixedCase) > 0 {
		log.Printf("[Database] usernames legacy normalizados para lowercase: %d", len(legacyMixedCase))
	}

	db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS users_username_lower_unique ON users (LOWER(username))`)
	return nil
}

// migrateRefreshURLToEnc move dados da coluna legacy `refresh_url` (texto plano)
// para `refresh_token_enc` em `credential_entries` e dropa a coluna antiga.
//
// Decisões (review do AEP-0052, Bloco 6, B30):
//
//   - **Idempotente:** se a coluna `refresh_url` já foi dropada em boot
//     anterior, é noop.
//   - **Não cifra:** o conteúdo era texto plano (URL com token na query) e
//     vai para `refresh_token_enc` como está. A re-cifragem é responsabilidade
//     de quem ler/usar o valor (`credentials.Manager` cifra no save). Isso
//     **não** atende à expectativa do reviewer de cifragem por DEK durante a
//     migração (DEK não está disponível no boot, antes do login). Após esta
//     migração, o conteúdo permanece em plain dentro de `refresh_token_enc`
//     até o próximo write/refresh.
//   - **Logs:** registra quantas linhas foram tocadas e se o drop falhou.
//   - **DROP COLUMN via SQL direto:** `Migrator().DropColumn` faz lookup na
//     struct Go (que não tem mais o campo) e vira noop silencioso. SQL puro
//     `ALTER TABLE ... DROP COLUMN` é suportado em SQLite >= 3.35 (todas as
//     builds modernas, inclusive `glebarez/sqlite` Pure Go).
//   - **Sem transação dedicada:** GORM/SQLite executa ALTER + UPDATE em
//     auto-commit, mas em caso de crash entre passos a próxima execução
//     reinicia do ponto correto (idempotente).
func migrateRefreshURLToEnc() error {
	if db == nil {
		return nil
	}
	if !legacyColumnExists("credential_entries", "refresh_url") {
		return nil
	}

	res := db.Exec(`UPDATE credential_entries SET refresh_token_enc = refresh_url WHERE refresh_url IS NOT NULL AND refresh_url <> '' AND (refresh_token_enc IS NULL OR refresh_token_enc = '')`)
	if res.Error != nil {
		return fmt.Errorf("migrar refresh_url para refresh_token_enc: %w", res.Error)
	}
	if res.RowsAffected > 0 {
		log.Printf("[Database] credential_entries: %d linhas migradas refresh_url → refresh_token_enc", res.RowsAffected)
	}

	if err := db.Exec(`ALTER TABLE credential_entries DROP COLUMN refresh_url`).Error; err != nil {
		log.Printf("[Database] AVISO: falha ao dropar coluna legacy refresh_url: %v", err)
	}
	return nil
}

// legacyColumnExists checa se uma coluna existe no DB via PRAGMA, sem
// depender da struct Go atual (necessário para colunas removidas do model
// mas ainda presentes no schema legado).
func legacyColumnExists(table, column string) bool {
	if db == nil {
		return false
	}
	var n int
	err := db.Raw(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column).Scan(&n).Error
	return err == nil && n > 0
}
