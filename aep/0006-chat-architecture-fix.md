# Plano de Reestruturação da Arquitetura de Chat

**Status:** Superseded — plano Svelte substituído pela arquitetura React e pelos contratos das AEPs 0037/0040

## Problema Atual

A arquitetura híbrida (chat simples → tabs) criou uma incompatibilidade fundamental entre MessageService e Svelte:

```
MessageService (JavaScript puro)
  └─ stores: { messages, conversationId, ... } ← Svelte não consegue observar
       └─ Chat.svelte tenta acessar via $messageService.stores.X ❌
```

**Por que falha:** Svelte compila `$store` em subscription no momento da criação do componente. Stores nested (`obj.stores.x`) não são detectadas pelo compilador.

## Solução: Arquitetura Direta com Stores Locais

### Opção 1: Stores no Componente (RECOMENDADA - Rápida)

**Tempo estimado:** 2-3 horas
**Risco:** Baixo
**Compatibilidade:** 100% com Svelte

```javascript
// Chat.svelte
import { writable } from 'svelte/store';

// Stores locais gerenciadas pelo componente
const messages = writable([]);
const conversationId = writable(null);
const isStreaming = writable(false);

// MessageService vira um controller puro (sem stores)
const messageService = new MessageServiceController({
  onMessagesUpdate: (msgs) => messages.set(msgs),
  onStreamingUpdate: (streaming) => isStreaming.set(streaming),
  // ...
});

// Agora funciona! ✅
$: console.log('Messages mudou:', $messages.length);
```

**Mudanças necessárias:**

1. **MessageService** → Remove stores internos, passa a receber callbacks:
   ```javascript
   class MessageServiceController {
     constructor({ onMessagesUpdate, onConversationUpdate, ... }) {
       this.callbacks = { onMessagesUpdate, ... };
     }
     
     _updateMessages(msgs) {
       this.callbacks.onMessagesUpdate(msgs);
     }
   }
   ```

2. **Chat.svelte** → Cria stores locais, passa callbacks para MessageService:
   ```javascript
   const messages = writable([]);
   const messageService = new MessageServiceController({
     onMessagesUpdate: (v) => messages.set(v)
   });
   
   // Uso direto com $
   {#each $messages as msg}
   ```

3. **ChatTabsContainer** → Cada guia tem suas próprias stores, MessageService vira stateless

**Vantagens:**
- ✅ Svelte funciona nativamente (`$messages`)
- ✅ Cada guia independente
- ✅ Sem subscriptions manuais
- ✅ Reatividade garantida
- ✅ Simples de debugar

**Desvantagens:**
- Precisa refatorar MessageService (remover stores internas)
- Cada guia duplica stores (mas isso já acontece hoje)

### Opção 2: MessageService como Svelte Store (Médio prazo)

**Tempo estimado:** 4-6 horas
**Risco:** Médio

```javascript
// message-service.js
import { writable, derived } from 'svelte/store';

export function createMessageService() {
  const { subscribe, set, update } = writable({
    messages: [],
    conversationId: null,
    isStreaming: false
  });
  
  return {
    subscribe,
    updateMessages: (msgs) => update(s => ({ ...s, messages: msgs })),
    // ...
  };
}

// Chat.svelte
const messageService = createMessageService();
$: console.log($messageService.messages); // ✅ Funciona!
```

### Opção 3: Context API do Svelte (Longo prazo)

**Tempo estimado:** 1-2 dias
**Risco:** Alto

Usa `setContext`/`getContext` para compartilhar stores entre componentes.

## Migração Passo a Passo (Opção 1 - RECOMENDADA)

### Fase 1: Preparação (30 min)

1. ✅ Criar branch `fix/chat-architecture`
2. ✅ Backup do MessageService atual
3. ✅ Documentar API atual

### Fase 2: Refatorar MessageService (1h)

**Arquivo:** `frontend/src/lib/chat/message-service.js`

