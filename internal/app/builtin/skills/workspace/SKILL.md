---
name: workspace
version: 2.1.0
description: "Gerenciamento de workspace: orienta o assistente sobre como navegar entre abas, abrir recursos e usar deep links para controlar a interface"
displayName: Workspace Manager
author: Assistente
type: agent
category: productivity
difficulty: beginner
auto_load: true
autoload_reason: Workspace navigation and deep-link controls are needed throughout the session so the assistant can drive the UI at any moment
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

# Workspace
{{- if .WorkspaceName }}

**{{ .WorkspaceName }}**{{ if .WorkspaceProfile }} (perfil: {{ .WorkspaceProfile }}){{ end }}
{{- if .Tabs }}

Abas:
{{- range $i, $tab := .Tabs }}
{{ $i }}. {{ if $tab.IsActive }}▶ {{ end }}{{ $tab.Title }} ({{ $tab.Type }}) → `assistente://{{ if eq $tab.Type "chat" }}conversation{{ else }}{{ $tab.Type }}{{ end }}/{{ $tab.ContentID }}`
{{- end }}
{{- end }}
{{- else }}
_Nenhum workspace ativo._
{{- end }}

{{- if .Surface }}

Superfície atual:
- tipo: `{{ .Surface.Type }}`
{{- if .Surface.Title }}
- título: `{{ .Surface.Title }}`
{{- end }}
{{- if and .Surface.State (index .Surface.State "filePath") }}
- arquivo ativo: `{{ index .Surface.State "filePath" }}`
{{- end }}
{{- if and .Surface.State (index .Surface.State "sessionId") }}
- sessão de terminal: `{{ index .Surface.State "sessionId" }}`
{{- end }}
{{- if and .Surface.State (index .Surface.State "tasklistId") }}
- tasklist ativa: `{{ index .Surface.State "tasklistId" }}`
{{- end }}
{{- if and .Surface.Context (index .Surface.Context "selectedText") }}
- seleção atual:

```text
{{ index .Surface.Context "selectedText" }}
```
{{- end }}
{{- if and .Surface.Context (index .Surface.Context "historyPreview") }}
- histórico recente do terminal:

```text
{{ index .Surface.Context "historyPreview" }}
```
{{- end }}
{{- if and .Surface.Context (index .Surface.Context "tasksPreview") }}
- resumo atual da tasklist:

```text
{{ index .Surface.Context "tasksPreview" }}
```
{{- end }}
{{- end }}

Ao responder sobre o workspace, use títulos e tipos. Exemplo:

> Você tem 3 abas:
> 1. **Debugging** (conversa, aba atual)
> 2. Sprint Tasks (lista de tarefas)
> 3. Nova conversa

Use `open_deep_link` para navegar. Deep links das abas ativas estão listados acima. Referência completa:

**Conversas:**
- `assistente://conversation/{id}` — abrir conversa existente
- `assistente://conversation/new` — nova conversa (aceita `?message=...&title=...`)
- `assistente://conversation/{id}/send?message=...` — enviar mensagem em conversa existente

**Criar abas:**
- `assistente://tasklist/new` — nova lista de tarefas (aceita `?title=...`)
- `assistente://editor/new` — novo documento vazio (aceita `?title=...`)
- `assistente://editor/open?file=caminho` — abrir arquivo no editor (aceita `&title=...`)
- `assistente://terminal/new` — novo terminal (aceita `?cmd=...` para executar comando, `&title=...`)

**Páginas:** `assistente://navigate/{rota}`
- Rotas: `history`, `tasklists`, `settings`, `profiles`, `providers`, `credentials`, `skills`, `mcp`, `channels`, `allowlists`, `help`, `about`, `update`

**Criar/editar recurso:** `assistente://{recurso}/new`, `assistente://{recurso}/edit/{id}`
- Recursos: `profiles`, `providers`, `credentials`, `allowlists`, `skills`, `mcp`, `channels`, `tasklists`
