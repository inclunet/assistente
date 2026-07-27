package channels

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"assistente/internal/database"
	"assistente/internal/logging"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	storeDB      *gorm.DB
	knownOwners  sync.Map // slug → userID visto em Save/Adopt
)

// UseDatabase ativa a persistência SQLite para a fachada channels (AEP-0083).
// Deve ser chamado no boot após database.Init. Sem isso, o pacote usa filesystem.
func UseDatabase(db *gorm.DB) {
	mu.Lock()
	defer mu.Unlock()
	storeDB = db
}

func usingDB() bool {
	return storeDB != nil
}

func rememberOwner(slug, userID string) {
	slug = strings.TrimSpace(slug)
	userID = strings.TrimSpace(userID)
	if slug == "" || userID == "" {
		return
	}
	knownOwners.Store(slug, userID)
}

func knownOwner(slug string) string {
	if v, ok := knownOwners.Load(strings.TrimSpace(slug)); ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func loadConversations(tx *gorm.DB, channelID string) (map[string]string, error) {
	var rows []database.ChannelContactConversation
	if err := tx.Where("channel_id = ?", channelID).Find(&rows).Error; err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		out[r.ContactExternalID] = r.ConversationID
	}
	return out, nil
}

// loadConversationsByChannelIDs evita N+1 em ListAll/ListForUser.
func loadConversationsByChannelIDs(tx *gorm.DB, channelIDs []string) (map[string]map[string]string, error) {
	out := make(map[string]map[string]string, len(channelIDs))
	if len(channelIDs) == 0 {
		return out, nil
	}
	var rows []database.ChannelContactConversation
	if err := tx.Where("channel_id IN ?", channelIDs).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, r := range rows {
		m := out[r.ChannelID]
		if m == nil {
			m = make(map[string]string)
			out[r.ChannelID] = m
		}
		m[r.ContactExternalID] = r.ConversationID
	}
	return out, nil
}

func syncConversations(tx *gorm.DB, channelID string, conversations map[string]string) error {
	if err := tx.Where("channel_id = ?", channelID).Delete(&database.ChannelContactConversation{}).Error; err != nil {
		return err
	}
	for contactID, convID := range conversations {
		contactID = strings.TrimSpace(contactID)
		convID = strings.TrimSpace(convID)
		if contactID == "" || convID == "" {
			continue
		}
		row := database.ChannelContactConversation{
			ChannelID:         channelID,
			ContactExternalID: contactID,
			ConversationID:    convID,
		}
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

// findChannelRow resolve uma row por slug com a heurística do gateway (sem user ctx):
// 1) exatamente um enabled com o slug; 2) owner conhecido; 3) exatamente uma row qualquer.
func findChannelRow(tx *gorm.DB, slug string) (*database.Channel, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return nil, nil
	}

	var enabled []database.Channel
	if err := tx.Where("slug = ? AND enabled = ?", slug, true).Find(&enabled).Error; err != nil {
		return nil, err
	}
	if len(enabled) == 1 {
		return &enabled[0], nil
	}

	if owner := knownOwner(slug); owner != "" {
		var row database.Channel
		err := tx.Where("slug = ? AND user_id = ?", slug, owner).First(&row).Error
		if err == nil {
			return &row, nil
		}
		if err != gorm.ErrRecordNotFound {
			return nil, err
		}
	}

	var all []database.Channel
	if err := tx.Where("slug = ?", slug).Find(&all).Error; err != nil {
		return nil, err
	}
	if len(all) == 1 {
		return &all[0], nil
	}
	if len(enabled) > 1 {
		logging.Warnf(context.Background(), "channels.store", "[Channels] slug %q tem %d canais enabled; Load ambíguo", slug, len(enabled))
	}
	return nil, nil
}

func findChannelRowForUser(tx *gorm.DB, slug, userID string) (*database.Channel, error) {
	slug = strings.TrimSpace(slug)
	userID = strings.TrimSpace(userID)
	var row database.Channel
	q := tx.Where("slug = ?", slug)
	if userID != "" {
		q = q.Where("user_id = ?", userID)
	} else {
		q = q.Where("user_id = '' OR user_id IS NULL")
	}
	err := q.First(&row).Error
	if err == nil {
		return &row, nil
	}
	if err != gorm.ErrRecordNotFound {
		return nil, err
	}
	// Save autenticado: adotar órfão com o mesmo slug em vez de criar duplicata.
	if userID != "" {
		var orphan database.Channel
		err = tx.Where("slug = ? AND (user_id = '' OR user_id IS NULL)", slug).First(&orphan).Error
		if err == nil {
			return &orphan, nil
		}
		if err != gorm.ErrRecordNotFound {
			return nil, err
		}
	}
	return nil, nil
}

func loadFromDB(slug string) (*ChannelConfig, error) {
	tx := storeDB
	row, err := findChannelRow(tx, slug)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, nil
	}
	convs, err := loadConversations(tx, row.ID)
	if err != nil {
		return nil, err
	}
	cfg := RowToConfig(row, convs)
	rememberOwner(slug, cfg.OwnerUserID)
	return cfg, nil
}

