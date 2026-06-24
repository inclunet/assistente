package contextprovider

import "strings"

// TrimTaggedBlockToBudget trims a tagged prompt block while preserving its
// enclosing tags and adding a truncation notice. If the minimal tagged envelope
// cannot fit the budget, the block is omitted.
func TrimTaggedBlockToBudget(content string, tag string, truncationNotice string, budgetChars int) string {
	if budgetChars <= 0 {
		return ""
	}
	content = strings.TrimSpace(content)
	if RuneLen(content) <= budgetChars {
		return content
	}
	prefix := "<" + tag + ">\n"
	suffix := "\n</" + tag + ">"
	if !strings.HasPrefix(content, prefix) || !strings.HasSuffix(content, suffix) {
		return trimBlockToBudget(content, budgetChars)
	}
	notice := "\n" + truncationNotice
	minimal := prefix + truncationNotice + suffix
	if RuneLen(minimal) > budgetChars {
		return ""
	}
	bodyBudget := budgetChars - RuneLen(prefix) - RuneLen(notice) - RuneLen(suffix)
	if bodyBudget <= 0 {
		return minimal
	}
	body := strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(content, prefix), suffix))
	bodyRunes := []rune(body)
	if len(bodyRunes) > bodyBudget {
		body = strings.TrimSpace(string(bodyRunes[:bodyBudget]))
	}
	if body == "" {
		return minimal
	}
	return prefix + body + notice + suffix
}

func MinimalTaggedBlockLen(tag string, truncationNotice string) int {
	return RuneLen("<"+tag+">\n") + RuneLen(truncationNotice) + RuneLen("\n</"+tag+">")
}

func RuneLen(value string) int {
	count := 0
	for range value {
		count++
	}
	return count
}

func trimBlockToBudget(content string, budgetChars int) string {
	if budgetChars <= 0 {
		return ""
	}
	content = strings.TrimSpace(content)
	if RuneLen(content) <= budgetChars {
		return content
	}
	return ""
}
