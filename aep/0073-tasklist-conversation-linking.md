# AEP-0073 — Vínculo de Tasks e Tasklists a Conversas

Status: ✅ Done
Data: 2026-06-09
Autor: Inclunet + Cursor Agent
PR: #232

## Resumo

Esta AEP introduz um **vínculo opcional entre tasks/tasklists e conversas**,
permitindo que uma task individual ou uma tasklist inteira seja "filha" de uma
conversa. O relacionamento é **1:N** — uma conversa pode ter várias tasks e/ou
várias tasklists vinculadas, e o vínculo de uma task é **independente** do
vínculo da sua lista.

Três frentes se conectam:

1. **Modelo de dados** — campo opcional `conversation_id` (`*string`, nullable e
   indexado) em `Task` e `TaskList`, propagado por toda a stack (repository →
   store → service → eventos de domínio → bindings Wails → UI).
2. **Acesso pelo agente** — parâmetro `conversation_id` nas tools `task` e
   `task_list` (vincular/desvincular) e uma nova tool **`get_conversation_info`**,
   que permite ao agente **descobrir o `conversation_id` da conversa em
   andamento** e listar tasks/tasklists já vinculadas.
3. **Contexto automático no prompt** — as tasklists vinculadas à conversa atual
   são injetadas no template do skill `tasklist-manager`, dando ao agente
   visibilidade imediata do que pertence àquela conversa.

## Motivação

As tasklists (AEP-0036) e seus eventos de domínio/custom actions (AEP-0067) já
permitem orquestrar trabalho, mas **não havia ligação formal entre uma conversa
e o trabalho derivado dela**. Casos de uso concretos que ficavam inviáveis ou
manuais:

- Uma skill de suporte técnico (techsupport) que, ao analisar um chamado, queira
  **registrar a conversa atual** na task que está criando, criando rastreio
  bidirecional entre o atendimento e a tarefa.
- Um perfil de programação que, ao receber uma demanda no chat, **crie uma
  tasklist com vários passos** já vinculada àquela conversa, para ser executada
  ao longo do diálogo.
- Abrir, a partir de um card ou lista no Kanban, **a conversa de origem** com um
  clique.

O bloqueio central era que **o agente não tinha como obter o `conversation_id`
da conversa corrente** para usar/editar. Esta AEP resolve isso com a tool
`get_conversation_info` e o parâmetro explícito nas tools de tasklist.

## Relação com outras AEPs

- **AEP-0036** (Task List Manager Feature): AEP base das tasklists; o modelo de
  dados é estendido aqui com `conversation_id`.
- **AEP-0067** (Eventos de domínio de Tasklists e Custom Actions): os payloads de
  evento (`task:*`, `taskList:*`) passam a carregar `conversation_id`, e o
  campo entra na detecção de campos alterados.
- **AEP-0023** (Deep Links): a UI usa `openTaskLink`/deep link para abrir a
  conversa vinculada a partir do card/lista.
- **AEP-0072** (Skill Catalog & Loading) / `tasklist-manager`: o skill recebe no
  contexto as tasklists vinculadas à conversa atual.

## Decisões de escopo

Decisões tomadas com o usuário durante o planejamento:

1. **Onde o vínculo existe**: em **Task E TaskList** (mais flexível — uma conversa
   pode ter tasks soltas e/ou listas inteiras).
2. **Como o `conversation_id` é preenchido nas tools**: **apenas parâmetro
   explícito** (o modelo precisa informar). Isso motivou a criação da tool
   `get_conversation_info` para o agente descobrir o id corrente.
3. **`get_conversation_info`**: criada.
4. **Fragmentação da entrega**: PR único (backend + tools + UI + docs).

## Design

### Modelo de dados

```go
// internal/database/models.go
type TaskList struct {
    // ...
    // Vínculo opcional com uma conversa (1 conversa : N tasklists).
    ConversationID *string `json:"conversation_id,omitempty" gorm:"index"`
}

type Task struct {
    // ...
    // Vínculo opcional com uma conversa (1 conversa : N tasks).
    // Independente do vínculo da lista.
    ConversationID *string `json:"conversation_id,omitempty" gorm:"index"`
}
```

Semântica do ponteiro em toda a stack:

