# Plano: Isolamento de Abas e Conversas (Backend-Driven)

**Status:** Superseded — implementação Svelte substituída pelos contratos de superfícies das AEPs 0056/0057

Este documento define a arquitetura alvo e o plano de migração para separar completamente a gestão de Abas da gestão de Conversas, com fluxo 100% dirigido pelo backend e eventos escopados por conversa. O objetivo é eliminar vazamentos entre abas, estabilizar a reatividade após F5 e simplificar responsabilidades.

## Objetivos
- Isolar completamente Abas e Conversas (estado, eventos, ciclo de vida).
- Garantir que eventos de conversa impactem somente sua aba/conversa.
- Fluxo backend-driven claro: página → abas → conversa → mensagens.
- Remover acoplamentos e side-effects globais na UI.

## Princípios
- Single responsibility por camada (Abas ≠ Conversas).
- Eventos escopados por `conversationId` (e opcionalmente `tabId`).
- Stores imutáveis e específicas por instância (uma por aba).
- Controllers sem estado global; cada aba instancia/destroi os seus.
- Nenhum componente decide “se o evento é meu”: a assinatura já é filtrada.

## Fluxo Backend-Driven
1. Load Page: monta layout e [frontend/src/pages/chat/ChatTabsContainer.svelte](frontend/src/pages/chat/ChatTabsContainer.svelte).
2. Load Tabs: `TabsService.GetTabs()` popula apenas lista e aba ativa.
3. Render Tabs: para cada aba, [frontend/src/pages/chat/ChatTab.svelte](frontend/src/pages/chat/ChatTab.svelte) monta sem side-effects globais.
4. Load Conversation (por aba): `ChatTab` instancia `ConversationController` com stores isoladas; se houver `initialConversationId`, chama `controller.loadConversation(id)`.
5. Load Messages: o `ConversationController` executa `GetConversationInfo(id)` e `GetMessages(id)`, atualiza stores; a UI re-renderiza.
6. Streaming: o controller conecta eventos escopados da conversa e atualiza apenas suas stores.

## Escopo de Eventos
- Padrão de eventos (do backend):
  - Legados: `chat:stream`, `chat:done`, `chat:messages_ready`, `chat:internal_message`.
  - Escopados: os mesmos nomes com sufixo `:<conversationId>`, por exemplo: `chat:stream:123`.
  - Consumo no frontend: controllers de conversa assinam apenas os eventos escopados da sua `conversationId` (preferencial). Eventos legados permanecem por compatibilidade, mas não devem ser usados pelos controllers por aba.
  - Benefício: nenhuma aba precisa filtrar eventos na UI; não há vazamento entre instâncias.

## Estrutura por Aba
- `ConversationStores` (por aba):
  - Primárias: `messages`, `conversationId`, `conversationTitle`, `isStreaming`, `streamingMessageId`, `streamingContent`.
  - Derivadas: `hasConversation`, `isEmpty`, `messageCount`, `threadedMessages` (lazy-sync a partir de `messages`).
- `ConversationController` (por aba):
  - Métodos: `init()`, `bind()`, `loadConversation(id)`, `send({ text, media, params })`, `updateSettings(showInternal)`, `loadChildren(messageId)`, `setAsLastConversation()`, `clear()`, `destroy()`.
  - Responsável por: chamadas Wails (`GetConversationInfo`, `GetMessages`, `SendMessage`, etc.), assinatura de eventos escopados da conversa, atualização imutável das stores.
- `EventRouter` (helper):
  - API: `on(eventName, handler) → unsubscribe`, `off(...)`, `dispose()`, com suporte a registrar nos canais escopados.

## UI Pura (View)
- [frontend/src/pages/chat/Chat.svelte](frontend/src/pages/chat/Chat.svelte):
  - Passa a receber `controller` e `stores` via props (de `ChatTab`).
  - Fica responsável por UI, acessibilidade, mídia e interações; chama `controller.send()`.
  - Remove imports diretos do runtime/backend e bindings globais de eventos.

