package database

import (
	"errors"
	"testing"
)

func TestMessageCommandPinRevisionDetectsABA(t *testing.T) {
	setupTestDB(t)
	cid := createTestConversation(t, "pin")
	mid := createTestMessage(t, cid, "user", "private")
	before, err := MessageCommandSnapshotWithContext(testCtx(), cid, mid, false)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if _, err := ToggleMessagePinWithContext(testCtx(), mid); err != nil {
			t.Fatal(err)
		}
	}
	after, err := MessageCommandSnapshotWithContext(testCtx(), cid, mid, false)
	if err != nil || before == after {
		t.Fatal("ABA revision unchanged", err)
	}
	err = WithConversationLifecycle(testCtx(), func() error {
		_, err := MutateMessageIfUnchangedWithinLifecycleWithContext(testCtx(), cid, mid, before, false)
		return err
	})
	if !errors.Is(err, ErrMessageContentChanged) {
		t.Fatal("stale pin accepted", err)
	}
}

func TestMessageCommandDeleteRollbackAndCrossConversation(t *testing.T) {
	for _, external := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "external-child"}[external], func(t *testing.T) {
			setupTestDB(t)
			cid := createTestConversation(t, "delete")
			mid := createTestMessage(t, cid, "user", "private")
			childCID := cid
			if external {
				childCID = createTestConversation(t, "other")
			}
			child := ChatMessage{ConversationID: childCID, ParentID: &mid, Role: "assistant", Content: "reply"}
			if err := db.Create(&child).Error; err != nil {
				t.Fatal(err)
			}
			revision, err := MessageCommandSnapshotWithContext(testCtx(), cid, mid, true)
			if external {
				if !errors.Is(err, ErrMessageContentChanged) {
					t.Fatal("cross-conversation cascade accepted", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := db.Exec("CREATE TRIGGER reject_root_delete BEFORE DELETE ON chat_messages WHEN OLD.role = 'user' BEGIN SELECT RAISE(ABORT, 'reject root'); END").Error; err != nil {
				t.Fatal(err)
			}
			err = WithConversationLifecycle(testCtx(), func() error {
				_, err := MutateMessageIfUnchangedWithinLifecycleWithContext(testCtx(), cid, mid, revision, true)
				return err
			})
			if err == nil {
				t.Fatal("expected rollback")
			}
			var count int64
			if err := db.Model(&ChatMessage{}).Where("id IN ?", []string{mid, child.ID}).Count(&count).Error; err != nil || count != 2 {
				t.Fatal("partial delete committed", count, err)
			}
		})
	}
}

func TestMessageEditCommandAtomicCompareAndRollback(t *testing.T) {
	for _, scenario := range []string{"original", "changed", "rollback", "success"} {
		t.Run(scenario, func(t *testing.T) {
			setupTestDB(t)
			cid := createTestConversation(t, "edit")
			mid := createTestMessage(t, cid, "user", "original")
			original := "original"
			if scenario == "original" {
				original = "stale"
			}
			revision, err := MessageEditCommandSnapshotWithContext(testCtx(), cid, mid, original)
			if scenario == "original" {
				if !errors.Is(err, ErrMessageContentChanged) {
					t.Fatal("stale original", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if scenario == "changed" {
				if err := UpdateMessageTextWithContext(testCtx(), mid, "concurrent"); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "rollback" {
				if err := db.Exec("CREATE TRIGGER reject_edit AFTER UPDATE OF content ON chat_messages BEGIN SELECT RAISE(ABORT, 'reject edit'); END").Error; err != nil {
					t.Fatal(err)
				}
			}
			err = WithConversationLifecycle(testCtx(), func() error {
				_, err := UpdateMessageIfUnchangedWithinLifecycleWithContext(testCtx(), cid, mid, revision, "edited")
				return err
			})
			if scenario == "success" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil {
				t.Fatal("unsafe update")
			}
			message, err := GetMessageWithContext(testCtx(), mid)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"changed": "concurrent", "rollback": "original", "success": "edited"}[scenario]
			if message.Content != want {
				t.Fatalf("got %q want %q", message.Content, want)
			}
		})
	}
}
