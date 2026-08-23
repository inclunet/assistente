# Plano: Task List Manager Feature

**Status:** Done

> **Nota de status (alinhamento 2026-06)** — Documento parcialmente **histórico**.
> A feature foi implementada, mas o **modelo de dados** abaixo divergiu do código
> real. As seções de modelo de dados foram corrigidas para refletir a
> implementação (`internal/database/models.go`). Divergências principais que esta
> nota e as correções abaixo resolvem:
> - **IDs**: hoje são **UUIDv7** (AEP-0046), não inteiros sequenciais. Exemplos
>   com IDs `int` no texto são ilustrativos/históricos.
> - **Origem externa**: `ExternalSource`/`ExternalID`/`ExternalParentID`/
>   `ExternalUpdatedAt` vivem em **`TaskNote`**, **não** em `Task`. No `Task`, o
>   campo **`Code` funciona como external id** (equivalente, preenchível manual).
> - **`Task`** não tem `Priority`, `Tags`, `Progress` nem `LinkedMessageID`
>   (não implementados). Tem `Code`, `Link`, `AssigneeName/ID`, `CreatorName/ID`,
>   `CompletedAt`.
> - **`TaskList`** não tem `Type` (personal/ephemeral), `WorkflowID` (FK),
>   `Metadata` (JSON) nem `ConversationID` no model atual. Tem `UserID`, `Slug`,
>   `Description`, `PreferredViewMode`, `ValidationPolicy`. O `Workflow` é uma
>   **tabela separada** referenciada por `TaskListID` (não FK no `TaskList`).
> - Evolução posterior (eventos de domínio + custom actions de card): ver
>   **AEP-0067**.

## TL;DR

Implementar um **sistema de gerenciamento de TaskLists reutilizáveis** que funciona em 3 contextos:
1. **TaskLists pessoais** — página dedicada (`/tasklists`) com tabs persistentes entre sessões
2. **TaskLists de conversa** — vinculadas 1:1 a conversas, visíveis no chat e na página dedicada com badge
3. **Hierarquia flexível** — TaskList → Tasks → SubTasks (promovíveis/rebaixáveis dinamicamente)

Com suporte futuro para sincronização externa (Jira, GitHub) via background service + tool calling.

---

## Arquitetura

### **1. Modelo de Dados**      (Backend)
    
> As definições abaixo refletem a **implementação real** (`internal/database/models.go`).
> Campos comuns (`ID`, `CreatedAt`, `UpdatedAt`) vêm de `UUIDModel` (IDs UUIDv7, AEP-0046).

**TaskListWorkflow** (define máquina de estado **per-tasklist**, customizável)
- `ID` (UUIDModel)
- `TaskListID` (uniqueIndex) → Cada TaskList tem seu próprio workflow (tabela separada)
- `Statuses` (string JSON): array `[{ id, order, label, color, icon }]`
  - `id`: identificador imutável (nunca muda mesmo se renomear)
  - `order`: posição (reordenável sempre)
  - `label`: nome do status (imutável se TaskList tiver Tasks; mutável se vazio)
  - `color`, `icon`: propriedades visuais (sempre mutáveis)
- `AllowedTransitions` (string JSON): `{ "1": [2,3], "2": [3] }` (keyed by status ID)
- `InitialStatusID` (int) → ID do status inicial para novas tasks
- _(Histórico/não implementado: `Name`, `DoneStatus`, `CanBeModifiedByLLM`.)_

**TaskList** (entidade raiz; workflow em tabela separada)
- `ID` (UUIDModel), `UserID`
- `Title`, `Slug` (identificador estável portável, opcional/único quando não vazio)
- `Description`
- `PreferredViewMode` ("list" | "kanban")
- `ValidationPolicy` (string JSON opcional — regras de code de tasks e notas externas)
- Relacionamentos: `Workflow` (por `TaskListID`), `Tasks`
- _(Histórico/não implementado no model: `Type` personal/ephemeral, `ConversationID`, `WorkflowID` FK, `Metadata`.)_

**Task** (com hierarquia via `ParentID` e status por ID)
- `ID` (UUIDModel), `TaskListID`
- `Title`, `Description`
- `Code` (size:128, index) ← **funciona como external id** (preenchível manual; normalmente equivale ao id do sistema de origem)
- `Link` (size:512) ← link externo do card
- `StatusID` (int, aponta para `TaskListWorkflow.Statuses[].id`)
- `ParentID` (nullable) ← recursão para subtasks
- `Order`
- `AssigneeName`, `AssigneeID`
- `CreatorName`, `CreatorID`
- `DueDate`, `CompletedAt` (nullable)
- Relacionamentos: `TaskList`, `Parent`, `Subtasks`, `Notes`
- _(Histórico/não implementado: `Priority`, `Tags`, `Progress`, `LinkedMessageID`, `ExternalID/ExternalSource` no Task — origem externa vive em `TaskNote`.)_

