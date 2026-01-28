<script>
  import { onMount, onDestroy } from 'svelte';
  import { EventsOn, EventsOff } from '../../../wailsjs/runtime/runtime.js';
  import { SendMessage, GetMessages } from '../../../wailsjs/go/main/App.js';
  import ChatContainer from '../../components/chat/wrappers/ChatContainer.svelte';
  import ChatInput from '../../components/chat/core/input/ChatInput.svelte';
  import SendButton from '../../components/chat/core/input/SendButton.svelte';

  // Props
  export let tabId;
  export let initialConversationId = null;
  export let hasApiKey = false;
  export let isActive = true;

  // Estado SIMPLES
  let messages = [];
  let conversationId = initialConversationId;
  let inputMessage = '';
  let isLoading = false;
  let streamingContent = '';
  let currentStreamMessageId = null;

  // Carrega mensagens do backend
  async function loadMessages() {
    if (!conversationId) {
      messages = [];
      return;
    }
    
    try {
      const result = await GetMessages(conversationId);
      messages = result || [];
      console.log(`[Chat ${tabId}] ✅ Carregou ${messages.length} mensagens`);
    } catch (err) {
      console.error(`[Chat ${tabId}] ❌ Erro ao carregar mensagens:`, err);
    }
  }

  // Envia mensagem
  async function handleSubmit() {
    if (!inputMessage.trim() || isLoading) return;

    const userText = inputMessage.trim();
    inputMessage = '';
    isLoading = true;
    streamingContent = '';

    try {
      const result = await SendMessage({
        conversationId: conversationId,
        content: userText,
        media: [],
        useTools: true,
        chatParams: {
          temperature: 0.7,
          max_tokens: 4096,
          top_p: 1.0
        }
      });

      console.log(`[Chat ${tabId}] ✅ Mensagem enviada, conversation:`, result.conversationId);
      
      // Atualiza conversationId se for nova conversa
      if (!conversationId) {
        conversationId = result.conversationId;
      }

    } catch (err) {
      console.error(`[Chat ${tabId}] ❌ Erro ao enviar:`, err);
      isLoading = false;
    }
  }

  // Escuta eventos do backend
  onMount(() => {
    console.log(`[Chat ${tabId}] 🚀 Montado`);

    // Carrega mensagens iniciais
    if (conversationId) {
      loadMessages();
    }

    // Evento: backend salvou a mensagem do usuário
    EventsOn('chat:messages_ready', (data) => {
      console.log(`[Chat ${tabId}] 📨 messages_ready:`, data);
      loadMessages(); // Recarrega tudo do backend
    });

    // Evento: streaming de resposta
    EventsOn('chat:stream', (event) => {
      if (event.type === 'content') {
        // Acumula conteúdo
        streamingContent += event.content || '';
        
        // Atualiza mensagem temporária
        if (currentStreamMessageId) {
          messages = messages.map(m => 
            m.id === currentStreamMessageId 
              ? { ...m, content: streamingContent }
              : m
          );
        } else {
          // Cria mensagem temporária
          currentStreamMessageId = `temp-${Date.now()}`;
          messages = [...messages, {
            id: currentStreamMessageId,
            role: 'assistant',
            content: streamingContent,
            conversationId: conversationId,
            createdAt: new Date().toISOString()
          }];
        }
      }
    });

    // Evento: streaming terminou
    EventsOn('chat:done', (data) => {
      console.log(`[Chat ${tabId}] ✅ Streaming done`);
      isLoading = false;
      streamingContent = '';
      currentStreamMessageId = null;
      
      // Recarrega mensagens finais do backend
      loadMessages();
    });

    // Evento: erro
    EventsOn('chat:error', (error) => {
      console.error(`[Chat ${tabId}] ❌ Erro:`, error);
      isLoading = false;
    });
  });

  onDestroy(() => {
    console.log(`[Chat ${tabId}] 🗑️ Desmontado`);
    EventsOff('chat:messages_ready');
    EventsOff('chat:stream');
    EventsOff('chat:done');
    EventsOff('chat:error');
  });

  // API pública
  export async function loadConversation(conversation) {
    conversationId = conversation?.id;
    await loadMessages();
  }

  export function clear() {
    conversationId = null;
    messages = [];
    inputMessage = '';
    isLoading = false;
  }
</script>

<div class="chat-wrapper">
  <ChatContainer
    messages={messages}
    threadedMessages={[]}
    autoFocusInput={isActive}
  />

  <div class="input-area">
    <ChatInput
      bind:value={inputMessage}
      on:submit={handleSubmit}
      disabled={isLoading || !hasApiKey}
      placeholder={isLoading ? "Aguardando resposta..." : "Digite sua mensagem..."}
    />
    <SendButton
      on:click={handleSubmit}
      disabled={!inputMessage.trim() || isLoading || !hasApiKey}
      isLoading={isLoading}
    />
  </div>
</div>

<style>
  .chat-wrapper {
    display: flex;
    flex-direction: column;
    height: 100%;
  }

  .input-area {
    display: flex;
    gap: 8px;
    padding: 16px;
    border-top: 1px solid var(--border-color);
  }
</style>
