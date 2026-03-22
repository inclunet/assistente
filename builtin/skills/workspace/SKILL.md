---
name: workspace
version: 1.0.0
description: "Gerenciamento de workspace: orienta o assistente sobre como navegar entre abas, abrir recursos e usar deep links para controlar a interface"
displayName: Workspace Manager
author: Assistente
type: agent
category: productivity
difficulty: beginner
auto_load: true
platforms:
  - windows
  - macos
  - linux
tools:
  allowed:
    - open_deep_link
    - read_file
    - list_directory
    - list_task_lists
    - get_task_list
behavior:
  interactive:
    confirmDestructive: false
    showProgress: false
output:
  format: markdown
---

# Workspace — Gerenciamento de Abas e Navegação

Você pode controlar a interface do aplicativo usando **deep links**. Use a ferramenta `open_deep_link` para executar qualquer URI abaixo.

## Deep Links disponíveis

### Conversas
| Ação | URI | Descrição |
|------|-----|-----------|
| Abrir conversa | `assistente://conversation/{id}` | Abre ou ativa uma aba de conversa existente |
| Nova conversa | `assistente://conversation/new` | Cria uma nova aba de conversa |
| Nova conversa com mensagem | `assistente://conversation/new?message=...&title=...` | Cria conversa e envia mensagem |
| Enviar mensagem | `assistente://conversation/{id}/send?message=...` | Envia mensagem em conversa existente |

### Abas de recursos
| Ação | URI | Descrição |
|------|-----|-----------|
| Abrir task list | `assistente://tasklist/{id}` | Abre ou ativa uma aba de task list |
| Abrir editor | `assistente://editor/{id}` | Abre ou ativa uma aba do editor |
| Abrir terminal | `assistente://terminal/{id}` | Abre ou ativa uma aba de terminal |

### Navegação
| Ação | URI | Descrição |
|------|-----|-----------|
| Navegar para página | `assistente://navigate/{rota}` | Rotas: `history`, `editor`, `terminal`, `settings`, `profiles`, `skills`, `providers`, `credentials`, `mcp`, `channels`, `allowlists`, `help`, `about` |
| Página inicial (chat) | `assistente://navigate/` | Volta para a tela de chat |

### Recursos (editar/criar)
| Ação | URI | Descrição |
|------|-----|-----------|
| Editar recurso | `assistente://{recurso}/edit/{id}` | Abre formulário de edição. Recursos: `profiles`, `providers`, `credentials`, `allowlists`, `skills`, `mcp`, `channels` |
| Criar recurso | `assistente://{recurso}/new` | Abre formulário de criação |

## Estado do workspace

O workspace é persistido em `.assistente/workspace.yaml` no diretório de trabalho. Use `read_file` para ler o estado atual e descobrir quais abas estão abertas:

```yaml
# Exemplo de workspace.yaml
id: ws-abc123
name: Meu Projeto
tabs:
  active: tab-xyz
  items:
    - id: tab-xyz
      type: chat
      content_id: "42"
      title: Debugging
      position: 0
    - id: tab-abc
      type: tasklist
      content_id: "5"
      title: Sprint Tasks
      position: 1
```

## Fluxo recomendado

1. Para saber quais abas estão abertas, use `read_file` no `.assistente/workspace.yaml`.
2. Para abrir ou navegar, construa a URI adequada e passe para `open_deep_link`.
3. Para ver ou gerenciar task lists, use `list_task_lists` e `get_task_list`.
4. Para abrir uma task list como aba, use `open_deep_link` com `assistente://tasklist/{id}`.

## Dicas

- O `open_deep_link` faz dedup automático: se a aba já existir, apenas ativa; se não, cria.
- Parâmetros como `message` e `title` devem ser URL-encoded.
- IDs de task lists são numéricos. IDs de conversas são numéricos. IDs de abas são strings geradas.
