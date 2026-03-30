# AEP-0035: Split View

- **Status**: Draft
- **Autor**: Leonardo Gleison Ferreira
- **Data**: 2026-03-23
- **Depende de**: AEP-0034 (Unified Workspace)

## Resumo

Permite visualizar múltiplas abas simultaneamente no workspace através de painéis divididos (split view). A divisão é recursiva — suporta N painéis em qualquer combinação de horizontal e vertical, similar ao VS Code. Para acessibilidade, apenas o painel ativo é interativo; os demais recebem `inert`.

## Motivação

Com o workspace unificado (AEP-0034), o usuário trabalha com abas mistas (chat, editor, terminal, tasklist) em uma única barra. Porém, só consegue ver uma aba por vez. Em fluxos de trabalho comuns — investigar um ticket enquanto roda comandos, ou editar código enquanto conversa com o LLM — a necessidade de alternar constantemente entre abas fragmenta o contexto visual.

### Problemas

- **Contexto visual limitado**: apenas 1 aba visível por vez
- **Alternância constante**: Ctrl+Tab repetido para comparar conteúdos
- **Fluxos multi-aba**: investigação (chat + terminal), desenvolvimento (editor + chat), triagem (tasklist + chat) exigem visão simultânea

## Modelo Core

### Árvore de Splits

O layout é uma árvore binária recursiva onde:

- **Folha (leaf)**: contém um **grupo de abas** (editor group)
- **Ramo (branch)**: divide o espaço entre filhos em uma direção (horizontal ou vertical)

```
SplitNode (branch, horizontal)
├── SplitNode (leaf, group-1)    ← [💬 FSD-456] [>_ terminal]
└── SplitNode (branch, vertical)
    ├── SplitNode (leaf, group-2) ← [📝 main.go]
    └── SplitNode (leaf, group-3) ← [✅ Tickets]
```

### Editor Groups

Cada grupo é um container independente de abas dentro de um split:

| Propriedade | Descrição |
|-------------|-----------|
| `id` | ID único do grupo |
| `activeTab` | Qual aba está visível neste grupo |

As abas continuam na lista global do workspace (`tabs.items`), mas cada uma aponta para o grupo ao qual pertence via `group_id`.

### Grupo Ativo

O workspace mantém um `active_group` que indica qual grupo está com foco. Atalhos de teclado, F6 e interações operam sobre o grupo ativo.

## Tipos

### Backend (Go)

```go
type SplitDirection string

const (
    SplitHorizontal SplitDirection = "horizontal"
    SplitVertical   SplitDirection = "vertical"
)

type SplitNode struct {
    Type      string         `json:"type" yaml:"type"`                                   // "leaf" | "branch"
    GroupID   string         `json:"group_id,omitempty" yaml:"group_id,omitempty"`        // leaf only
    Direction SplitDirection `json:"direction,omitempty" yaml:"direction,omitempty"`       // branch only
    Children  []SplitNode    `json:"children,omitempty" yaml:"children,omitempty"`         // branch only
    Sizes     []float64      `json:"sizes,omitempty" yaml:"sizes,omitempty"`               // branch only (proporções)
}

type EditorGroup struct {
    ID        string `json:"id" yaml:"id"`
    ActiveTab string `json:"active_tab" yaml:"active_tab"`
}
```

### Alterações em tipos existentes

```go
// Tab ganha GroupID
type Tab struct {
    // ... campos existentes
    GroupID string `json:"group_id,omitempty" yaml:"group_id,omitempty"`
}

// TabsState ganha campos de split
type TabsState struct {
    Active      string        `json:"active" yaml:"active"`
    ActiveGroup string        `json:"active_group,omitempty" yaml:"active_group,omitempty"`
    Groups      []EditorGroup `json:"groups,omitempty" yaml:"groups,omitempty"`
    Layout      *SplitNode    `json:"layout,omitempty" yaml:"layout,omitempty"`
    Items       []Tab         `json:"items" yaml:"items"`
}
```

### Frontend (TypeScript)

```typescript
interface SplitNode {
  type: 'leaf' | 'branch';
  groupId?: string;
  direction?: 'horizontal' | 'vertical';
  children?: SplitNode[];
  sizes?: number[];
}

interface EditorGroup {
  id: string;
  activeTabId: string | null;
}

// WorkspaceTab ganha groupId
interface WorkspaceTab {
  // ... campos existentes
  groupId?: string;
}

// WorkspaceData ganha campos de split
interface WorkspaceData {
  // ... campos existentes
  groups: EditorGroup[];
  activeGroupId: string | null;
  splitLayout: SplitNode | null;
}
```

## Backward Compatibility

Workspaces existentes sem `layout`/`groups` continuam funcionando:

- Se `layout` é `null`: grupo único implícito contendo todas as abas
- Na primeira operação de split: cria a estrutura explicitamente
- Migração é lazy — não reescreve YAML existente até que o usuário faça split

## APIs