**TaskNote** (nota/interação de uma task; carrega a origem externa)
- `ID` (UUIDModel), `UserID`, `TaskID`
- `Type` (TaskNoteType: 1 interna, 2 cliente, 3 agente, 4 sistema)
- `Content`
- `AuthorName`, `AuthorID`
- `ExternalSource` (coluna `external_source`), `ExternalID`, `ExternalParentID`, `ExternalUpdatedAt` ← origem externa (sync Jira/FSD/etc.); upsert idempotente por `(external_source, external_id)`

**TaskListHistory** (persistência de tabs)
- `TaskListID`, `Position`, `LastAccessedAt` ← recarrega tabs ao abrir app

---

## Observação Importante: Status por ID vs String

O modelo usa **StatusID (int)** em vez de string para permitir:
- ✅ Reordenar estados (só muda `order`, não ID)
- ✅ Renomear labels **se sem tasks** (label é mutável, ID é imutável)
- ✅ Manter integridade de referências (Tasks sempre apontam para StatusID válido)

**Exemplo**:
```
Workflow inicial:
[{id:1, label:"A Fazer"}, {id:2, label:"Em Andamento"}, {id:3, label:"Concluído"}]

Reordenar: arrastar col 3 → col 1
Resultado:
[{id:3, order:0, label:"Concluído"}, {id:1, order:1, label:"A Fazer"}, {id:2, order:2, label:"Em Andamento"}]

Tasks com StatusID 2 ainda apontam para "Em Andamento" (label pode mudar, ID não)
```

---

### **2. State Frontend** (Zustand)

**Novo store**: `taskListStore.ts`
- `openTabs[]` — TaskLists abertos em abas (similar ao chat)
- `activeTabId` — aba ativa
- `expandedTasks` — quais subtasks estão expandidos
- `workflows` — cache de TaskListWorkflows **por TaskList** (carregado com cada TaskList)
- Actions: createTaskList, cloneTaskList, addTask, updateTask, linkToConversation, promoteSubTaskList, **changeTaskStatus**, **updateWorkflow**, **reorderStatusStates**, **setTaskListViewMode**, etc.

**Frontend persiste**:
- Tabs abertos + estado (qual aba estava ativa)
- Tarefas expandidas/colapsadas
- Posição das abas

**ViewMode (por TaskList)**:
- Ao carregar TaskList, fetch `PreferredViewMode` (list ou kanban)
- User pode alternar views na PageToolbar (view selector)
- Alteração salva em DB via `SetTaskListViewMode(taskListID, mode)`
- Próxima vez que abre mesma tasklist, usa preferência salva

**Status dinâmico (por ID)**:
- Ao carregar TaskList, fetch seu workflow (statuses com IDs e propriedades)
- Task.StatusID é um int que aponta para Status[].id do workflow
- Dropdown de status mostra apenas transições válidas (AllowedTransitions do workflow)
- Reordenação: ao arrastar status no kanban (ou via UI), apenas `order` muda
- Imutabilidade: Ao tentar editar label com Tasks existentes, bloqueia com mensagem

---

### **3. Páginas & Rotas**

**Nova rota**: `/tasklists` → `TaskListsPage.tsx`

Layout:
```
[Topbar com history + tabs (como ChatPage)]
        ↓
[Toolbar: search, filtro status/tipo, view mode selector (list/kanban), ações]
        ↓
[TaskListView: TasksTable (list) OU TasksKanban (kanban) — based on PreferredViewMode]
```

**View Modes (por TaskList)**:
1. **List View** (Fase 1) — DataGrid com expand/collapse hierárquico, status dropdown
2. **Kanban View** (Fase 1: selector, Fase 3: implementação) — Colunas por status do workflow, drag-drop, acessibilidade

**Seletor de View Mode**:
- Toggle button/selector na PageToolbar (list icon vs kanban icon)
- Ao clicar: alterna entre views (ambas renderizando mesma TaskList)
- Salva preferência em DB: `SetTaskListViewMode(taskListID, mode)`
- Próxima abertura da mesma TaskList usa preferência salva

**Na ChatPage**: 
- Badge mostrando "TaskList: Feature Dev" se vinculada
- Botão "Desvincular" e "Abrir em página dedicada"
- Botão para abrir `TaskListSelectorModal` (trocar/ligar tasklist)

---

### **4. Linking Conversa ↔ TaskList**

**Unidirecional + reversível**:
1. Conversa pode ter 1 TaskList vinculada (FK)
2. TaskList pessoal pode estar referenciada em múltiplas conversas (via links/deep links)
3. **Desvincular**: Remove FK, auto-promove ephemeral → pessoal
4. **Trocar**: Modal selector abre grid de TaskLists, user escolhe qual vincular
5. **Deep links**: `/tasklists?open=123` abre TaskList no modal

---

### **5. Bindings Wails** (Backend)

