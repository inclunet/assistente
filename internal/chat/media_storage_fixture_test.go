package chat

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"assistente/internal/database"
	"gorm.io/gorm"
)

// Fixtures use the production attachment writer, then move its links onto an
// already admitted response. This does not require a production-only test API.
func attachFixtureMedia(t testing.TB, db *gorm.DB, messageID, media string) {
	t.Helper()
	var target struct{ ConversationID, UserID string }
	if err := db.Raw("SELECT m.conversation_id,c.user_id FROM chat_messages m JOIN conversations c ON c.id=m.conversation_id WHERE m.id=?", messageID).Scan(&target).Error; err != nil {
		t.Fatal(err)
	}
	ctx := database.WithUserID(context.Background(), target.UserID)
	scratch, err := database.NewMessageRepository(db).CreateMessageWithContext(ctx, database.MessageOptions{ConversationID: target.ConversationID, Role: "user", Media: media})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("UPDATE chat_message_attachments SET message_id=? WHERE message_id=?", messageID, scratch.ID).Error; err != nil {
			return err
		}
		return tx.Delete(&database.ChatMessage{}, "id=?", scratch.ID).Error
	}); err != nil {
		t.Fatal(err)
	}
}

func equalFixtureMedia(left, right string) bool {
	var a, b any
	return json.Unmarshal([]byte(left), &a) == nil && json.Unmarshal([]byte(right), &b) == nil && reflect.DeepEqual(a, b)
}
