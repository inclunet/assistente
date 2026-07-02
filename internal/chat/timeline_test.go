package chat

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"

	"assistente/internal/database"
)

func resetInvalidToolCallsLogStateForTest(t *testing.T) {
	t.Helper()
	invalidToolCallsLogState.Lock()
	defer invalidToolCallsLogState.Unlock()
	invalidToolCallsLogState.seen = make(map[string]struct{})
	invalidToolCallsLogState.order = nil
}

func TestConsolidateTimelineTurnMessages_ToolOnlyPlaceholderOrdersToolCalls(t *testing.T) {
	turnID := "turn-1"
	consolidated := ConsolidateTimelineTurnMessages([]database.ChatMessage{
		{
			UUIDModel:  database.UUIDModel{ID: "tool-b-message"},
			Role:       "tool",
			Content:    "resultado b",
			TurnID:     &turnID,
			ToolCallID: "tool-b",
		},
		{
			UUIDModel:  database.UUIDModel{ID: "tool-a-message"},
			Role:       "tool",
			Content:    "resultado a",
			TurnID:     &turnID,
			ToolCallID: "tool-a",
		},
	}, nil)

	var calls []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(consolidated.ToolCalls), &calls); err != nil {
		t.Fatalf("unmarshal placeholder tool calls: %v", err)
	}
	if len(calls) != 2 || calls[0].ID != "tool-a" || calls[1].ID != "tool-b" {
		t.Fatalf("expected deterministic tool call order by id, got %+v", calls)
	}
}

func TestConsolidateTimelineTurnMessages_OrdersMessagesBeforeChoosingRepresentative(t *testing.T) {
	turnID := "turn-1"
	baseTime := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	consolidated := ConsolidateTimelineTurnMessages([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-final", CreatedAt: baseTime.Add(2 * time.Minute)},
			Role:      "assistant",
			Content:   "resposta final",
			TurnID:    &turnID,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-intermediate", CreatedAt: baseTime.Add(time.Minute)},
			Role:      "assistant",
			Content:   "resposta intermediaria",
			TurnID:    &turnID,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-first", CreatedAt: baseTime},
			Role:      "assistant",
			Content:   "primeira resposta",
			TurnID:    &turnID,
		},
	}, nil)

	if consolidated.ID != "assistant-final" {
		t.Fatalf("expected latest assistant as representative, got %s", consolidated.ID)
	}
	if consolidated.Content != "resposta final" {
		t.Fatalf("expected latest non-empty content, got %q", consolidated.Content)
	}
}

// Em turnos agenticos a resposta final e gravada num placeholder criado no
// INICIO do turno, enquanto as iteracoes intermediarias com tool calls vem depois.
func TestConsolidateTimelineTurn_AgenticPlaceholderIsConclusion(t *testing.T) {
	turnID := "turn-1"
	baseTime := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	result := ConsolidateTimelineTurn([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-final", CreatedAt: baseTime},
			Role:      "assistant",
			Content:   "conclusao do turno",
			TurnID:    &turnID,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-step1", CreatedAt: baseTime.Add(time.Minute)},
			Role:      "assistant",
			Content:   "vou verificar primeiro",
			TurnID:    &turnID,
			ToolCalls: `[{"id":"tool-1","type":"function","function":{"name":"search","arguments":"{}"}}]`,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-step2", CreatedAt: baseTime.Add(2 * time.Minute)},
			Role:      "assistant",
			Content:   "raciocinio intermediario",
			TurnID:    &turnID,
			ToolCalls: `[{"id":"tool-2","type":"function","function":{"name":"search","arguments":"{}"}}]`,
		},
	}, nil)

	if result.Message.Content != "conclusao do turno" {
		t.Fatalf("expected conclusion as canonical content, got %q", result.Message.Content)
	}
	if len(result.Segments) == 0 {
		t.Fatalf("expected segments to be populated")
	}
	last := result.Segments[len(result.Segments)-1]
	if last.Type != "text" || last.Content != "conclusao do turno" {
		t.Fatalf("expected conclusion as last text segment, got type=%q content=%q", last.Type, last.Content)
	}
	if first := result.Segments[0]; first.Type == "text" && first.Content == "conclusao do turno" {
		t.Fatalf("conclusion should not be the first segment")
	}
}

