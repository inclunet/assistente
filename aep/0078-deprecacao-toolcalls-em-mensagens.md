# AEP-0078 — Deprecação de `tool_calls` em Mensagens

Status: Done — backfill, cutover e remoção física concluídos

## Resumo

Deprecar o legado L3 definido na AEP-0063: o campo `chat_messages.tool_calls` em mensagens `assistant`. A associação entre intenção de chamada e resultado deve passar a ser montada a partir de `tool_invocations.tool_call_id`, sem depender da ordem das mensagens nem de JSON embutido em `chat_messages`.

A compatibilidade histórica passa a ser uma fase finita. A
[AEP-0104](0104-tool-invocations-como-ledger-canonico.md) fará backfill,
cutover e remoção física; o estado final não mantém fallback de leitura.

## Motivação

`tool_invocations` é a trilha técnica canônica para chamadas de tools.
Timeline, exportação, sumarização, estatísticas e histórico consultam somente o
ledger; checkpoints concluídos estão em estado `canonical`.

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
- `conversation_id` e `turn_id` explícitos após o backfill;
- `tool_call_id = call_id`.

Durante a transição, o backfill aceitou os dois formatos históricos de
`origin_id`. Após o cutover, leitores usam os vínculos explícitos e nunca caem
para `chat_messages`.

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

### D3 — Leitores usam exclusivamente `tool_invocations`

Os leitores usam `tool_invocations` como única fonte técnica:

- timeline do chat em `internal/app/db.go`;
- exportação/hidratação em `internal/portability/service.go`;
- sumarização em `internal/summarization/service.go`;
- frontend que renderiza somente `turnSegments[].toolInvocations`.

### D4 — Escrita de L3 removida após a transição

Os leitores foram migrados e cobertos por testes canônicos. O agentic loop não
grava `chat_messages.tool_calls`; mensagens usam o snapshot em
`tool_invocations`. A migração v19 removeu fisicamente os campos após o
backfill.

Não existe fallback de execução associado a essa compatibilidade: todos os
executores exigem o ledger no wiring e falham antes de qualquer efeito quando a
auditoria canônica não está disponível. `role=tool` necessária ao protocolo do
LLM existe somente em memória no loop corrente.

### D5 — Dados antigos são migrados antes do cutover

Durante a janela finita de migração, o backfill aceitou:

- mensagens `assistant` com `tool_calls`;
- mensagens `role=tool` usadas como fallback;
- invocações ausentes por retenção ou por dados anteriores à AEP-0063.

O backfill retomável converte esses formatos em `tool_invocations`, valida
contagens/hashes e bloqueia o cutover em caso de ambiguidade. Em estado
`canonical`, nenhum leitor consulta esses formatos e as colunas foram
removidas.

## Mapa do estado implementado

| Área | Estado atual | Evidência |
|---|---|---|
| Persistência de chamadas | caminho feliz grava snapshot em `tool_invocations`, sem novo L3 | `internal/agent/agentic_loop.go` e testes do agentic loop |
| Timeline | transporta somente a projeção leve do ledger | `internal/chat/timeline.go`, `projection_read.go` e testes |
| Exportação | bloco `toolInvocations` canônico; protocolo técnico em mensagens é rejeitado | `internal/portability/service.go` e `service_test.go` |
| Sumarização | hidrata resultados somente do ledger | `internal/summarization/service.go` e `service_test.go` |
| Modelo persistido | `ChatMessage` contém somente campos conversacionais | `internal/database/models.go` |
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
- [x] A API canônica não consulta `chat_messages.tool_calls`.

### Fase 3 — Migrar timeline, export e sumarização

- [x] Timeline monta segmentos a partir de invocações hidratadas.
- [x] Portabilidade exporta dados novos por `tool_invocations`.
- [x] Sumarização usa resultados hidratados.
- [x] Testes cobrem o formato canônico e a rejeição do protocolo antigo.

### Fase 4 — Parar de gravar L3 em mensagens novas

- [x] Escrita de `toolCallsJSON` removida de mensagens novas no caminho feliz.
- [x] Fallback `role=tool` removido do runtime; falha de persistência não cria
      cópia em mensagens.
- [x] Reload, exportação e sumarização funcionam sem L3 novo.

### Fase 5 — Desencorajar uso novo do campo

- [x] `ChatMessage.ToolCalls` removido do modelo persistido.
- [x] Novos consumidores usam `tool_invocations`.
- [x] `EnrichedMessage.toolCalls` e o parser/consolidador persistido do frontend
      foram removidos; detalhes integrais usam binding batch user-scoped.
- [x] Remoção física entregue pela migração v19.

### Fase 6 — Backfill e remoção física ✅

- [x] Migrar todo L1/L3 publicado para o ledger com estado por conversa,
      contagens e hashes; ambiguidades permanecem `pending`.
- [x] Remover fallback de runtime.
- [x] Remover fallback de leitura e portabilidade após o cutover.
- [x] Reconstruir `chat_messages` sem `tool_calls`/`tool_call_id` e impedir
      `role=tool`.

## Riscos

| Risco | Impacto | Mitigação |
|---|---|---|
| Perda de legibilidade se a tool for removida do catálogo | Histórico pode mostrar apenas IDs técnicos | Snapshot de exibição em `tool_invocations` |
| Retenção de `tool_invocations` apagar dados necessários para conversas antigas | UI/export/sumarização ficariam incompletos | Enquanto L3 for removido de mensagens novas, retenção de invocações de chat deve acompanhar ciclo de vida da conversa ou manter snapshot suficiente |
| Divergência entre input técnico e argumentos exibíveis | UI pode mostrar dados redigidos demais ou sensíveis demais | Definir redaction única para snapshot exibível |
| Migração quebrar export de conversas antigas | Perda de portabilidade | Fixtures publicadas e roundtrip pelo bloco canônico antes do drop |
| Leitores ignorarem invocações migradas | Resultados antigos podem sumir da timeline/export | Backfill exige `conversation_id`/`turn_id`, contagens e hashes antes do cutover |
| Chamadas aninhadas/MCP ficarem sem ordenação clara | Timeline incorreta | Usar `parent_invocation_id`, iteração e timestamps como ordenação determinística |

## Critérios de aceitação

- [x] Conversas novas renderizam timeline por `tool_invocations`.
- [x] Exportação nova hidrata chamadas/resultados por invocações.
- [x] Sumarização não exige `m.ToolCalls`.
- [x] Conversas antigas são migradas para o ledger antes do render/export.
- [x] Testes cobrem formato canônico, upgrades publicados e rejeição do legado.
- [x] Agentic loop não grava L3 no caminho feliz nem orfana resultados.
- [x] Banco canônico não contém linhas `role=tool` nem colunas L3.
- [x] Código canônico não contém parser ou fallback de L1/L3.

As evidências individualizadas estão no mapa acima; regressões centrais:
`internal/agent/service_tool_calls_persistence_test.go`,
`internal/chat/timeline_test.go`, `internal/portability/service_test.go`,
`internal/summarization/service_test.go` e
`internal/toolinvocations/repository_test.go`.

## Relação com AEPs e Issues

- Depende da AEP-0063, especialmente do plano de transição L3.
- Complementa AEP-0039 para UX/eventos de tool calling.
- Desbloqueia a implementação da issue #190.
- Este AEP é o entregável da issue #191.
