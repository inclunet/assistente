# Recipe: cron → MCP event chain (FSD-style)

A scheduled job calls an MCP tool, then chains to a second job via an event. This is the
backbone of the FSD/CICDDELIV pipelines: one atomic tool call per job, wired by events.

## 1. Producer — scheduled search

Runs every weekday at 9am, searches Jira, and emits `fsd.tickets.fetched` on success.

```json
{
  "name": "FSD Daily Search",
  "pipeline": "fsd",
  "tool": "mcp_jira__search_issues",
  "triggers": [{ "type": "cron", "expression": "0 9 * * 1-5" }],
  "inputs": { "jql": "project = FSD AND status = 'To Do' ORDER BY created DESC" },
  "events": {
    "on_success": "fsd.tickets.fetched",
    "emit_when": "{{ gt (len .output.issues) 0 }}"
  },
  "error_policy": { "strategy": "retry", "max_retries": 3, "retry_delay": "30s", "backoff": "exponential" }
}
```

- `emit_when` avoids waking the chain when the search returned nothing (`<no value>`/empty → falsy).
- `retry` + `exponential` backoff absorbs transient Jira rate limits.

## 2. Consumer — reacts to the event

Listens to `fsd.tickets.fetched` and posts a summary. It reads the upstream payload via `.event`.

```json
{
  "name": "FSD Notify Summary",
  "pipeline": "fsd",
  "tool": "mcp_slack__post_message",
  "triggers": [{ "type": "event", "listen": "fsd.tickets.fetched" }],
  "inputs": {
    "channel": "#fsd",
    "text": "{{ len .event.issues }} new FSD tickets — keys: {{ join (pluck .event.issues \"key\") \", \" }}"
  }
}
```

## Notes

- The two jobs share the `fsd` pipeline, so disabling the pipeline pauses the whole chain.
- The emitted event payload also carries `_chain_id` (read with `{{ .event._chain_id }}`) for tracing.
- Validate the consumer before enabling: set a `dry_run_config.mock_output` matching the producer's
  output shape and run `dry_run: true`.
