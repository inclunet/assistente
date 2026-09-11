package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"assistente/internal/credentials"
)

type espiaoAvisos struct {
	noopStreamHandler
	avisos   []TurnNotice
	conteudo string
	finish   FinishInfo
	usage    Usage
}

func (e *espiaoAvisos) OnTurnNotice(n TurnNotice) { e.avisos = append(e.avisos, n) }

func (e *espiaoAvisos) OnChunk(content string)                        { e.conteudo += content }
func (e *espiaoAvisos) OnDone(_ string, usage Usage, _ string)        { e.usage = usage }
func (e *espiaoAvisos) OnToolCalls([]ToolCall, string, Usage, string) {}
func (e *espiaoAvisos) OnFinishReason(info FinishInfo)                { e.finish = info }

// sseChatCompletion devolve um chunk SSE Chat Completions com conteÃºdo.
const sseChatCompletion = "data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":null}]}\n\n" +
	"data: {\"id\":\"1\",\"object\":\"chat.completion.chunk\",\"model\":\"m\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\n" +
	"data: [DONE]\n\n"

func TestStreamRetryAvisoSoEmFalhaTransitoria(t *testing.T) {
	var tentativas atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)

		if strings.Contains(string(body), "prompt_cache_key") {
			// Auto-ajuste: provider rejeita o parÃ¢metro. NÃ£o Ã© falha de rede.
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":{"message":"Unknown parameter: prompt_cache_key"}}`))
			return
		}
		if tentativas.Add(1) == 1 {
			// Falha transitÃ³ria de verdade na primeira tentativa limpa.
			// x-should-retry=false impede o retry interno da SDK openai-go
			// (senÃ£o ele consome o erro antes do laÃ§o do provider).
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
		MaxTokens:               77,
		PromptCacheKey:          "cache-key",
		PromptCacheHintFallback: &PromptCacheHintFallback{},
	}
	provider.StreamChat(ctx, []Message{{Role: "user", Content: "olÃ¡"}}, params, handler)

	if handler.err != "" {
		t.Fatalf("turno deveria concluir sem erro; veio %q", handler.err)
	}
	if got := handler.finish; got.Reason != FinishReasonStop || got.RawReason != "stop" ||
		got.Provider != "compat-test" || got.Model != "m" || got.OutputLimit != 77 || got.ResponseBytes != 2 {
		t.Fatalf("diagnóstico de término incompleto: %#v", got)
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

func TestChatCompletionsNaoEnviaLimiteZeroComoMaxCompletionTokens(t *testing.T) {
	var body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		body = string(payload)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseChatCompletion))
	}))
	defer server.Close()

	provider := NewOpenAIProvider(&ProviderConfig{
		ID:       "compat-zero",
		Name:     "Compat Zero",
		BaseURL:  server.URL + "/v1",
		AuthMode: AuthModeNone,
	}, credentials.NewManager(nil))
	handler := &espiaoAvisos{}

	provider.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}},
		ChatParams{Model: "m", MaxTokensMode: "completion_tokens"}, handler)

	if handler.err != "" {
		t.Fatalf("stream falhou: %s", handler.err)
	}
	if strings.Contains(body, "max_completion_tokens") || strings.Contains(body, `"max_tokens"`) {
		t.Fatalf("limite default/ausente foi serializado como zero: %s", body)
	}
}
