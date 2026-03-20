# Refatoração Completa da Arquitetura de Chat

**Data:** 15 de Janeiro de 2026  
**Motivo:** Sistema de múltiplas guias não funciona - reactive statements do Svelte nunca executam  
**Estimativa:** 4-6 horas  
**Status:** EM DESENVOLVIMENTO

## Problema Identificado

### Sintoma
- Mensagens não aparecem em tempo real durante streaming
- Campo de input não limpa após envio
- Mensagens só aparecem após reload da página
- Reactive statements nunca executam (`$:` nunca dispara)

### Causa Raiz
**Arquitetura incompatível com Svelte:**

```javascript
// ANTES (Quebrado)
MessageService {
  stores: {
    messages: writable([]),
    conversationId: writable(null),
    ...
  }
}

// Chat.svelte tenta usar
const messageService = externalMessageService;
$: console.log($messages);  // ❌ NÃO FUNCIONA

// Por quê?
// Svelte $ syntax só funciona com stores TOP-LEVEL
// messageService.stores.messages é NESTED = Svelte não detecta
```

**Tentativas que falharam:**
1. ❌ `$messageService.stores.messages` - Sintaxe inválida
2. ❌ Subscriptions manuais - Callbacks não acionam reatividade Svelte
3. ❌ Reactive assignments - `$: localX = $storeX` não funciona com stores nested
4. ❌ Stores locais com subscriptions - Atribuições em callbacks são invisíveis para Svelte
5. ❌ Force updates com counters - Não força reactive statements

**Conclusão:** A arquitetura precisa ser completamente redesenhada para ser compatível com Svelte.

## Solução: Inversão de Responsabilidades

### Princípio
**"Stores no componente, lógica no controller"**

- ✅ **Stores:** Gerenciadas pelo componente Svelte (top-level)
- ✅ **Controller:** Stateless, só executa operações e atualiza stores externas
- ✅ **Chat.svelte:** Recebe stores como props, usa `$store` nativamente

### Nova Arquitetura

```
ChatTabsContainer.svelte
  └─ Para cada guia: ChatTab.svelte (NOVO)
       │
       ├─ Cria stores (top-level):
       │    const messages = writable([]);
       │    const conversationId = writable(null);
       │    ...
       │
       ├─ Cria controller (stateless):
       │    const controller = new MessageController(stores);
       │
       └─ Passa para Chat.svelte:
            <Chat 
              {messages}           ← Store Svelte
              {conversationId}     ← Store Svelte
              {controller}         ← Funções
            />

Chat.svelte (recebe props)
  └─ Usa diretamente:
       $: console.log($messages);  ✅ FUNCIONA!
       {#each $messages as msg}     ✅ FUNCIONA!
```

## Estrutura de Arquivos

### Antes
```
lib/chat/
  ├─ message-service.js  (1584 linhas, stores + lógica misturadas)
  └─ index.js

pages/chat/
  ├─ ChatTabsContainer.svelte  (gerencia guias + services)
  └─ Chat.svelte              (3000 linhas, tenta usar stores nested)
```

### Depois
```
lib/chat/
  ├─ stores.js              (NOVO - Factory de stores isoladas)
  ├─ message-controller.js  (NOVO - Controller stateless)
  ├─ utils.js              (NOVO - Funções utilitárias)
  └─ index.js              (Exporta tudo)

pages/chat/
  ├─ ChatTabsContainer.svelte  (Gerencia guias, cria ChatTabs)
  ├─ ChatTab.svelte           (NOVO - Wrapper: stores + controller)
  └─ Chat.svelte              (UI puro, recebe stores como props)
```

## Detalhamento dos Novos Arquivos

### 1. `lib/chat/stores.js`
**Propósito:** Factory que cria stores isoladas para cada guia

```javascript
import { writable, derived } from 'svelte/store';

/**
 * Cria um conjunto isolado de stores para uma guia de chat
 * Cada guia tem suas próprias stores completamente independentes
 */
export function createChatStores() {
  // Stores primárias
  const messages = writable([]);
  const conversationId = writable(null);
  const conversationTitle = writable('');
  const conversationData = writable(null);
  const isStreaming = writable(false);
  
  // Stores derivadas (calculadas automaticamente)
  const hasConversation = derived(conversationId, $id => $id !== null);
  const isEmpty = derived(messages, $msgs => $msgs.length === 0);
  const messageCount = derived(messages, $msgs => $msgs.length);
  const threadedMessages = derived(
    conversationData, 
    $data => $data?.threads || []
  );
  
  return {
    // Primárias
    messages,
    conversationId,
    conversationTitle,
    conversationData,
    isStreaming,
    
    // Derivadas
    hasConversation,
    isEmpty,
    messageCount,
    threadedMessages
  };
}
```

