/**
 * LLM Chat Adapter
 * 
 * Este arquivo NÃO faz parte do componente chat/.
 * É específico do nosso projeto e faz a ponte entre o backend Wails/LLM
 * e os componentes de chat genéricos.
 * 
 * Uso:
 *   import { createLLMHandlers, convertMessages } from '$lib/llmChatAdapter';
 *   
 *   const handlers = createLLMHandlers({ ... });
 *   const messages = convertMessages(rawMessages);
 */

import { EventsOn, EventsOff } from '../../wailsjs/runtime/runtime.js';

// ========================================
// Conversão de Mensagens
// ========================================

/**
 * Converte mensagens do formato do backend LLM para formato genérico
 * @param {Array} llmMessages - Mensagens do backend
 * @returns {Array} Mensagens no formato do componente chat
 */
export function convertMessages(llmMessages) {
  if (!llmMessages || !Array.isArray(llmMessages)) {
    return [];
  }
  
  return llmMessages.map((msg, index) => convertMessage(msg, index));
}

/**
 * Converte uma única mensagem
 * @param {Object} llmMessage - Mensagem do backend
 * @param {number} index - Índice na lista
 * @returns {Object} Mensagem no formato do componente
 */
export function convertMessage(llmMessage, index = 0) {
  if (!llmMessage) return null;
  
  const message = {
    id: llmMessage.id || llmMessage.ID || `msg_${index}`,
    content: llmMessage.content || '',
    timestamp: llmMessage.created_at || llmMessage.createdAt || null,
    media: convertMedia(llmMessage.media),
    
    // Metadados específicos do LLM (mantidos para uso interno)
    role: llmMessage.role,
    internal: llmMessage.internal,
    toolCalls: llmMessage.tool_calls || llmMessage.toolCalls,
    toolsInfo: llmMessage.toolsInfo,
    isStreaming: llmMessage.isStreaming,
    agent_name: llmMessage.agent_name || llmMessage.agentName,
    tool_name: llmMessage.tool_name || llmMessage.toolName,
  };
  
  const author = getAuthor(llmMessage);
  const isMe = llmMessage.role === 'user';
  
  // Filhos (threads)
  const children = llmMessage.children 
    ? llmMessage.children.map((child, i) => convertMessage(child, i))
    : [];
  
  const childCount = llmMessage.child_count || llmMessage.childCount || children.length || 0;
  
  return {
    message,
    author,
    isMe,
    children,
    childCount,
    isPinned: llmMessage.pinned || llmMessage.isPinned || false,
    isStreaming: llmMessage.isStreaming || false,
  };
}

/**
 * Obtém dados do autor baseado na mensagem
 * @param {Object} msg - Mensagem do backend
 * @returns {Object} Dados do autor
 */
function getAuthor(msg) {
  const role = msg.role;
  const agentName = msg.agent_name || msg.agentName;
  const toolName = msg.tool_name || msg.toolName;
  
  if (role === 'user') {
    return {
      id: 'user',
      name: 'Você',
      avatar: '👤',
      role: 'user',
      color: 'var(--chat-user-border)',
    };
  }
  
  if (role === 'assistant') {
    return {
      id: 'assistant',
      name: 'Assistente',
      avatar: '🤖',
      role: 'assistant',
      color: 'var(--chat-assistant-border)',
    };
  }
  
  if (role === 'agent') {
    return {
      id: agentName || 'agent',
      name: formatAgentName(agentName),
      avatar: '🔧',
      role: 'agent',
      color: 'var(--chat-agent-border)',
    };
  }
  
  if (role === 'tool') {
    return {
      id: toolName || agentName || 'tool',
      name: formatAgentName(toolName || agentName),
      avatar: '📥',
      role: 'tool',
      color: 'var(--chat-tool-border)',
    };
  }
  
  // Fallback
  return {
    id: 'system',
    name: 'Sistema',
    avatar: '⚙️',
    role: 'system',
    color: 'var(--chat-text-muted)',
  };
}

/**
 * Formata nome de agente (snake_case -> Title Case)
 * @param {string} name
 * @returns {string}
 */
function formatAgentName(name) {
  if (!name) return 'Agente';
  return name
    .split('_')
    .map(word => word.charAt(0).toUpperCase() + word.slice(1))
    .join(' ');
}

/**
 * Converte mídia do backend para formato do componente
 * @param {Array} media
 * @returns {Array}
 */
function convertMedia(media) {
  if (!media || !Array.isArray(media)) return [];
  
  return media.map(item => ({
    type: item.type || 'image',
    preview: item.preview || item.url || item.data,
    base64: item.base64 || item.data,
    file: item.file,
    altText: item.altText || item.alt_text || item.description,
    mimeType: item.mimeType || item.mime_type,
  }));
}

// ========================================
// Criação de Handlers
// ========================================

/**
 * Cria handlers para o ChatContainer baseado no nosso backend
 * 
 * @param {Object} config - Configuração
 * @param {Function} config.sendMessage - Função para enviar mensagem
 * @param {Function} config.getChildren - Função para obter filhos de uma mensagem
 * @param {Function} config.updateMessage - Função para atualizar mensagem
 * @param {Function} config.deleteMessage - Função para excluir mensagem
 * @param {Function} config.pinMessage - Função para fixar/desafixar mensagem
 * @param {Object} config.ttsService - Serviço de TTS
 * @param {Object} config.runtime - Runtime do Wails
 * @returns {Object} Handlers para o ChatContainer
 */
