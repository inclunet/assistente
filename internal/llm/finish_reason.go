package llm

import "strings"

func finishInfo(raw string, reason FinishReason) FinishInfo {
	return FinishInfo{
		Reason:    reason,
		RawReason: strings.TrimSpace(raw),
	}
}

// finishInfoWithToolCalls alinha o motivo ao callback terminal quando o
// transporte encerra uma resposta com chamadas completas, mas informa apenas
// stop (ou omite o motivo). Motivos de segurança/limite nunca são sobrescritos.
func finishInfoWithToolCalls(info FinishInfo, toolCallCount int) FinishInfo {
	if toolCallCount > 0 && (info.Reason == "" || info.Reason == FinishReasonStop) {
		info.Reason = FinishReasonToolCalls
	}
	return info
}

// finishInfoWithDiagnostics acrescenta metadados não sensíveis conhecidos pelo
// transporte. OutputLimit é zero quando o request deixou o provider aplicar o
// próprio default; ResponseBytes mede apenas o texto visível, nunca seu conteúdo.
func finishInfoWithDiagnostics(info FinishInfo, provider *ProviderConfig, model string, outputLimit, responseBytes int) FinishInfo {
	if provider != nil {
		info.Provider = strings.TrimSpace(provider.ID)
		if info.Provider == "" {
			info.Provider = strings.TrimSpace(provider.Name)
		}
	}
	info.Model = strings.TrimSpace(model)
	info.OutputLimit = outputLimit
	info.ResponseBytes = responseBytes
	return info
}

func normalizeOpenAIChatFinishReason(raw string) FinishInfo {
	switch normalized := strings.ToLower(strings.TrimSpace(raw)); normalized {
	case "":
		return FinishInfo{}
	case "stop":
		return finishInfo(raw, FinishReasonStop)
	case "tool_calls", "function_call":
		return finishInfo(raw, FinishReasonToolCalls)
	case "length", "max_tokens":
		return finishInfo(raw, FinishReasonMaxTokens)
	case "content_filter":
		return finishInfo(raw, FinishReasonContentFilter)
	default:
		return finishInfo(raw, FinishReasonOther)
	}
}

func normalizeOpenAIResponsesFinishReason(raw string) FinishInfo {
	switch normalized := strings.ToLower(strings.TrimSpace(raw)); normalized {
	case "":
		return FinishInfo{}
	case "completed", "stop":
		return finishInfo(raw, FinishReasonStop)
	case "max_output_tokens", "max_tokens":
		return finishInfo(raw, FinishReasonMaxTokens)
	case "content_filter":
		return finishInfo(raw, FinishReasonContentFilter)
	case "cancelled", "canceled":
		return finishInfo(raw, FinishReasonCancelled)
	default:
		return finishInfo(raw, FinishReasonOther)
	}
}

func normalizeAnthropicFinishReason(raw string) FinishInfo {
	switch normalized := strings.ToLower(strings.TrimSpace(raw)); normalized {
	case "":
		return FinishInfo{}
	case "end_turn", "stop_sequence":
		return finishInfo(raw, FinishReasonStop)
	case "tool_use":
		return finishInfo(raw, FinishReasonToolCalls)
	case "max_tokens":
		return finishInfo(raw, FinishReasonMaxTokens)
	case "refusal":
		return finishInfo(raw, FinishReasonContentFilter)
	default:
		return finishInfo(raw, FinishReasonOther)
	}
}

func normalizeGoogleFinishReason(raw string) FinishInfo {
	switch normalized := strings.ToUpper(strings.TrimSpace(raw)); normalized {
	case "", "FINISH_REASON_UNSPECIFIED":
		return FinishInfo{}
	case "STOP":
		return finishInfo(raw, FinishReasonStop)
	case "MAX_TOKENS":
		return finishInfo(raw, FinishReasonMaxTokens)
	case "SAFETY", "RECITATION", "LANGUAGE", "BLOCKLIST", "PROHIBITED_CONTENT", "SPII", "IMAGE_SAFETY", "IMAGE_PROHIBITED_CONTENT", "IMAGE_RECITATION":
		return finishInfo(raw, FinishReasonContentFilter)
	default:
		return finishInfo(raw, FinishReasonOther)
	}
}

func normalizeACPFinishReason(raw string) FinishInfo {
	switch normalized := strings.ToLower(strings.TrimSpace(raw)); normalized {
	case "":
		return FinishInfo{}
	case "end_turn":
		return finishInfo(raw, FinishReasonStop)
	case "max_tokens":
		return finishInfo(raw, FinishReasonMaxTokens)
	case "cancelled", "canceled":
		return finishInfo(raw, FinishReasonCancelled)
	case "refusal":
		return finishInfo(raw, FinishReasonContentFilter)
	default:
		return finishInfo(raw, FinishReasonOther)
	}
}