**Características:**
- ✅ Cada chamada cria stores completamente isoladas
- ✅ Stores derivadas são calculadas automaticamente
- ✅ Zero dependências entre guias

### 2. `lib/chat/message-controller.js`
**Propósito:** Controller stateless que executa operações e atualiza stores externas

```javascript
import { get } from 'svelte/store';

/**
 * Controller para operações de chat
 * NÃO gerencia state interno - apenas atualiza stores passadas no construtor
 */
export class MessageController {
  constructor(stores) {
    this.stores = stores;  // Referência às stores externas
    this._backendUnsubscribers = [];
    this._initialized = false;
  }
  
  /**
   * Inicializa controller e conecta eventos do backend
   */
  async init() {
    if (this._initialized) return;
    
    await this._loadWailsFunctions();
    this.bindBackendEvents();
    this._initialized = true;
  }
  
  /**
   * Envia mensagem
   */
  async sendMessage(content, media, params) {
    const convId = get(this.stores.conversationId) || 0;
    
    // Backend salva mensagem e dispara eventos
    const result = await SendMessage(convId, content, media, params);
    
    // Eventos do backend vão atualizar as stores automaticamente
    return result;
  }
  
  /**
   * Carrega conversa do banco
   */
  async loadConversation(id) {
    const conv = await GetConversation(id);
    
    // Atualiza stores diretamente
    this.stores.conversationId.set(conv.id);
    this.stores.conversationTitle.set(conv.title);
    this.stores.conversationData.set(conv);
    
    const messages = await GetMessages(id);
    this.stores.messages.set(messages);
  }
  
  /**
   * Conecta eventos do backend
   */
  bindBackendEvents() {
    // Stream de mensagens
    const handleStream = (event) => {
      // Só processa se for desta conversa
      if (event.conversationId !== get(this.stores.conversationId)) return;
      
      // Atualiza stores
      this.stores.isStreaming.set(true);
      this.stores.messages.update(msgs => {
        const lastMsg = msgs[msgs.length - 1];
        if (lastMsg?.isStreaming) {
          lastMsg.content = event.content;
          return [...msgs];
        }
        return [...msgs, { content: event.content, isStreaming: true }];
      });
    };
    
    const unsubStream = EventsOn('chat:stream', handleStream);
    this._backendUnsubscribers.push(() => EventsOff('chat:stream', handleStream));
    
    // Stream finalizado
    const handleStreamEnd = (event) => {
      if (event.conversationId !== get(this.stores.conversationId)) return;
      this.stores.isStreaming.set(false);
    };
    
    const unsubEnd = EventsOn('chat:stream_end', handleStreamEnd);
    this._backendUnsubscribers.push(() => EventsOff('chat:stream_end', handleStreamEnd));
    
    // Demais eventos...
  }
  
  /**
   * Limpa recursos
   */
  destroy() {
    this._backendUnsubscribers.forEach(unsub => unsub());
    this._backendUnsubscribers = [];
  }
}
```

**Características:**
- ✅ Stateless - todo estado nas stores externas
- ✅ Fácil de testar - só funções puras
- ✅ Roteamento de eventos por conversationId
- ✅ Cleanup automático

### 3. `lib/chat/utils.js`
**Propósito:** Funções utilitárias

Migração das funções utilitárias do message-service.js atual.

### 4. `pages/chat/ChatTab.svelte`
**Propósito:** Wrapper que conecta stores + controller para uma guia

