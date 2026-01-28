<script>
  import { onMount, onDestroy } from 'svelte';
  import { TabPanel } from '../../components/tabs';
  import ChatTab from './ChatTab.svelte';
  import { 
    GetTabs, 
    CreateTab, 
    CloseTab, 
    SetActiveTab,
    LoadConversationInTab
  } from '../../../wailsjs/go/main/App.js';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime.js';

  export let hasApiKey = false;
  export let defaultModel = '';
  export let defaultChatParams = { temperature: 0.7, max_tokens: 4096, top_p: 1.0 };

  // ========================================
  // Estado (vem do backend)
  // ========================================
  
  let tabs = [];
  let activeTabId = null;
  let chatTabRefs = {};
  
  // Funções de cancelamento dos event listeners
  let unsubTabsUpdated = null;
  let unsubTabsActivated = null;
  let unsubConversationDeleted = null;
  let unsubDatabaseReset = null;
  
  // Variável reativa para forçar atualização do TabPanel
  $: activeTabString = String(activeTabId);
  
  // ========================================
  // Lifecycle
  // ========================================
  
  // Handler de teclado global para navegação entre abas
  function handleGlobalKeyDown(event) {
    // Ctrl+Tab ou Ctrl+Shift+Tab (pode não funcionar se bloqueado pelo browser)
    if (event.ctrlKey && event.key === 'Tab') {
      event.preventDefault();
      event.stopPropagation();
      
      if (event.shiftKey) {
        navigateToPreviousTab();
      } else {
        navigateToNextTab();
      }
      return;
    }
    
    // Ctrl+PageUp/PageDown: navegação entre abas
    if (event.ctrlKey && (event.key === 'PageUp' || event.key === 'PageDown')) {
      event.preventDefault();
      event.stopPropagation();
      
      if (event.key === 'PageUp') {
        navigateToPreviousTab();
      } else {
        navigateToNextTab();
      }
      return;
    }
  }
  
  onMount(async () => {
    await loadTabs();
    
    // Escuta eventos do backend - armazena função de cancelamento
    unsubTabsUpdated = EventsOn('tabs:updated', handleTabsUpdated);
    unsubTabsActivated = EventsOn('tabs:activated', handleTabActivated);
    unsubConversationDeleted = EventsOn('conversation:deleted', handleConversationDeleted);
    unsubDatabaseReset = EventsOn('database:reset', handleDatabaseReset);
    
    // Adiciona listener de teclado global com CAPTURE=true para ter prioridade
    window.addEventListener('keydown', handleGlobalKeyDown, true);
    
    // Limpa localStorage obsoleto (uma vez)
    if (localStorage.getItem('chat_tabs_state')) {
      localStorage.removeItem('chat_tabs_state');
      console.log('🧹 localStorage de abas antigas removido');
    }
  });
  
  onDestroy(() => {
    // Usa funções de cancelamento ao invés de EventsOff genérico
    if (unsubTabsUpdated) unsubTabsUpdated();
    if (unsubTabsActivated) unsubTabsActivated();
    if (unsubConversationDeleted) unsubConversationDeleted();
    if (unsubDatabaseReset) unsubDatabaseReset();
    
    window.removeEventListener('keydown', handleGlobalKeyDown, true);
  });
  
  // ========================================
  // Backend Communication
  // ========================================
  
  async function loadTabs() {
    try {
      console.log('🔄 [ChatTabs] Carregando abas do backend...');
      const response = await GetTabs();
      console.log('📦 [ChatTabs] Resposta do backend:', response);
      tabs = response.tabs || [];
      activeTabId = response.active_tab_id || null;
      
      console.log('[ChatTabs] Abas carregadas do backend:', tabs.length, 'Aba ativa:', activeTabId);
      console.log('[ChatTabs] Detalhes das abas:', tabs.map(t => ({ id: t.id, title: t.title, conversation_id: t.conversation_id, is_active: t.is_active })));
    } catch (e) {
      console.error('[ChatTabs] Erro ao carregar abas:', e);
      tabs = [];
    }
  }
  
  // ========================================
  // Event Handlers
  // ========================================
  
  function handleTabsUpdated(data) {
    const newTabs = data.tabs || [];
    const newActiveTabId = data.active_tab_id || null;
    
    // Verifica se as tabs mudaram (comparando IDs)
    const tabIdsChanged = tabs.length !== newTabs.length || 
                          tabs.some((t, i) => t.id !== newTabs[i]?.id);
    
    if (tabIdsChanged) {
      // Tabs foram adicionadas/removidas/reordenadas - substitui array completo
      tabs = newTabs;
    } else {
      // Apenas propriedades mudaram - atualiza in-place sem destruir componentes
      tabs = tabs.map((tab, index) => {
        const newTab = newTabs[index];
        return newTab ? { ...tab, ...newTab } : tab;
      });
    }
    
    activeTabId = newActiveTabId;
  }
  
  function handleTabActivated(data) {
    activeTabId = data.tab_id;
  }
  
  function handleConversationDeleted(data) {
    // Backend já limpou as abas
    loadTabs();
  }
  
  function handleDatabaseReset() {
    console.log('🗑️ [ChatTabs] Banco resetado - limpando todas as guias');
    // Recarrega abas do backend (que serão resetadas)
    loadTabs();
  }
  
  // ========================================
  // User Actions
  // ========================================
  
  function navigateToNextTab() {
    console.log('➡️ [ChatTabsContainer] navigateToNextTab chamado');
    console.log('   Tabs disponíveis:', tabs.length);
    console.log('   Tab ativa atual:', activeTabId);
    console.log('   Todas as tabs:', tabs.map(t => ({ id: t.id, title: t.title, is_active: t.is_active })));
    
    if (tabs.length <= 1) return;
    const currentIndex = tabs.findIndex(t => t.id === activeTabId);
    const nextIndex = (currentIndex + 1) % tabs.length;
    
    console.log('   Index atual:', currentIndex, 'Próximo index:', nextIndex);
    console.log('   Próxima tab:', tabs[nextIndex]);
    
    selectTab(tabs[nextIndex].id);
  }
  
  function navigateToPreviousTab() {
    console.log('⬅️ [ChatTabsContainer] navigateToPreviousTab chamado');
    console.log('   Tabs disponíveis:', tabs.length);
    console.log('   Tab ativa atual:', activeTabId);
    
    if (tabs.length <= 1) return;
    const currentIndex = tabs.findIndex(t => t.id === activeTabId);
    const previousIndex = (currentIndex - 1 + tabs.length) % tabs.length;
    
    console.log('   Index atual:', currentIndex, 'Index anterior:', previousIndex);
    console.log('   Tab anterior:', tabs[previousIndex]);
    
    selectTab(tabs[previousIndex].id);
  }
  
  async function addNewTab() {
    try {
      await CreateTab('Nova conversa', '💬');
    } catch (e) {
      console.error('[ChatTabs] Erro ao criar aba:', e);
      if (e.message?.includes('limite')) {
        alert('Limite de 20 abas atingido');
      }
    }
  }
  
  async function closeTab(tabId) {
    try {
      await CloseTab(tabId);
    } catch (e) {
      console.error('[ChatTabs] Erro ao fechar aba:', e);
    }
  }
  
  async function selectTab(tabId) {
    try {
      await SetActiveTab(tabId);
      
      setTimeout(() => {
        const ref = chatTabRefs[tabId];
        if (ref?.focusInput) ref.focusInput();
      }, 100);
    } catch (e) {
      console.error('[ChatTabs] Erro ao selecionar aba:', e);
    }
  }
  
  // ========================================
  // Public API
  // ========================================
  
  export async function openConversation(conversationId) {
    if (!activeTabId) return;
    
    try {
      await LoadConversationInTab(activeTabId, conversationId);
      
      const ref = chatTabRefs[activeTabId];
      if (ref?.loadConversation) {
        await ref.loadConversation(conversationId);
      }
    } catch (e) {
      console.error('[ChatTabs] Erro ao abrir conversa:', e);
    }
  }
  
  export function startNewConversation() {
    const ref = chatTabRefs[activeTabId];
    if (ref?.clear) ref.clear();
  }
  
  export function focusInput() {
    const ref = chatTabRefs[activeTabId];
    if (ref?.focusInput) ref.focusInput();
  }
