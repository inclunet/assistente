# AEP-0034: Unified Workspace

- **Status**: Done
- **Autor**: Leonardo Gleison Ferreira
- **Data**: 2026-03-20

## Resumo

Unificação da experiência do Assistente em torno do conceito de **Workspace** — um container de abas mistas (chat, editor, terminal, tasklist) que substitui a navegação fragmentada entre mini apps separados. Abas de qualquer tipo coexistem na mesma barra, eliminando troca de contexto.

## Motivação

Hoje o Assistente tem vários mini apps (chat, editor, terminal, e o futuro tasklist), cada um com seu próprio sistema de abas e navegação. O usuário precisa ir para o editor, voltar pro terminal, ir pra tasklist — fragmentando o contexto de trabalho.

O padrão de abas se repete em todos eles: são sessões paralelas dentro de um container. Mas vivem isolados, sem relação entre si.

### Problemas

- **Fragmentação de contexto**: trocar de app = trocar de contexto mental
- **Navegação ineficiente**: ir e voltar entre áreas diferentes
- **Abas duplicadas**: cada app tem seu sistema de abas independente
- **Sem relação**: chat não sabe que existe um terminal ou editor relacionado
- **Sem agrupamento**: não há como organizar abas por projeto ou atividade

## Filosofia

```
Mini apps separados   →  Tipos de aba dentro de um workspace
Navegação entre apps  →  Navegação entre abas (Ctrl+Tab)
Contexto fragmentado  →  Contexto unificado
Cada app com suas abas →  Uma barra de abas com tipos mistos
```

## Modelo Core

### Três camadas

```
Conteúdo (entidade persistente)
  │  Conversa, arquivo, terminal session, tasklist
  │  Existe independente, tem ID único, persiste (DB ou disco)
  │
Aba (view de um conteúdo dentro de um workspace)
  │  Pertence a UM workspace (morre com ele)
  │  Aponta para UM conteúdo
  │  Tem posição, ordem, estado visual
  │
Workspace (container de abas)
     Criado pelo usuário, tem nome e ID
     Contém N abas (exclusivas dele)
     1 workspace ativo por janela do app
```

### Regras fundamentais

| Regra | Detalhe |
|-------|---------|
| Mesmo conteúdo em workspaces diferentes | ✅ Cada workspace tem sua própria aba apontando pro conteúdo |
| Mesmo conteúdo 2x no mesmo workspace | ❌ No máximo 1 aba por conteúdo por workspace |
| Deletar workspace | Abas morrem, conteúdo sobrevive |
| Deletar conteúdo | Abas que apontam pra ele em qualquer workspace são removidas |
| 1 janela do app | 1 workspace ativo, workspace picker pra alternar |
| Múltiplas janelas | Cada janela pode ter um workspace diferente |
| Reabrir app | Abre no último workspace usado |
| Workspace novo | Sempre abre com 1 aba de chat vazia |
| Limite de abas | Ilimitado |
| Limite de workspaces | Ilimitado |
| Abas órfãs | Permitidas — qualquer aba pode existir sem depender de outra |

### Diagrama

```
┌─ Workspace "On-call" (janela 1) ──────────────────────┐
│ [💬 FSD-456] [💬 FSD-789] [✅ Tickets] [>_ terminal]  │
│      │            │            │              │        │
│      ▼            ▼            ▼              ▼        │
│   conv-abc    conv-def    tasklist-01    term-sess-1   │
└────────────────────────────────────────────────────────┘
                    │
                    │ mesmo conteúdo (conv-def)
                    │
┌─ Workspace "Review" (janela 2) ───────────────────┐
│ [💬 FSD-789] [📝 notas.md]                         │
│      │            │                                │
│      ▼            ▼                                │
│   conv-def    file-notas                           │
└────────────────────────────────────────────────────┘
```

## Tipos de Aba

| Tipo | Ícone | Conteúdo | Toolbar específica |
|------|-------|----------|-------------------|
| Chat | 💬 | Conversa com LLM | Modelo, perfil, título, limpar, exportar |
| Editor | 📝 | Edição de arquivo | Arquivo, linguagem, salvar, encoding |
| Terminal | >_ | Shell interativo | Shell, diretório atual, limpar |
| Tasklist | ✅ | Lista de tarefas | Filtros, ordenação, fonte (local/jira) |

