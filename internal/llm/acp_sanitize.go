package llm

import (
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Saneamento da saída do agente (AEP-0084 D11). O que chega pelo protocolo é
// dado não confiável, e parte dele vira rótulo na tela, chave e — o que mais
// importa aqui — texto lido pelo leitor de telas. O título de uma ferramenta,
// por exemplo, costuma ser a linha de comando literal que o agente rodou.
//
// Isto vale para rótulo, não para a resposta: o texto do turno é conteúdo de
// mensagem, renderizado como markdown como o de qualquer outro provedor, e
// achatá-lo destruiria a formatação.

// agentLabelLimit corta o rótulo num tamanho que ainda cabe num anúncio. Um
// comando de mil caracteres não informa ninguém: atravanca a tela e faz o
// leitor de telas recitar argumento por argumento.
const agentLabelLimit = 200

// agentLabelInputBudget é o quanto do texto original vale a pena examinar. Quem
// manda é o agente, e ele manda o tamanho que quiser; nenhum rótulo honesto
// precisa de mais do que isto para render as primeiras agentLabelLimit runas,
// mesmo em escrita de três bytes por caractere.
const agentLabelInputBudget = 8 << 10

// ansiEscape casa as sequências de escape que ferramentas de terminal usam para
// cor e movimento de cursor. Sem tirá-las antes dos controles, sobraria o resto
// da sequência ("[31m") como se fosse texto.
var ansiEscape = regexp.MustCompile("\x1b(?:\\[[0-9;?]*[ -/]*[@-~]|\\][^\x07\x1b]*(?:\x07|\x1b\\\\)?|[@-Z\\\\-_])")

// sanitizeAgentLabel prepara um texto do agente para virar rótulo de UI ou
// anúncio: sem escapes de terminal, sem caracteres de controle, sem marcas
// invisíveis de formatação — inclusive as de inversão de direção, que deixam
// um texto ser lido diferente do que ele é —, em linha única e com tamanho
// limitado.
func sanitizeAgentLabel(s string) string {
	s = ansiEscape.ReplaceAllString(withinBudget(s), "")

	// A saída tem tamanho conhecido, e uma runa além do limite já basta para
	// saber que o rótulo foi cortado.
	out := make([]rune, 0, agentLabelLimit+1)
	pendingSpace := false
	for _, r := range s {
		switch {
		case unicode.IsSpace(r):
			pendingSpace = true
			continue
		case r == utf8.RuneError, unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			continue
		}
		if pendingSpace && len(out) > 0 {
			out = append(out, ' ')
		}
		pendingSpace = false
		out = append(out, r)
		if len(out) > agentLabelLimit {
			return strings.TrimRight(string(out[:agentLabelLimit]), " ") + "…"
		}
	}
	return string(out)
}

// withinBudget corta o texto antes de qualquer trabalho sobre ele, em fronteira
// de runa para não partir um caractere ao meio.
func withinBudget(s string) string {
	if len(s) <= agentLabelInputBudget {
		return s
	}
	cut := agentLabelInputBudget
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	s = s[:cut]
	// O corte também pode cair no meio de uma sequência de escape. Sem o byte
	// final ela não casa com ansiEscape, o \x1b sai como controle e o resto
	// ("[31") sobraria como texto no rótulo.
	if esc := strings.LastIndexByte(s, 0x1b); esc >= 0 && ansiEscape.FindStringIndex(s[esc:]) == nil {
		s = s[:esc]
	}
	return s
}
