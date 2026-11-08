package database

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func TestBoundedTimelineSourcesAuthorizeEvenEmptySelection(t *testing.T) {
	setupOrderingTestDB(t)
	conversationID, _ := createConvWithOrderedMessages(t)
	if messages, err := ReadBoundedTimelineSources(db.WithContext(testCtx()), conversationID, nil, nil); err != nil || len(messages) != 0 {
		t.Fatal(messages, err)
	}
	if _, err := ReadBoundedTimelineSources(db.WithContext(context.Background()), conversationID, nil, nil); !errors.Is(err, ErrUserScopeRequired) {
		t.Fatal(err)
	}
	if _, err := ReadBoundedTimelineSources(db.WithContext(WithUserID(context.Background(), "other")), conversationID, nil, nil); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatal(err)
	}
}

func TestBoundedTimelineSourceSQLDoesNotTransferExcessContent(t *testing.T) {
	setupOrderingTestDB(t)
	conversationID, ids := createConvWithOrderedMessages(t)
	if err := db.Model(&ChatMessage{}).Where("id=?", ids[0]).Update("content", strings.Repeat("x", MaxTimelineSourceBytes+1)).Error; err != nil {
		t.Fatal(err)
	}
	tx := db.WithContext(testCtx())
	args, err := timelineSourceReadArgs(tx, conversationID, ids[:1], nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Raw(timelineBoundedSourceSQL, append(args, MaxTimelineSourceBytes)...).Rows()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	columns, err := rows.Columns()
	if err != nil {
		t.Fatal(err)
	}
	values := make([]any, len(columns))
	dest := make([]any, len(columns))
	for i := range values {
		dest[i] = &values[i]
	}
	if !rows.Next() {
		t.Fatal("orçamento rejeitado perdeu a sentinela")
	}
	if err := rows.Scan(dest...); err != nil {
		t.Fatal(err)
	}
	for i, value := range values {
		switch v := value.(type) {
		case string:
			if v != "" {
				t.Fatalf("coluna %s transferiu %d bytes", columns[i], len(v))
			}
		case []byte:
			if len(v) != 0 {
				t.Fatalf("coluna %s transferiu %d bytes", columns[i], len(v))
			}
		}
	}
	if rows.Next() || rows.Err() != nil {
		t.Fatal("sentinela inesperada", rows.Err())
	}
	if messages, err := ReadBoundedTimelineSources(tx, conversationID, ids[:1], nil); messages != nil || !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal(len(messages), err)
	}
}

func TestBoundedTimelineSourcesCountPersistedTimestampBytes(t *testing.T) {
	setupOrderingTestDB(t)
	conversationID, ids := createConvWithOrderedMessages(t)
	if err := db.Exec("UPDATE chat_messages SET updated_at=? WHERE id=?", strings.Repeat("d", MaxTimelineSourceBytes+1), ids[0]).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := ReadBoundedTimelineSources(db.WithContext(testCtx()), conversationID, ids[:1], nil); !errors.Is(err, ErrTimelineSourceLimit) {
		t.Fatal("data excessiva alcançou o scanner", err)
	}
}
