package llm

import (
	"encoding/json"
	"errors"
	"testing"

	"assistente/internal/llmcapabilities"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/packages/param"
	"google.golang.org/genai"
)

func TestRecognizeProviderErrorRequiresExplicitSupportedSignature(t *testing.T) {
	tests := []struct {
		name         string
		providerType ProviderType
		format       APIFormat
		apiErr       error
		wantCategory ProviderErrorCategory
		wantField    llmcapabilities.FieldKey
		wantCode     string
		wantType     string
	}{
		{
			name:         "OpenAI explicit unsupported parameter",
			providerType: ProviderOpenAI,
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Code: "unsupported_parameter", Type: "invalid_request_error", Param: "max_completion_tokens"},
			wantCategory: ProviderErrorUnsupportedParameter,
			wantField:    llmcapabilities.FieldMaxOutputTokens,
			wantCode:     "unsupported_parameter",
			wantType:     "invalid_request_error",
		},
		{
			name:         "xAI 400 does not use OpenAI-only unsupported signature",
			providerType: ProviderGrok,
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Code: "unsupported_parameter", Param: "temperature"},
			wantCategory: ProviderErrorInvalidRequest,
		},
		{
			name:         "contradictory authentication type rejects unsupported parameter code",
			providerType: ProviderOpenAI,
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Code: "unsupported_parameter", Type: "authentication_error", Param: "temperature"},
			wantCategory: ProviderErrorAuthentication,
		},
		{
			name:         "Responses API uses its exact output token parameter",
			providerType: ProviderOpenAI,
			format:       APIFormatOpenAIResponses,
			apiErr:       &openai.Error{StatusCode: 400, Code: "unsupported_parameter", Type: "invalid_request_error", Param: "max_output_tokens"},
			wantCategory: ProviderErrorUnsupportedParameter,
			wantField:    llmcapabilities.FieldMaxOutputTokens,
			wantCode:     "unsupported_parameter",
			wantType:     "invalid_request_error",
		},
		{
			name:         "Responses API does not infer output field from chat alias",
			providerType: ProviderOpenAI,
			format:       APIFormatOpenAIResponses,
			apiErr:       &openai.Error{StatusCode: 400, Code: "unsupported_parameter", Param: "max_tokens"},
			wantCategory: ProviderErrorUnsupportedParameter,
			wantCode:     "unsupported_parameter",
		},
		{
			name:         "LiteLLM explicit error type",
			providerType: ProviderType("litellm"),
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Type: "UnsupportedParamsError", Param: "temperature"},
			wantCategory: ProviderErrorUnsupportedParameter,
			wantField:    llmcapabilities.FieldTemperature,
			wantType:     "unsupportedparamserror",
		},
		{
			name:         "LiteLLM documented exception name in JSON message",
			providerType: ProviderType("litellm"),
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Code: "400", Param: "temperature", Message: "litellm.UnsupportedParamsError: unsupported parameter"},
			wantCategory: ProviderErrorUnsupportedParameter,
			wantField:    llmcapabilities.FieldTemperature,
			wantType:     "unsupportedparamserror",
		},
		{
			name:         "LiteLLM message prefix cannot override a conflicting error type",
			providerType: ProviderType("litellm"),
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Param: "temperature", Type: "authentication_error", Message: "litellm.UnsupportedParamsError: unsupported parameter"},
			wantCategory: ProviderErrorAuthentication,
			wantType:     "authentication_error",
		},
		{
			name:         "LiteLLM unsupported type with conflicting code is not learned",
			providerType: ProviderType("litellm"),
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Code: "invalid_api_key", Type: "UnsupportedParamsError", Param: "temperature"},
			wantCategory: ProviderErrorAuthentication,
		},
		{
			name:         "unsupported value is not field support",
			providerType: ProviderOpenAI,
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Code: "unsupported_value", Param: "temperature"},
			wantCategory: ProviderErrorInvalidValue,
			wantField:    llmcapabilities.FieldTemperature,
			wantCode:     "unsupported_value",
		},
		{
			name:         "permission response is not an invalid value",
			providerType: ProviderOpenAI,
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 403, Code: "unsupported_value", Param: "temperature"},
			wantCategory: ProviderErrorPermission,
			wantCode:     "unsupported_value",
		},
		{
			name:         "generic HTTP 400 never learns",
			providerType: ProviderOpenAI,
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Type: "invalid_request_error", Param: "temperature"},
			wantCategory: ProviderErrorInvalidRequest,
			wantType:     "invalid_request_error",
		},
		{
			name:         "unknown param is discarded",
			providerType: ProviderOpenAI,
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Code: "unsupported_parameter", Param: "customer_secret"},
			wantCategory: ProviderErrorUnsupportedParameter,
			wantCode:     "unsupported_parameter",
		},
		{
			name:         "unknown code is discarded",
			providerType: ProviderOpenAI,
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Code: "customer supplied free text", Type: "invalid_request_error", Param: "temperature"},
			wantCategory: ProviderErrorInvalidRequest,
			wantType:     "invalid_request_error",
		},
		{
			name:         "LocalAI generic 400 remains ambiguous",
			providerType: ProviderLocalAI,
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Code: "400", Type: "invalid_request_error", Param: "temperature"},
			wantCategory: ProviderErrorInvalidRequest,
			wantType:     "invalid_request_error",
		},
		{
			name:         "ZAI numeric invalid request remains ambiguous",
			providerType: ProviderType("zai"),
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Code: "1210"},
			wantCategory: ProviderErrorInvalidRequest,
		},
		{
			name:         "OpenRouter generic 400 remains ambiguous",
			providerType: ProviderType("openrouter"),
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 400, Code: "400", Type: "invalid_request_error"},
			wantCategory: ProviderErrorInvalidRequest,
			wantType:     "invalid_request_error",
		},
		{
			name:         "model missing is not a generic invalid request",
			providerType: ProviderOpenAI,
			format:       APIFormatOpenAI,
			apiErr:       &openai.Error{StatusCode: 404, Type: "invalid_request_error"},
			wantCategory: ProviderErrorNotFound,
			wantType:     "invalid_request_error",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			provider := &ProviderConfig{Type: test.providerType, APIFormat: test.format, BaseURL: "https://provider.invalid/v1"}
			got, ok := RecognizeProviderError(provider, test.apiErr)
			if !ok {
				t.Fatal("RecognizeProviderError() did not recognize the typed API envelope")
			}
			if got.Category != test.wantCategory || got.Field != test.wantField || got.Code != test.wantCode || got.Type != test.wantType {
				t.Fatalf("RecognizeProviderError() = %#v", got)
			}
			if got.Param != "" && got.Field == "" {
				t.Fatalf("unmapped param leaked into normalized error: %#v", got)
			}
		})
	}
}

