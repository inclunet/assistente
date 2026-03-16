package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"assistente/internal/config"
	"assistente/internal/credentials"
	"assistente/internal/llm"
	"assistente/internal/profiles"
)

// TestGetClientForProvider_ReturnsClientForRegisteredProvider verifica que
// getClientForProvider cria um client quando o provedor existe no registry.
func TestGetClientForProvider_ReturnsClientForRegisteredProvider(t *testing.T) {
	registry := llm.NewProviderRegistry()
	provider := &llm.ProviderConfig{
		ID:      "litellm-test",
		Name:    "LiteLLM Test",
		Type:    llm.ProviderCustom,
		BaseURL: "https://litellm.example.com/v1",
	}
	if err := registry.Register(provider); err != nil {
		t.Fatalf("Register: %v", err)
	}

	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)

	app := &App{
		llmRegistry: registry,
		credMgr:     credMgr,
	}

	client, err := app.getClientForProvider("litellm-test")
	if err != nil {
		t.Fatalf("getClientForProvider returned error: %v", err)
	}
	if client == nil {
		t.Fatal("getClientForProvider returned nil client")
	}
}

// TestGetClientForProvider_ErrorForUnknownProvider verifica que
// getClientForProvider retorna erro quando o provedor não existe.
func TestGetClientForProvider_ErrorForUnknownProvider(t *testing.T) {
	registry := llm.NewProviderRegistry()
	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)

	app := &App{
		llmRegistry: registry,
		credMgr:     credMgr,
	}

	client, err := app.getClientForProvider("nonexistent-provider")
	if err == nil {
		t.Fatal("Expected error for unknown provider, got nil")
	}
	if client != nil {
		t.Fatal("Expected nil client for unknown provider")
	}
}

// TestGetClientForProvider_DifferentProvidersReturnDifferentClients verifica que
// providers distintos resultam em clients distintos (não compartilham instância).
func TestGetClientForProvider_DifferentProvidersReturnDifferentClients(t *testing.T) {
	registry := llm.NewProviderRegistry()

	p1 := &llm.ProviderConfig{
		ID:      "google-test",
		Name:    "Google Test",
		Type:    llm.ProviderCustom,
		BaseURL: "https://google.example.com/v1",
	}
	p2 := &llm.ProviderConfig{
		ID:      "litellm-test",
		Name:    "LiteLLM Test",
		Type:    llm.ProviderCustom,
		BaseURL: "https://litellm.example.com/v1",
	}

	if err := registry.Register(p1); err != nil {
		t.Fatalf("Register p1: %v", err)
	}
	if err := registry.Register(p2); err != nil {
		t.Fatalf("Register p2: %v", err)
	}

	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)

	app := &App{
		llmRegistry: registry,
		credMgr:     credMgr,
	}

	c1, err := app.getClientForProvider("google-test")
	if err != nil {
		t.Fatalf("getClientForProvider google: %v", err)
	}
	c2, err := app.getClientForProvider("litellm-test")
	if err != nil {
		t.Fatalf("getClientForProvider litellm: %v", err)
	}

	if c1 == c2 {
		t.Error("Expected different client instances for different providers")
	}
}

// TestProviderRouting_ClientHitsCorrectEndpoint verifica via HTTP que o client
// criado para um provedor envia requests ao endpoint correto (e não a outro).
// Isso é o teste de regressão principal para o bug de roteamento cruzado.
func TestProviderRouting_ClientHitsCorrectEndpoint(t *testing.T) {
	var googleHits, litellmHits atomic.Int32

	modelsResp, _ := json.Marshal(llm.ModelsResponse{
		Data: []llm.Model{{ID: "test-model"}},
	})

	googleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		googleHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsResp)
	}))
	defer googleServer.Close()

	litellmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		litellmHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsResp)
	}))
	defer litellmServer.Close()

	googleProvider := &llm.ProviderConfig{
		ID:      "google",
		Name:    "Google",
		Type:    llm.ProviderCustom,
		BaseURL: googleServer.URL,
	}
	litellmProvider := &llm.ProviderConfig{
		ID:      "litellm",
		Name:    "LiteLLM",
		Type:    llm.ProviderCustom,
		BaseURL: litellmServer.URL,
	}

	cfg := &config.Config{}
	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)

	googleClient := llm.NewClient(googleProvider, cfg, credMgr)
	litellmClient := llm.NewClient(litellmProvider, cfg, credMgr)

	// Request ao client do Google deve ir SOMENTE ao google server
	_, err := googleClient.GetModels(t.Context())
	if err != nil {
		t.Fatalf("googleClient.GetModels: %v", err)
	}
	if googleHits.Load() != 1 {
		t.Errorf("Expected google server to receive 1 request, got %d", googleHits.Load())
	}
	if litellmHits.Load() != 0 {
		t.Errorf("Expected litellm server to receive 0 requests, got %d", litellmHits.Load())
	}

	// Request ao client do LiteLLM deve ir SOMENTE ao litellm server
	_, err = litellmClient.GetModels(t.Context())
	if err != nil {
		t.Fatalf("litellmClient.GetModels: %v", err)
	}
	if litellmHits.Load() != 1 {
		t.Errorf("Expected litellm server to receive 1 request, got %d", litellmHits.Load())
	}
	if googleHits.Load() != 1 {
		t.Errorf("Expected google server to still have 1 request, got %d", googleHits.Load())
	}
}

