package llm

import (
	"context"
	"math/rand"
	"strings"
	"time"
)

// StreamHandler é a interface para lidar com eventos de streaming de LLM.
type StreamHandler interface {
	OnChunk(content string)
	OnThinking(content string)
	OnThinkingDone(fullReasoning string)
	OnToolCalls(calls []ToolCall, fullResponse string, usage Usage, model string)
	OnError(err string)
	OnDone(fullResponse string, usage Usage, model string)
}

func nextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}

func sleepWithJitter(ctx context.Context, base time.Duration) {
	if base <= 0 {
		return
	}

	jitter := time.Duration(rand.Intn(250)) * time.Millisecond
	wait := base + jitter

	t := time.NewTimer(wait)
	defer t.Stop()

	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

// processThinkingTags detecta e extrai conteúdo de tags <thinking> do streaming.
// Retorna o conteúdo que NÃO é thinking para ser processado normalmente.
func processThinkingTags(content string, isThinking *bool, thinkingBuffer, fullReasoning *strings.Builder, handler StreamHandler) string {
	var result strings.Builder
	i := 0

	for i < len(content) {
		if *isThinking {
			endIdx := strings.Index(content[i:], "</thinking>")
			if endIdx != -1 {
				thinkingContent := content[i : i+endIdx]
				thinkingBuffer.WriteString(thinkingContent)
				fullReasoning.WriteString(thinkingContent)
				handler.OnThinking(thinkingContent)

				*isThinking = false
				i += endIdx + len("</thinking>")
			} else {
				thinkingContent := content[i:]
				thinkingBuffer.WriteString(thinkingContent)
				fullReasoning.WriteString(thinkingContent)
				handler.OnThinking(thinkingContent)
				return result.String()
			}
		} else {
			startIdx := strings.Index(content[i:], "<thinking>")
			if startIdx != -1 {
				result.WriteString(content[i : i+startIdx])
				*isThinking = true
				thinkingBuffer.Reset()
				i += startIdx + len("<thinking>")
			} else {
				result.WriteString(content[i:])
				break
			}
		}
	}

	return result.String()
}
