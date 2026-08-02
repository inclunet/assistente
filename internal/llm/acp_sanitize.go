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
	s = ansiEscape.ReplaceAllString(s, "")

	var out strings.Builder
	out.Grow(len(s))
	pendingSpace := false
	for _, r := range s {
		switch {
		case r == utf8.RuneError:
			continue
		case unicode.IsSpace(r):
			pendingSpace = true
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			continue
		default:
			if pendingSpace && out.Len() > 0 {
				out.WriteRune(' ')
			}
			pendingSpace = false
			out.WriteRune(r)
		}
	}
	return truncateRunes(out.String(), agentLabelLimit)
}

// truncateRunes corta por runa, e não por byte, para não partir um caractere
// ao meio e deixar lixo na tela.
func truncateRunes(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return strings.TrimRight(string(runes[:limit]), " ") + "…"
}
