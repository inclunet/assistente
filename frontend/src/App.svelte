<script>
  import { onMount } from 'svelte';
  import { Layout } from './components/layout';
  import { Chat } from './pages/chat';
  import { Settings } from './pages/settings';
  import { ConversationList } from './pages/history';
  import { FAQManager } from './pages/faq';
  import { MemoryManager } from './pages/memory';
  import { AgentManager } from './pages/agents';
  import { OAuthManager } from './pages/oauth';
  import { GetConfig, GetConversation } from '../wailsjs/go/main/App.js';

  // ========================================
  // Mapa de páginas (roteamento idiomático)
  // ========================================
  const pages = {
    chat: { component: Chat, fullWidth: true },
    history: { component: ConversationList },
    faq: { component: FAQManager },
    memories: { component: MemoryManager },
    agents: { component: AgentManager },
    oauth: { component: OAuthManager },
    settings: { component: Settings, wrapper: 'settings' }
  };

  // ========================================
  // Estado global
  // ========================================
  let configLoaded = false;
  let hasApiKey = false;
  let defaultModel = '';
  let defaultChatParams = { temperature: 0.7, max_tokens: 4096, top_p: 1.0 };
  let currentPage = 'chat';
  let currentConversation = null;
  
  // Ref do componente atual (para métodos como focusList, refresh, etc.)
  let currentComponent;

  // ========================================
  // Props dinâmicas por página
  // ========================================
  $: pageProps = {
    chat: {
      hasApiKey,
      defaultModel,
      defaultChatParams,
      conversation: currentConversation
    },
    history: {
      currentConversationId: currentConversation?.id
    },
    faq: {},
    memories: {},
    agents: {},
    oauth: {},
    settings: {}
  };

  // ========================================
  // Handlers de eventos por página
  // ========================================
  // Nota: ConversationList recarrega dados no onMount, então não precisa
  // de refresh explícito quando conversas são atualizadas no Chat.
  const pageEvents = {
    history: {
      select: (e) => { currentConversation = e.detail; currentPage = 'chat'; },
      new: () => startNewConversation()
    },
    settings: {
      saved: () => { hasApiKey = true; currentPage = 'chat'; }
    }
  };

  // ========================================
  // Navegação
  // ========================================
  function navigateTo(page) {
    currentPage = page;
    
    // Foca no grid quando navegar para listas
    setTimeout(() => {
      if (page !== 'chat' && page !== 'settings') {
        currentComponent?.focusList?.();
      }
    }, 100);
  }

  function handleNavigate(event) {
    navigateTo(event.detail);
  }

  function startNewConversation() {
    currentConversation = null;
    currentPage = 'chat';
    setTimeout(() => currentComponent?.startNewConversation?.(), 0);
  }

  // ========================================
  // Inicialização
  // ========================================
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

  // ========================================
  // Helper: dispatch de eventos dinâmico
  // ========================================
  function handlePageEvent(eventName, event) {
    const handler = pageEvents[currentPage]?.[eventName];
    if (handler) handler(event);
  }
</script>

{#if !configLoaded}
  <div class="loading-fullscreen" role="status" aria-busy="true">
    <div class="loading-spinner" aria-hidden="true"></div>
    <p>Carregando...</p>
  </div>
{:else}
  <Layout {currentPage} {hasApiKey} on:navigate={handleNavigate}>
    {@const pageConfig = pages[currentPage]}
    
    {#if pageConfig.fullWidth}
      <!-- Páginas full-width (chat) -->
      <svelte:component
        this={pageConfig.component}
        bind:this={currentComponent}
        {...pageProps[currentPage]}
      />
    {:else if pageConfig.wrapper === 'settings'}
      <!-- Página de configurações com wrapper especial -->
      <div class="page-container">
        <div class="settings-wrapper">
          <h2>⚙️ Configurações</h2>
          <svelte:component
            this={pageConfig.component}
            bind:this={currentComponent}
            {...pageProps[currentPage]}
            on:saved={(e) => handlePageEvent('saved', e)}
          />
          {#if !hasApiKey}
            <p class="settings-notice">
              Configure sua chave de API para começar a usar o assistente.
            </p>
          {/if}
        </div>
      </div>
    {:else}
      <!-- Páginas padrão com container -->
      <div class="page-container">
        <svelte:component
          this={pageConfig.component}
          bind:this={currentComponent}
          {...pageProps[currentPage]}
          on:select={(e) => handlePageEvent('select', e)}
          on:new={(e) => handlePageEvent('new', e)}
        />
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