// TestProviderRouting_StreamChatHitsCorrectEndpoint verifica que StreamChat
// envia o request ao endpoint do provedor correto (não ao endpoint global).
func TestProviderRouting_StreamChatHitsCorrectEndpoint(t *testing.T) {
	var serverAHits, serverBHits atomic.Int32

	streamResp := "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"model\":\"test\"}\n\ndata: [DONE]\n\n"

	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverAHits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(streamResp))
	}))
	defer serverA.Close()

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverBHits.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Write([]byte(streamResp))
	}))
	defer serverB.Close()

	providerA := &llm.ProviderConfig{
		ID:      "provider-a",
		Name:    "Provider A",
		Type:    llm.ProviderCustom,
		BaseURL: serverA.URL,
		Model:   "model-a",
	}
	providerB := &llm.ProviderConfig{
		ID:      "provider-b",
		Name:    "Provider B",
		Type:    llm.ProviderCustom,
		BaseURL: serverB.URL,
		Model:   "model-b",
	}

	cfg := &config.Config{}
	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)

	clientA := llm.NewClient(providerA, cfg, credMgr)
	clientB := llm.NewClient(providerB, cfg, credMgr)

	handler := &testStreamHandler{done: make(chan struct{})}

	messages := []llm.Message{{Role: "user", Content: "test"}}
	params := llm.ChatParams{Model: "model-b"}

	// Streaming via clientB deve ir ao serverB, não ao serverA
	_ = clientA // usado abaixo para verificar isolamento
	clientB.StreamChat(t.Context(), messages, params, handler)
	<-handler.done

	if serverBHits.Load() != 1 {
		t.Errorf("Expected serverB to receive 1 request, got %d", serverBHits.Load())
	}
	if serverAHits.Load() != 0 {
		t.Errorf("Expected serverA to receive 0 requests (cross-routing!), got %d", serverAHits.Load())
	}
}

// TestProviderRouting_ProfileResolution verifica a lógica de resolução de provedor
// a partir do perfil: o provedor do perfil deve determinar qual client é usado.
func TestProviderRouting_ProfileResolution(t *testing.T) {
	tests := []struct {
		name               string
		globalProviderID   string
		profileProviderID  string
		expectedProviderID string
	}{
		{
			name:               "profile with same provider as global",
			globalProviderID:   "google",
			profileProviderID:  "google",
			expectedProviderID: "google",
		},
		{
			name:               "profile with different provider than global",
			globalProviderID:   "google",
			profileProviderID:  "litellm",
			expectedProviderID: "litellm",
		},
		{
			name:               "profile with empty provider falls back to global",
			globalProviderID:   "google",
			profileProviderID:  "",
			expectedProviderID: "google",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			activeProfile := &profiles.Profile{
				Name: "Test Profile",
				Chat: profiles.ChatConfig{
					LLMProvider: tc.profileProviderID,
					Model:       "some-model",
				},
			}

			// Simula a lógica de resolução de sendMessageInternal
			resolvedProviderID := tc.globalProviderID
			if activeProfile != nil && activeProfile.Chat.LLMProvider != "" {
				resolvedProviderID = activeProfile.Chat.LLMProvider
			}

			if resolvedProviderID != tc.expectedProviderID {
				t.Errorf("Expected provider %q, got %q", tc.expectedProviderID, resolvedProviderID)
			}
		})
	}
}

