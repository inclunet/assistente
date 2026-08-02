package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"assistente/internal/acp"
	"assistente/internal/logging"
)

// As instruções do perfil precisam chegar ao agente (AEP-0084 D4). O pipeline
// injeta persona, skills e blocos de contexto numa única mensagem `system`
// antes do StreamChat, e "mandar só a última mensagem do usuário" não pode
// significar descartá-las: seria o único provedor do barramento a ignorar o
// perfil.
//
// O ACP não tem papel `system` — `session/prompt` recebe blocos de conteúdo do
// usuário —, então as instruções vão como blocos delimitados, e a fronteira
// entre o que se repete e o que muda é a que o app já marca na mensagem para
// cache de prompt:
//
//   - o prefixo estável (persona, skills) vai uma vez por sessão;
//   - o que vem depois dele — resumo da conversa, memória, tasklists, contexto
//     de workspace — vai no turno em que mudar.
//
// O texto não é saneado: ele é do app e da pessoa que configurou o perfil, não
// do agente. Quem precisa de saneamento é o caminho de volta (D11).
const (
	acpProfileNotice = "Instruções do app para esta conversa. Não é mensagem da pessoa."
	acpProfileOpen   = "<app_instructions>"
	acpProfileClose  = "</app_instructions>"
	acpContextNotice = "Contexto do app para este turno. Não é mensagem da pessoa."
	acpContextOpen   = "<app_context>"
	acpContextClose  = "</app_context>"
)

// turnInstructions é o que este turno precisa contar ao agente, já decidido
// contra o que a sessão dele ouviu antes.
type turnInstructions struct {
	prefix     string
	prefixHash string
	suffix     string
	suffixHash string
}

// profileInstructions separa a mensagem de sistema nas duas partes que o app já
// distingue e descarta o que a sessão já ouviu.
func profileInstructions(messages []Message, conv *acp.Conversation) turnInstructions {
	stable, dynamic := splitSystemPrompt(messages)
	instructions := turnInstructions{}
	if stable != "" {
		if hash := textHash(stable); conv.NeedsPrefix(hash) {
			instructions.prefix, instructions.prefixHash = stable, hash
		}
	}
	if dynamic != "" {
		if hash := textHash(dynamic); conv.NeedsSuffix(hash) {
			instructions.suffix, instructions.suffixHash = dynamic, hash
		}
	}
	return instructions
}

// blocks devolve o que vai antes da mensagem da pessoa neste turno.
func (t turnInstructions) blocks() []acp.Content {
	var blocks []acp.Content
	if t.prefix != "" {
		blocks = append(blocks, acp.TextContent(delimited(acpProfileNotice, acpProfileOpen, t.prefix, acpProfileClose)))
	}
	if t.suffix != "" {
		blocks = append(blocks, acp.TextContent(delimited(acpContextNotice, acpContextOpen, t.suffix, acpContextClose)))
	}
	return blocks
}

// markSent registra o que o agente acabou de ouvir. Falhar aqui custa uma
// repetição no turno seguinte, e não o turno: por isso vira aviso, não erro.
func (t turnInstructions) markSent(ctx context.Context, conv *acp.Conversation) {
	if t.prefixHash != "" {
		if err := conv.MarkPrefixSent(ctx, t.prefixHash); err != nil {
			logging.Warnf(ctx, acpProviderComponent,
				"[ACP] instruções do perfil entregues, mas não anotadas: %v", err)
		}
	}
	if t.suffixHash != "" {
		conv.MarkSuffixSent(t.suffixHash)
	}
}

// splitSystemPrompt corta a mensagem de sistema na fronteira que o app já
// marca para cache de prompt: antes dela está o que se repete turno a turno,
// depois dela o que muda. Sem a marca, tudo conta como estável — e qualquer
// mudança no conjunto o reenvia inteiro, que é o comportamento seguro.
func splitSystemPrompt(messages []Message) (stable, dynamic string) {
	for _, message := range messages {
		if message.Role != "system" {
			continue
		}
		text := message.GetContentAsString()
		cut := message.SystemCacheControlPrefixLen
		if cut <= 0 || cut > len(text) {
			cut = len(text)
		}
		return strings.TrimSpace(text[:cut]), strings.TrimSpace(text[cut:])
	}
	return "", ""
}

func delimited(notice, open, body, closing string) string {
	var b strings.Builder
	b.WriteString(notice)
	b.WriteString("\n")
	b.WriteString(open)
	b.WriteString("\n")
	b.WriteString(body)
	b.WriteString("\n")
	b.WriteString(closing)
	return b.String()
}

func textHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