export function createLLMHandlers(config) {
  const {
    sendMessage,
    getChildren,
    updateMessage,
    deleteMessage,
    pinMessage,
    ttsService,
    onMessageUpdate,
    onError,
  } = config;
  
  return {
    // === Backend ===
    
    onSend: sendMessage ? async (content, media) => {
      try {
        return await sendMessage(content, media);
      } catch (error) {
        console.error('Erro ao enviar mensagem:', error);
        if (onError) onError(error);
        throw error;
      }
    } : null,
    
    onLoadChildren: getChildren ? async (messageId) => {
      try {
        const children = await getChildren(messageId);
        return convertMessages(children);
      } catch (error) {
        console.error('Erro ao carregar filhos:', error);
        if (onError) onError(error);
        throw error;
      }
    } : null,
    
    onEdit: updateMessage ? async (messageId, newContent) => {
      try {
        await updateMessage(messageId, newContent);
        if (onMessageUpdate) onMessageUpdate();
      } catch (error) {
        console.error('Erro ao editar mensagem:', error);
        if (onError) onError(error);
        throw error;
      }
    } : null,
    
    onDelete: deleteMessage ? async (messageId) => {
      try {
        await deleteMessage(messageId);
        if (onMessageUpdate) onMessageUpdate();
      } catch (error) {
        console.error('Erro ao excluir mensagem:', error);
        if (onError) onError(error);
        throw error;
      }
    } : null,
    
    onPin: pinMessage ? async (messageId, pinned) => {
      try {
        await pinMessage(messageId, pinned);
        if (onMessageUpdate) onMessageUpdate();
      } catch (error) {
        console.error('Erro ao fixar mensagem:', error);
        if (onError) onError(error);
        throw error;
      }
    } : null,
    
    // === TTS ===
    
    onSpeak: ttsService ? (text) => {
      if (ttsService.speak) {
        ttsService.speak(text);
      } else if ('speechSynthesis' in window) {
        const utterance = new SpeechSynthesisUtterance(text);
        if (ttsService.voice) utterance.voice = ttsService.voice;
        if (ttsService.rate) utterance.rate = ttsService.rate;
        if (ttsService.volume) utterance.volume = ttsService.volume;
        speechSynthesis.speak(utterance);
      }
    } : null,
    
    onStopSpeaking: () => {
      if (ttsService?.stop) {
        ttsService.stop();
      } else if ('speechSynthesis' in window) {
        speechSynthesis.cancel();
      }
    },
    
    // === Error Handling ===
    
    onError: onError || ((error) => {
      console.error('Erro no chat:', error);
    }),
  };
}

// ========================================
// Streaming Setup
// ========================================

/**
 * Configura listeners de streaming do Wails
 * 
 * @param {Object} callbacks - Callbacks para eventos de streaming
 * @param {Function} callbacks.onChunk - Chamado quando um chunk chega
 * @param {Function} callbacks.onDone - Chamado quando o streaming termina
 * @param {Function} callbacks.onError - Chamado quando ocorre erro
 * @param {Function} callbacks.onToolStart - Chamado quando uma ferramenta inicia
 * @param {Function} callbacks.onToolEnd - Chamado quando uma ferramenta termina
 * @returns {Function} Função para cleanup
 */
export function setupStreamingListeners(callbacks) {
  const {
    onChunk,
    onDone,
    onError,
    onToolStart,
    onToolEnd,
  } = callbacks;
  
  // Registra listeners
  if (onChunk) EventsOn('chat:stream', onChunk);
  if (onDone) EventsOn('chat:done', onDone);
  if (onError) EventsOn('chat:error', onError);
  if (onToolStart) EventsOn('chat:tool_start', onToolStart);
  if (onToolEnd) EventsOn('chat:tool_end', onToolEnd);
  
  // Retorna função de cleanup
  return () => {
    if (onChunk) EventsOff('chat:stream');
    if (onDone) EventsOff('chat:done');
    if (onError) EventsOff('chat:error');
    if (onToolStart) EventsOff('chat:tool_start');
    if (onToolEnd) EventsOff('chat:tool_end');
  };
}

// ========================================
// Utilidades
// ========================================

/**
 * Converte mídia pendente para o formato do backend
 * @param {Array} pendingMedia - Mídia pendente do componente
 * @returns {Array} Mídia no formato do backend
 */
export function convertMediaForBackend(pendingMedia) {
  if (!pendingMedia || !Array.isArray(pendingMedia)) return [];
  
  return pendingMedia.map(item => ({
    type: item.type,
    data: item.base64 || item.preview?.replace(/^data:[^;]+;base64,/, ''),
    file_name: item.file?.name,
    mime_type: item.file?.type || item.mimeType,
    alt_text: item.altText,
  }));
}

/**
 * Verifica se a mensagem precisa de scroll automático
 * @param {Object} message - Mensagem
 * @returns {boolean}
 */
export function shouldAutoScroll(message) {
  // Scroll para mensagens do usuário ou novas do assistente
  return message.role === 'user' || 
         message.role === 'assistant' ||
         message.isStreaming;
}

/**
 * Filtra mensagens internas baseado na configuração
 * @param {Array} messages - Mensagens
 * @param {boolean} showInternal - Mostrar mensagens internas?
 * @returns {Array}
 */
export function filterMessages(messages, showInternal = false) {
  if (showInternal) return messages;
  
  return messages.filter(msg => {
    const message = msg.message || msg;
    return !message.internal;
  });
}

