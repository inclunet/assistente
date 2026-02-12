/**
 * Chat Utilities - Funções utilitárias para processamento de mensagens
 */

import { Message, MessageNode } from '../store/chatStore';

/**
 * Extrai mensagens flat de uma estrutura de threads
 */
export function extractMessagesFromThreads(threads: MessageNode[]): Message[] {
  if (!threads || !threads.length) return [];
  
  const messages: Message[] = [];
  
  function traverse(nodes: MessageNode[]) {
    for (const node of nodes) {
      if (node.message) {
        messages.push(node.message);
      }
      if (node.children && node.children.length > 0) {
        traverse(node.children);
      }
    }
  }
  
  traverse(threads);
  return messages;
}

/**
 * Verifica se uma mensagem é de um agente
 * Note: agentName was removed from EnrichedMessage, so this always returns false
 */
export function isAgentMessage(_message: Message): boolean {
  return false;
}
