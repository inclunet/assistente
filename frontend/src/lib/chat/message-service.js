/**
 * MessageService - Serviço de gerenciamento de mensagens de chat
 * 
 * Responsabilidades:
 * - CRUD de mensagens (save, load, update)
 * - Transformação de dados (threads, flat)
 * - Gerenciamento de streaming
 * - Stores reativos Svelte
 * 
 * Uso com Svelte Stores (recomendado):
 *   import { 
 *     messages, 
 *     conversationId, 
 *     isStreaming,
 *     messageService 
 *   } from '$lib/chat/message-service.js';
 *   
 *   // No template - reativo automático!
 *   {#each $messages as msg}...{/each}
 *   {#if $isStreaming}Loading...{/if}
 *   
 *   // Ações
 *   await messageService.loadConversation(conv);
 *   await messageService.saveMessage('user', 'Olá!');
 */

import { writable, derived, get } from 'svelte/store';

// ========================================
// Svelte Stores (estado reativo)
// ========================================

/** ID da conversa atual */
export const conversationId = writable(null);

/** Título da conversa atual */
export const conversationTitle = writable('');

/** Dados completos da conversa (com threads) */
export const conversationData = writable(null);

/** Mensagens flat para compatibilidade */
export const messages = writable([]);

/** Mensagens organizadas em threads */
export const threadedMessages = writable([]);

/** Exibir mensagens internas (tool calls, agent) */
export const showInternalMessages = writable(false);

/** ID da mensagem sendo streamada */
export const streamingMessageId = writable(null);

/** Conteúdo acumulado do streaming */
export const streamingContent = writable('');

/** Indica se está em streaming */
export const isStreaming = writable(false);

/** Indica se está executando tools */
export const executingTools = writable(false);

/** Mensagem de status das tools */
export const toolsMessage = writable('');

/** Indica se há uma conversa carregada */
export const hasConversation = derived(conversationId, $id => $id !== null);

/** Indica se a conversa está vazia */
export const isEmpty = derived(messages, $msgs => $msgs.length === 0);

/** Contagem de mensagens */
export const messageCount = derived(messages, $msgs => $msgs.length);

// ========================================
// Imports dinâmicos do Wails
// ========================================
let CreateConversation = null;
let AddMessage = null;
let AddMessageWithTokens = null;
let AddMessageWithMedia = null;
let AddMessageWithTokensAndMedia = null;
let GetConversationInfo = null;
let GetMessages = null;
let UpdateConversationModel = null;
let UpdateConversationSettings = null;
let SetLastConversation = null;
let EventsOn = null;
let EventsOff = null;

async function loadWailsFunctions() {
  try {
    const wails = await import('../../../wailsjs/go/main/App.js');
    CreateConversation = wails.CreateConversation;
    AddMessage = wails.AddMessage;
    AddMessageWithTokens = wails.AddMessageWithTokens;
    AddMessageWithMedia = wails.AddMessageWithMedia;
    AddMessageWithTokensAndMedia = wails.AddMessageWithTokensAndMedia;
    GetConversationInfo = wails.GetConversationInfo;
    GetMessages = wails.GetMessages;
    UpdateConversationModel = wails.UpdateConversationModel;
    UpdateConversationSettings = wails.UpdateConversationSettings;
    SetLastConversation = wails.SetLastConversation;
  } catch (e) {
    console.warn('Wails functions not available:', e.message);
  }
  
  // Carrega runtime events
  try {
    const runtime = await import('../../../wailsjs/runtime/runtime.js');
    EventsOn = runtime.EventsOn;
    EventsOff = runtime.EventsOff;
  } catch (e) {
    console.warn('Wails runtime not available:', e.message);
  }
}

/**
 * Serviço de mensagens de chat
 */
class MessageService extends EventTarget {
  constructor() {
    super();
    
    // Stores são exportados como módulo-level
    // O serviço usa get() para ler e .set() para atualizar
    
    // Event subscriptions
    this._eventUnsubscribers = [];
    this._boundToBackend = false;
    
    // Listeners por componente (para remoção segura)
    // Map<componentId, Map<eventType, handler>>
    this._componentListeners = new Map();
    
    // Debounce para messagesUpdated durante streaming (evita re-renders excessivos)
    this._messagesUpdateDebounceTimer = null;
    this._pendingMessagesUpdate = null;
    
    // Inicialização
    this._initialized = false;
    this._initPromise = this._init();
  }
  
  /**
   * Dispara messagesUpdated com debounce durante streaming
   * Isso evita re-renders excessivos quando muitas mensagens de agente chegam rapidamente
   */
  _dispatchMessagesUpdatedDebounced(data) {
    this._pendingMessagesUpdate = data;
    
    // Se streaming ativo, usa debounce maior para reduzir re-renders
    if (get(isStreaming)) {
      if (this._messagesUpdateDebounceTimer) {
        clearTimeout(this._messagesUpdateDebounceTimer);
      }
      this._messagesUpdateDebounceTimer = setTimeout(() => {
        if (this._pendingMessagesUpdate) {
          this._dispatchEvent('messagesUpdated', this._pendingMessagesUpdate);
          this._pendingMessagesUpdate = null;
        }
        this._messagesUpdateDebounceTimer = null;
      }, 500); // 500ms de debounce durante streaming para permitir navegação fluida
    } else {
      // Sem streaming, dispara imediatamente
      this._dispatchEvent('messagesUpdated', data);
      this._pendingMessagesUpdate = null;
    }
  }
  
