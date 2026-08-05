package acp

import (
	"testing"
	"time"
)

func nomesDeComandos(commands []Command) []string {
	nomes := make([]string, 0, len(commands))
	for _, command := range commands {
		nomes = append(nomes, command.Name)
	}
	return nomes
}

func iguais(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// esperaComandos espera o próximo anúncio de comandos. O aviso vem pela
// goroutine de entrega do transporte, e não pela chamada que abriu a sessão.
func esperaComandos(t *testing.T, avisos <-chan []Command) []Command {
	t.Helper()
	select {
	case commands := <-avisos:
		return commands
	case <-time.After(testTimeout):
		t.Fatal("o agente não anunciou os comandos da sessão")
		return nil
	}
}

// O agente conta quais comandos oferece assim que a sessão abre, fora de
// qualquer turno (AEP-0084 D8). Este teste sobe o agente de verdade e escuta o
// que chega pelo fio: montar o evento à mão provaria só que o tradutor traduz.
func TestOAgenteAnunciaOsComandosAoAbrirASessao(t *testing.T) {
	ctx := testContext(t)

	avisos := make(chan []Command, 8)
	cfg := fakeConfig(t, scriptComandos)
	cfg.OnCommands = func(_ string, commands []Command) {
		select {
		case avisos <- commands:
		default:
		}
	}
	client, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	sess := startSession(t, client, ctx)
	anunciados := esperaComandos(t, avisos)

	if got := nomesDeComandos(anunciados); !iguais(got, []string{"plan", "resumir"}) {
		t.Fatalf("comandos anunciados = %v", got)
	}
	// O nome sem conteúdo e o repetido saem: um não dá o que digitar depois da
	// barra, o outro faria a pessoa escolher entre dois itens idênticos.
	if got := nomesDeComandos(sess.Commands()); !iguais(got, []string{"plan", "resumir"}) {
		t.Fatalf("comandos guardados na sessão = %v", got)
	}

	// A descrição vira texto de tela e de leitor de telas: o escape de terminal
	// que o agente mandou não pode chegar lá (AEP-0084 D11).
	if desc := anunciados[0].Description; desc != "Monta um plano" {
		t.Errorf("descrição do comando = %q; o saneamento não passou", desc)
	}
	if !anunciados[0].AcceptsInput {
		t.Error("o comando que aceita argumento não foi marcado como tal")
	}
	if anunciados[1].AcceptsInput {
		t.Error("o comando sem entrada foi marcado como se aceitasse argumento")
	}
}

// A lista do agente é o conjunto completo: quando ele manda outra, ela substitui
// a anterior inteira. Somar as duas deixaria no menu comandos que já não existem.
func TestAListaNovaDeComandosSubstituiAAnterior(t *testing.T) {
	ctx := testContext(t)

	avisos := make(chan []Command, 8)
	cfg := fakeConfig(t, scriptComandos)
	cfg.OnCommands = func(_ string, commands []Command) {
		select {
		case avisos <- commands:
		default:
		}
	}
	client, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("criar cliente: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	sess := startSession(t, client, ctx)
	esperaComandos(t, avisos)

	col := &collector{}
	if _, err := sess.Prompt(ctx, []Content{TextContent("oi")}, col.sink); err != nil {
		t.Fatalf("turno falhou: %v", err)
	}

	depois := esperaComandos(t, avisos)
	if got := nomesDeComandos(depois); !iguais(got, []string{"revisar"}) {
		t.Fatalf("comandos depois da troca = %v", got)
	}
	if got := nomesDeComandos(sess.Commands()); !iguais(got, []string{"revisar"}) {
		t.Fatalf("a sessão continuou oferecendo a lista antiga: %v", got)
	}
}
