# Review PR 89 — AEP-0046 UUIDv7

Este arquivo consolida os pontos levantados no review completo do PR 89
(`feat/aep-0046-uuid-migration`). O foco é deixar um checklist acionável para
correção dentro da própria branch, com contexto suficiente para quem for ajustar
o código.

## Contexto

O PR migra IDs sequenciais `uint`/`INTEGER` para `string` com UUIDv7 em entidades
centrais do banco SQLite, além de ajustar contratos Go, Wails, TypeScript,
fixtures e deep links.

Validação executada durante o review:

- `go test ./internal/database ./internal/workspace`: passou.
- `go test ./...`: passou.
- TypeScript não foi validado no worktree isolado porque não havia
  `node_modules`; `npm exec tsc -- --noEmit` tentou instalar o pacote errado
  `tsc@2.0.4`.

## Achados críticos

### 1. Deep links podem abrir a conversa errada ou criar conversa duplicada

Arquivos envolvidos:

- `frontend/src/lib/deepLinks.ts`
- `frontend/src/hooks/useWorkspaceChatBridge.ts`
- `frontend/src/lib/workspaceConversation.ts`
- `frontend/src/lib/deepLinks.test.ts`

Problema:

`executeDeepLink()` usa o helper interno `openOrCreateChatTab()` para
`conversation:open` e `conversation:send`. Quando a conversa ainda não está aberta
em uma aba, esse helper chama:

```ts
await wsStore.addTab('chat', title || t('chat.newConversation'));
```

A aba é criada sem `conversation_id`/`conversationId`. Depois disso,
`useWorkspaceChatBridge()` detecta uma aba `chat` ativa sem `conversationId` e
chama `ensureWorkspaceTabConversationId()`, que por sua vez chama
`CreateConversation()` e persiste um novo UUID na aba.

Impacto:

- `assistente://conversation/<uuid>` pode abrir uma aba que acaba associada a
  outra conversa criada na hora, não à conversa do link.
- `assistente://conversation/<uuid>/send?message=...` pode carregar/enviar para a
  conversa correta no `chatStore`, mas a aba fica associada a outra conversa no
  workspace.
- `assistente://conversation/new?message=...` cria uma conversa via
  `chatStore.createConversation()`, mas a aba sem vínculo pode receber outra
  conversa via bridge.
- Os testes atuais em `frontend/src/lib/deepLinks.test.ts` validam o comportamento
  incorreto, por exemplo esperando `addTab('chat', 'Nova Conversa')` sem estado
  inicial.

Correção sugerida:

- Ao criar aba para uma conversa existente, passar o ID alvo explicitamente:

```ts
await wsStore.addTab('chat', title || t('chat.newConversation'), {
  conversation_id: conversationId,
});
```

- Para `conversation:new`, garantir que o ID retornado por `createConversation()`
  seja usado ao criar a aba. Hoje `createConversation()` não retorna o ID no tipo
  da store, então há duas opções:
  - alterar `createConversation()` para retornar a conversa/ID criado; ou
  - usar `useChatStore.getState().activeConversationId` imediatamente após a
    criação, com teste cobrindo o fluxo.
- Atualizar os testes de `deepLinks.test.ts` para exigir que a nova aba tenha
  `conversation_id` e para cobrir que o bridge não cria uma conversa extra.
- Considerar aceitar `conversationId` também em `initialState` ou ajustar
  `workspaceStore.addTab()` para ter uma API explícita de criação de chat tab com
  `conversationId`, evitando duplicar o nome backend `conversation_id` no frontend.

## Achados altos

### 2. Workspaces legados perdem vínculos com chats e tasklists

Arquivos envolvidos:

- `internal/workspace/manager.go`
- `internal/workspace/types.go`
- `frontend/src/hooks/useWorkspaceTasklistBridge.ts`
- `frontend/src/store/taskListStore.ts`

Problema:

A migração do banco preserva dados antigos criando mapas `old_id -> uuid`, mas os
workspaces em YAML ficam fora da transação e não recebem esse remapeamento.

Para abas `chat`, `loadWorkspaceFile()` limpa `ConversationID` quando o valor não
é UUIDv7:

```go
if t.Type == TabTypeChat && t.ConversationID != "" {
    if !isValidUUIDv7(t.ConversationID) {
        t.ConversationID = ""
        needsSave = true
    }
}
```

Isso evita carregar um ID inválido, mas também quebra o vínculo entre a aba e a
conversa antiga migrada.

Para abas `tasklist`, o problema é pior: no bloco de migração de `content_id`, o
valor legado é copiado diretamente para `state["tasklistId"]`:

```go
case TabTypeTasklist:
    if t.State == nil {
        t.State = map[string]any{}
    }
    if _, ok := t.State["tasklistId"]; !ok {
        t.State["tasklistId"] = t.ContentID
    }
```

Depois da migração do banco, `task_lists.id` é UUIDv7. Um `tasklistId` antigo como
`"3"` não aponta para nenhuma lista real. O frontend interpreta qualquer valor
truthy como ID válido em `useWorkspaceTasklistBridge()` e chama `loadTaskList("3")`,
que falha silenciosamente e deixa a aba sem conteúdo correto.

Impacto:

- Usuários com workspace existente perdem associação de abas de chat com as
  conversas anteriores.
- Abas de tasklist podem abrir vazias/erro por apontarem para IDs numéricos que
  não existem mais.
- O banco preserva os dados, mas a experiência do workspace sugere perda ou troca
  de conteúdo.

Correção sugerida:

- Expor o mapa `old_id -> uuid` gerado em `migrateToUUIDv7()` para uma etapa de
  migração de workspace, ou persistir um arquivo temporário de remapeamento
  consumido pelo `workspace.Manager`.
- Alternativamente, migrar workspaces antes de descartar os mapas, dentro de uma
  fase coordenada do startup.
- Para `chat`, remapear `conversation_id`/`content_id` numérico para o novo UUID,
  em vez de apenas limpar.
- Para `tasklist`, remapear `content_id` e `state.tasklistId` numéricos para os
  novos UUIDs de `task_lists`.
- Se o remapeamento de workspaces ficar fora do escopo, documentar explicitamente
  no AEP e criar fallback de UI: ao falhar `loadTaskList(id)`, não criar uma nova
  lista silenciosamente sem avisar o usuário.
- Adicionar testes em `internal/workspace/manager_test.go` cobrindo:
  - `conversation_id: 42` em workspace legado remapeado para UUID;
  - `content_id: 3` em aba `tasklist` remapeado para UUID;
  - `state.tasklistId: 3` ou `"3"` vindo de YAML legado.

### 3. Estatísticas de tokens ainda usam comparação lexicográfica de IDs

Arquivo envolvido:

- `internal/database/database.go`

Problema:

`GetDetailedTokenStats()` continua separando mensagens dentro/fora do contexto por
comparação direta de UUID:

```go
Where("conversation_id = ? AND id <= ?", conversationID, summaryUpToMessageID)
Where("conversation_id = ? AND id > ?", conversationID, summaryUpToMessageID)
```

O próprio PR já corrigiu `internal/chat/history.go` e
`internal/summarization/service.go` para usar índice na lista ordenada por
`created_at`, porque UUIDv7 não garante ordenação total dentro do mesmo
milissegundo.

Impacto:

- O modal/relatório de tokens pode contar mensagens em `inContext` e
  `outOfContext` incorretamente.
- Em conversas rápidas, mensagens criadas no mesmo milissegundo podem ficar do
  lado errado do corte de resumo.
- O bug é difícil de perceber, porque os totais básicos ainda podem parecer
  plausíveis.

Correção sugerida:

- Reescrever `GetDetailedTokenStats()` para carregar as mensagens raiz ordenadas
  por `created_at ASC` e encontrar `summaryUpToMessageID` por índice, como já é
  feito em `HistoryLoader.Load()`.
- Usar a mesma regra para quando `summaryUpToMessageID` não for encontrado:
  preferencialmente tratar como sem resumo válido e contar tudo como in-context,
  alinhando com o comportamento de prompt para evitar duplicação.
- Adicionar teste com IDs UUIDv7 fora de ordem lexicográfica dentro do mesmo
  timestamp, garantindo a contagem correta pelo índice.