func TestConsolidateTimelineTurnMessages_DeduplicatesToolCallsByID(t *testing.T) {
	turnID := "turn-1"
	baseTime := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	consolidated := ConsolidateTimelineTurnMessages([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-first", CreatedAt: baseTime},
			Role:      "assistant",
			TurnID:    &turnID,
			ToolCalls: `[{"id":"tool-1","type":"function","function":{"name":"search","arguments":"{}"}}]`,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-duplicate", CreatedAt: baseTime.Add(time.Minute)},
			Role:      "assistant",
			TurnID:    &turnID,
			ToolCalls: `[{"id":"tool-1","type":"function","function":{"name":"search_again","arguments":"{}"}}]`,
		},
		{
			UUIDModel:  database.UUIDModel{ID: "tool-result", CreatedAt: baseTime.Add(2 * time.Minute)},
			Role:       "tool",
			Content:    "resultado preservado",
			TurnID:     &turnID,
			ToolCallID: "tool-1",
		},
	}, nil)

	var calls []struct {
		ID       string `json:"id"`
		Function struct {
			Name string `json:"name"`
		} `json:"function"`
		Result string `json:"result"`
	}
	if err := json.Unmarshal([]byte(consolidated.ToolCalls), &calls); err != nil {
		t.Fatalf("unmarshal consolidated tool calls: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one deduplicated tool call, got %+v", calls)
	}
	if calls[0].ID != "tool-1" || calls[0].Function.Name != "search" || calls[0].Result != "resultado preservado" {
		t.Fatalf("expected first tool call enriched with result, got %+v", calls[0])
	}
}

func TestConsolidateTimelineTurn_BuildsChronologicalSegmentsForAgenticTurn(t *testing.T) {
	turnID := "turn-1"
	baseTime := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	result := ConsolidateTimelineTurn([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-iter1", CreatedAt: baseTime},
			Role:      "assistant",
			TurnID:    &turnID,
			Content:   "vou pesquisar",
			ToolCalls: `[{"id":"tool-1","type":"function","function":{"name":"search","arguments":"{\"q\":\"foo\"}"}}]`,
		},
		{
			UUIDModel:  database.UUIDModel{ID: "tool-1-result", CreatedAt: baseTime.Add(time.Minute)},
			Role:       "tool",
			TurnID:     &turnID,
			ToolCallID: "tool-1",
			Content:    "resultado da busca",
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-iter2", CreatedAt: baseTime.Add(2 * time.Minute)},
			Role:      "assistant",
			TurnID:    &turnID,
			Content:   "agora vou buscar mais detalhes",
			ToolCalls: `[{"id":"tool-2","type":"function","function":{"name":"fetch","arguments":"{\"id\":1}"}}]`,
		},
		{
			UUIDModel:  database.UUIDModel{ID: "tool-2-result", CreatedAt: baseTime.Add(3 * time.Minute)},
			Role:       "tool",
			TurnID:     &turnID,
			ToolCallID: "tool-2",
			Content:    "detalhes",
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-final", CreatedAt: baseTime.Add(4 * time.Minute)},
			Role:      "assistant",
			TurnID:    &turnID,
			Content:   "resposta final",
		},
	}, nil)

	if result.Message.ID != "assistant-final" {
		t.Fatalf("expected last assistant as representative, got %s", result.Message.ID)
	}
	if result.Message.Content != "resposta final" {
		t.Fatalf("expected final content to drive representative content, got %q", result.Message.Content)
	}
	if len(result.Segments) != 5 {
		t.Fatalf("expected 5 chronological segments (text->tools->text->tools->text), got %d: %+v", len(result.Segments), result.Segments)
	}
	expected := []struct {
		kind    string
		content string
		toolIDs []string
	}{
		{kind: "text", content: "vou pesquisar"},
		{kind: "tool_calls", toolIDs: []string{"tool-1"}},
		{kind: "text", content: "agora vou buscar mais detalhes"},
		{kind: "tool_calls", toolIDs: []string{"tool-2"}},
		{kind: "text", content: "resposta final"},
	}
	for i, want := range expected {
		seg := result.Segments[i]
		if seg.Type != want.kind {
			t.Fatalf("segment[%d] expected kind %q, got %q", i, want.kind, seg.Type)
		}
		if want.kind == "text" && seg.Content != want.content {
			t.Fatalf("segment[%d] expected text %q, got %q", i, want.content, seg.Content)
		}
		if want.kind == "tool_calls" {
			if len(seg.ToolCalls) != len(want.toolIDs) {
				t.Fatalf("segment[%d] expected %d tool calls, got %d", i, len(want.toolIDs), len(seg.ToolCalls))
			}
			for j, expectedID := range want.toolIDs {
				if seg.ToolCalls[j].ID != expectedID {
					t.Fatalf("segment[%d] tool[%d] expected id %q, got %q", i, j, expectedID, seg.ToolCalls[j].ID)
				}
			}
		}
	}
	if result.Segments[1].ToolCalls[0].Result != "resultado da busca" {
		t.Fatalf("expected tool-1 result attached to its segment, got %+v", result.Segments[1].ToolCalls[0])
	}
	if result.Segments[3].ToolCalls[0].Result != "detalhes" {
		t.Fatalf("expected tool-2 result attached to its segment, got %+v", result.Segments[3].ToolCalls[0])
	}
}

