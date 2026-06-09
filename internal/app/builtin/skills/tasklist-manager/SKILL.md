---
name: tasklist-manager
version: 1.4.0
description: Provides context about task lists linked to the current conversation and instructions for managing tasks and workflows via tool calling
displayName: Task List Manager
author: Assistente
type: agent
category: productivity
difficulty: beginner
auto_load: true
autoload_reason: The active conversation's task lists and workflow context must be present from the start so the assistant can manage tasks accurately
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
- To create or update a task list (including custom workflows and custom actions), use `task_list`. Custom actions are passed via the `custom_actions` array (see the section below).
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

Each task list has an optional **`custom_actions`** field (AEP-0067) that adds custom items to the card context menu, the card detail screen, and/or the board menu. You can read, create, update and clear them yourself with the `task_list` tool — don't just describe them, configure them when the user asks.

- **Where they appear** — each action declares one or more *surfaces*: `card_menu` (card right-click menu, the default), `card_detail` (buttons on the task detail screen), `board_menu` (board-level menu, not tied to a card).
- **What an action does** — it can **publish a domain event** to the Job EventBus (so a job with `trigger.type: event` reacts to it) **and/or open a link** (an internal deep link `assistente://…` or an external `http(s)://` URL). At least one of `event`/`link` is required.
- **Per-action fields (these names are exact — unknown fields are rejected)** — use ONLY these keys:
  - `id` (string, required) — stable slug; no spaces/whitespace or path separators (`/` `\`); unique within the list.
  - `label` (string, required) — text shown on the item/button.
  - `icon` (string, optional) — emoji/icon.
  - `surfaces` (string array, optional) — any of `card_menu`, `card_detail`, `board_menu`; defaults to `["card_menu"]` when omitted.
  - `event` (string, optional) — domain event name to publish; no whitespace. Required unless `link` is set.
  - `payload_template` (**string**, optional) — a **Go template string** that must render to a JSON object; used as the event payload. Only applies together with `event`. It is NOT a JSON object literal.
  - `link` (string, optional) — Go template string that renders to a deep link (`assistente://…`) or `http(s)://` URL. Required unless `event` is set.
  - `when` (string, optional) — Go template string; the action only shows when it renders to a truthy value.
  - `confirm` (string, optional) — confirmation text shown before running.
  - `danger` (boolean, optional) — renders with a destructive style.
  - ⚠️ There are no `emits_event`, `enabled_when`, `condition`, `visible`, `enabled` or `version` fields — sending any unknown key fails validation.
- **Templating** — `link`, `payload_template` and `when` are Go templates with the card as root `.task` plus `.now`. Available `.task` fields: `task_id`, `id` (same as task_id), `task_list_id`, `task_list_slug`, `code`, `title`, `description`, `status_id`, `parent_id`, `assignee_id`, `assignee_name`, `creator_id`, `link`. There is no per-task `external_source`/`external_id`/`external_url` — for Jira-mirrored lists the issue key is typically in `.task.code` and the issue URL in `.task.link`. Use the `json` helper for safe values, e.g. `{{ "{{" }} json .task.code {{ "}}" }}`. (See the `job-manager` skill for full template syntax.)
- **Concrete example** (array passed as `custom_actions`):

```json
[
  {
    "id": "investigate",
    "label": "🔍 Investigar",
    "surfaces": ["card_menu", "card_detail"],
    "event": "task.investigate_requested",
    "payload_template": "{\"task_code\": {{ "{{" }} json .task.code {{ "}}" }}, \"external_url\": {{ "{{" }} json .task.link {{ "}}" }}}"
  },
  {
    "id": "open_in_jira",
    "label": "🔗 Abrir no Jira",
    "surfaces": ["card_menu", "card_detail"],
    "when": "{{ "{{" }} ne .task.link \"\" {{ "}}" }}",
    "link": "{{ "{{" }} .task.link {{ "}}" }}"
  }
]
```

- **How to manage them with the tool** — call `task_list` with `task_list_id` (or `task_list_slug`) and a `custom_actions` array. The array **replaces** the list's actions wholesale (send the full desired set, not a delta), so to read the current ones first call `task_list` with the list id — they're echoed back under `custom_actions`. Send `custom_actions: []` to remove all of them; omit the field to leave them unchanged. Invalid configs (unknown field, missing `id`/`label`, `id` with spaces, neither `event` nor `link`, `payload_template` without `event`, `payload_template` sent as an object instead of a template string, etc.) are rejected with an error message — fix and resend. The same actions can also be edited by the user in the **UI** (task list → Custom Actions editor); both paths write the same `custom_actions` JSON.
- **Refresh example** — the optional **"Atualizar"** (manual refresh) item is itself a board custom action that publishes `tasklist.list.refresh_requested`; a job can listen to it to re-sync the list. (See the `job-manager` skill for the full domain-event catalog and recipes.)
{{- end }}
