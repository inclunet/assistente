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

type espiaoAvisos struct {
	noopStreamHandler
	avisos []TurnNotice
}

func (e *espiaoAvisos) OnTurnNotice(n TurnNotice) { e.avisos = append(e.avisos, n) }

// sseChatCompletion devolve um chunk SSE Chat Completions com conteúdo.
const sseChatCompletion = "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n" +
	"data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
	"data: [DONE]\n\n"

func TestStreamRetryAvisoSoEmFalhaTransitoria(t *testing.T) {
	var tentativas atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)

		if strings.Contains(string(body), "prompt_cache_key") {
			// Auto-ajuste: provider rejeita o parâmetro. Não é falha de rede.
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Unknown parameter: prompt_cache_key"}}`))
			return
		}
		if tentativas.Add(1) == 1 {
			// Falha transitória de verdade na primeira tentativa limpa.
			// x-should-retry=false impede o retry interno da SDK openai-go
			// (senão ele consome o erro antes do laço do provider).
			w.Header().Set("x-should-retry", "false")
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseChatCompletion))
	}))
	defer server.Close()

	ctx := context.Background()
	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	p := &ProviderConfig{
		ID:       "compat-test",
		Name:     "Compat Test",
		BaseURL:  server.URL + "/v1",
		Type:     ProviderOpenAI,
		Model:    "m",
		AuthMode: AuthModeNone,
	}
	provider := NewOpenAIProvider(p, credMgr)
	handler := &espiaoAvisos{}

	params := ChatParams{
		Model:                   "m",
		PromptCacheKey:          "cache-key",
		PromptCacheHintFallback: &PromptCacheHintFallback{},
	}
	provider.StreamChat(ctx, []Message{{Role: "user", Content: "olá"}}, params, handler)

	if handler.err != "" {
		t.Fatalf("turno deveria concluir sem erro; veio %q", handler.err)
	}

	var avisosRetry int
	for _, aviso := range handler.avisos {
		if aviso.Kind == TurnNoticeStreamRetry {
			avisosRetry++
			if aviso.Count < 1 {
				t.Errorf("aviso stream_retry com Count=%d; esperava >= 1", aviso.Count)
			}
		} else {
			t.Errorf("aviso inesperado kind=%q", aviso.Kind)
		}
	}
	if avisosRetry != 1 {
		t.Fatalf("esperava exatamente 1 aviso stream_retry (falha 502); veio %d", avisosRetry)
	}
}
