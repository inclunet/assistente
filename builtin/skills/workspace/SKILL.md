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

## Estado atual do workspace
{{- if .WorkspaceName }}

**Workspace:** {{ .WorkspaceName }}
{{- if .WorkspaceProfile }} | **Perfil:** {{ .WorkspaceProfile }}{{ end }}
{{- if .Tabs }}

| # | Aba | Tipo |
|---|-----|------|
{{- range $i, $tab := .Tabs }}
| {{ $i }} | {{ if $tab.IsActive }}▶ {{ end }}{{ $tab.Title }} | {{ $tab.Type }} |
{{- end }}
{{- end }}
{{- else }}
_Nenhum workspace ativo._
{{- end }}

## Fluxo recomendado

1. Para saber o que está aberto, consulte a tabela acima.
2. Para abrir ou navegar, construa a URI adequada e passe para `open_deep_link`.
3. Para ver ou gerenciar task lists, use `list_task_lists` e `get_task_list`.
4. Para abrir uma task list como aba, use `open_deep_link` com `assistente://tasklist/{id}`.

## Dicas

- O `open_deep_link` faz dedup automático: se a aba já existir, apenas ativa; se não, cria.
- Parâmetros como `message` e `title` devem ser URL-encoded.
