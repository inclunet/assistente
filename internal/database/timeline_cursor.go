package database

import (
	"encoding/base64"
	"encoding/json"
	"errors"
)

var ErrTimelineCursorInvalid = errors.New("cursor de timeline inválido")
var ErrTimelineCursorStale = errors.New("âncora de timeline mudou; sincronização necessária")

// Cursor não concede acesso. A leitura autoriza owner/conversa antes de validar
// sua chave contra o mesmo snapshot SQL usado na paginação.
type timelineCursor struct {
	Version        int     `json:"v"`
	ConversationID string  `json:"c"`
	ParentID       *string `json:"p"`
	CreatedAt      string  `json:"t"`
	AnchorID       string  `json:"a"`
	Slot           int     `json:"s"`
	ItemID         string  `json:"i"`
}

func EncodeTimelineCursor(conversationID string, parentID *string, item MessageWindowItem) string {
	key := timelineKeyArgs(item)
	value := timelineCursor{1, conversationID, parentID, item.AnchorCreatedAt, item.AnchorID, key[2].(int), key[3].(string)}
	data, _ := json.Marshal(value) // apenas strings, ponteiro de string e inteiros
	return base64.RawURLEncoding.EncodeToString(data)
}

func decodeTimelineCursor(query MessageWindowQuery) (*timelineCursor, error) {
	if len(query.Cursor) > 8192 || query.Anchor != "" || query.AnchorItemID != "" || query.AnchorMessageID != "" {
		return nil, ErrTimelineCursorInvalid
	}
	data, err := base64.RawURLEncoding.Strict().DecodeString(query.Cursor)
	if err != nil {
		return nil, ErrTimelineCursorInvalid
	}
	var cursor timelineCursor
	if json.Unmarshal(data, &cursor) != nil || cursor.Version != 1 || cursor.ConversationID != query.ConversationID || !sameCursorScope(cursor.ParentID, query.ParentID) || cursor.CreatedAt == "" || cursor.AnchorID == "" || cursor.ItemID == "" || cursor.Slot < 0 || cursor.Slot > 1 {
		return nil, ErrTimelineCursorInvalid
	}
	return &cursor, nil
}

func sameCursorScope(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func (cursor *timelineCursor) matches(item MessageWindowItem) bool {
	key := timelineKeyArgs(item)
	return cursor.CreatedAt == item.AnchorCreatedAt && cursor.AnchorID == item.AnchorID && cursor.Slot == key[2].(int) && cursor.ItemID == key[3].(string)
}