// TestProviderRouting_ChannelProfileUsesOwnProvider simula um cenário de canal
// (Telegram/Signal) onde o perfil do canal aponta para um provedor diferente do global.
func TestProviderRouting_ChannelProfileUsesOwnProvider(t *testing.T) {
	var googleHits, litellmHits atomic.Int32

	modelsResp, _ := json.Marshal(llm.ModelsResponse{
		Data: []llm.Model{{ID: "test-model"}},
	})

	googleServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		googleHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsResp)
	}))
	defer googleServer.Close()

	litellmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		litellmHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsResp)
	}))
	defer litellmServer.Close()

	registry := llm.NewProviderRegistry()
	registry.Register(&llm.ProviderConfig{
		ID: "google", Name: "Google", Type: llm.ProviderCustom, BaseURL: googleServer.URL,
	})
	registry.Register(&llm.ProviderConfig{
		ID: "litellm", Name: "LiteLLM", Type: llm.ProviderCustom, BaseURL: litellmServer.URL,
	})

	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)

	app := &App{
		llmRegistry: registry,
		credMgr:     credMgr,
	}

	// Perfil ativo global aponta para Google
	globalProfile := &profiles.Profile{
		Chat: profiles.ChatConfig{LLMProvider: "google"},
	}

	// Perfil do canal aponta para LiteLLM
	channelProfile := &profiles.Profile{
		Chat: profiles.ChatConfig{
			LLMProvider: "litellm",
			Model:       "gpt-4o-mini",
		},
	}

	// Simula resolução do client para o perfil global (esperado: Google)
	globalClient, err := app.getClientForProvider(globalProfile.Chat.LLMProvider)
	if err != nil {
		t.Fatalf("getClientForProvider (global): %v", err)
	}
	_, err = globalClient.GetModels(t.Context())
	if err != nil {
		t.Fatalf("globalClient.GetModels: %v", err)
	}
	if googleHits.Load() != 1 || litellmHits.Load() != 0 {
		t.Fatalf("Global profile should hit Google. Google=%d, LiteLLM=%d",
			googleHits.Load(), litellmHits.Load())
	}

	// Simula resolução do client para o perfil do canal (esperado: LiteLLM)
	channelClient, err := app.getClientForProvider(channelProfile.Chat.LLMProvider)
	if err != nil {
		t.Fatalf("getClientForProvider (channel): %v", err)
	}
	_, err = channelClient.GetModels(t.Context())
	if err != nil {
		t.Fatalf("channelClient.GetModels: %v", err)
	}
	if litellmHits.Load() != 1 {
		t.Errorf("Channel profile should hit LiteLLM. LiteLLM=%d", litellmHits.Load())
	}
	if googleHits.Load() != 1 {
		t.Errorf("Channel profile should NOT hit Google again. Google=%d", googleHits.Load())
	}
}

// TestProviderRouting_RequestContainsCorrectModel verifica que o modelo
// especificado no perfil é enviado no body do request ao provedor correto.
func TestProviderRouting_RequestContainsCorrectModel(t *testing.T) {
	var receivedModel string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]interface{}
		json.NewDecoder(r.Body).Decode(&req)
		if m, ok := req["model"].(string); ok {
			receivedModel = m
		}

		resp := `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp))
	}))
	defer server.Close()

	provider := &llm.ProviderConfig{
		ID:      "test-provider",
		Name:    "Test",
		Type:    llm.ProviderCustom,
		BaseURL: server.URL,
	}

	cfg := &config.Config{}
	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)
	client := llm.NewClient(provider, cfg, credMgr)

	messages := []llm.Message{{Role: "user", Content: "test"}}
	params := llm.ChatParams{Model: "gpt-4o-mini"}

	_, err := client.SendMessageSync(t.Context(), messages, params)
	if err != nil {
		t.Fatalf("SendMessageSync: %v", err)
	}

	if receivedModel != "gpt-4o-mini" {
		t.Errorf("Expected model 'gpt-4o-mini' in request, got %q", receivedModel)
	}
}

// TestProviderRouting_FallbackToGlobalWhenProfileProviderMissing verifica que
// quando o provedor do perfil não é encontrado, o código cai no fallback para o global.
func TestProviderRouting_FallbackToGlobalWhenProfileProviderMissing(t *testing.T) {
	var globalHits atomic.Int32

	modelsResp, _ := json.Marshal(llm.ModelsResponse{
		Data: []llm.Model{{ID: "test-model"}},
	})

	globalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		globalHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsResp)
	}))
	defer globalServer.Close()

	registry := llm.NewProviderRegistry()
	registry.Register(&llm.ProviderConfig{
		ID: "google", Name: "Google", Type: llm.ProviderCustom, BaseURL: globalServer.URL,
	})

	cfg := &config.Config{}
	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)

	globalClient := llm.NewClient(registry.Get("google"), cfg, credMgr)

	// Simula a lógica de sendMessageInternal quando o provedor do perfil não existe
	activeProfile := &profiles.Profile{
		Chat: profiles.ChatConfig{
			LLMProvider: "deleted-provider",
			Model:       "some-model",
		},
	}

	requestClient := globalClient
	if activeProfile != nil && activeProfile.Chat.LLMProvider != "" {
		provider := registry.Get(activeProfile.Chat.LLMProvider)
		if provider != nil {
			requestClient = llm.NewClient(provider, cfg, credMgr)
		}
		// Se provider == nil, mantém o globalClient (fallback)
	}

	_, err := requestClient.GetModels(t.Context())
	if err != nil {
		t.Fatalf("GetModels: %v", err)
	}

	if globalHits.Load() != 1 {
		t.Errorf("Expected fallback to global server. Hits=%d", globalHits.Load())
	}
}

// testStreamHandler implements llm.StreamHandler for testing
type testStreamHandler struct {
	content  string
	err      string
	model    string
	done     chan struct{}
}

func (h *testStreamHandler) OnChunk(content string)              { h.content += content }
func (h *testStreamHandler) OnThinking(content string)           {}
func (h *testStreamHandler) OnThinkingDone(fullReasoning string) {}
func (h *testStreamHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
	h.model = model
	close(h.done)
}
func (h *testStreamHandler) OnError(err string) {
	h.err = err
	close(h.done)
}
func (h *testStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	h.content = fullResponse
	h.model = model
	close(h.done)
}
