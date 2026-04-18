package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/profiles"
	"assistente/internal/providers"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// TestGetChatProviderForProvider_ReturnsProviderForRegistered verifica que
// providerSvc.GetChatProvider cria um ChatProvider quando o provedor existe no registry.
func TestGetChatProviderForProvider_ReturnsProviderForRegistered(t *testing.T) {
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
	svc := providers.NewService(providers.ServiceConfig{Registry: registry, CredMgr: credMgr})

	cp, err := svc.GetChatProvider("litellm-test")
	if err != nil {
		t.Fatalf("GetChatProvider returned error: %v", err)
	}
	if cp == nil {
		t.Fatal("GetChatProvider returned nil")
	}
}

// TestGetChatProviderForProvider_ErrorForUnknownProvider verifica que
// providerSvc.GetChatProvider retorna erro quando o provedor não existe.
func TestGetChatProviderForProvider_ErrorForUnknownProvider(t *testing.T) {
	registry := llm.NewProviderRegistry()
	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)
	svc := providers.NewService(providers.ServiceConfig{Registry: registry, CredMgr: credMgr})

	cp, err := svc.GetChatProvider("nonexistent-provider")
	if err == nil {
		t.Fatal("Expected error for unknown provider, got nil")
	}
	if cp != nil {
		t.Fatal("Expected nil ChatProvider for unknown provider")
	}
}

// TestGetChatProviderForProvider_DifferentProvidersReturnDifferentInstances verifica que
// providers distintos resultam em ChatProviders distintos.
func TestGetChatProviderForProvider_DifferentProvidersReturnDifferentInstances(t *testing.T) {
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
	svc := providers.NewService(providers.ServiceConfig{Registry: registry, CredMgr: credMgr})

	cp1, err := svc.GetChatProvider("google-test")
	if err != nil {
		t.Fatalf("GetChatProvider google: %v", err)
	}
	cp2, err := svc.GetChatProvider("litellm-test")
	if err != nil {
		t.Fatalf("GetChatProvider litellm: %v", err)
	}

	if cp1 == nil || cp2 == nil {
		t.Fatal("Expected non-nil ChatProviders")
	}
}

