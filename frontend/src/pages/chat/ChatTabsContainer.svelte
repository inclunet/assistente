<script>
  import { onMount, onDestroy, createEventDispatcher, tick } from 'svelte';
  import { TabPanel } from '../../components/tabs';
  import Chat from './Chat.svelte';
  import { createMessageService } from '../../lib/chat/index.js';
  import { GetConversation } from '../../../wailsjs/go/main/App.js';

  export let hasApiKey = false;
  export let defaultModel = '';
  export let defaultChatParams = { temperature: 0.7, max_tokens: 4096, top_p: 1.0 };
  
  /** ID da conversa para abrir inicialmente (migração) */
  export let initialConversationId = null;

  const dispatch = createEventDispatcher();

  // ========================================
  // Estado das guias
  // ========================================
  
  /** @type {Array<{id: string, conversationId: number|null, title: string, icon: string, createdAt: string}>} */
  let tabs = [];
  
  /** ID da guia ativa */
  let activeTabId = '';
  
  /** Map de messageService por guia */
  const serviceMap = new Map();
  
  /** Chave para persistência em localStorage */
  const STORAGE_KEY = 'chat_tabs_state';
  
  /** Versão do schema para migrações futuras */
  const SCHEMA_VERSION = 1;
  
  /** Limite máximo de guias abertas */
  const MAX_TABS = 20;

  // ========================================
  // Persistência
  // ========================================
  
  function loadTabsState() {
    try {
      const saved = localStorage.getItem(STORAGE_KEY);
      if (saved) {
        const state = JSON.parse(saved);
        
        // Valida versão
        if (state.version !== SCHEMA_VERSION) {
          console.log('[ChatTabs] Schema version mismatch, resetting tabs');
          return false;
        }
        
        tabs = state.tabs || [];
        activeTabId = state.activeTabId || '';
        
        // Cria messageService para cada guia
        tabs.forEach(tab => {
          if (!serviceMap.has(tab.id)) {
            const service = createMessageService(tab.id);
            serviceMap.set(tab.id, service);
          }
        });
        
        // Se nenhuma guia, cria uma padrão
        if (tabs.length === 0) {
          return false;
        }
        
        // Valida activeTabId
        if (!tabs.find(t => t.id === activeTabId)) {
          activeTabId = tabs[0]?.id || '';
        }
        
        return true;
      }
    } catch (e) {
      console.error('[ChatTabs] Erro ao carregar estado:', e);
    }
    return false;
  }
  
  function saveTabsState() {
    try {
      const state = {
        tabs: tabs.map(t => ({
          id: t.id,
          conversationId: t.conversationId,
          title: t.title,
          icon: t.icon,
          createdAt: t.createdAt
        })),
        activeTabId,
        version: SCHEMA_VERSION
      };
      localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
    } catch (e) {
      console.error('[ChatTabs] Erro ao salvar estado:', e);
    }
  }
  
  // Salva estado quando tabs ou activeTabId mudam
  // Usa JSON.stringify para detectar mudanças no conteúdo do array
  $: tabsJson = JSON.stringify(tabs);
  $: if (tabsJson && activeTabId) {
    saveTabsState();
  }

  // ========================================
  // Gerenciamento de guias
  // ========================================
  
  function generateTabId() {
    return crypto.randomUUID();
  }
  
  function addNewTab(conversationId = null, title = 'Nova conversa') {
    if (tabs.length >= MAX_TABS) {
      console.warn('[ChatTabs] Limite de guias atingido:', MAX_TABS);
      return null;
    }
    
    const id = generateTabId();
    const newTab = {
      id,
      conversationId,
      title: title || 'Nova conversa',
      icon: '💬',
      createdAt: new Date().toISOString()
    };
    
    tabs = [...tabs, newTab];
    
    // Cria messageService para a nova guia
    const service = createMessageService(id);
    serviceMap.set(id, service);
    
    // Ativa a nova guia
    activeTabId = id;
    
    console.log('[ChatTabs] Nova guia criada:', id);
    return id;
  }
  
  function closeTab(tabId) {
    const tabIndex = tabs.findIndex(t => t.id === tabId);
    if (tabIndex < 0) return;
    
    // Limpa messageService
    const service = serviceMap.get(tabId);
    if (service) {
      service.destroy();
      serviceMap.delete(tabId);
    }
    
    // Remove guia
    tabs = tabs.filter(t => t.id !== tabId);
    
    // Se fechou a guia ativa, ativa outra
    if (activeTabId === tabId && tabs.length > 0) {
      // Tenta ativar a próxima, ou a anterior se era a última
      const newIndex = Math.min(tabIndex, tabs.length - 1);
      activeTabId = tabs[newIndex]?.id || '';
    }
    
    // Se não há mais guias, cria uma nova
    if (tabs.length === 0) {
      addNewTab();
    }
    
    console.log('[ChatTabs] Guia fechada:', tabId);
  }
  
  function updateTabTitle(tabId, title) {
    tabs = tabs.map(t => 
      t.id === tabId ? { ...t, title: title || 'Nova conversa' } : t
    );
  }
  
  function updateTabConversation(tabId, conversationId, title = null) {
    tabs = tabs.map(t => {
      if (t.id === tabId) {
        return { 
          ...t, 
          conversationId,
          title: title || t.title
        };
      }
      return t;
    });
  }

  // ========================================
  // API pública
  // ========================================
  
  /**
   * Abre uma conversa do histórico.
   * Se já está aberta em alguma guia, foca nela.
   * Caso contrário, abre na guia atual.
   */
  export async function openConversation(conversation) {
    if (!conversation?.id) return;
    
    // Verifica se já está aberta em alguma guia
    const existingTab = tabs.find(t => t.conversationId === conversation.id);
    if (existingTab) {
      activeTabId = existingTab.id;
      return;
    }
    
    // Abre na guia atual
    const currentTab = tabs.find(t => t.id === activeTabId);
    if (currentTab) {
      updateTabConversation(activeTabId, conversation.id, conversation.title);
      
      // Carrega conversa no messageService da guia
      const service = serviceMap.get(activeTabId);
      if (service) {
        await service.loadConversation(conversation, defaultModel);
      }
    }
  }
  
  /**
   * Inicia uma nova conversa na guia atual.
   */
  export function startNewConversation() {
    const service = serviceMap.get(activeTabId);
    if (service) {
      service.clear();
      updateTabConversation(activeTabId, null, 'Nova conversa');
    }
  }
  
  /**
   * Foca no input da guia atual
   * Nota: Delega para o DOM diretamente
   */
  export function focusInput() {
    // Busca o painel ativo e foca no textarea dentro dele
    const activePane = document.querySelector('.tab-pane--active #message-input');
    if (activePane) {
      activePane.focus();
    } else {
      // Fallback: tenta sem a classe (pode ainda não ter sido aplicada)
      const allPanes = document.querySelectorAll('.tab-pane');
      for (const pane of allPanes) {
        if (!pane.hidden) {
          const input = pane.querySelector('#message-input');
          if (input) {
            input.focus();
            return;
          }
        }
      }
    }
  }

  // ========================================
  // Handlers de eventos
  // ========================================
  
  function handleTabChange({ detail }) {
    activeTabId = detail.tabId;
    // Não move o foco automaticamente - usuário pode estar apenas explorando as guias
  }
  
  /**
   * Quando muda de aba via Ctrl+Tab/Ctrl+Shift+Tab
   * Move o foco para o input da nova aba
   */
  async function handleTabSwitch({ detail }) {
    activeTabId = detail.tabId;
    // Aguarda o DOM atualizar (o atributo hidden muda)
    await tick();
    await tick();
    focusInput();
  }
  
  function handleTabClose({ detail }) {
    closeTab(detail.tabId);
  }
  
  async function handleAddTab() {
    addNewTab();
    
    // Aguarda a nova guia ser criada e renderizada
    await tick();
    await tick();
    await tick(); // Terceiro tick para garantir que o Chat foi montado
    focusInput();
  }
  
  /**
   * Quando uma conversa é criada ou atualizada em uma guia
   */
  function handleConversationCreated(tabId, event) {
    const { conversationId, title } = event.detail || {};
    if (conversationId) {
      updateTabConversation(tabId, conversationId, title);
    }
  }
  
  /**
   * Quando o título de uma conversa muda
   */
  function handleTitleChanged(tabId, event) {
    const { title } = event.detail || {};
    if (title) {
      updateTabTitle(tabId, title);
    }
  }
  
  /**
   * Quando uma conversa é selecionada via picker de histórico
   * Carrega a conversa na aba atual
   */
  async function handleConversationSelected(tabId, event) {
    const { conversationId, conversation } = event.detail || {};
    if (conversationId && conversation) {
      // Atualiza a aba com os dados da conversa
      updateTabConversation(tabId, conversationId, conversation.title);
      
      // Carrega a conversa no messageService da aba
      const service = serviceMap.get(tabId);
      if (service) {
        try {
          await service.loadConversation(conversation, defaultModel);
        } catch (e) {
          console.error('[ChatTabs] Erro ao carregar conversa selecionada:', e);
        }
      }
    }
  }

  // ========================================
  // Migração do sistema antigo
  // ========================================
  
  async function migrateFromSingleChat() {
    // Carrega última conversa se fornecida
    if (initialConversationId) {
      try {
        const conv = await GetConversation(initialConversationId);
        if (conv) {
          const tabId = addNewTab(conv.id, conv.title || 'Conversa anterior');
          
          // Carrega a conversa no messageService
          if (tabId) {
            const service = serviceMap.get(tabId);
            if (service) {
              await service.loadConversation(conv, defaultModel);
            }
          }
          return;
        }
      } catch (e) {
        console.warn('[ChatTabs] Conversa inicial não encontrada:', e);
      }
    }
    
    // Se não encontrou conversa, cria guia vazia
    addNewTab();
  }

  // ========================================
  // Lifecycle
  // ========================================
  
  onMount(async () => {
    // Tenta carregar estado salvo
    const loaded = loadTabsState();
    
    if (loaded) {
      // Carrega conversas salvas nas guias
      for (const tab of tabs) {
        if (tab.conversationId) {
          const service = serviceMap.get(tab.id);
          if (service) {
            try {
              const conv = await GetConversation(tab.conversationId);
              if (conv) {
                await service.loadConversation(conv, defaultModel);
              }
            } catch (e) {
              console.warn('[ChatTabs] Erro ao carregar conversa da guia:', tab.id, e);
              // Limpa conversationId inválido
              updateTabConversation(tab.id, null, 'Nova conversa');
            }
          }
        }
      }
    } else {
      // Migra do sistema antigo
      await migrateFromSingleChat();
    }
    
    // Foca no input após inicialização
    await tick();
    focusInput();
  });
  
  onDestroy(() => {
    // Salva estado final
    saveTabsState();
    
    // Limpa todos os messageServices
    for (const [tabId, service] of serviceMap) {
      service.destroy();
    }
    serviceMap.clear();
  });

  // ========================================
  // Conversão para formato do TabPanel
  // ========================================
  
  $: tabPanelTabs = tabs.map(t => ({
    id: t.id,
    label: t.title || 'Nova conversa',
    icon: t.icon,
    closable: tabs.length > 1, // Última guia não pode ser fechada
    data: { conversationId: t.conversationId }
  }));
