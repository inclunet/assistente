package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"

	"assistente/internal/llmcapabilities"
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go"
	"google.golang.org/genai"
)

// ProviderErrorCategory é a classificação sanitizada de um erro de API.
type ProviderErrorCategory string

const (
	ProviderErrorUnsupportedParameter ProviderErrorCategory = "unsupported_parameter"
	ProviderErrorInvalidValue         ProviderErrorCategory = "invalid_value"
	ProviderErrorInvalidRequest       ProviderErrorCategory = "invalid_request"
	ProviderErrorAuthentication       ProviderErrorCategory = "authentication"
	ProviderErrorPermission           ProviderErrorCategory = "permission"
	ProviderErrorQuota                ProviderErrorCategory = "quota_or_rate_limit"
	ProviderErrorTransient            ProviderErrorCategory = "transient"
	ProviderErrorNotFound             ProviderErrorCategory = "not_found"
	ProviderErrorUnrecognized         ProviderErrorCategory = "unrecognized"
)

// ProviderError contém apenas dados allowlisted. Nunca armazena a mensagem ou
// o payload bruto recebido do provedor.
type ProviderError struct {
	Category   ProviderErrorCategory
	StatusCode int
	Recognizer string
	Code       string
	Type       string
	Param      string
	Field      llmcapabilities.FieldKey
}

func (e ProviderError) SanitizedForSentField(sent bool) ProviderError {
	if !sent {
		e.Param = ""
		e.Field = ""
	}
	return e
}

// RecognizeProviderError lê somente envelopes tipados pelos SDKs e campos
// estruturados documentados. Erros não reconhecidos continuam sem inferência.
func RecognizeProviderError(provider *ProviderConfig, err error) (ProviderError, bool) {
	if provider == nil || err == nil {
		return ProviderError{}, false
	}
	switch provider.GetAPIFormat() {
	case APIFormatOpenAI:
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			return recognizeOpenAIError(provider, apiErr.StatusCode, apiErr.Code, apiErr.Type, apiErr.Param)
		}
	case APIFormatOpenAIResponses:
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			return recognizeOpenAIError(provider, apiErr.StatusCode, apiErr.Code, apiErr.Type, apiErr.Param)
		}
	case APIFormatAnthropic:
		var apiErr *anthropic.Error
		if errors.As(err, &apiErr) {
			return recognizeAnthropicError(provider, apiErr)
		}
	case APIFormatGoogle:
		var apiErr genai.APIError
		if errors.As(err, &apiErr) {
			return recognizeGoogleError(provider, apiErr)
		}
	}
	return ProviderError{}, false
}

// ProviderErrorDiagnostic devolve apenas campos allowlisted para logs locais.
// Um erro sem envelope reconhecido recebe assinatura estável sem copiar texto.
func ProviderErrorDiagnostic(provider *ProviderConfig, err error) ProviderError {
	if normalized, ok := RecognizeProviderError(provider, err); ok {
		return normalized
	}
	format := providerErrorFormat(provider)
	category := ProviderErrorUnrecognized
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkError) {
		category = ProviderErrorTransient
	}
	return ProviderError{
		Category:   category,
		Recognizer: "provider-error:" + format + ":" + string(category) + ":v1",
	}
}

func providerErrorFormat(provider *ProviderConfig) string {
	if provider != nil {
		switch provider.GetAPIFormat() {
		case APIFormatOpenAI, APIFormatOpenAIResponses, APIFormatAnthropic, APIFormatGoogle:
			return string(provider.GetAPIFormat())
		}
	}
	return "unknown"
}