### Chat modal por aba

Editor, terminal e tasklist podem abrir um **chat modal** vinculado à própria aba do workspace. Esse modal:

- Usa a **mesma conversa persistida por aba** usada por uma aba de chat dedicada
- Usa o **mesmo pipeline de envio** do chat principal, sempre endereçado por `conversationId`
- Só adiciona capacidades contextuais da superfície ativa (ex.: contexto do editor, histórico do terminal, resumo da tasklist)
- Não cria um produto paralelo; é apenas outra superfície para a mesma conversa

Se a aba ainda não tiver conversa, o `conversationId` é criado sob demanda e persistido na configuração da aba do workspace antes do primeiro envio.

## Split View e Acessibilidade

O workspace suporta split view — múltiplas abas visíveis simultaneamente na tela.

Porém, para acessibilidade (leitor de tela), o comportamento é **sempre 1 aba por vez**. O split é puramente visual e não altera a semântica de navegação.

## Layout da Janela

```
┌─────────────────────────────────────────────────────────────┐
│ Workspace Toolbar                                           │
│ [📂 On-call ▾] [+ ▾]                      [🔍] [⚙️]       │
├─────────────────────────────────────────────────────────────┤
│ Tablist                                                     │
│ [💬 FSD-456] [💬 FSD-789] [✅ Tickets] [>_ terminal]       │
├─────────────────────────────────────────────────────────────┤
│ Content Panel Toolbar                                       │
│ (específica do tipo de aba ativa)                           │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ Content Area                                                │
│ (conteúdo da aba ativa)                                     │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│ Status Bar (futuro)                                         │
│ Jobs rodando, conexões MCP, notificações                    │
└─────────────────────────────────────────────────────────────┘
```

### Camadas

| Camada | Responsabilidade | Sempre visível |
|--------|-----------------|----------------|
| **Workspace Toolbar** | Identidade do workspace, criar/trocar, ações globais | Sim |
| **Tablist** | Abas do workspace, navegação, criar/fechar/mover | Sim |
| **Content Panel Toolbar** | Ações específicas do tipo de conteúdo ativo | Sim (muda conforme a aba) |
| **Content Area** | O conteúdo em si | Sim |
| **Status Bar** | Info contextual, jobs, status de conexões | Futuro |

### Landmark Navigation (F6)

F6 cicla entre as landmarks em ordem circular. Shift+F6 faz o ciclo inverso.

```
F6 →  Workspace Toolbar
F6 →  Tablist
F6 →  Content Panel Toolbar
F6 →  Content Area
F6 →  Status Bar (quando existir)
F6 →  Workspace Toolbar  ← volta ao início
```

Cada landmark é uma ARIA landmark region. Dentro de cada região, Tab e setas navegam os elementos internos.

## Atalhos de Teclado

### Workspace

| Atalho | Ação |
|--------|------|
| **Ctrl+Shift+N** | Novo workspace |

### Abas

| Atalho | Ação |
|--------|------|
| **Ctrl+T** | Nova aba (chat por padrão) |
| **Ctrl+N** | Menu "Criar..." (chat, editor, terminal, tasklist) |
| **Ctrl+W** | Fecha aba ativa |
| **Ctrl+F4** | Fecha aba ativa (alternativo) |
| **Ctrl+Tab** | Próxima aba |
| **Ctrl+Shift+Tab** | Aba anterior |
| **Ctrl+PageDown** | Próxima aba |
| **Ctrl+PageUp** | Aba anterior |
| **Ctrl+1..9** | Vai direto pra aba N |

Observação: quando um **chat modal** estiver aberto, a troca de abas fica bloqueada até ele ser fechado. Isso evita que o modal continue apontando para uma superfície diferente da aba original.

### Movimentação (Alt+Setas — convenção interna)