## Achados médios

### 4. Helpers `GetMessagesAfterID()` e `GetMessagesBetweenIDs()` continuam perigosos

Arquivo envolvido:

- `internal/database/database.go`

Problema:

As funções ainda usam `id > ?` e `id <= ?`:

```go
func GetMessagesAfterID(conversationID string, afterID string) ([]ChatMessage, error) {
    err := db.Where("conversation_id = ? AND parent_id IS NULL AND id > ?", conversationID, afterID).
        Order("created_at ASC").Find(&messages).Error
}
```

Hoje a busca por uso indica que elas aparecem principalmente em testes de
`internal/database/summary_test.go`, mas elas permanecem como API pública do
pacote `database`.

Impacto:

- Qualquer uso futuro em código real pode reintroduzir o bug de ordenação por ID.
- Os testes atuais passam porque criam mensagens sequencialmente e assumem que o
  UUIDv7 acompanha a ordem, mas isso é uma propriedade frágil.

Correção sugerida:

- Remover as funções se não forem mais necessárias.
- Se forem mantidas, mudar a semântica para índice/`created_at` e renomear para
  refletir que o corte é por posição da mensagem, não por comparação de ID.
- Adicionar testes que simulem UUIDs fora de ordem lexicográfica para provar que a
  implementação não depende de `id >`.

### 5. Backup pré-migração não é confiável com WAL

Arquivos envolvidos:

- `internal/database/database.go`
- `internal/database/migration_uuid.go`
- `aep/0046-uuid-migration.md`

Problema:

`database.Init()` ativa WAL antes da migração:

```go
db.Exec("PRAGMA journal_mode=WAL")
db.Exec("PRAGMA synchronous=NORMAL")
```

Depois, `createBackup()` copia apenas o arquivo principal:

```go
src, err := os.ReadFile(dbPath)
if err := os.WriteFile(backupPath, src, 0600); err != nil { ... }
```

Em SQLite com WAL, páginas recentes podem estar no arquivo `-wal`. Copiar apenas
`conversations.db` pode gerar backup inconsistente ou incompleto. Além disso, o
arquivo inteiro é lido para memória.

O AEP diz que um backup é criado antes da migração, mas o código só loga um aviso
se o backup falhar e continua:

```go
if err := createBackup(); err != nil {
    log.Printf("[Migration] Aviso: não foi possível criar backup: %v", err)
    // Continua mesmo sem backup — a transação protege
}
```

Impacto:

- O usuário pode acreditar que há um ponto de recuperação seguro, mas o `.bak`
  pode não conter o estado real do banco.
- Em bancos grandes, `os.ReadFile()` aumenta o risco de uso excessivo de memória.
- Há divergência entre a garantia percebida/documentada e a implementação.

Correção sugerida:

- Criar o backup antes de ativar WAL, ou executar `PRAGMA wal_checkpoint(FULL)`
  antes de copiar.
- Preferir API de backup SQLite ou cópia em stream incluindo estado consistente.
- Decidir se falha no backup deve abortar a migração. Se continuar sem backup for
  intencional, atualizar o AEP e logs para deixar claro que a transação é a única
  proteção automática.
- Corrigir o nome documentado no AEP (`conversations.db.bak`) ou o nome real
  (`conversations.db.pre-uuid.bak`) para evitar confusão operacional.

### 6. Fixtures frontend ainda usam IDs numéricos

Arquivos com exemplos encontrados:

- `frontend/src/pages/ChatPage.test.tsx`
- `frontend/src/test/a11y-composed.test.tsx`
- `frontend/src/lib/messageMenuItems.test.ts`
- `frontend/src/lib/chatUtils.test.ts`
- `frontend/src/hooks/useDocumentTitle.test.tsx`
- `frontend/src/components/chat/MessageList.test.tsx`
- `frontend/src/components/chat/MessageNode.test.tsx`
- `frontend/src/components/chat/ChatSessionView.test.tsx`
- `frontend/src/components/chat/ChatMessage.test.tsx`

Problema:

Vários testes ainda constroem objetos com `conversationId: 1`, `conversationId:
10`, `conversationId: 0` ou mocks que retornam número:

