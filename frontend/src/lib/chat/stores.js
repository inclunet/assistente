/**
 * Chat Stores - Factory de stores isoladas para cada guia
 * 
 * Cada guia de chat tem seu próprio conjunto de stores completamente independente.
 * Isso garante isolamento total entre guias e compatibilidade com reatividade Svelte.
 */

import { writable, derived } from 'svelte/store';

/**
 * Cria um conjunto isolado de stores para uma guia de chat
 * 
 * @returns {Object} Objeto contendo todas as stores
 */
export function createChatStores() {
  // ========================================
  // STORES PRIMÁRIAS
  // ========================================
  
  /** Array de mensagens da conversa */
  const messages = writable([]);
  
  /** ID da conversa atual (null = nova conversa) */
  const conversationId = writable(null);
  
  /** Título da conversa */
  const conversationTitle = writable('');
  
  /** Indica se está em streaming */
  const isStreaming = writable(false);
  
  /** Lista de tools sendo executadas */
  const executingTools = writable([]);
  
  /** Mensagem de status das tools */
  const toolsMessage = writable(null);
  
  /** Exibir mensagens internas (tool calls, agent) */
  const showInternalMessages = writable(false);
  
  /** ID da mensagem sendo streamada */
  const streamingMessageId = writable(null);
  
  /** Conteúdo acumulado do streaming */
  const streamingContent = writable('');
  
  // ========================================
  // STORES DERIVADAS (calculadas automaticamente)
  // ========================================
  
  /** Indica se existe uma conversa carregada */
  const hasConversation = derived(
    conversationId,
    $id => $id !== null
  );
  
  /** Indica se a conversa está vazia */
  const isEmpty = derived(
    messages,
    $msgs => $msgs.length === 0
  );
  
  /** Número de mensagens */
  const messageCount = derived(
    messages,
    $msgs => $msgs.length
  );
  
  /** Mensagens em formato de árvore (threads) - WRITABLE para lazy loading */
  const threadedMessages = writable([]);
  
  // Sincroniza com messages, mas preserva children existentes
  messages.subscribe(($msgs) => {
    threadedMessages.update(currentThreaded => {
      // Cria mapa de children existentes por message.id
      const childrenMap = new Map();
      currentThreaded.forEach(node => {
        const msgId = node.message?.id || node.message?.ID;
        if (msgId && node.children?.length > 0) {
          childrenMap.set(msgId, node.children);
        }
      });
      
      // Cria novos nodes, preservando children quando existir
      return $msgs.map((msg, index) => {
        const msgId = msg.id || msg.ID;
        return {
          message: msg,
          children: childrenMap.get(msgId) || [], // Preserva children ou usa []
          originalIndex: index
        };
      });
    });
  });
  
  return {
    // Primárias
    messages,
    conversationId,
    conversationTitle,
    isStreaming,
    executingTools,
    toolsMessage,
    showInternalMessages,
    streamingMessageId,
    streamingContent,
    
    // Derivadas
    hasConversation,
    isEmpty,
    messageCount,
    threadedMessages
  };
}