  /**
   * Adiciona listener associado a um componente específico
   * Isso permite remover todos os listeners de um componente de uma vez
   */
  addComponentListener(componentId, eventType, handler) {
    if (!this._componentListeners.has(componentId)) {
      this._componentListeners.set(componentId, new Map());
    }
    
    const componentMap = this._componentListeners.get(componentId);
    
    // Remove listener anterior do mesmo tipo se existir
    if (componentMap.has(eventType)) {
      const oldHandler = componentMap.get(eventType);
      this.removeEventListener(eventType, oldHandler);
    }
    
    // Adiciona novo listener
    componentMap.set(eventType, handler);
    this.addEventListener(eventType, handler);
  }
  
  /**
   * Remove todos os listeners de um componente específico
   */
  removeComponentListeners(componentId) {
    const componentMap = this._componentListeners.get(componentId);
    if (!componentMap) return;
    
    for (const [eventType, handler] of componentMap) {
      this.removeEventListener(eventType, handler);
    }
    
    this._componentListeners.delete(componentId);
    console.log('[MessageService] Removed all listeners for component:', componentId);
  }
  
  async _init() {
    await loadWailsFunctions();
    this._initialized = true;
  }
  
  async ready() {
    await this._initPromise;
  }
  
  // === Backend Event Binding ===
  
  /**
   * Conecta aos eventos do backend (Wails)
   * Deve ser chamado uma vez no onMount do componente principal
   * Suporta hot reload: se já está bound, faz unbind primeiro
   */
  bindBackendEvents() {
    if (!EventsOn) {
      console.warn('[MessageService] EventsOn not available');
      return;
    }
    
    // Se já está bound, faz unbind primeiro (suporta hot reload)
    if (this._boundToBackend) {
      console.log('[MessageService] Already bound, rebinding...');
      this.unbindBackendEvents();
    }
    
    // Sistema de streaming unificado (chat:stream é emitido pelo backend)
    // NOTA: chat:chunk era o sistema antigo, removido para evitar duplicação
    this._eventUnsubscribers.push(
      EventsOn('chat:stream', (event) => this._handleStreamEvent(event))
    );
    
    // Streaming finalizado
    this._eventUnsubscribers.push(
      EventsOn('chat:done', (data) => this._handleDone(data))
    );
    
    // Erros
    this._eventUnsubscribers.push(
      EventsOn('chat:error', (error) => this._handleError(error))
    );
    
    // Execução de ferramentas
    this._eventUnsubscribers.push(
      EventsOn('chat:tools', (data) => this._handleToolsExecution(data))
    );
    
    // Resultados de ferramentas
    this._eventUnsubscribers.push(
      EventsOn('chat:tool_results', (data) => this._handleToolResults(data))
    );
    
    // Conversa criada pelo backend
    this._eventUnsubscribers.push(
      EventsOn('chat:conversation_created', (data) => this._handleConversationCreated(data))
    );
    
    // Mensagens prontas (após salvar)
    this._eventUnsubscribers.push(
      EventsOn('chat:messages_ready', (data) => this._handleMessagesReady(data))
    );
    
    // Mensagens internas (tool calls, tool results)
    this._eventUnsubscribers.push(
      EventsOn('chat:internal_message', (data) => this._handleInternalMessage(data))
    );
    
    // Mensagens de agentes em tempo real
    this._eventUnsubscribers.push(
      EventsOn('chat:agent_message', (data) => this._handleAgentMessage(data))
    );
    
    this._boundToBackend = true;
    console.log('[MessageService] Backend events bound successfully');
  }
  
  /**
   * Desconecta dos eventos do backend
   * Deve ser chamado no onDestroy do componente principal
   */
  unbindBackendEvents() {
    if (EventsOff) {
      this._eventUnsubscribers.forEach(unsub => {
        if (typeof unsub === 'function') unsub();
      });
    }
    this._eventUnsubscribers = [];
    this._boundToBackend = false;
    console.log('[MessageService] Backend events unbound');
  }
  
  // === Backend Event Handlers ===
  
