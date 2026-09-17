# AEP-0104 — Tool invocations como ledger canônico

**Status:** Done — ledger exclusivo, backfill e cutover físico entregues
(endurecido para bancos legados grandes: `hash_mismatch` é aviso de auditoria
não bloqueante e o `foreign_key_check` do cutover é escopado às tabelas
reconstruídas — ver D5, D9 e "Endurecimento da migração v18/v19")

## Resumo

`tool_invocations` é o ledger técnico único para execuções de tools originadas
por chat, jobs, dry-run, catálogo e MCP. `chat_messages` contém somente conteúdo
conversacional; `job_runs` contém somente estado operacional.

A transição é finita e termina com a remoção física de `tool_calls` e
`tool_call_id` de `chat_messages`, de `role=tool` persistida, e das cópias
`tool_name`, `inputs` e `output` de `job_runs`. O contexto enviado ao LLM pode
conter `role=tool` apenas em memória durante o loop corrente.

## Motivação

O contrato da AEP-0063 separou mensagens e execuções, mas o baseline anterior
à implementação mantinha três fontes parciais:

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

A resolução `nome -> tool_catalog_id` roda a CADA invocação (chat e jobs) e,
sob carga de jobs, o `SELECT` correspondente vira o maior ofensor de contenção
do writer SQLite (observado até ~80s e `context deadline exceeded`). O
`toolinvocations.DBRepository` mantém um cache em memória desse mapeamento,
alinhado à invariante 8 (escopo por usuário):

- chave composta `(user_id, nome)` — nenhuma entrada de um usuário é servida a
  outro;
- só entradas **positivas e não-archival** são cacheadas — o mapeamento é
  estável (upsert reusa o mesmo ID; detach preserva a linha), enquanto archival
  é placeholder que a ordenação suplanta quando a tool real aparece;
- "não encontrado" nunca é cacheado (tool nova resolve na próxima chamada);
- TTL curto (60s) limita a janela de staleness em eventos raros de
  exclusão+recriação de linha do catálogo.

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

A consolidação ordena rodadas pelo número de iteração (incluindo zero), não
por chave textual ou ID de chamada. Invocações sem fala associada continuam
entre as rodadas e antes da conclusão; chamadas paralelas mantêm a ordem
`queued_at,id` da projeção. A fala vinculada a uma rodada precede suas tools.
O registro final pode ter sido criado antes do loop: sem usage, a seleção
considera sua atualização persistida, não somente `created_at`. Textos legados
sem vínculo preservam sua ordem relativa antes da próxima fala vinculada; não
se fabricam timestamps ou mensagens para preencher informação ausente.

Evidências: regressões de `internal/chat/timeline_test.go`, leitura real em
`internal/app/db_chronology_test.go` e ordem do patch terminal em
`internal/agent/service_stats_test.go`. O contrato permanece **Done**; não há
alteração de schema nem nova representação persistida.

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

O export v2 usa o bloco `toolInvocations` dentro da conversa e nunca serializa
`toolCalls` ou `toolCallId` em mensagens. Importações com `role=tool`,
`toolCalls` ou `toolCallId` embutidos em mensagens são rejeitadas: a
compatibilidade existiu somente durante a janela de migração de bancos e não é
um contrato permanente de portabilidade. Rich export renderiza a projeção
canônica.

### D5 — Máquina de estados de migração

O banco mantém estado explícito por conversa/run:

- `pending`: legado pode existir e o backfill não provou cobertura;
- `backfilled`: histórico foi representado e contagens/hashes conferem; a
  escrita de compatibilidade só termina no deploy da fase 3;
- `canonical`: leitores legados estão proibidos e o schema físico foi
  reconstruído.

Estados avançam monotonicamente quando o conjunto legado não muda. Durante a
janela transitória entre as fases 2 e 3, uma nova escrita legada invalida a
prova anterior: o recurso volta de `backfilled` para `pending` até o backfill
incremental conferir o novo conjunto. Ambiguidade, owner vazio, JSON inválido
sem representação segura ou perda real de linhas (`count_mismatch`) bloqueiam o
avanço e produzem diagnóstico sem payload.

