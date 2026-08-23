# AEP-0078 — Deprecação de `tool_calls` em Mensagens

Status: Done — escrita L3 removida do caminho feliz; leitores usam `tool_invocations` com fallback legado

## Resumo

Deprecar o legado L3 definido na AEP-0063: o campo `chat_messages.tool_calls` em mensagens `assistant`. A associação entre intenção de chamada e resultado deve passar a ser montada a partir de `tool_invocations.tool_call_id`, sem depender da ordem das mensagens nem de JSON embutido em `chat_messages`.

A mudança não remove leitura de dados antigos. Mensagens históricas com `tool_calls` continuam legíveis como fallback de compatibilidade.

## Motivação

`tool_invocations` já é a trilha técnica canônica para chamadas de tools, mas a UI, a exportação e a sumarização ainda usam `chat_messages.tool_calls` para descobrir quais chamadas pertencem a cada turno e como associar resultados por `tool_call_id`.

Isso mantém duas fontes parciais de verdade:

- `chat_messages.tool_calls`: intenção de chamada, nome legível, argumentos e metadados de exibição;
- `tool_invocations`: status, input/output técnico, duração, erro, `tool_catalog_id`, `tool_call_id` e origem.

No baseline anterior à implementação, o L3 ainda era necessário para leitura e
por isso não era seguro parar de gravá-lo. A migração preservou essa leitura
somente como fallback para dados históricos.

## Decisões

### D1 — `tool_invocations.tool_call_id` é a chave de associação

Leitores novos associam chamada e resultado por `tool_invocations.tool_call_id`, filtrando por origem de chat:

- `origin_type = chat`;
- `origin_id = turn_id` para invocações novas;
- `origin_id = assistant_message_id` como fallback legado para dados já gravados nesse formato;
- `tool_call_id = call_id`.

Durante a transição, consultas de hidratação devem aceitar os dois formatos de `origin_id`. A implementação pode preferir `turn_id` e, quando não houver resultado, buscar por IDs de mensagens assistant do mesmo turno antes de cair para `chat_messages.tool_calls`.

`parent_invocation_id` fica reservado para chamadas aninhadas, encadeadas ou executadas por MCP quando houver relação técnica entre invocações.

### D2 — `tool_invocations` precisa carregar snapshot de exibição

A AEP-0063 decidiu que `tool_catalog_id` é a referência canônica da tool e que `tool_invocations.input` guarda o input normalizado/redigido. Para eliminar o L3 sem degradar UI/export, `tool_invocations` precisa expor dados estáveis de exibição por invocação.

Adicionar, em fase própria, campos ou metadata versionada para:

- nome lógico exibível da tool;
- argumentos exibíveis/redigidos;
- origem MCP/nativa;
- `server_label`, quando aplicável;
- número de iteração do agentic loop;
- duração já existente, quando disponível.

O catálogo continua sendo a fonte canônica de identidade da tool. O snapshot evita que histórico antigo perca legibilidade se o catálogo mudar ou se o input técnico for redigido demais para UI.

### D3 — Leitores devem preferir `tool_invocations`

Os leitores passam a usar `tool_invocations` como fonte primária e `chat_messages.tool_calls` apenas como fallback legado:

- timeline do chat em `internal/app/db.go`;
- exportação/hidratação em `internal/portability/service.go`;
- sumarização em `internal/summarization/service.go`;
- frontend que renderiza `toolCalls` quando receber payload legado.

### D4 — Escrita de L3 removida após a transição

Os leitores foram migrados e cobertos por testes de compatibilidade. O agentic
loop não grava `chat_messages.tool_calls` no caminho feliz; mensagens novas usam
o snapshot em `tool_invocations`. O campo permanece apenas para leitura de dados
históricos, e sua remoção física não faz parte desta AEP.

### D5 — Dados antigos continuam legíveis

Não haverá backfill destrutivo obrigatório. Leituras de conversas antigas devem continuar aceitando:

- mensagens `assistant` com `tool_calls`;
- mensagens `role=tool` usadas como fallback;
- invocações ausentes por retenção ou por dados anteriores à AEP-0063.

## Mapa do estado implementado