  _handleStreamEvent(event) {
    // Sistema unificado de streaming
    // NOTA: O backend Go envia campos em PascalCase (Content, Done)
    // e o Content JÁ vem ACUMULADO, não precisamos acumular aqui
    
    const content = event.Content ?? event.content ?? '';
    const done = event.Done ?? event.done ?? false;
    const messageId = event.MessageId ?? event.messageId;
    
    // Log de streaming removido para reduzir ruído no console
    
    if (messageId && !get(streamingMessageId)) {
      // Inicia streaming com ID do backend
      streamingMessageId.set(messageId);
      isStreaming.set(true);
      this._dispatchEvent('streamingStarted', { messageId });
    }
    
    if (content && !done) {
      // IMPORTANTE: content já vem acumulado do backend, NÃO acumular de novo!
      streamingContent.set(content);
      this._updateStreamingMessage(get(streamingContent), true);
      
      this._dispatchEvent('streamingChunk', { 
        messageId: get(streamingMessageId),
        content: get(streamingContent)
      });
    }
    
    if (done) {
      // Streaming finalizado
      const fullResponse = event.FullResponse ?? event.fullResponse ?? get(streamingContent);
      
      // Atualiza mensagem final
      this._updateStreamingMessage(fullResponse, false);
      
      // Limpa estado
      isStreaming.set(false);
      const finalMessageId = get(streamingMessageId);
      streamingMessageId.set(null);
      streamingContent.set('');
      
      // Recarrega conversa para obter dados salvos do banco
      this.reload();
      
      // Emite evento para UI reagir (sons, TTS, etc.)
      this._dispatchEvent('streamingEnded', { 
        messageId: finalMessageId, 
        content: fullResponse
      });
    }
  }
  
  _handleDone(data) {
    // Chat finalizado completamente
    isStreaming.set(false);
    executingTools.set(false);
    this.reload();
    
    this._dispatchEvent('chatDone', { 
      conversationId: get(conversationId),
      ...data
    });
  }
  
  _handleError(errorMessage) {
    isStreaming.set(false);
    executingTools.set(false);
    
    // Remove mensagem de streaming se existir
    if (get(streamingMessageId)) {
      this._removeStreamingMessage();
    }
    
    streamingMessageId.set(null);
    streamingContent.set('');
    
    this._dispatchEvent('error', { message: errorMessage });
  }
  
  _handleToolsExecution(data) {
    executingTools.set(true);
    toolsMessage.set(data.message || 'Executando ferramentas...');
    
    // Atualiza mensagem de streaming para mostrar tools
    const currentMessages = get(messages);
    if (currentMessages.length > 0) {
      const lastIdx = currentMessages.length - 1;
      const lastMsg = currentMessages[lastIdx];
      if (lastMsg.role === 'assistant') {
        const newMessages = [...currentMessages];
        newMessages[lastIdx] = { ...lastMsg, toolsInfo: `🔧 ${get(toolsMessage)}` };
        messages.set(newMessages);
        this._dispatchEvent('messagesUpdated', { messages: newMessages });
      }
    }
    
    this._dispatchEvent('toolsExecution', { 
      message: get(toolsMessage),
      executing: true 
    });
  }
  
  _handleToolResults(data) {
    executingTools.set(false);
    toolsMessage.set('');
    
    this._dispatchEvent('toolResults', { 
      results: data.results,
      count: data.results?.length || 0
    });
  }
  
  _handleConversationCreated(data) {
    conversationId.set(data.conversationId);
    conversationTitle.set(data.title || 'Nova conversa');
    
    this._dispatchEvent('conversationCreated', {
      conversationId: data.conversationId,
      title: get(conversationTitle)
    });
  }
  
  async _handleMessagesReady(data) {
    // Atualiza conversa se necessário
    if (data.conversationId && data.conversationId !== get(conversationId)) {
      conversationId.set(data.conversationId);
    }
    
    // IMPORTANTE: Atualiza o ID da mensagem do usuário local com o ID real do backend
    // Isso é necessário para que mensagens internas (tool calls) possam encontrar o parent
    if (data.userMessageId) {
      // Encontra a última mensagem do usuário que ainda não tem ID (placeholder local)
      // Procura de trás para frente para pegar a mais recente
      const currentMessages = get(messages);
      let messageUpdated = false;
      const newMessages = currentMessages.map((m, i) => {
        if (!messageUpdated && m.role === 'user' && (m.id === null || m.id === undefined)) {
          // Verifica se é a última mensagem do usuário sem ID
          const isLastUserWithoutId = currentMessages.slice(i + 1).every(
            msg => msg.role !== 'user' || msg.id !== null
          );
          if (isLastUserWithoutId) {
            messageUpdated = true;
            console.log('[MessageService] Atualizado ID da mensagem do usuário:', data.userMessageId, 'no índice:', i);
            return { ...m, id: data.userMessageId, ID: data.userMessageId };
          }
        }
        return m;
      });
      if (messageUpdated) {
        messages.set(newMessages);
      }
      
      // Atualiza também nos threads
      const currentData = get(conversationData);
      if (currentData?.threads) {
        let threadUpdated = false;
        const newThreads = currentData.threads.map((t, i) => {
          if (!threadUpdated && t.message?.role === 'user' && (t.message?.id === null || t.message?.id === undefined)) {
            const isLastUserWithoutId = currentData.threads.slice(i + 1).every(
              thread => thread.message?.role !== 'user' || thread.message?.id !== null
            );
            if (isLastUserWithoutId) {
              threadUpdated = true;
              return {
                ...t,
                message: { ...t.message, id: data.userMessageId, ID: data.userMessageId }
              };
            }
          }
          return t;
        });
        if (threadUpdated) {
          conversationData.set({ ...currentData, threads: newThreads });
        }
      }
    }
    
    // Adiciona mensagem placeholder de streaming
    this._addStreamingPlaceholder();
    isStreaming.set(true);
    
    this._dispatchEvent('messagesReady', data);
  }
  