</script>

<div class="chat-tabs-container">
  {#key activeTabString}
  <TabPanel
    tabs={tabs.map(t => ({
      id: String(t.id),
      label: t.title,
      icon: t.icon,
      closeable: tabs.length > 1
    }))}
    activeTab={activeTabString}
    closableTabs={true}
    on:tabSelect={e => selectTab(Number(e.detail))}
    on:tabClose={e => closeTab(Number(e.detail))}
    on:closeRequest={e => closeTab(Number(e.detail.tabId))}
    on:tabSwitch={e => selectTab(Number(e.detail.tabId))}
  >
    {#each tabs as tab (tab.id)}
      {@const isTabActive = tab.id === activeTabId}
      <div 
        class="tab-content" 
        class:active={isTabActive}
        style="display: {isTabActive ? 'flex' : 'none'}; height: 100%;"
      >
        <ChatTab
          bind:this={chatTabRefs[tab.id]}
          tabId={String(tab.id)}
          initialConversationId={tab.conversation_id}
          isActive={isTabActive}
          onNewTab={addNewTab}
          {defaultModel}
          {defaultChatParams}
        />
      </div>
    {/each}
  </TabPanel>
  {/key}
</div>

<style>
  .chat-tabs-container {
    height: 100%;
    display: flex;
    flex-direction: column;
  }
  
  .tab-content {
    display: none !important;
    height: 100%;
  }
  
  .tab-content.active {
    display: flex !important;
  }
</style>
