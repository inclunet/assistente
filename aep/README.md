# AEP — Assistente Enhancement Proposals

Propostas de melhoria para o Assistente, inspiradas nos PEPs (Python) e RFCs.

## Índice

| AEP | Título | Status |
|-----|--------|--------|
| [0001](0001-jobs-event-driven-automation.md) | Jobs — Event-Driven Automation | 📝 Draft |
| [0002](0002-tool-calling.md) | Tool Calling System | ✅ Done |
| [0003](0003-chat-tabs.md) | Sistema de Abas de Chat | ✅ Done |
| [0004](0004-chat-refactor.md) | Refatoração do Chat | ✅ Done |
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
| [0036](0036%20-%20plan-taskListManager.md) | Task List Manager Feature | ✅ Done |
| [0037](0037-sdk-migration-chat-provider.md) | SDK Migration + ChatProvider Interface | 🚧 In Progress |
| [0042](0042-chat-surface-context.md) | Chat Surface Context | 🚧 In Progress |
| [0043](0043-tts-stt-voices.md) | Evolução TTS/STT: Vozes (Assistant + User) | 📝 Draft |
| [0044](0044-profile-settings-revamp.md) | Profile Settings Revamp (Tabbed Panels) | 📝 Draft |
| [0045](0045-cli-interface.md) | Interface CLI como alternativa ao Wails | ✅ Done |
| [0046](0046-uuid-migration.md) | Migração de IDs Sequenciais para UUIDv7 | ✅ Done |
| [0047](0047-import-export.md) | Importação e Exportação de Conteúdo | ✅ Done |
| [0048](0048-jobs-database-migration.md) | Migração de Jobs para Banco de Dados | 🚧 In Progress |
| [0049](0049-mcp-database-migration.md) | Migração de MCP Servers para Banco de Dados | ✅ Done |
| [0052](0052-multi-user-accounts.md) | Sistema de Contas de Usuário | 🚧 In Progress |
| [0053](0053-mcp-graceful-degradation.md) | Degradação graciosa de MCP nativo no chat | 📝 Draft |
| [0056](0056-workspace-self-contained-tabs.md) | Workspace com Abas Autocontidas | 📝 Draft |
| [0057](0057-chat-session-identity.md) | Sessões de Superfície e Timeline de Chat | 📝 Draft |
| [0058](0058-global-accessibility-voice-arbitration.md) | Arbitragem Global de Acessibilidade e Voz | 📝 Draft |
| [0059](0059-long-conversation-performance.md) | Performance de Conversas Longas | 📝 Draft |
| [0060](0060-command-policy-parser.md) | Parser de Política de Comandos | 📝 Draft |
| [0061](0061-credential-loss-incident-and-defenses.md) | Incidente de Perda de Credenciais e Defesas | 📝 Draft |
| [0062](0062-profile-application-and-local-provider-auth.md) | Aplicação de Perfil e Auth de Provider Local | 📝 Draft |
| [0063](0063-tool-invocations-and-common-executor.md) | Tool Invocations e Executor Comum | 📝 Draft |
| [0064](0064-streaming-recovery-explicito.md) | Recuperação Explícita de Streaming e Cancelamento | 📝 Draft |
| [0065](0065-llm-rate-limiting.md) | Rate Limiting nas Chamadas ao Provedor LLM | 📝 Draft |
| [0066](0066-connection-status-indicator.md) | Indicador de Status de Conexão com a API LLM | 📝 Draft |
| [0067](0067-tasklist-domain-events-and-custom-actions.md) | Eventos de Domínio de Tasklists e Custom Actions | 📝 Draft |
| [0068](0068-subagentes-segundo-plano.md) | Sub-agentes em segundo plano (tool de sub-conversas) | 📝 Draft |
| [0069](0069-feed-read-tool.md) | Tool feed_read (RSS/Atom/JSON Feed/Podcast → JSON canônico) | 📝 Draft |
| [0070](0070-web-search-tool.md) | Tool web_search (busca web → JSON canônico paginável) | 📝 Draft |

## Status Legend

- 📝 **Draft** — em discussão/design
- 📋 **Open** — aprovada, aguardando implementação
- 🚧 **In Progress** — sendo implementada
- ✅ **Done** — implementada
- ❌ **Deprecated** — desprezada/cancelada
