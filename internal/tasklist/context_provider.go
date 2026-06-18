package tasklist

import (
	"context"
	"strconv"
	"strings"

	"assistente/internal/contextprovider"
)

type ContextProvider struct{}

var taskListContextLineReplacer = strings.NewReplacer("\n", " ", "\r", " ", "<", "", ">", "", "|", "\\|")

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
		listID := sanitizeContextLine(list.ID)
		description := sanitizeContextLine(list.Description)
		sb.WriteString("\n## ")
		sb.WriteString(sanitizeContextLine(list.Title))
		if listID != "" {
			sb.WriteString(" (ID: ")
			sb.WriteString(listID)
			sb.WriteString(")")
		}
		sb.WriteString("\n")
		if description != "" {
			sb.WriteString(description)
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
			if icon := sanitizeContextLine(task.StatusIcon); icon != "" {
				sb.WriteString(icon)
				sb.WriteString(" ")
			}
			sb.WriteString(sanitizeContextLine(task.Status))
			sb.WriteString(" | ")
			sb.WriteString(sanitizeContextLine(task.Title))
			sb.WriteString(" | ")
			sb.WriteString(sanitizeContextLine(task.ID))
			sb.WriteString(" |\n")
		}
	}
	sb.WriteString("</linked_task_lists>")
	return sb.String()
}

func sanitizeContextLine(value string) string {
	return taskListContextLineReplacer.Replace(strings.TrimSpace(value))
}
