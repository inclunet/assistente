---
title: "Ferramentas"
weight: 12
---

# Ferramentas

O assistente expõe 15 famílias de tools ao LLM: filesystem, shell, web, feed, http, memory, tasklist, mcpserver, history, job, questionnaire, skillloader, subagent e outras. Cada tool tem baseline operacional por perfil e é auditável no histórico do turno.

## Histórico

As tools de histórico permitem localizar e recuperar contexto de conversas anteriores:

- `search_conversations` pesquisa mensagens e retorna trechos com seus IDs;
- `get_conversation_info` consulta os metadados e o resumo de uma conversa;
- `get_messages` recupera o conteúdo textual integral de até 20 mensagens pelos IDs. O parâmetro opcional `include_tool_results` inclui também os resultados de tools dos mesmos turnos.

`get_messages` respeita a conta autenticada: mensagens de outro usuário não são retornadas. Áudio e mídias binárias em base64 são omitidos para evitar payloads excessivos; `content`, `tool_calls` e `tool_call_id` permanecem disponíveis.

### Busca no histórico

A tool `search_conversations` faz busca textual nas mensagens do usuário. O
parâmetro `query` é obrigatório e aceita palavras, frases exatas entre aspas,
prefixos com `*` e os operadores `OR`, `AND` e `NOT`.

Por padrão, a busca é global: omitir `conversation_id` pesquisa todas as
conversas do usuário, preservando o comportamento das chamadas existentes.
Para limitar os resultados, informe o ID de uma conversa em
`conversation_id`. Dentro de um chat, também é possível usar o valor especial
`current`; nesse caso, a tool obtém com segurança a conversa corrente do
contexto da chamada. Se não houver uma conversa corrente disponível, a chamada
é rejeitada em vez de executar uma busca global.

Exemplos:

- `{"query": "autenticação JWT"}` — busca global.
- `{"query": "decisão final", "conversation_id": "019...", "limit": 10}` —
  busca apenas na conversa informada.
- `{"query": "próximos passos", "conversation_id": "current"}` — busca apenas
  na conversa em andamento.

Em todos os casos, os resultados permanecem restritos à conta autenticada.