</script>

<div class="chat-tabs-container">
  <TabPanel
    tabs={tabPanelTabs}
    bind:activeTab={activeTabId}
    closableTabs={tabs.length > 1}
    addable={false}
    keepMounted={true}
    size="sm"
    on:change={handleTabChange}
    on:tabSwitch={handleTabSwitch}
    on:close={handleTabClose}
    on:closeRequest={handleTabClose}
    on:add={handleAddTab}
  >
    <svelte:fragment slot="tab-content" let:tabId>
      {#if serviceMap.has(tabId)}
        {@const service = serviceMap.get(tabId)}
        <Chat
          {hasApiKey}
          {defaultModel}
          {defaultChatParams}
          externalMessageService={service}
          isActive={tabId === activeTabId}
          onNewTab={handleAddTab}
          on:conversationCreated={(e) => handleConversationCreated(tabId, e)}
          on:conversationUpdated={(e) => handleConversationCreated(tabId, e)}
          on:titleChanged={(e) => handleTitleChanged(tabId, e)}
          on:conversationSelected={(e) => handleConversationSelected(tabId, e)}
        />
      {/if}
    </svelte:fragment>
  </TabPanel>
</div>

<style>
  .chat-tabs-container {
    display: flex;
    flex-direction: column;
    height: 100%;
    width: 100%;
    overflow: hidden;
  }
  
  /* Ajusta o TabPanel para ocupar todo o espaço */
  .chat-tabs-container :global(.tab-panel) {
    height: 100%;
  }
  
  .chat-tabs-container :global(.tab-content) {
    height: 100%;
    overflow: hidden;
  }
  
  .chat-tabs-container :global(.tab-pane) {
    height: 100%;
    overflow: hidden;
  }
  
  .chat-tabs-container :global(.tab-pane--active) {
    display: flex;
    flex-direction: column;
  }
</style>

