package database

import (
	"reflect"
	"testing"
	"time"
)

func TestTimelineMessageScanPreservesValuesAndLegacyNulls(t *testing.T) {
	setupOrderingTestDB(t)
	if err := db.Create(&Conversation{UUIDModel: UUIDModel{ID: "conversation"}, UserID: testUserID, Title: "test"}).Error; err != nil {
		t.Fatal(err)
	}
	parent, turn := "parent", "turn"
	expected := ChatMessage{UUIDModel: UUIDModel{ID: "message", CreatedAt: time.Date(2026, 11, 7, 12, 34, 56, 123456789, time.UTC)},
		ConversationID: "conversation", ParentID: &parent, TurnID: &turn, Role: "assistant", Content: "ação <>&\x00",
		Reasoning: "raciocínio", Model: "model", Source: "channel", Pinned: true,
		PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5, CacheReadTokens: 7, CacheWriteTokens: 11, CacheMissTokens: 13}
	if err := db.Create(&expected).Error; err != nil {
		t.Fatal(err)
	}
	for _, legacyNulls := range []bool{false, true} {
		if legacyNulls {
			if err := db.Exec(`UPDATE chat_messages SET parent_id=NULL, turn_id=NULL, created_at=NULL, updated_at=NULL,
                content=NULL, reasoning=NULL, model=NULL, source=NULL, prompt_tokens=NULL, completion_tokens=NULL,
                total_tokens=NULL, cache_read_tokens=NULL, cache_write_tokens=NULL, cache_miss_tokens=NULL`).Error; err != nil {
				t.Fatal(err)
			}
		}
		var previous []ChatMessage
		if err := db.Find(&previous).Error; err != nil {
			t.Fatal(err)
		}
		actual, err := ReadBoundedTimelineSources(db.WithContext(testCtx()), "conversation", []string{"message"}, nil)
		if err != nil || !reflect.DeepEqual(actual, previous) {
			t.Fatalf("leitura divergente (nulls=%v): %v\n%+v\n%+v", legacyNulls, err, actual, previous)
		}
	}
}