Exemplos principais:
```go
// TaskList CRUD
CreateTaskList(title, conversationID?, workflowTemplate?) → TaskList
GetTaskListWithTasks(id) → TaskListWithTasks (hierarchy + workflow info)
UpdateTaskList(id, updates) → TaskList
DeleteTaskList(id) → error
CloneTaskList(taskListID, newTitle?) → TaskList // copia workflow + estrutura, sem tasks

// Workflow management (per-tasklist)
GetWorkflow(taskListID) → TaskListWorkflow
UpdateWorkflow(taskListID, updates) → error (com validação: não pode renomear label se tem tasks)
ReorderWorkflowStatuses(taskListID, newOrder [statusID]) → error
ValidateWorkflowChange(taskListID, proposedChanges) → error (pré-validação antes de update)

// Task CRUD
CreateTask(taskListID, parentTaskID?, title, statusID?) → Task
UpdateTask(id, updates) → Task (valida statusID contra workflow)
DeleteTask(id) → error

// Linking & operations
LinkTaskListToConversation(taskListID, conversationID) → error
UnlinkTaskListFromConversation(taskListID) → error
PromoteSubTaskListToRoot(subTaskListID) → error
MoveTask(taskID, newParentID?, position) → error
SyncTaskListFromExternal(id) → error

// View preference (per TaskList)
SetTaskListViewMode(taskListID, mode) → error  // "list" ou "kanban"
```

**Events**: 
- tasklist:created, tasklist:cloned, tasklist:updated, tasklist:deleted
- task:created, task:updated, task:deleted
- workflow:updated (com details: which statusID changed, how)
- etc.

---

### **5b. LLM Tools (Tool Calling - Fase 4)**

LLM terá acesso via **tool calling** a operações TaskList/Task:

| Tool | Descrição | Parâmetros | Retorno |
|------|-----------|-----------|---------|
| **upsert_task** | Cria ou atualiza task (unificado) | `task_list_id` (int), `task_id?` (int, se update), `title` (str), `description?` (str), `status_id?` (int), `priority?` (int), `parent_task_id?` (int), `progress?` (int) | Task com ID |
| **get_task_list** | Busca tasklist com todas as tasks | `task_list_id` (int) | TaskListWithTasks (hierarchy) |
| **get_task_list_status** | Status resumido de uma tasklist | `task_list_id` (int) | {total_tasks, todo_count, doing_count, done_count, workflow_name} |
| **create_task_list** | Cria nova tasklist com workflow | `title` (str), `conversation_id?` (int), `workflow_template?` (str: "simple"\|"kanban"\|"custom") | TaskList com workflow |
| **list_task_lists** | Lista todas tasklists do usuário (pessoais + conversas atuais) | Nenhum | [{id, title, type, workflow_name, task_count}] |
| **bulk_upsert_tasks** | Cria/atualiza múltiplas tasks em batch | `updates` [{task_id?, task_list_id, title, status_id?, priority?, progress?}] | {success_count, failed_count, errors?} |
| **delete_task** | Deleta uma task | `task_id` (int) | {success: bool, message: str} |

**Lógica de `upsert_task`**:
- Se `task_id` fornecido → UPDATE (validar status_id contra AllowedTransitions)
- Se `task_id` não fornecido → CREATE (usar InitialStatus como default)

**Status ID Enum (Dinâmico)**:
- Cada tool com parâmetro `status_id` recebe enum **gerado do workflow da tasklist**
- LLM sempre se refere a status por ID (1, 2, 3...), nunca por label
- Frontend/Backend mapeia ID → label para renderizar

**Validações Automáticas**:
- ✅ LLM tenta setar `status_id` que não existe no workflow → erro claro
- ✅ LLM tenta criar task em tasklist deleted → erro
- ✅ LLM tenta atualizar task com transição não-permitida → erro (com lista de transitions válidas)

---

---

---

### **6. Três Fases de Implementação**

#### **Fase 1: Core** (1-2 semanas)
- ✅ DB schema (TaskListWorkflow, TaskList, Task, TaskListHistory)
- ✅ Backend CRUD + repository functions
- ✅ Workflow management (CRUD workflows)
- ✅ Status validation contra workflow
- ✅ Frontend store, types, components básicos (list view)
- ✅ Linking conversa ↔ tasklist (com workflow inheritance)
- ✅ UI leitura/edição inline (status dropdown dinâmico contra workflow)
- ❌ Tab persistence, ❌ Kanban view, ❌ Tool calling, ❌ External sync

#### **Fase 2: Tabs + UX + Kanban Planning** (1-2 semanas)
- ✅ Persistência de tabs (DB + restore ao reload)
- ✅ TopBar com history dropdown (como chat)
- ✅ Promoção/demoção de subtasks
- ✅ Navegação keyboard (Tab, arrows, Enter, ESC)
- ✅ i18n completo
- ✅ **View Mode selector** na PageToolbar (alterna list/kanban, salva preferência por TaskList)
- 📋 **Kanban View planning** (design de acessibilidade, componente arquitetura)
  - Definir aria-roles para kanban (ARIA gridcell, live regions)
  - Keyboard nav para drag/drop alternativo (sem mouse)
  - Design visual (colunas por status, cards)
  - Testes de acessibilidade (WAVE, screen reader)