## Tabs isoladas
- `TabsService`:
  - Onde: `frontend/src/lib/tabs/tabs-service.js`.
  - Métodos: `GetTabs`, `CreateTab`, `CloseTab`, `SetActiveTab`, `LoadConversationInTab` (sem tocar mensagens/estado de conversa).
- [frontend/src/pages/chat/ChatTabsContainer.svelte](frontend/src/pages/chat/ChatTabsContainer.svelte):
  - Usa apenas `TabsService`; gerencia navegação/fechamento/ativação; não escuta `chat:*`.
- [frontend/src/pages/chat/ChatTab.svelte](frontend/src/pages/chat/ChatTab.svelte):
  - Instancia `ConversationController` e `ConversationStores` por aba; injeta no `Chat.svelte`; destrói no unmount.

## Alterações de Arquivos (propostas)
- Novos:
  - `frontend/src/lib/events/event-router.js` (EventRouter por instância)
  - `frontend/src/lib/chat/conversation-stores.js` (stores isoladas por conversa)
  - `frontend/src/lib/chat/conversation-controller.js` (controller por conversa)
  - `frontend/src/lib/tabs/tabs-service.js` (serviço de abas)
- Refactors:
  - [frontend/src/pages/chat/Chat.svelte](frontend/src/pages/chat/Chat.svelte): view pura consumindo `controller`/`stores` via props.
  - [frontend/src/pages/chat/ChatTab.svelte](frontend/src/pages/chat/ChatTab.svelte): criação/limpeza de controller/stores por aba.
  - [frontend/src/pages/chat/ChatTabsContainer.svelte](frontend/src/pages/chat/ChatTabsContainer.svelte): usar `TabsService` e nenhum evento de conversa.
  - [frontend/src/lib/chat/message-service.js](frontend/src/lib/chat/message-service.js): migrar/limpar o que for de estado para o controller; manter utilitários em `utils.js`.

## Migração Incremental
1. Adicionar `EventRouter` + `TabsService` (sem alterar UI).
2. Criar `ConversationStores` + `ConversationController` mantendo a interface do atual controller para uma aba “piloto”.
3. Adaptar `ChatTab` para injetar `controller/stores` no `Chat.svelte` da aba piloto.
4. Refatorar `Chat.svelte` para view pura na aba piloto (sem imports de runtime/backend); validar fluxo (F5, envio, streaming).
5. Estender para todas as abas; remover bindings globais remanescentes.
6. Migrar/limpar `message-service` (somente helpers puros ficam).

## Critérios de Aceite
- F5 em conversa existente: input limpa, placeholders aparecem, streaming atualiza; sem interferência entre abas.
- Eventos `chat:*` não vazam: cada controller assina apenas `chat:*:<conversationId>`.
- Fechar aba destrói listeners/stores (sem logs/eventos residuais).
- Nova conversa e mídia/voz/threads funcionam sem regressões.

## Casos de Teste Manual
- Conversa existente + F5 + enviar mensagem.
- Duas abas com conversas diferentes streamando simultaneamente.
- Fechar uma aba durante streaming; verificar que a outra continua ok.
- Alternar mostrar/ocultar mensagens internas por conversa e persistir preferência.

## Riscos e Mitigações
- Eventos sem `tabId`: escopo por `conversationId` é suficiente; se necessário, incluir `tabId` futuramente.
- Regressões em voz/mídia/threads: migrar em etapas com checagem por recurso.
- Reatividade: reforçar imutabilidade de arrays/objetos nas stores.

## Observabilidade/Logs
- Logs padronizados por controller: `[Conversation <convId>/<tabId>]`.
- Métricas básicas: tempo de load de conversa, duração de streaming.

## Rollback
- Mantém compatibilidade com eventos legados; podemos reverter controllers para usar `chat:*` global em último caso.