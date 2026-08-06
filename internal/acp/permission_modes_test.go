package acp

import "testing"

// Os dois modos do Claude Code que respondem sozinhos a todo pedido de
// permissão. Reconhecê-los é o que permite avisar que a única barreira do app
// caiu (AEP-0084 D9).
func TestModosConhecidosPorDispensarOPedidoDePermissao(t *testing.T) {
	for _, mode := range []string{"dontAsk", "bypassPermissions"} {
		if !ModeSkipsPermissionPrompt(mode) {
			t.Errorf("o modo %q desliga o pedido de permissão e não foi reconhecido", mode)
		}
	}
}

// O valor vem do agente pelo fio. Deixar de reconhecê-lo por causa da caixa ou
// de um espaço sobrando calaria justamente onde o silêncio custa caro.
func TestOModoEReconhecidoSemDependerDaCaixaOuDoEspaco(t *testing.T) {
	for _, mode := range []string{"  dontAsk  ", "DONTASK", "BypassPermissions"} {
		if !ModeSkipsPermissionPrompt(mode) {
			t.Errorf("o modo %q é um dos que dispensam a pergunta e não foi reconhecido", mode)
		}
	}
}

// A lista é de valores conhecidos, e não uma classificação de modos: o que não
// se sabe fica de fora. O `acceptEdits` é o caso que mais tenta — ele dispensa
// a pergunta só para edição e continua perguntando pelo resto, então avisar que
// a barreira caiu inteira seria falso.
func TestModoQueContinuaPerguntandoNaoEntraNaLista(t *testing.T) {
	for _, mode := range []string{"", "   ", "agent", "plan", "ask", "default", "auto", "acceptEdits"} {
		if ModeSkipsPermissionPrompt(mode) {
			t.Errorf("o modo %q foi tomado por um que desliga o pedido de permissão", mode)
		}
	}
}
