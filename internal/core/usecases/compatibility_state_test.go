package usecases

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"assistente/internal/credentials"
	"assistente/internal/database"
	"assistente/internal/llm"
	"assistente/internal/llmcapabilities"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestCompatibilityLearningPersistsBeforeRetryAndNextSend(t *testing.T) {
	tests := []struct {
		name          string
		format        llm.APIFormat
		field         llmcapabilities.Capability
		staleRevision bool
	}{
		{name: "chat completions", format: llm.APIFormatOpenAI, field: llmcapabilities.CapabilityChatCompletions},
		{name: "responses", format: llm.APIFormatOpenAIResponses, field: llmcapabilities.CapabilityResponses},
		{name: "chat completions stale revision", format: llm.APIFormatOpenAI, field: llmcapabilities.CapabilityChatCompletions, staleRevision: true},
		{name: "responses stale revision", format: llm.APIFormatOpenAIResponses, field: llmcapabilities.CapabilityResponses, staleRevision: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
			if err != nil {
				t.Fatal(err)
			}
			sqlDB, err := db.DB()
			if err != nil {
				t.Fatal(err)
			}
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { _ = sqlDB.Close() })
			if err := db.Exec("PRAGMA foreign_keys = ON").Error; err != nil {
				t.Fatal(err)
			}
			if err := db.AutoMigrate(&database.LLMProvider{}, &database.CredentialEntry{}); err != nil {
				t.Fatal(err)
			}
			if err := database.MigrateLLMModelCapabilities(db); err != nil {
				t.Fatal(err)
			}

			var attempts int
			var temperatureSent []bool
			ctx := database.WithUserID(context.Background(), "owner")
			var repository *database.LLMModelCapabilitiesRepository
			var modelID string
			const compatibilityRevision = 1

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts++
				if test.staleRevision {
					if err := db.Model(&database.LLMProvider{}).Where("id = ?", "phase2-provider").Update("base_url", "https://changed.example/v1").Error; err != nil {
						t.Errorf("alterar revisão antes da persistência: %v", err)
					}
				}
				var payload map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
					t.Errorf("decodificar payload da tentativa %d: %v", attempts, err)
				}
				_, sent := payload["temperature"]
				temperatureSent = append(temperatureSent, sent)

				if attempts == 1 {
					if !sent {
						t.Error("a primeira tentativa deveria enviar temperature")
					}
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusBadRequest)
					responsePayload, _ := json.Marshal(map[string]any{"error": map[string]any{"message": "temperature is not supported", "type": "invalid_request_error", "param": "temperature", "code": "unsupported_parameter"}})
					_, _ = w.Write(responsePayload)
					return
				}

				if sent {
					t.Errorf("tentativa %d enviou temperature após aprender a restrição", attempts)
				}
				fields, err := repository.ListUnsupportedFieldsForRevision(ctx, modelID, compatibilityRevision)
				if err != nil {
					t.Errorf("ler restrição persistida antes da tentativa %d: %v", attempts, err)
				} else if len(fields) != 1 || fields[0].CapabilityCode != string(test.field) ||
					fields[0].FieldCode != string(llmcapabilities.FieldTemperature) {
					t.Errorf("restrição não estava persistida antes da tentativa %d: %+v", attempts, fields)
				}

				w.Header().Set("Content-Type", "application/json")
				if test.format == llm.APIFormatOpenAIResponses {
					responsePayload, _ := json.Marshal(map[string]any{"id": "resp-test", "object": "response", "created_at": 1700000000, "status": "completed", "model": "gpt-test", "output": []any{map[string]any{"id": "msg-test", "type": "message", "status": "completed", "role": "assistant", "content": []any{map[string]any{"type": "output_text", "text": "ok", "annotations": []any{}}}}}})
					_, _ = w.Write(responsePayload)
					return
				}
				responsePayload, _ := json.Marshal(map[string]any{"id": "chatcmpl-test", "object": "chat.completion", "created": 1700000000, "model": "gpt-test", "choices": []any{map[string]any{"index": 0, "message": map[string]any{"role": "assistant", "content": "ok"}, "finish_reason": "stop"}}})
				_, _ = w.Write(responsePayload)
			}))
			defer server.Close()

			providerRow := &database.LLMProvider{
				ID: "phase2-provider", UserID: "owner", Name: "OpenAI", Type: string(llm.ProviderOpenAI),
				ErrorFamily: string(llm.ProviderErrorFamilyOpenAI),
				BaseURL:     server.URL + "/v1", APIFormat: string(test.format),
				CompatibilityRevision: compatibilityRevision,
			}
			if err := database.NewProviderRepository(db).SaveLLMProvider(ctx, providerRow); err != nil {
				t.Fatal(err)
			}
			repository = database.NewLLMModelCapabilitiesRepository(db)
			model, err := repository.SaveModel(ctx, providerRow.ID, "gpt-test", "GPT Test")
			if err != nil {
				t.Fatal(err)
			}
			modelID = model.ID

			uc := NewSendMessageUseCase(SendMessageConfig{ModelCapabilities: repository})
			providerConfig := &llm.ProviderConfig{
				ID: providerRow.ID, Name: providerRow.Name, Type: llm.ProviderOpenAI,
				ErrorFamily: llm.ProviderErrorFamilyOpenAI,
				BaseURL:     providerRow.BaseURL, APIFormat: test.format, AuthMode: llm.AuthModeNone,
				DefaultModel: "gpt-test", CompatibilityRevision: compatibilityRevision,
			}
			var provider llm.ChatProvider
			if test.format == llm.APIFormatOpenAIResponses {
				provider = llm.NewOpenAIResponsesProvider(providerConfig, credentials.NewManager(nil))
			} else {
				provider = llm.NewOpenAIProvider(providerConfig, credentials.NewManager(nil))
			}
			params := llm.ChatParams{Model: "gpt-test", Temperature: testParameterPointer(float64(0.4))}
			params.Compatibility = uc.loadCompatibilityState(ctx, providerConfig, params.Model)
			if params.Compatibility == nil {
				t.Fatal("o use case não criou estado de compatibilidade")
			}
			models, err := repository.ListModels(ctx, providerRow.ID)
			if err != nil || len(models) != 1 || models[0].DisplayName != "GPT Test" {
				t.Fatalf("carregar compatibilidade alterou nome do modelo: %+v, %v", models, err)
			}

			first, err := provider.SendChat(ctx, []llm.Message{{Role: "user", Content: "oi"}}, params)
			if test.staleRevision {
				if err == nil || attempts != 1 || len(temperatureSent) != 1 || !temperatureSent[0] ||
					params.Compatibility.IsUnsupported(test.field, llmcapabilities.FieldTemperature) {
					t.Fatalf("revisão obsoleta: resposta=%q erro=%v tentativas=%d parâmetros=%v", first, err, attempts, temperatureSent)
				}
				var updatedProvider database.LLMProvider
				if err := db.First(&updatedProvider, "id = ?", providerRow.ID).Error; err != nil {
					t.Fatal(err)
				}
				if updatedProvider.CompatibilityRevision != compatibilityRevision+1 {
					t.Fatalf("revisão atual = %d, esperada %d", updatedProvider.CompatibilityRevision, compatibilityRevision+1)
				}
				fields, err := repository.ListUnsupportedFieldsForRevision(ctx, modelID, updatedProvider.CompatibilityRevision)
				if err != nil || len(fields) != 0 {
					t.Fatalf("gravação obsoleta contaminou a revisão nova: campos=%+v err=%v", fields, err)
				}
				return
			}
			if err != nil || first != "ok" {
				t.Fatalf("primeiro envio = %q, %v", first, err)
			}
			if attempts != 2 {
				t.Fatalf("tentativas do primeiro envio = %d, esperado 2", attempts)
			}

			nextParams := llm.ChatParams{Model: "gpt-test", Temperature: testParameterPointer(float64(0.4))}
			nextParams.Compatibility = uc.loadCompatibilityState(ctx, providerConfig, nextParams.Model)
			if nextParams.Compatibility == nil || !nextParams.Compatibility.IsUnsupported(test.field, llmcapabilities.FieldTemperature) {
				t.Fatal("envio seguinte não carregou a restrição do banco")
			}
			next, err := provider.SendChat(ctx, []llm.Message{{Role: "user", Content: "oi de novo"}}, nextParams)
			if err != nil || next != "ok" {
				t.Fatalf("envio seguinte = %q, %v", next, err)
			}
			if attempts != 3 || len(temperatureSent) != 3 || !temperatureSent[0] || temperatureSent[1] || temperatureSent[2] {
				t.Fatalf("tentativas=%d temperatureSent=%v; esperado [true false false]", attempts, temperatureSent)
			}
		})
	}
}

func testParameterPointer[T any](value T) *T { return &value }
