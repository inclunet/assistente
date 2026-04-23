package agent

import (
	"log"
	"strconv"
	"unicode/utf8"

	"assistente/internal/llm"
)

// Constantes de estimativa de tokens (AEP-0039 Fase 4).
const (
	// charsPerToken é a heurística de ~4 chars por token (consistente com summarization/service.go).
	charsPerToken = 4

	// contextSafetyThreshold é o percentual máximo de uso do contexto antes de truncar resultados.
	// Quando estimatedTokens > contextLimit * contextSafetyThreshold, os resultados são truncados.
	contextSafetyThreshold = 0.90

	// MaxResultDisplaySize é o tamanho máximo de um resultado para exibição em eventos UI (bytes).
	MaxResultDisplaySize = 200

	// minResultContextSize é o tamanho mínimo desejado para cada resultado de tool no contexto LLM (bytes).
	// Sob pressão extrema de contexto, o truncamento efetivo ainda pode ficar abaixo disso
	// se o orçamento total disponível não for suficiente para garantir esse piso a todos os resultados.
	minResultContextSize = 512
)

// estimateTokens estima tokens de um texto usando heurística chars/token.
// Usa contagem de runes (caracteres Unicode) para consistência com charsPerToken,
// evitando distorção em textos com multibyte UTF-8.
func estimateTokens(text string) int {
	charCount := utf8.RuneCountInString(text)
	if charCount == 0 {
		return 0
	}
	return (charCount + charsPerToken - 1) / charsPerToken
}

// estimateMessageTokens estima o total de tokens de uma slice de llm.Message.
func estimateMessageTokens(messages []llm.Message) int {
	total := 0
	for _, m := range messages {
		switch c := m.Content.(type) {
		case string:
			total += estimateTokens(c)
		case []llm.ContentPart:
			for _, part := range c {
				switch part.Type {
				case "text":
					total += estimateTokens(part.Text)
				case "image_url":
					// Custo fixo estimado para imagens (~85 tokens para low detail)
					total += 85
				}
			}
		case []interface{}:
			// Fallback para conteúdo multimodal deserializado como []interface{}
			for _, item := range c {
				if m, ok := item.(map[string]interface{}); ok {
					if t, ok := m["text"].(string); ok {
						total += estimateTokens(t)
					} else if m["type"] == "image_url" {
						total += 85
					}
				}
			}
		}
		for _, tc := range m.ToolCalls {
			total += estimateTokens(tc.Function.Arguments)
			total += estimateTokens(tc.Function.Name)
		}
		// overhead per message (~4 tokens for role/formatting)
		total += 4
	}
	return total
}

// ContextPreCheckResult contém o resultado do pre-check de contexto.
type ContextPreCheckResult struct {
	// Truncated indica se algum resultado foi truncado.
	Truncated bool
	// OriginalTokens é a estimativa antes do truncamento.
	OriginalTokens int
	// FinalTokens é a estimativa após truncamento.
	FinalTokens int
	// AvailableTokens é o budget disponível para resultados de tools.
	AvailableTokens int
}