func TestChatCompletionOutputTokenAliasesAreComparedExactlyAndNeverPersistedBroadly(t *testing.T) {
	legacy := openai.ChatCompletionNewParams{MaxTokens: param.NewOpt(int64(64))}
	completion := openai.ChatCompletionNewParams{MaxCompletionTokens: param.NewOpt(int64(64))}
	legacyError := ProviderError{Field: llmcapabilities.FieldMaxOutputTokens, Param: "max_tokens"}
	completionError := ProviderError{Field: llmcapabilities.FieldMaxOutputTokens, Param: "max_completion_tokens"}

	if !openAIChatProviderErrorParamWasSent(&legacy, legacyError) || openAIChatProviderErrorParamWasSent(&legacy, completionError) {
		t.Fatal("legacy token parameter was not matched to its exact outgoing alias")
	}
	if !openAIChatProviderErrorParamWasSent(&completion, completionError) || openAIChatProviderErrorParamWasSent(&completion, legacyError) {
		t.Fatal("completion token parameter was not matched to its exact outgoing alias")
	}
	if openAIChatCompatibilityFieldWasSent(&legacy, legacyError) || openAIChatCompatibilityFieldWasSent(&completion, completionError) {
		t.Fatal("ambiguous token aliases were eligible for broad persistent suppression")
	}
}

func TestProviderErrorDisplayNamesExactSentAlias(t *testing.T) {
	err := ProviderError{
		Category: ProviderErrorUnsupportedParameter,
		Field:    llmcapabilities.FieldMaxOutputTokens,
		Param:    "max_completion_tokens",
	}
	if message := err.DisplayMessage(); message != "provider_error:v1:unsupported_parameter:max_completion_tokens" {
		t.Fatalf("DisplayMessage() = %q, want a stable, sanitized translation marker", message)
	}
}

func TestProviderErrorDisplayMessageOmitsUnknownParameterAndBoundsStatus(t *testing.T) {
	err := ProviderError{Category: ProviderErrorUnsupportedParameter, Param: "customer_secret", StatusCode: 700}
	if got, want := err.DisplayMessage(), "provider_error:v1:unsupported_parameter:"; got != want {
		t.Fatalf("DisplayMessage() = %q, want %q", got, want)
	}
}

func TestRecognizeProviderErrorNormalizesAnthropicAndGoogleWithoutLearningFrom400(t *testing.T) {
	var anthropicError anthropic.Error
	if err := json.Unmarshal([]byte(`{"type":"error","error":{"type":"invalid_request_error","message":"raw provider message"}}`), &anthropicError); err != nil {
		t.Fatal(err)
	}
	anthropicError.StatusCode = 400
	gotAnthropic, ok := RecognizeProviderError(&ProviderConfig{Type: ProviderClaude, APIFormat: APIFormatAnthropic}, &anthropicError)
	if !ok || gotAnthropic.Category != ProviderErrorInvalidRequest || gotAnthropic.Field != "" || gotAnthropic.Param != "" {
		t.Fatalf("Anthropic error classification = %#v, recognized=%v", gotAnthropic, ok)
	}
	if gotAnthropic.DisplayMessage() == "" || gotAnthropic.DisplayMessage() == "raw provider message" {
		t.Fatalf("Anthropic display message leaked or was empty: %q", gotAnthropic.DisplayMessage())
	}

	googleError := genai.APIError{Code: 400, Status: "INVALID_ARGUMENT", Message: "raw provider message"}
	gotGoogle, ok := RecognizeProviderError(&ProviderConfig{Type: ProviderType("google"), APIFormat: APIFormatGoogle}, googleError)
	if !ok || gotGoogle.Category != ProviderErrorInvalidRequest || gotGoogle.Field != "" || gotGoogle.Param != "" {
		t.Fatalf("Google error classification = %#v, recognized=%v", gotGoogle, ok)
	}
	if gotGoogle.DisplayMessage() == "" || gotGoogle.DisplayMessage() == "raw provider message" {
		t.Fatalf("Google display message leaked or was empty: %q", gotGoogle.DisplayMessage())
	}
}

func TestRecognizeProviderErrorRejectsUnknownFormatsAndUnstructuredErrors(t *testing.T) {
	if got, ok := RecognizeProviderError(&ProviderConfig{Type: ProviderType("custom"), APIFormat: APIFormat("unknown")}, errors.New("unsupported parameter")); ok || got.Category != "" {
		t.Fatalf("unstructured error classification = %#v, recognized=%v", got, ok)
	}
	if got, ok := RecognizeProviderError(nil, &openai.Error{StatusCode: 400}); ok || got.Category != "" {
		t.Fatalf("nil-provider classification = %#v, recognized=%v", got, ok)
	}
}
