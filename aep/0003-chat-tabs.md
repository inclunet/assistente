# Implementação de Guias (Tabs) de Chat

**Status:** Done

## Resumo

Este documento descreve o plano de implementação de um sistema de múltiplas guias para o chat, permitindo ao usuário ter várias conversas abertas simultaneamente, similar a um navegador de abas.

## Requisitos Funcionais

### RF1: Múltiplas Guias
- O usuário pode abrir múltiplas conversas em guias separadas
- Cada guia mantém seu estado de conversa independente
- Guias podem ser fechadas individualmente

### RF2: Persistência de Guias
- Todas as guias abertas devem ser salvas e restauradas quando a aplicação iniciar
- O estado inclui: ID das conversas abertas, guia ativa

### RF3: Integração com Histórico
- Ao selecionar uma conversa do histórico, ela é aberta na guia atual (substitui a conversa)
- Se a conversa já está aberta em outra guia, foca nessa guia em vez de abrir duplicada

### RF4: Nova Conversa
- Ao iniciar nova conversa (Ctrl+N ou enviar mensagem em guia vazia), ela é atribuída à guia atual
- Nova guia pode ser criada via Ctrl+T ou botão "+"

### RF5: Atalhos de Teclado
- `Ctrl+T`: Nova guia
- `Ctrl+W`: Fecha guia atual
- `Ctrl+Tab`: Próxima guia
- `Ctrl+Shift+Tab`: Guia anterior
- `Ctrl+1-9`: Ir para guia N

---

## Análise da Arquitetura Atual

### Componentes Relevantes

#### 1. `TabPanel` (`components/tabs/TabPanel.svelte`)
Componente de abas genérico já implementado com:
- ✅ Suporte a abas dinâmicas (add/remove)
- ✅ Abas fecháveis
- ✅ Atalhos Ctrl+T, Ctrl+W, Ctrl+Tab
- ✅ Navegação por teclado completa
- ✅ ARIA completo para acessibilidade
- ✅ Reordenação por drag-and-drop
- ✅ Slot para conteúdo das abas

#### 2. `Chat.svelte` (`pages/chat/Chat.svelte`)
Página principal do chat:
- Recebe `conversation` como prop
- Usa `messageService` singleton para gerenciar mensagens
- Mantém estado local de UI (model, voice, etc.)

**Problema atual**: O `messageService` é singleton, gerencia apenas UMA conversa por vez.

#### 3. `messageService` (`lib/chat/message-service.js`)
Serviço singleton de mensagens:
- Stores Svelte: `conversationId`, `messages`, `isStreaming`, etc.
- Método `loadConversation(conv)` - substitui conversa atual
- Método `clear()` - limpa conversa atual

#### 4. `App.svelte`
Orquestra navegação entre páginas:
- Mantém `currentConversation` 
- Passa conversa para `Chat.svelte` via prop

#### 5. `ConversationList.svelte` (`pages/history`)
Lista de conversas do histórico:
- Dispara evento `select` com conversa selecionada
- App.svelte trata e navega para chat

### Fluxo Atual de Dados

```
App.svelte
  │
  ├── currentConversation (state)
  │
  └── Chat.svelte
        │
        ├── prop: conversation
        │
        └── messageService (singleton)
              │
              ├── conversationId (store)
              ├── messages (store)
              └── isStreaming (store)
```

---

## Arquitetura Proposta

### Opção A: MessageService Multi-Instância (Recomendada)

Criar uma instância do messageService para cada guia aberta. Cada instância gerencia sua própria conversa.

```
App.svelte
  │
  ├── chatTabs: Tab[]  (state)
  │     { id, conversationId, title, messageServiceInstance }
  │
  ├── activeTabId (state)
  │
  └── TabPanel
        │
        └── Chat.svelte (per tab)
              │
              ├── prop: tabMessageService (instância isolada)
              └── prop: conversation
```

**Vantagens:**
- Isolamento completo entre guias
- Streaming independente por guia
- Mantém stores reativos funcionando
- Compatível com hot reload

**Desvantagens:**
- Precisa adaptar Chat.svelte para aceitar messageService como prop
- Múltiplas conexões de eventos do backend

### Opção B: MessageService Singleton com Contexto

Manter singleton, mas implementar sistema de contexto/slots que salvam/restauram estado por guia.

**Vantagens:**
- Menor mudança no código existente
- Uma única conexão com backend

