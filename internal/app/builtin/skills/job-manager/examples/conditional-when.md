# Recipe: conditional `when` / `emit_when`

Gate work at both ends of a job: skip the tool call on irrelevant triggers (`trigger.when`), and
suppress noisy downstream events (`events.emit_when`). Both are Go templates; falsy = skip
(`<no value>`, `""` and `"false"` are all falsy).

## Input gate — only act on relevant events

Listens to a generic Jira webhook event but only runs when the event is an issue update:

```json
{
  "name": "FSD React To Issue Updates",
  "tool": "mcp_jira__get_issue",
  "triggers": [
    {
      "type": "event",
      "listen": "jira.webhook",
      "when": "{{ eq .event.webhookEvent \"jira:issue_updated\" }}"
    }
  ],
  "inputs": { "issue_key": "{{ .event.issue.key }}" },
  "events": {
    "on_success": "fsd.issue.refreshed",
    "emit_when": "{{ eq .output.fields.status.name \"Done\" }}"
  }
}
```

- `trigger.when` is evaluated **before** the tool runs — avoids wasteful calls on unrelated webhooks.
- `events.emit_when` is evaluated **after** the tool runs — here it only propagates when the issue
  reached `Done`, keeping the downstream chain quiet otherwise.

## Output gate combined with `default`

Use `default <fallback> <value>` (fallback first!) to make conditions robust to missing fields:

```json
{
  "events": {
    "on_success": "report.ready",
    "emit_when": "{{ gt (default 0 .output.count) 0 }}"
  }
}
```

`{{ default 0 .output.count }}` yields `.output.count` unless it is nil/zero, in which case `0`,
so the event is suppressed when there is nothing to report.

## Notes

- `when` + `emit_when` are the symmetric input/output pair — see the table in `SKILL.md`.
- For per-item gating in a `for_each` fan-out, `emit_when` is evaluated per item with the item as `.output`.