| Atalho | Ação |
|--------|------|
| **Alt+←** | Move aba pra esquerda (com foco na aba) |
| **Alt+→** | Move aba pra direita (com foco na aba) |
| **Alt+↑** | Move aba pra esquerda (alias — tablist é single row) |
| **Alt+↓** | Move aba pra direita (alias — tablist é single row) |

Alt+Setas é contextual — o foco deve estar no elemento (aba) para movimentá-lo. Dentro de outros componentes (editor, tasklist), Alt+Setas move o elemento focado naquele contexto.

### Menu Ctrl+N

```
Criar nova aba:
  💬 Chat              Ctrl+N, C
  📝 Editor            Ctrl+N, E
  ✅ Tasklist           Ctrl+N, T
  >_ Terminal          Ctrl+N, R
```

## Workspace Toolbar

```
┌─────────────────────────────────────────────────────────┐
│ [📂 On-call ▾]  [+ ▾]                  [🔍] [⚙️]      │
│  workspace picker  new tab               busca  config  │
└─────────────────────────────────────────────────────────┘
```

### Workspace Picker

```
Workspaces:
  ● On-call                   ~/dev/assistente
    Dev Assistente             ~/dev/assistente
    Pessoal                    (avulso)
  ──────────────
  🔍 Buscar workspace...
  ──────────────
  + Novo workspace            Ctrl+Shift+N
  📁 Abrir pasta como workspace...
```

## Páginas de Listagem

Cada tipo de conteúdo que precisa de gerenciamento tem uma **página de listagem** acessível como área default ou via navegação.

### Comportamento

| Ação | Resultado |
|------|----------|
| Clica no item da lista | Abre **modal** com o detalhe |
| Menu de contexto → "Enviar ao workspace" | Cria aba no workspace ativo com esse conteúdo |
| Menu de contexto → "Renomear" | Renomeia o conteúdo |
| Menu de contexto → "Deletar" | Deleta o conteúdo (remove abas em todos os workspaces) |

### Listagens

| Página | Descrição |
|--------|-----------|
| `ChatHistory` | Histórico de conversas — busca, listagem, acesso |
| `TasklistLibrary` | Acervo de tasklists — listagem, criar, gerenciar |

Editor e Terminal **não têm listagem** — são conteúdo efêmero ou abrem direto no workspace.

### Componentes reutilizáveis

O componente de detalhe é o mesmo tanto no modal da listagem quanto na aba do workspace:

| Componente | Usado em |
|------------|----------|
| `ChatView` | Modal da ChatHistory + aba no workspace |
| `EditorView` | Aba no workspace |
| `TerminalView` | Aba no workspace |
| `TasklistView` | Modal da TasklistLibrary + aba no workspace |

## Persistência

### Decisões

| O quê | Onde | Formato |
|-------|------|---------|
| Estado do workspace + abas | `.assistente/workspace.yaml` | Arquivo YAML |
| Conversas | DB (como hoje) | SQLite/IndexedDB |
| Índice global de workspaces | `~/.assistente/workspaces/index.yaml` | Arquivo YAML |

### Localização do workspace

| Tipo | Caminho |
|------|---------|
| Vinculado a pasta/projeto | `<pasta>/.assistente/workspace.yaml` |
| Avulso (criado na UI) | `~/.assistente/workspaces/<id>/.assistente/workspace.yaml` |
| Default (sem diretório) | `~/.assistente/workspaces/default/.assistente/workspace.yaml` |

### Resolução ao abrir o app

```
App abre em <path>
  │
  ├── <path>/.assistente/workspace.yaml existe?
  │     └── Sim → carrega esse workspace
  │
  ├── Não → é um diretório válido?
  │     └── Sim → cria workspace novo ali
  │
  └── Sem path (abriu o app direto)?
        └── Carrega último usado (via index.yaml)
              └── Se não tem histórico → default
```

### Identificação

```yaml
# .assistente/workspace.yaml
id: "ws-a1b2c3"          # ID único (nunca muda)
name: "On-call"           # display name (renomeável livremente)
```

Pasta de workspaces avulsos usa **ID**, não nome. O nome é só metadata dentro do YAML.

### index.yaml

