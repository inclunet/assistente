package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"assistente/internal/credentials"
)

// sseAnthropicCompletion devolve um turno mínimo Anthropic Messages SSE.
const sseAnthropicCompletion = "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"1\",\"model\":\"claude-test\",\"usage\":{\"input_tokens\":1}}}\n\n" +
	"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n" +
	"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n" +
	"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n" +
	"event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":2}}\n\n" +
	"event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n"

func TestAnthropicStreamRetryAvisoEFinalizacao(t *testing.T) {
	var tentativas atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tentativas.Add(1) == 1 {
			// Falha transitória de verdade; x-should-retry=false impede o
			// retry interno da SDK anthropic de consumir o erro antes do
			// laço do provider.
			w.Header().Set("x-should-retry", "false")
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseAnthropicCompletion))
	}))
	defer server.Close()

	ctx := context.Background()
	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	provider := NewAnthropicProvider(&ProviderConfig{
		ID:       "anthropic-test",
		Name:     "Anthropic Test",
		BaseURL:  server.URL,
		Type:     ProviderClaude,
		Model:    "claude-test",
		AuthMode: AuthModeNone,
	}, credMgr)
	handler := &espiaoAvisos{}

	provider.StreamChat(ctx, []Message{{Role: "user", Content: "olá"}}, ChatParams{Model: "claude-test"}, handler)

	if handler.err != "" {
		t.Fatalf("turno deveria concluir sem erro; veio %q", handler.err)
	}
	if !strings.Contains(handler.conteudo, "ok") {
		t.Fatalf("resposta incompleta: %q", handler.conteudo)
	}

	var avisosRetry int
	for _, aviso := range handler.avisos {
		if aviso.Kind == TurnNoticeStreamRetry {
			avisosRetry++
		} else {
			t.Errorf("aviso inesperado kind=%q", aviso.Kind)
		}
	}
	if avisosRetry != 1 {
		t.Fatalf("esperava exatamente 1 aviso stream_retry (falha 502); veio %d", avisosRetry)
	}
}
