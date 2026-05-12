---
name: tasklist-manager
version: 1.1.0
description: Provides context about task lists linked to the current conversation and instructions for managing tasks and workflows via tool calling
displayName: Task List Manager
author: Assistente
type: agent
category: productivity
difficulty: beginner
auto_load: true
platforms:
  - windows
  - macos
  - linux
behavior:
  interactive:
    confirmDestructive: false
    showProgress: false
output:
  format: markdown
---
{{- if .HasTaskLists }}

# Task Lists — Conversation Context

This conversation has linked task lists. Use this context to track progress, update tasks, and help the user manage their work.

## Linked Task Lists
{{- range .TaskLists }}

### {{ .Title }} (ID: {{ .ID }})
{{- if .Description }}
{{ .Description }}
{{- end }}
{{- if .Tasks }}

| # | Status | Task | ID |
|---|--------|------|----|
{{- range $i, $t := .Tasks }}
| {{ $i }} | {{ $t.StatusIcon }} {{ $t.Status }} | {{ $t.Title }} | {{ $t.ID }} |
{{- end }}
{{- else }}
_No tasks yet._
{{- end }}
{{- end }}

## Guidelines

- When the user asks about progress or status, refer to the task data above.
- When a task is completed or its status changes during the conversation, use `task` with `task_id` + updated fields.
- When the user asks to add a new task, use `task` with `task_list_id` + `title`.
- Use `task_list` with `task_list_id` for the latest data if significant time has passed. Use `task` with `task_id` alone to read task details + notes.
- Do NOT invent task IDs — always use the IDs shown above or retrieved via tools.
- To create or update a task list (including custom workflows), use `task_list`.
- When changing workflows, provide `status_migration` if removing statuses that have tasks.
- Status IDs are stable integers — always reference statuses by ID, not by label.
- `task` supports `assignee_name` and `assignee_id` to track who is currently working on a task.
- Set `assignee_name` to empty string to clear the assignee. Omit the field entirely to preserve the current value.
- Assignee changes are automatically recorded as system notes for audit trail.
- To delete a task, use `task` with `task_id` + `delete: true`.
- To duplicate a task, use `task` with `task_id` + `duplicate: true`.
- To move a task, use `task` with `task_id` + different `task_list_id`.
{{- if .ToolCallingEnabled }}
- Tools available: `task_list` (CRUD for task lists), `task` (CRUD for tasks), `task_note` (create/update notes).
{{- end }}
{{- end }}