```javascript
// ANTES (quebrado)
class MessageService extends EventTarget {
  constructor() {
    this.stores = {
      messages: writable([]),
      conversationId: writable(null)
    };
  }
  
  _updateMessages(msgs) {
    this.stores.messages.update(() => msgs);
  }
}

// DEPOIS (funcionando)
class MessageServiceController {
  constructor(callbacks) {
    this.callbacks = callbacks; // { onMessagesUpdate, onConversationUpdate, ... }
    this.state = {
      messages: [],
      conversationId: null
    };
  }
  
  _updateMessages(msgs) {
    this.state.messages = msgs;
    this.callbacks.onMessagesUpdate?.(msgs);
  }
  
  getState() {
    return { ...this.state };
  }
}
```

### Fase 3: Atualizar Chat.svelte (1h)

**Arquivo:** `frontend/src/pages/chat/Chat.svelte`

```svelte
<script>
  import { writable } from 'svelte/store';
  import { MessageServiceController } from '../../lib/chat/message-service.js';
  
  // STORES LOCAIS - gerenciadas pelo componente
  const messages = writable([]);
  const conversationId = writable(null);
  const conversationTitle = writable('');
  const conversationData = writable(null);
  const isStreaming = writable(false);
  
  // MessageService agora é um controller que atualiza as stores via callbacks
  const messageService = new MessageServiceController({
    onMessagesUpdate: (v) => messages.set(v || []),
    onConversationIdUpdate: (v) => conversationId.set(v),
    onConversationTitleUpdate: (v) => conversationTitle.set(v || ''),
    onConversationDataUpdate: (v) => conversationData.set(v),
    onIsStreamingUpdate: (v) => isStreaming.set(v || false)
  });
  
  onMount(async () => {
    await messageService.ready();
    // Sem subscriptions! O messageService chama os callbacks diretamente
  });
  
  // REACTIVE STATEMENTS FUNCIONAM AGORA! ✅
  $: console.log('🔥 Messages mudou:', $messages.length);
  $: threadedMessages = $conversationData?.threads?.map(...) || [];
  $: visibleMessages = showInternalMessages ? $messages : $messages.filter(m => !m.internal);
</script>

<!-- USO DIRETO COM $ -->
<h2>{$conversationTitle || 'Nova conversa'}</h2>
<ChatContainer messages={$messages} {threadedMessages} />
```

### Fase 4: Atualizar ChatTabsContainer (30 min)

Cada guia recebe suas stores e cria seu próprio MessageServiceController.

### Fase 5: Testes (30 min)

1. Testar streaming em tempo real
2. Testar múltiplas guias
3. Testar troca entre guias
4. Testar reload da página

## Checklist de Implementação

- [ ] Backup código atual
- [ ] Criar `MessageServiceController` sem stores internas
- [ ] Refatorar métodos para usar callbacks em vez de stores
- [ ] Criar stores locais no Chat.svelte
- [ ] Conectar callbacks do controller às stores locais
- [ ] Substituir todas referências `messageService.stores.X` por `$localStore`
- [ ] Testar streaming
- [ ] Testar múltiplas guias
- [ ] Remover código morto (subscriptions manuais, workarounds)
- [ ] Documentar nova arquitetura

## Resultado Esperado

```
ANTES:
Backend → MessageService.stores.messages → ❌ Svelte não observa → UI não atualiza

DEPOIS:
Backend → MessageService.callback() → Store local.set() → ✅ Svelte observa → UI atualiza ✅
```

## Alternativa Mínima (Se Opção 1 for muito complexa)

**Quick Fix:** Forçar re-render manual do Chat.svelte quando MessageService atualizar:

```javascript
let forceUpdateKey = 0;

messageService.addEventListener('messagesUpdated', () => {
  forceUpdateKey++; // Força Svelte re-renderizar tudo
});

// No template
{#key forceUpdateKey}
  <ChatContainer messages={messageService.state.messages} />
{/key}
```

**Tempo:** 15 minutos
**Qualidade:** Gambiarra, mas funciona
