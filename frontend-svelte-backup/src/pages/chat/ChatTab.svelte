<script>
  import { createEventDispatcher } from 'svelte';
  import Chat from './Chat.svelte';
  
  const dispatch = createEventDispatcher();
  
  // ========================================
  // Props
  // ========================================
  
  /** ID único desta guia */
  export let tabId;
  
  /** ID da conversa para carregar inicialmente (opcional) */
  export let initialConversationId = null;
  
  console.log(`[ChatTab ${tabId}] Props recebidas:`, { initialConversationId });
  
  /** Props de configuração do chat */
  export let defaultModel = '';
  export let defaultChatParams = { temperature: 0.7, max_tokens: 4096, top_p: 1.0 };
  
  /** Flag indicando se componente está ativo (visível) */
  export let isActive = true;
  
  /** Callback para nova aba */
  export let onNewTab = null;
  
  // Referência ao componente Chat para chamar métodos públicos
  let chatComponent;
  
  console.log(`[ChatTab ${tabId}] Componente criado`);
  
  // ========================================
  // API Pública
  // ========================================
  
  /**
   * Carrega uma conversa nesta guia
   * @param {Object} conversation - Objeto da conversa com id e title
   */
  export async function loadConversation(conversation) {
    console.log(`[ChatTab ${tabId}] loadConversation chamado:`, conversation);
    if (chatComponent) {
      await chatComponent.loadConversation(conversation);
    }
  }
  
  /**
   * Limpa o chat desta guia (nova conversa)
   */
  export function clear() {
    console.log(`[ChatTab ${tabId}] clear chamado`);
    if (chatComponent) {
      chatComponent.clear();
    }
  }
  
  // Event handlers
  function handleConversationCreated(event) {
    console.log(`[ChatTab ${tabId}] Conversa criada:`, event.detail);
    dispatch('conversationCreated', event.detail);
  }
  
  function handleConversationUpdated(event) {
    console.log(`[ChatTab ${tabId}] Conversa atualizada:`, event.detail);
    dispatch('conversationUpdated', event.detail);
  }
  
  function handleTitleChanged(event) {
    console.log(`[ChatTab ${tabId}] Título mudou:`, event.detail);
    dispatch('titleChanged', event.detail);
  }
  
  function handleConversationSelected(event) {
    console.log(`[ChatTab ${tabId}] Conversa selecionada:`, event.detail);
    dispatch('conversationSelected', event.detail);
  }
</script>

<!--
  Passa apenas configurações para Chat.svelte
  Chat.svelte gerencia suas próprias stores e controller
-->
<Chat 
  bind:this={chatComponent}
  {tabId}
  {initialConversationId}
  {defaultModel}
  {defaultChatParams}
  {isActive}
  {onNewTab}
  
  on:conversationCreated={handleConversationCreated}
  on:conversationUpdated={handleConversationUpdated}
  on:titleChanged={handleTitleChanged}
  on:conversationSelected={handleConversationSelected}
/>
