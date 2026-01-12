<script>
  import { onMount, tick } from 'svelte';
  import { Layout } from './components/layout';
  import { ChatTabsContainer } from './pages/chat';
  import { Settings } from './pages/settings';
  import { ConversationList } from './pages/history';
  import { FAQManager } from './pages/faq';
  import { MemoryManager } from './pages/memory';
  import { AgentManager } from './pages/agents';
  import { OAuthManager } from './pages/oauth';
  import { GetConfig } from '../wailsjs/go/main/App.js';

  // ========================================
  // Mapa de páginas (roteamento idiomático)
  // ========================================
  const pages = {
    chat: { component: ChatTabsContainer, fullWidth: true },
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
  
  // ID da última conversa (para migração de guias)
  let initialConversationId = null;
  
  // Ref do componente atual (para métodos como focusList, refresh, etc.)
  let currentComponent;
  
  // Ref específica para o ChatTabsContainer
  let chatTabsRef;

  // ========================================
  // Props dinâmicas por página
  // ========================================
  $: pageProps = {
    chat: {
      hasApiKey,
      defaultModel,
      defaultChatParams,
      initialConversationId
    },
    history: {
      currentConversationId: null // Agora gerenciado pelo ChatTabsContainer
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
      select: async (e) => { 
        currentPage = 'chat';
        // Aguarda transição e abre no container de guias
        await tick();
        chatTabsRef?.openConversation(e.detail);
      },
      new: () => startNewConversation()
    },
    settings: {
      saved: async () => { 
        // Recarrega configuração para atualizar hasApiKey e defaultModel
        try {
          const config = await GetConfig();
          hasApiKey = config && config.api_key && config.api_key.length > 0;
          
          if (config.chat_params && config.chat_params.model) {
            defaultModel = config.chat_params.model;
          } else {
            defaultModel = config.default_model || '';
          }
          
          // Só navega para chat se tiver API key e modelo
          if (hasApiKey && defaultModel) {
            currentPage = 'chat';
          }
        } catch (error) {
          console.error('Erro ao recarregar configuração:', error);
        }
      }
    }
  };

  // ========================================
  // Navegação
  // ========================================
  function navigateTo(page) {
    // Bloqueia acesso ao chat se não houver modelo configurado
    if (page === 'chat' && (!hasApiKey || !defaultModel)) {
      showBlockedMessage();
      currentPage = 'settings';
      return;
    }

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
    // Bloqueia se não houver modelo configurado
    if (!hasApiKey || !defaultModel) {
      showBlockedMessage();
      currentPage = 'settings';
      return;
    }

    currentPage = 'chat';
    setTimeout(() => chatTabsRef?.startNewConversation?.(), 0);
  }

  function showBlockedMessage() {
    alert('Você precisa configurar a API e selecionar um modelo LLM padrão antes de acessar o chat. Por favor, complete a configuração primeiro.');
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
      
      // Guarda ID da última conversa para migração (o ChatTabsContainer usa)
      if (config.last_conversation_id) {
        initialConversationId = config.last_conversation_id;
      }
      
      configLoaded = true;
      
      if (!hasApiKey || !defaultModel) {
        currentPage = 'settings';
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
      <!-- Páginas full-width (chat com guias) -->
      <svelte:component
        this={pageConfig.component}
        bind:this={chatTabsRef}
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
