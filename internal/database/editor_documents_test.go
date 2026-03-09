package database

import (
	"testing"
	"time"
)

func TestEditorDocuments_CRUD(t *testing.T) {
	setupTestDB(t)
	if err := db.AutoMigrate(&EditorDocument{}); err != nil {
		t.Fatalf("failed to migrate EditorDocument: %v", err)
	}

	if err := UpsertEditorDocument(EditorDocument{ID: "draft-1", Title: "Doc", Mode: "markdown", Markdown: "# oi"}); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	md, found, err := GetEditorDocumentMarkdown("draft-1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !found {
		t.Fatalf("expected found=true")
	}
	if md != "# oi" {
		t.Fatalf("unexpected markdown: %q", md)
	}

	if err := UpsertEditorDocument(EditorDocument{ID: "draft-1", Markdown: "# tchau"}); err != nil {
		t.Fatalf("upsert update failed: %v", err)
	}

	md, found, err = GetEditorDocumentMarkdown("draft-1")
	if err != nil {
		t.Fatalf("get after update failed: %v", err)
	}
	if !found || md != "# tchau" {
		t.Fatalf("expected updated markdown, got found=%v md=%q", found, md)
	}

	if err := DeleteEditorDocument("draft-1"); err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	_, found, err = GetEditorDocumentMarkdown("draft-1")
	if err != nil {
		t.Fatalf("get after delete failed: %v", err)
	}
	if found {
		t.Fatalf("expected found=false")
	}
}

func TestEditorDocuments_CleanupOrphans(t *testing.T) {
	setupTestDB(t)
	if err := db.AutoMigrate(&EditorDocument{}); err != nil {
		t.Fatalf("failed to migrate EditorDocument: %v", err)
	}

	if err := UpsertEditorDocument(EditorDocument{ID: "keep", Markdown: "k"}); err != nil {
		t.Fatalf("upsert keep failed: %v", err)
	}
	if err := UpsertEditorDocument(EditorDocument{ID: "old-orphan", Markdown: "o"}); err != nil {
		t.Fatalf("upsert old-orphan failed: %v", err)
	}
	if err := UpsertEditorDocument(EditorDocument{ID: "new-orphan", Markdown: "n"}); err != nil {
		t.Fatalf("upsert new-orphan failed: %v", err)
	}

	oldTime := time.Now().Add(-48 * time.Hour)
	if err := db.Model(&EditorDocument{}).Where("id = ?", "old-orphan").Update("updated_at", oldTime).Error; err != nil {
		t.Fatalf("update old-orphan updated_at failed: %v", err)
	}

	cutoff := time.Now().Add(-24 * time.Hour)
	deleted, err := CleanupOrphanEditorDocuments(CleanupOrphanEditorDocumentsArgs{
		KeepIDs:       []string{"keep"},
		UpdatedBefore: cutoff,
	})
	if err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected deleted=1, got %d", deleted)
	}

	_, found, err := GetEditorDocumentMarkdown("keep")
	if err != nil || !found {
		t.Fatalf("expected keep to remain, found=%v err=%v", found, err)
	}

	_, found, err = GetEditorDocumentMarkdown("old-orphan")
	if err != nil {
		t.Fatalf("get old-orphan failed: %v", err)
	}
	if found {
		t.Fatalf("expected old-orphan deleted")
	}

	_, found, err = GetEditorDocumentMarkdown("new-orphan")
	if err != nil || !found {
		t.Fatalf("expected new-orphan to remain (grace period), found=%v err=%v", found, err)
	}
}
