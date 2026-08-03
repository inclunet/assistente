package acp

import (
	"strings"
	"testing"
	"unicode/utf8"
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
			if got := SanitizeLabel(caso.texto); got != caso.quer {
				t.Errorf("SanitizeLabel(%q) = %q, quer %q", caso.texto, got, caso.quer)
			}
		})
	}
}

func TestRotuloLongoNaoViraRecitacaoNoLeitorDeTelas(t *testing.T) {
	longo := "npm test -- " + strings.Repeat("arquivo.spec.ts ", 60)

	got := SanitizeLabel(longo)

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

func TestRotuloAbsurdoNaoEVarridoInteiro(t *testing.T) {
	// Quem manda o texto é o agente, e ele manda o tamanho que quiser: varrer
	// megabytes para produzir 200 runas seria trabalho gasto à toa.
	enorme := "início do comando " + strings.Repeat("x", 4<<20)

	got := SanitizeLabel(enorme)

	if len([]rune(got)) != agentLabelLimit+1 {
		t.Fatalf("tamanho = %d runas, quer o limite mais a reticência", len([]rune(got)))
	}
	if !strings.HasPrefix(got, "início do comando xxx") {
		t.Errorf("o começo do rótulo se perdeu: %q", got)
	}
}

func TestEscapeCortadoAoMeioNaoVazaComoTexto(t *testing.T) {
	// O corte por tamanho pode cair dentro de uma sequência de escape. Sem o
	// byte final ela não é reconhecida, e o miolo dela sobraria no rótulo.
	// O enchimento é invisível de propósito: só assim a varredura chega até o
	// limite de entrada em vez de parar nas primeiras 200 runas. Ele para a
	// quatro bytes do corte, e o escape seguinte fica partido bem em cima dele.
	enchimento := strings.Repeat("\x1b[0m", (agentLabelInputBudget-4)/4)
	if len(enchimento) != agentLabelInputBudget-4 {
		t.Fatalf("enchimento com %d bytes: o corte não cairia dentro do escape", len(enchimento))
	}
	cortadoAoMeio := enchimento + "\x1b[31" + strings.Repeat("m vermelho", 100)

	got := SanitizeLabel(cortadoAoMeio)

	if got != "" {
		t.Errorf("rótulo = %q, o resto do escape vazou", got)
	}
}

func TestCaractereMultibyteNaoEPartidoAoMeioNoCorteDeEntrada(t *testing.T) {
	// O corte por bytes cai no meio de um caractere de três bytes; partido, ele
	// viraria lixo na tela.
	enorme := strings.Repeat("ç", agentLabelInputBudget)

	got := SanitizeLabel(enorme)

	if !utf8.ValidString(got) {
		t.Fatalf("rótulo saneado não é UTF-8 válido: %q", got)
	}
	if !strings.HasPrefix(got, "ççç") {
		t.Errorf("rótulo = %q", got)
	}
}
