package llm

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"assistente/internal/credentials"
)

// Corpo real observado no assistente.log (LiteLLM), redigido.
const litellmTokenRateLimitBody = `{"message":"Rate limit exceeded for api_key: redacted. Limit type: tokens. Current limit: 1000000, Remaining: 0. Limit resets at: 2026-09-15 17:56:48 UTC","type":"throttling_error","param":null,"code":"429"}`

func TestLooksLikeTokenRateLimit(t *testing.T) {
	cases := []struct {
		name string
		err  string
		want bool
	}{
		{"litellm tokens (janela + resets at)", `POST "https://x/responses": 429 Too Many Requests ` + litellmTokenRateLimitBody, true},
		{"limit type tokens", "429 Too Many Requests: Limit type: tokens", true},
		{"response.failed sem 429 na mensagem", `Rate limit exceeded for api_key: x. Limit type: tokens. Remaining: 0. Limit resets at 2026-09-15 17:56:48 UTC`, true},
		{"token com resets at (sem 429)", `token quota exceeded, resets at 2026-09-15 17:56:48 UTC`, true},
		{"TPM transitório NÃO é cota por janela", `429 Rate limit reached for gpt-4o on tokens per min (TPM), please retry`, false},
		{"TPM abreviado", `429 Too Many Requests: TPM limit for tokens reached`, false},
		{"token sem sinal de janela (ambíguo => retry)", `429 {"type":"throttling_error","message":"token budget exhausted"}`, false},
		{"429 de RPM (não é token)", `429 Too Many Requests {"message":"Rate limit exceeded: requests per minute","type":"requests"}`, false},
		{"429 genérico sem token", "429 Too Many Requests", false},
		{"503 sem sinal de token", "503 Service Unavailable", false},
		{"erro comum", "connection reset by peer", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeTokenRateLimit(tc.err); got != tc.want {
				t.Fatalf("looksLikeTokenRateLimit(%q) = %v; want %v", tc.err, got, tc.want)
			}
		})
	}
	// Um 429 de token continua sendo, por definição, um erro retryável genérico;
	// o tratamento específico apenas o intercepta antes do retry.
	if !isRetryableError("429") {
		t.Fatal("429 deveria ser retryável genérico (guarda de token roda antes)")
	}
}

// TestResponses_TokenRateLimitFalhaRapido garante que um 429 por cota de tokens
// (entregue como evento response.failed pela Responses API) não é retentado em
// loop e é reportado como código terminal não-retryável. Regressão dos ~15
// retries por turno vistos no assistente.log. O servidor é consultado uma única
// vez: se o guard não interceptasse, a tentativa cairia no retry genérico de
// 429 e o servidor seria chamado repetidamente.
func TestResponses_TokenRateLimitFalhaRapido(t *testing.T) {
	// error.message carrega o mesmo sinal do corpo do LiteLLM ("429" + limite de
	// tokens). Mantido como string simples para não aninhar JSON no SSE.
	failedEvent := `event: response.failed` + "\n" +
		`data: {"type":"response.failed","sequence_number":1,"response":{"id":"resp_1","object":"response","created_at":1,"status":"failed","model":"m","output":[],"error":{"code":"rate_limit_exceeded","message":"Rate limit exceeded for api_key: redacted. Limit type: tokens. Remaining: 0. Limit resets at 2026-09-15 17:56:48 UTC"}}}` + "\n\n"

	var hits int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(failedEvent))
	}))
	defer server.Close()

	p := NewOpenAIResponsesProvider(&ProviderConfig{
		ID: "resp-429", BaseURL: server.URL + "/v1", APIFormat: APIFormatOpenAIResponses, AuthMode: AuthModeNone,
	}, credentials.NewManager(nil))
	h := &spyHandler{}
	p.StreamChat(t.Context(), []Message{{Role: "user", Content: "oi"}}, ChatParams{Model: "m"}, h)

	if h.err != streamTokenRateLimitError {
		t.Fatalf("erro=%q, esperado %q", h.err, streamTokenRateLimitError)
	}
	if !h.nonRetryable {
		t.Fatal("429 de tokens deve ser terminal (não-retryável)")
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Fatalf("provedor chamado %d vezes; 429 de tokens não pode retentar em loop", got)
	}
}
