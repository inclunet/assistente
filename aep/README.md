# AEP — Assistente Enhancement Proposals

Propostas de melhoria para o Assistente, inspiradas nos PEPs (Python) e RFCs.

Este diretório é o **repositório único** de decisões arquiteturais do projeto
(ver `CLAUDE.md`). Não criar outro diretório para AEPs — tudo fica em `aep/`.

> **Inventário (2026-06-21):** este índice cobre **todos os documentos `.md` de
> `aep/`** (≈90 arquivos). Cada número de AEP tem **um documento principal**; séries
> multi-arquivo têm o principal listado na tabela e os demais em
> [Apêndices por AEP](#apêndices-por-aep). Convenção em
> [Convenção de numeração e anexos](#convenção-de-numeração-e-anexos).

## Índice

| AEP | Título | Status |
|-----|--------|--------|
| [0001](0001-jobs-event-driven-automation.md) | Jobs — Event-Driven Automation | 📝 Draft |
| [0002](0002-tool-calling.md) | Tool Calling System | ✅ Done |
| [0003](0003-chat-tabs.md) | Sistema de Abas de Chat | ✅ Done |
| [0004](0004-chat-refactor.md) | Refatoração do Chat (Svelte) | 🗄️ Superseded |
| [0005](0005-chat-refactor-v2.md) | Refatoração do Chat v2 | ✅ Done |
| [0006](0006-chat-architecture-fix.md) | Fix Arquitetura de Chat | ✅ Done |
| [0007](0007-chat-tabs-isolation.md) | Isolamento de Abas e Conversas | ✅ Done |
| [0008](0008-tab-management-backend.md) | Gerenciamento de Abas no Backend | ✅ Done |
| [0009](0009-chat-component-refactoring.md) | Componentização do Chat | ✅ Done |
| [0010](0010-streaming-architecture.md) | Arquitetura de Streaming | ✅ Done |
| [0011](0011-thread-hierarchy.md) | Thread Hierarchy v2 | ✅ Done |
| [0012](0012-llm-provider-manager.md) | Multi-Provider LLM Architecture | ✅ Done |
| [0013](0013-llm-refactor.md) | LLM Client Refactor | ✅ Done |
| [0014](0014-credential-persistence.md) | Persistência de Credenciais | ✅ Done |
| [0015](0015-provider-auto-credential.md) | Auto-extração de Credenciais | ✅ Done |
| [0016](0016-http-request-tool.md) | HTTP Request Tool | ✅ Done |
| [0017](0017-http-request-security.md) | HTTP Request Security | ✅ Done |
| [0018](0018-http-unified-client.md) | Cliente HTTP Unificado | ✅ Done |
| [0019](0019-http-client-centralization.md) | Centralização do Cliente HTTP | ✅ Done |
| [0020](0020-mcp-implementation.md) | MCP Complete Implementation | ✅ Done |
| [0021](0021-mcp-native-mode.md) | MCP Modo Nativo (v2) | 🚧 In Progress |
| [0022](0022-welcome-wizard.md) | Welcome Wizard | ✅ Done |
| [0023](0023-deep-links.md) | Deep Links (assistente://) | ✅ Done |
| [0024](0024-speech-architecture.md) | Arquitetura de Voz (TTS/STT) | ✅ Done |
| [0025](0025-interaction-profiles.md) | Perfis de Interação por Voz | ✅ Done |
| [0026](0026-credential-fixes.md) | Correções no Sistema de Credenciais | ✅ Done |
| [0027](0027-profiles-refactor.md) | Refatoração ProfilesPage | ✅ Done |
| [0028](0028-componentization.md) | Componentização Frontend | ✅ Done |
| [0029](0029-auto-update.md) | Sistema de Auto-Update | ✅ Done |
| [0030](0030-email-system.md) | Sistema de Email | 📋 Open |
| [0031](0031-email-refinements-security.md) | Email + Chat Security | 📋 Open |
| [0032](0032-editor-rico.md) | Editor Rico + Inline Chat | 📋 Open |
| [0033](0033-mcp-oauth-autodiscovery.md) | MCP OAuth Auto-Discovery | 📋 Open |
| [0034](0034-unified-workspace.md) | Unified Workspace | ✅ Done |
| [0035](0035-split-view.md) | Split View | 📝 Draft |
| [0036](0036-plan-tasklistmanager.md) | Task List Manager Feature | ✅ Done |
| [0037](0037-sdk-migration-chat-provider.md) | SDK Migration + ChatProvider Interface | ✅ Done |
| [0038](0038-voice-model-refactor.md) | Refatoração do Modelo de Voz (por Role) | 📝 Draft |
| [0039](0039-tool-calling-revamp.md) | Tool Calling — Revamp & Enhancements | 📝 Draft |
| [0040](0040-backend-driven-messaging.md) | Backend-Driven Messaging | ✔️ Accepted |
| [0041](0041-proactive-tts.md) | TTS Proativo (Backend-Driven) | ✅ Done |
| [0042](0042-chat-surface-context.md) | Chat Surface Context | 🚧 In Progress |
| [0043](0043-tts-stt-voices.md) | Evolução TTS/STT: Vozes (Assistant + User) | 📝 Draft |
| [0044](0044-profile-settings-revamp.md) | Profile Settings Revamp (Tabbed Panels) | 📝 Draft |
| [0045](0045-cli-interface.md) | Interface CLI como alternativa ao Wails | ✅ Done |
| [0046](0046-uuid-migration.md) | Migração de IDs Sequenciais para UUIDv7 | ✅ Done |
| [0047](0047-import-export.md) | Importação e Exportação de Conteúdo | ✅ Done |
| [0048](0048-jobs-database-migration.md) | Migração de Jobs para Banco de Dados | 🚧 In Progress |
| [0049](0049-mcp-database-migration.md) | Migração de MCP Servers para Banco de Dados | ✅ Done |
| [0050](0050-profiles-database-migration.md) | Migração de Profiles para Banco de Dados (adiada) | 📝 Draft |
| [0051](0051-skills-database-migration.md) | Migração de Skills para Banco de Dados | 📝 Draft |
| [0052](0052-multi-user-accounts.md) | Sistema de Contas de Usuário | 🚧 In Progress |
| [0053](0053-mcp-graceful-degradation.md) | Degradação graciosa de MCP nativo no chat | 📝 Draft |
| [0056](0056-workspace-self-contained-tabs.md) | Workspace com Abas Autocontidas | 📝 Draft |
| [0057](0057-chat-session-identity.md) | Sessões de Superfície e Timeline de Chat | 📝 Draft |
| [0058](0058-global-accessibility-voice-arbitration.md) | Arbitragem Global de Acessibilidade e Voz | 📝 Draft |
| [0059](0059-long-conversation-performance.md) | Performance de Conversas Longas | 📝 Draft |
| [0060](0060-command-policy-parser.md) | Parser de Política de Comandos | 📝 Draft |
| [0061](0061-credential-loss-incident-and-defenses.md) | Incidente de Perda de Credenciais e Defesas | ✔️ Accepted |
| [0062](0062-profile-application-and-local-provider-auth.md) | Aplicação de Perfil e Auth de Provider Local | ✅ Done |
| [0063](0063-tool-invocations-and-common-executor.md) | Tool Invocations e Executor Comum | ✅ Done |
| [0064](0064-streaming-recovery-explicito.md) | Recuperação Explícita de Streaming e Cancelamento | 📝 Draft |
| [0065](0065-llm-rate-limiting.md) | Rate Limiting nas Chamadas ao Provedor LLM | 📝 Draft |
| [0066](0066-connection-status-indicator.md) | Indicador de Status de Conexão com a API LLM | 📝 Draft |
| [0067](0067-tasklist-domain-events-and-custom-actions.md) | Eventos de Domínio de Tasklists e Custom Actions | 📝 Draft |
| [0068](0068-subagentes-segundo-plano.md) | Sub-agentes em segundo plano (tool de sub-conversas) | 📝 Draft |
| [0069](0069-feed-read-tool.md) | Tool feed_read (RSS/Atom/JSON Feed/Podcast → JSON canônico) | 📝 Draft |
| [0070](0070-web-search-tool.md) | Tool web_search (busca web → JSON canônico paginável) | 📝 Draft |
| [0071](0071-structured-tool-output-size-policy.md) | Política canônica de tamanho para saídas estruturadas | ✅ Done |
| [0072](0072-skill-catalog-and-loading.md) | Skill Catalog & Loading (descoberta, gating, carregamento sob demanda) | 📝 Draft |
| [0073](0073-tasklist-conversation-linking.md) | Vínculo de Tasks e Tasklists a Conversas | ✅ Done |
| [0074-A](0074-prompt-cache-e-contexto-dinamico.md) | Prompt Cache, Custo de LLM e Layout da Request ⚠️ | 📝 Draft |
| [0074-B](0074-database-compaction-and-retention.md) | Compactação e Retenção do Banco de Dados ⚠️ | ✅ Done |
| [0075](0075-context-providers.md) | Context Providers | ✅ Done |
| [0076](0076-schema-versioning-migrations.md) | Versionamento de Schema do Banco (schema_migrations) | ✅ Done |
| [0077](0077-tool-planner-and-tools-subsystem-evolution.md) | ToolPlanner e Evolução do Subsistema de Tools | ✅ Done |
| [0078](0078-deprecacao-toolcalls-em-mensagens.md) | Deprecação de `tool_calls` em Mensagens | 📝 Draft |

> **Números livres:** 0054 e 0055 estão vagos (lacunas). Novos AEPs devem ser
> numerados sequencialmente a partir do **maior número existente** (0078 → próximo
> 0079), salvo decisão explícita de reaproveitar uma lacuna.

## Status Legend

- 📝 **Draft** — em discussão/design (inclui status "Proposto" e "Rascunho")
- 📋 **Open** — aprovada, aguardando implementação
- 🚧 **In Progress** — sendo implementada
- ✅ **Done** — implementada (inclui "Concluído"/"Implementado")
- ✔️ **Accepted** — aceita como contrato/decisão vigente, mesmo sem 100% implementada (inclui o rótulo em português "Aceito", ex.: 0040 e 0061)
- 🗄️ **Superseded** — substituída/obsoleta (mantida como registro histórico)
- ❌ **Deprecated** — desprezada/cancelada

## Convenção de numeração e anexos

Para evitar ambiguidades como as resolvidas pela issue #263:

1. **Um número, um tema.** Cada número de AEP corresponde a **um único tema**. Dois
   documentos que se autodenominam "AEP-NNNN" para temas diferentes é uma colisão
   e deve ser resolvido (renumerar o intruso ou rebaixá-lo a apêndice do tema dono
   do número).
2. **Documento principal vs. apêndices.** Uma série multi-arquivo tem **um documento
   principal** (`NNNN-tema.md`, listado no índice) e **apêndices** nomeados
   `NNNN-tema-<subtópico>.md` (executive-summary, metrics, fases, quick-start,
   token-management, etc.). Apêndices **não** ganham linha própria no índice; ficam
   em [Apêndices por AEP](#apêndices-por-aep).
3. **Sem espaços nem extensões fora de `.md`.** Nomes de arquivo usam apenas
   `kebab-case` minúsculo (`0036-plan-tasklistmanager.md`). Nada de espaços ou `.txt`.
4. **Numeração sequencial.** Novos AEPs seguem o maior número existente. Lacunas
   (0054, 0055) ficam reservadas/documentadas.
5. **Status no topo do documento.** Todo AEP declara o `Status` logo após o título,
   alinhado com a legenda acima e com este índice.

## Apêndices por AEP

Documentos secundários que **pertencem** ao mesmo tema do AEP principal:

- **0012 — Multi-Provider LLM Architecture** (principal: `0012-llm-provider-manager.md`)
  - `0012-llm-provider-token-management.md`
  - `0012-llm-provider-token-ui.md`
  - `0012-llm-provider-test-coverage.md`
  - `0012-llm-provider-phase8-validation.md`
- **0016 — HTTP Request Tool** (principal: `0016-http-request-tool.md`)
  - `0016-http-request-examples.md`
- **0020 — MCP Complete Implementation** (principal: `0020-mcp-implementation.md`)
  - `0020-mcp-improvements-summary.md` (histórico/superseded)
- **0024 — Arquitetura de Voz (TTS/STT)** (principal: `0024-speech-architecture.md`)
  - `0024-speech-system-status.md`
  - `0024-speech-tts-refactor.md` *(ex-"AEP-0028: Speech/TTS Refactor"; renumerado para
    cá pela issue #263 — ver [colisões resolvidas](#colisões-de-numeração))*
- **0028 — Componentização Frontend** (principal: `0028-componentization.md`)
  - `0028-component-architecture.md`
  - `0028-componentization-index.md`
  - `0028-componentization-overview.md` *(ex-`.txt`; convertido pela issue #263)*
  - `0028-componentization-executive-summary.md`
  - `0028-componentization-summary.md`
  - `0028-componentization-fases.md`
  - `0028-componentization-metrics.md`
  - `0028-componentization-quick-reference.md`
  - `0028-componentization-quick-start.md`
- **0029 — Sistema de Auto-Update** (principal: `0029-auto-update.md`)
  - `0029-auto-update-github-actions.md`
  - `0029-auto-update-portable-vs-installed.md`
  - `0029-auto-update-quickstart.md`

## Colisões de numeração

### ✅ Resolvida — "AEP-0028"
Existiam dois temas sob o número 0028: a **série de Componentização Frontend** (dona
do número) e um documento de **Speech/TTS Refactor** que se autodenominava
"AEP-0028". A issue #263 rebaixou o segundo a **apêndice do AEP-0024** (tema de voz
ao qual pertence), renomeando `0028-speech-tts-refactor.md` →
`0024-speech-tts-refactor.md` e atualizando as referências textuais (ex.: AEP-0041).

### ⚠️ Pendente — "AEP-0074"
Existem **dois temas distintos** sob o número 0074:

- `0074-prompt-cache-e-contexto-dinamico.md` (**Prompt Cache** — referenciado como
  "AEP-0074" pelas AEPs 0072 e 0075);
- `0074-database-compaction-and-retention.md` (**Compactação/Retenção do Banco** —
  referenciado como "AEP-0074" por código em `internal/` e por comentários, ex.:
  `internal/database/maintenance.go`).

A renumeração de um deles exige alterar **referências em código fora de `aep/`**, o
que está fora do escopo da issue #263 (apenas governança/docs). A colisão fica
**registrada aqui** e deve ser resolvida em uma issue/PR dedicada que também atualize
as referências no código.
