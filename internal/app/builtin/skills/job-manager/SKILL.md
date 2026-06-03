---
name: job-manager
version: 2.2.0
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
  payload_template: |                                  # reshape payload (optional); wrap every value with `json`
    {
      "id": {{ json .output.id }},
      "source": {{ json .event.origin }}
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

## Domain events — listen to app surfaces (AEP-0067)

Besides job-to-job events, the app **surfaces** publish semantic **domain events** onto the same EventBus. A job can subscribe to them with `trigger.type: event` + `listen: "<name>"` exactly like any other event. The first producer is the **tasklist** surface; the catalog is extensible to future surfaces (`chat.*`, `workspace.tab.*`, `terminal.*`, `editor.*`).

Key facts:

- These events appear in the JobBuilder event picker even when **no job** emits them yet (static catalog in `internal/jobs/domain_events.go`).
- Emission is **cheap when nobody listens** — but the details differ by producer:
  - The **tasklist surface** checks `HasDomainListener` *before* building the payload, so with no subscriber **nothing is computed**.
  - **User-triggered custom actions** are different: they render `link`/`payload_template` first and only then call `PublishDomainEvent`, which **no-ops** if there are no subscribers. The publish/fan-out is skipped, but the template render already happened (negligible, and only on explicit user action).
- Domain events published via `PublishDomainEvent` carry **provenance** fields for anti-loop guards (see [Anti-loop guidance (provenance)](#anti-loop-guidance-provenance)). Note: `_source_job_id` is only set for job-originated mutations and is empty for user-originated events.

### Tasklist domain event catalog

| Event | Emitted when | Notable payload fields (besides provenance) |
|-------|--------------|---------------------------------------------|
| `tasklist.task.created` | A task is created | `task_id`, `task_list_id`, `task_list_slug`, `code`, `title`, `status_id`, `parent_id`, `assignee_id`, `link` |
| `tasklist.task.updated` | A task's fields change | + `changed_fields` (array of field names; omitted if the post-update snapshot could not be reloaded) |
| `tasklist.task.status_changed` | Status changes | + `from_status_id` |
| `tasklist.task.assignee_changed` | Assignee changes | + `from_assignee_id` |
| `tasklist.task.moved` | Task moves to another list | + `from_task_list_id` |
| `tasklist.task.reparented` | Task's parent changes | + `from_parent_id` |
| `tasklist.task.reordered` | Task order changes within a column | `task_list_id`, `task_list_slug`, `status_id`, `ordered_ids` (list-level event; no per-card fields) |
| `tasklist.task.completed` | Task reaches a completed state | task fields (`completed_at` set) |
| `tasklist.task.deleted` | Task is deleted | task fields (best-effort: if the pre-delete snapshot can't be loaded, only `task_id` is published) |
| `tasklist.note.added` / `.updated` / `.deleted` | Note lifecycle | `note_id`, `task_id`, `task_list_id`, `note_type`, `source`, `external_id`, `author_id`. `.added` always carries full fields; `.updated`/`.deleted` are best-effort — only `note_id` is published when the note snapshot can't be (re)loaded |
| `tasklist.list.created` / `.updated` | List lifecycle | `task_list_id`, `task_list_slug`, `title` |
| `tasklist.list.cleared` / `.deleted` | List cleared/deleted | `task_list_id`, `task_list_slug` (no `title`) |
| `tasklist.list.cloned` | List cloned | `task_list_id`, `task_list_slug`, `title`, + `source_task_list_id` |
| `tasklist.list.refresh_requested` | User triggers an optional **Atualizar** board custom action (manual one-shot; not a fixed button) | `task_list_id`, `task_list_slug` |
| `tasklist.workflow.updated` | Workflow changes | `task_list_id`, `task_list_slug`, `initial_status_id` |
| `tasklist.item.opened` | *(reserved — present in the static catalog so it shows in the picker, but no producer emits it yet)* | task fields |

`code` ≡ external id of the card. Use `task_list_slug` (stable) over numeric ids in `inputs` (see the slug section).

### Custom actions (per-TaskList, user-defined)

A `TaskList` can define **custom actions** that appear in the card context menu, the card detail screen and/or the board menu. Each action can **publish a templated event** (any name — including a job-listenable one) and/or **open a templated link/deep link**. The event a custom action publishes is a first-class EventBus event: just create a job with `trigger.listen: "<that event name>"`.

Custom-action events also appear in `ListKnownEvents`, so they show up in the picker once configured. Their payload includes `action_id`, `task_list_id`, `task_list_slug`, the task identity (when fired from a card) plus whatever the action's `payload_template` adds. They carry the same provenance fields.

### Recipe: react to a manual refresh

```json
{
  "name": "Sync FSD on manual refresh",
  "tool": "mcp_jira__search_issue",
  "triggers": [
    { "type": "event", "listen": "tasklist.list.refresh_requested",
      "when": "{{ eq .event.task_list_slug \"fsd\" }}" }
  ],
  "inputs": { "jql": "project = FSD AND updated >= -7d" },
  "events": { "on_success": "fsd.issue.found", "for_each": "issues" }
}
```

## Template Reference

This is the **formal contract** for every templated field (`inputs`, `output.map`, `trigger.when`, `events.emit_when`, `events.payload_template`). Read it before writing any template — most job bugs come from a wrong mental model here. (Source of truth: `internal/jobs/template.go` and `internal/jobs/executor.go`.)

### Engine

The engine is **Go `text/template`** — *not* Jinja, Mustache or Handlebars. Go syntax, Go semantics, Go pipelines. A template that "looks right" in another engine will silently misbehave here.

### Root variables (the only three)

The data passed to every template is exactly:

```go
data := map[string]any{
    "event":  ctx.Event,   // payload of the event that triggered the job
    "output": ctx.Output,  // the tool result of THIS job — or the current item in fan-out
    "now":    ctx.Now,     // time.Now()
}
```

| Variable | What it is | When to use it |
|----------|-----------|----------------|
| `.event.X` (≡ `$.event.X`) | Payload of the event that triggered the job | `trigger.type: event` jobs consuming the upstream payload, in `inputs`/`when` |
| `.output.X` | Tool result of this job **or the current item when `for_each` fan-out is active** | `output.map`, `emit_when`, `payload_template` |
| `.now` | `time.Now()` (use with `date`) | timestamps |

Key facts:

- `$` is the template root. At the top level `.event` **is** `$.event` — they are interchangeable. `$` only matters **inside a `range`**, where `.` is rebound to the current element but `$.event` still reaches the root.
- There is **no `.item`**. In a `for_each` fan-out, the current array element becomes `.output` (see [Fan-out and iteration](#patterns-fan-out-and-iteration)). Writing `{{ .item.key }}` resolves to nothing.
- A **missing reference renders `<no value>`** — never an error, never `<nil>`. In conditions (`when`/`emit_when`), `<no value>`, `""` and `"false"` are all **falsy**.
- All three roots (`event`, `output`, `now`) **always exist** in the data map, but `.output` is only **populated after the tool runs**. During `inputs` resolution (and `trigger.when`), `.output` is still **empty (nil)**, so `.output.*` resolves to `<no value>` there. `.output` only carries data in `output.map`, `events.payload_template` and `events.emit_when`.

### Functions

**Go built-ins** are available: `eq`, `ne`, `lt`, `le`, `gt`, `ge`, `and`, `or`, `not`, `index`, `len`, `print`, `printf`, plus actions `if`/`else`, `range`, `with`. There is **no `upper`, `lower` or `toJson`** — they do not exist; do not use them.

**Custom functions** (from `templateFuncs`):

| Function | Signature / Usage | Description |
|----------|-------------------|-------------|
| `pluck` | `{{ pluck .output.issues "key" }}` | Extract a (dot-path) field from every item of a slice → new slice |
| `any` | `{{ any .output.issues "fields.priority.name" "Critical" }}` | True if any item's dot-path equals the value |
| `join` | `{{ join .output.keys ", " }}` | Join a slice into a string with a separator |
| `json` | `{{ json .output }}` | Serialize a value to a JSON string (this is the `toJson` replacement) |
| `default` | `{{ default 50 .event.limit }}` | **Argument order is `default <fallback> <value>`** — returns `<value>` unless it is nil/zero, in which case returns `<fallback>`. ⚠️ Fallback comes **first** (unlike Sprig's `default`). |
| `date` | `{{ date .now "2006-01-02" }}` | Format a `time.Time` (or RFC3339 string) using a Go layout |
| `now` | `{{ date (now) "2006-01-02" }}` | Function returning the current `time.Time`. Takes **no arguments**. ⚠️ Distinct from the root variable `.now` — see the pitfall below. |
| `secret` | `{{ secret "API_KEY" }}` | Resolve a secret by name — never hardcode credentials |
| `adf_markdown` | `{{ adf_markdown .event.description }}` | Render an Atlassian Document Format (ADF) node to Markdown |
| `adf_text` | `{{ adf_text .event.description }}` | Render an ADF node to plain text |

### Auto-corrections applied before parsing

Two forgiving rewrites run on every template **before** it is parsed:

- **`fixTemplateDots`**: a leading `{{ event.x }}` / `{{ output.x }}` / `{{ now }}` gets the missing dot → `{{ .event.x }}`. This only fixes the root word at the **start** of a `{{ … }}` block; inside `if`/`range`/`with` you must write the dot yourself (e.g. `{{ if .event.x }}`). **Always write the leading dot** — do not rely on the auto-fix.
- **`fixArrayAccess`**: JS-style numeric dot indexing is converted to a Go `index` call → `{{ .event.content.0.id }}` becomes `{{ (index .event.content 0).id }}`. You can write either form.

> ⚠️ **`now` function vs `.now` variable pitfall.** Both the `now` *function* (which takes **no arguments** — `func() time.Time`) and the `.now` root *variable* return the current time. But `fixTemplateDots` rewrites a leading `now` without a dot, so any block that **starts** with `now` — `{{ now }}` or even a pipe like `{{ now | … }}` — is silently rewritten to `{{ .now }}`, i.e. it becomes the **variable**, not a function call. (There is no valid `{{ now "…" }}` form — passing arguments to `now` is an error anyway.) To actually **call** the function, keep it off the start of the block by **parenthesizing** it: `{{ date (now) "2006-01-02" }}` — there the auto-fix leaves it alone. In practice just use the `.now` variable (e.g. `{{ date .now "2006-01-02" }}`) unless you specifically need a fresh `time.Now()`.

### `payload_template` renders a JSON string

`events.payload_template` is special: the template must render a **JSON object string** that is then `json.Unmarshal`-ed into the emitted payload map. Rules:

- The rendered text must be a valid JSON **object** (a `map`), not an array or scalar.
- **Wrap *every* dynamic value with the `json` function — including strings** — and do **not** add manual quotes around it. `json` escapes and quotes the value correctly. Manually quoting (`"id": "{{ .output.id }}"`) does **not** guarantee valid JSON: if the value contains a `"`, a newline or other control characters, `json.Unmarshal` fails and you hit the silent fallback below. Correct form:

  ```
  { "task_code": {{ json .output.key }}, "title": {{ json .output.fields.summary }}, "raw": {{ json .output.fields }} }
  ```

- **Silent fallback**: if the rendered text is not valid JSON, the error is only logged and the job emits the **original, unshaped output** instead. There is no run failure — so a broken `payload_template` looks like "it ignored my template". Validate it with a dry-run and inspect the emitted payload.

### Common template errors & how to diagnose

| Symptom | Cause | Diagnosis / fix |
|---------|-------|-----------------|
| Field comes out as `<no value>` | Wrong path, or used `.item.*` in fan-out, or referenced `.output.*` during `inputs`/`when` resolution — `.output` exists as a root but is still **empty (nil)** before the tool runs, so `.output.*` is `<no value>` there | Inspect the resolved value with a dry-run; confirm the path against the upstream payload via `job(job_id, run_id)` → `output` |
| `template: invalid character ':' in variable reference` | JSON/Jinja-style syntax inside `{{ }}` (e.g. `{{ event:foo }}` or `{{ {"a":1} }}`) | Use Go syntax. To emit a JSON literal in `payload_template`, put the JSON **outside** `{{ }}` and only interpolate values inside them |
| `payload_template` seems ignored | Rendered text is not a valid JSON object → silent fallback to raw output | Dry-run and check the emitted payload; render a valid JSON object wrapping **every** dynamic value with `json` (no manual quotes), e.g. `{ "task_code": {{ json .output.key }} }` |
| Used `upper`/`lower`/`toJson` and parse fails | Those functions do not exist | Use `json` for serialization; do case changes upstream or omit |
| `default` returns the wrong branch | Argument order: it is `default <fallback> <value>` | Put the fallback first: `{{ default 50 .event.limit }}` |

See [`troubleshooting.md`](./troubleshooting.md) for runtime (non-template) errors.

## Patterns: fan-out and iteration

`events.for_each` turns one job run into **N events** — one per element of an output array. Use it whenever a single tool call returns a list and you want one downstream job run **per item** (e.g. one task per Jira issue).

How it works (`resolveForEachItems` + `emitSuccess` in `executor.go`):

1. `for_each` is a **dot-path into the tool output** that must resolve to an array (e.g. `"issues"`, `"data.items"`). If it does not resolve to an array, the job falls back to emitting a **single** event.
2. For each element:
   - If the element is an object, its keys become the event payload (so a child reads `{{ .event.<key> }}`). If it is a scalar, it is wrapped as `{{ .event.content }}`.
   - Two bookkeeping fields are added: `_fan_out_index` and `_fan_out_total`.
   - `emit_when` is evaluated **per item**, with the item exposed as `.output` — filter items with `{{ .output.X }}`.
   - `payload_template` is applied **per item**, again with the item as `.output`.
3. Each emitted event is published under `events.on_success`.

> ⚠️ Inside the producer's `emit_when` / `payload_template`, the current item is `.output.X` — **never `.item.X`** (`.item` does not exist). In the **child** job that listens to the event, the same fields are read as `.event.X`.

### Recipe: one task per issue from a Jira search

**Producer** — searches Jira and fans out one event per issue:

```json
{
  "name": "FSD Search Tickets",
  "tool": "mcp_jira__search_issues",
  "triggers": [{ "type": "cron", "expression": "0 9 * * 1-5" }],
  "inputs": { "jql": "project = FSD AND status = 'To Do'" },
  "events": {
    "on_success": "fsd.issue.found",
    "for_each": "issues",
    "emit_when": "{{ ne .output.fields.status.name \"Done\" }}",
    "payload_template": "{ \"key\": {{ json .output.key }}, \"summary\": {{ json .output.fields.summary }} }"
  }
}
```

**Consumer** — one run per emitted issue, reading the item via `.event`:

```json
{
  "name": "FSD Create Task Per Issue",
  "tool": "mcp_tasklist__create_task",
  "triggers": [{ "type": "event", "listen": "fsd.issue.found" }],
  "inputs": {
    "task_list_slug": "fsd",
    "title": "{{ .event.key }} — {{ .event.summary }}"
  }
}
```

If `mcp_jira__search_issues` returns `{ "issues": [ {…}, {…} ] }`, the producer emits `fsd.issue.found` once per issue and the consumer runs once per issue.

## Inputs: use stable string slugs (`task_list_slug`), not numeric ids

Prefer **stable string identifiers** in `inputs` over numeric ids. The canonical example is `task_list_slug: "fsd"` instead of `task_list_id: 2`.

Why this matters:

- Input templates always produce strings, and `CoerceInputs` only coerces a **string → number/bool/array/object** when the tool schema asks for it. It never coerces a JSON **number → string**.
- So a literal `task_list_id: 2` (a JSON number) sent to a tool whose schema declares that field as a `string` fails at execution with:

  ```
  cannot unmarshal number into Go struct field <…> of type string
  ```

- With `error_policy.strategy: skip`, that failure is recorded as **`skipped`** rather than `failed` — so the job can stay broken silently for days.

**Fix:** use the string slug (`task_list_slug`), or pass the id as a quoted string if the tool's schema really expects a string. Confirm the field type in the tool catalog schema before choosing.

## `error_policy` in practice

Confirmed semantics (retry loop in `executor.go`):

- `maxAttempts` is `1` by default. It becomes `max_retries + 1` **only when `strategy: retry` and `max_retries > 0`**.
- **`strategy: skip` never retries**, even if `max_retries` is set. When all attempts fail, the final status is flipped from `failed` to **`skipped`**.
- **`strategy: stop`** also does not retry; the run ends as **`failed`** and `on_failure` is emitted.
- `on_exhausted: notify` fires **regardless of strategy** once attempts are exhausted (defaults to the `chat` channel if `notify_channels` is empty).

### When to use each strategy

| Strategy | Retries? | Final status on failure | Use when |
|----------|----------|-------------------------|----------|
| `retry` | Yes (`max_retries+1` attempts) | `failed` after exhaustion | Transient errors: network blips, rate limits, DB locks, flaky upstreams |
| `stop` | No | `failed` (emits `on_failure`) | Deterministic bugs where retrying wastes calls and you want the failure to propagate downstream |
| `skip` | No | `skipped` (no failure propagation) | Non-critical jobs where an occasional failure is acceptable — **but watch out:** failures hide as `skipped` |

### Backoff (only meaningful with `strategy: retry`)

`retry_delay` is the base delay (default `30s`). For retry attempt `n` (1-based):

| `backoff` | Delay for attempt `n` | Example with `retry_delay: 10s` |
|-----------|------------------------|----------------------------------|
| `fixed` (default) | `base` | 10s, 10s, 10s |
| `linear` | `base × n` | 10s, 20s, 30s |
| `exponential` | `base × 2^(n-1)` | 10s, 20s, 40s |

Pick `fixed` for steady rate limits, `linear`/`exponential` to back off from a struggling dependency.

## Dry-run & validation

Validate a job before enabling it, without firing downstream events, using the two dry-run mechanisms:

- **`job(job_id, dry_run: true)` action** — runs the job once and **suppresses event emission** (`ExecuteDryRun`):
  - If the persisted `dry_run_config.mock_output` is set, it is returned **without invoking the underlying tool** — ideal for inspecting the output/payload shape your downstream jobs will consume, with **no MCP call at all**.
  - If no mock is set, the underlying tool **is executed for real** (use only with safe/read-only tools), and only event emission is suppressed.
- **`dry_run_config.enabled: true` + `mock_output`** (persisted) — normal/scheduled runs skip the tool and return the mock, but **still take the success path so `on_success` events ARE emitted**. Use this to develop a downstream chain against a deterministic mock payload.

Recommended flow: set a representative `dry_run_config.mock_output`, run `dry_run: true` to confirm the output shape, then unset/disable the mock before going live.

## Auditing chronically broken jobs

Failures (and especially `skipped`) can pile up unnoticed. Two complementary recipes using the `job` tool:

1. **Per-job failure rate** — `list_runs` is authoritative for status. For each job, pull the recent window and compare failed/skipped vs total:

```json
{ "job_id": "update-fsd-tiket-statuses", "list_runs": true, "status": ["failed", "skipped"], "limit": 100 }
```

   Compare the returned count against an unfiltered `list_runs` (`limit: 100`) for the same job to get a failure ratio. To sweep every job, first call `job()` (no args) to list all jobs, then iterate.

2. **Cross-job scan** — `list_events` accepts an **optional** `job_id` (omit for global) and an `event_type` filter, so you can spot failures across all jobs for a day:

```json
{ "list_events": true, "event_type": "failed", "date": "2026-06-01" }
```

Consider a daily **meta-monitoring** job (see [`examples/`](./examples/)) that flags any job exceeding a failure threshold.

## Traceability: event ↔ runs

Event payloads are enriched with correlation/provenance fields you **can read downstream** (and filter on with `trigger.when`):

- `_chain_id` — stable id of the whole reactive chain (the originating run id). Read it in a child via `{{ .event._chain_id }}`.
- `_chain_history` — ordered history used by the circuit breaker for loop/depth detection. For job-to-job chains it accumulates job ids; for a domain event published from a surface it starts with the originating **event name**.
- `_source` / `_source_job_id` — added by `PublishDomainEvent` (i.e. on domain events). `_source` is `"user"` (a human acted in the UI) or `"job"` (an automation acted); domain events inherit this from the acting context (`internal/eventctx`). `_source_job_id` holds the job id when `_source == "job"`, and is empty for user-originated events.
- For fan-out items, `_fan_out_index` / `_fan_out_total` are also present on the event.

### Anti-loop guidance (provenance)

Domain events fire for **every** mutation, including those a job itself performs (e.g. a job that moves a card emits `tasklist.task.status_changed`). Without care, a job listening to the very events it causes will loop. Two layers protect you:

1. **Circuit breaker** — `_chain_history` + `_chain_id` cap chain depth and detect repeats automatically.
2. **Explicit `trigger.when` on provenance** — the recommended, intentional guard. React only to human actions, or skip events your own automation caused:

```yaml
# Only react to human-driven changes (ignore job-driven mutations)
when: '{{ eq .event._source "user" }}'

# Or: react to job-driven changes but never to your own.
# Must also require _source == "job": _source_job_id is empty for user events,
# so `ne _source_job_id "move-card-job"` alone would also match human actions.
when: '{{ and (eq .event._source "job") (ne .event._source_job_id "move-card-job") }}'
```

Prefer gating high-frequency events (`tasklist.task.updated`, `.status_changed`, `.moved`) on `_source` so an automation chain cannot feed itself.

To trace a run: `job(job_id, run_id)` returns the `RunDetail` with `run_events` (operational timeline) and `domain_events` (the emitted/received events correlated by run). First-class visible run fields for cross-run correlation (`triggered_by_event`, `triggered_by_run_id`, `triggered_by_job_id`) are proposed in **#164**.

## Examples

Canonical, copy-pasteable recipes live in [`examples/`](./examples/):

- [`cron-mcp-chain.md`](./examples/cron-mcp-chain.md) — scheduled job that kicks off an MCP event chain (FSD-style).
- [`fanout-tasks.md`](./examples/fanout-tasks.md) — webhook/event → N tasks via `for_each` fan-out.
- [`meta-monitoring.md`](./examples/meta-monitoring.md) — a daily job that audits other jobs' failure rates.
- [`conditional-when.md`](./examples/conditional-when.md) — `when`/`emit_when` gating on upstream output.

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
