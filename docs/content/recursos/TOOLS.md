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