**Desvantagens:**
- Streaming só pode acontecer em uma guia por vez
- Complexidade de salvar/restaurar estado
- Não recomendado

### Decisão: Opção A - Multi-Instância

---

## Estrutura de Dados

### Estado das Guias (a ser persistido em localStorage)

```typescript
interface ChatTab {
  id: string;           // ID único da guia (UUID)
  conversationId: number | null;  // ID da conversa no banco (null = nova conversa)
  title: string;        // Título exibido na aba
  icon?: string;        // Ícone (emoji)
  createdAt: string;    // ISO timestamp
}

interface ChatTabsState {
  tabs: ChatTab[];
  activeTabId: string;
  version: number;      // Para migrações futuras
}
```

### Chave de Persistência

```javascript
const STORAGE_KEY = 'chat_tabs_state';
```

---

## Plano de Implementação

### Fase 1: Preparação do MessageService

#### 1.1. Refatorar MessageService para Multi-Instância

**Arquivo:** `lib/chat/message-service.js`

Mudanças necessárias:
1. Mover stores para dentro da classe (ao invés de module-level)
2. Criar método factory `createMessageService()`
3. Manter export singleton para retrocompatibilidade

```javascript
// Nova estrutura
export function createMessageService() {
  // Stores privados da instância
  const conversationId = writable(null);
  const messages = writable([]);
  // ... outros stores

  class MessageServiceInstance extends EventTarget {
    // Getters que retornam stores
    get stores() {
      return {
        conversationId,
        messages,
        // ...
      };
    }
    // Métodos existentes
  }

  return new MessageServiceInstance();
}

// Mantém singleton para retrocompatibilidade
export const messageService = createMessageService();
```

#### 1.2. Adaptar Chat.svelte para Instância Injetada

**Arquivo:** `pages/chat/Chat.svelte`

```javascript
// Antes
import { messageService } from '../../lib/chat/index.js';

// Depois
export let messageService = null; // Injetado pelo pai
// ou usa singleton se não injetado (retrocompatibilidade)
$: service = messageService || defaultMessageService;
```

### Fase 2: Implementação do Container de Guias

#### 2.1. Criar ChatTabsContainer

**Arquivo:** `pages/chat/ChatTabsContainer.svelte`

Responsabilidades:
- Gerenciar array de guias abertas
- Criar/destruir instâncias de messageService
- Salvar/restaurar estado em localStorage
- Renderizar TabPanel com Chat.svelte por guia

