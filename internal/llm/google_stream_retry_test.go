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
const sseGeminiChunk = "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}],\"role\":\"model\"},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":1,\"candidatesTokenCount\":2,\"thoughtsTokenCount\":5,\"totalTokenCount\":8}}\r\n\r\n"

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

	provider.StreamChat(ctxCred, []Message{{Role: "user", Content: "olá"}}, ChatParams{Model: "gemini-test", MaxTokens: 321}, handler)

	if handler.err != "" {
		t.Fatalf("turno deveria concluir sem erro; veio %q", handler.err)
	}
	if !strings.Contains(handler.conteudo, "ok") {
		t.Fatalf("resposta incompleta: %q", handler.conteudo)
	}
	if got := handler.finish; got.Reason != FinishReasonStop || got.RawReason != "STOP" ||
		got.Provider != "google-test" || got.Model != "gemini-test" || got.OutputLimit != 321 || got.ResponseBytes != 2 {
		t.Fatalf("diagnóstico de término incompleto: %#v", got)
	}
	if !handler.usage.Reported || !handler.usage.ReasoningTokensReported || handler.usage.ReasoningTokens != 5 {
		t.Fatalf("usage de reasoning não preservada: %#v", handler.usage)
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

func TestGooglePreservaReasoningZeroExplicitamenteReportado(t *testing.T) {
	const stream = "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}],\"role\":\"model\"},\"finishReason\":\"STOP\",\"index\":0}],\"usageMetadata\":{\"promptTokenCount\":1,\"thoughtsTokenCount\":0,\"totalTokenCount\":1}}\r\n\r\n"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(stream))
	}))
	defer server.Close()

	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	ctxCred := database.WithUserID(context.Background(), "user-zero")
	if err := credMgr.RegisterPatternWithContext(ctxCred, "gemini-zero.test", &credentials.AuthConfig{
		Type: "bearer", Token: "test-key",
	}); err != nil {
		t.Fatalf("RegisterPatternWithContext() error = %v", err)
	}
	provider := NewGoogleProvider(&ProviderConfig{
		ID: "google-zero", Name: "Google Zero", BaseURL: server.URL,
		Type: ProviderType("gemini"), Model: "gemini-test", CredentialPattern: "gemini-zero.test",
	}, credMgr)
	handler := &espiaoAvisos{}

	provider.StreamChat(ctxCred, []Message{{Role: "user", Content: "oi"}},
		ChatParams{Model: "gemini-test"}, handler)

	if handler.err != "" {
		t.Fatalf("stream falhou: %s", handler.err)
	}
	if !handler.usage.Reported || !handler.usage.ReasoningTokensReported ||
		handler.usage.ReasoningTokens != 0 {
		t.Fatalf("zero explícito de reasoning não foi preservado: %#v", handler.usage)
	}
	if handler.usage.OutputTokensReported {
		t.Fatalf("output ausente não pode virar zero reportado: %#v", handler.usage)
	}
}

func TestGoogleTimeoutParcialPreservaDiagnosticos(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(sseGeminiChunk))
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer server.Close()

	credMgr := credentials.NewManager([]byte("test-key-exactly-32-bytes-long!!"))
	ctxCred := database.WithUserID(context.Background(), "user-timeout")
	if err := credMgr.RegisterPatternWithContext(ctxCred, "gemini-timeout.test", &credentials.AuthConfig{
		Type: "bearer", Token: "test-key",
	}); err != nil {
		t.Fatalf("RegisterPatternWithContext() error = %v", err)
	}
	provider := NewGoogleProvider(&ProviderConfig{
		ID: "google-timeout", Name: "Google Timeout", BaseURL: server.URL,
		Type: ProviderType("gemini"), Model: "gemini-test", CredentialPattern: "gemini-timeout.test",
		StreamIdleTimeoutSeconds: 1,
	}, credMgr)
	handler := &espiaoAvisos{}

	provider.StreamChat(ctxCred, []Message{{Role: "user", Content: "oi"}},
		ChatParams{Model: "gemini-test", MaxTokens: 321}, handler)

	if handler.err != streamIdleErrorMessage || !handler.naoRetentavel {
		t.Fatalf("timeout terminal inválido: err=%q nonRetryable=%v", handler.err, handler.naoRetentavel)
	}
	if got := handler.finish; got.Provider != "google-timeout" || got.Model != "gemini-test" ||
		got.OutputLimit != 321 || got.ResponseBytes != len("ok") {
		t.Fatalf("diagnóstico de timeout incompleto: %+v", got)
	}
	if !handler.usage.OutputTokensReported || handler.usage.CompletionTokens != 2 ||
		!handler.usage.ReasoningTokensReported || handler.usage.ReasoningTokens != 5 {
		t.Fatalf("usage de timeout incompleta: %+v", handler.usage)
	}
}
