# AEP-0001: Jobs — Event-Driven Automation

- **Status**: Draft
- **Autor**: Leonardo Gleison Ferreira
- **Data**: 2026-03-20
- **Inspiração**: [Huginn](https://github.com/huginn/huginn) evolved — com MCP, tool calling e LLM

## Resumo

Sistema de automação event-driven integrado ao Assistente. Cada **Job** é uma unidade atômica que executa **1 tool call** e emite **1 evento**. Jobs se encadeiam por eventos, formando pipelines reativas sem acoplamento direto.

## Motivação

O Assistente já possui infraestrutura robusta de MCP (Jira, Confluence, Slack, Gmail, GitHub, etc.) e tool calling. Falta uma camada de **orquestração temporal e reativa** que permita:

- Agendar tool calls em horários específicos
- Reagir a eventos (output de outros jobs, webhooks)
- Encadear ações automaticamente (buscar dados → transformar → notificar)
- Monitorar APIs e serviços periodicamente

## Filosofia

```
Huginn agents    →  Jobs
Huginn events    →  Events (mesmo conceito)
Huginn scenarios →  Pipelines (grupo visual de jobs)
HTTP/scraping    →  MCP tools + internal tools + HTTP + LLM (futuro)
Liquid templates →  Template engine {{ event.campo }}
Ruby agent types →  Qualquer tool MCP registrada = novo "type" automático
```

**Diferencial vs Huginn**: no Huginn, cada agent type precisa ser escrito em Ruby. Aqui, cada MCP server conectado expande automaticamente o catálogo de tools disponíveis para Jobs.

## Decisões de Design

| Aspecto | Decisão |
|---|---|
| Modelo | 1 Job = 1 tool call (atômica) |
| Encadeamento | Eventos custom definidos pelo usuário |
| LLM no loop | Não na v1, futuro como tool type |
| Persistência | 1 arquivo YAML por job |
| Gerenciamento | Filesystem — sem tools especiais, usa read/write/edit/delete_file |
| Hot reload | File watcher na pasta de jobs + validação YAML |
| Criação | Chat (skill job-builder) + UI (Job Builder visual) |
| Error policy | Configurável por job |
| Notificação | Chat do Assistente + Telegram/Signal |
| Runtime v1 | Só com app aberto |
| Dry run | Suportado — executa com mock ou valores reais, sem efeitos colaterais |

## Modelo Core

```
1 Job = 1 Tool Call = 1 Unidade Atômica
```

Encadeamento é feito por eventos — um job emite, outro escuta:

```
┌─────────┐  evento A  ┌─────────┐  evento B  ┌─────────┐
│  Job A  │───────────▶│  Job B  │───────────▶│  Job C  │
│ 1 call  │            │ 1 call  │            │ 1 call  │
└─────────┘            └─────────┘            └─────────┘
     │
     │ on_failure
     ▼
┌─────────┐
│  Job D  │
│ (error) │
└─────────┘
```

## Triggers

Todos suportados, qualquer um pode disparar o job:

| Tipo | Descrição | Exemplo |
|---|---|---|
| `cron` | Horário fixo (cron expression) | `0 9 * * 1-5` (seg-sex 9h) |
| `interval` | A cada X tempo | `every: 2h` |
| `event` | Evento emitido por outro job | `listen: "tickets.fetched"` |
| `hotkey` | Atalho de teclado | `keys: "Ctrl+Shift+J"` |
| `manual` | Sempre disponível via UI/chat | — |
| `webhook` | HTTP externo (v2) | `path: "/jobs/xyz/trigger"` |

## Schema YAML

```yaml
# ~/.assistente/jobs/fetch-jira-tickets.yaml

# ── Identidade ────────────────────────────
id: fetch-jira-tickets
name: "Buscar tickets FSD"
description: "Consulta tickets abertos do time no Jira"
enabled: true
pipeline: morning-routine   # agrupamento lógico (opcional)
tags: ["jira", "daily"]

# ── Triggers (qualquer um dispara) ────────
triggers:
  - type: cron
    expression: "0 9 * * 1-5"
  - type: hotkey
    keys: "Ctrl+Shift+J"
  - type: event
    listen: "morning.start"
  - type: manual

# ── Tool Call ─────────────────────────────
tool: mcp.atlassian.searchJiraIssuesUsingJql

inputs:
  # Valores fixos
  cloudId: "c43390d3-e5f8-43ca-9eec-c382a5220bd9"
  jql: "project = FSD AND status != Done"
  maxResults: 50
  fields: ["summary", "status", "priority"]
  # Valores dinâmicos (extraídos do evento que disparou)
  # maxResults: "{{ event.limit }}"

# ── Output ────────────────────────────────
output:
  # Schema esperado (validação + documentação)
  schema:
    type: object
    properties:
      issues:
        type: array
        description: "Tickets retornados"
      total:
        type: number
        description: "Total de resultados"

  # Mapeamento — renomeia/transforma pro payload do evento
  map:
    tickets: "{{ output.issues }}"
    count: "{{ output.total }}"
    keys: "{{ output.issues | pluck('key') }}"
    has_critical: "{{ output.issues | any('fields.priority.name', 'Critical') }}"

# ── Eventos emitidos ──────────────────────
events:
  on_success: "tickets.fetched"
  on_failure: "tickets.fetch_failed"
  # Filtrar payload (default = output.map completo)
  payload_filter:
    include: ["tickets", "count"]

# ── Error Policy ──────────────────────────
error_policy:
  strategy: retry        # retry | stop | skip
  max_retries: 3
  retry_delay: 30s
  backoff: exponential   # linear | exponential | fixed
  on_exhausted: notify   # notify | ignore
  notify_channels: ["chat", "telegram"]

# ── Dry Run ───────────────────────────────
dry_run:
  enabled: false
  mock_output:
    issues:
      - key: "FSD-999"
        fields:
          summary: "[DRY RUN] Ticket de teste"
          status: { name: "Open" }
          priority: { name: "Medium" }
    total: 1

# ── Metadata ──────────────────────────────
metadata:
  created_at: "2026-03-20T10:15:00Z"
  created_by: chat
```

## Data Flow — Inputs e Outputs

### Input Mapping

Cada campo aceita valor fixo ou template `{{ }}`. Detecção automática:

- `{{ event.* }}` → variável extraída do evento que disparou o job
- `{{ secrets.* }}` → referência a secret (nunca inline)
- Sem `{{ }}` → valor fixo

```yaml
inputs:
  cloudId: "c43390d3-e5f8-43ca-9eec-c382a5220bd9"  # fixo
  maxResults: "{{ event.limit }}"                     # do evento
  token: "{{ secrets.JIRA_TOKEN }}"                   # secret
```

### Output Map

Renomeia/transforma campos do output da tool call pro payload do evento:

```yaml
output:
  map:
    tickets: "{{ output.issues }}"
    count: "{{ output.total }}"
    keys: "{{ output.issues | pluck('key') }}"
```

O payload do evento emitido será os campos mapeados. Sem map, vai o output completo.

### Autocomplete no Encadeamento

Quando um job escuta um evento, o builder sabe qual job emite esse evento e oferece autocomplete dos campos disponíveis no payload.

## Exemplo: Pipeline Completa

### Job 1 — Buscar tickets

```yaml
id: fetch-jira-tickets
tool: mcp.atlassian.searchJiraIssuesUsingJql
triggers:
  - type: cron
    expression: "0 9 * * 1-5"
inputs:
  cloudId: "c43390d3-e5f8-43ca-9eec-c382a5220bd9"
  jql: "project = FSD AND status != Done"
  maxResults: 50
output:
  map:
    tickets: "{{ output.issues }}"
    count: "{{ output.total }}"
events:
  on_success: "tickets.fetched"
  on_failure: "tickets.fetch_failed"
```

### Job 2 — Atualizar tasklist

```yaml
id: update-tasklist
tool: internal.writeFile
triggers:
  - type: event
    listen: "tickets.fetched"
inputs:
  path: "~/.assistente/tasklists/jira-fsd.md"
  content: |
    # Tickets FSD - {{ now | date('DD/MM/YYYY') }}
    Total: {{ event.count }}
    {% for t in event.tickets %}
    - [ ] [{{ t.key }}] {{ t.fields.summary }} ({{ t.fields.status.name }})
    {% endfor %}
events:
  on_success: "tasklist.updated"
  on_failure: "tasklist.update_failed"
```

### Job 3 — Notificar

```yaml
id: notify-tasklist-ready
tool: internal.notify
triggers:
  - type: event
    listen: "tasklist.updated"
inputs:
  channels: ["chat", "telegram"]
  message: "✅ Tasklist atualizada: {{ event.count }} tickets"
events:
  on_success: "morning.jira.done"
```

### Job 4 — Reagir a erro

```yaml
id: handle-jira-failure
tool: internal.notify
triggers:
  - type: event
    listen: "tickets.fetch_failed"
inputs:
  channels: ["telegram"]
  message: "❌ Falha ao buscar tickets: {{ event.error }}"
```

### Fluxo visual

```
[cron 9h]
    │
    ▼
┌──────────────────┐  tickets.fetched   ┌──────────────────┐  tasklist.updated  ┌──────────────────┐
│ fetch-jira       │──────────────────▶│ update-tasklist   │──────────────────▶│ notify-ready     │
└────────┬─────────┘                   └──────────────────┘                   └──────────────────┘
         │ tickets.fetch_failed
         ▼
┌──────────────────┐
│ handle-failure   │
└──────────────────┘
```

## Job Builder (UI)

O Builder é um **form generator dinâmico** que monta a interface automaticamente a partir dos input schemas das tools MCP/internas.

### Fluxo

1. **Escolher tool** — catálogo pesquisável com todas as tools disponíveis
2. **Preencher inputs** — formulário gerado do schema, cada campo aceita fixo ou `{{ template }}`
3. **Dry run** — executa com valores fixos, mostra output real
4. **Explorar output** — árvore navegável com click-to-copy path (`{{ output.campo }}`)
5. **Mapear output** — definir aliases e transformações
6. **Configurar evento** — nome do evento, payload filter
7. **Configurar trigger** — cron, interval, event, hotkey, manual
8. **Error policy** — strategy, retries, notificação
9. **Salvar** — gera YAML em `~/.assistente/jobs/`

### Schema → Form (geração automática)

| Schema Type | Componente UI |
|---|---|
| `string` | Text input |
| `string` + `enum` | Select / dropdown |
| `number` | Number input |
| `boolean` | Toggle / checkbox |
| `array` de `string` | Tag input (chips) |
| `object` | Grupo colapsável de subcampos |

### Output Explorer

Após dry run, output vira árvore navegável:

```
▼ output
  ▼ issues                      array (42 items)
    ▼ [0]
      ├─ key: "FSD-123"         📋 {{ output.issues[0].key }}
      ▼ fields
        ├─ summary: "Fix bug"   📋 {{ output.issues[0].fields.summary }}
        ▼ status
          └─ name: "Open"       📋 {{ output.issues[0].fields.status.name }}
  ├─ total: 42                  📋 {{ output.total }}
```

Clicar no 📋 copia o path como template para usar em inputs de outros jobs.

### Autocomplete de Eventos

Ao criar Job B que escuta evento de Job A, os campos do payload aparecem como sugestões:

```
{{ event.   ← cursor aqui
  ┌──────────────────────┐
  │ event.tickets  array │
  │ event.count    number│
  │ event.keys     array │
  └──────────────────────┘
```

## Skill: Job Builder (Chat)

O LLM gerencia jobs usando ferramentas de filesystem que já existem — sem tools especializadas:

| Ação | Como |
|---|---|
| Listar jobs | `list_directory ~/.assistente/jobs/` |
| Ver job | `read_file ~/.assistente/jobs/<id>.yaml` |
| Criar job | `write_file ~/.assistente/jobs/<id>.yaml` |
| Editar job | `edit_file ~/.assistente/jobs/<id>.yaml` |
| Deletar job | `delete_file ~/.assistente/jobs/<id>.yaml` |
| Ativar/desativar | `edit_file` → `enabled: true/false` |
| Ver logs | `read_file ~/.assistente/jobs/runs/<id>/*.json` |
| Ver eventos | `read_file ~/.assistente/jobs/events/<date>.jsonl` |

O app monitora a pasta com file watcher — qualquer alteração é validada e assume efeito imediato (hot reload). YAML inválido é ignorado e logado.

## Tool Catalog

O app gera automaticamente `~/.assistente/jobs/catalog.yaml` (read-only) com todas as tools disponíveis e seus schemas de input/output. Atualizado quando MCP servers conectam/desconectam.

O LLM e o Builder UI consultam o catálogo para:
- Sugerir a tool certa para o caso de uso
- Gerar formulários dinâmicos
- Validar inputs

## Pipelines (agrupamento)

Pipeline não é uma entidade separada — é o campo `pipeline` no YAML que agrupa jobs visualmente:

```
📋 morning-routine
├── fetch-jira-tickets      ⏰ cron 9h           ✅ last: 09:00:02
├── update-tasklist          📡 tickets.fetched   ✅ last: 09:00:04
├── notify-tasklist-ready    📡 tasklist.updated   ✅ last: 09:00:05
└── handle-jira-failure      📡 tickets.fetch_failed  💤 never
```

## Event Log

Timeline persistida em JSONL (append-only, um arquivo por dia):

```
~/.assistente/jobs/events/2026-03-20.jsonl
```

```
09:00:00 ⏰ [cron] → fetch-jira-tickets TRIGGERED
09:00:02 ✅ [fetch-jira-tickets] → emitted "tickets.fetched" (42 tickets)
09:00:02 📡 [update-tasklist] ← received "tickets.fetched"
09:00:04 ✅ [update-tasklist] → emitted "tasklist.updated"
09:00:05 ✅ [notify-tasklist-ready] → emitted "morning.jira.done"
```

## Run Logs

Cada execução gera um JSON em `~/.assistente/jobs/runs/<jobId>/`:

```json
{
  "runId": "run_abc123",
  "jobId": "fetch-jira-tickets",
  "trigger": { "type": "cron", "at": "2026-03-20T09:00:00Z" },
  "status": "completed",
  "startedAt": "2026-03-20T09:00:01Z",
  "completedAt": "2026-03-20T09:00:02Z",
  "duration": "1.2s",
  "outputSize": 4523,
  "eventsEmitted": ["tickets.fetched"]
}
```

## Segurança

| Concern | Abordagem |
|---|---|
| Secrets | `{{ secrets.KEY }}` — nunca inline no YAML |
| Loop infinito | Circuit breaker: max chain depth + max runs/hora por job |
| Tools destrutivas | Mesmas restrições de security do Assistente |
| Webhooks (v2) | Rate limiting + auth token |

## Filesystem

```
~/.assistente/
  jobs/
    fetch-jira-tickets.yaml
    update-tasklist.yaml
    notify-tasklist-ready.yaml
    handle-jira-failure.yaml
    catalog.yaml                  ← auto-gerado (read-only)
    runs/
      fetch-jira-tickets/
        2026-03-20T09-00-00.json
      update-tasklist/
        2026-03-20T09-00-02.json
    events/
      2026-03-20.jsonl
```

## Arquitetura (App Side)

```
File Watcher (jobs/*.yaml)
    │
    ▼
YAML Parser + Schema Validator
    │
    ├─ válido → Job Registry → Scheduler (agenda triggers)
    └─ inválido → ignora + loga erro
    
Trigger Layer (cron, interval, hotkey, event, manual)
    │
    ▼
Job Executor (executa a tool call)
    │
    ▼
Event Bus (emit evento → listeners disparam outros jobs)
    │
    ▼
Run Logger + Event Log + Notification (chat/telegram/signal)
```

## Roadmap

### v1 — Core Engine
- Job YAML schema + parser + validação
- Triggers: cron, interval, manual, hotkey, event
- Execution engine (1 job = 1 tool call)
- Event bus + encadeamento
- Input mapping com templates
- Output schema + map
- Error policy configurável
- Run logging + Event log
- Dry run
- Notificação: chat + telegram/signal
- Skill job-builder (criação via chat)
- UI: lista, toggle, run manual, logs, event timeline
- Pipelines (agrupamento visual)
- Tool catalog (auto-gerado)
- Job Builder UI (form dinâmico + output explorer + autocomplete)

### v2 — Expansão
- Trigger: webhook externo
- Trigger: event join (esperar múltiplos eventos)
- Internal tools: condition (branching), transform, delay
- Import/export de jobs
- Templates pré-montados (morning routine, triage, etc.)

### v3 — LLM & Background
- LLM como tool type (summarize, classify, extract, freeform)
- Background daemon (roda com app fechado)
- Dashboard visual de pipelines (grafo de eventos)
- Métricas (runs/dia, success rate, duração média)