- `nil` → **não altera** o vínculo (no update) / sem vínculo (no read).
- `""` (string vazia recebida nas tools) → **limpa** o vínculo (grava `NULL`).
- valor não vazio → **vincula** àquela conversa.

A coluna é nullable e indexada; a criação do índice é feita pelo auto-migrate do
GORM (sem migração SQL manual).

### Camadas backend

- **Repository** (`internal/database/tasklist_repository.go`): novas funções
  `SetTaskConversationWithContext`, `SetTaskListConversationWithContext`,
  `GetTasksByConversationIDWithContext`, `GetTaskListsByConversationIDWithContext`
  — todas fail-closed por `user_id` no contexto.
- **Store/Service** (`internal/tasklist/`): interface estendida, implementação no
  `db_store`, e métodos no `service` que emitem os eventos `task:updated` /
  `taskList:updated` ao alterar o vínculo.
- **Eventos de domínio** (`internal/tasklist/domain_events.go`): `conversation_id`
  incluído em `taskPayload`/`listPayload` e na detecção `changedTaskFields`.
- **Bindings Wails** (`internal/app/app_tasklist.go`, `controllers/`):
  `SetTaskConversation`, `SetTaskListConversation`, `GetTasksByConversation`,
  `GetTaskListsByConversation`.

### Tools do agente

- **`task` / `task_list`**: novo parâmetro `conversation_id`. Há um caminho
  especial de **update apenas-do-vínculo** (sem exigir `title`/demais campos),
  preservando o restante da entidade. Na duplicação, o vínculo é herdado da
  origem salvo se um `conversation_id` explícito for passado.
- **`get_conversation_info`** (`internal/tools/history/get_conversation_info.go`):
  retorna `conversation_id`, título, canal, contagem de mensagens, resumo
  rolante e as tasks/tasklists já vinculadas. Sem `conversation_id` explícito,
  usa a conversa corrente carimbada no `InvocationContext`. Parâmetros opcionais
  `include_messages` / `message_limit` para trazer mensagens recentes.

Fluxo típico do agente: chamar `get_conversation_info` (sem args) → obter o
`conversation_id` corrente → chamar `task`/`task_list` com esse `conversation_id`.

### Contexto no prompt

`internal/chat/interactor.go` ganhou o provider
`LinkedTaskLists(ctx, conversationID) []TemplateTaskList`. Em `PrepareMessages`,
quando há `conversation_id`, as tasklists vinculadas são carregadas e expostas no
template do skill `tasklist-manager` (`TaskLists` / `HasTaskLists`). O wiring é
feito em `internal/app/app.go` via `linkedTaskListsForConversation`.

### UI

- **`TaskForm`** e **cabeçalho da lista** (`TaskListHeader`): `Select` para
  vincular/editar a conversa, reaproveitando `GetConversations`.
- **`TaskDetailModal`** e **`TaskListHeader`**: badge clicável que abre a conversa
  vinculada via deep link.
- **Store** (`taskListStore`): normalização de `conversation_id` (snake→camel) e
  ações `setTaskConversation` / `setTaskListConversation` com update otimista.
- **i18n**: chaves `conversation`, `conversationDescription`, `conversationNone`,
  `conversationUntitled` em pt-BR/en/es.

## Compatibilidade e migração

- Campo opcional e nullable; conteúdo existente continua válido (`conversation_id`
  ausente = sem vínculo).
- Sem migração SQL manual: o índice é criado pelo auto-migrate do GORM.
- Tools retrocompatíveis: omitir `conversation_id` mantém o comportamento atual.

## Testes

- Go: `build`, `vet` e `test` dos pacotes afetados (tasklist, tools/tasklist,
  history, database, chat) verdes; novo `TestTask_ConversationLink` cobre criar
  vinculado, update apenas-do-vínculo (preserva título) e limpar vínculo.
- Frontend: `tsc --noEmit`, `eslint` e `vitest` (store + componentes de
  tasklists) verdes.

## Trabalho futuro

- Filtro/agrupamento de tasklists por conversa nas telas de listagem.
- Custom action "abrir conversa" pré-configurada (AEP-0067) quando houver vínculo.
- Exibir, na timeline da conversa, as tasks/tasklists derivadas dela.
