import { writable, derived } from 'svelte/store';

/**
 * Cria stores isoladas para uma conversa (por aba)
 */
export function createConversationStores() {
  const messages = writable([]);
  const conversationId = writable(null);
  const conversationTitle = writable('');
  const isStreaming = writable(false);
  const executingTools = writable([]);
  const toolsMessage = writable(null);
  const showInternalMessages = writable(false);
  const streamingMessageId = writable(null);
  const streamingContent = writable('');

  const hasConversation = derived(conversationId, $id => $id !== null);
  const isEmpty = derived(messages, $msgs => ($msgs || []).length === 0);
  const messageCount = derived(messages, $msgs => ($msgs || []).length);

  const threadedMessages = writable([]);
  messages.subscribe(($msgs) => {
    threadedMessages.update(current => {
      const childrenMap = new Map();
      (current || []).forEach(node => {
        const id = node?.message?.id ?? node?.message?.ID ?? node?.id ?? node?.ID;
        if (id && Array.isArray(node.children) && node.children.length > 0) {
          childrenMap.set(id, node.children);
        }
      });
      return ($msgs || []).map((msg, index) => {
        const id = msg?.id ?? msg?.ID;
        return {
          message: msg,
          children: childrenMap.get(id) || [],
          originalIndex: index
        };
      });
    });
  });

  return {
    // primárias
    messages,
    conversationId,
    conversationTitle,
    isStreaming,
    executingTools,
    toolsMessage,
    showInternalMessages,
    streamingMessageId,
    streamingContent,

    // derivadas
    hasConversation,
    isEmpty,
    messageCount,
    threadedMessages,
  };
}
