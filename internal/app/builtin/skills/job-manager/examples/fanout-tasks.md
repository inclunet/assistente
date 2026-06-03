# Recipe: event/webhook → N tasks via `for_each` fan-out

Turn one tool call that returns a list into **one downstream run per item**. Here a search of Jira
issues fans out into one task per issue.

## 1. Producer — fan-out on the output array

```json
{
  "name": "FSD Search For Fan-out",
  "pipeline": "fsd",
  "tool": "mcp_jira__search_issue",
  "triggers": [{ "type": "cron", "expression": "*/30 * * * *" }],
  "inputs": { "jql": "project = FSD AND status = 'To Do'" },
  "events": {
    "on_success": "fsd.issue.found",
    "for_each": "issues",
    "emit_when": "{{ ne .output.fields.status.name \"Done\" }}",
    "payload_template": "{ \"key\": {{ json .output.key }}, \"summary\": {{ json .output.fields.summary }}, \"priority\": {{ json .output.fields.priority.name }} }"
  }
}
```

- `for_each: "issues"` points to the array inside the tool output (`{ "issues": [ … ] }`).
- During fan-out the **current item is `.output`** — note `.output.key`, **never `.item.key`**.
- `emit_when` runs **per item**, so only matching issues emit an event.
- `payload_template` reshapes each item into a tidy event payload (must render a JSON object). Wrap
  **every** dynamic value with the `json` function (no manual quotes) so quotes/newlines are escaped
  correctly — otherwise an invalid render silently falls back to the raw output.

## 2. Consumer — one run per emitted item

```json
{
  "name": "FSD Create Task Per Issue",
  "pipeline": "fsd",
  "tool": "mcp_tasklist__create_task",
  "triggers": [{ "type": "event", "listen": "fsd.issue.found" }],
  "inputs": {
    "task_list_slug": "fsd",
    "title": "[{{ .event.priority }}] {{ .event.key }} — {{ .event.summary }}"
  }
}
```

- The fields shaped by the producer's `payload_template` are read here as `.event.*`.
- Use the stable string `task_list_slug` (not a numeric `task_list_id`) to avoid
  `cannot unmarshal number into Go struct field … of type string`.

## Notes

- If `for_each` does not resolve to an array, the producer emits a **single** event instead of fanning out.
- Each fan-out event carries `_fan_out_index` / `_fan_out_total` for ordering/aggregation.
- A webhook trigger (`{ "type": "webhook", "path": "/fsd" }`) works the same way as the cron producer above.