  _handleInternalMessage(data) {
    const internalMsg = {
      id: data.id,
      ID: data.id,
      role: data.role,
      content: data.content || '',
      internal: true,
      agentName: data.agentName || '',
      agent_name: data.agentName || '',
      toolCallId: data.toolCallId || '',
      toolName: data.toolName || '',
      toolCalls: data.toolCalls || null
    };
    
    // Adiciona ao array flat
    messages.set([...get(messages), internalMsg]);
    
    // Encontra o node pai nas threads e adiciona como filho
    const parentId = data.parentId;
    
    if (parentId && get(conversationData)?.threads) {
      const parentNode = this._findNodeById(get(conversationData).threads, parentId);
      if (parentNode) {
        // Cria node filho
        const childNode = {
          message: internalMsg,
          level: (parentNode.level || 0) + 1,
          children: [],
          childCount: 0
        };
        
        // Adiciona como filho
        if (!parentNode.children) parentNode.children = [];
        parentNode.children = [...parentNode.children, childNode];
        
        // Atualiza childCount
        parentNode.childCount = parentNode.children.length;
        
        // Força atualização dos threads
        conversationData.set({ ...get(conversationData) });
      }
    }
    
    this._dispatchEvent('internalMessage', { message: internalMsg, parentId });
    // Usa debounce durante streaming para evitar re-renders excessivos
    this._dispatchMessagesUpdatedDebounced({ 
      messages: get(messages),
      threads: get(conversationData)?.threads
    });
  }
  
  /**
   * Encontra um node na árvore de threads pelo ID da mensagem
   */
  _findNodeById(threads, messageId) {
    for (const node of threads) {
      if (node.message?.id === messageId || node.message?.ID === messageId) {
        return node;
      }
      if (node.children?.length > 0) {
        const found = this._findNodeById(node.children, messageId);
        if (found) return found;
      }
    }
    return null;
  }
  
  _handleAgentMessage(data) {
    const agentMsg = {
      id: data.id,
      ID: data.id,
      parentId: data.parentId,
      role: data.role,
      content: data.content || '',
      internal: true,
      agentName: data.agentName || '',
      agent_name: data.agentName || '',
      toolCallId: data.toolCallId || '',
      toolCalls: data.toolCalls || null
    };
    
    // Adiciona ao array flat
    messages.set([...get(messages), agentMsg]);
    
    // Encontra o node pai nas threads e adiciona como filho (nível 2)
    const parentId = data.parentId;
    if (parentId && get(conversationData)?.threads) {
      const parentNode = this._findNodeById(get(conversationData).threads, parentId);
      if (parentNode) {
        // Cria node filho
        const childNode = {
          message: agentMsg,
          level: (parentNode.level || 0) + 1,
          children: [],
          childCount: 0
        };
        
        // Adiciona como filho
        if (!parentNode.children) parentNode.children = [];
        parentNode.children = [...parentNode.children, childNode];
        
        // Atualiza childCount
        parentNode.childCount = parentNode.children.length;
        
        // Força atualização dos threads
        conversationData.set({ ...get(conversationData) });
      }
    }
    
    this._dispatchEvent('agentMessage', { 
      agentName: data.agentName,
      role: data.role,
      content: data.content,
      toolCalls: data.toolCalls
    });
    
    // Usa debounce durante streaming para evitar re-renders excessivos
    this._dispatchMessagesUpdatedDebounced({ 
      messages: get(messages),
      threads: get(conversationData)?.threads
    });
  }
  
  // === Streaming Helpers ===
  
  /**
   * Adiciona mensagens locais (placeholders) para feedback imediato
   * Chamado pelo Chat.svelte antes de enviar ao backend
   */
  addLocalMessages(userMessage, assistantPlaceholder) {
    
    // Adiciona ao array flat
    messages.set([...get(messages), userMessage, assistantPlaceholder]);
    
    // Adiciona aos threads
    const userNode = {
      message: userMessage,
      level: 0,
      originalIndex: get(messages).length - 2,
      children: [],
      childCount: 0
    };
    const assistantNode = {
      message: assistantPlaceholder,
      level: 0,
      originalIndex: get(messages).length - 1,
      children: [],
      childCount: 0
    };
    
    const currentData = get(conversationData) || { threads: [] };
    const newThreads = [...(currentData.threads || []), userNode, assistantNode];
    conversationData.set({ ...currentData, threads: newThreads });
    
    isStreaming.set(true);
    
    // IMPORTANTE: Dispara evento para atualizar UI imediatamente
    this._dispatchEvent('messagesUpdated', { 
      messages: get(messages),
      threads: newThreads
    });
  }
  
