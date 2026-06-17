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
- When changing task status, use the stable status ID returned by the tools, not only the label.
- When removing workflow statuses, provide `status_migration` if existing tasks use those statuses.
- Use `task` with `task_id` and updated fields when a task changes.
- Use `task` with `task_list_id` and `title` when adding a task.
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

Each task list can define `custom_actions` for card menus, task detail buttons, or board menus.

- Each action needs a stable `id` and a visible `label`.
- Supported surfaces are `card_menu`, `card_detail`, and `board_menu`.
- An action may publish a domain event, open a link, or both.
- The `custom_actions` array replaces the current list of actions; read the list first if you need to preserve existing actions.
- Send `custom_actions: []` to remove all custom actions.
- Unknown fields are rejected. Do not send fields such as `emits_event`, `enabled_when`, `condition`, `visible`, `enabled`, or `version`.
- Template strings for `payload_template`, `link`, and `when` are part of task list configuration, not skill runtime templates. Treat them as data passed to the `task_list` tool.

## Good Workflow

1. Clarify the target task list or task if the user did not provide enough context.
2. Read current data with the appropriate tool.
3. Apply the smallest tool call that satisfies the request.
4. Summarize the result with IDs that the user may need next.
