---
name: tasklist-manager
version: 2.0.0
description: Instructions for managing task lists, tasks, notes, workflows and conversation links through the task_list, task and task_note tools
displayName: Task List Manager
author: Assistente
type: agent
category: productivity
difficulty: beginner
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

# Task Lists — Operational Guide

Use this skill when the user asks to manage task lists, tasks, notes, workflows, custom actions, or links between a conversation and task work.

Task lists are persisted in the application database and managed through tools. Do not invent files or IDs.

## Tools

- `task_list`: create, read, update, delete and configure task lists, workflows and custom actions.
- `task`: create, read, update, delete, duplicate, move and link individual tasks.
- `task_note`: create and update task notes.
- `get_conversation_info`: read the current conversation ID and any linked task lists/tasks.

## Core Rules

- Read the current state with `task_list`, `task`, or `get_conversation_info` before changing existing records.
- Do not invent task IDs, list IDs, status IDs or workflow IDs.
- When the user asks about progress or status, use linked task data from context if present; otherwise read the latest state with tools.
- When a task is completed or its status changes, use `task` with `task_id` and updated fields.
- When adding a task, use `task` with `task_list_id` and `title`.
- Use `task_list` with `task_list_id` for latest list data if significant time has passed.
- Use `task` with `task_id` alone to read task details and notes.
- When changing task status, use the stable status ID returned by the tools, not only the label.
- When changing workflows, provide `status_migration` if removing statuses that have tasks.
- Status IDs are stable integers; always reference statuses by ID, not by label.
- `task` supports `assignee_name` and `assignee_id` to track who is currently working on a task.
- Set `assignee_name` to an empty string to clear the assignee. Omit the field entirely to preserve the current value.
- Assignee changes are automatically recorded as system notes for audit trail.
- Use `task_note` for task notes instead of embedding note history in descriptions.
- To delete a task, call `task` with `task_id` and `delete: true`.
- To duplicate a task, call `task` with `task_id` and `duplicate: true`.
- To move a task, call `task` with `task_id` and the target `task_list_id`.

## Conversation Links

A task or an entire task list can be linked to a conversation.

- Get the current conversation ID with `get_conversation_info` when the user asks to link work to this chat.
- Link a task by calling `task` with the task reference and `conversation_id`.
- Link a whole list by calling `task_list` with the list reference and `conversation_id`.
- Send an empty `conversation_id` to clear a link. Omit the field to preserve the current link.
- If linked lists are relevant but not shown in the prompt, read them through `get_conversation_info` or `task_list`.

## Custom Actions

Each task list has an optional `custom_actions` field (AEP-0067) that adds custom items to the card context menu, the card detail screen, and/or the board menu. You can read, create, update and clear them with the `task_list` tool. Configure them when the user asks; do not only describe them.

- **Where they appear**: each action declares one or more `surfaces`: `card_menu` (card right-click menu, the default), `card_detail` (buttons on the task detail screen), `board_menu` (board-level menu, not tied to a card).
- **What an action does**: it can publish a domain event to the Job EventBus so a job with `trigger.type: event` reacts to it, open a link, or both. Links can be internal deep links (`assistente://...`) or external `http(s)://` URLs. At least one of `event` or `link` is required.
- **Replacement semantics**: the `custom_actions` array replaces the list's actions wholesale. Read the current list first when you need to preserve existing actions. Send `custom_actions: []` to remove all actions. Omit the field to leave actions unchanged.
- **Unknown fields are rejected**. Do not send fields such as `emits_event`, `enabled_when`, `condition`, `visible`, `enabled`, or `version`.

### Exact Custom Action Fields

Use only these keys:

- `id` (string, required): stable slug; no whitespace or path separators (`/` or `\`); unique within the list.
- `label` (string, required): text shown on the item/button.
- `icon` (string, optional): emoji/icon.
- `surfaces` (string array, optional): any of `card_menu`, `card_detail`, `board_menu`; defaults to `["card_menu"]` when omitted.
- `event` (string, optional): domain event name to publish; no whitespace. Required unless `link` is set.
- `payload_template` (string, optional): Go template string that must render to a JSON object; used as the event payload. Only valid together with `event`. It is not a JSON object literal.
- `link` (string, optional): Go template string that renders to a deep link (`assistente://...`) or `http(s)://` URL. Required unless `event` is set.
- `when` (string, optional): Go template string; the action only shows when it renders to a truthy value.
- `confirm` (string, optional): confirmation text shown before running.
- `danger` (boolean, optional): renders with a destructive style.

### Custom Action Templating

The `link`, `payload_template`, and `when` fields are Go templates rendered with the card as root `.task` plus `.now`.

Available `.task` fields: `task_id`, `id` (same as `task_id`), `task_list_id`, `task_list_slug`, `code`, `title`, `description`, `status_id`, `parent_id`, `assignee_id`, `assignee_name`, `creator_id`, `link`.

There is no per-task `external_source`, `external_id`, or `external_url`. For Jira-mirrored lists, the issue key is typically in `.task.code` and the issue URL in `.task.link`.

Use the `json` helper for safe values in JSON payload templates, for example `{{ json .task.code }}`. See the `job-manager` skill for the full template syntax.

### Concrete Custom Actions Example

This is the array passed as `custom_actions`:

```json
[
  {
    "id": "investigate",
    "label": "Investigar",
    "surfaces": ["card_menu", "card_detail"],
    "event": "task.investigate_requested",
    "payload_template": "{\"task_code\": {{ json .task.code }}, \"external_url\": {{ json .task.link }}}"
  },
  {
    "id": "open_in_jira",
    "label": "Abrir no Jira",
    "surfaces": ["card_menu", "card_detail"],
    "when": "{{ ne .task.link \"\" }}",
    "link": "{{ .task.link }}"
  }
]
```

### Managing Custom Actions

- Call `task_list` with `task_list_id` or `task_list_slug` and a `custom_actions` array.
- The current actions are echoed back under `custom_actions` when reading a task list.
- Invalid configs are rejected with an error message. Fix and resend.
- The same actions can also be edited by the user in the UI (`Task list` → `Custom Actions` editor); both paths write the same `custom_actions` JSON.
- The optional "Atualizar" manual refresh item is itself a board custom action that publishes `tasklist.list.refresh_requested`; a job can listen to it to re-sync the list.

## Good Workflow

1. Clarify the target task list or task if the user did not provide enough context.
2. Read current data with the appropriate tool.
3. Apply the smallest tool call that satisfies the request.
4. Summarize the result with IDs that the user may need next.
