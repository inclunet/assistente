# Review PR 89 — AEP-0046 UUIDv7

Este documento registra o estado do review do PR 89
(`feat/aep-0046-uuid-migration`) após o commit `29b40529`
(`fix: address PR review round 3 (findings 1-3)`).

## Veredito

O PR está próximo de merge, mas ainda não está merge-ready. A migração principal
e os contratos de ID em string estão bem cobertos, porém ainda há um problema de
segurança/idempotência no consumo do arquivo `uuid-migration-remap.json`: ele
pode ser removido mesmo após falha parcial na migração de workspaces.

Recomendação: corrigir o cleanup do remap antes do merge e adicionar teste de
regressão para falha de persistência de workspace durante a migração.

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

### 1. Alta — Remap pode ser apagado mesmo com falha parcial de workspace

Arquivo envolvido:

- `internal/workspace/manager.go`

Problema:

`migrateAllWorkspacesAndCleanupRemap()` chama `database.DeleteIDRemapFile()`
sempre que encontra um remap, mesmo se algum workspace do índice falhar ao
carregar, parsear ou salvar durante a migração.

Trecho relevante:

```go
func (m *Manager) migrateAllWorkspacesAndCleanupRemap() {
    remap, remapDir := loadIDRemap()
    if remap == nil {
        return
    }
    // ...
    database.DeleteIDRemapFile(remapDir)
}
```

O problema é agravado porque `loadWorkspaceFile()` não propaga erro de
persistência quando `needsSave` é verdadeiro:

```go
if needsSave {
    if err := m.saveWorkspace(&ws, filepath.Dir(filepath.Dir(path))); err != nil {
        log.Printf("[Workspace] Aviso: falha ao salvar migração de workspace: %v", err)
    }
}
```

Impacto:

- Um workspace pode ser remapeado apenas em memória, falhar ao salvar e perder a
  segunda chance porque o remap foi removido.
- Na próxima abertura, IDs legados em `conversation_id`, `content_id` ou
  `state.tasklistId` podem ser limpos ou causar criação de conversas/listas novas.
- A perda é silenciosa e difícil de recuperar sem restaurar o remap manualmente.

Correção sugerida:

- Fazer `loadWorkspaceFile()` retornar erro quando `needsSave == true` e
  `saveWorkspace()` falhar; ou
- Separar o resultado em algo como `workspaceMigrated bool` e
  `workspaceMigrationSaved bool`; e
- Só apagar `uuid-migration-remap.json` se todos os workspaces conhecidos que
  precisavam de migração foram salvos com sucesso.

Teste sugerido:

- Criar dois workspaces com IDs legados e um remap.
- Forçar falha de escrita em um deles.
- Garantir que o remap permanece no disco.
- Garantir que, após corrigir a falha e reexecutar, o workspace pendente é
  migrado e só então o remap é removido.

### 2. Média — Teste ainda assume ordenação FIFO por `id` UUIDv7

Arquivo envolvido:

- `internal/tests/integration/firstrun_history_test.go`

Problema:

O teste usa `Order("id ASC")` e comenta que UUIDv7 preserva ordem de inserção.
Isso contradiz a própria cautela adicionada no PR: UUIDv7 só ordena bem entre
milissegundos diferentes; dentro do mesmo milissegundo a parte aleatória pode
quebrar a ordem lexicográfica.

Impacto:

- Teste frágil/flaky em máquinas rápidas.
- Invariante enganosa para manutenção futura.
- O teste não espelha o caminho de produção, que em geral ordena mensagens por
  `created_at`.

Correção sugerida:

- Trocar para `Order("created_at ASC")`, preferencialmente com desempate estável
  se necessário.
- Atualizar o comentário para não afirmar monotonicidade por `id`.

### 3. Média — FKs inválidas e self-refs órfãs podem virar dados sujos/silenciosos

Arquivo envolvido:

- `internal/database/migration_uuid.go`

Problema:

Durante `migrateTable()`, FKs para outras tabelas que não existem no mapa antigo
viram `NULL`. Para self-references (`parent_id`, `turn_id`) que apontam para uma
linha inexistente, o placeholder numérico em string pode permanecer na coluna
TEXT porque o segundo passe só substitui IDs que existem em `idMap`.

Impacto:

- Relações quebradas antes da migração continuam quebradas, mas agora podem ficar
  invisíveis (`NULL`) ou com valor inválido (`"999"` em coluna de UUID).
- Como o projeto não aparenta habilitar `PRAGMA foreign_keys`, esse cenário não é
  bloqueado pelo SQLite.

Correção sugerida:

- Logar contagens de FKs órfãs por tabela/coluna durante a migração.
- Para self-refs não resolvidas, limpar para `NULL` explicitamente depois do
  segundo passe.
- Adicionar teste garantindo que self-ref órfã não permanece como string numérica.

