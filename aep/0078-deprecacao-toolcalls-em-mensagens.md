# AEP-0078 — Deprecação de `tool_calls` em Mensagens

Status: Draft

## Resumo

Deprecar o legado L3 definido na AEP-0063: o campo `chat_messages.tool_calls` em mensagens `assistant`. A associação entre intenção de chamada e resultado deve passar a ser montada a partir de `tool_invocations.tool_call_id`, sem depender da ordem das mensagens nem de JSON embutido em `chat_messages`.

A mudança não remove leitura de dados antigos. Mensagens históricas com `tool_calls` continuam legíveis como fallback de compatibilidade.

## Motivação

`tool_invocations` já é a trilha técnica canônica para chamadas de tools, mas a UI, a exportação e a sumarização ainda usam `chat_messages.tool_calls` para descobrir quais chamadas pertencem a cada turno e como associar resultados por `tool_call_id`.

Isso mantém duas fontes parciais de verdade:

- `chat_messages.tool_calls`: intenção de chamada, nome legível, argumentos e metadados de exibição;
- `tool_invocations`: status, input/output técnico, duração, erro, `tool_catalog_id`, `tool_call_id` e origem.

Enquanto o L3 permanecer necessário para leitura, não é seguro parar de gravá-lo em mensagens novas.

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

### D4 — Escrita de L3 só para compatibilidade durante a transição

Enquanto todos os leitores não forem migrados, o agentic loop continua gravando `chat_messages.tool_calls`.

Depois da migração de leitura e dos testes de compatibilidade, mensagens novas podem parar de gravar `tool_calls`. A remoção física da coluna não faz parte desta AEP.

### D5 — Dados antigos continuam legíveis

Não haverá backfill destrutivo obrigatório. Leituras de conversas antigas devem continuar aceitando:

- mensagens `assistant` com `tool_calls`;
- mensagens `role=tool` usadas como fallback;
- invocações ausentes por retenção ou por dados anteriores à AEP-0063.

## Mapa de Dependências Atuais

| Área | Dependência atual de L3 | Caminho |
|---|---|---|
| Persistência de chamadas | grava `assistant` com JSON enriquecido em `tool_calls` | `internal/agent/agentic_loop.go` |
| Timeline | faz parse de `message.ToolCalls`, consolida segmentos e injeta resultados de `tool_invocations` | `internal/app/db.go` |
| Exportação | procura turnos com `msg.ToolCalls`, hidrata `result` e reserializa JSON | `internal/portability/service.go` |
| Sumarização | usa `m.ToolCalls` para listar resultados de tools no prompt de resumo | `internal/summarization/service.go` |
| Modelo persistido | `ChatMessage.ToolCalls` documentado como JSON de chamadas solicitadas | `internal/database/models.go` |
| Trilha técnica | já possui `ToolCallID`, `ParentInvocationID`, `Input`, `Output` e `Metadata` | `internal/database/models_tool_invocations.go` |

## Fases

### Fase 1 — Snapshot de exibição em `tool_invocations`

1. Definir schema para os dados exibíveis da chamada, preferencialmente em campos explícitos se forem consultados frequentemente, ou em `metadata` versionado se permanecerem auxiliares.
2. Atualizar gravação em `internal/agent/agentic_loop.go` e `internal/toolinvocations` para preencher o snapshot junto da invocação.
3. Garantir redaction consistente com o que hoje vai para `tool_calls`.

### Fase 2 — APIs de leitura por turno

1. Criar função de repository para listar invocações de chat por `turn_id`, aceitando também `assistant_message_id` legado como `origin_id`, ordenadas por iteração/tempo.
2. Retornar DTO de exibição contendo `tool_call_id`, nome, argumentos redigidos, output, status, erro, duração e metadados MCP.
3. Cobrir ausência de invocações com fallback para `chat_messages.tool_calls`.

### Fase 3 — Migrar timeline, export e sumarização

1. Migrar `internal/app/db.go` para montar `TurnSegmentToolCall` a partir dos DTOs de invocação.
2. Migrar `internal/portability/service.go` para exportar chamadas/resultados por `tool_invocations`, usando `tool_calls` só para dados antigos.
3. Migrar `internal/summarization/service.go` para construir o prompt a partir de invocações hidratadas, sem exigir `m.ToolCalls`.
4. Atualizar testes de timeline, export/import e sumarização cobrindo dados novos e legados.

### Fase 4 — Parar de gravar L3 em mensagens novas

1. Remover a escrita de `toolCallsJSON` em novas mensagens `assistant` do caminho feliz.
2. Manter fallback `role=tool` apenas para falha de persistência técnica, conforme AEP-0063.
3. Garantir que reload de conversa, export e sumarização funcionem sem `chat_messages.tool_calls`.

### Fase 5 — Desencorajar uso novo do campo

1. Documentar `ChatMessage.ToolCalls` como legado de leitura.
2. Evitar novos consumidores frontend/backend do campo.
3. Avaliar, em AEP futura, se a coluna pode permanecer indefinidamente ou se precisa de migração/removal.

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
