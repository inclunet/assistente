---
name: job-manager
version: 1.0.0
description: Provides context and instructions for managing event-driven automation jobs — creation, editing, triggers, conditional events, and pipelines
displayName: Job Manager
author: Assistente
type: agent
category: automation
difficulty: intermediate
auto_load: false
platforms:
  - windows
  - macos
  - linux
tools:
  allowed:
    - read_file
    - write_file
    - edit_file
    - list_directory
    - delete_file
filesystem:
  read:
    - "~/.assistente/jobs/**"
  write:
    - "~/.assistente/jobs/**"
behavior:
  interactive:
    confirmDestructive: true
    showProgress: false
output:
  format: markdown
---

# Job Manager — Event-Driven Automation

You manage the job automation system. Each **Job** is an atomic unit: 1 job = 1 tool call. Jobs chain via events, forming reactive pipelines.

## File Layout

```
~/.assistente/jobs/
  <job-id>.yaml          ← one file per job
  catalog.yaml           ← auto-generated tool catalog (read-only)
  runs/<job-id>/*.json   ← execution logs
  events/<date>.jsonl    ← event timeline
```

## YAML Schema

```yaml
id: my-job
name: "Human-readable name"
description: "What this job does"
enabled: true
pipeline: my-pipeline        # optional grouping

triggers:
  - type: cron
    expression: "0 9 * * 1-5"
  - type: event
    listen: "upstream.done"
    when: '{{ eq .event.type "relevant" }}'  # optional condition
  - type: manual

tool: mcp.some.tool
inputs:
  field: "fixed value"
  dynamic: "{{ .event.data }}"
  secret_val: "{{ secret \"API_KEY\" }}"

output:
  map:
    result: "{{ .output.data }}"

events:
  on_success: "my-job.done"
  on_failure: "my-job.failed"
  emit_when: '{{ ne .output.status "unchanged" }}'  # optional
  for_each: "items"              # fan-out (optional)
  payload_template: |            # reshape payload (optional)
    {
      "id": "{{ .output.id }}",
      "source": "{{ .event.origin }}"
    }
  payload_filter:                # whitelist/blacklist (optional)
    include: ["id", "status"]

error_policy:
  strategy: retry    # retry | stop | skip
  max_retries: 3
  retry_delay: 30s
  backoff: exponential
```

## Trigger Types

| Type | Fields | Description |
|------|--------|-------------|
| `cron` | `expression` | Cron schedule (e.g. `0 9 * * 1-5`) |
| `interval` | `every` | Periodic (e.g. `2h`, `30m`) |
| `event` | `listen`, `when` | React to named event; optional `when` condition |
| `hotkey` | `keys` | Keyboard shortcut (e.g. `Ctrl+Shift+J`) |
| `manual` | — | Always available via UI or chat |

### `when` (Trigger Condition)

Go template evaluated **before** the tool runs. If falsy, the job is silently skipped.

- Context: `{{ .event.* }}` (trigger payload), `{{ .now }}` (current time)
- Empty/omitted = always runs

```yaml
when: '{{ eq .event.webhookEvent "jira:issue_updated" }}'
```

## Event Emission

### `emit_when` (Output Condition)

Go template evaluated **after** the tool runs. If falsy, the success event is suppressed.

- Context: `{{ .output.* }}` (tool result), `{{ .event.* }}` (trigger payload), `{{ .now }}`
- For fan-out: evaluated **per item** — only matching items emit events
- Empty/omitted = always emits

```yaml
emit_when: '{{ eq .output.status "Done" }}'
```

### `when` + `emit_when` — Symmetric Pair

| Stage | Field | Saves |
|-------|-------|-------|
| Input | `trigger.when` | Avoids unnecessary tool calls |
| Output | `events.emit_when` | Avoids unnecessary event propagation |

## Template Functions

Available in `inputs`, `when`, `emit_when`, `payload_template`, `output.map`:

| Function | Usage | Description |
|----------|-------|-------------|
| `eq`, `ne`, `lt`, `gt` | `{{ eq .output.x "y" }}` | Comparison |
| `pluck` | `{{ pluck .output.items "key" }}` | Extract field from each item |
| `any` | `{{ any .output.items "status" "critical" }}` | Check if any item matches |
| `join` | `{{ join .output.tags ", " }}` | Join slice with separator |
| `json` | `{{ json .output.data }}` | Serialize to JSON string |
| `default` | `{{ default 50 .event.limit }}` | Fallback for nil/zero |
| `date` | `{{ date .now "2006-01-02" }}` | Format time |
| `secret` | `{{ secret "API_KEY" }}` | Resolve secret by name |

## How to Manage Jobs

| Action | How |
|--------|-----|
| List jobs | `list_directory ~/.assistente/jobs/` |
| View job | `read_file ~/.assistente/jobs/<id>.yaml` |
| Create job | `write_file ~/.assistente/jobs/<id>.yaml` with valid YAML |
| Edit job | `edit_file ~/.assistente/jobs/<id>.yaml` |
| Delete job | `delete_file ~/.assistente/jobs/<id>.yaml` |
| Enable/disable | `edit_file` → `enabled: true/false` |
| View logs | `read_file ~/.assistente/jobs/runs/<id>/<timestamp>.json` |
| View events | `read_file ~/.assistente/jobs/events/<date>.jsonl` |

The app watches the jobs folder — any change is validated and takes effect immediately (hot reload). Invalid YAML is ignored and logged.

## Guidelines

- Always set a meaningful `id` (no spaces or path separators).
- Every job needs at least one trigger and a `tool`.
- Use `{{ .event.* }}` in inputs to pass data from upstream jobs.
- Use `{{ secret "KEY" }}` for credentials — never hardcode secrets.
- Use `pipeline` to visually group related jobs.
- When creating pipelines, define clear event names (e.g. `tickets.fetched`, `report.generated`).
- Use `when` on triggers to avoid wasteful tool calls.
- Use `emit_when` on events to avoid noisy downstream propagation.
- For fan-out, set `for_each` to the output array path and optionally `emit_when` to filter items.
- Test with `dry_run.enabled: true` and `mock_output` before enabling real execution.
