# Review PR 89 — AEP-0046 UUIDv7

Este documento registra o estado do review do PR 89
(`feat/aep-0046-uuid-migration`) após o commit `61d981f6`
(`fix: address comprehensive PR review (findings 1-8)`).

## Estado Atual

Os achados críticos da primeira rodada foram em grande parte endereçados:

- Deep links de conversa agora criam abas `chat` com `conversationId` no estado
  inicial e `workspaceStore.addTab()` move esse valor para `conversation_id`.
- `conversation:new` agora usa o ID retornado por `chatStore.createConversation()`
  para criar a aba já vinculada à conversa correta.
- `GetDetailedTokenStats()` deixou de usar comparação lexicográfica de UUID e
  passou a cortar por índice em mensagens ordenadas por `created_at`.
- `GetMessagesAfterID()` e `GetMessagesBetweenIDs()` também foram reescritas para
  usar posição na lista ordenada por `created_at`.
- O backup pré-migração agora executa `PRAGMA wal_checkpoint(FULL)` antes de
  copiar o banco.
- A AEP-0046 foi ajustada para explicar que IDs de status de workflow continuam
  numéricos dentro do JSON, e não são PKs UUIDv7.
- Fixtures frontend mais óbvias com `conversationId` numérico foram migradas para
  strings UUIDv7.

Validação executada após atualizar a branch:

- `go test ./internal/database ./internal/workspace`: passou.

## Achados Restantes

### 1. Remapeamento de workspace pode falhar quando o banco não está em `homeDir`

Arquivos envolvidos:

- `internal/database/migration_uuid.go`
- `internal/workspace/manager.go`
- `internal/configdir/resolver.go`
- `internal/configdir/paths.go`

Problema:

A migração persiste `uuid-migration-remap.json` no diretório real do banco:

```go
remapPath := filepath.Join(filepath.Dir(dbPath), idRemapFilename)
```

Mas o workspace manager tenta carregar e remover o arquivo apenas a partir de
`m.homeDir`:

```go
remap := database.LoadIDRemapFile(m.homeDir)
database.DeleteIDRemapFile(m.homeDir)
```

O resolver do banco aceita `conversations.db` em três locais, com prioridade
crescente: diretório do executável, `~/.assistente` e `.assistente` do diretório
de trabalho. Quando o banco migrado estiver no workdir ou no diretório do
executável, o remap é salvo ao lado desse banco, mas o workspace procura em
`~/.assistente`. Nesse cenário, `conversation_id`, `content_id` e
`state.tasklistId` legados continuam sem remapeamento.

Impacto:

- Usuários com `conversations.db` no workdir ou no diretório do executável podem
  perder o vínculo das abas antigas com conversas/listas migradas.
- O problema afeta justamente o caso que a correção tentou resolver: preservar
  referências de workspace YAML após converter IDs numéricos para UUIDv7.
- Não há testes cobrindo esse fluxo; uma busca por `uuid-migration-remap`,
  `LoadIDRemapFile` e `Remap` em `*_test.go` não encontrou casos.

Correção sugerida:

- Persistir o remap sempre em `configdir.GetHomeDir()` se o workspace manager só
  deve ler dali; ou
- Fazer o workspace manager procurar o remap nos mesmos diretórios retornados por
  `configdir.GetBasePaths()`; ou
- Expor uma função de resolução no pacote `database`/`configdir` para carregar o
  remap do mesmo diretório em que o `conversations.db` foi migrado.

Testes sugeridos:

- Banco/remap em `homeDir`: `conversation_id: 42`, `content_id: 3` e
  `state.tasklistId: "3"` são remapeados.
- Banco/remap em workdir: o workspace também encontra o remap e remapeia.
- Banco/remap ausente: chat inválido é limpo e tasklist inválida recebe fallback
  explícito/documentado.

### 2. Arquivo de remapeamento é removido antes de garantir persistência do workspace

Arquivo envolvido:

