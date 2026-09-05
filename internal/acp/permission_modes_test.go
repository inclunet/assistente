package acp

import "testing"

// Os modos comprovados que dispensam o pedido de permissão. Reconhecê-los é o
// que permite avisar que a única barreira do app caiu (AEP-0084 D9 e AEP-0086
// Q2).
func TestModosConhecidosPorDispensarOPedidoDePermissao(t *testing.T) {
	for _, mode := range []string{"dontAsk", "bypassPermissions", "agent-full-access"} {
		if !ModeSkipsPermissionPrompt(mode) {
			t.Errorf("o modo %q desliga o pedido de permissão e não foi reconhecido", mode)
		}
	}
}

// O valor vem do agente pelo fio. Deixar de reconhecê-lo por causa da caixa ou
// de um espaço sobrando calaria justamente onde o silêncio custa caro.
func TestOModoEReconhecidoSemDependerDaCaixaOuDoEspaco(t *testing.T) {
	for _, mode := range []string{"  dontAsk  ", "DONTASK", "BypassPermissions", " AGENT-FULL-ACCESS "} {
		if !ModeSkipsPermissionPrompt(mode) {
			t.Errorf("o modo %q é um dos que dispensam a pergunta e não foi reconhecido", mode)
		}
	}
}

// A lista é de valores conhecidos, e não uma classificação de modos: o que não
// se sabe fica de fora. `acceptEdits` dispensa a pergunta só para edição; o
// `agent` do Codex conserva `approvalPolicy: on-request` e delega decisões à
// revisão automática. Avisar que a barreira caiu inteira seria falso nos dois.
func TestModoQueContinuaPerguntandoNaoEntraNaLista(t *testing.T) {
	for _, mode := range []string{"", "   ", "agent", "plan", "ask", "default", "auto", "acceptEdits"} {
		if ModeSkipsPermissionPrompt(mode) {
			t.Errorf("o modo %q foi tomado por um que desliga o pedido de permissão", mode)
		}
	}
}
