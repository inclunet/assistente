/**
 * Chat Utilities - Funções utilitárias para chat
 * 
 * Funções helper para processamento de mensagens, formatação, etc.
 */

/**
 * Parseia tool_calls de uma mensagem
 * Aceita string JSON ou objeto
 * 
 * @param {string|object|null} toolCalls - Tool calls para parsear
 * @returns {object[]|null} Array de tool calls ou null
 */
export function parseToolCalls(toolCalls) {
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
 * 
 * @param {string} name - Nome do agente em snake_case
 * @returns {string} Nome formatado
 * 
 * @example
 * formatAgentName('image_agent') // 'Image Agent'
 * formatAgentName('faq_agent') // 'Faq Agent'
 */
export function formatAgentName(name) {
  if (!name) return 'Agente';
  
  return name
    .split('_')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
}

/**
 * Converte MessageNode do backend para formato do frontend
 * Normaliza campos PascalCase → camelCase e estrutura hierárquica
 * 
 * @param {object} node - Nó de mensagem do backend
 * @param {number} index - Índice do nó no array pai
 * @param {number} level - Nível de profundidade na árvore
 * @returns {object} Nó normalizado
 */
export function convertMessageNode(node, index = 0, level = 0) {
  if (!node) return null;
  
  const m = node.message || node.Message || {};
  
  // Parseia tool_calls se for string
  let toolCalls = null;
  const rawToolCalls = m.tool_calls || m.ToolCalls || m.toolCalls;
  if (rawToolCalls) {
    toolCalls = parseToolCalls(rawToolCalls);
  }
  
  // Extrai toolName do primeiro tool call
  let toolName = '';
  if (toolCalls && toolCalls.length > 0) {
    toolName = toolCalls[0].function?.name || 
               toolCalls[0].Function?.Name || 
               '';
  }
  
  // Converte children recursivamente
  const rawChildren = node.children || node.Children || [];
  const children = rawChildren.map((child, childIndex) => 
    convertMessageNode(child, childIndex, level + 1)
  );
  
  // Retorna nó normalizado
  return {
    __normalized: true,
    message: {
      id: m.id || m.ID,
      parentId: m.parent_id || m.ParentID || m.parentId,
      role: m.role || m.Role,
      content: m.content || m.Content || '',
      toolCalls: toolCalls,
      toolCallId: m.tool_call_id || m.ToolCallID || m.toolCallId,
      agentName: m.agent_name || m.AgentName || m.agentName,
      toolName: toolName,
      internal: (m.parent_id || m.ParentID) != null || 
                m.internal || 
                m.Internal || 
                false,
      model: m.model || m.Model,
      promptTokens: m.prompt_tokens || m.PromptTokens,
      completionTokens: m.completion_tokens || m.CompletionTokens,
      totalTokens: m.total_tokens || m.TotalTokens,
      isStreaming: m.isStreaming || m.IsStreaming || node.isStreaming || false,
      toolsInfo: m.toolsInfo || null,
      media: m.media || m.Media || null,
      pinned: m.pinned || m.Pinned || false
    },
    agentName: m.agent_name || m.AgentName || m.agentName,
    toolName: toolName,
    level: node.level ?? node.Level ?? level,
    originalIndex: node.originalIndex ?? index,
    children,
    childCount: node.child_count ?? 
                node.ChildCount ?? 
                node.childCount ?? 
                children.length
  };
}

/**
 * Extrai mensagens flat de uma estrutura de threads
 * 
 * @param {object[]} threads - Array de threads
 * @returns {object[]} Array flat de mensagens
 */
export function extractMessagesFromThreads(threads) {
  if (!threads || !threads.length) return [];
  
  const messages = [];
  
  function traverse(nodes) {
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
 * Normaliza threads recursivamente aplicando level e originalIndex
 * 
 * @param {object[]} threads - Array de threads
 * @param {number} level - Nível de profundidade atual
 * @returns {object[]} Threads normalizadas
 */
export function normalizeThreads(threads, level = 0) {
  if (!threads || !threads.length) return [];
  
  return threads.map((node, index) => {
    // Se já está normalizado, apenas atualiza children
    if (node?.__normalized) {
      const nodeLevel = node.level ?? level;
      const normalizedChildren = normalizeThreads(node.children, nodeLevel + 1);
      
      return {
        ...node,
        level: nodeLevel,
        originalIndex: node.originalIndex ?? index,
        children: normalizedChildren,
        childCount: normalizedChildren.length
      };
    }
    
    // Converte nó não normalizado
    return convertMessageNode(node, index, level);
  });
}