```ts
const ensureWorkspaceTabHasConversationMock = vi.fn().mockResolvedValue(1);
conversationId: 1
conversationId: 0
```

`frontend/wailsjs/go/models.ts` agora define `conversationId: string` em
`main.EnrichedMessage`, e `WorkspaceTab.conversationId` também é `string |
undefined`.

Impacto:

- Os testes deixam de representar o contrato real do backend pós-migração.
- Podem mascarar bugs em comparações estritas `===` entre string e number.
- Dependendo da cobertura do `tsconfig`, podem quebrar `tsc` ou forçar casts
  indevidos.

Correção sugerida:

- Trocar todos os fixtures por UUIDv7 válido ou string vazia quando o caso for
  "sem conversa".
- Evitar `"1"` como string, porque `isBackendId()` rejeita IDs que não sejam
  UUIDv7.
- Adicionar helpers de teste centralizados, por exemplo:

```ts
export const TEST_CONVERSATION_ID =
  '01926b90-7a5a-7c4e-8d3f-000000000001';
```

- Rodar `npm run build`/`tsc` após a troca para garantir que não há `number`
  sobrando em contratos de conversa/mensagem.

## Achados baixos / documentação

### 7. AEP diverge da implementação em status de workflow

Arquivo envolvido:

- `aep/0046-uuid-migration.md`

Problema:

O AEP lista `task_list_workflow_statuses` como tabela migrada e também diz que
`tasks.status_id` e `task_list_workflows.initial_status_id` mudam para `string`.
Na implementação atual, os status do workflow permanecem como IDs numéricos
embutidos em JSON (`TaskListWorkflowStatus.ID` continua `number` no frontend e
`initial_status_id INTEGER` continua no schema).

Impacto:

- Quem usar o AEP como contrato arquitetural pode tentar "corrigir" o código para
  algo que não parece ser a decisão atual.
- A divergência dificulta reviews futuros e manutenção do workflow.

Correção sugerida:

- Atualizar o AEP para declarar explicitamente que status de workflow não são PKs
  de tabela e permanecem numéricos dentro do JSON.
- Remover `task_list_workflow_statuses` da lista de tabelas migradas, se ela não
  existir como tabela persistida.
- Ajustar a tabela de FKs para refletir:
  - `tasks.status_id` permanece `INTEGER`;
  - `task_list_workflows.initial_status_id` permanece `INTEGER`;
  - apenas `task_list_workflows.task_list_id` migra para UUIDv7.

### 8. Comentários e testes ainda reforçam premissas antigas

Arquivos com exemplos:

- `frontend/src/hooks/useWorkspaceChatBridge.ts`
- `internal/workspace/manager_test.go`
- `frontend/src/lib/deepLinks.test.ts`

Problema:

Alguns comentários ou testes ainda falam em `conversationId > 0`, ou validam IDs
numéricos legados como caminho normal de teste. Parte disso é intencional para
cobrir migração, mas precisa ficar claramente separado de fixtures pós-migração.

Correção sugerida:

- Atualizar comentários para "UUIDv7 não vazio" ou "ID backend válido".
- Em testes de legado, nomear explicitamente o caso como legado e verificar a
  sanitização/remapeamento esperado.
- Em testes de runtime normal, usar sempre UUIDv7.

## Checklist de correção sugerido

1. Corrigir criação de abas em deep links para persistir `conversation_id`.
2. Ajustar `conversation:new` para criar aba já associada à conversa criada.
3. Atualizar testes de deep link para reprovar criação de aba sem conversa.
4. Definir estratégia de remapeamento de workspaces YAML usando mapas da migração.
5. Corrigir `GetDetailedTokenStats()` para usar corte por índice/ordem de criação.
6. Remover ou reimplementar `GetMessagesAfterID()` e `GetMessagesBetweenIDs()`.
7. Tornar backup pré-migração consistente com WAL ou abortar se backup falhar.
8. Migrar fixtures frontend restantes para UUIDv7.
9. Atualizar AEP-0046 para refletir status de workflow numéricos.
10. Rodar validações finais:
    - `go test ./...`
    - `npm run build` em `frontend/`
    - `npm run test` em `frontend/`

