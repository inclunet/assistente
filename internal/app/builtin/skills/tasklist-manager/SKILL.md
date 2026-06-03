---
name: tasklist-manager
version: 1.2.0
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

## Custom actions (custom context-menu items & buttons)

Each task list has an optional **`custom_actions`** field (AEP-0067) that lets the user add their own items to the card context menu, the card detail screen, and/or the board menu. Use this to answer questions about it and to guide the user.

- **Where they appear** — each action declares one or more *surfaces*: `card_menu` (card right-click menu, the default), `card_detail` (buttons on the task detail screen), `board_menu` (board-level menu, not tied to a card).
- **What an action does** — it can **publish a domain event** to the Job EventBus (so a job with `trigger.type: event` reacts to it) **and/or open a link** (an internal deep link `assistente://…` or an external `http(s)://` URL). At least one of `event`/`link` is required.
- **Per-action fields**: `id` (stable slug), `label`, `icon` (optional emoji), `surfaces`, `event` (optional), `payload_template` (optional Go template → JSON object; only valid together with `event`), `link` (optional Go template → deep link/URL), `when` (optional Go template; the action only shows when it renders truthy), `confirm` (optional confirmation text), `danger` (styles it as destructive).
- **Templating** — the `link`, `payload_template` and `when` fields are Go templates rendered with the card as root `.task` (fields like `.task.code`, `.task.title`, `.task.link`, `.task.task_list_id`, `.task.task_list_slug`) plus `.now`. For the exact template syntax and the safe `json` helper, see the `job-manager` skill.
- **How it's configured** — custom actions are edited in the **UI** (task list → Custom Actions editor), which persists the `custom_actions` JSON. The agent tools (`task_list`/`task`) do **not** create or edit custom actions, so when asked to set one up, walk the user through the editor instead of attempting a tool call.
- **Common example** — the optional **"Atualizar"** (manual refresh) item is itself a board custom action that publishes `tasklist.list.refresh_requested`; a job can listen to it to re-sync the list. (See the `job-manager` skill for the full domain-event catalog and recipes.)
{{- end }}
