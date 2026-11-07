package database

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
)

var ErrTimelineCursorInvalid = errors.New("cursor de timeline inválido")
var ErrTimelineCursorStale = errors.New("âncora de timeline mudou; sincronização necessária")

// Cursor não concede acesso. A leitura autoriza owner/conversa antes de validar
// sua chave contra o mesmo snapshot SQL usado na paginação.
type timelineCursor struct {
	Version        int     `json:"v"`
	ConversationID string  `json:"c"`
	ParentID       *string `json:"p,omitempty"`
	CreatedAt      string  `json:"t"`
	ItemID         string  `json:"i"`
}

func EncodeTimelineCursor(conversationID string, parentID *string, item MessageWindowItem) string {
	key := timelineKeyArgs(item)
	value := timelineCursor{2, conversationID, parentID, item.AnchorCreatedAt, key[3].(string)}
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
	if json.Unmarshal(data, &cursor) != nil || cursor.Version != 2 || cursor.ConversationID != query.ConversationID || !sameCursorScope(cursor.ParentID, query.ParentID) || cursor.CreatedAt == "" {
		return nil, ErrTimelineCursorInvalid
	}
	kind, id, found := strings.Cut(cursor.ItemID, ":")
	if !found || id == "" || (kind != "message" && kind != "turn") {
		return nil, ErrTimelineCursorInvalid
	}
	return &cursor, nil
}

func sameCursorScope(a, b *string) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func (cursor *timelineCursor) matches(item MessageWindowItem) bool {
	key := timelineKeyArgs(item)
	// ItemID já determina âncora e slot; conferir novamente a data impede
	// reutilizar uma fronteira cuja âncora persistida mudou.
	return cursor.CreatedAt == item.AnchorCreatedAt && cursor.ItemID == key[3].(string)
}