#### **Fase 3: Kanban Implementation** (1-2 semanas)
- ✅ TasksKanban.tsx component (visual)
- ✅ Drag-drop com react-dnd (acessível)
- ✅ Keyboard-based reparenting (setas + Enter)
- ✅ Status automation (ex: mover para "done" auto-marca conclusão)
- ✅ Acessibilidade enforcement (contrast, focus, announcer)

#### **Fase 4: LLM + Sync Prep** (1-2 semanas)
- ✅ Tool calling (LLM criar/atualizar tasks com status dinâmico)
  - 7 tools registrados em `internal/tools/tasklist/`
  - Validação contra workflow em cada tool.Execute()
  - Enum de status é gerado do workflow, LLM sempre usa schema atualizado
  - Error handling user-friendly (descreve qual status é válido)
- ✅ System prompt context injection (opt-in)
  - User pode habilitar "Include task list context"
  - Tasks resumo incluído no system prompt se enabled
- ✅ ExternalID management (Jira, GitHub fields)
- ✅ Estrutura de sync adapters (sem runtime)
  - Sync adapter respects workflow status mapping (ex: Jira "In Progress" → app "doing")

---

## Arquivos Relevantes

| Arquivo | Tipo | Ação |
|---------|------|------|
| `internal/database/models.go` | Modify | Adicionar Types TaskListWorkflow, TaskList, Task |
| `internal/database/tasklist_repository.go` | **NEW** | Queries: CRUD, hierarchy, workflow queries |
| `app.go` / `app_tasklist.go` | Modify/NEW | Wails bindings (CRUD + workflow) |
| `db.go` | Modify | TaskList + workflow DB setup |
| `frontend/src/types/tasklist.ts` | **NEW** | Interfaces TypeScript (Task, TaskList, Workflow) |
| `frontend/src/store/taskListStore.ts` | **NEW** | Zustand store + workflow cache |
| `frontend/src/pages/TaskListsPage.tsx` | **NEW** | Página principal |
| `frontend/src/components/tasklist/TaskListView.tsx` | **NEW** | View mode selector + router |
| `frontend/src/components/tasklist/TasksTable.tsx` | **NEW** | List view (Phase 1) |
| `frontend/src/components/tasklist/TasksKanban.tsx` | **NEW** | Kanban view (Phase 3), com acessibilidade |
| `frontend/src/components/tasklist/TaskForm.tsx` | **NEW** | Inline/modal form (status dropdown dinâmico) |
| `frontend/src/components/tasklist/TaskListHeader.tsx` | **NEW** | Título, workflow selector, sync status |
| `frontend/src/components/tasklist/WorkflowModal.tsx` | **NEW** | Modal para criar/editar workflows |
| `frontend/src/components/tasklist/TaskListSelectorModal.tsx` | **NEW** | Modal picker para linking conversas |
| `frontend/src/pages/ChatPage.tsx` | Modify | Badge + button |
| `frontend/src/lib/router.tsx` | Modify | Adicionar `/tasklists` |
| `frontend/src/locales/*.ts` | Modify | i18n keys (3 idiomas) |

**Componentes Adicionais para Clonagem & Workflows**:
- `TaskListContextMenu.tsx` — Menu com ação "Clone"
- `WorkflowStatusEditor.tsx` — Sub-component para editar order/label/color/icon de status
- `WorkflowTransitionEditor.tsx` — Sub-component para definir AllowedTransitions
- `CloneTaskListModal.tsx` — Modal para clone (com opção: clone estrutura apenas vs. com tasks)

**Tools para LLM (Phase 4)** — Nova estrutura (7 tools em 7 arquivos):
- `internal/tools/tasklist/factory.go` — **NEW** — Factory pra registrar todos tools
- `internal/tools/tasklist/upsert_task.go` — **NEW** — Tool: upsert_task (unified create/update)
- `internal/tools/tasklist/get_task_list.go` — **NEW** — Tool: get_task_list
- `internal/tools/tasklist/get_task_list_status.go` — **NEW** — Tool: get_task_list_status
- `internal/tools/tasklist/create_task_list.go` — **NEW** — Tool: create_task_list
- `internal/tools/tasklist/list_task_lists.go` — **NEW** — Tool: list_task_lists
- `internal/tools/tasklist/bulk_upsert_tasks.go` — **NEW** — Tool: bulk_upsert_tasks
- `internal/tools/tasklist/delete_task.go` — **NEW** — Tool: delete_task
- `app.go` | Modify | Registrar tools em `initToolRegistry()` (gated by feature flag)

---

## Verificação

**Testes Automáticos**:
```bash
go test ./...                      # Backend CRUD + linking
npm run test --prefix frontend     # Vitest (store, components)
npm run lint --prefix frontend     # TypeScript + ESLint
npm run build --prefix frontend    # Build sem erros
```