### Backend (Go) — Métodos novos no Manager

| Método | Parâmetros | Retorno | Descrição |
|--------|-----------|---------|-----------|
| `SplitTab` | `tabId string, direction SplitDirection` | `*Workspace, error` | Cria novo grupo adjacente, move a aba para ele |
| `MoveTabToGroup` | `tabId string, groupId string` | `*Workspace, error` | Move aba entre grupos |
| `CloseGroup` | `groupId string` | `*Workspace, error` | Fecha grupo, redistribui abas para grupo adjacente |
| `SetActiveGroup` | `groupId string` | `*Workspace, error` | Define grupo ativo (foco) |
| `ResizeSplit` | `splitPath string, sizes []float64` | `error` | Atualiza proporções de um nó branch |

### Wails Bindings

Cada método acima é exposto via `app.go` como binding Wails, com evento correspondente:

| Evento | Payload | Quando |
|--------|---------|--------|
| `workspace:split_changed` | `Workspace` | Após split/close/resize |
| `workspace:group_activated` | `{ group_id: string }` | Após troca de grupo ativo |

## Componentes Frontend

### Novos

| Componente | Responsabilidade |
|------------|-----------------|
| `SplitContainer` | Renderiza a árvore recursivamente com flexbox |
| `SplitResizer` | Divisor arrastável entre painéis (mouse + teclado) |
| `GroupPanel` | Folha: mini tab-bar do grupo + toolbar do conteúdo + content area |
| `SplitDropZone` | Indicadores visuais nas bordas para drag-to-split |

### Alterados

| Componente | Mudança |
|------------|---------|
| `WorkspaceContent` | Renderiza `SplitContainer` em vez de conteúdo direto |
| `WorkspaceTabList` | Indicadores visuais de qual grupo cada aba pertence |
| `WorkspaceLayout` | Landmarks F6 adaptados para grupo ativo |

## Layout Visual

### Sem split (padrão, como hoje)

```
┌─────────────────────────────────────────┐
│ Topbar                                  │
│ Workspace Toolbar                       │
│ [💬 FSD-456] [📝 main.go] [>_ term]    │
├─────────────────────────────────────────┤
│                                         │
│          Conteúdo da aba ativa          │
│                                         │
└─────────────────────────────────────────┘
```

### Com split horizontal (2 grupos)

```
┌─────────────────────────────────────────┐
│ Topbar                                  │
│ Workspace Toolbar                       │
│ [💬 FSD-456] [📝 main.go] [>_ term]    │
├───────────────────┬─────────────────────┤
│ Grupo 1 (ativo)   │ Grupo 2             │
│ [💬 FSD-456]      │ [📝 main.go]       │
│ ─────────────     │ ─────────────       │
│                   │                     │
│ Chat ativo        │ Editor ativo        │
│                   │                     │
└───────────────────┴─────────────────────┘
```

### Com split misto (3 grupos)

```
┌─────────────────────────────────────────┐
│ Topbar                                  │
│ Workspace Toolbar                       │
│ [💬 FSD-456] [📝 main.go] [>_ term]    │
├───────────────────┬─────────────────────┤
│ Grupo 1           │ Grupo 2             │
│ [💬 FSD-456]      │ [📝 main.go]       │
│                   ├─────────────────────┤
│                   │ Grupo 3             │
│                   │ [>_ terminal]       │
└───────────────────┴─────────────────────┘
```

## Atalhos de Teclado

| Atalho | Ação |
|--------|------|
| `Ctrl+\` | Dividir aba ativa para a direita |
| `Ctrl+Shift+\` | Dividir aba ativa para baixo |
| `Alt+Shift+←` | Focar grupo à esquerda |
| `Alt+Shift+→` | Focar grupo à direita |
| `Alt+Shift+↑` | Focar grupo acima |
| `Alt+Shift+↓` | Focar grupo abaixo |

Atalhos existentes (`Ctrl+Tab`, `Ctrl+W`, `Ctrl+1..9`) operam sobre o **grupo ativo**.

## Drag & Drop

### Tab para borda do conteúdo

Arrastar uma aba para a borda esquerda/direita/superior/inferior da content area cria um novo split naquela direção. Drop zones visuais (faixas semi-transparentes) aparecem durante o arrasto.

### Tab entre grupos

Arrastar uma aba para a mini tab-bar de outro grupo move a aba para aquele grupo.

### Fechar grupo por arrasto

Quando a última aba de um grupo é arrastada para fora, o grupo é fechado e o espaço é redistribuído.

## Acessibilidade

### Princípio

O split é **puramente visual**. Para leitores de tela, o comportamento é **sempre 1 grupo ativo por vez**.

### Implementação

| Aspecto | Comportamento |
|---------|--------------|
| Grupo ativo | `aria-hidden="false"`, interativo |
| Grupos inativos | `aria-hidden="true"` + `inert` |
| Troca de grupo | Screen reader anuncia "Grupo N ativo" |
| F6 | Cicla landmarks dentro do grupo ativo (toolbar + content) |
| Tab trap | Tab navega apenas dentro do grupo ativo |

### Navegação entre grupos

`Alt+Shift+Setas` move o foco entre grupos. Ao entrar em um grupo:

1. O grupo anterior recebe `inert`
2. O novo grupo perde `inert`
3. Screen reader anuncia a troca
4. Foco vai para o último elemento focado naquele grupo

## Persistência

O layout de split é salvo no `workspace.yaml`:

```yaml
tabs:
  active: "tab-1"
  active_group: "group-1"
  groups:
    - id: "group-1"
      active_tab: "tab-1"
    - id: "group-2"
      active_tab: "tab-3"
  layout:
    type: branch
    direction: horizontal
    sizes: [0.5, 0.5]
    children:
      - type: leaf
        group_id: "group-1"
      - type: leaf
        group_id: "group-2"
  items:
    - id: "tab-1"
      type: chat
      content_id: "conv-abc"
      group_id: "group-1"
    - id: "tab-2"
      type: editor
      content_id: "file-main-go"
      group_id: "group-1"
    - id: "tab-3"
      type: terminal
      content_id: "term-def"
      group_id: "group-2"
