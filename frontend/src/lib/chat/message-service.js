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
    
    // Inicialização
    this._initialized = false;
    this._initPromise = this._init();
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
   */
  bindBackendEvents() {
    if (this._boundToBackend || !EventsOn) {
      console.warn('[MessageService] Backend events already bound or EventsOn not available');
      return;
    }
    
    // Streaming de chunks
    this._eventUnsubscribers.push(
      EventsOn('chat:chunk', (chunk) => this._handleChunk(chunk))
    );
    
    // Novo sistema de streaming unificado
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
    console.log('[MessageService] Backend events bound');
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
  
  _handleChunk(chunk) {
    if (chunk.done) {
      // Streaming finalizado
      const fullResponse = chunk.fullResponse || this._streamingContent;
      
      // Atualiza mensagem de streaming
      this._updateStreamingMessage(fullResponse, false);
      
      // Finaliza streaming
      this._isStreaming = false;
      const messageId = this._streamingMessageId;
      this._streamingMessageId = null;
      this._streamingContent = '';
      
      // Recarrega conversa para obter dados salvos
      this.reload();
      
      // Emite evento para UI reagir (sons, TTS, etc.)
      this._dispatchEvent('streamingEnded', { 
        messageId, 
        content: fullResponse,
        toolCalls: chunk.toolCalls || null
      });
    } else {
      // Recebendo chunk
      this._streamingContent += chunk.content;
      this._updateStreamingMessage(this._streamingContent, true);
      
      this._dispatchEvent('streamingChunk', { 
        messageId: this._streamingMessageId,
        chunk: chunk.content,
        content: this._streamingContent
      });
    }
  }
  
  _handleStreamEvent(event) {
    // Sistema unificado de streaming
    const { messageId, content, done } = event;
    
    if (messageId && !this._streamingMessageId) {
      // Inicia streaming com ID do backend
      this._streamingMessageId = messageId;
      this._isStreaming = true;
      this._dispatchEvent('streamingStarted', { messageId });
    }
    
    if (content) {
      this._streamingContent += content;
      this._updateStreamingMessage(this._streamingContent, true);
      
      this._dispatchEvent('streamingChunk', { 
        messageId: this._streamingMessageId,
        chunk: content,
        content: this._streamingContent
      });
    }
    
    if (done) {
      this._handleChunk({ done: true, fullResponse: event.fullResponse });
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
    console.log('[MessageService] Conversa criada:', data);
    this._conversationId = data.conversationId;
    this._conversationTitle = data.title || 'Nova conversa';
    
    this._dispatchEvent('conversationCreated', {
      conversationId: data.conversationId,
      title: this._conversationTitle
    });
  }
  
  async _handleMessagesReady(data) {
    console.log('[MessageService] Mensagens prontas:', data);
    
    // Atualiza conversa se necessário
    if (data.conversationId && data.conversationId !== this._conversationId) {
      this._conversationId = data.conversationId;
    }
    
    // Adiciona mensagem placeholder de streaming
    this._addStreamingPlaceholder();
    this._isStreaming = true;
    
    this._dispatchEvent('messagesReady', data);
  }
  
  _handleInternalMessage(data) {
    console.log('[MessageService] Mensagem interna:', data);
    
    const internalMsg = {
      id: data.id,
      role: data.role,
      content: data.content || '',
      internal: true,
      agentName: data.agentName || '',
      toolCallId: data.toolCallId || '',
      toolName: data.toolName || '',
      toolCalls: data.toolCalls || null
    };
    
    this._messages = [...this._messages, internalMsg];
    
    this._dispatchEvent('internalMessage', { message: internalMsg });
    this._dispatchEvent('messagesUpdated', { messages: this._messages });
  }
  
  _handleAgentMessage(data) {
    console.log('[MessageService] Mensagem de agente:', data);
    
    // Armazena se mensagens internas estiverem habilitadas
    if (this._showInternalMessages) {
      const agentMsg = {
        id: data.id,
        parentId: data.parentId,
        role: data.role,
        content: data.content || '',
        internal: true,
        agentName: data.agentName || '',
        toolCallId: data.toolCallId || '',
        toolCalls: data.toolCalls || null
      };
      this._messages = [...this._messages, agentMsg];
      this._dispatchEvent('messagesUpdated', { messages: this._messages });
    }
    
    this._dispatchEvent('agentMessage', { 
      agentName: data.agentName,
      role: data.role,
      content: data.content,
      toolCalls: data.toolCalls
    });
  }
  
  // === Streaming Helpers ===
  
  _addStreamingPlaceholder() {
    const placeholder = {
      id: null, // Sem ID ainda
      role: 'assistant',
      content: '',
      isStreaming: true
    };
    this._messages = [...this._messages, placeholder];
    this._dispatchEvent('messagesUpdated', { messages: this._messages });
  }
  
  _updateStreamingMessage(content, isStreaming) {
    const idx = this._messages.findIndex(m => m.role === 'assistant' && m.isStreaming);
    if (idx >= 0) {
      this._messages[idx].content = content;
      this._messages[idx].isStreaming = isStreaming;
      this._messages = [...this._messages];
      this._dispatchEvent('messagesUpdated', { messages: this._messages });
    }
  }
  
  _removeStreamingMessage() {
    const idx = this._messages.findIndex(m => m.isStreaming);
    if (idx >= 0) {
      this._messages = this._messages.filter((_, i) => i !== idx);
      this._dispatchEvent('messagesUpdated', { messages: this._messages });
    }
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
   * Recarrega mensagens raiz do banco de dados
   */
  async reload() {
    if (!this._conversationId) return false;
    
    try {
      // Recarrega metadados
      const convInfo = await GetConversationInfo(this._conversationId);
      
      // Recarrega mensagens raiz
      const rootMessages = await GetMessages(this._conversationId, null);
      
      this._conversationData = {
        ...this._conversationData,
        threads: rootMessages
      };
      this._messages = this._extractMessagesFromThreads(rootMessages);
      
      this._dispatchEvent('messagesReloaded', {
        messages: this._messages
      });
      
      return true;
    } catch (error) {
      console.error('Erro ao recarregar mensagens:', error);
      return false;
    }
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
  
  /**
   * Adiciona conteúdo ao streaming
   * @param {string} chunk - Chunk de conteúdo
   */
  appendStreamingContent(chunk) {
    this._streamingContent += chunk;
    
    if (this._streamingMessageId) {
      this.updateMessageContent(this._streamingMessageId, this._streamingContent);
    }
    
    this._dispatchEvent('streamingChunk', { 
      messageId: this._streamingMessageId,
      chunk,
      content: this._streamingContent
    });
  }
  
  /**
   * Finaliza streaming
   */
  endStreaming() {
    const messageId = this._streamingMessageId;
    const content = this._streamingContent;
    
    this._streamingMessageId = null;
    this._streamingContent = '';
    
    this._dispatchEvent('streamingEnded', { messageId, content });
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