**Divergência apenas de hash (`hash_mismatch`) é aviso de auditoria não
bloqueante.** Quando os dados estão presentes (contagens conferem) e só o hash
diverge, a reconciliação conclui o recurso (`backfilled`) preservando
`last_error_code='hash_mismatch'` para auditoria e emitindo um `warn` sem
payload. O motivo é estrutural: para invocações já existentes, o ledger contém
o dado autoritativo de runtime (envelope canônico `{"content":…}` ou output já
gravado), que legitimamente não coincide, por hash, com o output legado bruto
do recurso. Comparar `digestValue` (legado bruto) com `digestCanonicalOutput`
(ledger) é comparar representações normalizadas de formas diferentes, então a
divergência é sistemática e benigna. Manter esses recursos `pending` para
sempre travava a inicialização em bancos grandes com histórico. `count_mismatch`
e ambiguidade continuam bloqueando por representarem perda ou associação
duvidosa.

### D6 — Backfill retomável e idempotente

O backfill processa lotes transacionais e registra checkpoint. Para chat, casa
dados por usuário, conversa, turno, marcador assistant, `tool_call_id` e
iteração. Resultado `role=tool` íntegro prevalece sobre cópia embutida.
Não existe associação global somente por `tool_call_id`; identidade ou turno
ausente bloqueia a prova. Invocações já existentes são adotadas, nunca
duplicadas. Para jobs, a origem é `job_run` e o `origin_id` é o ID do run.
Checkpoint cujo recurso foi excluído é removido transacionalmente.

Cada item registra proveniência de migração, tamanho e hashes normalizados de
input/output. Reiniciar retoma somente itens pendentes; executar novamente após
conclusão é no-op. Enquanto a fase 3 não encerra os escritores legados, o boot
também executa a varredura idempotente depois do registro da v18; assim, dados
criados no intervalo entre os deploys 2 e 3 entram no ledger antes do corte.

### D7 — Escrita exclusiva

O deploy da fase 3 é o ponto exato em que termina qualquer dual-write:

- `ToolInvocationService` é dependência obrigatória de todo executor de tool;
  wiring sem ledger falha durante a montagem/inicialização;
- uma defesa runtime adicional falha antes de qualquer efeito caso uma
  instância inválida seja construída fora dos construtores;
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
- zero linhas ou payloads técnicos sem representação integral no ledger;
- igualdade de contagens (perda de linhas = `count_mismatch` = bloqueio);
  divergência apenas de hash é aviso de auditoria e não bloqueia (ver D5);
- `integrity_check=ok` e `foreign_key_check` **escopado às tabelas
  reconstruídas pelo cutover** (`chat_messages` e `job_runs`).

O `foreign_key_check` é escopado de propósito: o cutover só reconstrói
`chat_messages` e `job_runs`, então um check GLOBAL abortaria por órfãos
PRÉ-EXISTENTES em tabelas não tocadas (ex.: `chat_tabs`→`conversations`,
`http_endpoints`→`http_agents`), problemas alheios ao ledger que travariam a
migração. `integrity_check` permanece global.

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

- [x] Adicionar vínculos, previews estruturais sem valores, snapshot, estado e
      checkpoints (`tool_ledger_migration_states`).
- [x] Implementar catálogo archival indisponível e backfill de chat/jobs.
- [x] Cobrir fresh DB, releases 0.1.9–0.5.0, dois usuários, crash/restart,
      lotes, idempotência e constraints.

Evidências: migração v18, fixtures publicadas com chamada/resultado técnico,
testes `TestToolLedgerBackfill*`, `TestPublishedDatabase019*`,
`TestPublishedReleaseDatabases*`, `foreign_key_check`, `integrity_check` e
segundo boot sem duplicação. A AEP permanece `In Progress`: a fase 3 ainda
precisa encerrar a escrita de compatibilidade.

### Fase 3 — Escrita ledger-only

- [x] Remover fallbacks persistidos e fechar falhas de auditoria antes da
      execução; falha posterior de `Complete` nunca cria cópia em mensagens.
- [x] Cobrir local, MCP bridge/nativo, retry, timeout, cancel, canais e
      subagentes.

