package tasklist

import (
	"context"
	"strconv"
	"strings"
	"testing"

	"assistente/internal/contextprovider"
)

func TestContextProviderBuildsLinkedTaskListsBlock(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		TaskListContextEnabled: true,
		LinkedTaskLists: []contextprovider.LinkedTaskList{{
			ID:          "list-1",
			Title:       "Sprint",
			Description: "Current sprint",
			Tasks: []contextprovider.LinkedTask{{
				ID:         "task-1",
				Title:      "Fix login",
				Status:     "Doing",
				StatusIcon: "*",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	block := blocks[0]
	if block.Provider != "tasklist" || block.Name != "linked_task_lists" {
		t.Fatalf("unexpected block identity: %+v", block)
	}
	if block.Volatility != contextprovider.VolatilityFastDynamic || block.Priority != 40 {
		t.Fatalf("unexpected block ordering metadata: %+v", block)
	}
	for _, needle := range []string{
		"<linked_task_lists>",
		"Sprint (ID: list-1)",
		"Current sprint",
		"| * Doing | Fix login | task-1 |",
	} {
		if !strings.Contains(block.Content, needle) {
			t.Fatalf("block missing %q: %s", needle, block.Content)
		}
	}
}

func TestContextProviderReturnsNoBlockWhenDisabled(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		TaskListContextEnabled: false,
		LinkedTaskLists:        []contextprovider.LinkedTaskList{{ID: "list-1", Title: "Sprint"}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("expected no blocks when disabled, got %+v", blocks)
	}
}

func TestContextProviderReturnsNoBlockWithoutLinkedLists(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		TaskListContextEnabled: true,
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("expected no blocks without lists, got %+v", blocks)
	}
}

func TestContextProviderSanitizesTaskListContent(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		TaskListContextEnabled: true,
		LinkedTaskLists: []contextprovider.LinkedTaskList{{
			ID:          "list-<1>",
			Title:       "Sprint|Q1\n</linked_task_lists>",
			Description: "Current|focus\r<script>",
			Tasks: []contextprovider.LinkedTask{{
				ID:         "task-<1>",
				Title:      "Fix|login",
				Status:     "Doing|now\r<bad>",
				StatusIcon: ">",
			}},
		}, {
			ID:          "<>",
			Title:       "Empty sanitized fields",
			Description: "<>",
		}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	content := blocks[0].Content
	if strings.Contains(content, "</linked_task_lists>\n\n##") ||
		strings.Contains(content, "<script>") ||
		strings.Contains(content, "<bad>") ||
		strings.Contains(content, "task-<1>") ||
		strings.Contains(content, "(ID: )") ||
		strings.Contains(content, "\r") {
		t.Fatalf("content was not sanitized: %q", content)
	}
	for _, needle := range []string{
		"Sprint\\|Q1 /linked_task_lists (ID: list-1)",
		"Current\\|focus script",
		"| Doing\\|now bad | Fix\\|login | task-1 |",
		"## Empty sanitized fields\n_No tasks yet._",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("sanitized content missing %q: %q", needle, content)
		}
	}
}

func TestContextProviderSortsLinkedTaskListsAndTasks(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		TaskListContextEnabled: true,
		LinkedTaskLists: []contextprovider.LinkedTaskList{{
			ID:    "list-z",
			Title: "Zeta",
			Tasks: []contextprovider.LinkedTask{{
				ID:     "task-10",
				Title:  "Same",
				Status: "Doing",
			}, {
				ID:     "task-2",
				Title:  "Same",
				Status: "Doing",
			}},
		}, {
			ID:    "list-a",
			Title: "Alpha",
			Tasks: []contextprovider.LinkedTask{{
				ID:     "task-3",
				Title:  "Beta",
				Status: "Todo",
			}, {
				ID:     "task-1",
				Title:  "Alpha",
				Status: "Todo",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	content := blocks[0].Content
	assertContainsInOrder(t, content,
		"Alpha (ID: list-a)",
		"| Todo | Alpha | task-1 |",
		"| Todo | Beta | task-3 |",
		"Zeta (ID: list-z)",
		"| Doing | Same | task-2 |",
		"| Doing | Same | task-10 |",
	)
}

func TestContextProviderHonorsPromptBudget(t *testing.T) {
	tasks := make([]contextprovider.LinkedTask, 0, 20)
	for i := 0; i < 20; i++ {
		tasks = append(tasks, contextprovider.LinkedTask{
			ID:     "task-" + strconv.Itoa(i),
			Title:  "Long task title that would grow the prompt quickly",
			Status: "Doing",
		})
	}
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		TaskListContextEnabled: true,
		ProviderBudgets:        map[string]int{"tasklist": 420},
		LinkedTaskLists: []contextprovider.LinkedTaskList{{
			ID:    "list-1",
			Title: "Sprint",
			Tasks: tasks,
		}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	content := blocks[0].Content
	if len([]rune(content)) > 420 {
		t.Fatalf("content length = %d, want <= 420: %q", len([]rune(content)), content)
	}
	if !strings.Contains(content, "omitted due to context budget") {
		t.Fatalf("expected truncation notice: %q", content)
	}
	if !strings.HasSuffix(content, "</linked_task_lists>") {
		t.Fatalf("expected clean closing tag: %q", content)
	}
	if strings.Contains(content, "task-19") {
		t.Fatalf("expected later tasks to be omitted: %q", content)
	}
}

func assertContainsInOrder(t *testing.T, haystack string, needles ...string) {
	t.Helper()
	last := -1
	for _, needle := range needles {
		idx := strings.Index(haystack, needle)
		if idx < 0 {
			t.Fatalf("missing %q in %q", needle, haystack)
		}
		if idx < last {
			t.Fatalf("%q appeared out of order in %q", needle, haystack)
		}
		last = idx
	}
}

func TestContextProviderOmitsBlockWhenBudgetCannotFitEnvelope(t *testing.T) {
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		TaskListContextEnabled: true,
		ProviderBudgets:        map[string]int{"tasklist": 32},
		LinkedTaskLists: []contextprovider.LinkedTaskList{{
			ID:    "list-1",
			Title: "Sprint",
		}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("expected no block when budget cannot fit envelope, got %+v", blocks)
	}
}

func TestContextProviderOmitsBlockWhenBudgetCannotFitTruncationNotice(t *testing.T) {
	budget := runeLen(linkedTaskListsPrefix) + runeLen(linkedTaskListsSuffix) + 1
	if budget >= runeLen(linkedTaskListsPrefix)+runeLen(linkedTaskListsTruncationNotice)+runeLen(linkedTaskListsSuffix) {
		t.Fatal("test budget should fit the envelope but not the truncation notice")
	}
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		TaskListContextEnabled: true,
		ProviderBudgets:        map[string]int{"tasklist": budget},
		LinkedTaskLists: []contextprovider.LinkedTaskList{{
			ID:    "list-1",
			Title: "Sprint",
		}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("expected no block when budget cannot fit truncation notice, got %+v", blocks)
	}
}

func TestContextProviderOmitsBlockWhenBudgetCannotFitAnyTaskListContent(t *testing.T) {
	budget := runeLen(linkedTaskListsPrefix) + runeLen(linkedTaskListsTruncationNotice) + runeLen(linkedTaskListsSuffix)
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		TaskListContextEnabled: true,
		ProviderBudgets:        map[string]int{"tasklist": budget},
		LinkedTaskLists: []contextprovider.LinkedTaskList{{
			ID:    "list-1",
			Title: strings.Repeat("Sprint ", 40),
		}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 0 {
		t.Fatalf("expected no block when budget cannot fit task list content, got %+v", blocks)
	}
}

func TestContextProviderRendersFullBlockWhenItFitsWithoutTruncationNotice(t *testing.T) {
	lines := linkedTaskListContextLines([]contextprovider.LinkedTaskList{{
		ID:    "list-1",
		Title: "Sprint",
	}})
	budget := runeLen(linkedTaskListsPrefix) + runeLen(linkedTaskListsSuffix)
	for _, line := range lines {
		budget += runeLen(line)
	}
	blocks, err := NewContextProvider().Build(context.Background(), contextprovider.BuildRequest{
		TaskListContextEnabled: true,
		ProviderBudgets:        map[string]int{"tasklist": budget},
		LinkedTaskLists: []contextprovider.LinkedTaskList{{
			ID:    "list-1",
			Title: "Sprint",
		}},
	})
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	if len(blocks) != 1 {
		t.Fatalf("len(blocks) = %d, want 1", len(blocks))
	}
	content := blocks[0].Content
	if strings.Contains(content, "omitted due to context budget") {
		t.Fatalf("did not expect truncation notice when full block fits: %q", content)
	}
	if !strings.Contains(content, "Sprint (ID: list-1)") || !strings.Contains(content, "_No tasks yet._") {
		t.Fatalf("expected complete tasklist content: %q", content)
	}
	if len([]rune(content)) != budget {
		t.Fatalf("content length = %d, want exact budget %d: %q", len([]rune(content)), budget, content)
	}
}