  _addStreamingPlaceholder() {
    // Verifica se já existe um placeholder de streaming
    const existingIdx = get(messages).findIndex(m => m.role === 'assistant' && m.isStreaming);
    if (existingIdx >= 0) {
      // Já existe, não adiciona outro
      return;
    }
    
    const placeholder = {
      id: null, // Sem ID ainda
      role: 'assistant',
      content: '',
      isStreaming: true
    };
    
    // Adiciona ao array flat
    messages.set([...get(messages), placeholder]);
    
    // Adiciona também aos threads para renderização
    const threadNode = {
      message: placeholder,
      level: 0,
      originalIndex: get(messages).length - 1,
      children: [],
      childCount: 0
    };
    
    const currentData = get(conversationData) || { threads: [] };
    const newThreads = [...(currentData.threads || []), threadNode];
    conversationData.set({ ...currentData, threads: newThreads });
    
    this._dispatchEvent('messagesUpdated', { 
      messages: get(messages),
      threads: newThreads
    });
  }
  
  _updateStreamingMessage(content, isStreaming) {
    // IMPORTANTE: Buscar ANTES de atualizar, pois procuramos por isStreaming === true
    // Busca no array flat
    const idx = get(messages).findIndex(m => m.role === 'assistant' && m.isStreaming);
    
    // Busca nos threads ANTES de atualizar o flat (para evitar dessincronização)
    let threadIdx = -1;
    if (get(conversationData)?.threads) {
      threadIdx = get(conversationData).threads.findIndex(
        t => t.message?.role === 'assistant' && t.message?.isStreaming
      );
    }
    
    // Agora atualiza o flat array
    if (idx >= 0) {
      const currentMessages = get(messages);
      const newMessages = [...currentMessages];
      newMessages[idx] = { ...newMessages[idx], content, isStreaming };
      messages.set(newMessages);
    }
    
    // Atualiza nos threads
    if (threadIdx >= 0 && get(conversationData)?.threads) {
      const currentData = get(conversationData);
      const newThreads = [...currentData.threads];
      newThreads[threadIdx] = {
        ...newThreads[threadIdx],
        message: {
          ...newThreads[threadIdx].message,
          content,
          isStreaming
        }
      };
      conversationData.set({ ...currentData, threads: newThreads });
    }
    
    this._dispatchEvent('messagesUpdated', { 
      messages: get(messages),
      threads: get(conversationData)?.threads
    });
  }
  
  _removeStreamingMessage() {
    // Remove do array flat
    const idx = get(messages).findIndex(m => m.isStreaming);
    if (idx >= 0) {
      messages.set(get(messages).filter((_, i) => i !== idx));
    }
    
    // Remove dos threads
    if (get(conversationData)?.threads) {
      const currentData = get(conversationData);
      const newThreads = currentData.threads.filter(
        t => !t.message?.isStreaming
      );
      conversationData.set({ ...currentData, threads: newThreads });
    }
    
    this._dispatchEvent('messagesUpdated', { 
      messages: get(messages),
      threads: get(conversationData)?.threads
    });
  }
  
  // === Getters (lê dos stores) ===
  
  get conversationId() { return get(conversationId); }
  get conversationTitle() { return get(conversationTitle); }
  get conversationData() { return get(conversationData); }
  get messages() { return get(messages); }
  get showInternalMessages() { return get(showInternalMessages); }
  get streamingMessageId() { return get(streamingMessageId); }
  get streamingContent() { return get(streamingContent); }
  get isStreaming() { return get(isStreaming); }
  get executingTools() { return get(executingTools); }
  get toolsMessage() { return get(toolsMessage); }
  get isBoundToBackend() { return this._boundToBackend; }
  
  get hasConversation() { return get(conversationId) !== null; }
  get isEmpty() { return get(messages).length === 0; }
  get messageCount() { return get(messages).length; }
  
  // === Setters ===
  
  setShowInternalMessages(value) {
    showInternalMessages.set(value);
    // Reorganiza threads se tiver dados
    if (get(conversationData)) {
      this._updateThreadedMessages();
    }
  }
  
  // === Operações de Conversa ===
  