- `internal/workspace/manager.go`

Problema:

`loadWorkspaceFile()` remove `uuid-migration-remap.json` antes de chamar
`saveWorkspace()`:

```go
if remap != nil && needsSave {
    database.DeleteIDRemapFile(m.homeDir)
}

if needsSave {
    _ = m.saveWorkspace(&ws, filepath.Dir(filepath.Dir(path)))
}
```

Além disso, o erro de `saveWorkspace()` é ignorado.

Impacto:

- Se a escrita do YAML falhar, o remap já foi apagado e a próxima inicialização
  não consegue repetir a migração das referências do workspace.
- Esse é um risco operacional pequeno, mas é exatamente no caminho de recuperação
  da migração.

Correção sugerida:

- Salvar o workspace primeiro.
- Só apagar o remap se `saveWorkspace()` retornar `nil`.
- Logar falhas de persistência da migração do workspace, em vez de ignorar o erro.

### 3. Remapeamento de tasklist inválida ainda pode deixar aba apontando para ID legado

Arquivos envolvidos:

- `internal/workspace/manager.go`
- `frontend/src/hooks/useWorkspaceTasklistBridge.ts`

Problema:

Para `tasklist`, quando `state.tasklistId` é string não UUIDv7 e não há entrada no
remap, o valor permanece como estava:

```go
if tlID, ok := t.State["tasklistId"].(string); ok && tlID != "" && !isValidUUIDv7(tlID) {
    if remap != nil {
        if newID, ok := remap.TaskLists[tlID]; ok {
            t.State["tasklistId"] = newID
            needsSave = true
        }
    }
}
```

O frontend considera qualquer `tasklistId` truthy como existente e tenta carregar
esse ID:

```ts
if (taskListId) {
  syncExistingTaskList(taskListId);
}
```

Impacto:

- Se não houver remap, se o remap estiver no diretório errado ou se o ID legado
  não existir no mapa, a aba de tasklist continua apontando para `"3"`/`"42"`.
- `loadTaskList()` falha e retorna `null`, mas a aba permanece visualmente
  vinculada a um conteúdo inexistente.

Correção sugerida:

- Após tentativa de remap, se `tasklistId` continuar inválido, limpar a chave
  `state.tasklistId` ou marcar a aba como órfã com feedback claro.
- Se a opção for limpar, o bridge pode criar uma nova tasklist de forma explícita.
- Se a opção for preservar, a UI deve mostrar erro de referência legada não
  migrada, em vez de falhar silenciosamente.

### 4. Falta cobertura de regressão para as novas regras por índice

Arquivos envolvidos:

- `internal/database/database.go`
- `internal/database/summary_test.go`

Problema:

`GetDetailedTokenStats()`, `GetMessagesAfterID()` e
`GetMessagesBetweenIDs()` foram corrigidas para não usar `id >`/`id <=`. Porém,
os testes existentes não parecem forçar o caso que motivou a correção: UUIDs fora
da ordem lexicográfica dentro da ordem de criação.

Impacto:

- A correção é plausível, mas pode regredir sem que os testes capturem.
- `GetDetailedTokenStats()` não possui teste dedicado para contagem
  in-context/out-of-context por índice.

Correção sugerida:

- Criar mensagens com IDs UUIDv7 controlados fora da ordem lexicográfica, mas com
  `created_at` ordenado.
- Validar que `GetDetailedTokenStats()` separa tokens pelo índice do
  `summaryUpToMessageID`.
- Validar que `GetMessagesAfterID()` e `GetMessagesBetweenIDs()` usam posição da
  mensagem na lista e não comparação de string.

### 5. Fixtures frontend ainda usam strings numéricas como IDs em alguns testes

Arquivos com exemplos:

