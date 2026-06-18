package tasklist

import (
	"context"
	"strconv"
	"strings"

	"assistente/internal/contextprovider"
)

const defaultPromptBudget = 4000
const linkedTaskListsPrefix = "<linked_task_lists>\nThis conversation has linked task lists. Use this context to track progress, update tasks, and help the user manage their work.\n"
const linkedTaskListsSuffix = "\n</linked_task_lists>"
const linkedTaskListsTruncationNotice = "\n... Additional linked task list content omitted due to context budget."

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
	content := buildLinkedTaskListsBlock(req.LinkedTaskLists, req.Budget(p.Name(), defaultPromptBudget))
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

func buildLinkedTaskListsBlock(lists []contextprovider.LinkedTaskList, budgetChars int) string {
	if budgetChars <= 0 {
		budgetChars = defaultPromptBudget
	}
	if runeLen(linkedTaskListsPrefix)+runeLen(linkedTaskListsTruncationNotice)+runeLen(linkedTaskListsSuffix) > budgetChars {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(linkedTaskListsPrefix)
	for _, list := range lists {
		listID := sanitizeContextLine(list.ID)
		description := sanitizeContextLine(list.Description)
		var heading strings.Builder
		heading.WriteString("\n## ")
		heading.WriteString(sanitizeContextLine(list.Title))
		if listID != "" {
			heading.WriteString(" (ID: ")
			heading.WriteString(listID)
			heading.WriteString(")")
		}
		heading.WriteString("\n")
		if !writeTaskListContextLine(&sb, budgetChars, heading.String(), true) {
			return closeLinkedTaskListsBlock(&sb, budgetChars, true)
		}
		if description != "" {
			if !writeTaskListContextLine(&sb, budgetChars, description+"\n", true) {
				return closeLinkedTaskListsBlock(&sb, budgetChars, true)
			}
		}
		if len(list.Tasks) == 0 {
			if !writeTaskListContextLine(&sb, budgetChars, "_No tasks yet._\n", true) {
				return closeLinkedTaskListsBlock(&sb, budgetChars, true)
			}
			continue
		}
		if !writeTaskListContextLine(&sb, budgetChars, "\n| # | Status | Task | ID |\n|---|--------|------|----|\n", true) {
			return closeLinkedTaskListsBlock(&sb, budgetChars, true)
		}
		for idx, task := range list.Tasks {
			var line strings.Builder
			line.WriteString("| ")
			line.WriteString(strconv.Itoa(idx))
			line.WriteString(" | ")
			if icon := sanitizeContextLine(task.StatusIcon); icon != "" {
				line.WriteString(icon)
				line.WriteString(" ")
			}
			line.WriteString(sanitizeContextLine(task.Status))
			line.WriteString(" | ")
			line.WriteString(sanitizeContextLine(task.Title))
			line.WriteString(" | ")
			line.WriteString(sanitizeContextLine(task.ID))
			line.WriteString(" |\n")
			if !writeTaskListContextLine(&sb, budgetChars, line.String(), true) {
				return closeLinkedTaskListsBlock(&sb, budgetChars, true)
			}
		}
	}
	return closeLinkedTaskListsBlock(&sb, budgetChars, false)
}

func sanitizeContextLine(value string) string {
	return taskListContextLineReplacer.Replace(strings.TrimSpace(value))
}

func writeTaskListContextLine(sb *strings.Builder, budgetChars int, line string, reserveTruncation bool) bool {
	reserved := linkedTaskListsSuffix
	if reserveTruncation {
		reserved = linkedTaskListsTruncationNotice + linkedTaskListsSuffix
	}
	if runeLen(sb.String())+runeLen(line)+runeLen(reserved) > budgetChars {
		return false
	}
	sb.WriteString(line)
	return true
}

func closeLinkedTaskListsBlock(sb *strings.Builder, budgetChars int, truncated bool) string {
	if truncated {
		_ = writeTaskListContextLine(sb, budgetChars, linkedTaskListsTruncationNotice, false)
	}
	sb.WriteString(linkedTaskListsSuffix)
	return sb.String()
}

func runeLen(value string) int {
	count := 0
	for range value {
		count++
	}
	return count
}
