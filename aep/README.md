# AEP — Assistente Enhancement Proposals

Propostas de melhoria para o Assistente, inspiradas nos PEPs (Python) e RFCs.

Este diretório é o **repositório único** de decisões arquiteturais do projeto
(ver `CLAUDE.md`). Não criar outro diretório para AEPs — tudo fica em `aep/`.

> **Inventário (2026-08-23):** este índice contém **99 documentos principais
> para 98 números ocupados**. A diferença é a colisão histórica 0074, representada
> temporariamente como 0074-A e 0074-B até sua renumeração. Séries multi-arquivo
> têm o principal listado na tabela e os demais em
> [Apêndices por AEP](#apêndices-por-aep). Convenção em
> [Convenção de numeração e anexos](#convenção-de-numeração-e-anexos).

## Índice

| AEP | Título | Status |
|-----|--------|--------|
| [0001](0001-jobs-event-driven-automation.md) | Jobs — Event-Driven Automation | 🚧 In Progress |
| [0002](0002-tool-calling.md) | Plano de Implementação — Tool Calling | ✅ Done |
| [0003](0003-chat-tabs.md) | Sistema de Abas de Chat | 🗄️ Superseded |
| [0004](0004-chat-refactor.md) | Refatoração do Chat (Svelte) | 🗄️ Superseded |
| [0005](0005-chat-refactor-v2.md) | Refatoração do Chat v2 | 🗄️ Superseded |
| [0006](0006-chat-architecture-fix.md) | Fix Arquitetura de Chat | 🗄️ Superseded |
| [0007](0007-chat-tabs-isolation.md) | Isolamento de Abas e Conversas | 🗄️ Superseded |
| [0008](0008-tab-management-backend.md) | Gerenciamento de Abas no Backend | 🗄️ Superseded |
| [0009](0009-chat-component-refactoring.md) | Componentização do Chat | 🗄️ Superseded |
| [0010](0010-streaming-architecture.md) | Arquitetura de Streaming | 🗄️ Superseded |
| [0011](0011-thread-hierarchy.md) | Thread Hierarchy v2 | ✅ Done |
| [0012](0012-llm-provider-manager.md) | Multi-Provider LLM Architecture | 🚧 In Progress |
| [0013](0013-llm-refactor.md) | LLM Client Refactor | 🗄️ Superseded |
| [0014](0014-credential-persistence.md) | Persistência de Credenciais | ✅ Done |
| [0015](0015-provider-auto-credential.md) | Auto-extração de Credenciais | ✅ Done |
| [0016](0016-http-request-tool.md) | HTTP Request Tool | ✅ Done |
| [0017](0017-http-request-security.md) | HTTP Request Security | ✅ Done |
| [0018](0018-http-unified-client.md) | Cliente HTTP Unificado | 🚧 In Progress |
| [0019](0019-http-client-centralization.md) | Centralização do Cliente HTTP | 🚧 In Progress |
| [0020](0020-mcp-implementation.md) | Implementação do Model Context Protocol (MCP) — núcleo e extensões | 🚧 In Progress |
| [0021](0021-mcp-native-mode.md) | MCP Modo Nativo (revisão v7) | 🚧 In Progress |
| [0022](0022-welcome-wizard.md) | Welcome Wizard | ✅ Done |
| [0023](0023-deep-links.md) | Deep Links (assistente://) | ✅ Done |
| [0024](0024-speech-architecture.md) | Arquitetura de Voz (TTS/STT) | 🚧 In Progress |
| [0025](0025-interaction-profiles.md) | Perfis de Interação por Voz | ✅ Done |
| [0026](0026-credential-fixes.md) | Correções no Sistema de Credenciais | ✅ Done |
| [0027](0027-profiles-refactor.md) | Refatoração ProfilesPage | 🚧 In Progress |
| [0028](0028-componentization.md) | Componentização Frontend | 🚧 In Progress |
| [0029](0029-auto-update.md) | Sistema de Auto-Update | 🚧 In Progress |
| [0030](0030-email-system.md) | Sistema de Email | 📋 Open |
| [0031](0031-email-refinements-security.md) | Email + Chat Security | 📋 Open |
| [0032](0032-editor-rico.md) | Editor Rico + Inline Chat | 🚧 In Progress |
| [0033](0033-mcp-oauth-autodiscovery.md) | MCP OAuth Auto-Discovery | 🚧 In Progress |
| [0034](0034-unified-workspace.md) | Unified Workspace | ✅ Done |
| [0035](0035-split-view.md) | Split View | 📝 Draft |
| [0036](0036-plan-tasklistmanager.md) | Task List Manager Feature | 🚧 In Progress |
| [0037](0037-sdk-migration-chat-provider.md) | SDK Migration + ChatProvider Interface | 🚧 In Progress |
| [0038](0038-voice-model-refactor.md) | Refatoração do Modelo de Voz (por Role) | ✅ Done |
| [0039](0039-tool-calling-revamp.md) | Tool Calling — Revamp & Enhancements | 🚧 In Progress |
| [0040](0040-backend-driven-messaging.md) | Backend-Driven Messaging | ✔️ Accepted |
| [0041](0041-proactive-tts.md) | TTS Proativo (Backend-Driven) | ✅ Done |
| [0042](0042-chat-surface-context.md) | Chat Surface Context | 🚧 In Progress |
| [0043](0043-tts-stt-voices.md) | Evolução TTS/STT: Vozes (Assistant + User) | 🗄️ Superseded |
| [0044](0044-profile-settings-revamp.md) | Profile Settings Revamp (Tabbed Panels) | 🚧 In Progress |
| [0045](0045-cli-interface.md) | Interface CLI como alternativa ao Wails | 🚧 In Progress |
| [0046](0046-uuid-migration.md) | Migração de IDs Sequenciais para UUIDv7 | ✅ Done |
| [0047](0047-import-export.md) | Importação e Exportação de Conteúdo | 🚧 In Progress |
| [0048](0048-jobs-database-migration.md) | Migração de Jobs para Banco de Dados | ✅ Done |
| [0049](0049-mcp-database-migration.md) | Migração de MCP Servers para Banco de Dados | 🚧 In Progress |
| [0050](0050-profiles-database-migration.md) | Migração de Profiles para Banco de Dados (adiada) | 📝 Draft |
| [0051](0051-skills-database-migration.md) | Migração de Skills para Banco de Dados | 📝 Draft |
| [0052](0052-multi-user-accounts.md) | Sistema de Contas de Usuário | 🚧 In Progress |
| [0053](0053-mcp-graceful-degradation.md) | Degradação graciosa de MCP nativo no chat | 📝 Draft |
| [0056](0056-workspace-self-contained-tabs.md) | Workspace com Abas Autocontidas | ✅ Done |
| [0057](0057-chat-session-identity.md) | Sessões de Superfície e Timeline de Chat | ✅ Done |
| [0058](0058-global-accessibility-voice-arbitration.md) | Arbitragem Global de Acessibilidade e Voz | ✅ Done |
| [0059](0059-long-conversation-performance.md) | Performance de Conversas Longas | 🚧 In Progress |
| [0060](0060-command-policy-parser.md) | Parser e Política de Comandos | ✅ Done |
| [0061](0061-credential-loss-incident-and-defenses.md) | Incidente de Perda de Credenciais e Defesas | ✔️ Accepted |
| [0062](0062-profile-application-and-local-provider-auth.md) | Aplicação de Perfil e Auth de Provider Local | ✅ Done |
| [0063](0063-tool-invocations-and-common-executor.md) | Tool Invocations e Executor Comum | ✅ Done |
| [0064](0064-streaming-recovery-explicito.md) | Recuperação explícita de resposta interrompida (continuação) e cancelamento de geração | ✅ Done |
| [0065](0065-llm-rate-limiting.md) | Rate Limiting nas Chamadas ao Provedor LLM | ✅ Done |
| [0066](0066-connection-status-indicator.md) | Indicador de Status de Conexão com a API LLM | ✅ Done |
| [0067](0067-tasklist-domain-events-and-custom-actions.md) | Eventos de Domínio de Tasklists e Custom Actions | 🚧 In Progress |
| [0068](0068-subagentes-segundo-plano.md) | Sub-agentes em segundo plano (tool de sub-conversas) | ✅ Done |
| [0069](0069-feed-read-tool.md) | Tool `feed_read` (RSS/Atom/JSON Feed/Podcast → JSON canônico) | ✅ Done |
| [0070](0070-web-search-tool.md) | Tool `web_search` (busca web → JSON canônico paginável) | ✅ Done |
| [0071](0071-structured-tool-output-size-policy.md) | Política canônica de tamanho para saídas estruturadas | ✅ Done |
| [0072](0072-skill-catalog-and-loading.md) | Skill Loading Runtime | ✅ Done |
| [0073](0073-tasklist-conversation-linking.md) | Vínculo de Tasks e Tasklists a Conversas | ✅ Done |
| [0074-A](0074-prompt-cache-e-contexto-dinamico.md) | Prompt Cache, Custo de LLM e Layout da Request ⚠️ | 🚧 In Progress |
| [0074-B](0074-database-compaction-and-retention.md) | Compactação e Retenção do Banco de Dados ⚠️ | ✅ Done |
| [0075](0075-context-providers.md) | Context Providers | ✅ Done |
| [0076](0076-schema-versioning-migrations.md) | Versionamento de Schema do Banco (schema_migrations) | ✅ Done |
| [0077](0077-tool-planner-and-tools-subsystem-evolution.md) | ToolPlanner e Evolução do Subsistema de Tools | ✅ Done |
| [0078](0078-deprecacao-toolcalls-em-mensagens.md) | Deprecação de `tool_calls` em Mensagens | ✅ Done |
| [0079](0079-editor-modo-apresentacao-reveal.md) | Modo Apresentação Reveal.js no Editor | ✅ Done |
| [0080](0080-surface-context-unificado.md) | SurfaceContext Unificado | 🚧 In Progress |
| [0081](0081-politica-tools-por-perfil-e-carregamento-sob-demanda.md) | Política de Tools por Perfil e Carregamento sob Demanda | 🚧 In Progress |
| [0082](0082-network-trust-allowlist.md) | Network Trust Allowlist | ✅ Done |
| [0083](0083-channels-database-migration.md) | Migração de Canais e Contatos para Banco de Dados | ✅ Done |
| [0084](0084-agentes-acp-como-providers.md) | Agentes de código ACP como providers LLM | ✅ Done |
| [0085](0085-i18n-de-dialogos-do-questionnaire.md) | i18n dos diálogos que o backend manda para a tela (questionnaire) | ✅ Done |
| [0086](0086-registro-acp-descoberta-e-instalacao-de-agentes.md) | Descoberta e instalação de agentes pelo registro ACP | ✅ Done |
| [0087](0087-tela-de-erro-acessivel-e-diagnosticavel.md) | Tela de erro acessível e diagnosticável | 🚧 In Progress |
| [0088](0088-strangler-fig-borda-wails-app.md) | Concluir migração Strangler Fig da borda Wails (`App`) | ✅ Done |
| [0089](0089-terminais-como-recursos-efemeros.md) | Terminais como recursos efêmeros | 🚧 In Progress |
| [0090](0090-ordem-botoes-dialogos.md) | Ordem de botões em diálogos (confirmação antes de cancelar) | ✅ Done |
| [0091](0091-dialogos-de-decisao-unificados.md) | Diálogos de decisão unificados (estilo Windows + NVDA) | ✅ Done |
| [0092](0092-filesystem-path-trust-allowlist.md) | Allowlist escopável de paths fora do sandbox (filesystem trust) | ✅ Done |
| [0093](0093-leitura-documentos-como-markdown.md) | Leitura unificada de documentos como Markdown (`read_file` / busca) | ✅ Done |
| [0094](0094-navegacao-em-conteudo-renderizado.md) | Navegação em conteúdo renderizado | ✅ Done |
| [0095](0095-mermaid-acessivel-e-resiliente.md) | Mermaid acessível e resiliente | ✅ Done |
| [0096](0096-baseline-operacional-de-tools-por-perfil.md) | Baseline operacional de tools por perfil | ✅ Done |
| [0097](0097-capabilities-de-protocolo-por-provedor.md) | Capabilities de protocolo configuráveis por provedor | ✅ Done |
| [0098](0098-limite-de-saida-e-tool-calls-truncadas.md) | Limite de saída e tool calls truncadas | ✅ Done |
| [0099](0099-patch-canonico-multi-hunk.md) | Patch canônico multi-hunk | ✅ Done |
| [0100](0100-progresso-unificado-por-conversa.md) | Progresso unificado por conversa | ✅ Done |

> **Números livres:** 0054 e 0055 estão vagos (lacunas). Novos AEPs devem ser
> numerados sequencialmente a partir do **maior número existente** (0100 → próximo
> 0101), salvo decisão explícita de reaproveitar uma lacuna.

## Status Legend

- 📝 **Draft** — em discussão/design (inclui status "Proposto" e "Rascunho")
- 📋 **Open** — aprovada, aguardando implementação
- 🚧 **In Progress** — sendo implementada
- ✅ **Done** — implementada (inclui "Concluído"/"Implementado")
- ✔️ **Accepted** — aceita como contrato/decisão vigente, mesmo sem 100%
  implementada. `Accepted` é o status canônico; `Aceito` é apenas alias
  histórico de leitura e não deve ser usado no topo de documentos novos.
- 🗄️ **Superseded** — substituída/obsoleta (mantida como registro histórico)
- ❌ **Deprecated** — desprezada/cancelada

O status descreve o estado do **escopo aceito**, não a idade do documento.
Fases explicitamente adiadas para outra issue ou outro AEP não impedem `Done`
quando os critérios de aceitação do escopo entregue estão completos. Toda
mudança de status precisa estar apoiada por evidências no documento, como
checklists, caminhos de código, testes ou PRs.

Documento principal e índice são atualizados juntos no PR que implementa,
abandona ou substitui o trabalho. Entrega parcial permanece `In Progress` e
deve declarar o que ainda falta; uma decisão vigente que funciona como contrato,
sem exigir implementação integral, usa `Accepted`.

## Convenção de numeração e anexos

Para evitar ambiguidades como as resolvidas pela issue #263:

1. **Um número, um tema.** Cada número de AEP corresponde a **um único tema**. Dois
   documentos que se autodenominam "AEP-NNNN" para temas diferentes é uma colisão
   e deve ser resolvido (renumerar o intruso ou rebaixá-lo a apêndice do tema dono
   do número). A única exceção conhecida é a colisão 0074, ainda pendente e
   explicitamente contabilizada como dois documentos principais sob um número.
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
6. **Status faz parte da entrega.** O PR que alterar o estado de implementação
   atualiza os checklists ou evidências do AEP, o status no topo e esta tabela.

## Apêndices por AEP

Documentos secundários que **pertencem** ao mesmo tema do AEP principal:

- **0012 — Multi-Provider LLM Architecture** (principal: `0012-llm-provider-manager.md`)
  - `0012-llm-provider-token-management.md`
  - `0012-llm-provider-token-ui.md`
  - `0012-llm-provider-test-coverage.md`
  - `0012-llm-provider-phase8-validation.md`
- **0016 — HTTP Request Tool** (principal: `0016-http-request-tool.md`)
  - `0016-http-request-examples.md`
- **0020 — Implementação do Model Context Protocol (MCP) — núcleo e extensões** (principal: `0020-mcp-implementation.md`)
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
as referências no código. Até lá, o índice usa os rótulos 0074-A e 0074-B e conta
ambos como documentos principais: por isso há 99 documentos para 98 números
ocupados.
