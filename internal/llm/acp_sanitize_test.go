package llm

import (
	"strings"
	"testing"
)

func TestRotuloDoAgenteNaoCarregaOQueOTerminalEscreveu(t *testing.T) {
	casos := []struct {
		nome  string
		texto string
		quer  string
	}{
		{
			nome:  "cor de terminal",
			texto: "\x1b[1;31mfalhou\x1b[0m",
			quer:  "falhou",
		},
		{
			nome:  "título de janela",
			texto: "\x1b]0;titulo\x07npm test",
			quer:  "npm test",
		},
		{
			nome:  "quebra de linha e tabulação",
			texto: "git commit\n\t-m \"pronto\"",
			quer:  "git commit -m \"pronto\"",
		},
		{
			nome:  "espaços repetidos",
			texto: "   ls    -la   ",
			quer:  "ls -la",
		},
		{
			// Inversão de direção deixa o texto ser lido ao contrário do que é:
			// um comando pode se disfarçar de nome de arquivo inofensivo.
			nome:  "marca invisível de direção",
			texto: "arquivo\u202Egnp.exe",
			quer:  "arquivognp.exe",
		},
		{
			nome:  "caractere de controle solto",
			texto: "apaga\x08\x08tudo",
			quer:  "apagatudo",
		},
		{
			nome:  "texto comum passa inteiro",
			texto: "Lendo src/main.go",
			quer:  "Lendo src/main.go",
		},
	}
	for _, caso := range casos {
		t.Run(caso.nome, func(t *testing.T) {
			if got := sanitizeAgentLabel(caso.texto); got != caso.quer {
				t.Errorf("sanitizeAgentLabel(%q) = %q, quer %q", caso.texto, got, caso.quer)
			}
		})
	}
}

func TestRotuloLongoNaoViraRecitacaoNoLeitorDeTelas(t *testing.T) {
	longo := "npm test -- " + strings.Repeat("arquivo.spec.ts ", 60)

	got := sanitizeAgentLabel(longo)

	if len([]rune(got)) != agentLabelLimit+1 {
		t.Fatalf("tamanho = %d runas, quer o limite mais a reticência", len([]rune(got)))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("rótulo cortado precisa dizer que continua: %q", got)
	}
	if !strings.HasPrefix(got, "npm test -- arquivo.spec.ts") {
		t.Errorf("o começo do rótulo se perdeu: %q", got)
	}
}
