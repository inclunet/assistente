package agent

import (
	"log"
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

	// minResultContextSize é o tamanho mínimo que um resultado de tool pode ter no contexto LLM (bytes).
	// Mesmo sob pressão de contexto, nunca truncamos abaixo disso.
	minResultContextSize = 512
)

// estimateTokens estima tokens de um texto usando heurística chars/token.
func estimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + charsPerToken - 1) / charsPerToken
}

// estimateMessageTokens estima o total de tokens de uma slice de llm.Message.
func estimateMessageTokens(messages []llm.Message) int {
	total := 0
	for _, m := range messages {
		if s, ok := m.Content.(string); ok {
			total += estimateTokens(s)
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

	for i, r := range toolResults {
		// Calcula quota proporcional para este resultado
		proportion := float64(len(r)) / float64(totalSize)
		maxBytes := int(proportion * float64(availableBytes))

		// Nunca abaixo do mínimo
		if maxBytes < minResultContextSize {
			maxBytes = minResultContextSize
		}

		if len(r) > maxBytes {
			toolResults[i] = truncateUTF8Safe(r, maxBytes) +
				"\n\n[CONTEXTO TRUNCADO: resultado tinha " + intToStr(len(r)) + " bytes, limitado a " + intToStr(maxBytes) + " bytes para caber na janela de contexto]"
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

// intToStr converte int para string sem import de strconv (evita import desnecessário).
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	buf := make([]byte, 0, 20)
	for n > 0 {
		buf = append(buf, byte('0'+n%10))
		n /= 10
	}
	if neg {
		buf = append(buf, '-')
	}
	// reverse
	for i, j := 0, len(buf)-1; i < j; i, j = i+1, j-1 {
		buf[i], buf[j] = buf[j], buf[i]
	}
	return string(buf)
}
