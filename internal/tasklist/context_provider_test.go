package tasklist

import (
	"context"
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
				StatusIcon: ">",
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
		"| > Doing | Fix login | task-1 |",
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
