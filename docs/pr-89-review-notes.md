# Review PR 89 — AEP-0046 UUIDv7

Este documento registra o estado do review do PR 89
(`feat/aep-0046-uuid-migration`) após o commit `21c26bae`
(`fix: address PR review round 2 (findings 1-6)`).

## Estado Atual

As duas rodadas anteriores de achados foram majoritariamente endereçadas:

- Deep links de conversa criam abas `chat` com `conversationId` e
  `workspaceStore.addTab()` converte o valor para `conversation_id`.
- `conversation:new` usa o ID retornado por `chatStore.createConversation()` para
  abrir a aba já vinculada à conversa correta.
- `GetDetailedTokenStats()`, `GetMessagesAfterID()` e
  `GetMessagesBetweenIDs()` usam mensagens ordenadas por `created_at`, sem
  comparação lexicográfica de UUID.
- O backup pré-migração faz `PRAGMA wal_checkpoint(FULL)` antes de copiar o
  banco, e a AEP documenta que a política de backup é best-effort.
- A AEP-0046 foi ajustada para explicar que IDs de status de workflow continuam
  numéricos dentro do JSON.
- `uuid-migration-remap.json` agora é procurado nos diretórios de configuração
  (`exe`, `home`, `workdir`) em vez de apenas em `homeDir`.
- O remap só é apagado após `saveWorkspace()` bem-sucedido.
- `tasklistId` legado sem remap é limpo.
- Foram adicionados testes Go para remap de workspace e para ordenação por
  `created_at` com UUIDs fora da ordem lexicográfica.
- Fixtures em `frontend/src` foram migradas para UUIDv7.

Validação executada nesta rodada:

- `go test ./...`: passou.

Não rodei TypeScript nem E2E neste worktree.

## Achados Restantes

### 1. `loadIDRemap()` pode escolher um remap de menor prioridade

Arquivos envolvidos:

- `internal/workspace/manager.go`
- `internal/configdir/paths.go`
- `internal/database/migration_uuid.go`

Problema:

`loadIDRemap()` percorre `configdir.GetBasePaths()` na ordem retornada pelo
resolver:

```go
func loadIDRemap() (*database.IDRemapData, string) {
    for _, dir := range configdir.GetBasePaths() {
        if data := database.LoadIDRemapFile(dir); data != nil {
            return data, dir
        }
    }
    return nil, ""
}
```

`GetBasePaths()` retorna os diretórios em prioridade crescente:

```go
[]string{cachedExeDir, cachedHomeDir, cachedWorkDir}
```

O resolver de arquivos usa essa ordem para que o último encontrado vença. Já
`loadIDRemap()` retorna o primeiro arquivo encontrado. Se existir um
`uuid-migration-remap.json` antigo em `home` e o remap correto estiver em
`workdir`, o workspace pode aplicar o mapa errado.

Impacto:

- Workspaces podem ser remapeados com IDs de outra base de dados.
- O remap correto pode permanecer no diretório de maior prioridade, enquanto o
  remap errado é apagado após salvar o workspace.
- É um caso de borda, mas o sistema explicitamente suporta banco em `exe`, `home`
  e `workdir`, então a busca deve seguir a mesma semântica de prioridade.

Correção sugerida:

- Iterar `configdir.GetBasePaths()` em ordem reversa, respeitando a prioridade
  efetiva (`workdir`, depois `home`, depois `exe`); ou
- Resolver o remap a partir do mesmo diretório do `conversations.db` que foi
  migrado; ou
- Persistir metadados no remap identificando o caminho/origem do banco e validar
  antes de aplicar.

Teste sugerido:

- Criar dois remaps, um em `home` e outro em `workdir`, com mapas conflitantes.
  Garantir que o workspace aplica o remap do `workdir`.

### 2. Remap é apagado após migrar apenas o workspace carregado

Arquivo envolvido:

- `internal/workspace/manager.go`

Problema:

`loadWorkspaceFile()` apaga o `uuid-migration-remap.json` após salvar o workspace
que acabou de carregar:

```go
if needsSave {
    if err := m.saveWorkspace(&ws, filepath.Dir(filepath.Dir(path))); err != nil {
        log.Printf("[Workspace] Aviso: falha ao salvar migração de workspace: %v", err)
    } else if remap != nil && remapDir != "" {
        database.DeleteIDRemapFile(remapDir)
    }
}
```

Isso corrige o problema de apagar antes de salvar, mas ainda consome o remap no
primeiro workspace migrado.

Impacto:

- Se houver múltiplos workspaces no índice, só o workspace carregado primeiro terá
  referências legadas remapeadas.