- `frontend/src/lib/deepLinks.test.ts`
- `frontend/src/store/workspaceStore.test.ts`
- `frontend/src/store/chatStore.validation.test.ts`
- `frontend/src/components/chat/TokenStatsButton.test.tsx`
- `frontend/src/components/chat/TokenStatsModal.test.tsx`
- `frontend/src/services/chatSpeak/index.test.ts`
- `frontend/src/pages/HistoryPage.test.tsx`
- `frontend/src/hooks/useWorkspaceEditorBridge.test.tsx`
- `frontend/src/hooks/useEditorInlineChatPatch.test.ts`

Problema:

Os números do tipo `conversationId: 1` foram migrados em vários pontos, mas ainda
há strings como `"1"`, `"42"` e `"999"` sendo usadas como IDs de conversa ou
mensagem em testes.

Isso não é necessariamente bug quando o teste está cobrindo dados legados ou
eventos sintéticos, mas é incoerente com o contrato runtime de IDs persistidos
validado por `isBackendId()`.

Impacto:

- Pode mascarar bugs em código que diferencia UUIDv7 real de string arbitrária.
- Dificulta identificar quais testes são legado/sintético e quais representam o
  contrato normal pós-migração.

Correção sugerida:

- Trocar fixtures runtime normais por constantes UUIDv7 de teste.
- Manter `"1"`/`"42"` apenas em testes explicitamente legados ou sintéticos, com
  nomes/comentários que deixem isso claro.
- Alinhar `activeConversationId: "1"` em `deepLinks.test.ts` com o UUID retornado
  por `mockCreateConversation`, ou remover o campo se ele não for relevante.

### 6. Política de backup ainda permite migração sem backup

Arquivo envolvido:

- `internal/database/migration_uuid.go`

Problema:

O checkpoint WAL reduz bastante o risco de backup inconsistente. Ainda assim,
quando `createBackup()` falha, a migração continua:

```go
if err := createBackup(); err != nil {
    log.Printf("[Migration] Aviso: não foi possível criar backup: %v", err)
    // Continua mesmo sem backup — a transação protege
}
```

Impacto:

- A transação protege contra falha durante a migração, mas não substitui um ponto
  de recuperação externo para suporte.
- Se a decisão de produto for "nunca migrar sem backup", o código ainda não
  cumpre esse requisito.

Correção sugerida:

- Decidir explicitamente se falha de backup deve abortar a migração.
- Se continuar sem backup for aceitável, manter isso documentado na AEP e em logs
  de suporte.

## Pontos Resolvidos

### Deep links de conversa

`frontend/src/lib/deepLinks.ts` agora passa `{ conversationId }` para `addTab()` em
`conversation:open`, `conversation:send` e `conversation:new`.

### `workspaceStore.addTab()`

`frontend/src/store/workspaceStore.ts` extrai `conversationId` de `initialState` e
preenche `WorkspaceTab.conversationId`, evitando que o ID vá para `state`.

### Estatísticas e helpers de mensagem

`internal/database/database.go` usa mensagens ordenadas por `created_at` e corte
por índice em vez de comparar UUIDs lexicograficamente.

### Backup com WAL

`internal/database/migration_uuid.go` executa `PRAGMA wal_checkpoint(FULL)` antes
da cópia do arquivo principal do banco.

### AEP-0046

`aep/0046-uuid-migration.md` foi ajustada para refletir que status de workflow
permanecem IDs numéricos embutidos no JSON.

## Checklist Recomendado

1. Corrigir a localização do `uuid-migration-remap.json` para funcionar quando o
   banco estiver fora de `homeDir`.
2. Só apagar o remap após salvar o workspace com sucesso.
3. Definir fallback explícito para `tasklistId` legado sem remap.
4. Adicionar testes Go para remapeamento de workspace com remap em `homeDir` e em
   workdir.
5. Adicionar testes Go para token stats e helpers com UUIDs fora da ordem
   lexicográfica.
6. Trocar fixtures frontend runtime por UUIDv7 ou marcar strings numéricas como
   legado/sintético.
7. Decidir se falha de backup aborta a migração ou apenas emite aviso.