```svelte
<script>
  import { onMount, onDestroy } from 'svelte';
  import { createChatStores } from '../../lib/chat/stores.js';
  import { MessageController } from '../../lib/chat/message-controller.js';
  import Chat from './Chat.svelte';
  
  /** ID único desta guia */
  export let tabId;
  
  /** ID da conversa para carregar inicialmente (opcional) */
  export let initialConversationId = null;
  
  /** Props do chat (modelo, params, etc) */
  export let hasApiKey = false;
  export let defaultModel = '';
  export let defaultChatParams = {};
  
  // Cria stores isoladas para ESTA guia
  const stores = createChatStores();
  
  // Cria controller que gerencia ESTAS stores
  const controller = new MessageController(stores);
  
  onMount(async () => {
    // Inicializa controller (conecta backend events)
    await controller.init();
    
    // Se tem conversa inicial, carrega
    if (initialConversationId) {
      await controller.loadConversation(initialConversationId);
    }
  });
  
  onDestroy(() => {
    // Limpa recursos (event listeners)
    controller.destroy();
  });
  
  // Propaga eventos para o container
  function handleConversationCreated(event) {
    dispatch('conversationCreated', event.detail);
  }
  
  function handleTitleChanged(event) {
    dispatch('titleChanged', event.detail);
  }
</script>

<!-- Passa stores DIRETAMENTE como props -->
<Chat 
  {hasApiKey}
  {defaultModel}
  {defaultChatParams}
  
  messages={stores.messages}
  conversationId={stores.conversationId}
  conversationTitle={stores.conversationTitle}
  conversationData={stores.conversationData}
  isStreaming={stores.isStreaming}
  threadedMessages={stores.threadedMessages}
  
  {controller}
  
  on:conversationCreated={handleConversationCreated}
  on:titleChanged={handleTitleChanged}
/>
```

**Características:**
- ✅ Encapsula stores + controller
- ✅ Gerencia lifecycle (init/destroy)
- ✅ Repassa eventos para o container

### 5. `pages/chat/Chat.svelte` (Refatorado)
**Propósito:** UI puro, recebe stores como props

```svelte
<script>
  // === PROPS: Stores Svelte ===
  export let messages;           // writable<Message[]>
  export let conversationId;     // writable<number|null>
  export let conversationTitle;  // writable<string>
  export let conversationData;   // writable<ConversationData|null>
  export let isStreaming;        // writable<boolean>
  export let threadedMessages;   // derived<MessageNode[]>
  
  // === PROPS: Controller ===
  export let controller;         // MessageController
  
  // === PROPS: Config ===
  export let hasApiKey = false;
  export let defaultModel = '';
  export let defaultChatParams = {};
  
  // Estado local da UI (não das stores)
  let inputMessage = '';
  let isLoading = false;
  let pendingMedia = [];
  
  // REACTIVE STATEMENTS AGORA FUNCIONAM! ✅
  $: console.log('🔥 Messages mudou:', $messages.length);
  $: visibleMessages = $messages.filter(m => !m.internal);
  $: isEmpty = $messages.length === 0;
  
  async function handleSubmit() {
    if (!inputMessage.trim() || isLoading) return;
    
    isLoading = true;
    
    try {
      await controller.sendMessage(
        inputMessage, 
        pendingMedia, 
        defaultChatParams
      );
      
      // Limpa input IMEDIATAMENTE
      inputMessage = '';
      pendingMedia = [];
    } catch (err) {
      console.error('Erro ao enviar:', err);
    } finally {
      isLoading = false;
    }
  }
</script>

<!-- UI usa $ syntax diretamente -->
<div class="chat-container">
  <header>
    <h2>{$conversationTitle || 'Nova conversa'}</h2>
  </header>
  
  <div class="messages">
    {#each visibleMessages as message}
      <MessageBubble {message} />
    {/each}
    
    {#if $isStreaming}
      <LoadingIndicator />
    {/if}
  </div>
  
  <form on:submit|preventDefault={handleSubmit}>
    <input bind:value={inputMessage} disabled={isLoading} />
    <button disabled={isLoading || !inputMessage.trim()}>
      Enviar
    </button>
  </form>
</div>
```

**Mudanças principais:**
- ✅ Recebe stores como props (não cria internamente)
- ✅ Usa `$store` diretamente - funciona!
- ✅ Controller vira prop (só funções, sem state)
- ✅ Foco em UI, lógica no controller

## Comparação: Antes vs Depois

| Aspecto | ANTES | DEPOIS |
|---------|-------|--------|
| **Stores** | Nested em MessageService | Top-level em ChatTab |
| **Reatividade** | ❌ Não funciona | ✅ Nativa do Svelte |
| **Chat.svelte** | 3000 linhas, gerencia tudo | Limpo, só UI |
| **Testabilidade** | Difícil (state em service) | Fácil (controller stateless) |
| **Debugabilidade** | Complexa (subscriptions manuais) | Simples (Svelte DevTools) |
| **Isolamento guias** | Problemático (stores nested) | Perfeito (stores por guia) |
| **Manutenibilidade** | Baixa | Alta |