  /**
   * Carrega uma conversa existente
   * @param {Object} conversation - Objeto de conversa { id, title, model, ... }
   * @param {string} defaultModel - Modelo padrão se a conversa não tiver
   * @returns {Promise<boolean>}
   */
  async loadConversation(conversation, defaultModel = '') {
    await this.ready();
    
    if (!conversation?.id) {
      this.clear();
      return false;
    }
    
    try {
      // Carrega metadados da conversa
      const convInfo = await GetConversationInfo(conversation.id);
      
      // Carrega mensagens raiz (lazy loading)
      const rootMessages = await GetMessages(conversation.id, null);
      
      conversationId.set(conversation.id);
      conversationTitle.set(convInfo.title || 'Conversa sem título');
      conversationData.set({
        id: convInfo.id,
        title: convInfo.title,
        model: convInfo.model,
        show_internal_messages: convInfo.show_internal_messages,
        threads: rootMessages
      });
      showInternalMessages.set(convInfo.show_internal_messages || false);
      
      // Extrai mensagens flat das raízes
      messages.set(this._extractMessagesFromThreads(rootMessages));
      
      this._dispatchEvent('conversationLoaded', {
        conversationId: get(conversationId),
        title: get(conversationTitle),
        messages: get(messages),
        model: convInfo.model || defaultModel
      });
      
      return true;
    } catch (error) {
      console.error('Erro ao carregar conversa:', error);
      this._dispatchEvent('error', { message: 'Erro ao carregar conversa', error });
      return false;
    }
  }
  
  /**
   * Cria uma nova conversa
   * @param {string} title - Título da conversa
   * @param {string} model - Modelo a usar
   * @returns {Promise<number|null>} ID da conversa criada
   */
  async createConversation(title, model) {
    await this.ready();
    
    if (!CreateConversation) {
      console.error('CreateConversation not available');
      return null;
    }
    
    try {
      const conv = await CreateConversation(title, model);
      conversationId.set(conv.id);
      conversationTitle.set(title);
      conversationData.set({ threads: [] });
      messages.set([]);
      
      // Salva como última conversa
      try {
        await SetLastConversation(conv.id);
      } catch (e) {
        console.error('Erro ao salvar última conversa:', e);
      }
      
      this._dispatchEvent('conversationCreated', {
        conversationId: get(conversationId),
        title: get(conversationTitle)
      });
      
      return conv.id;
    } catch (error) {
      console.error('Erro ao criar conversa:', error);
      this._dispatchEvent('error', { message: 'Erro ao criar conversa', error });
      return null;
    }
  }
  
  /**
   * Limpa a conversa atual
   */
  clear() {
    conversationId.set(null);
    conversationTitle.set('');
    conversationData.set(null);
    messages.set([]);
    streamingMessageId.set(null);
    streamingContent.set('');
    
    this._dispatchEvent('conversationCleared', {});
  }
  
  // === Operações de Mensagem ===
  
  /**
   * Salva uma nova mensagem
   * @param {string} role - 'user' | 'assistant' | 'tool' | 'system'
   * @param {string} content - Conteúdo da mensagem
   * @param {Object} options - Opções adicionais
   * @returns {Promise<boolean>}
   */
  async saveMessage(role, content, options = {}) {
    await this.ready();
    
    const {
      toolCalls = '',
      toolResults = '',
      tokenInfo = null,
      media = null,
      model = ''
    } = options;
    
    // Cria conversa se não existir
    if (!get(conversationId)) {
      const title = role === 'user' ? content.substring(0, 50) : 'Nova conversa';
      const convId = await this.createConversation(title, model);
      if (!convId) return false;
    }
    
    // Serializa mídia
    const mediaJson = media && media.length > 0 ? JSON.stringify(media) : '';
    
    try {
      if (tokenInfo && tokenInfo.totalTokens > 0) {
        await AddMessageWithTokensAndMedia(
          get(conversationId),
          role,
          content,
          mediaJson,
          toolCalls,
          toolResults,
          tokenInfo.promptTokens,
          tokenInfo.completionTokens,
          tokenInfo.totalTokens,
          tokenInfo.model
        );
      } else if (mediaJson) {
        await AddMessageWithMedia(get(conversationId), role, content, mediaJson, toolCalls, toolResults);
      } else if (tokenInfo) {
        await AddMessageWithTokens(
          get(conversationId),
          role,
          content,
          toolCalls,
          toolResults,
          tokenInfo.promptTokens || 0,
          tokenInfo.completionTokens || 0,
          tokenInfo.totalTokens || 0,
          tokenInfo.model || ''
        );
      } else {
        await AddMessage(get(conversationId), role, content, toolCalls, toolResults);
      }
      
      this._dispatchEvent('messageSaved', { role, content });
      return true;
    } catch (error) {
      console.error('Erro ao salvar mensagem:', error);
      this._dispatchEvent('error', { message: 'Erro ao salvar mensagem', error });
      return false;
    }
  }
  
  /**
   * Atualiza o conteúdo de uma mensagem (usado durante streaming)
   * @param {number} messageId - ID da mensagem
   * @param {string} content - Novo conteúdo
   */
  updateMessageContent(messageId, content) {
    // Atualiza nos threads
    if (get(conversationData)?.threads) {
      this._updateInThreads(get(conversationData).threads, messageId, content);
      conversationData.set({ ...get(conversationData) });
    }
    
    // Atualiza no array flat
    const currentMessages = get(messages);
    const idx = currentMessages.findIndex(m => m.id === messageId);
    if (idx >= 0) {
      const newMessages = [...currentMessages];
      newMessages[idx] = { ...newMessages[idx], content };
      messages.set(newMessages);
    }
    
    this._dispatchEvent('messageUpdated', { messageId, content });
  }
  
