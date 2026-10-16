package speech

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

var (
	providerDiagnosticPattern    = regexp.MustCompile(`^Falha na resposta do agente: (provider_error:v1:[a-z_]+:[a-z_.]*(?::[1-5][0-9]{2})?)$`)
	mcpDiagnosticPattern         = regexp.MustCompile(`^mcp_error:v1:([^:\s]+):(list_tools|call|handshake|unknown)$`)
	prefixedMCPDiagnosticPattern = regexp.MustCompile(`^Falha na resposta do agente: (mcp_error:v1:[^:\s]+:(?:list_tools|call|handshake|unknown))$`)
	providerMarkerPattern        = regexp.MustCompile(`^provider_error:v1:([a-z_]+):([a-z_.]*)(?::([1-5][0-9]{2}))?$`)
)

var speechErrorFields = map[string]string{
	"max_tokens": "max tokens", "max_completion_tokens": "max completion tokens", "max_output_tokens": "max output tokens",
	"temperature": "temperature", "top_p": "top p", "reasoning_effort": "reasoning effort", "reasoning.effort": "reasoning effort",
	"frequency_penalty": "frequency penalty", "presence_penalty": "presence penalty", "seed": "seed", "max_reasoning_tokens": "max reasoning tokens",
	"parallel_tool_calls": "parallel tool calls", "voice": "voice", "speed": "speed", "audio_format": "audio format", "language": "language",
}

func isBackendErrorDiagnostic(role, text string) bool {
	if role != "assistant" {
		return false
	}
	return providerDiagnosticPattern.MatchString(text) || mcpDiagnosticPattern.MatchString(text) || prefixedMCPDiagnosticPattern.MatchString(text)
}

// translateBackendErrorForSpeech traduz somente os formatos de diagnóstico que
// o backend persiste, sem interpretar mensagens comuns ou exemplos de código.
func translateBackendErrorForSpeech(text, language string) string {
	lang := speechLanguage(language)
	if match := providerDiagnosticPattern.FindStringSubmatch(text); match != nil {
		if translated, ok := translateProviderMarkerForSpeech(match[1], lang); ok {
			prefixes := [3]string{"Falha na resposta do agente: ", "Agent response failed: ", "Falló la respuesta del agente: "}
			return prefixes[lang] + translated
		}
		return text
	}
	mcpMarker := text
	if match := prefixedMCPDiagnosticPattern.FindStringSubmatch(text); match != nil {
		mcpMarker = match[1]
	}
	if match := mcpDiagnosticPattern.FindStringSubmatch(mcpMarker); match != nil {
		server, err := url.PathUnescape(match[1])
		if err != nil {
			return text
		}
		stages := map[string][3]string{
			"list_tools": {"ao listar ferramentas", "while listing tools", "al listar herramientas"},
			"call":       {"ao executar uma ferramenta", "while running a tool", "al ejecutar una herramienta"},
			"handshake":  {"durante a conexão", "during connection setup", "durante la conexión"},
			"unknown":    {"durante a comunicação", "during communication", "durante la comunicación"},
		}
		stage := stages[match[2]][lang]
		messages := [3]string{
			"O servidor MCP %s falhou %s. Tente novamente ou verifique as credenciais e a conexão.",
			"MCP server %s failed %s. Try again or check its credentials and connection.",
			"El servidor MCP %s falló %s. Inténtalo de nuevo o comprueba sus credenciales y conexión.",
		}
		return fmt.Sprintf(messages[lang], server, stage)
	}
	return text
}