Evidências: runtime não chama `AddToolResultMessage`; execução sem repositório
ou sem `Create` falha antes do efeito; catálogo ausente gera entrada archival
user-scoped; MCP nativo não cria marcador técnico/fallback; retries recebem
`attempt` crescente; inserts de chat derivam `conversation_id` e `turn_id`
transacionalmente do owner. Testes em `internal/toolinvocations` e
`internal/agent` cobrem os caminhos, garantem previews sem valores e exercitam
o contador `tool_invocation_persistence_failures_total`.

Token stats contam iterações técnicas por `(conversation_id, turn_id,
model_iteration)` no ledger. A migração v20 materializa `model_iteration` e
`external` a partir do metadata já migrado, e os escritores mantêm ambos no
ledger. A contagem usa índices parciais por conversa/turno, sem interpretar
JSON nem consultar `chat_messages`. Por isso, iterações locais sem texto não
precisam criar uma linha `assistant` vazia nem qualquer marcador técnico em
`chat_messages`.

### Fase 4 — Consumidores canônicos

- [x] Migrar timeline, `turnPatch`, sumarização, token stats, busca, deleção,
      retenção e portabilidade.
- [x] Manter parser legado apenas durante o estado `pending`; removê-lo no
      cutover final.

Evidência: o gate user-scoped é resolvido em lote por conversa; timeline,
`turnPatch`, sumarização, estatísticas e a tool de histórico ignoram L1/L3
quando o checkpoint está `backfilled`. Exclusão usa os vínculos explícitos do
ledger. O export v2 grava `toolInvocations`, o import converte arquivos legados
antes de persistir e o roundtrip preserva payloads, IDs, tentativas e
ownership; HTML/PDF/Markdown projetam as invocações apenas durante o render.

### Fase 5 — Timeline leve e detalhes lazy

- [x] Entregar projeção única, binding batch user-scoped e cache LRU.
- [x] Cobrir offline, limites, invalidação, E2E e acessibilidade NVDA/teclado.

Evidência: janela e `turnPatch` transportam somente
`turnSegments[].toolInvocations`; `EnrichedMessage.toolCalls` e a consolidação
persistida paralela do frontend foram removidos. `GetToolInvocationDetails`
aceita até 100 IDs, faz uma consulta por lote e revalida ownership da conversa
ou run. O cache em memória usa chave `userId+invocationId`, TTL de cinco
minutos, LRU limitado a 4 MiB, coalescing e limpeza em logout, troca de usuário,
patch terminal e exclusão. Testes de projeção provam ausência de payload
integral, quantidade constante de queries e filtro entre usuários;
`ToolCallsSection` cobre carregamento sob demanda, teclado e axe, e o cenário
Playwright cobre prévia seguida de detalhe integral. A leitura usa SQLite local,
sem dependência de rede.

### Fase 6 — JobRun operacional

- [x] Ler detalhes por join com invocações.
- [x] Remover cópia técnica do run preservando eventos e resultado em memória
      para output map/emissão.

Evidência: `NewJobExecutor` rejeita montagem sem registry ou ledger, e
`executeTool` mantém defesa fail-closed sem chamar a tool. `LogRun` grava apenas
estado operacional, trigger, eventos, duração e erro; tool, input redigido e
output são hidratados em lote a partir de `tool_invocations` para consulta e
replay. Testes garantem zero efeito sem ledger, ausência de cópia técnica em
`job_runs` e roundtrip de detalhes exclusivamente pelo vínculo do ledger.

### Fase 7 — Cutover e remoção física

- [x] Executar gate, backup e rebuild.
- [x] Remover tipos, parsers, métodos, fallbacks e testes exclusivamente
      legados.
- [x] Marcar AEPs 0063, 0078 e 0104 como `Done`.

Evidência: a migração v19 só avança depois da reconciliação integral da v18,
cria backup SQLite com manifesto e SHA-256, reconstrói `chat_messages` e
`job_runs` com o schema emitido pelo GORM, recria FTS/índices, executa
`integrity_check`/`foreign_key_check` e promove checkpoints para `canonical`
na mesma transação. Fixtures 0.1.9–0.5.0 provam upgrade direto, restauração do
backup e segundo boot sem alteração. Modelos, repositories, timeline,
sumarização, estatísticas, histórico e portabilidade não contêm dual-read ou
dual-write; `CreateMessageWithContext` rejeita `role=tool`.