// PreCheckContextWindow verifica se os resultados de tools cabem na janela de contexto
// e os trunca proporcionalmente se necessário (AEP-0039 Fase 4).
//
// contextLimit: tamanho da janela de contexto do modelo (0 = sem limite, skip check).
// maxResponseTokens: tokens reservados para a resposta do LLM.
// existingMessages: mensagens já no histórico (incluindo system prompt).
// toolResults: conteúdos dos resultados das tools (serão truncados in-place se necessário).
//
// Retorna informações sobre o pre-check. Se contextLimit <= 0, retorna sem truncar.
func PreCheckContextWindow(contextLimit, maxResponseTokens int, existingMessages []llm.Message, toolResults []string) ContextPreCheckResult {
	if contextLimit <= 0 {
		return ContextPreCheckResult{}
	}

	existingTokens := estimateMessageTokens(existingMessages)
	if maxResponseTokens <= 0 {
		maxResponseTokens = 4096 // fallback conservador
	}

	// Budget disponível = contextLimit * 90% - tokens existentes - tokens de resposta
	safeLimit := int(float64(contextLimit) * contextSafetyThreshold)
	availableTokens := safeLimit - existingTokens - maxResponseTokens

	// Reserva overhead por mensagem de tool (~4 tokens cada para role/formatting),
	// já que cada tool result será adicionado como mensagem separada no prompt.
	toolMessageOverhead := len(toolResults) * 4
	availableTokens -= toolMessageOverhead

	if availableTokens < 0 {
		availableTokens = 0
	}

	// Estima tokens dos resultados das tools
	resultTokens := 0
	for _, r := range toolResults {
		resultTokens += estimateTokens(r)
	}

	result := ContextPreCheckResult{
		OriginalTokens:  resultTokens,
		FinalTokens:     resultTokens,
		AvailableTokens: availableTokens,
	}

	// Se cabem, nada a fazer
	if resultTokens <= availableTokens {
		return result
	}

	// Precisa truncar. Calcula budget proporcional por resultado.
	log.Printf("[Agent] context pre-check: %d tokens de tools excede budget de %d (contexto=%d, existente=%d, resposta=%d)",
		resultTokens, availableTokens, contextLimit, existingTokens, maxResponseTokens)

	// Distribui o budget proporcionalmente ao tamanho de cada resultado
	totalSize := 0
	for _, r := range toolResults {
		totalSize += len(r)
	}
	if totalSize == 0 {
		return result
	}

	availableBytes := availableTokens * charsPerToken
	truncatedTokens := 0

	// Caso especial: budget zero ou negativo — remove completamente os resultados
	// para não adicionar tokens ao prompt quando não há budget disponível.
	nResults := len(toolResults)
	if availableBytes <= 0 {
		for i := range toolResults {
			if len(toolResults[i]) > 0 {
				toolResults[i] = ""
				result.Truncated = true
			}
		}
		result.FinalTokens = 0
		if result.Truncated {
			log.Printf("[Agent] context pre-check: budget zero, %d resultados removidos", nResults)
		}
		return result
	}

	// Calcula quotas respeitando minResultContextSize, mas garante que a soma
	// das quotas não exceda availableBytes (evita estourar o budget).
	minTotal := nResults * minResultContextSize
	effectiveMin := minResultContextSize
	if minTotal > availableBytes && nResults > 0 {
		// Quando o budget é menor que o mínimo coletivo, distribui igualmente
		effectiveMin = availableBytes / nResults
		if effectiveMin < 1 {
			effectiveMin = 1
		}
	}

	// Primeira passada: calcula quotas e clampeia ao budget total.
	quotas := make([]int, nResults)
	quotaSum := 0
	for i, r := range toolResults {
		proportion := float64(len(r)) / float64(totalSize)
		q := int(proportion * float64(availableBytes))
		if q < effectiveMin {
			q = effectiveMin
		}
		quotas[i] = q
		quotaSum += q
	}

	// Se a soma das quotas excede o budget (possível com effectiveMin),
	// reduz proporcionalmente para caber.
	if quotaSum > availableBytes && quotaSum > 0 {
		scale := float64(availableBytes) / float64(quotaSum)
		quotaSum = 0
		for i := range quotas {
			quotas[i] = int(float64(quotas[i]) * scale)
			if quotas[i] < 0 {
				quotas[i] = 0
			}
			quotaSum += quotas[i]
		}
	}

	// Se o scaling zerou todas as quotas mas ainda há budget,
	// redistribui o budget total sem exceder availableBytes.
	// Quando availableBytes < nResults, alguns resultados ficam com quota 0.
	if quotaSum == 0 && availableBytes > 0 && nResults > 0 {
		baseQuota := availableBytes / nResults
		remainder := availableBytes % nResults
		quotaSum = 0
		for i := range quotas {
			quotas[i] = baseQuota
			if i < remainder {
				quotas[i]++
			}
			quotaSum += quotas[i]
		}
	}

	// Segunda passada: aplica truncamento reservando espaço para o aviso.
	for i, r := range toolResults {
		maxBytes := quotas[i]

		if len(r) > maxBytes {
			// Reserva bytes para o aviso de truncamento (não ultrapassar maxBytes no total).
			warning := "\n\n[CONTEXTO TRUNCADO: resultado tinha " + strconv.Itoa(len(r)) + " bytes, limitado a " + strconv.Itoa(maxBytes) + " bytes para caber na janela de contexto]"
			contentBudget := maxBytes - len(warning)
			if contentBudget >= 1 {
				toolResults[i] = truncateUTF8Safe(r, contentBudget) + warning
			} else {
				// Warning não cabe no budget — trunca sem aviso para respeitar maxBytes.
				toolResults[i] = truncateUTF8Safe(r, maxBytes)
			}
			result.Truncated = true
		}
		truncatedTokens += estimateTokens(toolResults[i])
	}

	result.FinalTokens = truncatedTokens
	if result.Truncated {
		log.Printf("[Agent] context pre-check: truncou %d → %d tokens estimados", resultTokens, truncatedTokens)
	}
	return result
}

// truncateUTF8Safe trunca string até maxBytes sem cortar runes.
func truncateUTF8Safe(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	for maxBytes > 0 && !utf8.RuneStart(s[maxBytes]) {
		maxBytes--
	}
	return s[:maxBytes]
}
