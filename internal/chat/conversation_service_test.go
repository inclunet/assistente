package chat

import (
	"testing"
	"time"

	"assistente/internal/database"
)

func TestBuildMessageTree_OrdersByCreatedAt(t *testing.T) {
	now := time.Now()

	// Create messages with IDs that would sort differently by string vs by time.
	// msg2 has a lexicographically smaller ID but was created later.
	msg1 := Message{
		UUIDModel:      database.UUIDModel{ID: "01970a9e-ffff-7000-8000-000000000001", CreatedAt: now},
		ConversationID: "conv-1",
		Role:           "user",
		Content:        "first",
	}
	msg2 := Message{
		UUIDModel:      database.UUIDModel{ID: "01970a9e-0001-7000-8000-000000000002", CreatedAt: now.Add(time.Second)},
		ConversationID: "conv-1",
		Role:           "assistant",
		Content:        "second",
	}

	tree := BuildMessageTree([]Message{msg2, msg1})

	if len(tree) != 2 {
		t.Fatalf("expected 2 root nodes, got %d", len(tree))
	}
	if tree[0].Message.ID != msg1.ID {
		t.Errorf("first node should be msg1 (earlier CreatedAt), got ID %s", tree[0].Message.ID)
	}
	if tree[1].Message.ID != msg2.ID {
		t.Errorf("second node should be msg2 (later CreatedAt), got ID %s", tree[1].Message.ID)
	}
}

func TestBuildMessageTree_ChildrenOrderedByCreatedAt(t *testing.T) {
	now := time.Now()
	parentID := "01970a9e-0000-7000-8000-000000000001"

	parent := Message{
		UUIDModel:      database.UUIDModel{ID: parentID, CreatedAt: now},
		ConversationID: "conv-1",
		Role:           "user",
		Content:        "parent",
	}
	// child2 has lexicographically smaller ID but later timestamp
	child1 := Message{
		UUIDModel:      database.UUIDModel{ID: "01970a9e-ffff-7000-8000-000000000010", CreatedAt: now.Add(time.Second)},
		ConversationID: "conv-1",
		ParentID:       &parentID,
		Role:           "assistant",
		Content:        "child1",
	}
	child2 := Message{
		UUIDModel:      database.UUIDModel{ID: "01970a9e-0001-7000-8000-000000000020", CreatedAt: now.Add(2 * time.Second)},
		ConversationID: "conv-1",
		ParentID:       &parentID,
		Role:           "tool",
		Content:        "child2",
	}

	tree := BuildMessageTree([]Message{child2, parent, child1})

	if len(tree) != 1 {
		t.Fatalf("expected 1 root node, got %d", len(tree))
	}
	if tree[0].ChildCount != 2 {
		t.Fatalf("expected 2 children, got %d", tree[0].ChildCount)
	}
	if tree[0].Children[0].Message.ID != child1.ID {
		t.Errorf("first child should be child1 (earlier CreatedAt), got %s", tree[0].Children[0].Message.ID)
	}
	if tree[0].Children[1].Message.ID != child2.ID {
		t.Errorf("second child should be child2 (later CreatedAt), got %s", tree[0].Children[1].Message.ID)
	}
}
