# AEP-0104 — Tool invocations como ledger canônico

**Status:** In Progress — contrato e baseline entregues; migração e cutover pendentes

## Resumo

`tool_invocations` é o ledger técnico único para execuções de tools originadas
por chat, jobs, dry-run, catálogo e MCP. `chat_messages` contém somente conteúdo
conversacional; `job_runs` contém somente estado operacional.

A transição é finita e termina com a remoção física de `tool_calls` e
`tool_call_id` de `chat_messages`, de `role=tool` persistida, e das cópias
`tool_name`, `inputs` e `output` de `job_runs`. O contexto enviado ao LLM pode
conter `role=tool` apenas em memória durante o loop corrente.

## Motivação

O contrato da AEP-0063 separou mensagens e execuções, mas o runtime ainda
mantém três fontes parciais:

- `chat_messages.tool_calls`, `chat_messages.tool_call_id` e linhas
  `role=tool`;
- `tool_invocations`, já preferida no caminho feliz;
- `job_runs.tool_name`, `job_runs.inputs` e `job_runs.output`.

Timeline, eventos, sumarização, estatísticas, retenção e portabilidade ainda
reconstroem ou duplicam a mesma execução. Isso aumenta payload, permite
divergência e torna a retenção perigosa.

## Invariantes

1. Toda execução persistível possui exatamente uma identidade em
   `tool_invocations`.
2. Falha ao persistir no ledger fecha a execução de chat; nunca autoriza gravar
   resultado em mensagem.
3. `chat_messages.role` persistida admite somente `user`, `assistant` e
   `system`.
4. Mensagens assistant intermediárias podem guardar fala e reasoning, mas não
   chamadas ou resultados técnicos.
5. A UI recebe uma projeção leve por `invocationId`; detalhes integrais são
   carregados em lote e sob demanda.
6. `job_run_events` permanece como timeline operacional. Definições de `jobs`
   mantêm tool e inputs porque representam configuração, não uma execução.
7. Compatibilidade de banco/import é uma etapa finita; estado canônico não
   consulta nem interpreta legado.
8. Toda consulta e cache de detalhes é escopado por usuário e origem.

## Decisões

### D1 — Identidade, ownership e vínculo de chat

`tool_invocations` ganha `conversation_id` e `turn_id` opcionais e indexados.
Para origem `chat`, ambos são obrigatórios após o backfill e pertencem à mesma
conversa do usuário. `origin_id` continua disponível para origens genéricas e
para a transição, mas não é a única forma de retenção de chat.

Cada linha preserva `tool_call_id`, `tool_catalog_id`, status, tentativas,
timestamps, input/output técnico e snapshot exibível redigido. Catálogo
archival é determinístico, indisponível para execução e existe apenas para
preservar identidade histórica quando a entrada original não pode ser
resolvida.

### D2 — Projeção leve

Janela e `turnPatch` retornam `ToolInvocationSummary` em
`turnSegments[].toolInvocations`, contendo:

- `invocationId`, `callId`, nome, origem e status;
- duração e ordem da iteração;
- previews limitados em bytes UTF-8;
- `hasDetails` e `resultAvailability`.

Output integral, input integral e metadata técnica não fazem parte desses
payloads. A representação escalar `message.toolCalls` é removida; não existe
duplicação entre `toolCalls` e `turnSegments`.

### D3 — Detalhes batch/lazy

Um binding batch recebe no máximo 100 IDs e executa uma consulta por lote. O
backend revalida ownership em cada chamada:

- chat: usuário é dono da conversa vinculada;
- job: usuário é dono do `job_run`;
- catálogo/system: `user_id` da invocação precisa coincidir.

O frontend usa LRU somente em memória, com chave `userId+invocationId`, TTL e
limite agregado de bytes. Logout/troca de usuário limpa o cache; eventos
terminais e deleções invalidam entradas. Nenhum payload técnico é persistido no
browser.

### D4 — Portabilidade

O export v2 ganha bloco aditivo `toolInvocations` dentro da conversa. Novos
exports não serializam `toolCalls` ou `toolCallId` em mensagens. O importador
aceita arquivos antigos e converte seus campos para invocações canônicas antes
de persistir. Rich export renderiza a projeção canônica.

