package acp

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

// SanitizeLabel prepara um texto do agente para virar rótulo de UI ou
// anúncio: sem escapes de terminal, sem caracteres de controle, sem marcas
// invisíveis de formatação — inclusive as de inversão de direção, que deixam
// um texto ser lido diferente do que ele é —, em linha única e com tamanho
// limitado.
func SanitizeLabel(s string) string {
	s = ansiEscape.ReplaceAllString(withinBudget(s, agentLabelInputBudget), "")

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

// agentContentBudget é o teto de um bloco de conteúdo. Ele é folgado perto do
// rótulo porque aqui o texto não é resumo: é o que a pessoa lê para decidir, e
// cortá-lo cedo faria alguém autorizar uma linha de comando cuja metade nunca
// apareceu na tela.
const agentContentBudget = 64 << 10

// SanitizeContent prepara um texto do agente para virar bloco de conteúdo — o
// comando que ele quer rodar, mostrado inteiro para quem vai autorizar. Tira o
// que engana e o que atravanca o leitor de telas, mas preserva as quebras de
// linha e não resume: um rótulo cortado informa mal; um comando cortado faz a
// pessoa autorizar o que não viu.
func SanitizeContent(s string) string {
	s = ansiEscape.ReplaceAllString(withinBudget(s, agentContentBudget), "")

	var out strings.Builder
	out.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n', r == '\t':
			out.WriteRune(r)
		case r == '\r':
			// O retorno de carro sozinho reescreve a linha no terminal: no
			// bloco ele some, e o par \r\n já entrega a quebra pelo \n.
			continue
		case r == utf8.RuneError, unicode.IsControl(r), unicode.Is(unicode.Cf, r):
			continue
		default:
			out.WriteRune(r)
		}
	}
	return strings.TrimSpace(out.String())
}

// SanitizeCommandPart prepara um pedaço de linha de comando vindo do agente —
// o programa ou um argumento dele. Não serve o saneamento de rótulo aqui:
// rótulo é para ler, e por isso ele resume; um comando é para copiar, e um
// comando resumido é pior do que comando nenhum, porque a pessoa cola no
// terminal uma linha que parece inteira e termina em `C:\Program Files\…`.
//
// Daqui sai o texto sem o que engana o olho e em linha única: quebra e
// tabulação viram espaço, porque coladas no terminal partiriam a linha em duas
// e a segunda metade rodaria sozinha.
func SanitizeCommandPart(s string) string {
	return strings.Join(strings.Fields(SanitizeContent(s)), " ")
}

// withinBudget corta o texto antes de qualquer trabalho sobre ele, em fronteira
// de runa para não partir um caractere ao meio.
func withinBudget(s string, budget int) string {
	if len(s) <= budget {
		return s
	}
	cut := budget
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
