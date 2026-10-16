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

func TestTaskListOperationsPreserveHierarchyAndRollbackWithForeignKeys(t *testing.T) {
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

	path, err := filepath.Abs(filepath.Join(t.TempDir(), "tasklists.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := InitPath(path); err != nil {
		initializedDB = db
		t.Fatalf("InitPath: %v", err)
	}
	initializedDB = db
	if err := db.Create(&User{
		UUIDModel: UUIDModel{ID: "tasklist-owner"}, Username: "tasklist-owner", PasswordHash: "test",
		Role: UserRoleUser, IsActive: true,
	}).Error; err != nil {
		t.Fatalf("criar usuário: %v", err)
	}
	ctx := WithUserID(context.Background(), "tasklist-owner")
	listA, err := CreateTaskListWithContext(ctx, "A", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	listB, err := CreateTaskListWithContext(ctx, "B", "", nil, "")
	if err != nil {
		t.Fatal(err)
	}
	createTree := func(listID string) (*Task, *Task, *Task) {
		t.Helper()
		root, err := CreateTaskWithContext(ctx, listID, "Raiz", "", "", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		child, err := CreateTaskWithContext(ctx, listID, "Filha", "", "", "", &root.ID)
		if err != nil {
			t.Fatal(err)
		}
		grandchild, err := CreateTaskWithContext(ctx, listID, "Neta", "", "", "", &child.ID)
		if err != nil {
			t.Fatal(err)
		}
		for _, task := range []*Task{root, child, grandchild} {
			if _, err := CreateTaskNoteWithContext(ctx, task.ID, TaskNoteInternal, "nota", "", ""); err != nil {
				t.Fatalf("criar nota de %s: %v", task.ID, err)
			}
		}
		return root, child, grandchild
	}
	root, child, grandchild := createTree(listA.ID)
	if err := db.Model(&Task{}).Where("id = ?", grandchild.ID).Update("task_list_id", listB.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteTaskWithContext(ctx, root.ID); err == nil {
		t.Fatal("DeleteTask deveria recusar descendente de outra lista")
	}
	var tasksCount, notesCount int64
	if err := taskQuery(ctx, db.Model(&Task{})).Where("tasks.id IN ?", []string{root.ID, child.ID, grandchild.ID}).Count(&tasksCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := taskNoteQuery(ctx, db.Model(&TaskNote{})).Where("task_notes.task_id IN ?", []string{root.ID, child.ID, grandchild.ID}).Count(&notesCount).Error; err != nil {
		t.Fatal(err)
	}
	if tasksCount != 3 || notesCount != 3 {
		t.Fatalf("DeleteTask removeu dados de árvore entre listas: tasks=%d notes=%d", tasksCount, notesCount)
	}
	if err := db.Model(&Task{}).Where("id = ?", grandchild.ID).Update("task_list_id", listA.ID).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := MoveTaskToListWithContext(ctx, root.ID, listB.ID); err != nil {
		t.Fatalf("mover árvore com neta: %v", err)
	}
	for _, expected := range []*Task{root, child, grandchild} {
		var moved Task
		if err := db.First(&moved, "id = ?", expected.ID).Error; err != nil {
			t.Fatal(err)
		}
		if moved.TaskListID != listB.ID {
			t.Errorf("task %s ficou na lista %s, esperado %s", moved.ID, moved.TaskListID, listB.ID)
		}
	}
	var movedRoot, movedChild, movedGrandchild Task
	if err := db.First(&movedRoot, "id = ?", root.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&movedChild, "id = ?", child.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.First(&movedGrandchild, "id = ?", grandchild.ID).Error; err != nil {
		t.Fatal(err)
	}
	if movedRoot.ParentID != nil || movedChild.ParentID == nil || *movedChild.ParentID != movedRoot.ID || movedGrandchild.ParentID == nil || *movedGrandchild.ParentID != movedChild.ID {
		t.Fatal("a árvore movida perdeu o vínculo entre raiz, filha e neta")
	}
	disconnected, err := CreateTaskWithContext(ctx, listB.ID, "Desconectada", "", "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := CreateTaskNoteWithContext(ctx, disconnected.ID, TaskNoteInternal, "nota desconectada", "", ""); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&Task{}).Where("id = ?", disconnected.ID).Update("parent_id", disconnected.ID).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.Model(&Task{}).Where("id = ?", grandchild.ID).Update("task_list_id", listA.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := ClearTaskListWithContext(ctx, listB.ID); err == nil {
		t.Fatal("ClearTaskList deveria recusar árvore cujo conjunto alcançado diverge da lista")
	}
	if err := taskNoteQuery(ctx, db.Model(&TaskNote{})).Where("task_notes.task_id IN ?", []string{root.ID, child.ID, grandchild.ID, disconnected.ID}).Count(&notesCount).Error; err != nil {
		t.Fatal(err)
	}
	if notesCount != 4 {
		t.Fatalf("ClearTaskList inconsistente apagou notas antes da validação: notes=%d", notesCount)
	}
	if err := db.Model(&Task{}).Where("id = ?", grandchild.ID).Update("task_list_id", listB.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&Task{}).Where("id = ?", disconnected.ID).Update("parent_id", nil).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TRIGGER reject_tasklist_delete BEFORE DELETE ON tasks BEGIN SELECT RAISE(ABORT, 'reject tasklist delete'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := ClearTaskListWithContext(ctx, listB.ID); err == nil {
		t.Fatal("ClearTaskList deveria falhar com trigger de regressão")
	}
	if err := taskQuery(ctx, db.Model(&Task{})).Where("tasks.task_list_id = ?", listB.ID).Count(&tasksCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := taskNoteQuery(ctx, db.Model(&TaskNote{})).Where("task_notes.task_id IN ?", []string{root.ID, child.ID, grandchild.ID, disconnected.ID}).Count(&notesCount).Error; err != nil {
		t.Fatal(err)
	}
	if tasksCount != 4 || notesCount != 4 {
		t.Fatalf("ClearTaskList deixou exclusão parcial: tasks=%d notes=%d", tasksCount, notesCount)
	}
	if err := db.Exec("DROP TRIGGER reject_tasklist_delete").Error; err != nil {
		t.Fatal(err)
	}
	if err := ClearTaskListWithContext(ctx, listB.ID); err != nil {
		t.Fatalf("limpar lista: %v", err)
	}
	if err := taskQuery(ctx, db.Model(&Task{})).Where("tasks.task_list_id = ?", listB.ID).Count(&tasksCount).Error; err != nil || tasksCount != 0 {
		t.Fatalf("ClearTaskList preservou tarefas: count=%d err=%v", tasksCount, err)
	}

	root, child, grandchild = createTree(listB.ID)
	if err := db.Exec("CREATE TRIGGER reject_tasklist_delete BEFORE DELETE ON tasks BEGIN SELECT RAISE(ABORT, 'reject tasklist delete'); END").Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteTaskListWithContext(ctx, listB.ID); err == nil {
		t.Fatal("DeleteTaskList deveria falhar com trigger de regressão")
	}
	if err := taskQuery(ctx, db.Model(&Task{})).Where("tasks.task_list_id = ?", listB.ID).Count(&tasksCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := taskNoteQuery(ctx, db.Model(&TaskNote{})).Where("task_notes.task_id IN ?", []string{root.ID, child.ID, grandchild.ID}).Count(&notesCount).Error; err != nil {
		t.Fatal(err)
	}
	var listCount int64
	if err := ScopeByUser(ctx, db.Model(&TaskList{}), "user_id").Where("id = ?", listB.ID).Count(&listCount).Error; err != nil {
		t.Fatal(err)
	}
	if tasksCount != 3 || notesCount != 3 || listCount != 1 {
		t.Fatalf("DeleteTaskList deixou exclusão parcial: tasks=%d notes=%d lists=%d", tasksCount, notesCount, listCount)
	}
	if err := db.Exec("DROP TRIGGER reject_tasklist_delete").Error; err != nil {
		t.Fatal(err)
	}
	if err := DeleteTaskListWithContext(ctx, listB.ID); err != nil {
		t.Fatalf("excluir lista: %v", err)
	}
	if err := ScopeByUser(ctx, db.Model(&TaskList{}), "user_id").Where("id = ?", listB.ID).Count(&listCount).Error; err != nil || listCount != 0 {
		t.Fatalf("lista não foi excluída: count=%d err=%v", listCount, err)
	}
}
