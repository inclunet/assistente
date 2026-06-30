# AEP-0054 — Split Servidor/Cliente + Resource API + Agent Local

**Status**: 📝 Draft  
**Criado em**: 2026-04-29  
**Depende de**: AEP-0052 (Multi-User Accounts), AEP-0040 (Backend-Driven Messaging), AEP-0042 (Chat Surface Context), AEP-0018 (HTTP Unified Client), AEP-0019 (HTTP Client Centralization), AEP-0034 (Unified Workspace)  
**Relacionado**: AEP-0045 (CLI Interface)

---

## Resumo

Introduzir uma arquitetura **API-first** que funcione de forma idêntica no modo local e no modo gerenciado, separando o produto em três papéis:

- **Servidor do Assistente** (local ou remoto): fonte de verdade para recursos persistentes (DB) e executor do loop agentic (LLM + tool-calling).
- **Cliente Desktop (Wails)**: UI principal do desktop.
- **Agent Local** (no desktop): executor de ferramentas que dependem do ambiente local (filesystem, terminal, integração com editor), acionado pelo servidor via RPC.

Este split habilita três modos de distribuição/deploy:

1. **All-in-one local (desktop offline)**: o executável Wails sobe um servidor HTTP local no mesmo processo; a UI consome tudo via HTTP.
2. **Server-only (container/Kubernetes)**: binário headless que sobe apenas a Resource API + streaming + auth; sem frontend embutido.
3. **Thin client (desktop leve)**: cliente Wails + Agent Local conectando a um servidor remoto; sem stack pesada de backend/DB/LLM no cliente.

> Fora de escopo (mas explicitamente previsto em PR separado): **cliente web + workspace mirroring via agent** (AEP-0055 planejada).

---

## Motivação

### 1) Evoluir para ambiente gerenciado sem bifurcar produto
No estado atual (Wails bridge), o frontend e o backend ficam muito acoplados ao ambiente desktop. Um contrato HTTP estável permite:
- rodar em container/Kubernetes com multi-cliente;
- alternar entre auth local e OIDC/IdP (AEP-0052);
- manter o desktop funcionando offline (all-in-one) sem retrabalho.

### 2) Centralizar dados e execução do loop agentic no servidor
Tudo que é DB-driven (conversas, providers, tasklists, jobs, skills, profiles, configs) deve poder rodar no servidor, local ou remoto.

### 3) Manter ferramentas locais locais
Operações de filesystem/terminal e integração com o ambiente do usuário não devem migrar para o servidor. Elas rodam via **Agent Local**.

---

## Decisões

### D1. O servidor é a fonte de verdade para recursos persistentes
Recursos persistentes e DB-driven são de responsabilidade do servidor:
- Conversas/mensagens (AEP-0040)
- Providers e credenciais do servidor
- Tasklists/jobs
- Skills/profiles
- MCP (conforme política de execução)

O cliente apenas renderiza estado e envia comandos.

### D2. O loop agentic roda no servidor
O servidor executa:
- montagem de contexto e threads (AEP-0040/AEP-0010)
- streaming de resposta
- planejamento de tool-calls
- roteamento de execução de tools (server-tools vs agent-tools)

### D3. O Agent Local executa tools que dependem do ambiente do usuário
O Agent Local existe para executar ferramentas que requerem:
- acesso ao filesystem do usuário
- acesso ao terminal/shell local
- contexto de editor/arquivos abertos

O agent é acionado pelo servidor via **Tool RPC**.

### D4. Workspaces do desktop permanecem arquivos locais
O “workspace UI” no desktop continua sendo persistido em `.assistente/workspace.yaml` e índice em `~/.assistente/workspaces/index.yaml` (ver `internal/workspace/*`).

O servidor **não assume** o filesystem do usuário.

### D5. Resource API (HTTP) é o contrato primário entre clientes e servidor
O servidor expõe uma **Resource API** para:
- autenticação/sessões (AEP-0052)
- CRUD de recursos (conversas, providers, tasklists, jobs, profiles, skills, MCP, etc.)
- streaming/eventos (WS ou SSE)

No modo all-in-one local, esta mesma API roda em `127.0.0.1`.

### D6. Auth e sessão seguem a AEP-0052
A Resource API deve suportar:
- `auth.mode=local`: JWT access + refresh (rotação) + roles simples
- `auth.mode=external`: resource server validando JWT via JWKS

O estado `VaultLocked` também é do servidor (AEP-0052): endpoints mínimos do cofre não dependem de login.

### D7. Surface Context segue a AEP-0042
O envio de chat deve usar o contrato:
- `surfaceStateJson` (espelho de `WorkspaceTab.state`)
- `surfaceContextJson` (contexto transitório)

Isso é essencial para o split, pois a UI precisa enviar contexto de editor/terminal de forma estruturada.

### D8. Tool RPC (servidor → agent) é bidirecional e correlacionado
O Tool RPC deve suportar:
- request/response tipados
- correlação por `turnId` e `toolCallId`
- timeout e cancelamento

### D9. Roteamento de execução de tools
Cada tool é classificada em runtime:
- **Server-tool**: executa no servidor (ex.: operações DB-driven)
- **Agent-tool**: executa no agent local (filesystem/terminal)
- **Hybrid**: política decide (ex.: HTTP request dependendo de credenciais/policy)

---

## Fases

### Fase 0 — Contratos (documentação + interfaces)
- Definir endpoints mínimos da Resource API e do streaming.
- Definir envelope e correlação do Tool RPC.

### Fase 1 — All-in-one local API-first
- Subir servidor HTTP local no desktop e fazer o frontend consumir chat via HTTP.
- Manter bridge Wails apenas para bootstrap mínimo (descobrir `baseURL`, etc.).

### Fase 2 — Agent Local + delegação de tools
- Implementar Tool RPC e roteamento server/agent.
- POC: tool de filesystem executada no agent e resultado retornando ao servidor.

### Fase 3 — Server-only (container)
- Extrair entrypoint headless com Resource API + streaming + auth.
- Tornar persistência configurável para ambientes gerenciados.

### Fase 4 — Thin client
- Cliente desktop conecta a servidor remoto.
- Agent Local permanece executando tools locais (quando permitido).

---

## Riscos

- **CORS/origin no desktop**: Wails UI não é mesma origem do `127.0.0.1`; é necessário CORS restritivo e explícito.
- **TLS na LAN**: HTTPS obrigatório fora de localhost (AEP-0052). Certificados/keys e UX precisam ser bem definidos.
- **Segurança do Tool RPC**: tool results são dados sensíveis; canal precisa ser autenticado e autorizado por `user_id`/`sid`.
- **Latência**: loop no servidor + tool no agent pode aumentar latência; precisa de timeouts e UX de progresso.
- **Compatibilidade**: manter modo local atual sem regressão durante transição.

---

## Critérios de aceitação

1. **Modo all-in-one**: o desktop sobe servidor local e a UI consome chat via HTTP (sem usar bridge para chat).
2. **Streaming**: resposta de chat chega em streaming via WS/SSE.
3. **Tool delegation**: uma tool de filesystem executa no Agent Local por solicitação do servidor e o resultado aparece no chat.
4. **Auth alinhado**: Resource API usa JWT/refresh (local) ou valida JWT (external) conforme AEP-0052.
5. **Workspace local preservado**: `.assistente/workspace.yaml` continua sendo a fonte persistida no desktop.
