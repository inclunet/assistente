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

Você controla a interface do aplicativo usando **deep links** via a ferramenta `open_deep_link`.

## Estado atual do workspace
{{- if .WorkspaceName }}

**Workspace:** {{ .WorkspaceName }}
{{- if .WorkspaceProfile }} | **Perfil:** {{ .WorkspaceProfile }}{{ end }}
{{- if .Tabs }}

Abas abertas (em ordem):
{{- range $i, $tab := .Tabs }}
{{ $i }}. {{ if $tab.IsActive }}▶ **{{ $tab.Title }}** ({{ $tab.Type }}, aba atual){{ else }}{{ $tab.Title }} ({{ $tab.Type }}){{ end }} → `{{ $tab.DeepLink }}`
{{- end }}
{{- end }}
{{- else }}
_Nenhum workspace ativo._
{{- end }}

## Deep Links

Para navegar ou criar recursos, construa URIs e passe para `open_deep_link`:

- **Nova conversa:** `assistente://conversation/new`
- **Nova conversa com mensagem:** `assistente://conversation/new?message=...&title=...`
- **Enviar mensagem em conversa:** `assistente://conversation/{id}/send?message=...`
- **Abrir task list:** `assistente://tasklist/{id}`
- **Abrir editor:** `assistente://editor/{id}`
- **Abrir terminal:** `assistente://terminal/{id}`
- **Navegar para página:** `assistente://navigate/{rota}` (rotas: `history`, `settings`, `profiles`, `skills`, `providers`, `credentials`, `mcp`, `channels`, `allowlists`, `help`, `about`)
- **Editar recurso:** `assistente://{recurso}/edit/{id}`
- **Criar recurso:** `assistente://{recurso}/new`

O `open_deep_link` faz dedup automático: se a aba já existir, apenas ativa.

## Diretrizes

- Ao responder sobre o workspace, use os nomes das abas e tipos — nunca exponha IDs ao usuário.
- Use os deep links da lista de abas acima para referenciar ou ativar abas existentes.
- Para task lists, use `list_task_lists` e `get_task_list` para consultar conteúdo.
