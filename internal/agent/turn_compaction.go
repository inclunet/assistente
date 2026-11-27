package agent

import (
	"context"
	"encoding/json"
	"strings"

	"assistente/internal/llm"
	"assistente/internal/logging"
	"assistente/internal/summarization"
	"assistente/internal/tools"
)

// TurnCompaction usa a conexão efetiva sem ferramentas nativas. O resumo só
// substitui o intervalo correspondente na entrada da próxima chamada.
type TurnCompaction struct {
	MinKeep   int
	Summarize func(context.Context, string, string, string) (string, error)
}

func (r *agenticLoopRunner) prepareToolContext(ctx context.Context, executions []tools.ToolExecutionResult, contents []string) bool {
	if r.params.ContextWindow <= 0 {
		return true
	}
	reasoning := r.replaysReasoningContent()
	original := append([]string(nil), contents...)
	check := func(messages []llm.Message) ContextPreCheckResult {
		copy(contents, original)
		return PreCheckContextWindow(r.params.ContextWindow, r.params.OutputTokenLimit(), messages, contents, reasoning)
	}
	budget := check(r.messages)
	if budget.Truncated && r.compaction != nil && r.compaction.Summarize != nil {
		spans := compactableRanges(r.messages, r.compaction.MinKeep, reasoning)
		for i, span := range spans {
			if !budget.Truncated || ctx.Err() != nil {
				break
			}
			if next := r.compactToolHistory(ctx, span, reasoning); next != nil {
				if i == 0 && len(spans) == 2 && span.end <= spans[1].start {
					shift := len(next) - len(r.messages)
					spans[1].start += shift
					spans[1].end += shift
				}
				r.messages = next
				budget = check(next)
			}
		}
		if ctx.Err() != nil {
			return false
		}
	}
	reconcileToolContentsWithContracts(ctx, executions, contents)
	used := 0
	for i, content := range contents {
		if content == "" && original[i] != "" {
			return false
		}
		used += estimateTokens(content)
	}
	return used <= budget.AvailableTokens
}

type historySpan struct{ start, end int }

func (r *agenticLoopRunner) compactToolHistory(ctx context.Context, span historySpan, reasoning bool) []llm.Message {
	start, end := span.start, span.end
	history := append([]llm.Message(nil), r.messages[start:end]...)
	for i := range history {
		history[i].Thinking = ""
		history[i].ReasoningContent = ""
	}
	raw, err := json.Marshal(history)
	if err != nil {
		return nil
	}
	summary, err := r.compaction.Summarize(ctx, r.params.Model, summarization.SummaryPrompt, string(raw))
	if err != nil || strings.TrimSpace(summary) == "" {
		logging.Warnf(ctx, "agent.context", "resumo no turno indisponível")
		return nil
	}
	var replacement []llm.Message
	if history[0].Role == "user" {
		// Preserve a alternância dos protocolos que exigem uma primeira mensagem de usuário.
		replacement = append(replacement, llm.Message{Role: "user", Content: "Resumo da conversa anterior?"})
	}
	replacement = append(replacement, llm.Message{Role: "assistant", Content: "Resumo do histórico anterior:\n" + summary})
	if estimateMessageTokens(replacement, reasoning) >= estimateMessageTokens(r.messages[start:end], reasoning) {
		return nil
	}
	next := append([]llm.Message(nil), r.messages[:start]...)
	next = append(next, replacement...)
	next = append(next, r.messages[end:]...)
	logging.Infof(ctx, "agent.context", "compactação no turno: %d mensagens resumidas", end-start)
	return next
}

func markCompactionTarget(messages []llm.Message) []llm.Message {
	last := -1
	for i, m := range messages {
		if m.Role == "user" {
			if m.TurnContextTarget {
				return messages
			}
			last = i
		}
	}
	if last >= 0 {
		messages = append([]llm.Message(nil), messages...)
		messages[last].TurnContextTarget = true
	}
	return messages
}

// Seleciona um bloco contíguo de turnos antigos ou rodadas de ferramentas já
// consumidas. Mantém o pedido atual, instruções e o lote mais recente intactos.
func compactableRanges(messages []llm.Message, minKeep int, reasoning bool) []historySpan {
	current, lastUser := -1, -1
	for i, m := range messages {
		if m.Role == "user" {
			lastUser = i
			if m.TurnContextTarget {
				current = i
			}
		}
	}
	if current < 0 || minKeep < 1 {
		return nil
	}
	var spans []historySpan
	consider := func(start, end int) {
		if start >= end {
			return
		}
		for _, m := range messages[start:end] {
			if m.Role != "assistant" && m.Role != "user" && m.Role != "tool" {
				return
			}
			if _, ok := m.Content.(string); !ok && m.Content != nil {
				return
			}
		}
		spans = append(spans, historySpan{start, end})
	}
	first := 0
	for first < current && messages[first].Role != "user" {
		first++
	}
	end := current + 1 - minKeep
	for end > first && messages[end].Role != "user" {
		end--
	}
	consider(first, end)
	end = len(messages) - minKeep
	for end > lastUser+1 && messages[end].Role != "assistant" {
		end--
	}
	consider(lastUser+1, end)
	if len(spans) == 2 {
		a, b := spans[0], spans[1]
		if estimateMessageTokens(messages[a.start:a.end], reasoning) < estimateMessageTokens(messages[b.start:b.end], reasoning) {
			spans[0], spans[1] = b, a
		}
	}
	return spans
}