func TestConsolidateTimelineTurn_SkipsSegmentsForTrivialTurn(t *testing.T) {
	turnID := "turn-1"
	baseTime := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	result := ConsolidateTimelineTurn([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-final", CreatedAt: baseTime},
			Role:      "assistant",
			TurnID:    &turnID,
			Content:   "resposta unica",
		},
	}, nil)

	if len(result.Segments) != 0 {
		t.Fatalf("expected no segments for trivial single-assistant turn, got %+v", result.Segments)
	}
}

func TestConsolidateTimelineTurn_ToolOnlyPlaceholderEmitsSegment(t *testing.T) {
	turnID := "turn-1"
	result := ConsolidateTimelineTurn([]database.ChatMessage{
		{
			UUIDModel:  database.UUIDModel{ID: "tool-a-message"},
			Role:       "tool",
			Content:    "resultado a",
			TurnID:     &turnID,
			ToolCallID: "tool-a",
		},
	}, nil)

	if len(result.Segments) != 1 {
		t.Fatalf("expected 1 placeholder tool_calls segment, got %+v", result.Segments)
	}
	if result.Segments[0].Type != "tool_calls" {
		t.Fatalf("expected tool_calls segment, got %q", result.Segments[0].Type)
	}
	if len(result.Segments[0].ToolCalls) != 1 || result.Segments[0].ToolCalls[0].ID != "tool-a" {
		t.Fatalf("expected single tool call with id tool-a, got %+v", result.Segments[0].ToolCalls)
	}
	if result.Segments[0].ToolCalls[0].Result != "resultado a" {
		t.Fatalf("expected tool result preserved, got %q", result.Segments[0].ToolCalls[0].Result)
	}
}

func TestConsolidateTimelineTurn_UsesRoleToolFallbackWithoutMessageToolCalls(t *testing.T) {
	turnID := "turn-1"
	result := ConsolidateTimelineTurn([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-marker"},
			Role:      "assistant",
			Content:   "",
			TurnID:    &turnID,
		},
		{
			UUIDModel:  database.UUIDModel{ID: "tool-a-message"},
			Role:       "tool",
			Content:    "resultado a",
			TurnID:     &turnID,
			ToolCallID: "tool-a",
		},
	}, nil)

	if len(result.Segments) != 1 {
		t.Fatalf("expected fallback tool segment, got %+v", result.Segments)
	}
	call := result.Segments[0].ToolCalls[0]
	if call.ID != "tool-a" || call.Result != "resultado a" {
		t.Fatalf("expected role=tool fallback call/result, got %+v", call)
	}
}

