/**
 * MessageService - Serviço de gerenciamento de mensagens de chat
 * 
 * Responsabilidades:
 * - CRUD de mensagens (save, load, update)
 * - Transformação de dados (threads, flat)
 * - Gerenciamento de streaming
 * - Eventos de atualização
 * 
 * Uso:
 *   import { messageService } from '$lib/chat/message-service.js';
 *   
 *   messageService.addEventListener('messagesUpdated', (e) => {
 *     console.log('Mensagens:', e.detail.messages);
 *   });
 *   
 *   await messageService.loadConversation(conversationId);
 *   await messageService.saveMessage('user', 'Olá!');
 */

// Imports dinâmicos do Wails
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
    
    // Estado
    this._conversationId = null;
    this._conversationTitle = '';
    this._conversationData = null; // ConversationWithThreads do backend
    this._messages = [];           // Array flat para compatibilidade
    this._showInternalMessages = false;
    
    // Streaming
    this._streamingMessageId = null;
    this._streamingContent = '';
    this._isStreaming = false;
    this._executingTools = false;
    this._toolsMessage = '';
    
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
    if (this._isStreaming) {
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
    
    if (messageId && !this._streamingMessageId) {
      // Inicia streaming com ID do backend
      this._streamingMessageId = messageId;
      this._isStreaming = true;
      this._dispatchEvent('streamingStarted', { messageId });
    }
    
    if (content && !done) {
      // IMPORTANTE: content já vem acumulado do backend, NÃO acumular de novo!
      this._streamingContent = content;
      this._updateStreamingMessage(this._streamingContent, true);
      
      this._dispatchEvent('streamingChunk', { 
        messageId: this._streamingMessageId,
        content: this._streamingContent
      });
    }
    
    if (done) {
      // Streaming finalizado
      const fullResponse = event.FullResponse ?? event.fullResponse ?? this._streamingContent;
      
      // Atualiza mensagem final
      this._updateStreamingMessage(fullResponse, false);
      
      // Limpa estado
      this._isStreaming = false;
      const finalMessageId = this._streamingMessageId;
      this._streamingMessageId = null;
      this._streamingContent = '';
      
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
    this._isStreaming = false;
    this._executingTools = false;
    this.reload();
    
    this._dispatchEvent('chatDone', { 
      conversationId: this._conversationId,
      ...data
    });
  }
  
  _handleError(errorMessage) {
    this._isStreaming = false;
    this._executingTools = false;
    
    // Remove mensagem de streaming se existir
    if (this._streamingMessageId) {
      this._removeStreamingMessage();
    }
    
    this._streamingMessageId = null;
    this._streamingContent = '';
    
    this._dispatchEvent('error', { message: errorMessage });
  }
  
  _handleToolsExecution(data) {
    this._executingTools = true;
    this._toolsMessage = data.message || 'Executando ferramentas...';
    
    // Atualiza mensagem de streaming para mostrar tools
    if (this._messages.length > 0) {
      const lastMsg = this._messages[this._messages.length - 1];
      if (lastMsg.role === 'assistant') {
        lastMsg.toolsInfo = `🔧 ${this._toolsMessage}`;
        this._messages = [...this._messages];
        this._dispatchEvent('messagesUpdated', { messages: this._messages });
      }
    }
    
    this._dispatchEvent('toolsExecution', { 
      message: this._toolsMessage,
      executing: true 
    });
  }
  
  _handleToolResults(data) {
    this._executingTools = false;
    this._toolsMessage = '';
    
    this._dispatchEvent('toolResults', { 
      results: data.results,
      count: data.results?.length || 0
    });
  }
  
  _handleConversationCreated(data) {
    this._conversationId = data.conversationId;
    this._conversationTitle = data.title || 'Nova conversa';
    
    this._dispatchEvent('conversationCreated', {
      conversationId: data.conversationId,
      title: this._conversationTitle
    });
  }
  
  async _handleMessagesReady(data) {
    // Atualiza conversa se necessário
    if (data.conversationId && data.conversationId !== this._conversationId) {
      this._conversationId = data.conversationId;
    }
    
    // IMPORTANTE: Atualiza o ID da mensagem do usuário local com o ID real do backend
    // Isso é necessário para que mensagens internas (tool calls) possam encontrar o parent
    if (data.userMessageId) {
      // Encontra a última mensagem do usuário que ainda não tem ID (placeholder local)
      // Procura de trás para frente para pegar a mais recente
      for (let i = this._messages.length - 1; i >= 0; i--) {
        const m = this._messages[i];
        if (m.role === 'user' && (m.id === null || m.id === undefined)) {
          this._messages[i].id = data.userMessageId;
          this._messages[i].ID = data.userMessageId;
          break;
        }
      }
      
      // Atualiza também nos threads
      if (this._conversationData?.threads) {
        // Procura de trás para frente
        for (let i = this._conversationData.threads.length - 1; i >= 0; i--) {
          const t = this._conversationData.threads[i];
          if (t.message?.role === 'user' && (t.message?.id === null || t.message?.id === undefined)) {
            t.message.id = data.userMessageId;
            t.message.ID = data.userMessageId;
            break;
          }
        }
      }
    }
    
    // Adiciona mensagem placeholder de streaming
    this._addStreamingPlaceholder();
    this._isStreaming = true;
    
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
    this._messages = [...this._messages, internalMsg];
    
    // Encontra o node pai nas threads e adiciona como filho
    const parentId = data.parentId;
    
    if (parentId && this._conversationData?.threads) {
      const parentNode = this._findNodeById(this._conversationData.threads, parentId);
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
        this._conversationData = { ...this._conversationData };
      }
    }
    
    this._dispatchEvent('internalMessage', { message: internalMsg, parentId });
    // Usa debounce durante streaming para evitar re-renders excessivos
    this._dispatchMessagesUpdatedDebounced({ 
      messages: this._messages,
      threads: this._conversationData?.threads
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
    this._messages = [...this._messages, agentMsg];
    
    // Encontra o node pai nas threads e adiciona como filho (nível 2)
    const parentId = data.parentId;
    if (parentId && this._conversationData?.threads) {
      const parentNode = this._findNodeById(this._conversationData.threads, parentId);
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
        this._conversationData = { ...this._conversationData };
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
      messages: this._messages,
      threads: this._conversationData?.threads
    });
  }
  
  // === Streaming Helpers ===
  
  /**
   * Adiciona mensagens locais (placeholders) para feedback imediato
   * Chamado pelo Chat.svelte antes de enviar ao backend
   */
  addLocalMessages(userMessage, assistantPlaceholder) {
    
    // Adiciona ao array flat
    this._messages = [...this._messages, userMessage, assistantPlaceholder];
    
    // Adiciona aos threads
    const userNode = {
      message: userMessage,
      level: 0,
      originalIndex: this._messages.length - 2,
      children: [],
      childCount: 0
    };
    const assistantNode = {
      message: assistantPlaceholder,
      level: 0,
      originalIndex: this._messages.length - 1,
      children: [],
      childCount: 0
    };
    
    if (!this._conversationData) {
      this._conversationData = { threads: [] };
    }
    this._conversationData.threads = [...(this._conversationData.threads || []), userNode, assistantNode];
    
    this._isStreaming = true;
    
    // IMPORTANTE: Dispara evento para atualizar UI imediatamente
    this._dispatchEvent('messagesUpdated', { 
      messages: this._messages,
      threads: this._conversationData.threads
    });
  }
  
  _addStreamingPlaceholder() {
    // Verifica se já existe um placeholder de streaming
    const existingIdx = this._messages.findIndex(m => m.role === 'assistant' && m.isStreaming);
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
    this._messages = [...this._messages, placeholder];
    
    // Adiciona também aos threads para renderização
    const threadNode = {
      message: placeholder,
      level: 0,
      originalIndex: this._messages.length - 1,
      children: [],
      childCount: 0
    };
    
    if (!this._conversationData) {
      this._conversationData = { threads: [] };
    }
    this._conversationData.threads = [...(this._conversationData.threads || []), threadNode];
    
    this._dispatchEvent('messagesUpdated', { 
      messages: this._messages,
      threads: this._conversationData.threads
    });
  }
  
  _updateStreamingMessage(content, isStreaming) {
    // IMPORTANTE: Buscar ANTES de atualizar, pois procuramos por isStreaming === true
    // Busca no array flat
    const idx = this._messages.findIndex(m => m.role === 'assistant' && m.isStreaming);
    
    // Busca nos threads ANTES de atualizar o flat (para evitar dessincronização)
    let threadIdx = -1;
    if (this._conversationData?.threads) {
      threadIdx = this._conversationData.threads.findIndex(
        t => t.message?.role === 'assistant' && t.message?.isStreaming
      );
    }
    
    // Agora atualiza o flat array
    if (idx >= 0) {
      this._messages[idx].content = content;
      this._messages[idx].isStreaming = isStreaming;
      this._messages = [...this._messages];
    }
    
    // Atualiza nos threads
    if (threadIdx >= 0) {
      this._conversationData.threads[threadIdx].message.content = content;
      this._conversationData.threads[threadIdx].message.isStreaming = isStreaming;
      this._conversationData.threads = [...this._conversationData.threads];
    }
    
    this._dispatchEvent('messagesUpdated', { 
      messages: this._messages,
      threads: this._conversationData?.threads
    });
  }
  
  _removeStreamingMessage() {
    // Remove do array flat
    const idx = this._messages.findIndex(m => m.isStreaming);
    if (idx >= 0) {
      this._messages = this._messages.filter((_, i) => i !== idx);
    }
    
    // Remove dos threads
    if (this._conversationData?.threads) {
      this._conversationData.threads = this._conversationData.threads.filter(
        t => !t.message?.isStreaming
      );
    }
    
    this._dispatchEvent('messagesUpdated', { 
      messages: this._messages,
      threads: this._conversationData?.threads
    });
  }
  
  // === Getters ===
  
  get conversationId() { return this._conversationId; }
  get conversationTitle() { return this._conversationTitle; }
  get conversationData() { return this._conversationData; }
  get messages() { return this._messages; }
  get showInternalMessages() { return this._showInternalMessages; }
  get streamingMessageId() { return this._streamingMessageId; }
  get streamingContent() { return this._streamingContent; }
  get isStreaming() { return this._isStreaming; }
  get executingTools() { return this._executingTools; }
  get toolsMessage() { return this._toolsMessage; }
  get isBoundToBackend() { return this._boundToBackend; }
  
  get hasConversation() { return this._conversationId !== null; }
  get isEmpty() { return this._messages.length === 0; }
  get messageCount() { return this._messages.length; }
  
  // === Setters ===
  
  setShowInternalMessages(value) {
    this._showInternalMessages = value;
    // Reorganiza threads se tiver dados
    if (this._conversationData) {
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
      
      this._conversationId = conversation.id;
      this._conversationTitle = convInfo.title || 'Conversa sem título';
      this._conversationData = {
        id: convInfo.id,
        title: convInfo.title,
        model: convInfo.model,
        show_internal_messages: convInfo.show_internal_messages,
        threads: rootMessages
      };
      this._showInternalMessages = convInfo.show_internal_messages || false;
      
      // Extrai mensagens flat das raízes
      this._messages = this._extractMessagesFromThreads(rootMessages);
      
      this._dispatchEvent('conversationLoaded', {
        conversationId: this._conversationId,
        title: this._conversationTitle,
        messages: this._messages,
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
      this._conversationId = conv.id;
      this._conversationTitle = title;
      this._conversationData = { threads: [] };
      this._messages = [];
      
      // Salva como última conversa
      try {
        await SetLastConversation(conv.id);
      } catch (e) {
        console.error('Erro ao salvar última conversa:', e);
      }
      
      this._dispatchEvent('conversationCreated', {
        conversationId: this._conversationId,
        title: this._conversationTitle
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
    this._conversationId = null;
    this._conversationTitle = '';
    this._conversationData = null;
    this._messages = [];
    this._streamingMessageId = null;
    this._streamingContent = '';
    
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
    if (!this._conversationId) {
      const title = role === 'user' ? content.substring(0, 50) : 'Nova conversa';
      const convId = await this.createConversation(title, model);
      if (!convId) return false;
    }
    
    // Serializa mídia
    const mediaJson = media && media.length > 0 ? JSON.stringify(media) : '';
    
    try {
      if (tokenInfo && tokenInfo.totalTokens > 0) {
        await AddMessageWithTokensAndMedia(
          this._conversationId,
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
        await AddMessageWithMedia(this._conversationId, role, content, mediaJson, toolCalls, toolResults);
      } else if (tokenInfo) {
        await AddMessageWithTokens(
          this._conversationId,
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
        await AddMessage(this._conversationId, role, content, toolCalls, toolResults);
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
    if (this._conversationData?.threads) {
      this._updateInThreads(this._conversationData.threads, messageId, content);
      this._conversationData = { ...this._conversationData };
    }
    
    // Atualiza no array flat
    const idx = this._messages.findIndex(m => m.id === messageId);
    if (idx >= 0) {
      this._messages[idx].content = content;
      this._messages = [...this._messages];
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
    if (!this._conversationId) return false;
    
    try {
      // Recarrega metadados
      const convInfo = await GetConversationInfo(this._conversationId);
      
      // Recarrega mensagens raiz
      const rootMessages = await GetMessages(this._conversationId, null);
      
      // Preserva os children já carregados dos threads existentes
      const oldThreads = this._conversationData?.threads || [];
      const mergedThreads = this._mergeThreadsPreservingChildren(rootMessages, oldThreads);
      
      this._conversationData = {
        ...this._conversationData,
        threads: mergedThreads
      };
      this._messages = this._extractMessagesFromThreads(mergedThreads);
      
      // Dispara messagesUpdated para atualizar a UI (além de messagesReloaded)
      this._dispatchEvent('messagesUpdated', {
        messages: this._messages,
        threads: mergedThreads
      });
      
      this._dispatchEvent('messagesReloaded', {
        messages: this._messages
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
    this._streamingMessageId = messageId;
    this._streamingContent = '';
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
    if (this._conversationData?.threads) {
      this._messages = this._extractMessagesFromThreads(this._conversationData.threads);
      this._dispatchEvent('messagesUpdated', { messages: this._messages });
    }
  }
  
  // === Configurações ===
  
  /**
   * Atualiza configurações da conversa
   */
  async updateSettings(showInternalMessages) {
    if (!this._conversationId || !UpdateConversationSettings) return false;
    
    try {
      await UpdateConversationSettings(this._conversationId, showInternalMessages);
      this._showInternalMessages = showInternalMessages;
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
    if (!this._conversationId || !UpdateConversationModel) return false;
    
    try {
      await UpdateConversationModel(this._conversationId, model);
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
    return this._messages.find(m => m.id === messageId);
  }
  
  /**
   * Encontra índice de uma mensagem
   */
  findMessageIndex(messageId) {
    return this._messages.findIndex(m => m.id === messageId);
  }
  
  /**
   * Retorna a última mensagem
   */
  getLastMessage() {
    return this._messages[this._messages.length - 1];
  }
  
  /**
   * Retorna mensagens de um role específico
   */
  getMessagesByRole(role) {
    return this._messages.filter(m => m.role === role);
  }
  
  // === Eventos ===
  
  _dispatchEvent(type, detail) {
    this.dispatchEvent(new CustomEvent(type, { detail }));
  }
}

// Exporta instância singleton
export const messageService = new MessageService();

// Exporta classe para instâncias customizadas
export { MessageService };

