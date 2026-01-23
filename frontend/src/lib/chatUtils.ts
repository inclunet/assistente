/**
 * Chat Utilities - Funções utilitárias para processamento de mensagens
 */

import { ToolCall, Message, MessageNode } from '../store/chatStore';

/**
 * Parseia tool_calls de uma mensagem
 * Aceita string JSON ou objeto
 */
export function parseToolCalls(toolCalls: string | ToolCall[] | null | undefined): ToolCall[] | null {
  if (!toolCalls) return null;
  
  if (typeof toolCalls === 'string') {
    try {
      return JSON.parse(toolCalls);
    } catch (e) {
      console.warn('[parseToolCalls] Erro ao parsear:', e);
      return null;
    }
  }
  
  return toolCalls;
}

/**
 * Formata nome de agente para exibição
 * Converte snake_case para Title Case
 */
export function formatAgentName(name: string | undefined): string {
  if (!name) return 'Agente';
  
  return name
    .split('_')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
}

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
 * Obtém os nomes das tools sendo executadas
 */
export function getToolCallNames(toolCalls: ToolCall[] | null | undefined): string {
  if (!toolCalls || toolCalls.length === 0) return '';
  
  const names = toolCalls
    .map(tc => tc.function?.name || 'unknown')
    .join(', ');
  
  return names;
}

/**
 * Verifica se uma mensagem é de um agente
 */
export function isAgentMessage(message: Message): boolean {
  return !!message.agentName && message.agentName.trim() !== '';
}
