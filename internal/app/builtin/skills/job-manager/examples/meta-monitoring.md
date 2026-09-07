# Recipe: meta-monitoring (audit chronically broken jobs)

Jobs can fail — or silently `skip` — for days without anyone noticing. This recipe surfaces them.

## Manual audit (ad-hoc)

1. List all jobs: `job()` (no args).
2. For each candidate, pull the recent window and compare bad vs total runs:

```json
{ "job_id": "update-fsd-tiket-statuses", "list_runs": true, "status": ["failed", "skipped"], "limit": 100 }
```

   then compare against the unfiltered count:

```json
{ "job_id": "update-fsd-tiket-statuses", "list_runs": true, "limit": 100 }
```

   A high `failed+skipped / total` ratio means the job is chronically broken.

3. Cross-job daily scan (global — omit `job_id`):

```json
{ "list_events": true, "event_type": "failed", "date": "2026-06-01" }
```

## Automated meta-job (daily)

A scheduled job that calls a monitoring/notification tool and only emits when something is wrong.
Wire it to a notifier (e.g. Slack) via an event consumer.

```json
{
  "name": "Jobs Health Watchdog",
  "pipeline": "ops",
  "tool": "mcp_jobs__failure_report",
  "triggers": [{ "type": "cron", "expression": "0 8 * * *" }],
  "inputs": { "window_runs": 100, "threshold_pct": 50 },
  "events": {
    "on_success": "ops.jobs.unhealthy",
    "emit_when": "{{ gt (len .output.unhealthy) 0 }}"
  }
}
```

```json
{
  "name": "Notify Unhealthy Jobs",
  "pipeline": "ops",
  "tool": "mcp_slack__post_message",
  "triggers": [{ "type": "event", "listen": "ops.jobs.unhealthy" }],
  "inputs": {
    "channel": "#ops",
    "text": "Jobs over failure threshold: {{ join (pluck .event.unhealthy \"job_id\") \", \" }}"
  }
}
```

## Notes

- `emit_when` keeps the alert quiet on healthy days (empty list → falsy).
- Replace `mcp_jobs__failure_report` with whatever reporting tool exists in your catalog; the pattern
  (scheduled audit → conditional event → notification) is what matters.
- Watch `skipped` explicitly: with `error_policy.strategy: skip`, failures hide as `skipped`.