```svelte
<script>
  import { onMount, onDestroy } from 'svelte';
  import { TabPanel } from '../../components/tabs';
  import Chat from './Chat.svelte';
  import { createMessageService } from '../../lib/chat/message-service.js';

  export let hasApiKey = false;
  export let defaultModel = '';
  export let defaultChatParams = {};

  // Estado das guias
  let tabs = [];
  let activeTabId = '';
  
  // Map de messageService por guia
  const serviceMap = new Map();

  const STORAGE_KEY = 'chat_tabs_state';

  // Carrega estado salvo
  onMount(() => {
    loadTabsState();
  });

  // Salva estado ao mudar
  $: if (tabs.length > 0) {
    saveTabsState();
  }

  function loadTabsState() {
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved) {
        const state = JSON.parse(saved);
        tabs = state.tabs || [];
        activeTabId = state.activeTabId || '';
        
        // Cria messageService para cada guia
        tabs.forEach(tab => {
          if (!serviceMap.has(tab.id)) {
            serviceMap.set(tab.id, createMessageService());
          }
        });
        
        // Se nenhuma guia, cria uma padrão
        if (tabs.length === 0) {
          addNewTab();
        }
      } else {
        addNewTab();
      }
    } catch (e) {
      console.error('Erro ao carregar guias:', e);
      addNewTab();
    }
  }

  function saveTabsState() {
    const state = {
      tabs: tabs.map(t => ({
        id: t.id,
        conversationId: t.conversationId,
        title: t.title,
        icon: t.icon,
        createdAt: t.createdAt
      })),
      activeTabId,
      version: 1
    };
    localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
  }

  function addNewTab() {
    const id = crypto.randomUUID();
    const newTab = {
      id,
      conversationId: null,
      title: 'Nova conversa',
      icon: '💬',
      createdAt: new Date().toISOString()
    };
    
    tabs = [...tabs, newTab];
    serviceMap.set(id, createMessageService());
    activeTabId = id;
  }

  function closeTab(tabId) {
    // Limpa messageService
    const service = serviceMap.get(tabId);
    if (service) {
      service.unbindBackendEvents();
      serviceMap.delete(tabId);
    }
    
    tabs = tabs.filter(t => t.id !== tabId);
    
    // Se fechou a guia ativa, ativa outra
    if (activeTabId === tabId && tabs.length > 0) {
      activeTabId = tabs[0].id;
    }
  }

  function handleTabChange({ detail }) {
    activeTabId = detail.tabId;
  }

  function handleTabClose({ detail }) {
    closeTab(detail.tabId);
  }

  function handleAddTab() {
    addNewTab();
  }

  // Atualiza título da guia quando conversa muda
  function updateTabTitle(tabId, title) {
    tabs = tabs.map(t => 
      t.id === tabId ? { ...t, title } : t
    );
  }

  // Atualiza conversationId da guia
  function updateTabConversation(tabId, conversationId) {
    tabs = tabs.map(t => 
      t.id === tabId ? { ...t, conversationId } : t
    );
  }

  // Abre conversa em guia existente (do histórico)
  export function openConversation(conversation) {
    // Verifica se já está aberta em alguma guia
    const existingTab = tabs.find(t => t.conversationId === conversation.id);
    if (existingTab) {
      activeTabId = existingTab.id;
      return;
    }
    
    // Abre na guia atual
    const currentTab = tabs.find(t => t.id === activeTabId);
    if (currentTab) {
      updateTabConversation(activeTabId, conversation.id);
      updateTabTitle(activeTabId, conversation.title || 'Conversa');
      
      // Carrega conversa no messageService da guia
      const service = serviceMap.get(activeTabId);
      if (service) {
        service.loadConversation(conversation, defaultModel);
      }
    }
  }

  // Nova conversa na guia atual
  export function startNewConversation() {
    const service = serviceMap.get(activeTabId);
    if (service) {
      service.clear();
      updateTabConversation(activeTabId, null);
      updateTabTitle(activeTabId, 'Nova conversa');
    }
  }

  // Converte tabs para formato do TabPanel
  $: tabPanelTabs = tabs.map(t => ({
    id: t.id,
    label: t.title,
    icon: t.icon,
    data: { conversationId: t.conversationId }
  }));
</script>

<TabPanel
  tabs={tabPanelTabs}
  bind:activeTab={activeTabId}
  closableTabs
  addable
  addLabel="Nova conversa"
  ariaLabel="Conversas abertas"
  on:change={handleTabChange}
  on:close={handleTabClose}
  on:closeRequest={handleTabClose}
  on:add={handleAddTab}
>
  <svelte:fragment slot="tab-content" let:tabId>
    {#if serviceMap.has(tabId)}
      <Chat
        {hasApiKey}
        {defaultModel}
        {defaultChatParams}
        messageService={serviceMap.get(tabId)}
        on:conversationCreated={(e) => {
          updateTabConversation(tabId, e.detail.conversationId);
          updateTabTitle(tabId, e.detail.title);
        }}
        on:titleChanged={(e) => {
          updateTabTitle(tabId, e.detail.title);
        }}
      />
    {/if}
  </svelte:fragment>
</TabPanel>
```

### Fase 3: Integração com App.svelte

#### 3.1. Substituir Chat por ChatTabsContainer

**Arquivo:** `App.svelte`

```javascript
// Antes
import { Chat } from './pages/chat';

// Depois
import { ChatTabsContainer } from './pages/chat';

// Remover currentConversation do estado global
// O container de guias gerencia isso agora
```

```svelte
<!-- Antes -->
<svelte:component
  this={pageConfig.component}
  bind:this={currentComponent}
  {...pageProps[currentPage]}
/>

<!-- Depois (para page === 'chat') -->
<ChatTabsContainer
  bind:this={chatTabsRef}
  {hasApiKey}
  {defaultModel}
  {defaultChatParams}
/>
```

#### 3.2. Atualizar Navegação do Histórico

```javascript
// Antes
const pageEvents = {
  history: {
    select: (e) => { currentConversation = e.detail; currentPage = 'chat'; },
  }
};

// Depois
const pageEvents = {
  history: {
    select: (e) => { 
      currentPage = 'chat';
      // Aguarda transição e abre no container
      setTimeout(() => {
        chatTabsRef?.openConversation(e.detail);
      }, 0);
    },
  }
};
```

### Fase 4: Refinar Experiência