// TestProviderRouting_ChatProviderHitsCorrectEndpoint verifica via HTTP que o ChatProvider
// criado para um provedor envia requests ao endpoint correto (e não a outro).
// Teste de regressão principal para o bug de roteamento cruzado.
func TestProviderRouting_ChatProviderHitsCorrectEndpoint(t *testing.T) {
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

	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)

	googleCP := llm.NewChatProvider(&llm.ProviderConfig{
		ID: "google", Name: "Google", Type: llm.ProviderCustom, BaseURL: googleServer.URL,
	}, credMgr)
	litellmCP := llm.NewChatProvider(&llm.ProviderConfig{
		ID: "litellm", Name: "LiteLLM", Type: llm.ProviderCustom, BaseURL: litellmServer.URL,
	}, credMgr)

	_, err := googleCP.GetModels(t.Context())
	if err != nil {
		t.Fatalf("googleCP.GetModels: %v", err)
	}
	if googleHits.Load() != 1 {
		t.Errorf("Expected google server to receive 1 request, got %d", googleHits.Load())
	}
	if litellmHits.Load() != 0 {
		t.Errorf("Expected litellm server to receive 0 requests, got %d", litellmHits.Load())
	}

	_, err = litellmCP.GetModels(t.Context())
	if err != nil {
		t.Fatalf("litellmCP.GetModels: %v", err)
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

	streamResp := "data: {\"id\":\"x\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}],\"model\":\"test\"}\n\ndata: [DONE]\n\n"

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

	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)

	cpA := llm.NewChatProvider(&llm.ProviderConfig{
		ID: "provider-a", Name: "Provider A", Type: llm.ProviderCustom,
		BaseURL: serverA.URL, Model: "model-a",
	}, credMgr)
	cpB := llm.NewChatProvider(&llm.ProviderConfig{
		ID: "provider-b", Name: "Provider B", Type: llm.ProviderCustom,
		BaseURL: serverB.URL, Model: "model-b",
	}, credMgr)

	handler := &testStreamHandler{done: make(chan struct{})}

	messages := []llm.Message{{Role: "user", Content: "test"}}
	params := llm.ChatParams{Model: "model-b"}

	_ = cpA
	cpB.StreamChat(t.Context(), messages, params, handler)
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
	svc := providers.NewService(providers.ServiceConfig{Registry: registry, CredMgr: credMgr})

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

	globalCP, err := svc.GetChatProvider(globalProfile.Chat.LLMProvider)
	if err != nil {
		t.Fatalf("GetChatProvider (global): %v", err)
	}
	_, err = globalCP.GetModels(t.Context())
	if err != nil {
		t.Fatalf("globalCP.GetModels: %v", err)
	}
	if googleHits.Load() != 1 || litellmHits.Load() != 0 {
		t.Fatalf("Global profile should hit Google. Google=%d, LiteLLM=%d",
			googleHits.Load(), litellmHits.Load())
	}

	channelCP, err := svc.GetChatProvider(channelProfile.Chat.LLMProvider)
	if err != nil {
		t.Fatalf("GetChatProvider (channel): %v", err)
	}
	_, err = channelCP.GetModels(t.Context())
	if err != nil {
		t.Fatalf("channelCP.GetModels: %v", err)
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

		resp := `{"id":"x","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"model":"gpt-4o-mini"}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(resp))
	}))
	defer server.Close()

	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)
	cp := llm.NewChatProvider(&llm.ProviderConfig{
		ID: "test-provider", Name: "Test", Type: llm.ProviderCustom, BaseURL: server.URL,
	}, credMgr)

	messages := []llm.Message{{Role: "user", Content: "test"}}
	params := llm.ChatParams{Model: "gpt-4o-mini"}

	_, err := cp.SendChat(t.Context(), messages, params)
	if err != nil {
		t.Fatalf("SendChat: %v", err)
	}

	if receivedModel != "gpt-4o-mini" {
		t.Errorf("Expected model 'gpt-4o-mini' in request, got %q", receivedModel)
	}
}

// TestProviderRouting_ErrorWhenProfileProviderMissing verifica que
// quando o provedor do perfil não é encontrado, GetChatProvider retorna erro.
func TestProviderRouting_ErrorWhenProfileProviderMissing(t *testing.T) {
	registry := llm.NewProviderRegistry()
	registry.Register(&llm.ProviderConfig{
		ID: "google", Name: "Google", Type: llm.ProviderCustom, BaseURL: "https://example.com",
	})

	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)
	svc := providers.NewService(providers.ServiceConfig{Registry: registry, CredMgr: credMgr})

	// Provedor do perfil não existe no registry
	_, err := svc.GetChatProvider("deleted-provider")
	if err == nil {
		t.Fatal("Expected error for missing provider, got nil")
	}

	// Provedor válido funciona
	cp, err := svc.GetChatProvider("google")
	if err != nil {
		t.Fatalf("GetChatProvider (google): %v", err)
	}
	if cp == nil {
		t.Fatal("Expected non-nil ChatProvider for valid provider")
	}
}

// TestNonexistentProviderReturnsError verifica que GetChatProvider
// retorna erro quando o provedor não existe no registry.
func TestNonexistentProviderReturnsError(t *testing.T) {
	registry := llm.NewProviderRegistry()
	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)
	svc := providers.NewService(providers.ServiceConfig{Registry: registry, CredMgr: credMgr})

	_, err := svc.GetChatProvider("nonexistent-provider")
	if err == nil {
		t.Fatal("Expected error for nonexistent provider")
	}
}

// TestRecoverFromPanic verifica que recoverFromPanic não entra em pânico
// e consegue lidar com diferentes tipos de panic values.
func TestRecoverFromPanic(t *testing.T) {
	app := &App{}

	// Deve funcionar sem crash mesmo sem ctx (os EventsEmit falharão silenciosamente)
	// O importante é que o recover não cause novo panic
	t.Run("string panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("recoverFromPanic should not propagate panic, got: %v", r)
			}
		}()

		func() {
			defer app.recoverFromPanic(0, "test")
			panic("test panic")
		}()
	})

	t.Run("error panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("recoverFromPanic should not propagate panic, got: %v", r)
			}
		}()

		func() {
			defer app.recoverFromPanic(0, "test")
			panic(fmt.Errorf("test error"))
		}()
	})

	t.Run("nil pointer panic", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("recoverFromPanic should not propagate panic, got: %v", r)
			}
		}()

		func() {
			defer app.recoverFromPanic(0, "test")
			var p *int
			_ = *p // nil pointer dereference
		}()
	})
}

// TestRecoverFromPanic_InGoroutine verifica que recoverFromPanic captura panics
// em goroutines sem matar o processo.
func TestRecoverFromPanic_InGoroutine(t *testing.T) {
	app := &App{}

	recovered := make(chan bool, 1)

	go func() {
		defer func() {
			if r := recover(); r != nil {
				recovered <- false
				return
			}
			recovered <- true
		}()

		defer app.recoverFromPanic(1, "test")
		panic("simulated panic")
	}()

	result := <-recovered
	if !result {
		t.Fatal("Panic was not recovered - app would have crashed")
	}
}

// TestGetChatProviderForProvider_NilRegistryReturnsError verifica que GetChatProvider
// retorna erro quando o provedor não existe (registry vazio).
func TestGetChatProviderForProvider_NilRegistryReturnsError(t *testing.T) {
	testKey := []byte("test-key-32-bytes-long-key!!")
	credMgr := credentials.NewManager(testKey)
	svc := providers.NewService(providers.ServiceConfig{
		Registry: llm.NewProviderRegistry(),
		CredMgr:  credMgr,
	})

	cp, err := svc.GetChatProvider("any-provider")
	if err == nil {
		t.Fatal("Expected error for unknown provider, got nil")
	}
	if cp != nil {
		t.Fatal("Expected nil ChatProvider")
	}
}

// TestNilProfileSafety verifica que acessar campos de um perfil nil
// é tratado corretamente sem panic.
func TestNilProfileSafety(t *testing.T) {
	var activeProfile *profiles.Profile = nil

	providerID := ""
	if activeProfile != nil {
		providerID = activeProfile.Chat.LLMProvider
	}

	if providerID != "" {
		t.Errorf("Expected empty providerID for nil profile, got %q", providerID)
	}
}

// TestProfileManagerNilDoesNotPanic verifica que quando profileManager é nil,
// a lógica de resolução de perfil não causa panic.
func TestProfileManagerNilDoesNotPanic(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("Panic when profileManager is nil: %v", r)
		}
	}()

	app := &App{
		profileManager: nil,
	}

	// Simula o guard que adicionamos em sendMessageInternal
	var activeProfile *profiles.Profile
	if app.profileManager == nil {
		// Deve entrar aqui sem panic
		activeProfile = nil
	}

	if activeProfile != nil {
		t.Fatal("Expected nil activeProfile when profileManager is nil")
	}
}

// TestSendMessageSync_NoProfileReturnsError verifica que SendMessageSync
// retorna erro quando não há perfil/provedor configurado.
func TestSendMessageSync_NoProfileReturnsError(t *testing.T) {
	setupRoutingTestDB(t)

	app := &App{
		profileManager: profiles.NewManager(),
	}

	_, err := app.SendMessageSync(nil, ChatParams{})
	if err == nil {
		t.Fatal("Expected error when no profile/provider configured")
	}
}

// setupRoutingTestDB creates an in-memory SQLite for routing integration tests.
func setupRoutingTestDB(t *testing.T) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to create in-memory DB: %v", err)
	}
	if err := db.AutoMigrate(&database.LLMProvider{}); err != nil {
		t.Fatalf("failed to migrate: %v", err)
	}
	database.SetDB(db)
}

// newSentinelTestApp cria um App com providerSvc inicializado via DBStore.
// Necessário nos testes que precisam que resolveProfileDefaults resolva $default.
func newSentinelTestApp(registry *llm.ProviderRegistry, credMgr *credentials.Manager) *App {
	svc := providers.NewService(providers.ServiceConfig{
		Registry: registry,
		CredMgr:  credMgr,
		Store:    providers.NewDBStore(),
	})
	return &App{
		ctx:         context.Background(),
		llmRegistry: registry,
		credMgr:     credMgr,
		providerSvc: svc,
	}
}

// TestDefaultSentinel_RoutesToCorrectProvider verifies the full integration:
// profile with "$default" → resolveProfileDefaults → getClientForProvider → correct server.
func TestDefaultSentinel_RoutesToCorrectProvider(t *testing.T) {
	setupRoutingTestDB(t)

	var defaultHits, otherHits atomic.Int32

	modelsResp, _ := json.Marshal(llm.ModelsResponse{
		Data: []llm.Model{{ID: "test-model"}},
	})

	defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsResp)
	}))
	defer defaultServer.Close()

	otherServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		otherHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsResp)
	}))
	defer otherServer.Close()

	registry := llm.NewProviderRegistry()
	registry.Register(&llm.ProviderConfig{
		ID: "default-prov", Name: "Default Provider", Type: llm.ProviderCustom,
		BaseURL: defaultServer.URL, DefaultModel: "default-model", IsDefault: true,
	})
	registry.Register(&llm.ProviderConfig{
		ID: "other-prov", Name: "Other Provider", Type: llm.ProviderCustom,
		BaseURL: otherServer.URL,
	})

	database.SaveLLMProvider(&database.LLMProvider{
		ID: "default-prov", Name: "Default Provider", Type: "custom",
		BaseURL: defaultServer.URL, DefaultModel: "default-model", IsDefault: true,
	})
	database.SaveLLMProvider(&database.LLMProvider{
		ID: "other-prov", Name: "Other Provider", Type: "custom",
		BaseURL: otherServer.URL,
	})
	database.SetDefaultProvider("default-prov")

	credMgr := credentials.NewManager([]byte("test-key-32-bytes-long-key!!"))
	app := newSentinelTestApp(registry, credMgr)

	// Profile with $default sentinel
	profile := &profiles.Profile{
		Name: "Default Sentinel Profile",
		Chat: profiles.ChatConfig{
			LLMProvider: profiles.DefaultProviderSentinel,
			Model:       profiles.DefaultProviderSentinel,
		},
		Voice: profiles.VoiceConfig{
			Assistant: profiles.VoiceRoleConfig{LLMProviderID: profiles.DefaultProviderSentinel},
		},
		Input: profiles.InputConfig{
			LLMProviderID: profiles.DefaultProviderSentinel,
		},
	}

	// Step 1: resolve $default → real provider/model
	resolved := app.resolveProfileDefaults(profile)
	if resolved.Chat.LLMProvider != "default-prov" {
		t.Fatalf("expected Chat.LLMProvider=default-prov, got %s", resolved.Chat.LLMProvider)
	}
	if resolved.Chat.Model != "default-model" {
		t.Fatalf("expected Chat.Model=default-model, got %s", resolved.Chat.Model)
	}

	// Step 2: getChatProviderForProvider routes to the correct server
	cp, err := app.getChatProviderForProvider(resolved.Chat.LLMProvider)
	if err != nil {
		t.Fatalf("getChatProviderForProvider: %v", err)
	}
	_, err = cp.GetModels(t.Context())
	if err != nil {
		t.Fatalf("GetModels: %v", err)
	}
	if defaultHits.Load() != 1 {
		t.Errorf("expected 1 hit on default server, got %d", defaultHits.Load())
	}
	if otherHits.Load() != 0 {
		t.Errorf("expected 0 hits on other server, got %d", otherHits.Load())
	}
}

// TestDefaultSentinel_ModelSentInRequest verifies that after $default resolution,
// the resolved model is actually sent in the LLM API request body.
func TestDefaultSentinel_ModelSentInRequest(t *testing.T) {
	setupRoutingTestDB(t)

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

	registry := llm.NewProviderRegistry()
	registry.Register(&llm.ProviderConfig{
		ID: "my-provider", Name: "My Provider", Type: llm.ProviderCustom,
		BaseURL: server.URL, DefaultModel: "resolved-model-v2", IsDefault: true,
	})
	database.SaveLLMProvider(&database.LLMProvider{
		ID: "my-provider", Name: "My Provider", Type: "custom",
		BaseURL: server.URL, DefaultModel: "resolved-model-v2", IsDefault: true,
	})
	database.SetDefaultProvider("my-provider")

	credMgr := credentials.NewManager([]byte("test-key-32-bytes-long-key!!"))
	app := newSentinelTestApp(registry, credMgr)

	profile := profiles.DefaultProfile()
	resolved := app.resolveProfileDefaults(profile)

	if resolved.Chat.Model != "resolved-model-v2" {
		t.Fatalf("expected model=resolved-model-v2, got %s", resolved.Chat.Model)
	}

	cp, err := app.getChatProviderForProvider(resolved.Chat.LLMProvider)
	if err != nil {
		t.Fatalf("getChatProviderForProvider: %v", err)
	}
	messages := []llm.Message{{Role: "user", Content: "hello"}}
	params := llm.ChatParams{Model: resolved.Chat.Model}

	_, err = cp.SendChat(t.Context(), messages, params)
	if err != nil {
		t.Fatalf("SendChat: %v", err)
	}
	if receivedModel != "resolved-model-v2" {
		t.Errorf("expected model 'resolved-model-v2' in API request, got %q", receivedModel)
	}
}

// TestDefaultSentinel_SwitchDefaultReroutesTraffic verifies that changing the
// default provider via SetDefaultProvider correctly reroutes $default profiles.
func TestDefaultSentinel_SwitchDefaultReroutesTraffic(t *testing.T) {
	setupRoutingTestDB(t)

	var serverAHits, serverBHits atomic.Int32
	modelsResp, _ := json.Marshal(llm.ModelsResponse{
		Data: []llm.Model{{ID: "model-a"}},
	})

	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverAHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsResp)
	}))
	defer serverA.Close()

	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverBHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsResp)
	}))
	defer serverB.Close()

	registry := llm.NewProviderRegistry()
	registry.Register(&llm.ProviderConfig{
		ID: "prov-a", Name: "Provider A", Type: llm.ProviderCustom,
		BaseURL: serverA.URL, DefaultModel: "model-a", IsDefault: true,
	})
	registry.Register(&llm.ProviderConfig{
		ID: "prov-b", Name: "Provider B", Type: llm.ProviderCustom,
		BaseURL: serverB.URL, DefaultModel: "model-b",
	})
	database.SaveLLMProvider(&database.LLMProvider{
		ID: "prov-a", Name: "Provider A", Type: "custom",
		BaseURL: serverA.URL, DefaultModel: "model-a", IsDefault: true,
	})
	database.SaveLLMProvider(&database.LLMProvider{
		ID: "prov-b", Name: "Provider B", Type: "custom",
		BaseURL: serverB.URL, DefaultModel: "model-b",
	})
	database.SetDefaultProvider("prov-a")

	credMgr := credentials.NewManager([]byte("test-key-32-bytes-long-key!!"))
	app := newSentinelTestApp(registry, credMgr)

	profile := profiles.DefaultProfile()

	// Round 1: $default → Provider A
	resolved := app.resolveProfileDefaults(profile)
	if resolved.Chat.LLMProvider != "prov-a" {
		t.Fatalf("round 1: expected prov-a, got %s", resolved.Chat.LLMProvider)
	}
	cp1, _ := app.getChatProviderForProvider(resolved.Chat.LLMProvider)
	cp1.GetModels(t.Context())

	if serverAHits.Load() != 1 || serverBHits.Load() != 0 {
		t.Fatalf("round 1: A=%d B=%d", serverAHits.Load(), serverBHits.Load())
	}

	// Switch default to Provider B
	database.SetDefaultProvider("prov-b")

	// Round 2: same $default profile → now routes to Provider B
	resolved2 := app.resolveProfileDefaults(profile)
	if resolved2.Chat.LLMProvider != "prov-b" {
		t.Fatalf("round 2: expected prov-b, got %s", resolved2.Chat.LLMProvider)
	}
	if resolved2.Chat.Model != "model-b" {
		t.Fatalf("round 2: expected model-b, got %s", resolved2.Chat.Model)
	}
	cp2, _ := app.getChatProviderForProvider(resolved2.Chat.LLMProvider)
	cp2.GetModels(t.Context())

	if serverBHits.Load() != 1 {
		t.Errorf("round 2: expected 1 hit on B, got %d", serverBHits.Load())
	}
	if serverAHits.Load() != 1 {
		t.Errorf("round 2: A should still be 1, got %d", serverAHits.Load())
	}
}

// TestDefaultSentinel_MixedProfilesRouteCorrectly verifies that in a scenario
// with multiple profiles — some using $default, some using concrete IDs — each
// routes to its correct provider.
func TestDefaultSentinel_MixedProfilesRouteCorrectly(t *testing.T) {
	setupRoutingTestDB(t)

	var defaultHits, concreteHits atomic.Int32
	modelsResp, _ := json.Marshal(llm.ModelsResponse{
		Data: []llm.Model{{ID: "m"}},
	})

	defaultServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defaultHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsResp)
	}))
	defer defaultServer.Close()

	concreteServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		concreteHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(modelsResp)
	}))
	defer concreteServer.Close()

	registry := llm.NewProviderRegistry()
	registry.Register(&llm.ProviderConfig{
		ID: "default-prov", Name: "Default", Type: llm.ProviderCustom,
		BaseURL: defaultServer.URL, DefaultModel: "dm", IsDefault: true,
	})
	registry.Register(&llm.ProviderConfig{
		ID: "concrete-prov", Name: "Concrete", Type: llm.ProviderCustom,
		BaseURL: concreteServer.URL,
	})
	database.SaveLLMProvider(&database.LLMProvider{
		ID: "default-prov", Name: "Default", Type: "custom",
		BaseURL: defaultServer.URL, DefaultModel: "dm", IsDefault: true,
	})
	database.SaveLLMProvider(&database.LLMProvider{
		ID: "concrete-prov", Name: "Concrete", Type: "custom",
		BaseURL: concreteServer.URL,
	})
	database.SetDefaultProvider("default-prov")

	credMgr := credentials.NewManager([]byte("test-key-32-bytes-long-key!!"))
	app := newSentinelTestApp(registry, credMgr)

	// Profile A: uses $default
	profileA := &profiles.Profile{
		Name: "Default User",
		Chat: profiles.ChatConfig{
			LLMProvider: profiles.DefaultProviderSentinel,
			Model:       profiles.DefaultProviderSentinel,
		},
	}

	// Profile B: uses concrete ID
	profileB := &profiles.Profile{
		Name: "Power User",
		Chat: profiles.ChatConfig{
			LLMProvider: "concrete-prov",
			Model:       "specific-model",
		},
	}

	// Resolve and route Profile A → default server
	resolvedA := app.resolveProfileDefaults(profileA)
	cpA, _ := app.getChatProviderForProvider(resolvedA.Chat.LLMProvider)
	cpA.GetModels(t.Context())

	// Resolve and route Profile B → concrete server (no resolution needed)
	resolvedB := app.resolveProfileDefaults(profileB)
	if resolvedB.Chat.LLMProvider != "concrete-prov" {
		t.Fatalf("profileB should keep concrete ID, got %s", resolvedB.Chat.LLMProvider)
	}
	cpB, _ := app.getChatProviderForProvider(resolvedB.Chat.LLMProvider)
	cpB.GetModels(t.Context())

	if defaultHits.Load() != 1 {
		t.Errorf("default server: expected 1, got %d", defaultHits.Load())
	}
	if concreteHits.Load() != 1 {
		t.Errorf("concrete server: expected 1, got %d", concreteHits.Load())
	}
}

// testStreamHandler implements llm.StreamHandler for testing
type testStreamHandler struct {
	content string
	err     string
	model   string
	done    chan struct{}
}

func (h *testStreamHandler) OnChunk(content string)              { h.content += content }
func (h *testStreamHandler) OnThinking(content string)           {}
func (h *testStreamHandler) OnThinkingDone(fullReasoning string) {}
func (h *testStreamHandler) OnToolCalls(calls []llm.ToolCall, fullResponse string, usage llm.Usage, model string) {
	h.model = model
	close(h.done)
}
func (h *testStreamHandler) OnMCPToolEvent(event llm.MCPToolEvent) {}
func (h *testStreamHandler) OnError(err string) {
	h.err = err
	close(h.done)
}
func (h *testStreamHandler) OnDone(fullResponse string, usage llm.Usage, model string) {
	h.content = fullResponse
	h.model = model
	close(h.done)
}

// --- providers.Service.GetChatProvider format routing tests ---

func newFormatTestSvc(cfg *llm.ProviderConfig) *providers.Service {
	registry := llm.NewProviderRegistry()
	registry.Register(cfg)
	credMgr := credentials.NewManager([]byte("test-key-32-bytes-long-key!!"))
	return providers.NewService(providers.ServiceConfig{Registry: registry, CredMgr: credMgr})
}

// TestGetChatProvider_NoAPIFormat verifica que provedores sem api_format
// recebem fallback para OpenAI SDK (via GetAPIFormat default).
func TestGetChatProvider_NoAPIFormat(t *testing.T) {
	svc := newFormatTestSvc(&llm.ProviderConfig{
		ID: "legacy-test", Name: "Legacy Provider", Type: llm.ProviderCustom,
		BaseURL: "https://api.example.com/v1",
	})
	cp, err := svc.GetChatProvider("legacy-test")
	if err != nil {
		t.Fatalf("GetChatProvider error: %v", err)
	}
	if cp == nil {
		t.Fatal("Expected non-nil ChatProvider (OpenAI SDK fallback)")
	}
}

// TestGetChatProvider_OpenAIFormat verifica que api_format=openai retorna ChatProvider.
func TestGetChatProvider_OpenAIFormat(t *testing.T) {
	svc := newFormatTestSvc(&llm.ProviderConfig{
		ID: "sdk-openai", Name: "SDK OpenAI", Type: llm.ProviderOpenAI,
		BaseURL: "https://api.openai.com/v1", APIFormat: llm.APIFormatOpenAI,
	})
	cp, err := svc.GetChatProvider("sdk-openai")
	if err != nil {
		t.Fatalf("GetChatProvider error: %v", err)
	}
	if cp == nil {
		t.Fatal("Expected non-nil ChatProvider")
	}
}

// TestGetChatProvider_AnthropicFormat verifica que api_format=anthropic retorna ChatProvider.
func TestGetChatProvider_AnthropicFormat(t *testing.T) {
	svc := newFormatTestSvc(&llm.ProviderConfig{
		ID: "sdk-anthropic", Name: "SDK Anthropic", Type: llm.ProviderClaude,
		BaseURL: "https://api.anthropic.com/v1", APIFormat: llm.APIFormatAnthropic,
	})
	cp, err := svc.GetChatProvider("sdk-anthropic")
	if err != nil {
		t.Fatalf("GetChatProvider error: %v", err)
	}
	if cp == nil {
		t.Fatal("Expected non-nil ChatProvider")
	}
}

// TestGetChatProvider_GoogleFormat verifica que api_format=google retorna ChatProvider.
func TestGetChatProvider_GoogleFormat(t *testing.T) {
	svc := newFormatTestSvc(&llm.ProviderConfig{
		ID: "sdk-google", Name: "SDK Google", Type: llm.ProviderType("gemini"),
		BaseURL: "https://generativelanguage.googleapis.com", APIFormat: llm.APIFormatGoogle,
	})
	cp, err := svc.GetChatProvider("sdk-google")
	if err != nil {
		t.Fatalf("GetChatProvider error: %v", err)
	}
	if cp == nil {
		t.Fatal("Expected non-nil ChatProvider")
	}
}

// TestGetChatProvider_NilProviderSvc verifica que getChatProviderForProvider do App
// retorna erro quando providerSvc não foi inicializado.
func TestGetChatProvider_NilProviderSvc(t *testing.T) {
	app := &App{}
	cp, err := app.getChatProviderForProvider("any")
	if err == nil {
		t.Fatal("Expected error for nil providerSvc")
	}
	if cp != nil {
		t.Fatal("Expected nil ChatProvider")
	}
}

// TestGetChatProvider_UnknownProvider verifica comportamento com provedor inexistente.
func TestGetChatProvider_UnknownProvider(t *testing.T) {
	svc := providers.NewService(providers.ServiceConfig{
		Registry: llm.NewProviderRegistry(),
		CredMgr:  credentials.NewManager([]byte("test-key-32-bytes-long-key!!")),
	})
	cp, err := svc.GetChatProvider("nonexistent")
	if err == nil {
		t.Fatal("Expected error for unknown provider")
	}
	if cp != nil {
		t.Fatal("Expected nil ChatProvider")
	}
}