func saveToDB(slug string, cfg *ChannelConfig) error {
	if cfg == nil {
		return fmt.Errorf("config do canal %s é nil", slug)
	}
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return fmt.Errorf("slug do canal é obrigatório")
	}
	if strings.TrimSpace(cfg.Type) == "" {
		cfg.Type = slug
	}
	if strings.TrimSpace(cfg.DisplayName) == "" {
		cfg.DisplayName = defaultDisplayName(cfg.Type)
	}

	userID := strings.TrimSpace(cfg.OwnerUserID)
	return storeDB.Transaction(func(tx *gorm.DB) error {
		existing, err := findChannelRowForUser(tx, slug, userID)
		if err != nil {
			return err
		}
		// UI/partial Save: preservar mapas runtime se o caller não enviou.
		preserveConversations := cfg.Conversations == nil
		if existing != nil {
			existingCfg := RowToConfig(existing, nil)
			if cfg.ReplyChatIDs == nil && len(existingCfg.ReplyChatIDs) > 0 {
				cfg.ReplyChatIDs = existingCfg.ReplyChatIDs
			}
			// Settings Signal: se Account/APIURL vierem vazios no partial save,
			// manter os valores já persistidos.
			if strings.TrimSpace(cfg.Account) == "" && existingCfg.Account != "" {
				cfg.Account = existingCfg.Account
			}
			if strings.TrimSpace(cfg.APIURL) == "" && existingCfg.APIURL != "" {
				cfg.APIURL = existingCfg.APIURL
			}
		}

		row := ConfigToRow(slug, cfg)
		if existing != nil {
			row.ID = existing.ID
			row.CreatedAt = existing.CreatedAt
			if err := tx.Save(&row).Error; err != nil {
				return err
			}
		} else {
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		if !preserveConversations {
			if err := syncConversations(tx, row.ID, cfg.Conversations); err != nil {
				return err
			}
		}
		rememberOwner(slug, userID)
		return nil
	})
}

func deleteFromDB(slug string) error {
	slug = strings.TrimSpace(slug)
	return storeDB.Transaction(func(tx *gorm.DB) error {
		row, err := findChannelRow(tx, slug)
		if err != nil {
			return err
		}
		if row == nil {
			return nil
		}
		if err := tx.Where("channel_id = ?", row.ID).Delete(&database.ChannelContactConversation{}).Error; err != nil {
			return err
		}
		if err := tx.Where("channel_id = ?", row.ID).Delete(&database.ChannelContact{}).Error; err != nil {
			return err
		}
		return tx.Delete(&database.Channel{}, "id = ?", row.ID).Error
	})
}