```

## Resizer

O divisor entre painéis é interativo:

| Interação | Comportamento |
|-----------|--------------|
| Mouse drag | Redimensiona proporcionalmente |
| Duplo clique | Equaliza tamanhos (50/50) |
| Arrow keys (quando focado) | Ajuste fino em incrementos de 5% |
| Tamanho mínimo | 15% do total (evita painéis invisíveis) |

## Regras de comportamento

| Situação | Resultado |
|----------|----------|
| Split de aba única no grupo | Aba move para novo grupo; grupo original fica com nova aba de chat |
| Fechar última aba de um grupo | Grupo é removido, espaço redistribuído |
| Fechar todos os splits | Volta ao layout single (sem split) |
| `Ctrl+W` no grupo com 1 aba e 1 grupo total | Comportamento atual (cria nova aba de chat) |
| `Ctrl+W` no grupo com 1 aba e N grupos | Fecha o grupo |
| Mover aba para grupo que já tem mesma content_id | Rejeitado (regra: 1 aba por content_id por workspace) |
| Nova aba (`Ctrl+T`) | Criada no grupo ativo |

## Roadmap

### Fase 1 — Core

- Modelo de dados (backend Go + frontend TypeScript)
- SplitContainer, GroupPanel, SplitResizer
- API: SplitTab, MoveTabToGroup, CloseGroup, SetActiveGroup, ResizeSplit
- Menu de contexto da aba: "Dividir à direita", "Dividir abaixo"
- `Ctrl+\` e `Ctrl+Shift+\`
- Acessibilidade: `inert` em grupos inativos, anúncio de troca
- Persistência no workspace.yaml
- Backward compatibility (lazy migration)

### Fase 2 — Polish

- Drag-to-split com drop zones visuais
- Drag entre grupos (mover aba)
- `Alt+Shift+Setas` para navegar entre grupos
- Resize por teclado (setas quando resizer focado)
- Indicadores visuais de grupo na tablist principal
- Duplo clique no resizer para equalizar

## Arquivos impactados

### Backend (Go)

| Arquivo | Mudança |
|---------|---------|
| `internal/workspace/types.go` | Novos tipos: `SplitNode`, `SplitDirection`, `EditorGroup`; campo `GroupID` em `Tab`; novos campos em `TabsState` |
| `internal/workspace/manager.go` | Novos métodos: `SplitTab`, `MoveTabToGroup`, `CloseGroup`, `SetActiveGroup`, `ResizeSplit` |
| `app.go` | Novos bindings Wails para os métodos acima |

### Frontend

| Arquivo | Mudança |
|---------|---------|
| `frontend/src/store/workspaceStore.ts` | Novos tipos, ações, getters para grupos e splits |
| `frontend/src/components/workspace/SplitContainer.tsx` | **Novo** — renderiza árvore de splits |
| `frontend/src/components/workspace/SplitResizer.tsx` | **Novo** — divisor arrastável |
| `frontend/src/components/workspace/GroupPanel.tsx` | **Novo** — painel de grupo com mini tab-bar |
| `frontend/src/components/workspace/SplitDropZone.tsx` | **Novo** (Fase 2) — zonas de drop para drag-to-split |
| `frontend/src/components/workspace/WorkspaceContent.tsx` | Usar `SplitContainer` |
| `frontend/src/components/workspace/WorkspaceTabList.tsx` | Indicadores de grupo |
| `frontend/src/components/workspace/WorkspaceLayout.tsx` | Landmarks adaptados ao grupo ativo |
| `frontend/src/hooks/useWorkspaceKeyboardShortcuts.ts` | Novos atalhos: `Ctrl+\`, `Ctrl+Shift+\`, `Alt+Shift+Setas` |
