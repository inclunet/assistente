---
name: job-manager
version: 2.0.0
description: Provides context and instructions for managing event-driven automation jobs and pipelines via the `job` and `job_pipeline` tools (DB-backed) — creation, editing, triggers, conditional events, runs and events inspection
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
behavior:
  interactive:
    confirmDestructive: true
    showProgress: false
output:
  format: markdown
---

# Job Manager — Event-Driven Automation

You manage the job automation system. Each **Job** is an atomic unit: 1 job = 1 tool call. Jobs chain via events, forming reactive pipelines.

Jobs and pipelines are persisted in the application database and managed **exclusively through the `job` and `job_pipeline` tools**. There are no YAML files on disk to read or write — every operation (list, read, create, update, delete, run, dry-run, inspect runs/events) goes through tool calls.

## Available Tools

| Tool | Purpose |
|------|---------|
| `job` | CRUD + execution + inspection for individual jobs |
| `job_pipeline` | CRUD + enable/disable for pipelines (groups of jobs) |

Both tools are composite: the same call shape switches between actions based on which fields are provided.

## Job Schema (object passed to the `job` tool)

The shape below is the **payload sent as tool arguments**, not a file. YAML is used only to illustrate the structure — you can pass the same shape as JSON.

```yaml
job_id: my-job                       # optional on create (slug auto-generated from name)
name: "Human-readable name"          # required on create
description: "What this job does"
enabled: true
pipeline: my-pipeline                # optional grouping; pipeline is auto-created on first use if the slug does not exist
tags: ["reports", "jira"]

triggers:                            # required on create (≥ 1)
  - type: cron
    expression: "0 9 * * 1-5"
  - type: event
    listen: "upstream.done"
    when: '{{ eq .event.type "relevant" }}'   # optional condition
  - type: manual

tool: mcp_jira__search_issues        # required on create — must exist in tool catalog. MCP tools are namespaced as `mcp_<serverSlug>__<toolName>`.
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
  emit_when: '{{ ne .output.status "unchanged" }}'   # optional
  for_each: "items"                                    # fan-out (optional)
  payload_template: |                                  # reshape payload (optional)
    {
      "id": "{{ .output.id }}",
      "source": "{{ .event.origin }}"
    }
  payload_filter:                                      # whitelist/blacklist (optional)
    include: ["id", "status"]

error_policy:
  strategy: retry                    # retry | stop | skip
  max_retries: 3
  retry_delay: 30s
  backoff: exponential               # linear | exponential | fixed
  on_exhausted: notify               # notify | ignore
  notify_channels: ["ops"]

max_runs_per_hour: 60

dry_run_config:                      # configuration-level dry-run, persisted on the job
  # When `enabled: true`, normal executions (run + scheduled triggers) skip the
  # underlying tool and return `mock_output`, but still take the success path —
  # so on_success events ARE emitted. The `dry_run: true` action, by contrast,
  # also suppresses event emission.
  enabled: false
  mock_output:
    status: "ok"
```

## Trigger Types

| Type | Fields | Description |
|------|--------|-------------|
| `cron` | `expression` | Cron schedule (e.g. `0 9 * * 1-5`) |
| `interval` | `every` | Periodic (e.g. `2h`, `30m`) |
| `event` | `listen`, `when` | React to a named event; optional `when` condition |
| `hotkey` | `keys` | Keyboard shortcut (e.g. `Ctrl+Shift+J`) |
| `manual` | — | Always available via UI or chat |
| `webhook` | `path` | HTTP webhook (v2) |

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

## How to Manage Jobs (`job` tool)

The `job` tool routes by combining `job_id` with the presence of action flags or write fields.

### Mutually Exclusive Actions

The following are **action flags** and cannot be combined with each other, nor with any write field (`name`, `description`, `pipeline`, `tags`, `tool`, `inputs`, `triggers`, `output`, `events`, `error_policy`, `max_runs_per_hour`, `dry_run_config`) or with `enabled`:

`delete: true` · `run: true` · `dry_run: true` · `list_runs: true` · `list_events: true` · `run_id: "<id>"`

### Action Matrix