### 4. Média — FTS5 pode ficar vazio se rebuild falhar após migração

Arquivos envolvidos:

- `internal/database/migration_uuid.go`
- `internal/database/database.go`

Problema:

A migração dropa `chat_messages_fts`. Depois, `Init()` chama `initFTS5()` e, se
percebe que `ftsCount < msgCount`, chama `RebuildFTSIndex()`. Se o rebuild
falhar, o erro é apenas logado e o app sobe com a busca full-text incompleta ou
vazia.

Impacto:

- Busca de histórico por conteúdo pode parecer quebrada após uma migração que
  terminou "com sucesso".

Correção sugerida:

- Tratar erro de `RebuildFTSIndex()` como fatal imediatamente após uma migração
  UUID; ou
- Persistir um marcador de "FTS rebuild pending" e tentar novamente no próximo
  startup até sucesso.

### 5. Média — Testes frontend usam `assistantmessageId` fora do contrato

Arquivos envolvidos:

- `frontend/e2e/chat/chat-streaming.spec.ts`
- `frontend/src/store/chatStore.validation.test.ts`
- `internal/core/ports/chat_events.go`

Problema:

Alguns testes emitem `chat:done` com `assistantmessageId`, mas o contrato Go usa
`assistantMessageId`. Hoje isso não quebra runtime porque o fluxo atual finaliza
streaming pelo `messageId` do evento `chat:stream`, mas o teste documenta um
payload errado.

Impacto:

- Futuras mudanças podem copiar o shape incorreto.
- Se o frontend passar a consumir `assistantMessageId` em `chat:done`, esses
  testes deixam de representar o backend real.

Correção sugerida:

- Trocar `assistantmessageId` por `assistantMessageId` nos testes.
- Se o campo realmente não for usado, considerar teste explícito do shape correto
  para manter contrato alinhado.

### 6. Baixa — Janela pós-commit sem remap

Arquivo envolvido:

- `internal/database/migration_uuid.go`

Problema:

`migrateToUUIDv7()` faz `tx.Commit()` e só depois chama `persistIDRemapFile()`.
Um crash ou kill do processo nesse intervalo deixa o banco já migrado para UUID,
mas sem arquivo de remap para atualizar workspaces legados.

Impacto:

- Workspaces com IDs numéricos podem não ser remapeados após restart.
- A recuperação exigiria restauração manual do backup ou reconstrução manual do
  mapa.

Correção sugerida:

- Reduzir a janela persistindo um remap temporário antes do commit e marcando-o
  como válido após commit; ou
- Documentar explicitamente essa janela como risco aceito.

### 7. Baixa — Ordenação por `parent_id` de tasks ficou menos significativa

Arquivo envolvido:

- `internal/database/tasklist_repository.go`

Problema:

`GetTaskListWithHierarchy()` usa `Order("parent_id ASC, order ASC")`. Após UUID,
`parent_id ASC` não carrega mais a semântica aproximada de criação que IDs
numéricos tinham.

Impacto:

- Provável impacto baixo, porque a hierarquia é reconstruída em memória e `order`
  ainda ordena dentro do nível.
- Ainda assim, a ordenação inicial por UUID pode tornar resultados menos
  previsíveis em cenários de empate/ordem não definida.

Correção sugerida:

- Usar `Order("order ASC, created_at ASC")` ou ordenar subtasks explicitamente
  por `order`/`created_at` após montar a árvore.

## Pontos Verificados Como Resolvidos

- Deep links de conversa criam abas `chat` com `conversationId`.
- `workspaceStore.addTab()` extrai `conversationId` do estado inicial e envia
  `conversation_id` ao backend.
- `loadIDRemap()` agora respeita a prioridade efetiva dos base paths.
- O cleanup do remap foi movido para depois da tentativa de migrar todos os
  workspaces conhecidos.
- Fixtures E2E relevantes foram migradas para UUIDv7.
- `GetDetailedTokenStats()`, `GetMessagesAfterID()` e
  `GetMessagesBetweenIDs()` usam posição em listas ordenadas por `created_at`, e
  não comparação lexicográfica de UUID.
- A migração adicionou checkpoint WAL antes do backup best-effort.
- A AEP-0046 deixa claro que IDs de status de workflow continuam numéricos.

## Checklist Recomendado Antes do Merge

1. Corrigir o cleanup do remap para não remover o arquivo após falha parcial de
   workspace.
2. Adicionar teste de regressão para falha de salvamento durante migração de
   workspace.
3. Ajustar `Order("id ASC")` no teste de histórico.
4. Corrigir `assistantmessageId` para `assistantMessageId` nos testes.
5. Decidir se FKs/self-refs órfãs devem ser logadas, limpas ou tratadas como erro
   de migração.
6. Rodar novamente `go test ./...`, `go vet ./...`, `npm run build`,
   `npm run test` e a fatia E2E de chat/workspace/histórico.

