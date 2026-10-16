package database

import (
	"context"
	"path/filepath"
	"testing"

	"gorm.io/gorm"
)

func TestDeleteTaskWithContextDeletesDescendantsAndNotesAtomicallyWithForeignKeys(t *testing.T) {
	previousDB, previousPath := db, dbPath
	var initializedDB *gorm.DB
	defer func() {
		if initializedDB != nil {
			if sqlDB, err := initializedDB.DB(); err == nil {
				_ = sqlDB.Close()
			}
		}
		db, dbPath = previousDB, previousPath
	}()

	path, err := filepath.Abs(filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := InitPath(path); err != nil {
		initializedDB = db
		t.Fatalf("InitPath: %v", err)
	}
	initializedDB = db

	for _, userID := range []string{"task-owner", "other-owner"} {
		if err := db.Create(&User{
			UUIDModel: UUIDModel{ID: userID}, Username: userID, PasswordHash: "test",
			Role: UserRoleUser, IsActive: true,
		}).Error; err != nil {
			t.Fatalf("criar usuário %s: %v", userID, err)
		}
	}
	ctx := WithUserID(context.Background(), "task-owner")
	otherCtx := WithUserID(context.Background(), "other-owner")
	list, err := CreateTaskListWithContext(ctx, "Lista", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	otherList, err := CreateTaskListWithContext(otherCtx, "Outra lista", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	root, err := CreateTaskWithContext(ctx, list.ID, "Raiz", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	child, err := CreateTaskWithContext(ctx, list.ID, "Filha", "", "", "", &root.ID)
	if err != nil {
		t.Fatal(err)
	}
	grandchild, err := CreateTaskWithContext(ctx, list.ID, "Neta", "", "", "", &child.ID)
	if err != nil {
		t.Fatal(err)
	}
	otherTask, err := CreateTaskWithContext(otherCtx, otherList.ID, "Outra", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, task := range []*Task{root, child, grandchild} {
		if _, err := CreateTaskNoteWithContext(ctx, task.ID, TaskNoteInternal, "nota", "", ""); err != nil {
			t.Fatalf("criar nota para %s: %v", task.ID, err)
		}
	}
	if _, err := CreateTaskNoteWithContext(otherCtx, otherTask.ID, TaskNoteInternal, "nota", "", ""); err != nil {
		t.Fatalf("criar nota para %s: %v", otherTask.ID, err)
	}

	if err := db.Exec("CREATE TRIGGER reject_task_delete BEFORE DELETE ON tasks BEGIN SELECT RAISE(ABORT, 'reject task delete'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteTaskWithContext(ctx, root.ID); err == nil {
		t.Fatal("falha no delete da raiz deveria abortar a operação")
	}
	if err := db.Exec("DROP TRIGGER reject_task_delete").Error; err != nil {
		t.Fatal(err)
	}
	var ownerTaskCount, ownerNoteCount int64
	if err := taskQuery(ctx, db.Model(&Task{})).Where("tasks.id IN ?", []string{root.ID, child.ID, grandchild.ID}).Count(&ownerTaskCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := taskNoteQuery(ctx, db.Model(&TaskNote{})).Where("task_notes.task_id IN ?", []string{root.ID, child.ID, grandchild.ID}).Count(&ownerNoteCount).Error; err != nil {
		t.Fatal(err)
	}
	if ownerTaskCount != 3 || ownerNoteCount != 3 {
		t.Fatalf("falha parcial removeu dados antes do rollback: tasks=%d notes=%d", ownerTaskCount, ownerNoteCount)
	}

	if err := DeleteTaskWithContext(ctx, root.ID); err != nil {
		t.Fatalf("excluir árvore completa: %v", err)
	}
	if err := taskQuery(ctx, db.Model(&Task{})).Where("tasks.id IN ?", []string{root.ID, child.ID, grandchild.ID}).Count(&ownerTaskCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := taskNoteQuery(ctx, db.Model(&TaskNote{})).Where("task_notes.task_id IN ?", []string{root.ID, child.ID, grandchild.ID}).Count(&ownerNoteCount).Error; err != nil {
		t.Fatal(err)
	}
	if ownerTaskCount != 0 || ownerNoteCount != 0 {
		t.Fatalf("árvore não foi removida: tasks=%d notes=%d", ownerTaskCount, ownerNoteCount)
	}
	if _, err := GetTaskWithContext(otherCtx, otherTask.ID); err != nil {
		t.Fatalf("exclusão cruzou o escopo do usuário: %v", err)
	}
	if notes, err := GetTaskNotesWithContext(otherCtx, otherTask.ID); err != nil || len(notes) != 1 {
		t.Fatalf("notas de outro usuário foram alteradas: len=%d err=%v", len(notes), err)
	}
}