**Manual** (Fase 1):
- [ ] Criar TaskList pessoal → adicionar tasks → expandir/colapsar funciona
- [ ] Vincular a conversa → badge aparece → desvincular → promove a pessoal
- [ ] Navegação por keyboard (Tab, setas, Enter, ESC)
- [ ] Tema escuro/claro funciona (variáveis CSS)

**Manual** (Fase 2):
- [ ] Abrir 2+ tasklists em abas → fechar app → reabrir → abas restauradas
- [ ] Histórico dropdown mostra tasklists recentes

---

## Decisões Críticas

| Decisão | Justificativa |
|---------|--------------|
| **Workflows per-tasklist** | Cada TaskList customiza seu workflow independente; LLM pode criar/modificar |
| **TaskLists clonáveis** | Mais simples que templates separados; user replica estrutura útil |
| **Status identificado por ID (int)** | Permite reordenar + renomear; labels vêm dados do workflow, não hardcoded |
| **Imutabilidade condicional** | Reordenar sempre permitido; renomear label bloqueado se task existe |
| **Kanban columns reordenáveis** | User reorganiza colunas por status; apenas `order` field muda |
| **1:1 Conversation ↔ TaskList** | Simplifica, mas allows refs via deeplinks |
| **Task hierarchy via ParentTaskID** | Vs. SubTaskList entity — mais simples, menos migration burden |
| **DB persistence de tabs** | Vs. localStorage — suporta tasklists always-on (Jira board) |
| **Status dinâmico em tool calling** | Enum de statusID é gerado do workflow, LLM sempre usa schema atualizado |
| **Kanban implementado Phase 3** | List view suficiente Phase 1, kanban exige mais UX/acessibilidade testing |
| **Phase 4 para sync/tools** | Reduz initial scope, permite feedback de users |

---

## Detalhes Complementares

### **Hierarquia de Tasks (Promocão/Demoção)**

Quando um usuário tem subtasks, pode:
1. **Promover** uma SubTaskList → TaskList root (cria TaskList novo, reparenta tasks)
2. **Rebaixar** uma TaskList → SubTask de uma TaskList parent (move para ParentTaskID)
3. Conversa ao **desvincular** tasklist ephemeral → auto-promove a pessoal

### **Deep Linking & Navegação**

- `/tasklists?open=123` → Abre TaskList #123 em aba nova (ou ativa se já aberta)
- `/chat/456?tasklistId=789` → Future: split view conversa + tasklist
- Badge em conversa → Clickável, navega para `/tasklists?open={id}`

### **Sincronização Externa (Future)**

Preparatório em Phase 4:
- Task.ExternalID, Task.ExternalSource (Jira, GitHub, GitLab, etc.)
- Background sync service (separate Go service ou goroutine)
- **Status mapping customizável**: Sync adapter mapeia external status → app workflow status **por ID**
  - Ex: Jira ["To Do", "In Progress", "Done"] → [1, 2, 3] (IDs do app workflow)
  - Admin pode configurar mapeamento: external_label → status_id
- Conflict resolution: last-write-wins com undo history
- Tool calling: LLM recebe enum de statusID dinâmico do workflow, pode criar/atualizar tasks

**Status Interop**:
- Backend mantém tabela de mapeamento: ExternalStatusMapping {externalSource, externalStatus, localStatusID}
- Ao sync: Jira "In Progress" → lookup mapping → statusID 2 → update Task.StatusID
- Ao exportar: Task.StatusID 2 → lookup reverse mapping → "In Progress" (enviado para Jira)

### **Workflow Customizável & Tool Calling**

**Backend**:
- Cada TaskList tem seu próprio TaskListWorkflow (criado junto)
- Ao criar TaskList, pode usar template padrão ou deixar LLM customizar (se CanBeModifiedByLLM=true)
- Ao validar status change, usar StatusID (int), não label
- AllowedTransitions keyed by StatusID (ex: {"1": [2, 3], "2": [1]})
- Validar label rename: se TaskList.Tasks existe, bloqueia
- Tool schema gerado dinamicamente:
  ```go
  // Example tool for "Create Task"
  {
    "name": "create_task",
    "parameters": {
      "task_list_id": "integer",
      "title": "string",
      "status_id": {
        "type": "integer",
        "enum": [1, 2, 3],  // IDs dinâmicos do workflow!
        "description": "Status ID according to workflow"
      },
      ...
    }
  }
  ```

**Frontend**:
- Quando carregar TaskList, fetch seu workflow (com Statuses array completo)
- Status dropdown renderiza labels + colors + icons do workflow
- Internally, Task usa StatusID (int), frontend mapeia para label na UI
- Ao reordenar no kanban: apenas `order` muda no backend via `ReorderWorkflowStatuses`
- Ao tentar renomear label com tasks: backend retorna erro, frontend mostra toast

**Fluxo de Customização de Workflow por LLM**:
1. User pede: "crie uma tasklist com workflow tipo Jira"
2. LLM chama `CreateTaskList(title, conversationID, workflowTemplate="jira")`
3. Backend inicializa TaskListWorkflow pré-configurado
4. Se LLM quer depois modificar: chama `UpdateWorkflow(taskListID, {statuses=[...], transitions=[...]})`
5. Backend valida, aceita ou rejeita
6. Frontend recebe `workflow:updated` event, atualiza status dropdown