func recognizeOpenAIError(provider *ProviderConfig, status int, code, errorType, param string) (ProviderError, bool) {
	family := openAIErrorFamily(provider.Type)
	format := string(provider.GetAPIFormat())
	recognizer := "provider-error:" + family + ":" + format + ":v1"
	code = strings.ToLower(strings.TrimSpace(code))
	errorType = strings.ToLower(strings.TrimSpace(errorType))
	field := canonicalParameterField(provider.GetAPIFormat(), param)
	result := ProviderError{StatusCode: status, Recognizer: recognizer}

	switch {
	case code == "unsupported_parameter" && (family == "openai" || family == "xai") &&
		(status == 0 || status == http.StatusBadRequest) &&
		(errorType == "" || errorType == "invalid_request_error"):
		result.Category = ProviderErrorUnsupportedParameter
		result.Code = code
		result.Type = allowlistedProviderType(errorType)
	case errorType == "unsupportedparamserror" && family == "litellm" &&
		(status == 0 || status == http.StatusBadRequest) && (code == "" || code == "400"):
		result.Category = ProviderErrorUnsupportedParameter
		result.Type = "unsupportedparamserror"
	case (family == "openai" || family == "xai" || family == "litellm") && (status == 0 || status == http.StatusBadRequest) && (code == "unsupported_value" || code == "invalid_value"):
		result.Category = ProviderErrorInvalidValue
		result.Code = code
	case code == "invalid_api_key" || errorType == "authentication_error":
		result.Category = ProviderErrorAuthentication
		result.Code = allowlistedProviderCode(family, code)
		result.Type = allowlistedProviderType(errorType)
	case code == "insufficient_quota" || code == "rate_limit_exceeded" || errorType == "rate_limit_error" || status == http.StatusTooManyRequests:
		result.Category = ProviderErrorQuota
		result.Code = allowlistedProviderCode(family, code)
		result.Type = allowlistedProviderType(errorType)
	case errorType == "permission_error" || status == http.StatusForbidden:
		result.Category = ProviderErrorPermission
		result.Code = allowlistedProviderCode(family, code)
		result.Type = allowlistedProviderType(errorType)
	case errorType == "server_error" || status >= 500:
		result.Category = ProviderErrorTransient
		result.Code = allowlistedProviderCode(family, code)
		result.Type = allowlistedProviderType(errorType)
	case status == http.StatusUnauthorized:
		result.Category = ProviderErrorAuthentication
	case code == "model_not_found" || errorType == "not_found_error" || status == http.StatusNotFound:
		result.Category = ProviderErrorNotFound
		result.Code = allowlistedProviderCode(family, code)
		result.Type = allowlistedProviderType(errorType)
	case errorType == "invalid_request_error" || status == http.StatusBadRequest:
		result.Category = ProviderErrorInvalidRequest
		result.Code = allowlistedProviderCode(family, code)
		result.Type = allowlistedProviderType(errorType)
	default:
		result.Category = ProviderErrorUnrecognized
		result.Code = allowlistedProviderCode(family, code)
		result.Type = allowlistedProviderType(errorType)
	}
	result.Code, result.Type = allowlistedProviderErrorPair(family, status, code, errorType)
	if (result.Category == ProviderErrorUnsupportedParameter || result.Category == ProviderErrorInvalidValue) && field != "" {
		result.Field = field
		result.Param = strings.ToLower(strings.TrimSpace(param))
	}
	result.Recognizer += ":" + string(result.Category)
	return result, true
}

func recognizeAnthropicError(provider *ProviderConfig, apiErr *anthropic.Error) (ProviderError, bool) {
	if apiErr == nil {
		return ProviderError{}, false
	}
	var envelope struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if json.Unmarshal([]byte(apiErr.RawJSON()), &envelope) != nil {
		return ProviderError{
			Category:   ProviderErrorUnrecognized,
			StatusCode: apiErr.StatusCode,
			Recognizer: "provider-error:anthropic:" + string(provider.GetAPIFormat()) + ":unrecognized:v1",
		}, true
	}
	errorType := strings.ToLower(strings.TrimSpace(envelope.Error.Type))
	result := ProviderError{
		StatusCode: apiErr.StatusCode,
		Recognizer: "provider-error:anthropic:" + string(provider.GetAPIFormat()) + ":v1",
	}
	if allowlistedAnthropicType(errorType) {
		result.Type = errorType
	} else {
		result.Category = ProviderErrorUnrecognized
		result.Recognizer += ":unrecognized"
		return result, true
	}
	switch errorType {
	case "authentication_error":
		result.Category = ProviderErrorAuthentication
	case "permission_error", "billing_error":
		result.Category = ProviderErrorPermission
	case "rate_limit_error":
		result.Category = ProviderErrorQuota
	case "overloaded_error", "timeout_error", "api_error":
		result.Category = ProviderErrorTransient
	case "not_found_error":
		result.Category = ProviderErrorNotFound
	case "invalid_request_error":
		result.Category = ProviderErrorInvalidRequest
	default:
		result.Category = ProviderErrorUnrecognized
	}
	result.Recognizer += ":" + string(result.Category)
	return result, true
}

