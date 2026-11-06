package database

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/gorm"
)

// Only transport consumers hydrate media. Storage keeps bytes separate from
// per-occurrence metadata, including names and unknown historical fields.
type storedMediaPart struct {
	Metadata   string
	DataField  string
	DataPrefix string
	MIME       string
	Content    []byte
	HasContent bool
}

func parseStoredMedia(media string) ([]storedMediaPart, error) {
	if strings.TrimSpace(media) == "" {
		return nil, nil
	}
	var parts []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(media), &parts); err != nil {
		return nil, errors.New("mídia deve ser uma lista JSON de anexos")
	}
	result := make([]storedMediaPart, 0, len(parts))
	for _, part := range parts {
		if part == nil {
			return nil, errors.New("anexo de mídia nulo")
		}
		item := storedMediaPart{}
		_ = json.Unmarshal(part["type"], &item.MIME)
		field := "data"
		raw, found := part[field]
		if !found {
			field = "url"
			raw, found = part[field]
		}
		if found {
			var data string
			if err := json.Unmarshal(raw, &data); err != nil {
				return nil, errors.New("conteúdo de mídia deve ser texto")
			}
			if field == "data" || strings.HasPrefix(data, "data:") {
				if strings.HasPrefix(data, "data:") {
					prefix, payload, ok := strings.Cut(data, ",")
					if !ok || !strings.HasSuffix(prefix, ";base64") {
						return nil, errors.New("data URI de mídia requer base64")
					}
					item.DataPrefix = prefix + ","
					data = payload
				}
				compact := strings.Map(func(r rune) rune {
					if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
						return -1
					}
					return r
				}, data)
				decoded, err := base64.StdEncoding.DecodeString(compact)
				if err != nil {
					return nil, errors.New("conteúdo de mídia não é base64 válido")
				}
				if decoded == nil {
					decoded = []byte{}
				}
				item.Content = decoded
				item.HasContent = true
				item.DataField = field
				delete(part, field)
			}
		}
		metadata, err := json.Marshal(part)
		if err != nil {
			return nil, err
		}
		item.Metadata = string(metadata)
		result = append(result, item)
	}
	return result, nil
}

func storeMessageMedia(tx *gorm.DB, messageID, media string) error {
	parts, err := parseStoredMedia(media)
	if err != nil {
		return err
	}
	var owner string
	if err := tx.Raw("SELECT c.user_id FROM chat_messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.id=?", messageID).Scan(&owner).Error; err != nil {
		return err
	}
	if owner == "" {
		return gorm.ErrRecordNotFound
	}
	if err := tx.Exec("DELETE FROM chat_message_attachments WHERE message_id=? AND purpose='media'", messageID).Error; err != nil {
		return err
	}
	for position, part := range parts {
		var contentID *string
		if part.HasContent {
			id, err := storeAttachmentContent(tx, owner, part.Content)
			if err != nil {
				return err
			}
			contentID = &id
		}
		if err := tx.Exec(`INSERT INTO chat_message_attachments(message_id,purpose,position,content_id,origin,mime_type,metadata,data_field,data_prefix) VALUES (?,'media',?,?,'received',?,?,?,?)`, messageID, position, contentID, part.MIME, part.Metadata, part.DataField, part.DataPrefix).Error; err != nil {
			return err
		}
	}
	// A received audio copied into the old dedicated slot is the same occurrence.
	// Generated or unclassified audio is never inferred away.
	var matches []struct{ AudioMIME, MediaMIME string }
	if err := tx.Raw(`SELECT a.mime_type AS audio_mime,m.mime_type AS media_mime FROM chat_message_attachments a JOIN chat_message_attachments m ON m.message_id=a.message_id AND m.purpose='media' AND m.content_id=a.content_id WHERE a.message_id=? AND a.purpose='audio' AND a.origin='received'`, messageID).Scan(&matches).Error; err != nil {
		return err
	}
	normalize := func(value string) string {
		value, _, _ = strings.Cut(value, ";")
		return strings.ToLower(strings.TrimSpace(value))
	}
	for _, match := range matches {
		if normalize(match.AudioMIME) == normalize(match.MediaMIME) {
			return tx.Exec("DELETE FROM chat_message_attachments WHERE message_id=? AND purpose='audio' AND origin='received'", messageID).Error
		}
	}
	return nil
}

func LoadMessageMedia(ctx context.Context, tx *gorm.DB, messages []ChatMessage) error {
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
			messages[i].Media = ""
		}
		var rows []struct {
			MessageID, Metadata, DataField, DataPrefix string
			ContentID                                  *string
			SHA256                                     string
			Size                                       int
			Content                                    []byte
		}
		if err := tx.WithContext(ctx).Table("chat_message_attachments a").Select("a.message_id,a.metadata,a.data_field,a.data_prefix,a.content_id,b.sha256,b.size,b.content").Joins("JOIN chat_messages m ON m.id=a.message_id JOIN conversations c ON c.id=m.conversation_id").Joins("LEFT JOIN chat_attachment_contents b ON b.id=a.content_id AND b.user_id=c.user_id").Where("c.user_id=? AND a.purpose='media' AND a.message_id IN ?", user, ids).Order("a.message_id,a.position").Scan(&rows).Error; err != nil {
			return err
		}
		parts := map[string][]map[string]json.RawMessage{}
		for _, row := range rows {
			var metadata map[string]json.RawMessage
			if err := json.Unmarshal([]byte(row.Metadata), &metadata); err != nil || metadata == nil {
				return errors.New("metadados de anexo inválidos")
			}
			if row.ContentID != nil {
				if err := validateAttachmentContent(row.Content, row.Size, row.SHA256); err != nil {
					return fmt.Errorf("integridade do anexo da mensagem %s inválida", row.MessageID)
				}
				encoded, _ := json.Marshal(row.DataPrefix + base64.StdEncoding.EncodeToString(row.Content))
				metadata[row.DataField] = encoded
			}
			parts[row.MessageID] = append(parts[row.MessageID], metadata)
		}
		for id, items := range parts {
			encoded, err := json.Marshal(items)
			if err != nil {
				return err
			}
			for _, i := range byID[id] {
				messages[i].Media = string(encoded)
			}
		}
	}
	return nil
}
