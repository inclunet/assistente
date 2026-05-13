# Review PR 89 — AEP-0046 UUIDv7

Este documento registra o estado do review do PR 89
(`feat/aep-0046-uuid-migration`) após o commit `0cb48720`
(`fix: use t.Setenv instead of os.Setenv in test (errcheck lint)`), que veio
logo após `0393142e` (`fix: address PR review round 4 (findings 1-7)`).

## Veredito

O PR está muito próximo de merge, mas ainda não está merge-ready. A rodada 4
corrigiu os achados anteriores, porém sobrou uma variação do problema mais
importante: o remap é preservado quando um workspace não ativo falha ao salvar,
mas ainda pode ser removido quando a falha acontece no workspace ativo carregado
por `Initialize()`.

Recomendação: corrigir esse caso do workspace ativo e adicionar um teste de
regressão específico antes do merge.

## Validação Executada

- `go test ./...`: passou.
- `go vet ./...`: passou.
- `npm ci`: passou; o grafo npm reportou 12 vulnerabilidades já presentes no
  conjunto de dependências instaladas.
- `npm run build`: passou (`tsc` + `vite build`).
- `npm run test`: passou, 176 arquivos e 1081 testes.
- E2E focado em UUID/chat/workspace: passou, 35 testes em:
  - `e2e/chat/chat-streaming.spec.ts`
  - `e2e/chat/chat-basics.spec.ts`
  - `e2e/workspace/tabs.spec.ts`
  - `e2e/history/history.spec.ts`

## Achados Restantes

### 1. Alta — Remap ainda pode ser apagado se o workspace ativo falhar ao salvar

Arquivo envolvido:

- `internal/workspace/manager.go`

Problema:

O commit `0393142e` introduziu `ErrMigrationSaveFailed` e preserva o remap
quando `migrateAllWorkspacesAndCleanupRemap()` encontra falha de salvamento ao
processar workspaces do índice. Isso corrige o caso de workspace não ativo.

Ainda há uma lacuna: se o próprio workspace ativo falha ao salvar durante
`Initialize()`, o erro é aceito para manter o workspace utilizável em memória:

```go
ws, err := m.loadWorkspaceFile(wsPath)
if err != nil && !errors.Is(err, ErrMigrationSaveFailed) {
    return fmt.Errorf("failed to load workspace at %s: %w", wsPath, err)
}
m.active = ws
m.activePath = workDir
m.touchIndex(ws, workDir)
initOK = true
```

Depois disso, o `defer` chama `migrateAllWorkspacesAndCleanupRemap()`. Essa
função pula o workspace ativo e, se nenhum outro workspace falhar, apaga o remap:

```go
for _, entry := range idx.Workspaces {
    // Skip active workspace — already loaded by Initialize.
    if m.active != nil && m.active.ID == entry.ID {
        continue
    }
    // ...
}
database.DeleteIDRemapFile(remapDir)
```

Impacto:

- O workspace ativo pode ser remapeado apenas em memória, falhar ao salvar e
  perder a segunda chance porque o remap foi removido no cleanup.
- Na próxima abertura, IDs legados em `conversation_id`, `content_id` ou
  `state.tasklistId` ainda podem ser limpos ou causar criação de conversas/listas
  novas.
- A perda é silenciosa e difícil de recuperar sem restaurar o remap manualmente.

Correção sugerida:

- Guardar em `Initialize()` se o load do workspace ativo retornou
  `ErrMigrationSaveFailed`.
- Se isso acontecer, não executar o cleanup do remap, ou passar essa informação
  para `migrateAllWorkspacesAndCleanupRemap()`.
- Alternativamente, fazer o cleanup reprocessar explicitamente o workspace ativo
  e só apagar o remap se a persistência dele também tiver sido confirmada.

Teste sugerido:

- Criar workspace ativo com `content_id` legado e remap disponível.
- Forçar falha de escrita no `workspace.yaml` ativo.
- Chamar `Initialize(workDir)` apontando para esse workspace.
- Garantir que `uuid-migration-remap.json` permanece no disco.
- Repetir a validação também para o caminho sem `workDir`, via `LastOpened`.

## Pontos Verificados Como Resolvidos

- Deep links de conversa criam abas `chat` com `conversationId`.
- `workspaceStore.addTab()` extrai `conversationId` do estado inicial e envia
  `conversation_id` ao backend.
- `loadIDRemap()` agora respeita a prioridade efetiva dos base paths.
- O cleanup do remap foi movido para depois da tentativa de migrar todos os
  workspaces conhecidos e preserva o arquivo quando um workspace não ativo falha
  ao salvar.
- `loadWorkspaceFile()` agora propaga `ErrMigrationSaveFailed` em falha de
  persistência.
- Fixtures E2E relevantes foram migradas para UUIDv7.
- `GetDetailedTokenStats()`, `GetMessagesAfterID()` e
  `GetMessagesBetweenIDs()` usam posição em listas ordenadas por `created_at`, e
  não comparação lexicográfica de UUID.
- `internal/tests/integration/firstrun_history_test.go` deixou de ordenar por
  `id ASC` e passou a usar `created_at ASC`.
- FKs órfãs agora são contabilizadas em log, e self-refs órfãs numéricas são
  limpas para `NULL`.
- Falha de rebuild FTS5 agora gera log em nível de erro com indicação de retry no
  próximo startup.
- Testes frontend usam `assistantMessageId`, alinhado ao contrato Go.
- `persistIDRemapFile()` foi movido para antes do `tx.Commit()`, reduzindo a
  janela de crash sem remap.
- `GetTaskListWithHierarchy()` deixou de ordenar por `parent_id ASC` e passou a
  ordenar por `"order" ASC, created_at ASC`.
- A migração adicionou checkpoint WAL antes do backup best-effort.
- A AEP-0046 deixa claro que IDs de status de workflow continuam numéricos.

## Checklist Recomendado Antes do Merge

1. Corrigir o cleanup do remap quando o workspace ativo retorna
   `ErrMigrationSaveFailed`.
2. Adicionar teste de regressão para falha de salvamento do workspace ativo.
3. Rodar novamente `go test ./...`, `go vet ./...`, `npm run build`,
   `npm run test` e a fatia E2E de chat/workspace/histórico.