```yaml
# ~/.assistente/workspaces/index.yaml
last_opened: "ws-d4e5f6"
workspaces:
  - id: "ws-a1b2c3"
    name: "On-call"
    path: "~/.assistente/workspaces/ws-a1b2c3"
    last_used: "2026-03-20T14:30:00Z"
  - id: "ws-d4e5f6"
    name: "Dev Assistente"
    path: "~/dev/assistente"
    last_used: "2026-03-20T10:00:00Z"
```

### workspace.yaml

```yaml
id: "ws-d4e5f6"
name: "Dev Assistente"
profile: programacao
created_at: "2026-03-20T10:00:00Z"
last_used: "2026-03-20T14:30:00Z"

tabs:
  active: "tab-3"
  items:
    - id: "tab-1"
      type: chat
      content_id: "conv-abc123"
      title: "Feature Workspace"
      position: 0
      profile_override:
        model: claude-sonnet-4

    - id: "tab-2"
      type: terminal
      content_id: "term-def456"
      title: "build"
      position: 1

    - id: "tab-3"
      type: editor
      content_id: "file-main-go"
      title: "main.go"
      position: 2
      state:
        scroll: 142
        cursor: { line: 42, col: 8 }

    - id: "tab-4"
      type: tasklist
      content_id: "tasklist-fsd"
      title: "Tickets FSD"
      position: 3
```

### Mudança de localização

| Ação | O que acontece |
|------|---------------|
| Vincular workspace avulso a uma pasta | Move `.assistente/workspace.yaml` pra nova pasta, deleta pasta avulsa |
| Desvincular de pasta | Move de volta pra `~/.assistente/workspaces/<id>/` |
| Renomear workspace | Só muda `name` no YAML, pasta não muda |

## Perfis

### Cascata

```
Workspace Profile (base)
  └── Tab Override (opcional, por aba)
```

O workspace define um perfil base (modelo LLM, skills, comportamento). Cada aba pode sobrescrever campos específicos.

**Exemplo:**
- Workspace "Dev" → perfil `programacao`, modelo `claude-sonnet-4`
- Aba "Chat rápido" → override: modelo `claude-haiku`

## Notificações

| Situação | Comportamento |
|----------|--------------|
| Atualização em workspace inativo | Badge no workspace picker (número de atualizações) |
| Ao abrir o workspace | Badge na aba específica que teve atualização |

## Integração com Jobs (AEP-0001)

### Status Bar (futuro)

```
🔄 2 jobs ativos  │  🟢 MCP: 4 connected  │  📡 Telegram: ok  │  ⚠️ 3 notificações
```

### Fluxo integrado

Com Jobs + Tasklist + Workspace, o fluxo de trabalho fica rastreável:

1. **Job busca tickets** no Jira → atualiza tasklist
2. **Tasklist** mostra tickets como abas potenciais
3. Usuário abre **aba de chat** pra investigar ticket específico
4. Chat cria **aba de terminal** se precisar rodar comandos
5. Resultado da investigação pode **voltar pro Jira** via Job

O contexto é rastreável de ponta a ponta — ticket → tasklist → conversa → ação.

## Roadmap

### v1 — Core Workspace

- Modelo workspace → abas → conteúdo
- Tipos de aba: chat, editor, terminal, tasklist
- Workspace Toolbar + Tablist + Content Panel Toolbar + Content Area
- Landmark navigation (F6)
- Todos os atalhos de teclado
- Persistência em workspace.yaml
- Índice global (index.yaml)
- Workspace picker (lista, busca)
- Perfil por workspace com override por aba
- Páginas de listagem (ChatHistory, TasklistLibrary) com modal
- "Enviar ao workspace" das listagens
- ~~Split view (visual) com acessibilidade (1 aba por vez)~~ → Extraído para **AEP-0035**
- Restauração completa ao reabrir o app
- Mover abas entre workspaces (drag & drop / menu)
- Exportar/importar workspace (estrutura, sem conteúdo)

### v2 — Expansão

- Status Bar (jobs, MCP, notificações)
- Busca cross-workspace
- Templates de workspace (on-call, dev, etc.)
- Vinculação automática conversa ↔ ticket
- Dashboard de workspaces (visão geral de todos)