func TestConsolidateTimelineTurn_AttachesInvocationByAssistantMessageID(t *testing.T) {
	turnID := "turn-1"
	result := ConsolidateTimelineTurn([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-marker"},
			Role:      "assistant",
			Content:   "",
			TurnID:    &turnID,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-text"},
			Role:      "assistant",
			Content:   "texto posterior",
			TurnID:    &turnID,
		},
	}, nil, []TurnSegmentToolCall{
		{
			ID:                 "tool-a",
			Type:               "function",
			Function:           TurnSegmentToolFunction{Name: "search", Arguments: "{}"},
			Result:             "resultado a",
			AssistantMessageID: "assistant-marker",
		},
	})

	if len(result.Segments) != 2 {
		t.Fatalf("expected tool segment attached to marker before later text, got %+v", result.Segments)
	}
	if result.Segments[0].Type != "tool_calls" || result.Segments[0].ToolCalls[0].ID != "tool-a" {
		t.Fatalf("expected first segment to be marker tool call, got %+v", result.Segments[0])
	}
	if result.Segments[1].Type != "text" || result.Segments[1].Content != "texto posterior" {
		t.Fatalf("expected second segment to be later text, got %+v", result.Segments[1])
	}
}

func TestConsolidateTimelineTurn_SkipsAssistantScopedInvocationGroupBeforeFallback(t *testing.T) {
	turnID := "turn-1"
	baseTime := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	result := ConsolidateTimelineTurn([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-iter1", CreatedAt: baseTime},
			Role:      "assistant",
			Content:   "primeira iteracao",
			TurnID:    &turnID,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-iter2", CreatedAt: baseTime.Add(time.Minute)},
			Role:      "assistant",
			Content:   "segunda iteracao",
			TurnID:    &turnID,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-final", CreatedAt: baseTime.Add(2 * time.Minute)},
			Role:      "assistant",
			Content:   "resposta final",
			TurnID:    &turnID,
		},
	}, nil, []TurnSegmentToolCall{
		{
			ID:                 "tool-a",
			Type:               "function",
			Function:           TurnSegmentToolFunction{Name: "search", Arguments: "{}"},
			Result:             "resultado a",
			Iteration:          1,
			AssistantMessageID: "assistant-iter1",
		},
		{
			ID:        "tool-b",
			Type:      "function",
			Function:  TurnSegmentToolFunction{Name: "fetch", Arguments: "{}"},
			Result:    "resultado b",
			Iteration: 2,
		},
	})

	if len(result.Segments) != 5 {
		t.Fatalf("expected text/tool/text/tool/final segments, got %+v", result.Segments)
	}
	if result.Segments[1].Type != "tool_calls" || result.Segments[1].ToolCalls[0].ID != "tool-a" {
		t.Fatalf("expected assistant-scoped first invocation, got %+v", result.Segments[1])
	}
	if result.Segments[3].Type != "tool_calls" || result.Segments[3].ToolCalls[0].ID != "tool-b" {
		t.Fatalf("expected fallback to skip consumed group and attach second invocation, got %+v", result.Segments[3])
	}
}

func TestNormalizeInvocationToolCallsPreservesInputOrderWithinIteration(t *testing.T) {
	normalized := normalizeInvocationToolCalls([]TurnSegmentToolCall{
		{
			ID:        "tool-z",
			Type:      "function",
			Iteration: 1,
			Function:  TurnSegmentToolFunction{Name: "second", Arguments: "{}"},
		},
		{
			ID:        "tool-a",
			Type:      "function",
			Iteration: 1,
			Function:  TurnSegmentToolFunction{Name: "first", Arguments: "{}"},
		},
		{
			ID:        "tool-later",
			Type:      "function",
			Iteration: 2,
			Function:  TurnSegmentToolFunction{Name: "later", Arguments: "{}"},
		},
	}, nil)

	if len(normalized) != 3 {
		t.Fatalf("expected 3 normalized calls, got %+v", normalized)
	}
	got := []string{normalized[0].ID, normalized[1].ID, normalized[2].ID}
	want := []string{"tool-z", "tool-a", "tool-later"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("expected order %v, got %v", want, got)
		}
	}
}