#### 4.1. Limite de Guias
- Definir máximo de guias abertas (ex: 20)
- Mostrar aviso ao atingir limite

#### 4.2. Indicadores Visuais
- Guia com streaming ativo: ícone pulsando
- Guia com mensagem não lida: badge/destaque
- Guia modificada não salva: indicador (se aplicável)

#### 4.3. Título Dinâmico
- Atualizar título da guia quando conversa receber título do backend
- Truncar títulos longos com ellipsis

---

## Migração

### Ao Iniciar pela Primeira Vez
1. Verificar se existe `chat_tabs_state` em localStorage
2. Se não existir, verificar `last_conversation_id` do config
3. Se houver conversa anterior, criar guia com ela
4. Caso contrário, criar guia vazia

### Código de Migração

```javascript
function migrateFromSingleChat() {
  const hasTabsState = localStorage.getItem('chat_tabs_state');
  if (hasTabsState) return; // Já migrado
  
  // Busca última conversa do config (via GetConfig)
  const config = await GetConfig();
  if (config.last_conversation_id) {
    // Cria guia com a última conversa
    const conv = await GetConversation(config.last_conversation_id);
    if (conv) {
      const initialState = {
        tabs: [{
          id: crypto.randomUUID(),
          conversationId: conv.id,
          title: conv.title || 'Conversa anterior',
          icon: '💬',
          createdAt: new Date().toISOString()
        }],
        activeTabId: null, // Será preenchido
        version: 1
      };
      initialState.activeTabId = initialState.tabs[0].id;
      localStorage.setItem('chat_tabs_state', JSON.stringify(initialState));
    }
  }
}
```

---

## Testes Necessários

### Unitários
- [ ] MessageService multi-instância funciona isoladamente
- [ ] Estado das guias persiste corretamente
- [ ] Serialização/deserialização do estado

### Integração
- [ ] Criar nova guia e enviar mensagem
- [ ] Fechar guia e verificar cleanup
- [ ] Alternar entre guias mantém estado
- [ ] Abrir conversa do histórico em guia atual
- [ ] Abrir conversa já aberta foca na guia existente
- [ ] Streaming funciona independente por guia
- [ ] Reiniciar app restaura guias corretamente

### Acessibilidade
- [ ] Navegação por teclado (Ctrl+Tab, Ctrl+W)
- [ ] Screen reader anuncia mudança de guia
- [ ] Foco correto após fechar guia

### Performance
- [ ] Múltiplas guias não degradam performance
- [ ] Memória é liberada ao fechar guias

---

## Status de Implementação

✅ **IMPLEMENTADO** (Janeiro 2026)

| Fase | Descrição | Status |
|------|-----------|--------|
| 1 | Refatorar MessageService para multi-instância | ✅ Concluído |
| 2 | Criar ChatTabsContainer | ✅ Concluído |
| 3 | Adaptar Chat.svelte para messageService injetado | ✅ Concluído |
| 4 | Integração App.svelte e navegação | ✅ Concluído |
| 5 | Testes e ajustes | 🔄 Em progresso |

---

## Riscos e Mitigações

### Risco 1: Múltiplas conexões de eventos
**Problema:** Cada messageService conecta aos eventos do backend separadamente.
**Mitigação:** Implementar um event hub centralizado que distribui eventos para a instância correta baseado no conversationId do evento.

### Risco 2: Memória
**Problema:** Muitas guias abertas podem consumir memória.
**Mitigação:** Limitar número máximo de guias; considerar lazy loading de histórico por guia.

### Risco 3: Conflito de backend events
**Problema:** Eventos de streaming sem conversationId podem ir para guia errada.
**Mitigação:** Garantir que todos os eventos do backend incluam conversationId para roteamento.

---

## Arquivos a Modificar

| Arquivo | Tipo de Mudança |
|---------|-----------------|
| `lib/chat/message-service.js` | Refatorar para multi-instância |
| `pages/chat/Chat.svelte` | Aceitar messageService como prop |
| `pages/chat/ChatTabsContainer.svelte` | **NOVO** - Container de guias |
| `pages/chat/index.js` | Exportar novo container |
| `App.svelte` | Usar container, atualizar navegação |
| `components/tabs/TabPanel.svelte` | Possíveis ajustes menores |

---

## Dependências Externas

Nenhuma nova dependência é necessária. O componente `TabPanel` existente atende todos os requisitos visuais e de acessibilidade.

