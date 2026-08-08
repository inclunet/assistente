package acpregistry

import (
	"slices"
	"strings"

	"assistente/internal/acp"
)

// detectable são os agentes que este app sabe procurar no disco, nomeados pelo
// `id` do registro (AEP-0086 D1 e D11).
//
// Não há tradução aqui, e é o ponto: desde que o `AgentKind` da detecção passou
// a ser o próprio `id` do registro, o app tem um nome só por agente. O que
// existe é esta lista de quem tem detector — que é curta por decisão, e não por
// falta de trabalho.
//
// Nenhum agente novo entra a partir do registro. Para os outros o app não sabe
// procurar, e é isso que a tela diz — em vez de alegar que procurou e não
// achou.
var detectable = []acp.AgentKind{
	acp.AgentKindCursor,
	acp.AgentKindClaudeCode,
}

// DetectableKind devolve o tipo de agente que a detecção sabe procurar para
// aquele `id` do registro. O `false` quer dizer que este app não tem detecção
// para o agente — o que é diferente de ter procurado e não encontrado.
func DetectableKind(id string) (acp.AgentKind, bool) {
	kind := acp.AgentKind(id)
	if slices.Contains(detectable, kind) {
		return kind, true
	}
	return "", false
}

// legacyProviderTypes traduz os tipos de provedor que existiram enquanto cada
// agente tinha o seu para o `id` da linha do registro (AEP-0086 D11, emenda).
//
// Ela ainda é necessária porque provedor com o vocabulário antigo continua
// chegando de fora: o banco de quem já tinha um é convertido na migração v12,
// mas um arquivo de exportação escrito antes da emenda pode ser importado
// depois, e nele o agente está nomeado no tipo.
var legacyProviderTypes = map[string]string{
	"cursor":      string(acp.AgentKindCursor),
	"claude-code": string(acp.AgentKindClaudeCode),
}

// LegacyProviderTypeAgentID devolve o `id` do registro que um tipo de provedor
// antigo nomeava. O `false` diz que aquele tipo nunca foi de agente.
//
// Quem chama precisa ter verificado antes que o provedor é um agente: `cursor`
// também é nome que alguém pode ter dado a um provedor HTTP, e converter aquilo
// em agente do registro inventaria configuração que ninguém pediu.
func LegacyProviderTypeAgentID(providerType string) (string, bool) {
	id, ok := legacyProviderTypes[strings.TrimSpace(providerType)]
	return id, ok
}

// DetectableKinds são os agentes que a detecção conhece, em ordem estável. Quem
// monta o catálogo procura uma vez por agente detectável, e não uma vez por
// linha: a procura vai ao sistema de arquivos, e repeti-la por linha custaria 38
// varreduras para responder sobre 2.
func DetectableKinds() []acp.AgentKind {
	kinds := slices.Clone(detectable)
	slices.Sort(kinds)
	return kinds
}