func TestConsolidateTimelineTurn_DoesNotPromoteIntermediateTextAsFinalContent(t *testing.T) {
	turnID := "turn-1"
	result := ConsolidateTimelineTurn([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-placeholder"},
			Role:      "assistant",
			Content:   "",
			TurnID:    &turnID,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-iter1"},
			Role:      "assistant",
			Content:   "vou buscar",
			TurnID:    &turnID,
		},
	}, nil, []TurnSegmentToolCall{
		{
			ID:                 "tool-a",
			Type:               "function",
			Function:           TurnSegmentToolFunction{Name: "search", Arguments: "{}"},
			Result:             "resultado a",
			AssistantMessageID: "assistant-iter1",
		},
	})

	if result.Message.Content != "" {
		t.Fatalf("expected no canonical final content while placeholder is still empty, got %q", result.Message.Content)
	}
	if len(result.Segments) != 2 {
		t.Fatalf("expected intermediate text and tool segment, got %+v", result.Segments)
	}
}

func TestConsolidateTimelineTurn_UsesLastFinalCandidateWhenInvocationLacksAssistantID(t *testing.T) {
	turnID := "turn-1"
	baseTime := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	result := ConsolidateTimelineTurn([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-placeholder", CreatedAt: baseTime},
			Role:      "assistant",
			Content:   "",
			TurnID:    &turnID,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-intermediate", CreatedAt: baseTime.Add(time.Minute)},
			Role:      "assistant",
			Content:   "vou consultar uma ferramenta",
			TurnID:    &turnID,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-final", CreatedAt: baseTime.Add(2 * time.Minute)},
			Role:      "assistant",
			Content:   "resposta final",
			TurnID:    &turnID,
		},
	}, nil, []TurnSegmentToolCall{
		{
			ID:        "tool-a",
			Type:      "function",
			Function:  TurnSegmentToolFunction{Name: "search", Arguments: "{}"},
			Result:    "resultado a",
			Iteration: 1,
		},
	})

	if len(result.Segments) != 3 {
		t.Fatalf("expected text, tool call and final text segments, got %+v", result.Segments)
	}
	if result.Segments[0].Type != "text" || result.Segments[0].Content != "vou consultar uma ferramenta" {
		t.Fatalf("expected intermediate text first, got %+v", result.Segments[0])
	}
	if result.Segments[1].Type != "tool_calls" || len(result.Segments[1].ToolCalls) != 1 || result.Segments[1].ToolCalls[0].ID != "tool-a" {
		t.Fatalf("expected invocation fallback attached before final text, got %+v", result.Segments[1])
	}
	if result.Segments[2].Type != "text" || result.Segments[2].Content != "resposta final" {
		t.Fatalf("expected last assistant as final text segment, got %+v", result.Segments[2])
	}
}