| Action | Call shape (key arguments) | Notes |
|--------|----------------------------|-------|
| List jobs | `job()` (no args) | Returns summaries of all jobs. |
| Read job | `job(job_id)` | Returns the full job config. Sensitive fields inside the **configured** `inputs` are redacted (by sensitive key names and `{{ secret … }}` references); `last_run` is cleared. **Naming asymmetry:** the persisted dry-run config is returned under the `dry_run` key (matching `jobs.Job`'s JSON), while create/update accept it as `dry_run_config` — translate the key when round-tripping. |
| Create with generated id | `job(name, tool, triggers, …)` | `name` + `tool` + ≥ 1 `triggers` are required. Slug is derived from `name`. |
| Create with explicit id | `job(job_id, name, tool, triggers, …)` | When the (sanitized) id does not exist and the required create fields are present, creates a new job. The stored id may differ from the raw `job_id` — see slug normalization below. |
| Update | `job(job_id, <one or more write fields>)` | Only the provided fields are changed. Sending `triggers: []` resets to a single `manual` trigger. |
| Toggle enabled | `job(job_id, enabled: true \| false)` | Sending `enabled` alone routes to the dedicated toggle path. Combining `enabled` with other write fields is **allowed** — it becomes a regular update that also updates `enabled`. Only **action flags** (`delete`, `run`, `dry_run`, `list_runs`, `list_events`, `run_id`) are mutually exclusive with writes/`enabled`. |
| Delete | `job(job_id, delete: true)` | Destructive — confirm with the user. |
| Run now | `job(job_id, run: true)` | Triggers a real execution and returns the resulting `RunLog`. |
| Dry-run | `job(job_id, dry_run: true)` | Runs the job once without emitting downstream events. Uses the job's **persisted** `dry_run_config`: if `mock_output` is set there, that mock is returned without invoking the underlying tool (regardless of `dry_run_config.enabled`); otherwise the underlying tool **is** invoked for real and only event emission is suppressed. `dry_run_config` cannot be sent in the same call — it's a write field and is mutually exclusive with action flags; configure it via an update first. |
| List runs | `job(job_id, list_runs: true, status?, started_after?, started_before?, include_dry_run?, limit?)` | `status` is an **array of strings** (e.g. `status: ["failed"]` or `status: ["failed", "retrying"]`), values ∈ `completed`, `failed`, `retrying`, `skipped` — sending a bare string fails JSON parsing. Dates are RFC3339. `limit` defaults to 20 (max 100). Dry-runs are excluded unless `include_dry_run: true`. |
| Get one run (with timelines) | `job(job_id, run_id)` | Returns the `RunDetail`: `RunLog` + `run_events` (operational timeline) + `domain_events` (correlated by run). |
| List events | `job(list_events: true, job_id?, date?, start_at?, end_at?, event_type?, event_name?, limit?, offset?)` | `job_id` is optional (omit for global). Defaults to today when no time filter is set. `date` is `YYYY-MM-DD` and is ignored if `start_at`/`end_at` are given. `limit` defaults to 50 (max 200). |

### Examples

List all jobs:

```json
{}
```

Create a job that runs every weekday at 9am:

```json
{
  "name": "Daily Jira Sync",
  "tool": "mcp_jira__search_issues",
  "triggers": [{ "type": "cron", "expression": "0 9 * * 1-5" }],
  "inputs": { "jql": "project = OPS AND updated >= -1d" },
  "events": { "on_success": "ops.tickets.fetched" }
}
```

Update only the schedule of an existing job:

```json
{
  "job_id": "daily-jira-sync",
  "triggers": [{ "type": "cron", "expression": "0 8 * * 1-5" }]
}
```

Disable a job:

```json
{ "job_id": "daily-jira-sync", "enabled": false }
```

Dry-run a job:

```json
{ "job_id": "daily-jira-sync", "dry_run": true }
```

Inspect failures from the last 24h:

```json
{
  "job_id": "daily-jira-sync",
  "list_runs": true,
  "status": ["failed"],
  "started_after": "2026-05-31T00:00:00Z",
  "limit": 50
}
```

Get the timeline of a specific run:

```json
{ "job_id": "daily-jira-sync", "run_id": "run-abc123" }
```

## How to Manage Pipelines (`job_pipeline` tool)

Pipelines are persisted groupings. A job joins a pipeline by setting `pipeline: <slug>`. Disabling a pipeline disables scheduling for **all jobs** that belong to it (effective enabled = job.enabled AND pipeline.enabled).

| Action | Call shape | Notes |
|--------|------------|-------|
| List pipelines | `job_pipeline()` | Returns all pipelines. |
| Read pipeline | `job_pipeline(slug)` | Returns one pipeline. |
| Create with generated slug | `job_pipeline(name, description?, metadata?, enabled?)` | `name` is required. Slug is derived from `name`. |
| Create with explicit slug | `job_pipeline(slug, name, …)` | Creates if the slug does not exist. |
| Update | `job_pipeline(slug, <one or more write fields>)` | Write fields: `name`, `description`, `metadata`, `enabled`. |
| Toggle enabled | `job_pipeline(slug, enabled: true \| false)` | Affects scheduling for every job in the pipeline. |
| Delete | `job_pipeline(slug, delete: true)` | Cannot be combined with write fields. Destructive — confirm with the user. |

### Example

Create and then disable an ops pipeline:

```json
{ "name": "Ops Reports", "description": "Daily and weekly ops reporting" }
```

```json
{ "slug": "ops-reports", "enabled": false }
```

## Guidelines

- Every job needs a `name`, a `tool`, and at least one trigger. Prefer letting the slug be generated from `name`; only set `job_id` explicitly when you need a stable, well-known id.
- Slug normalization is asymmetric and worth knowing:
  - For **lookup/routing**, the incoming `job_id` is just lowercased and has spaces replaced by `-`.
  - For **creation** (slug derived from `name`, or explicit `job_id` being persisted), an extra sanitization runs: `/` and `\` become `-`, any character outside `[a-z0-9_-]` is collapsed to `-`, repeated `-` are merged, and leading/trailing `-` are trimmed.
  - Net effect: the **stored** id may differ from what was sent. Prefer ids that are already `[a-z0-9_-]+` to avoid surprises.
- Pipeline slugs follow the lookup rule (lowercase + spaces → `-`). A pipeline is **auto-created** the first time a job references its slug, so you don't need to `job_pipeline(create)` upfront.
- Always reference an existing tool from the tool catalog in `tool`. MCP tools are namespaced as `mcp_<serverSlug>__<toolName>` (e.g. `mcp_jira__search_issues`) — do not use dotted names like `mcp.jira.search_issues`, they will not resolve.
- Use `{{ .event.* }}` in `inputs` to pass data from upstream jobs that emitted the event you are listening to.
- Use `{{ secret "KEY" }}` for credentials — never hardcode secrets in `inputs`.
- Use `pipeline` to group related jobs and to enable/disable them together.
- When designing pipelines, define clear, stable event names (e.g. `tickets.fetched`, `report.generated`) so downstream jobs can listen to them.
- Use `trigger.when` to avoid wasteful tool calls on irrelevant events.
- Use `events.emit_when` to avoid noisy downstream propagation.
- For fan-out, set `events.for_each` to the output array path and optionally `emit_when` to filter items per iteration.
- Before enabling a freshly created job, validate it with `dry_run: true`. For repeated mocked executions during development, set `dry_run_config.enabled: true` with a `mock_output`.
- **Round-tripping a read payload:** `job(read)` returns the persisted shape of `jobs.Job` (`id`, `dry_run`, ...), but the write API uses different keys (`job_id`, `dry_run_config`) and the **type** of `dry_run` also differs (object in the read payload vs. boolean action flag in tool args). To turn a read payload into an update, rename **both** keys: `id` → `job_id` and `dry_run` → `dry_run_config`. Otherwise the tool treats the call as a create (no `job_id`) and/or fails argument parsing on `dry_run` (object → bool mismatch).
- Always pass structured fields as **real JSON values**, never as stringified JSON. `inputs`, `output`, `events`, `error_policy`, `dry_run_config` are objects; `triggers`, `tags`, `status` are arrays; `enabled`/`max_runs_per_hour`/`limit`/`offset` are a boolean/integers. Send `"inputs": {"task_list_slug": "ops"}`, not `"inputs": "{\"task_list_slug\": \"ops\"}"`. The tool will defensively coerce a stringified value when it is unambiguous, but a non-coercible string (e.g. `"enabled": "yes"`) is rejected with an explanatory error.
- Never combine **action flags** (`delete`, `run`, `dry_run`, `list_runs`, `list_events`, `run_id`) with each other or with write fields / `enabled` — they are mutually exclusive. (Note: `enabled` together with other write fields **is** allowed; it just becomes a regular update.)
- When listing runs/events, prefer narrow filters (status, time window, `event_name`) over large `limit` values.
- For destructive actions (`delete`), confirm with the user first.