  _updateInThreads(nodes, messageId, content) {
    for (const node of nodes) {
      if (node.message.id === messageId) {
        node.message.content = content;
        return true;
      }
      if (node.children?.length > 0 && this._updateInThreads(node.children, messageId, content)) {
        return true;
      }
    }
    return false;
  }
  
  /**
   * Carrega filhos de uma mensagem (lazy loading)
   * @param {number} messageId - ID da mensagem pai
   * @returns {Promise<Array>} MessageNodes dos filhos
   */
  async loadChildren(messageId) {
    await this.ready();
    
    if (!GetMessages) return [];
    
    try {
      const children = await GetMessages(0, messageId);
      this._dispatchEvent('childrenLoaded', { messageId, children });
      return children;
    } catch (error) {
      console.error('Erro ao carregar filhos:', error);
      return [];
    }
  }
  
  /**
   * Recarrega mensagens raiz do banco de dados, preservando filhos já carregados
   */
  async reload() {
    if (!get(conversationId)) return false;
    
    try {
      // Recarrega metadados
      const convInfo = await GetConversationInfo(get(conversationId));
      
      // Recarrega mensagens raiz
      const rootMessages = await GetMessages(get(conversationId), null);
      
      // Preserva os children já carregados dos threads existentes
      const oldThreads = get(conversationData)?.threads || [];
      const mergedThreads = this._mergeThreadsPreservingChildren(rootMessages, oldThreads);
      
      conversationData.set({
        ...get(conversationData),
        threads: mergedThreads
      });
      messages.set(this._extractMessagesFromThreads(mergedThreads));
      
      // Dispara messagesUpdated para atualizar a UI (além de messagesReloaded)
      this._dispatchEvent('messagesUpdated', {
        messages: get(messages),
        threads: mergedThreads
      });
      
      this._dispatchEvent('messagesReloaded', {
        messages: get(messages)
      });
      
      return true;
    } catch (error) {
      console.error('Erro ao recarregar mensagens:', error);
      return false;
    }
  }
  
  /**
   * Mescla threads novos com antigos, preservando children já carregados
   */
  _mergeThreadsPreservingChildren(newThreads, oldThreads) {
    if (!oldThreads || oldThreads.length === 0) return newThreads;
    
    // Cria mapa de threads antigos por ID
    const oldThreadsMap = new Map();
    const buildMap = (threads) => {
      for (const t of threads) {
        const id = t.message?.id || t.message?.ID || t.ID || t.id;
        if (id) {
          oldThreadsMap.set(id, t);
        }
        if (t.children?.length > 0) {
          buildMap(t.children);
        }
      }
    };
    buildMap(oldThreads);
    
    // Mescla preservando children
    const merge = (threads) => {
      return threads.map(newThread => {
        const id = newThread.message?.id || newThread.message?.ID || newThread.ID || newThread.id;
        const oldThread = id ? oldThreadsMap.get(id) : null;
        
        // Se o thread antigo tinha children carregados, preserva
        if (oldThread?.children?.length > 0) {
          return {
            ...newThread,
            children: oldThread.children,
            childCount: Math.max(newThread.childCount || 0, oldThread.children.length)
          };
        }
        
        return newThread;
      });
    };
    
    return merge(newThreads);
  }
  
  // === Streaming ===
  
  /**
   * Inicia streaming de uma mensagem
   * @param {number} messageId - ID da mensagem sendo streamada
   */
  startStreaming(messageId) {
    streamingMessageId.set(messageId);
    streamingContent.set('');
    this._dispatchEvent('streamingStarted', { messageId });
  }
  
  // === Transformações ===
  
  /**
   * Extrai mensagens flat dos threads
   */
  _extractMessagesFromThreads(threads) {
    const result = [];
    
    const traverse = (nodes) => {
      for (const node of nodes) {
        const m = node.message;
        
        // Parseia tool_calls
        let toolCalls = null;
        if (m.tool_calls) {
          try {
            toolCalls = typeof m.tool_calls === 'string' ? JSON.parse(m.tool_calls) : m.tool_calls;
          } catch (e) { /* ignore */ }
        }
        
        // Extrai nome da tool
        let toolName = '';
        if (toolCalls && toolCalls.length > 0) {
          toolName = toolCalls[0].function?.name || toolCalls[0].Function?.Name || '';
        }
        
        // Reconstrói mídia
        let media = undefined;
        if (m.media) {
          try {
            const mediaArray = JSON.parse(m.media);
            if (Array.isArray(mediaArray) && mediaArray.length > 0) {
              media = mediaArray.map(item => ({
                type: item.type,
                preview: item.data,
                altText: item.altText || '',
                file: { name: item.filename || 'arquivo' }
              }));
            }
          } catch (e) { /* ignore */ }
        }
        
        result.push({
          id: m.id,
          parentId: m.parent_id || null,
          role: m.role,
          content: m.content,
          toolCalls: toolCalls,
          toolCallId: m.tool_call_id,
          agentName: m.agent_name,
          toolName: toolName,
          internal: m.parent_id != null,
          media: media,
          level: node.level
        });
        
        if (node.children?.length > 0) {
          traverse(node.children);
        }
      }
    };
    
    traverse(threads || []);
    return result;
  }
  
  
  /**
   * Atualiza mensagens threaded (força recálculo)
   */
  _updateThreadedMessages() {
    if (get(conversationData)?.threads) {
      messages.set(this._extractMessagesFromThreads(get(conversationData).threads));
      this._dispatchEvent('messagesUpdated', { messages: get(messages) });
    }
  }
  