**Examples de Workflows Padrão** (built-in templates):
1. **Simple** (default)
   - Statuses: [{id:1, order:0, label:"A Fazer"}, {id:2, order:1, label:"Concluído"}]
   - AllowedTransitions: {"1": [2], "2": [1]}

2. **Kanban Padrão**
   - Statuses: [{id:1, order:0, label:"A Fazer"}, {id:2, order:1, label:"Em Andamento"}, {id:3, order:2, label:"Concluído"}]
   - AllowedTransitions: {"1": [2], "2": [1, 3], "3": [2]}

3. **Jira-like**
   - Statuses: [{id:1, order:0, label:"Backlog"}, {id:2, order:1, label:"Selected"}, {id:3, order:2, label:"In Progress"}, {id:4, order:3, label:"In Review"}, {id:5, order:4, label:"Done"}]
   - AllowedTransitions: complex rules

### **Kanban View (Planejado Phase 2, Implementado Phase 3)**

**Design Geral**:
- Colunas mapeadas aos status do workflow (ordenadas por `Statuses[].order`)
- Cards são tasks (com título, descrição, tags, status color/icon visíveis)
- Drag-drop para mover entre colunas (respeitando AllowedTransitions)
- **Colunas reordenáveis**: arrastar coluna atualiza `order` de status no workflow
- Subtasks renderizadas como sub-cards ou colapsáveis
- **Labels imutáveis com items**: coluna com tasks exibe ícone lock se tentar editar label

**Reordenação & Imutabilidade**:
- User arrasta coluna A → B → C → updates order [statusC.id, statusA.id, statusB.id]
- Backend: reorder apenas `order` field, statusIDs stays same
- Tasks referem-se por StatusID (int), então renomear label é safe
- Se user tenta edit label: `ValidateWorkflowChange` retorna erro se has tasks

**Acessibilidade (CRUCIAL)**:
```
ARIA pattern: role="grid" com aria-describedby
├─ role="columnheader" para cada coluna (status label)
├─ role="gridcell" para cada card
└─ Keyboard nav:
   ─ Tab → navega entre cards
   ─ Arrows (left/right) → navega entre colunas/statuses
   ─ Shift+Arrows → drag-drop alternativo (move card left/right)
   ─ Enter → inicia edit do card
   ─ ESC → cancela operação
   
// Announcer feedback:
"Card: Feature Dev (statusID 2), status: Em Andamento, column 2 of 3"
"Moved to Concluído column"
"Column order updated: A Fazer, Em Andamento, Concluído"
```

**Componente**:
- `TasksKanban.tsx` — Layout grid + columns (reordenáveis via drag)
- Usar `react-dnd` com acessibilidade (HeadlessUI utilities)
- Status column header com drag handle (para reordenar colunas)
- Integrar com useAnnouncer para feedback de leitores de tela

### **Clonagem de TaskList**

Ao invés de ter templates separados, TaskLists são clonáveis:
1. User clica "Clone" em uma TaskList pessoal
2. Backend: cria nova TaskList com:
   - Novo Title (ex: "Feature Dev (copy)")
   - Clona TaskListWorkflow (novo ID)
   - Clona estrutura de Tasks (sem dados de tasks efetivas)
   - Metadata.cloneOriginID = original TaskList ID (para referência)
3. Novo workflow pode ser customizado sem afetar o original

Benefício: User pode ter múltiplas variações de workflow (Jira, Kanban, Simple) sem admin de templates separados.

### **i18n Strategy**

Todas strings visíveis:
```typescript
// exemplo
t('tasklist.modal.title')        // "Nova Lista de Tarefas"
t('task.status.label.1')         // Lookup dinâmico: `workflow.statuses.find(s => s.id === 1).label`
t('tasklist.badge.unlink')       // "Desvincular"
t('tasklist.viewMode.list')      // "Visualização em Lista"
t('tasklist.viewMode.kanban')    // "Visualização Kanban"
t('workflow.template.simple')    // "Workflow Simples"
t('workflow.template.kanban')    // "Workflow Kanban Padrão"
t('workflow.template.jira')      // "Workflow Jira"
t('error.workflow.cannotRenameWithItems') // "Não é possível renomear status com tarefas existentes"
```

Locales: `frontend/src/locales/{pt-BR,en,es}.ts`

**Workflow Labels (Dinâmicos)**:
- Labels armazenados no TaskListWorkflow (ex: { id: 1, label: "A Fazer" })
- Frontend renderiza label do workflow (lookup por StatusID)
- User pode editar labels **se TaskList não tiver tasks**
- Labels são traduzidos server-side ao criar workflow padrão (já com labels pt-BR set)

### **Padrões de Acessibilidade**