## Fluxo de Dados

### Streaming de Mensagem

```
1. Backend emite: chat:stream { conversationId: 1, content: "Olá" }
   ↓
2. MessageController.handleStream() recebe evento
   ↓
3. Valida: event.conversationId === get(stores.conversationId)
   ↓
4. Atualiza: stores.messages.update(msgs => [...msgs, newMsg])
   ↓
5. Svelte detecta mudança AUTOMATICAMENTE
   ↓
6. Re-renderiza: {#each $messages as msg} ✅
```

### Envio de Mensagem

```
1. Chat.svelte: handleSubmit()
   ↓
2. Chama: controller.sendMessage(text, media, params)
   ↓
3. Controller: await SendMessage(convId, text, media, params)
   ↓
4. Backend salva e emite eventos
   ↓
5. Eventos atualizam stores
   ↓
6. UI atualiza automaticamente ✅
```

## Plano de Migração

### Fase 1: Criar Novos Arquivos (1h)
- [x] Documentar refatoração
- [ ] Criar `lib/chat/stores.js`
- [ ] Criar `lib/chat/message-controller.js`
- [ ] Criar `lib/chat/utils.js` 
- [ ] Atualizar `lib/chat/index.js`
- [ ] Criar `pages/chat/ChatTab.svelte`

### Fase 2: Refatorar Chat.svelte (2h)
- [ ] Mudar stores de criação interna para props
- [ ] Substituir `messageService.X()` por `controller.X()`
- [ ] Remover subscriptions manuais
- [ ] Remover workarounds (counters, etc)
- [ ] Simplificar reactive statements
- [ ] Testar que `$messages` funciona

### Fase 3: Atualizar ChatTabsContainer (1h)
- [ ] Substituir criação de MessageService por ChatTab
- [ ] Remover `serviceMap`
- [ ] Simplificar gerenciamento de guias
- [ ] Atualizar persistência

### Fase 4: Testes (1h)
- [ ] Testar streaming em tempo real ✅
- [ ] Testar envio de mensagens ✅
- [ ] Testar múltiplas guias ✅
- [ ] Testar troca entre guias ✅
- [ ] Testar reload da página ✅

### Fase 5: Limpeza (30min)
- [ ] Remover `message-service.js` antigo
- [ ] Remover código morto
- [ ] Atualizar documentação
- [ ] Commit e push

## Riscos e Mitigações

| Risco | Probabilidade | Impacto | Mitigação |
|-------|---------------|---------|-----------|
| Quebrar funcionalidades existentes | Média | Alto | Testar extensivamente cada fase |
| Performance com muitas guias | Baixa | Médio | Stores isoladas = melhor performance |
| Bugs em edge cases | Média | Médio | Manter logs de debug |
| Conflito com código em desenvolvimento | Baixa | Alto | Trabalhar em branch separada |

## Critérios de Sucesso

- ✅ Streaming funciona em tempo real (mensagens aparecem caractere a caractere)
- ✅ Input limpa imediatamente após envio
- ✅ Reactive statements executam (`🔥 Messages mudou` aparece no console)
- ✅ Múltiplas guias funcionam independentemente
- ✅ Troca entre guias preserva estado
- ✅ Reload da página restaura guias
- ✅ Zero gambiarras no código
- ✅ Código limpo e testável

## Notas de Implementação

### Compatibilidade com Código Existente
- Manter exports em `lib/chat/index.js` para backward compatibility temporária
- Deprecar gradualmente MessageService antigo
- Documentar breaking changes

### Performance
- Stores isoladas por guia = melhor GC
- Menos re-renders (só a guia ativa renderiza)
- Derived stores calculadas automaticamente

### Debugging
- Svelte DevTools funciona nativamente
- Logs estruturados no controller
- State visível nas stores

### Testes Futuros
- Unit tests para MessageController (stateless = fácil)
- Integration tests com stores mockadas
- E2E tests com Playwright

## Referências

- [Svelte Stores Documentation](https://svelte.dev/docs#run-time-svelte-store)
- [Svelte Reactivity](https://svelte.dev/docs#component-format-script-3-$-marks-a-statement-as-reactive)
- Discussão: docs/CHAT_REFACTOR_PLAN.md (plano anterior, descartado)
- Issue: "mensagens não aparecem em tempo real após envio"