func TestConsolidateTimelineTurn_KeepsInitialPlaceholderFinalWhenInvocationLacksAssistantID(t *testing.T) {
	turnID := "turn-1"
	baseTime := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	result := ConsolidateTimelineTurn([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-final", CreatedAt: baseTime},
			Role:      "assistant",
			Content:   "conclusao do turno",
			TurnID:    &turnID,
		},
		{
			UUIDModel: database.UUIDModel{ID: "assistant-intermediate", CreatedAt: baseTime.Add(time.Minute)},
			Role:      "assistant",
			Content:   "vou consultar uma ferramenta",
			TurnID:    &turnID,
		},
	}, nil, []TurnSegmentToolCall{
		{
			ID:        "tool-a",
			Type:      "function",
			Function:  TurnSegmentToolFunction{Name: "search", Arguments: "{}"},
			Result:    "resultado a",
			Iteration: 1,
		},
	})

	if result.Message.Content != "conclusao do turno" {
		t.Fatalf("expected initial placeholder conclusion as canonical content, got %q", result.Message.Content)
	}
	if len(result.Segments) != 3 {
		t.Fatalf("expected intermediate text, tool call and final placeholder text segments, got %+v", result.Segments)
	}
	if result.Segments[0].Type != "text" || result.Segments[0].Content != "vou consultar uma ferramenta" {
		t.Fatalf("expected intermediate text first, got %+v", result.Segments[0])
	}
	if result.Segments[1].Type != "tool_calls" || len(result.Segments[1].ToolCalls) != 1 || result.Segments[1].ToolCalls[0].ID != "tool-a" {
		t.Fatalf("expected invocation fallback attached before final text, got %+v", result.Segments[1])
	}
	if result.Segments[2].Type != "text" || result.Segments[2].Content != "conclusao do turno" {
		t.Fatalf("expected initial placeholder conclusion as final text segment, got %+v", result.Segments[2])
	}
}

func TestParseToolCalls_InvalidJSONReturnsNil(t *testing.T) {
	resetInvalidToolCallsLogStateForTest(t)

	if calls := ParseToolCalls("message-invalid", "{invalid"); calls != nil {
		t.Fatalf("expected invalid tool calls JSON to be discarded, got %+v", calls)
	}
}

func TestConsolidateTimelineTurn_InvalidToolCallsClearsRepresentativeToolCalls(t *testing.T) {
	resetInvalidToolCallsLogStateForTest(t)

	turnID := "turn-1"
	result := ConsolidateTimelineTurn([]database.ChatMessage{
		{
			UUIDModel: database.UUIDModel{ID: "assistant-invalid-tool-calls"},
			Role:      "assistant",
			Content:   "resposta",
			TurnID:    &turnID,
			ToolCalls: "{invalid",
		},
	}, nil)

	if result.Message.ToolCalls != "" {
		t.Fatalf("expected invalid representative tool calls to be cleared, got %q", result.Message.ToolCalls)
	}
}

func TestParseToolCalls_InvalidJSONLogsOncePerMessage(t *testing.T) {
	resetInvalidToolCallsLogStateForTest(t)

	var buf bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(previousLogger)

	messageID := "message-invalid-log-once"
	ParseToolCalls(messageID, "{invalid")
	ParseToolCalls(messageID, "{invalid")

	if got := strings.Count(buf.String(), messageID); got != 1 {
		t.Fatalf("expected one invalid tool_calls log for %s, got %d logs: %s", messageID, got, buf.String())
	}
}

func TestParseToolCalls_InvalidJSONLogCacheIsBounded(t *testing.T) {
	resetInvalidToolCallsLogStateForTest(t)

	var buf bytes.Buffer
	previousLogger := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, nil)))
	defer slog.SetDefault(previousLogger)

	for i := 0; i < invalidToolCallsLogLimit+10; i++ {
		ParseToolCalls("message-invalid-"+strconv.Itoa(i), "{invalid")
	}

	invalidToolCallsLogState.Lock()
	defer invalidToolCallsLogState.Unlock()
	if len(invalidToolCallsLogState.seen) != invalidToolCallsLogLimit {
		t.Fatalf("expected invalid tool_calls log cache bounded to %d entries, got %d", invalidToolCallsLogLimit, len(invalidToolCallsLogState.seen))
	}
}

func TestMessageTimelineItemKey_UserMessagesIgnoreTurnID(t *testing.T) {
	turnID := "turn-1"
	message := database.ChatMessage{
		UUIDModel: database.UUIDModel{ID: "user-1"},
		Role:      "user",
		TurnID:    &turnID,
	}

	if key := MessageTimelineItemKey(message); key != "message:user-1" {
		t.Fatalf("expected user message key to ignore TurnID, got %q", key)
	}
}