- Workspaces abertos posteriormente podem perder `conversation_id`/`tasklistId`
  legados porque o remap já foi removido.
- A perda pode ser silenciosa: chat legado vira conversa nova e tasklist legado
  pode ser limpo.

Correção sugerida:

- Migrar todos os workspaces conhecidos antes de apagar o remap; ou
- Manter o remap por mais tempo e apagar apenas em uma etapa explícita de cleanup;
  ou
- Registrar no remap quais workspaces já foram processados e só removê-lo quando
  todos os workspaces conhecidos forem salvos.

Teste sugerido:

- Criar dois workspaces com IDs legados e um único remap. Carregar o primeiro e
  depois o segundo. Garantir que ambos são remapeados.

### 3. E2E ainda contém IDs numéricos em strings

Arquivos com exemplos:

- `frontend/e2e/chat/chat-interaction.spec.ts`
- `frontend/e2e/chat/chat-basics.spec.ts`
- `frontend/e2e/chat/chat-markdown.spec.ts`
- `frontend/e2e/chat/chat-tool-calling.spec.ts`
- `frontend/e2e/a11y/chat-toolbar.spec.ts`
- `frontend/e2e/a11y/chat-keyboard-navigation.spec.ts`
- `frontend/e2e/a11y/menu-keyboard.spec.ts`
- `frontend/e2e/a11y/landmark-navigation.spec.ts`
- `frontend/e2e/a11y/tablist-navigation.spec.ts`
- `frontend/e2e/a11y/message-operations.spec.ts`
- `frontend/e2e/a11y/modal-focus-trap.spec.ts`
- `frontend/e2e/history/history-interactions.spec.ts`

Problema:

A varredura de `frontend/src` não encontrou mais `conversationId` numérico em
strings, mas `frontend/e2e` ainda tem fixtures como:

```ts
conversationId: '1'
messageId: '2'
userMessageId: '100'
conversation_id: '1'
```

Em alguns testes isso é apenas fixture isolada, mas em fluxos de streaming pode
enfraquecer a cobertura. O mock padrão em `frontend/e2e/mocks/wails-runtime.ts`
usa conversa UUIDv7:

```ts
conversation_id: '01926b90-0000-7000-8000-000000000001'
```

Um teste que emite evento para `conversationId: '1'` pode não exercitar o caminho
real esperado, porque o `chatStore` filtra eventos por conversa ativa.

Impacto:

- Testes E2E podem passar sem validar o caminho real pós-UUIDv7.
- Regressões em guards como `isBackendId()` podem ficar mascaradas.
- Eventos emitidos com conversa diferente da ativa podem ser descartados
  silenciosamente.

Correção sugerida:

- Centralizar constantes UUIDv7 de teste no mock E2E.
- Trocar `conversationId`, `conversation_id`, `messageId`,
  `userMessageId` e `assistantMessageId` por UUIDv7 válidos nos testes E2E.
- Onde o teste intencionalmente cobre ID legado ou sintético, deixar isso
  explícito no nome/comentário.

## Pontos Verificados Como Resolvidos

### Deep links e aba de chat

`frontend/src/lib/deepLinks.ts` agora cria abas com `{ conversationId }`, e
`frontend/src/store/workspaceStore.ts` extrai esse campo para
`WorkspaceTab.conversationId`.

### Remap básico de workspace

`internal/workspace/manager.go` agora busca remap nos base paths, remapeia
`content_id`, `conversation_id` e `state.tasklistId`, limpa `tasklistId` inválido
sem remap, e só apaga o remap após salvar o workspace com sucesso.

### Ordenação por `created_at`

`internal/database/ordering_test.go` cobre:

- `GetMessagesAfterID()` usando ordem de criação, não ordem lexicográfica.
- `GetMessagesBetweenIDs()` usando ordem de criação.
- `GetDetailedTokenStats()` cortando por índice do `summaryUpToMessageID`.

### Backup

`internal/database/migration_uuid.go` executa `wal_checkpoint(FULL)` antes de
copiar o banco. A AEP deixa claro que a migração continua mesmo se o backup
falhar, usando rollback transacional como proteção principal.

## Checklist Recomendado

1. Ajustar `loadIDRemap()` para respeitar a mesma prioridade do resolver.
2. Evitar apagar o remap antes de processar todos os workspaces conhecidos.
3. Migrar fixtures E2E restantes para UUIDv7 ou marcar explicitamente os casos
   legados/sintéticos.
4. Rodar `go test ./...`, `npm run build` e a suíte E2E relevante após as
   correções.

