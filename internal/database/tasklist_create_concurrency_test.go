package database

import (
	"fmt"
	"strings"
	"testing"
)

func TestCreateTaskListConcurrentWritersRespectLimit(t *testing.T) {
	ctx := setupConcurrentConfigDB(t)
	for i := 0; i < MaxTaskLists-1; i++ {
		if _, err := CreateTaskListWithContext(ctx, fmt.Sprintf("Seed %d", i), "", nil, fmt.Sprintf("seed-%d", i)); err != nil {
			t.Fatalf("seed tasklist %d: %v", i, err)
		}
	}

	const writers = 8
	errs := runConcurrently(writers, func(i int) error {
		_, err := CreateTaskListWithContext(ctx, fmt.Sprintf("Writer %d", i), "", nil, fmt.Sprintf("writer-%d", i))
		return err
	})
	winners := 0
	for i, err := range errs {
		switch {
		case err == nil:
			winners++
		case strings.Contains(err.Error(), "limite de tasklists atingido"):
		default:
			t.Fatalf("writer %d: esperado sucesso ou limite, veio %v", i, err)
		}
	}
	if winners != 1 {
		t.Fatalf("exatamente um writer deve ocupar a última vaga; venceram %d", winners)
	}

	var lists, workflows int64
	if err := ScopeByUser(ctx, db.Model(&TaskList{}), "user_id").Count(&lists).Error; err != nil {
		t.Fatalf("count tasklists: %v", err)
	}
	if err := db.Model(&TaskListWorkflow{}).Count(&workflows).Error; err != nil {
		t.Fatalf("count workflows: %v", err)
	}
	if lists != MaxTaskLists || workflows != lists {
		t.Fatalf("criações concorrentes devem respeitar limite e atomicidade: lists=%d workflows=%d", lists, workflows)
	}
}

func TestCreateTaskListConcurrentWritersRejectDuplicateSlug(t *testing.T) {
	ctx := setupConcurrentConfigDB(t)
	const writers = 8
	errs := runConcurrently(writers, func(i int) error {
		_, err := CreateTaskListWithContext(ctx, fmt.Sprintf("Writer %d", i), "", nil, "same-slug")
		return err
	})
	winners := 0
	for i, err := range errs {
		switch {
		case err == nil:
			winners++
		case strings.Contains(err.Error(), "já está em uso"):
		default:
			t.Fatalf("writer %d: esperado sucesso ou conflito de slug, veio %v", i, err)
		}
	}
	if winners != 1 {
		t.Fatalf("exatamente um writer deve criar o slug; venceram %d", winners)
	}

	var lists, workflows int64
	if err := ScopeByUser(ctx, db.Model(&TaskList{}), "user_id").Count(&lists).Error; err != nil {
		t.Fatalf("count tasklists: %v", err)
	}
	if err := db.Model(&TaskListWorkflow{}).Count(&workflows).Error; err != nil {
		t.Fatalf("count workflows: %v", err)
	}
	if lists != 1 || workflows != 1 {
		t.Fatalf("slug duplicado deixou estado parcial: lists=%d workflows=%d", lists, workflows)
	}
}