- **DataGrid**: Reutilizar componente existente (já tem role="grid", navegação por teclado)
- **Modal**: Reutilizar componente existente (focus trap, aria-hidden parent, ESC)
- **Keyboard nav**: 
  - **List View**: Tab → navega entre tarefas; Arrows → expande/colapsada subtasks; Enter → edit
  - **Kanban View**: Tab → navega cards; Shift+Arrows → move card entre colunas; Arrows(left/right) → navega colunas; Enter → edit card
- **Tema**: CSS variables (`--bg-base`, `--text-primary`, etc.)
- **Status Visual**: Color + icon (não identificar apenas por cor)
- **Contraste**: Mínimo AA (4.5:1), visar AAA (7:1) onde possível
- **Feedback auditivo**: Usar `announce()` via `useAnnouncer` para ações (criar task, mover coluna, etc.)

### **Performance Considerations**

- **Lazy loading**: Like chat, carregar apenas raízes; subtasks under demand via expand
- **Hierarchy limit**: Se >500 tasks, considerar flat view com filtering
- **Debounce**: Edições inline debounce 500ms antes de enviar ao backend
- **Batch updates**: LLM tool calling pode atualizar múltiplas tasks em 1 request

---

## Próximos Passos (Se Aprovado)

1. **Refinar** detalhes de workflow customization (ex: suportar sub-workflows? lógica condicional de transições?)
2. **Definir workflows padrão** (Simple, Kanban, Jira-like como built-in, com labels pt-BR)
3. **Validar** acessibilidade Kanban + reordenação de colunas antes de implementar (Phase 3 planning)
4. **Reserve time** para Phase 1 (escolher week de start)
5. **Coordenar** com team se há outras features blockeando deps
6. **Considerar** MVP reduzido (ex: workflows fixed, só list plana) se timeline é aperada

**Questões respondidas** ✅:
- [x] Workflows são global ou per-tasklist? → **Per-tasklist**
- [x] Workflow templates clonáveis? → **TaskLists clonáveis** (mais simples)
- [x] Status pode ter propriedades (color, icon)? → **Sim** (color, icon em Status struct)
- [x] Kanban columns reordenáveis? → **Sim** (apenas `order` muda, não label se has items)

**Questões ainda em aberto** (para refinamento posterior):
- [ ] LLM automaticamente sempre modifica workflow ao criar tasklist, ou apenas se explicitamente pedido?
- [ ] Transições podem ter lógica customizável (ex: "done" requer todos os subtasks done)?
- [ ] Priorizar qual workflow padrão (Simple vs Kanban) como default ao new TaskList?
- [ ] Suportar remover status de um workflow se não for mais usado?
- [ ] Ao clonar TaskList, copiar também Tasks (com opção) ou sempre vazio?

---

## Arquitetura de Tools (LLM Tool Calling)

### **Localização dos Tools**

Nova estrutura em `internal/tools/tasklist/`:
```
internal/tools/
├── tasklist/
│   ├── upsert_task.go           # upsert_task tool (create OR update)
│   ├── get_task_list.go         # get_task_list tool
│   ├── get_task_list_status.go  # get_task_list_status tool
│   ├── create_task_list.go      # create_task_list tool
│   ├── list_task_lists.go       # list_task_lists tool
│   ├── bulk_upsert_tasks.go     # bulk_upsert_tasks tool
│   ├── delete_task.go           # delete_task tool
│   └── factory.go               # Factory para registrar todos tools
```

### **Estrutura de cada Tool**

Cada tool implementa a interface `Tool` de `internal/tools/types.go`:

```go
// Example: upsert_task.go (create OR update)
type UpsertTaskTool struct {
    app *App  // Referência para chamar backend
}

func (t *UpsertTaskTool) Name() string {
    return "upsert_task"
}

func (t *UpsertTaskTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
    // 1. Parse args (task_id?, task_list_id, title, status_id, etc.)
    // 2. If task_id: UPDATE; else: CREATE with InitialStatus as default
    // 3. Validate status_id contra workflow
    // 4. Call App.CreateTask() ou App.UpdateTask()
    // 5. Return result (Task ID, confirmation)
}
```

### **Registro de Tools**

Em `app.go`, função `initToolRegistry()`, add na seção apropriada:

```go
func (a *App) initToolRegistry() {
    // ... existing tools ...
    
    // TaskList tools (registered in Phase 4, gated by feature flag)
    if a.featureFlags.EnableTaskListTools {
        tasklistFactory := tasklist.NewFactory(a)
        a.toolRegistry.MustRegister(tasklistFactory.UpsertTask())      // unified create/update
        a.toolRegistry.MustRegister(tasklistFactory.GetTaskList())
        a.toolRegistry.MustRegister(tasklistFactory.GetTaskListStatus())
        a.toolRegistry.MustRegister(tasklistFactory.CreateTaskList())
        a.toolRegistry.MustRegister(tasklistFactory.ListTaskLists())
        a.toolRegistry.MustRegister(tasklistFactory.BulkUpsertTasks())
        a.toolRegistry.MustRegister(tasklistFactory.DeleteTask())
    }
}
```