func recognizeGoogleError(provider *ProviderConfig, apiErr genai.APIError) (ProviderError, bool) {
	status := strings.ToUpper(strings.TrimSpace(apiErr.Status))
	if status == "" && apiErr.Code == 0 {
		return ProviderError{}, false
	}
	result := ProviderError{
		StatusCode: apiErr.Code,
		Recognizer: "provider-error:google:" + string(provider.GetAPIFormat()) + ":v1",
	}
	switch {
	case status == "UNAUTHENTICATED" || apiErr.Code == http.StatusUnauthorized:
		result.Category = ProviderErrorAuthentication
	case status == "PERMISSION_DENIED" || apiErr.Code == http.StatusForbidden:
		result.Category = ProviderErrorPermission
	case status == "RESOURCE_EXHAUSTED" || apiErr.Code == http.StatusTooManyRequests:
		result.Category = ProviderErrorQuota
	case status == "UNAVAILABLE" || status == "DEADLINE_EXCEEDED" || apiErr.Code >= 500:
		result.Category = ProviderErrorTransient
	case status == "INVALID_ARGUMENT" || apiErr.Code == http.StatusBadRequest:
		result.Category = ProviderErrorInvalidRequest
	default:
		result.Category = ProviderErrorUnrecognized
	}
	result.Recognizer += ":" + string(result.Category)
	return result, true
}

func canonicalParameterField(format APIFormat, param string) llmcapabilities.FieldKey {
	param = strings.ToLower(strings.TrimSpace(param))
	switch format {
	case APIFormatOpenAI:
		switch param {
		case "max_tokens", "max_completion_tokens":
			return llmcapabilities.FieldMaxOutputTokens
		case "temperature":
			return llmcapabilities.FieldTemperature
		case "top_p":
			return llmcapabilities.FieldTopP
		case "reasoning_effort":
			return llmcapabilities.FieldReasoningEffort
		default:
			return ""
		}
	case APIFormatOpenAIResponses:
		switch param {
		case "max_output_tokens":
			return llmcapabilities.FieldMaxOutputTokens
		case "temperature":
			return llmcapabilities.FieldTemperature
		case "top_p":
			return llmcapabilities.FieldTopP
		case "reasoning.effort":
			return llmcapabilities.FieldReasoningEffort
		default:
			return ""
		}
	default:
		return ""
	}
}

func openAIErrorFamily(providerType ProviderType) string {
	switch strings.ToLower(strings.TrimSpace(string(providerType))) {
	case "openai", "chatgpt":
		return "openai"
	case "grok", "xai", "x.ai":
		return "xai"
	case "litellm":
		return "litellm"
	case "openrouter":
		return "openrouter"
	case "localai":
		return "localai"
	case "zai", "z.ai", "zhipu":
		return "zai"
	case "ollama":
		return "ollama"
	case "llamacpp", "llama.cpp":
		return "llamacpp"
	case "groq":
		return "groq"
	case "mistral":
		return "mistral"
	case "together":
		return "together"
	case "fireworks":
		return "fireworks"
	case "perplexity":
		return "perplexity"
	case "deepseek":
		return "deepseek"
	default:
		return "openai-compatible"
	}
}

