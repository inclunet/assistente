package apidto

import (
	"encoding/json"
	"testing"
)

func TestTokenStatsJSONTagsStable(t *testing.T) {
	t.Parallel()
	enabled := true
	s := TokenStats{
		ConversationID:     "c1",
		PromptTokens:       10,
		CompletionTokens:   20,
		TotalTokens:        30,
		PromptCacheEnabled: &enabled,
		ToolBreakdown: []ToolUsageBreakdown{{
			ToolName:              "search",
			CallCount:             2,
			TotalPromptTokens:     4,
			TotalCompletionTokens: 6,
			TotalTokens:           10,
		}},
	}

	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var asMap map[string]any
	if err := json.Unmarshal(raw, &asMap); err != nil {
		t.Fatalf("unmarshal map: %v", err)
	}

	for _, key := range []string{
		"conversationId",
		"promptTokens",
		"completionTokens",
		"totalTokens",
		"promptCacheEnabled",
		"toolBreakdown",
	} {
		if _, ok := asMap[key]; !ok {
			t.Fatalf("tag JSON ausente no contrato da borda: %q (payload=%s)", key, raw)
		}
	}

	tools, ok := asMap["toolBreakdown"].([]any)
	if !ok || len(tools) != 1 {
		t.Fatalf("toolBreakdown inválido: %#v", asMap["toolBreakdown"])
	}
	tool, ok := tools[0].(map[string]any)
	if !ok {
		t.Fatalf("item de toolBreakdown inválido: %#v", tools[0])
	}
	for _, key := range []string{
		"toolName",
		"callCount",
		"totalPromptTokens",
		"totalCompletionTokens",
		"totalTokens",
	} {
		if _, ok := tool[key]; !ok {
			t.Fatalf("tag JSON ausente em ToolUsageBreakdown: %q", key)
		}
	}
	if tool["toolName"] != "search" {
		t.Fatalf("toolName=%v, want search", tool["toolName"])
	}
}