func translateProviderMarkerForSpeech(marker string, lang int) (string, bool) {
	match := providerMarkerPattern.FindStringSubmatch(marker)
	if match == nil {
		return "", false
	}
	category, rawField, status := match[1], match[2], match[3]
	field := speechErrorFields[rawField]
	if field == "" {
		field = rawField
	}
	statusLabel := ""
	if status != "" {
		statusLabel = fmt.Sprintf(" (HTTP %s)", status)
	}
	categoryText := map[string][3]string{
		"unsupported_parameter": {"O provedor não aceita o parâmetro %s para este modelo. Confira os parâmetros configurados.%s", "The provider does not accept the %s parameter for this model. Check the configured parameters.%s", "El proveedor no acepta el parámetro %s para este modelo. Comprueba los parámetros configurados.%s"},
		"invalid_value":         {"O provedor recusou o valor configurado para %s. O campo e o valor salvos foram mantidos.%s", "The provider rejected the configured value for %s. The saved field and value were kept.%s", "El proveedor rechazó el valor configurado para %s. Se conservaron el campo y el valor guardados.%s"},
		"invalid_request":       {"O provedor recusou a solicitação. Confira o modelo e os parâmetros enviados.%s", "The provider rejected the request. Check the model and submitted parameters.%s", "El proveedor rechazó la solicitud. Comprueba el modelo y los parámetros enviados.%s"},
		"authentication":        {"O provedor recusou a autenticação. Confira a credencial configurada.%s", "The provider rejected authentication. Check the configured credential.%s", "El proveedor rechazó la autenticación. Comprueba la credencial configurada.%s"},
		"permission":            {"A credencial não tem permissão para realizar esta solicitação no provedor.%s", "The credential does not have permission to make this request to the provider.%s", "La credencial no tiene permiso para realizar esta solicitud al proveedor.%s"},
		"quota_or_rate_limit":   {"O limite ou a cota do provedor foi atingido. Aguarde ou confira o plano e os limites da conta.%s", "The provider quota or rate limit was reached. Wait or check the account plan and limits.%s", "Se alcanzó el límite o la cuota del proveedor. Espera o comprueba el plan y los límites de la cuenta.%s"},
		"transient":             {"O provedor está temporariamente indisponível. Tente novamente em alguns instantes.%s", "The provider is temporarily unavailable. Try again shortly.%s", "El proveedor no está disponible temporalmente. Inténtalo de nuevo en unos instantes.%s"},
		"not_found":             {"O modelo ou recurso solicitado não foi encontrado no provedor.%s", "The requested model or resource was not found at the provider.%s", "No se encontró el modelo o recurso solicitado en el proveedor.%s"},
		"unrecognized":          {"O provedor retornou um erro não reconhecido. Confira a configuração do provedor e tente novamente.%s", "The provider returned an unrecognized error. Check the provider configuration and try again.%s", "El proveedor devolvió un error no reconocido. Comprueba su configuración e inténtalo de nuevo.%s"},
	}
	texts, ok := categoryText[category]
	if !ok {
		return "", false
	}
	if (category == "unsupported_parameter" || category == "invalid_value") && field == "" {
		unknown := map[string][3]string{
			"unsupported_parameter": {"O provedor recusou um parâmetro, mas não identificou um campo que possa ser omitido com segurança.%s", "The provider rejected a parameter but did not identify a field that can be safely omitted.%s", "El proveedor rechazó un parámetro, pero no identificó un campo que se pueda omitir de forma segura.%s"},
			"invalid_value":         {"O provedor recusou um valor da configuração. Confira os parâmetros do modelo.%s", "The provider rejected a configured value. Check the model parameters.%s", "El proveedor rechazó un valor de la configuración. Comprueba los parámetros del modelo.%s"},
		}[category]
		return fmt.Sprintf(unknown[lang], statusLabel), true
	}
	if category == "unsupported_parameter" || category == "invalid_value" {
		return fmt.Sprintf(texts[lang], field, statusLabel), true
	}
	return fmt.Sprintf(texts[lang], statusLabel), true
}

func speechLanguage(language string) int {
	switch strings.ToLower(strings.TrimSpace(strings.ReplaceAll(language, "_", "-"))) {
	case "pt", "pt-br":
		return 0
	case "es", "es-es", "es-mx":
		return 2
	default:
		return 1
	}
}
