package llm

import "strings"

// localAIChatMLState mantém estado incremental do parser de ChatML do LocalAI
// para conseguir separar <|channel|>analysis... e <|channel|>final... durante streaming.
type localAIChatMLState struct {
	active  bool
	channel string // "analysis", "final", ""
	pending string
}

const (
	localAIAnalysisStart  = "<|channel|>analysis<|message|>"
	localAIFinalStart     = "<|channel|>final<|message|>"
	localAIEndToken       = "<|end|>"
	localAIStartAssistant = "<|start|>assistant"
)

var localAIMarkers = []string{
	localAIAnalysisStart,
	localAIFinalStart,
	localAIEndToken,
	localAIStartAssistant,
}

func localAIMaxMarkerLen() int {
	max := 0
	for _, m := range localAIMarkers {
		if len(m) > max {
			max = len(m)
		}
	}
	return max
}

// SplitLocalAIChatML separa o texto final (final) do reasoning (analysis) quando o
// LocalAI devolve o conteúdo no formato ChatML.
// Se não detectar o formato, ok=false e final=s.
func SplitLocalAIChatML(s string) (final string, reasoning string, ok bool) {
	if s == "" {
		return "", "", false
	}
	if !strings.Contains(s, "<|channel|>") && !strings.Contains(s, "<|start|>") && !strings.Contains(s, "<|end|>") {
		return s, "", false
	}

	var outFinal strings.Builder
	var outReason strings.Builder
	channel := "" // "analysis" | "final" | ""
	i := 0
	for i < len(s) {
		nextIdx, marker := findNextLocalAIMarker(s, i)
		if nextIdx == -1 {
			segment := s[i:]
			switch channel {
			case "analysis":
				outReason.WriteString(segment)
			case "final", "":
				outFinal.WriteString(segment)
			}
			break
		}

		segment := s[i:nextIdx]
		switch channel {
		case "analysis":
			outReason.WriteString(segment)
		case "final", "":
			outFinal.WriteString(segment)
		}

		switch marker {
		case localAIAnalysisStart:
			ok = true
			channel = "analysis"
		case localAIFinalStart:
			ok = true
			channel = "final"
		case localAIEndToken:
			ok = true
			channel = ""
		case localAIStartAssistant:
			ok = true
			// não muda channel; token apenas delimita mensagens
		}

		i = nextIdx + len(marker)
	}

	// Se detectou ChatML, remove resíduos de tokens comuns que podem sobrar.
	if ok {
		final = strings.ReplaceAll(outFinal.String(), "<|start|>", "")
		final = strings.ReplaceAll(final, "<|end|>", "")
		reasoning = strings.ReplaceAll(outReason.String(), "<|start|>", "")
		reasoning = strings.ReplaceAll(reasoning, "<|end|>", "")
		return final, reasoning, true
	}

	return s, "", false
}

func findNextLocalAIMarker(s string, start int) (idx int, marker string) {
	bestIdx := -1
	bestMarker := ""
	for _, m := range localAIMarkers {
		j := strings.Index(s[start:], m)
		if j == -1 {
			continue
		}
		j += start
		if bestIdx == -1 || j < bestIdx {
			bestIdx = j
			bestMarker = m
		}
	}
	return bestIdx, bestMarker
}

func longestSuffixPrefixLen(s string) int {
	maxMarker := localAIMaxMarkerLen()
	maxCheck := maxMarker - 1
	if maxCheck <= 0 {
		return 0
	}
	if len(s) < maxCheck {
		maxCheck = len(s)
	}

	// tenta do maior para o menor
	for l := maxCheck; l >= 1; l-- {
		suffix := s[len(s)-l:]
		for _, m := range localAIMarkers {
			if strings.HasPrefix(m, suffix) {
				return l
			}
		}
	}
	return 0
}

// processLocalAIChatML faz parsing incremental do formato LocalAI ChatML.
// Retorna apenas conteúdo do canal "final" (ou conteúdo normal quando não detectado).
func processLocalAIChatML(content string, st *localAIChatMLState, fullReasoning *strings.Builder, handler StreamHandler) string {
	if content == "" {
		return ""
	}

	s := st.pending + content
	st.pending = ""

	// Se ainda não ativou, detecta rapidamente ou segura sufixos que podem iniciar marcador.
	if !st.active {
		if strings.Contains(s, "<|channel|>") || strings.Contains(s, "<|start|>") || strings.Contains(s, "<|end|>") {
			st.active = true
		} else {
			suffixLen := longestSuffixPrefixLen(s)
			if suffixLen > 0 {
				st.pending = s[len(s)-suffixLen:]
				return s[:len(s)-suffixLen]
			}
			return content
		}
	}

	var out strings.Builder
	i := 0
	for i < len(s) {
		nextIdx, marker := findNextLocalAIMarker(s, i)
		if nextIdx == -1 {
			// Sem marcador completo; preserva sufixo potencial
			rest := s[i:]
			suffixLen := longestSuffixPrefixLen(rest)
			safe := rest
			if suffixLen > 0 {
				safe = rest[:len(rest)-suffixLen]
				st.pending = rest[len(rest)-suffixLen:]
			}
			emitLocalAISegment(safe, st, &out, fullReasoning, handler)
			break
		}

		segment := s[i:nextIdx]
		emitLocalAISegment(segment, st, &out, fullReasoning, handler)

		switch marker {
		case localAIAnalysisStart:
			st.channel = "analysis"
		case localAIFinalStart:
			st.channel = "final"
		case localAIEndToken:
			st.channel = ""
		case localAIStartAssistant:
			// ignora
		}
		i = nextIdx + len(marker)
	}

	return out.String()
}

func emitLocalAISegment(seg string, st *localAIChatMLState, out *strings.Builder, fullReasoning *strings.Builder, handler StreamHandler) {
	if seg == "" {
		return
	}

	switch st.channel {
	case "analysis":
		fullReasoning.WriteString(seg)
		handler.OnThinking(seg)
	case "final":
		out.WriteString(seg)
	case "":
		// Antes de ver um canal explícito, preserva como texto normal
		out.WriteString(seg)
	}
}