func listAllFromDB() (map[string]*ChannelConfig, error) {
	var rows []database.Channel
	if err := storeDB.Order("slug ASC, created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	convsByChannel, err := loadConversationsByChannelIDs(storeDB, ids)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*ChannelConfig, len(rows))
	for i := range rows {
		row := rows[i]
		cfg := RowToConfig(&row, convsByChannel[row.ID])
		// Em colisão de slug (multi-user), preserva a primeira e loga.
		if prev, ok := result[row.Slug]; ok {
			logging.Warnf(context.Background(), "channels.store",
				"[Channels] ListAll: slug %q duplicado (owners %q e %q); mantendo o primeiro",
				row.Slug, prev.OwnerUserID, cfg.OwnerUserID)
			continue
		}
		result[row.Slug] = cfg
		rememberOwner(row.Slug, cfg.OwnerUserID)
	}
	return result, nil
}

// ListForUser retorna canais do userID + órfãos (user_id vazio), chaveados por slug.
func ListForUser(userID string) (map[string]*ChannelConfig, error) {
	mu.Lock()
	defer mu.Unlock()
	if !usingDB() {
		all, err := listAllUnsafe()
		if err != nil {
			return nil, err
		}
		out := make(map[string]*ChannelConfig)
		for name, cfg := range all {
			owner := strings.TrimSpace(cfg.OwnerUserID)
			if owner == "" || owner == userID {
				out[name] = cfg
			}
		}
		return out, nil
	}
	userID = strings.TrimSpace(userID)
	var rows []database.Channel
	q := storeDB.Order("slug ASC, created_at ASC")
	if userID == "" {
		q = q.Where("user_id = '' OR user_id IS NULL")
	} else {
		q = q.Where("user_id = ? OR user_id = '' OR user_id IS NULL", userID)
	}
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for i := range rows {
		ids = append(ids, rows[i].ID)
	}
	convsByChannel, err := loadConversationsByChannelIDs(storeDB, ids)
	if err != nil {
		return nil, err
	}
	result := make(map[string]*ChannelConfig, len(rows))
	for i := range rows {
		row := rows[i]
		cfg := RowToConfig(&row, convsByChannel[row.ID])
		if prev, ok := result[row.Slug]; ok {
			// Prefer owned over orphan on collision.
			if strings.TrimSpace(prev.OwnerUserID) == "" && strings.TrimSpace(cfg.OwnerUserID) != "" {
				result[row.Slug] = cfg
			}
			continue
		}
		result[row.Slug] = cfg
		rememberOwner(row.Slug, cfg.OwnerUserID)
	}
	return result, nil
}

func saveConversationIDDB(channelName, contactID, conversationID string) error {
	return storeDB.Transaction(func(tx *gorm.DB) error {
		row, err := findChannelRow(tx, channelName)
		if err != nil {
			return err
		}
		if row == nil {
			return fmt.Errorf("canal %s não encontrado", channelName)
		}
		rec := database.ChannelContactConversation{
			ChannelID:         row.ID,
			ContactExternalID: contactID,
			ConversationID:    conversationID,
		}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "channel_id"}, {Name: "contact_external_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"conversation_id", "updated_at"}),
		}).Create(&rec).Error
	})
}

func adoptOrphansDB(userID string) ([]string, error) {
	var rows []database.Channel
	if err := storeDB.Where("user_id = '' OR user_id IS NULL").Find(&rows).Error; err != nil {
		return nil, err
	}
	migrated := make([]string, 0, len(rows))
	for i := range rows {
		row := rows[i]
		if strings.TrimSpace(row.UserID) != "" {
			continue
		}
		if err := storeDB.Model(&row).Update("user_id", userID).Error; err != nil {
			return migrated, fmt.Errorf("erro ao migrar canal DB %s: %w", row.Slug, err)
		}
		// Also adopt contacts for this channel.
		if err := storeDB.Model(&database.ChannelContact{}).
			Where("channel_id = ? AND (user_id = '' OR user_id IS NULL)", row.ID).
			Update("user_id", userID).Error; err != nil {
			return migrated, fmt.Errorf("erro ao migrar contatos do canal DB %s: %w", row.Slug, err)
		}
		rememberOwner(row.Slug, userID)
		migrated = append(migrated, row.Slug)
	}
	return migrated, nil
}

// ChannelIDBySlug resolve o UUID do canal para o pacote contacts (DB mode).
// Retorna ErrChannelNotFound quando o slug não existe.
func ChannelIDBySlug(slug string) (string, string, error) {
	mu.Lock()
	defer mu.Unlock()
	if !usingDB() {
		return "", "", fmt.Errorf("channels DB não habilitado")
	}
	row, err := findChannelRow(storeDB, slug)
	if err != nil {
		return "", "", err
	}
	if row == nil {
		return "", "", fmt.Errorf("%w: %s", ErrChannelNotFound, slug)
	}
	return row.ID, row.UserID, nil
}

// DB retorna o *gorm.DB injetado (nil se filesystem).
func DB() *gorm.DB {
	return storeDB
}