### **Geração Dinâmica de Schema**

O `Parameters()` de cada tool gera schema **dinamicamente** baseado no workflow:

```go
// Em upsert_task.go
func (t *UpsertTaskTool) Parameters() json.RawMessage {
    // 1. Se task_list_id foi passada, fetch seu workflow
    // 2. Extrair status IDs válidos
    // 3. Gerar JSON Schema com enum de status_id
    
    schema := map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "task_list_id": {"type": "integer"},
            "task_id":      {"type": "integer", "description": "If provided, UPDATE; else CREATE"},
            "title":        {"type": "string"},
            "status_id":    {
                "type":        "integer",
                "enum":        []int{1, 2, 3},  // Dinâmico do workflow!
                "description": "Status ID according to task list workflow",
            },
            // ...
        },
        "required": []string{"task_list_id", "title"},
    }
    return json.Marshal(schema)
}
```

**Problema**: Schema é estático (enviado uma vez ao inicio), não pode mudar mid-conversation.

**Solução**: 
- Enviar todos status IDs possíveis (1-100), con descrição sobre workflow
- Tool executa validação no `Execute()` e retorna erro se status inválido
- Output do erro é descritivo: "Status 5 not available in workflow 'Simple'" (liste válidos)

### **Validação em Tool Execution**

Cada tool valida params antes de executar:

```go
func (t *UpsertTaskTool) Execute(ctx context.Context, args json.RawMessage) (tools.ToolResult, error) {
    var req upsertTaskRequest
    if err := json.Unmarshal(args, &req); err != nil {
        return tools.ToolResult{
            Content: "Invalid parameters: " + err.Error(),
            IsError: true,
        }, nil  // Return error as content, not error return
    }
    
    // Fetch workflow to validate status_id
    workflow, _ := db.GetWorkflow(req.TaskListID)
    if !workflow.HasStatus(req.StatusID) {
        validStatuses := workflow.GetAllStatusIDs()
        return tools.ToolResult{
            Content: fmt.Sprintf(
                "Status %d not valid. Valid statuses: %v",
                req.StatusID, validStatuses),
            IsError: true,
        }, nil
    }
    
    // Determine CREATE or UPDATE
    var task *Task
    var err error
    if req.TaskID != nil {
        // UPDATE: validate transition
        oldTask, _ := db.GetTask(*req.TaskID)
        if !workflow.AllowedTransitions[oldTask.StatusID].Contains(req.StatusID) {
            return tools.ToolResult{
                Content: fmt.Sprintf(
                    "Transition from status %d to %d not allowed",
                    oldTask.StatusID, req.StatusID),
                IsError: true,
            }, nil
        }
        task, err = a.UpdateTask(ctx, *req.TaskID, req.StatusID, ...)
    } else {
        // CREATE: use InitialStatus if not provided
        if req.StatusID == nil {
            req.StatusID = &workflow.InitialStatus
        }
        task, err = a.CreateTask(ctx, req.TaskListID, req.Title, *req.StatusID, ...)
    }
    
    if err != nil {
        return tools.ToolResult{
            Content: "Error: " + err.Error(),
            IsError: true,
        }, nil
    }
    
    action := "created"
    if req.TaskID != nil {
        action = "updated"
    }
    
    return tools.ToolResult{
        Content: fmt.Sprintf("Task %s: %d (%s)", action, task.ID, task.Title),
        IsError: false,
    }, nil
}
```

### **Permissões & Gating**

Tools podem ser controlados via Profile config:

```go
type Profile struct {
    Chat ChatConfig
}

type ChatConfig struct {
    DisableTools bool             // Desabilita tools (não LLM tasks)
    EnabledTools []string         // Whitelist de tools específicos
    DisableTaskListTools bool      // Novo: desabilita só TaskList tools
    MaxTaskListChanges int         // Novo: máximo de tasks que LLM pode criar/mudar por conversa
}
```

---

## Resumo: Tools Necessários para Phase 4

| Fase | Tools | Status |
|------|-------|--------|
| **Phase 1-3** | Nenhum (Frontend-only) | ✅ CRUD manual |
| **Phase 4: LLM** | `upsert_task`, `get_task_list`, `get_task_list_status`, `create_task_list`, `list_task_lists`, `bulk_upsert_tasks`, `delete_task` | 📋 Implementar em 7 arquivos |
| **Future: Sync** | Sync adapters (Jira, GitHub, etc.) | 🔮 Out of scope |

**7 Tools:**
1. `upsert_task` — CREATE (sem task_id) ou UPDATE (com task_id)
2. `get_task_list` — Fetch completo
3. `get_task_list_status` — Status resumido
4. `create_task_list` — Nova tasklist
5. `list_task_lists` — Listar todas
6. `bulk_upsert_tasks` — Batch create/update
7. `delete_task` — Deletar task

Cada tool: 
- Implementa `Tool` interface
- Valida input contra workflow
- Retorna error-friendly content (nunca erro fatal)
- Emite eventos (task:created, task:updated, etc.)


