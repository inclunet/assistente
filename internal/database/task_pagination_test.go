package database

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func setupTaskPaginationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	previous := DB()
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if err := testDB.AutoMigrate(&TaskList{}, &TaskListWorkflow{}, &Task{}); err != nil {
		t.Fatal(err)
	}
	SetDB(testDB)
	t.Cleanup(func() {
		_ = sqlDB.Close()
		SetDB(previous)
	})
	return testDB
}

func TestListTasksPageWithContext_KeysetStatusAndStableTieBreaker(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	ctx := WithUserID(context.Background(), "user-a")
	list := TaskList{UUIDModel: UUIDModel{ID: "list-a"}, UserID: "user-a", Title: "Fila"}
	if err := testDB.Create(&list).Error; err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	tasks := []Task{
		{UUIDModel: UUIDModel{ID: "task-a", CreatedAt: base}, TaskListID: list.ID, Title: "A", StatusID: 1},
		{UUIDModel: UUIDModel{ID: "task-b", CreatedAt: base}, TaskListID: list.ID, Title: "B", StatusID: 1},
		{UUIDModel: UUIDModel{ID: "task-c", CreatedAt: base.Add(time.Minute)}, TaskListID: list.ID, Title: "C", StatusID: 2},
		{UUIDModel: UUIDModel{ID: "task-d", CreatedAt: base.Add(2 * time.Minute)}, TaskListID: list.ID, Title: "D", StatusID: 1},
	}
	for i := range tasks {
		if err := testDB.Create(&tasks[i]).Error; err != nil {
			t.Fatal(err)
		}
	}

	statusID := 1
	first, err := ListTasksPageWithContext(ctx, TaskPageQuery{
		TaskListID: list.ID,
		StatusID:   &statusID,
		Limit:      2,
		Sort:       TaskSortCreatedAtAsc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || first.NextCursor == "" {
		t.Fatalf("expected first page metadata, got %+v", first)
	}
	if got := []string{first.Tasks[0].ID, first.Tasks[1].ID}; got[0] != "task-a" || got[1] != "task-b" {
		t.Fatalf("unexpected first page order: %v", got)
	}

	second, err := ListTasksPageWithContext(ctx, TaskPageQuery{
		TaskListID: list.ID,
		StatusID:   &statusID,
		Limit:      2,
		Sort:       TaskSortCreatedAtAsc,
		Cursor:     first.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.HasMore || second.NextCursor != "" || len(second.Tasks) != 1 || second.Tasks[0].ID != "task-d" {
		t.Fatalf("unexpected second page: %+v", second)
	}
}

func TestListTasksPageWithContext_DescendingAndCursorBinding(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	ctx := WithUserID(context.Background(), "user-a")
	list := TaskList{UUIDModel: UUIDModel{ID: "list-a"}, UserID: "user-a", Title: "Fila"}
	if err := testDB.Create(&list).Error; err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)
	for _, task := range []Task{
		{UUIDModel: UUIDModel{ID: "task-a", CreatedAt: base}, TaskListID: list.ID, Title: "A", StatusID: 1},
		{UUIDModel: UUIDModel{ID: "task-b", CreatedAt: base.Add(time.Minute)}, TaskListID: list.ID, Title: "B", StatusID: 1},
	} {
		if err := testDB.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}
	page, err := ListTasksPageWithContext(ctx, TaskPageQuery{
		TaskListID: list.ID,
		Limit:      1,
		Sort:       TaskSortCreatedAtDesc,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Tasks) != 1 || page.Tasks[0].ID != "task-b" {
		t.Fatalf("unexpected descending page: %+v", page)
	}
	_, err = ListTasksPageWithContext(ctx, TaskPageQuery{
		TaskListID: list.ID,
		Limit:      1,
		Sort:       TaskSortCreatedAtAsc,
		Cursor:     page.NextCursor,
	})
	if err == nil || !strings.Contains(err.Error(), "não corresponde") {
		t.Fatalf("expected cursor binding error, got %v", err)
	}
}

func TestListTasksPageWithContext_EnforcesUserScope(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	otherList := TaskList{UUIDModel: UUIDModel{ID: "list-b"}, UserID: "user-b", Title: "Privada"}
	if err := testDB.Create(&otherList).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&Task{
		UUIDModel:  UUIDModel{ID: "task-private"},
		TaskListID: otherList.ID,
		Title:      "Segredo",
		StatusID:   1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	_, err := ListTasksPageWithContext(WithUserID(context.Background(), "user-a"), TaskPageQuery{
		TaskListID: otherList.ID,
		Limit:      10,
	})
	if err == nil {
		t.Fatal("expected cross-user query to fail")
	}
}

func TestListTasksPageWithContext_RootOrderPaginationPreservesHierarchyAndCompleteness(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	ctx := WithUserID(context.Background(), "user-a")
	list := TaskList{UUIDModel: UUIDModel{ID: "list-a"}, UserID: "user-a", Title: "Fila"}
	if err := testDB.Create(&list).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&TaskListWorkflow{TaskListID: list.ID, Statuses: "[]", AllowedTransitions: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	for _, task := range []Task{
		{UUIDModel: UUIDModel{ID: "root-c"}, TaskListID: list.ID, Title: "C", StatusID: 1, Order: 2},
		{UUIDModel: UUIDModel{ID: "root-a"}, TaskListID: list.ID, Title: "A", StatusID: 1, Order: 0},
		{UUIDModel: UUIDModel{ID: "root-b"}, TaskListID: list.ID, Title: "B", StatusID: 1, Order: 1},
		{UUIDModel: UUIDModel{ID: "child-a"}, TaskListID: list.ID, Title: "A.1", StatusID: 1, ParentID: stringPointer("root-a"), Order: 0},
	} {
		if err := testDB.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}

	first, err := ListTasksPageWithContext(ctx, TaskPageQuery{
		TaskListID: list.ID,
		Limit:      2,
		Sort:       TaskSortOrderAsc,
		RootOnly:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if first.TotalCount != 3 || !first.HasMore || len(first.Tasks) != 2 {
		t.Fatalf("metadados incompletos na primeira página: %+v", first)
	}
	if first.Tasks[0].ID != "root-a" || first.Tasks[1].ID != "root-b" {
		t.Fatalf("ordem visual não preservada: %v, %v", first.Tasks[0].ID, first.Tasks[1].ID)
	}
	if len(first.Tasks[0].Subtasks) != 1 || first.Tasks[0].Subtasks[0].ID != "child-a" {
		t.Fatalf("subtasks da raiz não foram preservadas: %+v", first.Tasks[0].Subtasks)
	}

	second, err := ListTasksPageWithContext(ctx, TaskPageQuery{
		TaskListID: list.ID,
		Limit:      2,
		Sort:       TaskSortOrderAsc,
		RootOnly:   true,
		Cursor:     first.NextCursor,
	})
	if err != nil {
		t.Fatal(err)
	}
	if second.TotalCount != 3 || second.HasMore || len(second.Tasks) != 1 || second.Tasks[0].ID != "root-c" {
		t.Fatalf("segunda página não completou a lista: %+v", second)
	}
}

func TestListTasksPageWithContext_PreservesArbitraryHierarchyWithoutNPlusOne(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	ctx := WithUserID(context.Background(), "user-a")
	list := TaskList{UUIDModel: UUIDModel{ID: "list-a"}, UserID: "user-a", Title: "Fila"}
	if err := testDB.Create(&list).Error; err != nil {
		t.Fatal(err)
	}
	parent := func(id string) *string { return &id }
	for _, task := range []Task{
		{UUIDModel: UUIDModel{ID: "root"}, TaskListID: list.ID, Title: "Raiz", StatusID: 1, Order: 0},
		{UUIDModel: UUIDModel{ID: "child"}, TaskListID: list.ID, Title: "Filha", StatusID: 1, ParentID: parent("root"), Order: 0},
		{UUIDModel: UUIDModel{ID: "grandchild"}, TaskListID: list.ID, Title: "Neta", StatusID: 1, ParentID: parent("child"), Order: 0},
		{UUIDModel: UUIDModel{ID: "great-grandchild"}, TaskListID: list.ID, Title: "Bisneta", StatusID: 1, ParentID: parent("grandchild"), Order: 0},
	} {
		if err := testDB.Create(&task).Error; err != nil {
			t.Fatal(err)
		}
	}

	page, err := ListTasksPageWithContext(ctx, TaskPageQuery{
		TaskListID: list.ID,
		Limit:      1,
		Sort:       TaskSortOrderAsc,
		RootOnly:   true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := page.Tasks[0].Subtasks[0].Subtasks[0].Subtasks[0].ID; got != "great-grandchild" {
		t.Fatalf("hierarquia profunda incompleta: %s", got)
	}
}

func TestGetAllTaskListsWithContext_ProjectsCountsWithoutTasks(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	ctx := WithUserID(context.Background(), "user-a")
	for _, list := range []TaskList{
		{UUIDModel: UUIDModel{ID: "list-a"}, UserID: "user-a", Title: "A"},
		{UUIDModel: UUIDModel{ID: "list-b"}, UserID: "user-a", Title: "B"},
	} {
		if err := testDB.Create(&list).Error; err != nil {
			t.Fatal(err)
		}
	}
	tasks := make([]Task, 2507)
	for i := range tasks {
		listID := "list-a"
		if i >= 2500 {
			listID = "list-b"
		}
		tasks[i] = Task{TaskListID: listID, Title: "Task", StatusID: 1, Order: i}
	}
	if err := testDB.CreateInBatches(tasks, 250).Error; err != nil {
		t.Fatal(err)
	}
	parentID := tasks[0].ID
	if err := testDB.Create(&Task{
		TaskListID: "list-a", ParentID: &parentID, Title: "Subtask", StatusID: 1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	lists, err := GetAllTaskListsWithContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	counts := make(map[string]int64, len(lists))
	for _, list := range lists {
		if list.Tasks != nil {
			t.Fatalf("catálogo hidratou tasks da lista %s", list.ID)
		}
		counts[list.ID] = list.TaskCount
	}
	if counts["list-a"] != 2500 || counts["list-b"] != 7 {
		t.Fatalf("contagens agregadas incorretas: %v", counts)
	}
}

func TestConversationTaskListProjectionIsBoundedAndOrdered(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	ctx := WithUserID(context.Background(), "user-a")
	conversationID := "conversation-a"
	list := TaskList{
		UUIDModel:      UUIDModel{ID: "list-a"},
		UserID:         "user-a",
		Title:          "A",
		ConversationID: &conversationID,
	}
	if err := testDB.Create(&list).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 150; i++ {
		if err := testDB.Create(&Task{
			TaskListID: list.ID, Title: fmt.Sprintf("Task %d", i), StatusID: 1, Order: i,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}

	lists, err := GetTaskListsByConversationIDWithContext(ctx, conversationID)
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || lists[0].TaskCount != 150 || lists[0].Tasks != nil {
		t.Fatalf("projeção de listas inesperada: %+v", lists)
	}
	tasks, err := GetTaskListContextTasksWithContext(ctx, []string{list.ID}, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(tasks) != 25 || tasks[0].Order != 0 || tasks[24].Order != 24 {
		t.Fatalf("projeção de contexto inesperada: len=%d first=%+v last=%+v", len(tasks), tasks[0], tasks[len(tasks)-1])
	}
}

func TestGetTaskListMetadataWithContext_DoesNotHydrateTasks(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	ctx := WithUserID(context.Background(), "user-a")
	list := TaskList{UUIDModel: UUIDModel{ID: "list-a"}, UserID: "user-a", Title: "Fila"}
	if err := testDB.Create(&list).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&TaskListWorkflow{TaskListID: list.ID, Statuses: "[]", AllowedTransitions: "{}"}).Error; err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 150; i++ {
		if err := testDB.Create(&Task{TaskListID: list.ID, Title: "Task", StatusID: 1, Order: i}).Error; err != nil {
			t.Fatal(err)
		}
	}

	metadata, err := GetTaskListMetadataWithContext(ctx, list.ID)
	if err != nil {
		t.Fatal(err)
	}
	if metadata.Workflow == nil {
		t.Fatal("workflow ausente no read model de metadados")
	}
	if metadata.Tasks != nil {
		t.Fatalf("read model hidratou tasks indevidamente: %d", len(metadata.Tasks))
	}
}

func stringPointer(value string) *string {
	return &value
}

func TestListTasksPageWithContext_RetriesMetadataReadDuringTransientLock(t *testing.T) {
	path := t.TempDir() + "/task-page-retry.db"
	testDB, cleanup := openSQLitePolicyTestDB(t, "file:"+path+"?_pragma=busy_timeout(1)&_pragma=journal_mode(DELETE)")
	defer cleanup()
	if err := testDB.AutoMigrate(&TaskList{}, &Task{}); err != nil {
		t.Fatal(err)
	}
	previous := DB()
	SetDB(testDB)
	defer SetDB(previous)

	ctx := WithUserID(context.Background(), "user-a")
	list := TaskList{UUIDModel: UUIDModel{ID: "list-a"}, UserID: "user-a", Title: "Fila"}
	if err := testDB.Create(&list).Error; err != nil {
		t.Fatal(err)
	}
	if err := testDB.Create(&Task{
		UUIDModel:  UUIDModel{ID: "task-a"},
		TaskListID: list.ID,
		Title:      "A",
		StatusID:   1,
	}).Error; err != nil {
		t.Fatal(err)
	}

	sqlDB, err := testDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	lockConn, err := sqlDB.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lockConn.Close() }()
	if _, err := lockConn.ExecContext(ctx, "BEGIN EXCLUSIVE"); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = lockConn.ExecContext(ctx, "ROLLBACK") }()

	released := make(chan struct{})
	go func() {
		time.Sleep(90 * time.Millisecond)
		_, _ = lockConn.ExecContext(ctx, "COMMIT")
		close(released)
	}()

	started := time.Now()
	page, err := ListTasksPageWithContext(ctx, TaskPageQuery{TaskListID: list.ID, Limit: 10})
	if err != nil {
		t.Fatalf("paged read should recover from transient metadata lock: %v", err)
	}
	<-released
	if elapsed := time.Since(started); elapsed < 50*time.Millisecond {
		t.Fatalf("read completed too quickly (%s), metadata retry path was not exercised", elapsed)
	}
	if len(page.Tasks) != 1 || page.Tasks[0].ID != "task-a" {
		t.Fatalf("unexpected page after retry: %+v", page)
	}
}

func TestEnsureTaskPaginationIndexes(t *testing.T) {
	testDB := setupTaskPaginationTestDB(t)
	if err := ensureTaskPaginationIndexes(testDB); err != nil {
		t.Fatal(err)
	}
	if err := ensureTaskPaginationIndexes(testDB); err != nil {
		t.Fatalf("index creation must be idempotent: %v", err)
	}
	var indexes []struct {
		Name string
	}
	if err := testDB.Raw(`PRAGMA index_list('tasks')`).Scan(&indexes).Error; err != nil {
		t.Fatal(err)
	}
	got := make(map[string]bool, len(indexes))
	for _, index := range indexes {
		got[index.Name] = true
	}
	for _, name := range []string{"idx_tasks_list_created_id", "idx_tasks_list_status_created_id", "idx_tasks_list_parent_order"} {
		if !got[name] {
			t.Fatalf("expected pagination index %q, got %v", name, got)
		}
	}
}

func BenchmarkListTasksPageWithContext_2581Roots(b *testing.B) {
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	if err := testDB.AutoMigrate(&TaskList{}, &TaskListWorkflow{}, &Task{}); err != nil {
		b.Fatal(err)
	}
	if err := ensureTaskPaginationIndexes(testDB); err != nil {
		b.Fatal(err)
	}
	previous := DB()
	SetDB(testDB)
	defer SetDB(previous)

	list := TaskList{UUIDModel: UUIDModel{ID: "list-a"}, UserID: "user-a", Title: "Fila"}
	if err := testDB.Create(&list).Error; err != nil {
		b.Fatal(err)
	}
	if err := testDB.Create(&TaskListWorkflow{TaskListID: list.ID, Statuses: "[]", AllowedTransitions: "{}"}).Error; err != nil {
		b.Fatal(err)
	}
	tasks := make([]Task, 2581)
	for i := range tasks {
		tasks[i] = Task{TaskListID: list.ID, Title: "Task", StatusID: 1, Order: i}
	}
	if err := testDB.CreateInBatches(tasks, 250).Error; err != nil {
		b.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "user-a")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		page, err := ListTasksPageWithContext(ctx, TaskPageQuery{
			TaskListID: list.ID,
			Limit:      100,
			Sort:       TaskSortOrderAsc,
			RootOnly:   true,
		})
		if err != nil {
			b.Fatal(err)
		}
		if len(page.Tasks) != 100 || page.TotalCount != 2581 {
			b.Fatalf("página inesperada: tasks=%d total=%d", len(page.Tasks), page.TotalCount)
		}
	}
}

func BenchmarkTaskListCatalogWithContext_2581Roots(b *testing.B) {
	testDB, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		b.Fatal(err)
	}
	if err := testDB.AutoMigrate(&TaskList{}, &TaskListWorkflow{}, &Task{}); err != nil {
		b.Fatal(err)
	}
	if err := ensureTaskPaginationIndexes(testDB); err != nil {
		b.Fatal(err)
	}
	previous := DB()
	SetDB(testDB)
	defer SetDB(previous)

	for i := 0; i < 10; i++ {
		list := TaskList{UUIDModel: UUIDModel{ID: fmt.Sprintf("list-%02d", i)}, UserID: "user-a", Title: "Fila"}
		if err := testDB.Create(&list).Error; err != nil {
			b.Fatal(err)
		}
	}
	tasks := make([]Task, 2581)
	for i := range tasks {
		tasks[i] = Task{
			TaskListID: fmt.Sprintf("list-%02d", i%10),
			Title:      "Task",
			StatusID:   1,
			Order:      i,
		}
	}
	if err := testDB.CreateInBatches(tasks, 250).Error; err != nil {
		b.Fatal(err)
	}
	ctx := WithUserID(context.Background(), "user-a")

	b.Run("antes_preload_ilimitado", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			var lists []TaskList
			if err := ScopeByUser(ctx, testDB.WithContext(ctx), "user_id").
				Preload("Workflow").
				Preload("Tasks", func(query *gorm.DB) *gorm.DB {
					return query.Where("parent_id IS NULL").Order(`"order" ASC`)
				}).
				Find(&lists).Error; err != nil {
				b.Fatal(err)
			}
		}
	})
	b.Run("depois_catalogo_projetado", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			lists, err := GetAllTaskListsWithContext(ctx)
			if err != nil {
				b.Fatal(err)
			}
			if len(lists) != 10 || lists[0].Tasks != nil {
				b.Fatalf("catálogo inesperado: %d listas", len(lists))
			}
		}
	})
}
