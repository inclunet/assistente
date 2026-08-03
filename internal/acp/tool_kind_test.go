package acp

import "testing"

func TestClasseDoProtocoloAtravessaComoEla(t *testing.T) {
	for _, kind := range []string{"read", "edit", "delete", "move", "search", "execute", "think", "fetch", "switch_mode", "other"} {
		if got := ToolKind(kind); got != kind {
			t.Errorf("ToolKind(%q) = %q, quer a própria classe", kind, got)
		}
	}
}

func TestClasseEscritaDeQualquerJeitoAindaEhReconhecida(t *testing.T) {
	if got := ToolKind("  EXECUTE \n"); got != "execute" {
		t.Errorf("ToolKind = %q, quer %q", got, "execute")
	}
}

func TestClasseInventadaPeloAgenteNaoChegaAQuemLe(t *testing.T) {
	// O kind vira nome exibido e texto anunciado: aceitar qualquer string
	// deixaria o agente escrever direto no leitor de telas.
	for _, kind := range []string{"", "invocar_o_kraken", "\x1b[31mexecute"} {
		if got := ToolKind(kind); got != ToolKindOther {
			t.Errorf("ToolKind(%q) = %q, quer %q", kind, got, ToolKindOther)
		}
	}
}
