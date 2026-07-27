package contacts

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"assistente/internal/channels"
	"assistente/internal/database"

	"gorm.io/gorm"
)

var storeDB *gorm.DB

// UseDatabase ativa persistência de contatos no SQLite (AEP-0083).
// Depende de channels.UseDatabase já ter sido chamado para resolver channel_id.
func UseDatabase(db *gorm.DB) {
	mu.Lock()
	defer mu.Unlock()
	storeDB = db
}

func usingDB() bool {
	return storeDB != nil
}

func resolveChannel(slug string) (channelID, userID string, err error) {
	channelID, userID, err = channels.ChannelIDBySlug(slug)
	if err != nil {
		return "", "", err
	}
	if channelID == "" {
		return "", "", fmt.Errorf("%w: %s", channels.ErrChannelNotFound, slug)
	}
	return channelID, userID, nil
}

func isChannelNotFound(err error) bool {
	return errors.Is(err, channels.ErrChannelNotFound)
}

func contactFromRow(row *database.ChannelContact) *AuthorizedContact {
	if row == nil {
		return nil
	}
	at := ""
	if row.AuthorizedAt != nil {
		at = row.AuthorizedAt.UTC().Format(time.RFC3339)
	}
	return &AuthorizedContact{
		ID:           row.ExternalID,
		DisplayName:  row.DisplayName,
		Username:     row.Username,
		AuthorizedAt: at,
	}
}

func getForChannelDB(channel string) ([]*AuthorizedContact, error) {
	channelID, _, err := resolveChannel(channel)
	if err != nil {
		// Canal inexistente → lista vazia (compatível com FS quando chave ausente).
		if isChannelNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	var rows []database.ChannelContact
	if err := storeDB.Where("channel_id = ?", channelID).Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]*AuthorizedContact, 0, len(rows))
	for i := range rows {
		out = append(out, contactFromRow(&rows[i]))
	}
	return out, nil
}

func getAllDB() (ContactsFile, error) {
	allChannels, err := channels.ListAll()
	if err != nil {
		return nil, err
	}
	out := make(ContactsFile)
	for slug := range allChannels {
		list, err := getForChannelDB(slug)
		if err != nil {
			return nil, err
		}
		if len(list) > 0 {
			out[slug] = list
		}
	}
	return out, nil
}

func isAuthorizedDB(channel string, maxContacts int, identifiers ...string) (hasContacts bool, isAllowed bool) {
	maxContacts = normalizeMaxContacts(maxContacts)
	list, err := getForChannelDB(channel)
	if err != nil || len(list) == 0 {
		return false, false
	}
	for _, contact := range list {
		for _, id := range identifiers {
			if id == "" {
				continue
			}
			if id == contact.ID || id == contact.Username {
				return true, true
			}
		}
	}
	if maxContacts > 0 && len(list) >= maxContacts {
		return true, false
	}
	return false, false
}

func authorizeDB(channel, id, displayName, username string, maxContacts int) error {
	maxContacts = normalizeMaxContacts(maxContacts)
	channelID, userID, err := resolveChannel(channel)
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	var existing database.ChannelContact
	err = storeDB.Where("channel_id = ? AND external_id = ?", channelID, id).First(&existing).Error
	if err == nil {
		existing.DisplayName = displayName
		existing.Username = username
		existing.AuthorizedAt = &now
		if userID != "" && strings.TrimSpace(existing.UserID) == "" {
			existing.UserID = userID
		}
		return storeDB.Save(&existing).Error
	}
	if err != gorm.ErrRecordNotFound {
		return err
	}

	var count int64
	if err := storeDB.Model(&database.ChannelContact{}).Where("channel_id = ?", channelID).Count(&count).Error; err != nil {
		return err
	}
	if maxContacts > 0 && int(count) >= maxContacts {
		return fmt.Errorf("limite de %d contato(s) atingido para o canal %s", maxContacts, channel)
	}

	row := database.ChannelContact{
		UserID:       userID,
		ChannelID:    channelID,
		ExternalID:   id,
		DisplayName:  displayName,
		Username:     username,
		AuthorizedAt: &now,
	}
	return storeDB.Create(&row).Error
}

func removeDB(channel, contactID string) error {
	channelID, _, err := resolveChannel(channel)
	if err != nil {
		if isChannelNotFound(err) {
			return nil
		}
		return err
	}
	return storeDB.Where("channel_id = ? AND external_id = ?", channelID, contactID).
		Delete(&database.ChannelContact{}).Error
}

func removeAllDB(channel string) error {
	channelID, _, err := resolveChannel(channel)
	if err != nil {
		if isChannelNotFound(err) {
			return nil
		}
		return err
	}
	return storeDB.Where("channel_id = ?", channelID).Delete(&database.ChannelContact{}).Error
}

func countDB(channel string) int {
	channelID, _, err := resolveChannel(channel)
	if err != nil {
		return 0
	}
	var count int64
	_ = storeDB.Model(&database.ChannelContact{}).Where("channel_id = ?", channelID).Count(&count)
	return int(count)
}
