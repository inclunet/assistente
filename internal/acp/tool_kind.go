package acp

import "strings"

// ToolKindOther é a classe de quem não se classifica.
const ToolKindOther = "other"

// toolKinds é o conjunto de classes do protocolo. O kind vira nome exibido,
// texto anunciado e chave de tradução, então aceitar qualquer string deixaria
// o agente escrever direto no anúncio do leitor de telas (AEP-0084 D7 e D11).
//
// O conjunto mora aqui, junto do protocolo, porque quem o consulta está em
// pacotes diferentes: o provider, ao nomear a ferramenta em andamento, e o
// aviso de permissão negada, ao dizer o que foi negado. Duas listas se
// afastariam, e a que ficasse para trás mostraria código cru na tela.
var toolKinds = map[string]struct{}{
	"read":        {},
	"edit":        {},
	"delete":      {},
	"move":        {},
	"search":      {},
	"execute":     {},
	"think":       {},
	"fetch":       {},
	"switch_mode": {},
	ToolKindOther: {},
}

// ToolKind normaliza a classe que o agente mandou. O que não for do conjunto
// conhecido vira "other": é o que quem exibe sabe traduzir.
func ToolKind(kind string) string {
	normalized := strings.ToLower(strings.TrimSpace(kind))
	if _, known := toolKinds[normalized]; known {
		return normalized
	}
	return ToolKindOther
}