  // === Configurações ===
  
  /**
   * Atualiza configurações da conversa
   */
  async updateSettings(showInternalMessages) {
    if (!get(conversationId) || !UpdateConversationSettings) return false;
    
    try {
      await UpdateConversationSettings(get(conversationId), showInternalMessages);
      showInternalMessages.set(showInternalMessages);
      this._updateThreadedMessages();
      return true;
    } catch (error) {
      console.error('Erro ao atualizar configurações:', error);
      return false;
    }
  }
  
  /**
   * Atualiza modelo da conversa
   */
  async updateModel(model) {
    if (!get(conversationId) || !UpdateConversationModel) return false;
    
    try {
      await UpdateConversationModel(get(conversationId), model);
      return true;
    } catch (error) {
      console.error('Erro ao atualizar modelo:', error);
      return false;
    }
  }
  
  // === Utilidades ===
  
  /**
   * Encontra uma mensagem por ID
   */
  findMessage(messageId) {
    return get(messages).find(m => m.id === messageId);
  }
  
  /**
   * Encontra índice de uma mensagem
   */
  findMessageIndex(messageId) {
    return get(messages).findIndex(m => m.id === messageId);
  }
  
  /**
   * Retorna a última mensagem
   */
  getLastMessage() {
    return get(messages)[get(messages).length - 1];
  }
  
  /**
   * Retorna mensagens de um role específico
   */
  getMessagesByRole(role) {
    return get(messages).filter(m => m.role === role);
  }
  
  // === Eventos ===
  
  _dispatchEvent(type, detail) {
    this.dispatchEvent(new CustomEvent(type, { detail }));
  }
}

// ========================================
// Funções utilitárias de mensagens
// ========================================

/**
 * Parseia tool_calls de uma mensagem
 * Aceita string JSON ou objeto
 */
export function parseToolCalls(toolCalls) {
  if (!toolCalls) return null;
  if (typeof toolCalls === 'string') {
    try {
      return JSON.parse(toolCalls);
    } catch (e) {
      return null;
    }
  }
  return toolCalls;
}

/**
 * Formata nome de agente para exibição
 * Converte snake_case para Title Case
 */
export function formatAgentName(name) {
  if (!name) return 'Agente';
  return name.split('_').map(word => 
    word.charAt(0).toUpperCase() + word.slice(1)
  ).join(' ');
}

/**
 * Converte MessageNode do backend para formato do frontend
 * Normaliza campos PascalCase → camelCase
 */
export function convertMessageNode(node, index = 0) {
  const m = node.message || node.Message || {};
  
  // Parseia tool_calls se for string
  let toolCalls = null;
  const rawToolCalls = m.tool_calls || m.ToolCalls;
  if (rawToolCalls) {
    try {
      toolCalls = typeof rawToolCalls === 'string' ? JSON.parse(rawToolCalls) : rawToolCalls;
    } catch (e) { /* ignore */ }
  }
  
  // Extrai toolName
  let toolName = '';
  if (toolCalls && toolCalls.length > 0) {
    toolName = toolCalls[0].function?.name || toolCalls[0].Function?.Name || '';
  }
  
  return {
    message: {
      id: m.id || m.ID,
      parentId: m.parent_id || m.ParentID,
      role: m.role || m.Role,
      content: m.content || m.Content || '',
      toolCalls: toolCalls,
      toolCallId: m.tool_call_id || m.ToolCallID,
      agentName: m.agent_name || m.AgentName,
      toolName: toolName,
      internal: (m.parent_id || m.ParentID) != null,
      model: m.model || m.Model,
      promptTokens: m.prompt_tokens || m.PromptTokens,
      completionTokens: m.completion_tokens || m.CompletionTokens,
      totalTokens: m.total_tokens || m.TotalTokens,
      isStreaming: m.isStreaming || false,
      toolsInfo: m.toolsInfo || null,
    },
    agentName: m.agent_name || m.AgentName,
    toolName: toolName,
    level: node.level ?? node.Level ?? 0,
    originalIndex: node.originalIndex ?? index,
    children: node.children || [],
    childCount: node.child_count ?? node.ChildCount ?? node.childCount ?? 0
  };
}

// Exporta instância singleton
export const messageService = new MessageService();

// Exporta classe para instâncias customizadas
export { MessageService };

