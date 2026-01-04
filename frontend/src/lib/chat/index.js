/**
 * Chat Module - Serviços de chat
 * 
 * Services:
 *   - messageService: Gerenciamento de mensagens (singleton)
 *   - createMessageService: Factory para criar instâncias isoladas (multi-guias)
 * 
 * Svelte Stores (estado reativo do singleton):
 *   - conversationId, conversationTitle, conversationData
 *   - messages, threadedMessages
 *   - isStreaming, executingTools, toolsMessage
 *   - showInternalMessages
 *   - hasConversation, isEmpty, messageCount (derived)
 * 
 * Uso com múltiplas guias:
 *   import { createMessageService } from '$lib/chat';
 *   const service = createMessageService();
 *   const { messages, isStreaming } = service.stores;
 */

// Serviço singleton e factory
export { messageService, MessageService, createMessageService } from './message-service.js';

// Svelte Stores (estado reativo do singleton para retrocompatibilidade)
export {
  conversationId,
  conversationTitle,
  conversationData,
  messages,
  threadedMessages,
  showInternalMessages,
  streamingMessageId,
  streamingContent,
  isStreaming,
  executingTools,
  toolsMessage,
  hasConversation,
  isEmpty,
  messageCount
} from './message-service.js';

// Funções utilitárias
export { parseToolCalls, formatAgentName, convertMessageNode } from './message-service.js';



