package tasklist

import (
	"context"
	"strconv"
	"strings"

	"assistente/internal/contextprovider"
)

type ContextProvider struct{}

func NewContextProvider() *ContextProvider {
	return &ContextProvider{}
}

func (p *ContextProvider) Name() string { return "tasklist" }

func (p *ContextProvider) Build(_ context.Context, req contextprovider.BuildRequest) ([]contextprovider.Block, error) {
	if !req.TaskListContextEnabled || len(req.LinkedTaskLists) == 0 {
		return nil, nil
	}
	content := buildLinkedTaskListsBlock(req.LinkedTaskLists)
	if strings.TrimSpace(content) == "" {
		return nil, nil
	}
	return []contextprovider.Block{{
		Provider:   p.Name(),
		Name:       "linked_task_lists",
		Volatility: contextprovider.VolatilityFastDynamic,
		Priority:   40,
		Content:    content,
	}}, nil
}

func buildLinkedTaskListsBlock(lists []contextprovider.LinkedTaskList) string {
	var sb strings.Builder
	sb.WriteString("<linked_task_lists>\n")
	sb.WriteString("This conversation has linked task lists. Use this context to track progress, update tasks, and help the user manage their work.\n")
	for _, list := range lists {
		sb.WriteString("\n## ")
		sb.WriteString(list.Title)
		if list.ID != "" {
			sb.WriteString(" (ID: ")
			sb.WriteString(list.ID)
			sb.WriteString(")")
		}
		sb.WriteString("\n")
		if strings.TrimSpace(list.Description) != "" {
			sb.WriteString(strings.TrimSpace(list.Description))
			sb.WriteString("\n")
		}
		if len(list.Tasks) == 0 {
			sb.WriteString("_No tasks yet._\n")
			continue
		}
		sb.WriteString("\n| # | Status | Task | ID |\n|---|--------|------|----|\n")
		for idx, task := range list.Tasks {
			sb.WriteString("| ")
			sb.WriteString(strconv.Itoa(idx))
			sb.WriteString(" | ")
			if task.StatusIcon != "" {
				sb.WriteString(task.StatusIcon)
				sb.WriteString(" ")
			}
			sb.WriteString(task.Status)
			sb.WriteString(" | ")
			sb.WriteString(task.Title)
			sb.WriteString(" | ")
			sb.WriteString(task.ID)
			sb.WriteString(" |\n")
		}
	}
	sb.WriteString("</linked_task_lists>")
	return sb.String()
}
