# AEP-0106 — Contenção do pipeline de jobs (limite de concorrência + cache por slug)

- **Status**: In Progress — Fase 1 (limite de concorrência) entregue; Fase 2 (cache de resolução por slug) pendente
- **Autor**: Leonardo Gleison Ferreira
- **Data**: 2026-09-16

## Resumo

O pipeline de jobs (scheduler + cadeias de evento) dispara execuções sem
qualquer teto de concorrência. O fan-out do `EventBus` cria uma goroutine por
handler, e cada run resolve job/trigger por slug e escreve em `job_runs`,
`job_run_events` e `tool_invocations`. Contra o único writer do SQLite (pool de
4, `busy_timeout` curto), a tempestade de writes concorrentes gera contenção que
deixou de ser apenas ruído e passou a **abortar execuções**.

Este AEP registra a decisão de tratar a contenção na raiz em duas frentes:

1. **Limitar a concorrência de execução** (Fase 1).
2. **Cachear a resolução job/trigger por slug** (Fase 2).

## Motivação

Evidência do `assistente.log` (sessão de ~13,5h, sem estas mudanças):

- **20.568** linhas `SLOW SQL >= 200ms`;
- **~550** ocorrências de `SQLITE_BUSY`/`database is locked`;
- **138** `context deadline exceeded` em `toolinvocations.service` (o
  `persistOpTimeout` de 3s estoura sob contenção ao persistir/marcar/completar a
  invocação);
- **144** `job attempt failed` em `jobs.executor` com
  `error="tool execute: tool invocation was not persisted"` — a cadeia
  Atlassian/CICD (`get-atlassian-cloud-id → search-cicddeliv-tikets →
  update-cicddeliv-tiket-statuses`) falhando por causa disso.

Ou seja: a contenção **quebra funcionalidade** (tools e jobs abortam), não é só
performance/ruído. A resolução `nome → tool_catalog_id`, antes o maior ofensor
de leitura, já foi cacheada pelo AEP-0104 (D1); restam a concorrência do fan-out
e a resolução job/trigger por slug.

## Decisões

### D1 — Teto de execuções automáticas concorrentes (Fase 1)

`Manager.executeJob` é o ponto comum por onde passam **todas** as execuções
automáticas: o scheduler (`NewScheduler(m.executeJob)`) e os handlers de trigger
de evento (`registerTriggersLocked`). Um semáforo de contagem (`runLimiter`,
canal com buffer) reserva um slot **após** os guards (job desabilitado ou tool
MCP indisponível não consomem slot) e o libera ao fim do run.

Propriedades:

- **Imutável após a criação** (criado no `NewManager`): pode ser lido no hot path
  sem lock, evitando qualquer deadlock com o `Stop` (que segura `m.mu` enquanto
  drena o `EventBus` via `wg.Wait`).
- **Sem deadlock de encadeamento**: o publicador (`EventBus.Publish`) é
  não-bloqueante e o run pai libera o slot ao retornar de `executor.Execute`
  (que publica o `*.success` e retorna sem esperar o run filho). O filho apenas
  aguarda um slot livre.
- **Aquisição sensível ao contexto**: se o `ctx` é cancelado antes do slot, o run
  aborta sem executar (não vaza slot, não trava o shutdown).
- **Configurável** via `ManagerConfig.MaxConcurrentRuns`; padrão
  `defaultMaxConcurrentRuns = 4`, alinhado ao `sqliteMaxOpenConns = 4`.

### D2 — Cache de resolução job/trigger por slug (Fase 2 — pendente)

`DBRepository.jobRowBySlug` (`WHERE user_id = ? AND slug = ?`) é chamado a cada
run (`LogRun` e afins) e aparece entre os sites quentes de contenção. A resolução
`(user_id, slug) → job` será cacheada em memória, escopada por usuário
(invariante análoga à do AEP-0104), com invalidação explícita nas mutações de job
(create/update/delete/enable/disable) e TTL curto como rede de segurança. A
resolução composta de trigger (`triggerIDForRun`) não será cacheada nesta fase
por ter chave composta e criar o trigger manual ausente — risco/superfície
maiores que o ganho.

## Fases

### Fase 1 — Limite de concorrência

- [x] `runLimiter` (semáforo por canal), `ManagerConfig.MaxConcurrentRuns` e
      `defaultMaxConcurrentRuns = 4`.
- [x] Aquisição/liberação em `executeJob`, após os guards, sensível ao contexto.
- [x] Testes: teto de concorrência, aborto com ctx cancelado, default para
      valores `<= 0`, release defensivo (`run_limiter_test.go`).

### Fase 2 — Cache de resolução por slug (pendente)

- [ ] Cache `(user_id, slug) → job` em `DBRepository`, escopado por usuário.
- [ ] Invalidação nas mutações de job + TTL curto.
- [ ] Testes de hit/miss, isolamento entre usuários e invalidação.

## Riscos

- **Throughput menor** sob muitos jobs independentes: mitigado pelo default 4
  (generoso para uso pessoal) e pela configurabilidade.
- **Shutdown mais lento** se houver muitos runs enfileirados: limitado porque,
  após `EventBus.Close`, nenhum novo fan-out é criado e os enfileirados drenam
  por slots que se liberam.
- **Staleness do cache por slug** (Fase 2): mitigado por invalidação nas
  mutações e TTL curto.

## Critérios de aceitação

- [x] Nenhuma execução automática ultrapassa `MaxConcurrentRuns` simultâneas.
- [x] Cancelamento de contexto ao aguardar slot não executa o run nem vaza slot.
- [ ] `jobRowBySlug` servido por cache user-scoped com invalidação nas mutações
      (Fase 2).
- [ ] Redução observável de `SLOW SQL`/`SQLITE_BUSY`/`context deadline exceeded`
      no log após deploy.

## Relações

Complementa o AEP-0001 (jobs event-driven) e o AEP-0104 (que já cacheou
`nome → tool_catalog_id`, o maior ofensor de leitura). Preserva o contrato de
shutdown gracioso do `EventBus`.