func allowlistedProviderErrorPair(family string, status int, code, errorType string) (string, string) {
	code = strings.ToLower(strings.TrimSpace(code))
	errorType = strings.ToLower(strings.TrimSpace(errorType))
	switch {
	case (family == "openai" || family == "xai") && code == "unsupported_parameter" && (status == 0 || status == http.StatusBadRequest) && (errorType == "" || errorType == "invalid_request_error"):
		return code, errorType
	case family == "litellm" && errorType == "unsupportedparamserror" && (code == "" || code == "400"):
		return "", errorType
	case code == "invalid_api_key" && (errorType == "" || errorType == "authentication_error"):
		return code, errorType
	case code == "insufficient_quota" && (errorType == "" || errorType == "rate_limit_error"):
		return code, errorType
	case code == "rate_limit_exceeded" && (errorType == "" || errorType == "rate_limit_error"):
		return code, errorType
	case code == "unsupported_value" && (family == "openai" || family == "xai" || family == "litellm"):
		return code, ""
	case code == "invalid_value" && (family == "openai" || family == "xai" || family == "litellm"):
		return code, ""
	case code == "model_not_found" && (errorType == "" || errorType == "invalid_request_error" || errorType == "not_found_error"):
		return code, errorType
	case code == "server_error" && (errorType == "" || errorType == "server_error"):
		return code, errorType
	case allowlistedProviderCode(family, code) == "" && allowlistedProviderType(errorType) != "":
		return "", errorType
	case errorType == "invalid_request_error" && (code == "" || code == "400"):
		return "", errorType
	default:
		return "", ""
	}
}

func allowlistedProviderCode(family, code string) string {
	if code == "" {
		return ""
	}
	switch code {
	case "unsupported_parameter", "unsupported_value", "invalid_value":
		if family == "openai" || family == "xai" || family == "litellm" {
			return code
		}
	case "invalid_api_key", "insufficient_quota", "rate_limit_exceeded", "model_not_found", "server_error":
		return code
	}
	return ""
}

func allowlistedProviderType(errorType string) string {
	switch errorType {
	case "authentication_error", "permission_error", "rate_limit_error", "server_error", "invalid_request_error", "not_found_error", "unsupportedparamserror":
		return errorType
	default:
		return ""
	}
}

func allowlistedAnthropicType(errorType string) bool {
	switch errorType {
	case "authentication_error", "permission_error", "billing_error", "rate_limit_error", "overloaded_error", "timeout_error", "api_error", "not_found_error", "invalid_request_error":
		return true
	default:
		return false
	}
}

// DisplayMessage devolve uma mensagem estável sem expor mensagem, payload,
// prompt, valor rejeitado ou outro texto livre do provedor.
func (e ProviderError) DisplayMessage() string {
	message := "O provedor retornou um erro não reconhecido. Confira a configuração do provedor e tente novamente."
	switch e.Category {
	case ProviderErrorUnsupportedParameter:
		if e.Field != "" {
			fieldName := string(e.Field)
			if e.Param != "" {
				fieldName = e.Param
			}
			message = fmt.Sprintf("O provedor não aceita o parâmetro %s para este modelo. Confira os parâmetros configurados.", fieldName)
		} else {
			message = "O provedor recusou um parâmetro, mas não identificou um campo que possa ser omitido com segurança."
		}
	case ProviderErrorInvalidValue:
		if e.Field != "" {
			message = fmt.Sprintf("O provedor recusou o valor configurado para %s. O campo e o valor salvos foram mantidos.", e.Field)
		} else {
			message = "O provedor recusou um valor da configuração. Confira os parâmetros do modelo."
		}
	case ProviderErrorInvalidRequest:
		message = "O provedor recusou a solicitação. Confira o modelo e os parâmetros enviados."
	case ProviderErrorAuthentication:
		message = "O provedor recusou a autenticação. Confira a credencial configurada."
	case ProviderErrorPermission:
		message = "A credencial não tem permissão para realizar esta solicitação no provedor."
	case ProviderErrorQuota:
		message = "O limite ou a cota do provedor foi atingido. Aguarde ou confira o plano e os limites da conta."
	case ProviderErrorTransient:
		message = "O provedor está temporariamente indisponível. Tente novamente em alguns instantes."
	case ProviderErrorNotFound:
		message = "O modelo ou recurso solicitado não foi encontrado no provedor."
	}
	if e.StatusCode > 0 {
		message += fmt.Sprintf(" (HTTP %d)", e.StatusCode)
	}
	return message
}
