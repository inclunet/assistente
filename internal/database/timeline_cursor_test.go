package database

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
)

func TestTimelineCursorV2RejectsOldVersionAndChangedContext(t *testing.T) {
	item := MessageWindowItem{Kind: MessageWindowItemKindTurn, ID: "opaque:turn", AnchorID: "opaque:turn", AnchorCreatedAt: "2026-11-07 12:34:56.123456789"}
	query := MessageWindowQuery{ConversationID: "conversation", Cursor: EncodeTimelineCursor("conversation", nil, item)}
	cursor, err := decodeTimelineCursor(query)
	if err != nil || !cursor.matches(item) {
		t.Fatal("cursor v2 perdeu identidade", err)
	}
	changed := item
	changed.AnchorCreatedAt += "1"
	if cursor.matches(changed) {
		t.Fatal("cursor aceitou data alterada")
	}
	for _, version := range []int{0, 1, 3} {
		old := *cursor
		old.Version = version
		raw, err := json.Marshal(old)
		if err != nil {
			t.Fatal(err)
		}
		invalid := query
		invalid.Cursor = base64.RawURLEncoding.EncodeToString(raw)
		if _, err := decodeTimelineCursor(invalid); !errors.Is(err, ErrTimelineCursorInvalid) {
			t.Fatal("versão inválida aceita", version, err)
		}
	}
	other := query
	other.ConversationID = "other"
	if _, err := decodeTimelineCursor(other); !errors.Is(err, ErrTimelineCursorInvalid) {
		t.Fatal("outra conversa aceita", err)
	}
	parent := "thread"
	other = query
	other.ParentID = &parent
	if _, err := decodeTimelineCursor(other); !errors.Is(err, ErrTimelineCursorInvalid) {
		t.Fatal("outro escopo aceito", err)
	}
}