## Endurecimento da migração v18/v19 (bancos legados grandes)

Em bancos grandes de produção com histórico (evidência real: ~2,8 GB, ~9.264
recursos), o cutover travava a inicialização e deixava o usuário preso em
"Autenticação indisponível" (o `InitDatabase` abortava de forma fatal antes de
subir os serviços de auth). Duas causas, ambas benignas quanto à integridade
dos dados:

1. **`hash_mismatch` sistemático em recursos já migrados.** 9.103 recursos
   (9.052 `job_run` + 49 `conversation`) ficaram `pending` com
   `hash_mismatch`, todos com `legacy_rows == ledger_rows` — os dados FORAM
   migrados; só o hash divergia. A causa é comparar o hash do output legado
   bruto (`digestValue`) com o hash do output canônico/runtime do ledger
   (`digestCanonicalOutput`), representações normalizadas de formas diferentes
   (ver D5). Correção: `hash_mismatch` passou a ser terminal (`backfilled`)
   preservando `last_error_code` para auditoria e emitindo `warn`; o gate da
   v18 e o gate da v19 deixaram de bloquear por hash/digest. `count_mismatch`
   e ambiguidade continuam bloqueando.
2. **`foreign_key_check` GLOBAL abortando por órfãos alheios.** A validação da
   v19 achava 32 órfãos PRÉ-EXISTENTES em tabelas fora do cutover
   (`chat_tabs`→`conversations`, `http_endpoints`→`http_agents`). Correção: o
   `foreign_key_check` foi escopado às tabelas efetivamente reconstruídas
   (`chat_messages` e `job_runs`); `integrity_check` permanece global (ver D9).

Evidência (testes em `internal/database`):

- `TestToolLedgerBackfillDivergenciaDeHashConcluiSemSobrescreverLedger`:
  recurso só com `hash_mismatch` conclui (`backfilled`), preserva o código,
  não sobrescreve o ledger e não bloqueia o gate da v19.
- `TestToolLedgerBackfillCountMismatchContinuaBloqueando`: `count_mismatch`
  segue pendente e bloqueia o gate.
- `TestValidateCutoverIgnoraOrfaoEmTabelaNaoLedger` e
  `TestValidateCutoverFalhaComOrfaoEmTabelaDoCutover`: órfão em tabela
  não-ledger não aborta o cutover; órfão em `chat_messages`/`job_runs` ainda
  aborta.
- Fixtures publicadas (`TestPublishedReleaseDatabasesUpgradeDirectlyAndIdempotently`,
  `TestPublishedReleaseCutoverCreatesRestorableBackup`) permanecem verdes: nelas
  os hashes conferem e o cutover conclui como antes.

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

- [x] 100% do legado representado no ledger; ambiguidades iguais a zero.
- [x] Contagens iguais antes/depois; divergência apenas de hash é registrada
      como aviso de auditoria (`hash_mismatch`), não bloqueia (ver D5).
- [x] `integrity_check=ok`, `foreign_key_check` das tabelas reconstruídas
      (`chat_messages`, `job_runs`) vazio e segundo boot no-op.
- [x] Todos os fluxos novos persistem somente no ledger.
- [x] Estado `canonical` executa zero consultas/parsers legados.
- [x] Janela usa quantidade constante de queries; detalhes usam uma query por
      lote de até 100 IDs e índice user+origem.
- [x] Janela/patch não incluem output integral e p95 de bytes não ultrapassa o
      baseline sem tools + 2 KiB por invocação.
- [x] p95 de janela não piora mais de 20% e respeita orçamento registrado pelo
      benchmark da fase 1.
- [x] Frontend mantém uma representação persistida por segmentos.
- [x] Axe sem violações; teclado, foco e roteiro NVDA validados.
- [x] Timeline e detalhes persistidos funcionam offline.
- [x] Logs e métricas não contêm payload ou segredo.
- [x] Não resta dual-read, dual-write, campo físico ou fallback legado.

## Relações

Evolui as AEPs 0047, 0059, 0063, 0074-B e 0078. Preserva os contratos das
AEPs 0040, 0068, 0071, 0076, 0098 e 0102.
