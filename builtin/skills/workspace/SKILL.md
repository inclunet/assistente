---
name: workspace
version: 2.0.0
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

Ao responder sobre o workspace, use títulos e tipos. Exemplo:

> Você tem 3 abas:
> 1. **Debugging** (conversa, aba atual)
> 2. Sprint Tasks (lista de tarefas)
> 3. Nova conversa

Use `open_deep_link` para navegar. Deep links das abas estão acima. Outros:
- Nova conversa: `assistente://conversation/new` (aceita `?message=...&title=...`)
- Páginas: `assistente://navigate/{rota}` (rotas: `history`, `settings`, `profiles`, `providers`, `credentials`, `skills`, `mcp`, `channels`, `allowlists`, `help`, `about`)
- Criar/editar recurso: `assistente://{recurso}/new`, `assistente://{recurso}/edit/{id}`
