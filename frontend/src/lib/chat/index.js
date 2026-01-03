/**
 * Chat Module - Serviços de chat
 * 
 * Services:
 *   - messageService: Gerenciamento de mensagens (singleton)
 * 
 * Svelte Stores (estado reativo):
 *   - conversationId, conversationTitle, conversationData
 *   - messages, threadedMessages
 *   - isStreaming, executingTools, toolsMessage
 *   - showInternalMessages
 *   - hasConversation, isEmpty, messageCount (derived)
 */

// Serviço singleton
export { messageService, MessageService } from './message-service.js';

// Svelte Stores (estado reativo)
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