### D5 — Máquina de estados de migração

O banco mantém estado explícito por conversa/run:

- `pending`: legado pode existir e o backfill não provou cobertura;
- `backfilled`: contagens e hashes conferem; escrita já é ledger-only;
- `canonical`: leitores legados estão proibidos e o schema físico foi
  reconstruído.

Estados só avançam. Ambiguidade, owner vazio, JSON inválido sem representação
segura ou diferença de hash bloqueiam o avanço e produzem diagnóstico sem
payload.

### D6 — Backfill retomável e idempotente

O backfill processa lotes transacionais e registra checkpoint. Para chat, casa
dados por usuário, conversa, turno, marcador assistant, `tool_call_id` e
iteração. Resultado `role=tool` íntegro prevalece sobre cópia embutida.
Invocações já existentes são adotadas, nunca duplicadas. Para jobs, a origem é
`job_run` e o `origin_id` é o ID do run.

Cada item registra proveniência de migração, tamanho e hashes normalizados de
input/output. Reiniciar retoma somente itens pendentes; executar novamente após
conclusão é no-op.

### D7 — Escrita exclusiva

O deploy da fase 3 é o ponto exato em que termina qualquer dual-write:

- `AddToolResultMessage` sai do runtime;
- não são persistidos `role=tool`, `tool_calls` ou `tool_call_id`;
- MCP nativo resolve entrada normal ou archival e falha fechado quando não
  puder registrar;
- retries atualizam a tentativa no ledger.

Representações `role=tool` necessárias ao protocolo LLM são montadas em memória
e descartadas ao concluir o loop.

### D8 — Retenção e exclusão

Invocações de chat acompanham a conversa por `conversation_id` explícito.
Invocações de jobs acompanham o run. Deleção, clear, import e retenção usam
transação e ownership; não inferem vínculo por JSON de mensagem. A varredura de
órfãos usa esses vínculos.

### D9 — Cutover e rollback

Antes do rebuild final, o app cria backup SQLite consistente, verifica checksum
e espaço livre. O gate exige:

- zero itens `pending`;
- zero ambiguidades;
- zero linhas `role=tool`;
- zero `tool_calls`/`tool_call_id` não vazios;
- igualdade de contagens e hashes;
- `foreign_key_check` vazio e `integrity_check=ok`.

O rebuild usa shadow tables e `INSERT` explícito em uma transação, recria
índices e FTS, e valida um segundo boot idempotente. Após o drop, rollback
suportado significa restaurar o backup; recriar colunas vazias é proibido.

### D10 — Observabilidade sem conteúdo sensível

Logs configuráveis `info`, `debug` e `trace`, escopados aos componentes do
ledger, registram fase, IDs locais, quantidade de linhas, bytes, duração,
estado e códigos de erro. Elevar o ledger não eleva o nível de outros
componentes. Nunca registram input, output, argumentos, conteúdo, credenciais
ou paths devolvidos por tools.

Métricas locais:

- `legacy_rows_remaining`, `backfill_ambiguous_total`;
- `backfill_rows`, `backfill_bytes`, `backfill_duration`;
- `canonical_cutover_blocked`;
- `timeline_query_count`, `timeline_latency`, `timeline_bytes`;
- `details_batch_size`, `details_latency`;
- `details_cache_hit`, `details_cache_miss`, `details_cache_eviction_bytes`;
- `invocation_persistence_failures`;
- `message_tool_payload_bytes`, que deve ser zero após a fase 3.

## Fases

### Fase 1 — Contrato, medição e governança

- [x] Registrar contrato, inventário, critérios e ordem de entrega.
- [x] Reabrir AEPs 0063 e 0078 como `In Progress`.
- [x] Manter AEP-0059 `In Progress` e registrar conteúdo pesado pendente.
- [x] Criar baseline reproduzível para 100/500/1000 mensagens e resultados de
      1 KiB/100 KiB/10 MiB.
- [x] Introduzir nível `trace` configurável e métricas locais sem payload.

