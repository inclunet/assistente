package tasklist

import (
	"context"
	"sort"
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

func (p *ContextProvider) Metadata() contextprovider.ProviderMetadata {
	return contextprovider.ProviderMetadata{
		Name:             p.Name(),
		DisplayName:      "Task Lists",
		Description:      "Task lists linked to the current conversation when tasklist support is active.",
		DefaultEnabled:   true,
		DefaultBudget:    defaultPromptBudget,
		SupportsSettings: false,
	}
}

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
	if runeLen(linkedTaskListsPrefix)+runeLen(linkedTaskListsSuffix) > budgetChars {
		return ""
	}
	lines := linkedTaskListContextLines(lists)
	if len(lines) == 0 {
		return ""
	}
	var sb strings.Builder
	sb.WriteString(linkedTaskListsPrefix)
	currentRunes := runeLen(linkedTaskListsPrefix)
	hasContent := false
	fullContentRunes := currentRunes + runeLen(linkedTaskListsSuffix)
	for _, line := range lines {
		fullContentRunes += runeLen(line)
	}
	if fullContentRunes <= budgetChars {
		for _, line := range lines {
			appendTaskListContextLine(&sb, &currentRunes, line)
			hasContent = true
		}
		return closeLinkedTaskListsBlock(&sb, &currentRunes, budgetChars, false, hasContent)
	}
	if runeLen(linkedTaskListsPrefix)+runeLen(linkedTaskListsTruncationNotice)+runeLen(linkedTaskListsSuffix) > budgetChars {
		return ""
	}
	for _, line := range lines {
		if !writeTaskListContextLine(&sb, &currentRunes, budgetChars, line, true) {
			return closeLinkedTaskListsBlock(&sb, &currentRunes, budgetChars, true, hasContent)
		}
		hasContent = true
	}
	return closeLinkedTaskListsBlock(&sb, &currentRunes, budgetChars, false, hasContent)
}

func linkedTaskListContextLines(lists []contextprovider.LinkedTaskList) []string {
	lists = sortedLinkedTaskLists(lists)
	lines := make([]string, 0)
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
		lines = append(lines, heading.String())
		if description != "" {
			lines = append(lines, description+"\n")
		}
		if len(list.Tasks) == 0 {
			lines = append(lines, "_No tasks yet._\n")
			continue
		}
		lines = append(lines, "\n| # | Status | Task | ID |\n|---|--------|------|----|\n")
		for idx, task := range sortedLinkedTasks(list.Tasks) {
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
			lines = append(lines, line.String())
		}
	}
	return lines
}

func sortedLinkedTaskLists(lists []contextprovider.LinkedTaskList) []contextprovider.LinkedTaskList {
	out := append([]contextprovider.LinkedTaskList(nil), lists...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func sortedLinkedTasks(tasks []contextprovider.LinkedTask) []contextprovider.LinkedTask {
	out := append([]contextprovider.LinkedTask(nil), tasks...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Status != out[j].Status {
			return out[i].Status < out[j].Status
		}
		if out[i].Title != out[j].Title {
			return out[i].Title < out[j].Title
		}
		return naturalLess(out[i].ID, out[j].ID)
	})
	return out
}

func naturalLess(left, right string) bool {
	leftPrefix, leftNumber, leftOK := splitTrailingNumber(left)
	rightPrefix, rightNumber, rightOK := splitTrailingNumber(right)
	if leftOK && rightOK && leftPrefix == rightPrefix && leftNumber != rightNumber {
		return leftNumber < rightNumber
	}
	return left < right
}

func splitTrailingNumber(value string) (string, int, bool) {
	idx := len(value)
	for idx > 0 && value[idx-1] >= '0' && value[idx-1] <= '9' {
		idx--
	}
	if idx == len(value) {
		return value, 0, false
	}
	number, err := strconv.Atoi(value[idx:])
	if err != nil {
		return value, 0, false
	}
	return value[:idx], number, true
}

func sanitizeContextLine(value string) string {
	return taskListContextLineReplacer.Replace(strings.TrimSpace(value))
}

func writeTaskListContextLine(sb *strings.Builder, currentRunes *int, budgetChars int, line string, reserveTruncation bool) bool {
	reserved := linkedTaskListsSuffix
	if reserveTruncation {
		reserved = linkedTaskListsTruncationNotice + linkedTaskListsSuffix
	}
	lineLen := runeLen(line)
	if *currentRunes+lineLen+runeLen(reserved) > budgetChars {
		return false
	}
	sb.WriteString(line)
	*currentRunes += lineLen
	return true
}

func appendTaskListContextLine(sb *strings.Builder, currentRunes *int, line string) {
	sb.WriteString(line)
	*currentRunes += runeLen(line)
}

func closeLinkedTaskListsBlock(sb *strings.Builder, currentRunes *int, budgetChars int, truncated bool, hasContent bool) string {
	if !hasContent {
		return ""
	}
	if truncated {
		_ = writeTaskListContextLine(sb, currentRunes, budgetChars, linkedTaskListsTruncationNotice, false)
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