| Área | Estado atual | Evidência |
|---|---|---|
| Persistência de chamadas | caminho feliz grava snapshot em `tool_invocations`, sem novo L3 | `internal/agent/agentic_loop.go` e testes do agentic loop |
| Timeline | hidrata chamadas e resultados por invocações, com fallback para mensagens antigas | `internal/chat/timeline.go` e `timeline_test.go` |
| Exportação | usa invocações para dados novos e preserva fallback legado | `internal/portability/service.go` e `service_test.go` |
| Sumarização | inclui resultados hidratados sem exigir `m.ToolCalls` | `internal/summarization/service.go` e `service_test.go` |
| Modelo persistido | `ChatMessage.ToolCalls` permanece somente para leitura compatível | `internal/database/models.go` |
| Trilha técnica | `ToolCallID`, `ParentInvocationID`, `Input`, `Output` e `Metadata` são canônicos | `internal/database/models_tool_invocations.go` |

## Fases

### Fase 1 — Snapshot de exibição em `tool_invocations`

- [x] Schema e metadata de exibição definidos em `tool_invocations`.
- [x] Agentic loop e `internal/toolinvocations` preenchem o snapshot.
- [x] Redaction permanece no pipeline compartilhado de invocações.

### Fase 2 — APIs de leitura por turno

- [x] Repository lista invocações por turno/origem em ordem determinística.
- [x] DTO de exibição contém identificação, input redigido, output, status,
      erro, duração e metadata.
- [x] Ausência de invocações usa fallback para `chat_messages.tool_calls`.

### Fase 3 — Migrar timeline, export e sumarização

- [x] Timeline monta segmentos a partir de invocações hidratadas.
- [x] Portabilidade exporta dados novos por `tool_invocations`.
- [x] Sumarização usa resultados hidratados.
- [x] Testes cobrem formatos novo e legado.

### Fase 4 — Parar de gravar L3 em mensagens novas

- [x] Escrita de `toolCallsJSON` removida de mensagens novas no caminho feliz.
- [x] Fallback `role=tool` restrito a falha de persistência técnica.
- [x] Reload, exportação e sumarização funcionam sem L3 novo.

### Fase 5 — Desencorajar uso novo do campo

- [x] `ChatMessage.ToolCalls` documentado como legado de leitura.
- [x] Novos consumidores usam `tool_invocations`.
- [x] Remoção física da coluna foi explicitamente deixada para migração futura;
      isso não reabre a deprecação funcional.

## Riscos

| Risco | Impacto | Mitigação |
|---|---|---|
| Perda de legibilidade se a tool for removida do catálogo | Histórico pode mostrar apenas IDs técnicos | Snapshot de exibição em `tool_invocations` |
| Retenção de `tool_invocations` apagar dados necessários para conversas antigas | UI/export/sumarização ficariam incompletos | Enquanto L3 for removido de mensagens novas, retenção de invocações de chat deve acompanhar ciclo de vida da conversa ou manter snapshot suficiente |
| Divergência entre input técnico e argumentos exibíveis | UI pode mostrar dados redigidos demais ou sensíveis demais | Definir redaction única para snapshot exibível |
| Migração quebrar export de conversas antigas | Perda de portabilidade | Fallback explícito para `chat_messages.tool_calls` e `role=tool` |
| Leitores novos ignorarem invocações legadas por `assistant_message_id` | Resultados antigos podem sumir da timeline/export | Consultas de transição aceitam `turn_id` e IDs de mensagens assistant do turno |
| Chamadas aninhadas/MCP ficarem sem ordenação clara | Timeline incorreta | Usar `parent_invocation_id`, iteração e timestamps como ordenação determinística |

## Critérios de aceitação

- Conversas novas com tools renderizam timeline sem depender de `chat_messages.tool_calls`.
- Exportação de conversas novas inclui chamadas e resultados a partir de `tool_invocations`.
- Sumarização inclui resultados relevantes de tools sem exigir `m.ToolCalls`.
- Conversas antigas com `tool_calls` continuam renderizando e exportando corretamente.
- Testes cobrem os dois formatos: novo (`tool_invocations`) e legado (`chat_messages.tool_calls`).
- O agentic loop pode parar de gravar L3 no caminho feliz sem orfanar resultados.

## Relação com AEPs e Issues

- Depende da AEP-0063, especialmente do plano de transição L3.
- Complementa AEP-0039 para UX/eventos de tool calling.
- Desbloqueia a implementação da issue #190.
- Este AEP é o entregável da issue #191.