### Fase 2 — Schema aditivo e backfill

- [ ] Adicionar vínculos, previews, snapshot, estado e checkpoints.
- [ ] Implementar catálogo archival e backfill de chat/jobs.
- [ ] Cobrir fresh DB, releases 0.1.9–0.5.0, dois usuários, crash/restart,
      lotes, idempotência e constraints.

### Fase 3 — Escrita ledger-only

- [ ] Remover fallbacks persistidos e fechar falhas de auditoria.
- [ ] Cobrir local, MCP bridge/nativo, retry, timeout, cancel, canais e
      subagentes.

### Fase 4 — Consumidores canônicos

- [ ] Migrar timeline, `turnPatch`, sumarização, token stats, busca, deleção,
      retenção e portabilidade.
- [ ] Manter parser legado apenas para estado `pending`.

### Fase 5 — Timeline leve e detalhes lazy

- [ ] Entregar projeção única, binding batch user-scoped e cache LRU.
- [ ] Cobrir offline, limites, invalidação, E2E e acessibilidade NVDA/teclado.

### Fase 6 — JobRun operacional

- [ ] Ler detalhes por join com invocações.
- [ ] Remover cópia técnica do run preservando eventos e resultado em memória
      para output map/emissão.

### Fase 7 — Cutover e remoção física

- [ ] Executar gate, backup e rebuild.
- [ ] Remover tipos, parsers, métodos, fallbacks e testes exclusivamente
      legados.
- [ ] Marcar AEPs 0063, 0078 e 0104 como `Done`.

## Entrega em PRs empilhados

1. `arquitetura/tool-invocations-canonicas` sobre `main`;
2. `db/backfill-tool-invocations` sobre a branch 1;
3. `refactor/tool-write-ledger-only` sobre a branch 2;
4. `refactor/tool-read-projections` sobre a branch 3;
5. `frontend/tool-details-lazy` sobre a branch 4;
6. `refactor/job-run-ledger` sobre a branch 5;
7. `db/drop-legacy-tool-message-fields` sobre a branch 6.

Merge deve ocorrer nessa ordem. Após cada merge, o próximo PR pode ser
retargetado para `main`.

## Riscos

- Associação errada no backfill: ambiguidades bloqueiam cutover.
- Catálogo ausente: entrada archival preserva identidade, sem reabilitar tool.
- Retenção apagar histórico: vínculo explícito e delete transacional.
- Payload continuar pesado: preview limitado e detalhe lazy.
- Cache cruzar usuários: chave composta e limpeza de sessão.
- Sumarização mudar semântica: golden tests de `ContentForModel` e tokens.
- NVDA perder cronologia: um item por turno, controles nomeados e foco
  restaurado.
- Rebuild impedir downgrade: backup verificado é o rollback suportado.

## Critérios de aceitação

- [ ] 100% do legado representado no ledger; ambiguidades iguais a zero.
- [ ] Contagens e hashes normalizados iguais antes/depois.
- [ ] `integrity_check=ok`, `foreign_key_check` vazio e segundo boot no-op.
- [ ] Todos os fluxos novos persistem somente no ledger.
- [ ] Estado `canonical` executa zero consultas/parsers legados.
- [ ] Janela usa quantidade constante de queries; detalhes usam uma query por
      lote de até 100 IDs e índice user+origem.
- [ ] Janela/patch não incluem output integral e p95 de bytes não ultrapassa o
      baseline sem tools + 2 KiB por invocação.
- [ ] p95 de janela não piora mais de 20% e respeita orçamento registrado pelo
      benchmark da fase 1.
- [ ] Frontend mantém uma representação persistida por segmentos.
- [ ] Axe sem violações; teclado, foco e roteiro NVDA validados.
- [ ] Timeline e detalhes persistidos funcionam offline.
- [ ] Logs e métricas não contêm payload ou segredo.
- [ ] Não resta dual-read, dual-write, campo físico ou fallback legado.

## Relações

Evolui as AEPs 0047, 0059, 0063, 0074-B e 0078. Preserva os contratos das
AEPs 0040, 0068, 0071, 0076, 0098 e 0102.
