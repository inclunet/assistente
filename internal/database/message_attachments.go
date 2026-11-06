package database

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	AttachmentReceived      = "received"
	AttachmentGenerated     = "generated"
	AttachmentLegacyUnknown = "legacy_unknown"
)

func validAttachmentOrigin(origin string) bool {
	return origin == AttachmentReceived || origin == AttachmentGenerated || origin == AttachmentLegacyUnknown
}

func decodeAttachment(data string) ([]byte, error) {
	compact := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
			return -1
		}
		return r
	}, data)
	decoded, err := base64.StdEncoding.DecodeString(compact)
	if err != nil || len(decoded) == 0 {
		return nil, errors.New("conteúdo de anexo não é base64 válido e não vazio")
	}
	return decoded, nil
}

// Only byte equality with an explicitly received media part proves provenance.
func legacyAudioOrigin(media string, audio []byte) string {
	var parts []struct {
		Type string `json:"type"`
		Data string `json:"data"`
	}
	if json.Unmarshal([]byte(media), &parts) == nil {
		for _, part := range parts {
			if strings.HasPrefix(strings.ToLower(strings.TrimSpace(part.Type)), "audio/") {
				data, err := decodeAttachment(part.Data)
				if err == nil && string(data) == string(audio) {
					return AttachmentReceived
				}
			}
		}
	}
	return AttachmentLegacyUnknown
}

// Caller holds the transaction and has authorized the message (or is migrating).
// Ownership is also enforced by SQLite, including writes outside this repository.
func storeMessageAudio(tx *gorm.DB, messageID, data, mime, origin string) error {
	if !validAttachmentOrigin(origin) {
		return errors.New("origem de anexo inválida")
	}
	var owner string
	if err := tx.Raw("SELECT c.user_id FROM chat_messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.id=?", messageID).Scan(&owner).Error; err != nil {
		return err
	}
	if owner == "" {
		return gorm.ErrRecordNotFound
	}
	if data == "" {
		return tx.Exec("DELETE FROM chat_message_attachments WHERE message_id=? AND purpose='audio'", messageID).Error
	}
	decoded, err := decodeAttachment(data)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(decoded)
	hash := hex.EncodeToString(sum[:])
	id, err := uuid.NewV7()
	if err != nil {
		return err
	}
	if err := tx.Exec(`INSERT INTO chat_attachment_contents(id,user_id,sha256,size,content) VALUES (?,?,?,?,?) ON CONFLICT(user_id,sha256) DO NOTHING`, id.String(), owner, hash, len(decoded), decoded).Error; err != nil {
		return err
	}
	var contentID string
	if err := tx.Raw(`SELECT id FROM chat_attachment_contents WHERE user_id=? AND sha256=? AND size=? AND content=?`, owner, hash, len(decoded), decoded).Scan(&contentID).Error; err != nil {
		return err
	}
	if contentID == "" {
		return errors.New("integridade do conteúdo de anexo inválida")
	}
	return tx.Exec(`INSERT INTO chat_message_attachments(message_id,purpose,position,content_id,origin,mime_type) VALUES (?,'audio',0,?,?,?) ON CONFLICT(message_id,purpose,position) DO UPDATE SET content_id=excluded.content_id,origin=excluded.origin,mime_type=excluded.mime_type`, messageID, contentID, origin, mime).Error
}

// Hydrate only consumers requiring audio. Visual readers never call this method.
func LoadMessageAudios(ctx context.Context, tx *gorm.DB, messages []ChatMessage, includeGenerated bool) error {
	user, err := RequireUserID(ctx)
	if err != nil {
		return err
	}
	for start := 0; start < len(messages); start += 400 {
		end := min(start+400, len(messages))
		ids := make([]string, 0, end-start)
		byID := map[string][]int{}
		for i := start; i < end; i++ {
			ids = append(ids, messages[i].ID)
			byID[messages[i].ID] = append(byID[messages[i].ID], i)
			messages[i].Audio = ""
			messages[i].AudioMimeType = ""
			messages[i].AudioOrigin = ""
		}
		var rows []struct {
			MessageID, Origin, MimeType, SHA256 string
			Size                                int
			Content                             []byte
		}
		q := tx.WithContext(ctx).Table("chat_message_attachments a").Select("a.message_id,a.origin,a.mime_type,b.sha256,b.size,b.content").Joins("JOIN chat_attachment_contents b ON b.id=a.content_id").Joins("JOIN chat_messages m ON m.id=a.message_id").Joins("JOIN conversations c ON c.id=m.conversation_id").Where("c.user_id=? AND b.user_id=? AND a.purpose='audio' AND a.message_id IN ?", user, user, ids)
		if !includeGenerated {
			q = q.Where("a.origin<>?", AttachmentGenerated)
		}
		if err := q.Scan(&rows).Error; err != nil {
			return err
		}
		for _, row := range rows {
			sum := sha256.Sum256(row.Content)
			if len(row.Content) != row.Size || hex.EncodeToString(sum[:]) != row.SHA256 {
				return fmt.Errorf("integridade do anexo da mensagem %s inválida", row.MessageID)
			}
			encoded := base64.StdEncoding.EncodeToString(row.Content)
			for _, i := range byID[row.MessageID] {
				messages[i].Audio = encoded
				messages[i].AudioMimeType = row.MimeType
				messages[i].AudioOrigin = row.Origin
			}
		}
	}
	return nil
}
