# Job Manager — Troubleshooting

Known errors observed in production pipelines (FSD/CICDDELIV), with the exact message, the
real cause, how to diagnose it, and the fix. For **template** errors specifically, see the
"Common template errors" table in `SKILL.md`.

## How to diagnose any failing job

1. List recent bad runs: `job(job_id, list_runs: true, status: ["failed", "skipped"], limit: 50)`.
2. Open one run's timeline: `job(job_id, run_id: "<id>")` → inspect:
   - `error` — the failure message.
   - `resolved_inputs` — what was actually sent to the tool after template resolution (secrets redacted).
   - `output` — the raw tool result (for chained jobs, what downstream consumers receive).
   - `run_events` — the ordered operational timeline (`triggered` → … → `failed`/`skipped`).
   - `domain_events` — emitted/received events correlated to the run.
3. For a cross-job view of a day: `job(list_events: true, event_type: "failed", date: "YYYY-MM-DD")`.

---

## `cannot unmarshal number into Go struct field <…> of type string`

- **Cause:** an `inputs` value is a JSON **number** but the tool's schema declares that field as a
  **string** (classic case: `task_list_id: 2` instead of `task_list_slug: "fsd"`). `CoerceInputs`
  only coerces *string → number/bool/array/object*; it never converts a number to a string.
- **Diagnose:** `list_runs` shows the run as `failed` (or `skipped` if `error_policy.strategy: skip`);
  the `run_id` detail's `error` field carries this message. Check the offending field in `resolved_inputs`.
- **Fix:** use the stable string slug (`task_list_slug`) or quote the value as a string. Verify the
  field type against the tool catalog schema before choosing.

## `tool execute: context canceled` / `cancelled during retry` (historical, fixed)

- **Status:** **fixed on `main`.** This was a runtime bug (**#164**) where event-driven runs inherited
  the event subscriber's context and were canceled when it ended. The fix (**PR #166**) decouples the
  run lifetime via `context.WithoutCancel` in the event subscriber, so this failure mode should no
  longer occur for event-driven runs after that merge. Kept here as a **known/historical** error for
  diagnosing older runs.
- **Cause:** the run's context was canceled mid tool-call. Once the context is canceled, the retry
  loop **short-circuits immediately** with `cancelled during retry` — it does **not** wait for
  `retry_delay` or perform further attempts.
- **Diagnose:** `run_events` ends with a `failed` entry whose message contains `context canceled` or
  `cancelled during retry`; the run usually has little/no `output`. It correlated with event-triggered
  (fan-out/chained) runs rather than manual `run: true`.
- **Fix:** ensure you are on a build that includes **#166/#164**. **`strategy: retry` does NOT help**
  with this failure mode — a canceled context aborts the retry loop instantly rather than retrying.
  For old/historical runs the only recourse was a manual rerun (`run: true`), which executes outside
  the canceled event context.

## `template: invalid character ':' in variable reference`

- **Cause:** JSON/Jinja-style syntax inside a `{{ … }}` block — e.g. `{{ event:foo }}` or pasting a
  raw JSON object `{{ {"a": 1} }}` into a template. Go `text/template` does not allow `:` in a
  variable reference.
- **Diagnose:** the run fails immediately at template resolution; the `error` includes the offending
  template text (`template exec error (template=…)` or `template parse error`).
- **Fix:** use Go template syntax. In `payload_template`, keep the JSON **structure outside** `{{ }}`
  and only interpolate values inside them, e.g. `{ "id": "{{ .output.id }}" }`.

## A field renders as `<no value>`

- **Cause:** a missing/wrong path. Common variants: using `.item.X` in a `for_each` fan-out
  (`.item` does not exist — use `.output.X`), referencing `.output.*` during `inputs`/`when`
  resolution (`.output` exists as a root variable but is still **empty/nil** before the tool runs,
  so `.output.*` renders `<no value>` there — it is only populated for `output.map`,
  `payload_template` and `emit_when`), or a typo in the dot-path.
- **Diagnose:** dry-run and inspect the resolved value, or compare the path to the upstream payload
  via `job(job_id, run_id)` → `output`.
- **Fix:** correct the path. Remember `<no value>` is falsy in `when`/`emit_when`.

## `payload_template` appears to be ignored

- **Cause:** the rendered text is not a valid JSON **object**, so the executor logs the parse error
  and **silently falls back** to emitting the original, unshaped output.
- **Diagnose:** dry-run the producer and inspect the emitted payload shape; check application logs for
  `payload_template JSON parse error`.
- **Fix:** ensure the template renders a single JSON object with quoted string values; serialize
  nested structures with `{{ json .output.X }}`.

## Job silently `skipped` for a long time

- **Cause:** `error_policy.strategy: skip` converts every exhausted failure into `skipped`, which is
  easy to miss because it is not counted as a failure.
- **Diagnose:** include `skipped` in `list_runs` status filters; audit failure rates per the
  "Auditing chronically broken jobs" section in `SKILL.md`.
- **Fix:** reconsider `skip` for important jobs (use `retry`/`stop`), and add meta-monitoring.
