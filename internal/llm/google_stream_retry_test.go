package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
)

// sseGeminiChunk devolve um chunk Gemini SSE com texto.
const sseGeminiChunk = "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}],\"role\":\"model\"},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":2,\"totalTokenCount\":3}}\r\n\r\n"

func TestGoogleStreamRetryAvisoEFinalizacao(t *testing.T) {
	var tentativas atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "streamGenerateContent") {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if tentativas.Add(1) == 1 {
			w.Header().Set("x-should-retry", "false")
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseGeminiChunk))
	}))
	defer server.Close()

	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	ctxCred := database.WithUserID(context.Background(), "user-1")
	if err := credMgr.RegisterPatternWithContext(ctxCred, "gemini.test", &credentials.AuthConfig{
		Type:  "bearer",
		Token: "test-key",
	}); err != nil {
		t.Fatalf("RegisterPatternWithContext() error = %v", err)
	}
	provider := NewGoogleProvider(&ProviderConfig{
		ID:                "google-test",
		Name:              "Google Test",
		BaseURL:           server.URL,
		Type:              ProviderType("gemini"),
		Model:             "gemini-test",
		CredentialPattern: "gemini.test",
	}, credMgr)
	handler := &espiaoAvisos{}

	provider.StreamChat(ctxCred, []Message{{Role: "user", Content: "olá"}}, ChatParams{Model: "gemini-test"}, handler)

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
