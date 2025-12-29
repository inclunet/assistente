<script>
  import { onMount } from 'svelte';
  import Layout from './components/Layout.svelte';
  import Chat from './components/Chat.svelte';
  import Settings from './components/Settings.svelte';
  import ConversationList from './components/ConversationList.svelte';
  import FAQManager from './components/FAQManager.svelte';
  import MemoryManager from './components/MemoryManager.svelte';
  import AgentManager from './components/AgentManager.svelte';
  import OAuthManager from './components/OAuthManager.svelte';
  import { GetConfig, GetConversation } from '../wailsjs/go/main/App.js';

  // Estado global
  let configLoaded = false;
  let hasApiKey = false;
  let defaultModel = '';
  let defaultChatParams = { temperature: 0.7, max_tokens: 4096, top_p: 1.0 };
  let currentPage = 'chat';
  let currentConversation = null;
  
  // Refs dos componentes
  let chatComponent;
  let conversationListComponent;
  let faqManagerComponent;
  let memoryManagerComponent;
  let agentManagerComponent;

  // Atalhos globais são gerenciados pelo Topbar (Alt+Key)
  // Atalhos locais (Ctrl+Key) são gerenciados por cada componente

  function navigateTo(page) {
    currentPage = page;
    
    // Foca no grid quando navegar para listas
    setTimeout(() => {
      switch (page) {
        case 'history':
          conversationListComponent?.focusList();
          break;
        case 'faq':
          faqManagerComponent?.focusList();
          break;
        case 'memories':
          memoryManagerComponent?.focusList();
          break;
        case 'agents':
          agentManagerComponent?.focusList();
          break;
      }
    }, 100);
  }

  function handleNavigate(event) {
    navigateTo(event.detail);
  }

  function startNewConversation() {
    currentConversation = null;
    currentPage = 'chat';
    if (chatComponent) {
      chatComponent.startNewConversation();
    }
  }

  onMount(async () => {
    try {
      const config = await GetConfig();
      hasApiKey = config && config.api_key && config.api_key.length > 0;
      
      // Carregar modelo e parâmetros (com retrocompatibilidade)
      if (config.chat_params && config.chat_params.model) {
        defaultModel = config.chat_params.model;
        defaultChatParams = {
          temperature: config.chat_params.temperature || 0.7,
          max_tokens: config.chat_params.max_tokens || 4096,
          top_p: config.chat_params.top_p || 1.0
        };
      } else {
        defaultModel = config.default_model || '';
      }
      
      configLoaded = true;
      
      if (!hasApiKey) {
        currentPage = 'settings';
      } else if (config.last_conversation_id) {
        try {
          const lastConv = await GetConversation(config.last_conversation_id);
          if (lastConv) {
            currentConversation = lastConv;
          }
        } catch (e) {
          // Conversa não existe mais, ignora
        }
      }
    } catch (error) {
      configLoaded = true;
      currentPage = 'settings';
    }
  });


  function handleSettingsSaved() {
    hasApiKey = true;
    currentPage = 'chat';
  }

  function handleSelectConversation(event) {
    currentConversation = event.detail;
    currentPage = 'chat';
  }

  function handleNewConversation() {
    startNewConversation();
  }

  function handleConversationUpdated() {
    if (conversationListComponent) {
      conversationListComponent.refresh();
    }
  }
</script>

{#if !configLoaded}
  <div class="loading-fullscreen" role="status" aria-busy="true">
    <div class="loading-spinner" aria-hidden="true"></div>
    <p>Carregando...</p>
  </div>
{:else}
  <Layout {currentPage} {hasApiKey} on:navigate={handleNavigate}>
    {#if currentPage === 'chat'}
      <Chat 
        bind:this={chatComponent}
        {hasApiKey} 
        {defaultModel}
        {defaultChatParams}
        conversation={currentConversation}
        on:conversationUpdated={handleConversationUpdated}
      />
    {:else if currentPage === 'history'}
      <div class="page-container">
        <ConversationList 
          bind:this={conversationListComponent}
          currentConversationId={currentConversation?.id}
          on:select={handleSelectConversation}
          on:new={handleNewConversation}
        />
      </div>
    {:else if currentPage === 'faq'}
      <div class="page-container">
        <FAQManager bind:this={faqManagerComponent} />
      </div>
    {:else if currentPage === 'memories'}
      <div class="page-container">
        <MemoryManager bind:this={memoryManagerComponent} />
      </div>
    {:else if currentPage === 'agents'}
      <div class="page-container">
        <AgentManager bind:this={agentManagerComponent} />
      </div>
    {:else if currentPage === 'oauth'}
      <div class="page-container">
        <OAuthManager />
      </div>
    {:else if currentPage === 'settings'}
      <div class="page-container">
        <div class="settings-wrapper">
          <h2>⚙️ Configurações</h2>
          <Settings on:saved={handleSettingsSaved} />
          {#if !hasApiKey}
            <p class="settings-notice">
              Configure sua chave de API para começar a usar o assistente.
            </p>
          {/if}
        </div>
      </div>
    {/if}
  </Layout>
{/if}

<style>
  .loading-fullscreen {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    height: 100vh;
    gap: var(--spacing-md);
    background: var(--color-bg-primary, #121212);
  }

  .loading-fullscreen p {
    color: var(--color-text-secondary);
  }
  
  .loading-spinner {
    width: 40px;
    height: 40px;
    border: 3px solid var(--color-border, #404040);
    border-top-color: var(--color-accent, #58a6ff);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }
  
  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .page-container {
    flex: 1;
    overflow: auto;
    padding: var(--spacing-lg, 24px);
    max-width: 1000px;
    margin: 0 auto;
    width: 100%;
  }
  
  .settings-wrapper {
    max-width: 600px;
  }
  
  .settings-wrapper h2 {
    margin: 0 0 var(--spacing-lg);
    font-size: var(--font-size-xl);
    color: var(--color-text-primary);
  }
  
  .settings-notice {
    margin-top: var(--spacing-lg);
    padding: var(--spacing-md);
    background: rgba(88, 166, 255, 0.1);
    border-radius: var(--border-radius);
    color: var(--color-text-secondary);
  }
</style>
