/**
 * MessageController - Controller stateless para operações de chat
 * 
 * NÃO gerencia state interno - apenas executa operações e atualiza stores
 * passadas no construtor. Isso garante compatibilidade com reatividade Svelte.
 */

import { get } from 'svelte/store';
import { normalizeThreads, extractMessagesFromThreads } from './utils.js';

// Imports dinâmicos do Wails (compartilhados entre instâncias)
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

let _wailsLoaded = false;
let _wailsLoadPromise = null;

async function loadWailsFunctions() {
  if (_wailsLoaded) return;
  if (_wailsLoadPromise) return _wailsLoadPromise;
  
  _wailsLoadPromise = (async () => {
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
      console.warn('[MessageController] Funções Wails não disponíveis:', e);
    }
    
    try {
      const runtime = await import('../../../wailsjs/runtime/runtime.js');
      EventsOn = runtime.EventsOn;
      EventsOff = runtime.EventsOff;
    } catch (e) {
      console.warn('[MessageController] Runtime Wails não disponível:', e);
    }
    
    _wailsLoaded = true;
  })();
  
  return _wailsLoadPromise;
}

/**
 * Controller para operações de chat
 * Stateless - apenas atualiza stores passadas no construtor
 */
export class MessageController {
  /**
   * @param {Object} stores - Stores Svelte criadas por createChatStores()
   * @param {string} [instanceId] - ID opcional da instância
   */
  constructor(stores, instanceId = null) {
    this.stores = stores;
    this._instanceId = instanceId || crypto.randomUUID();
    this._backendUnsubscribers = [];
    this._initialized = false;
    this._boundToBackend = false;
    
    console.log(`[MessageController ${this._instanceId}] Instância criada`);
  }
  
  /**
   * ID único desta instância
   */
  get instanceId() {
    return this._instanceId;
  }
  
  /**
   * Inicializa controller (carrega Wails functions)
   */
  async init() {
    if (this._initialized) {
      console.log(`[MessageController ${this._instanceId}] Já inicializado`);
      return;
    }
    
    console.log(`[MessageController ${this._instanceId}] Inicializando...`);
    await loadWailsFunctions();
    this._initialized = true;
    console.log(`[MessageController ${this._instanceId}] Inicializado`);
  }
  
  /**
   * Conecta eventos do backend (Wails)
   * Deve ser chamado após init()
   */
  bindBackendEvents() {
    if (this._boundToBackend) {
      console.warn(`[MessageController ${this._instanceId}] Já está bound ao backend`);
      return;
    }
    
    if (!EventsOn) {
      console.warn(`[MessageController ${this._instanceId}] EventsOn não disponível`);
      return;
    }
    
    console.log(`[MessageController ${this._instanceId}] Conectando eventos backend...`);
    
    // Evento: chat:stream (streaming de mensagens)
    const handleStream = (event) => {
      const currentConvId = get(this.stores.conversationId);
      
      // Só processa se for desta conversa OU se ainda não temos conversationId (nova conversa)
      if (currentConvId !== null && event.conversationId !== currentConvId) return;
      
      console.log(`[MessageController ${this._instanceId}] Stream recebido:`, {
        conversationId: event.conversationId,
        content: event.content?.substring(0, 50) + '...',
        done: event.done
      });
      
      // Atualiza isStreaming
      this.stores.isStreaming.set(!event.done);
      
      // Atualiza mensagens - IMUTABILIDADE COMPLETA
      this.stores.messages.update(msgs => {
        const lastMsg = msgs[msgs.length - 1];
        
        if (lastMsg && (lastMsg.isStreaming || lastMsg.role === 'assistant')) {
          // Atualiza última mensagem - CRIA NOVO OBJETO
          return msgs.map((msg, i) => 
            i === msgs.length - 1
              ? { ...msg, content: event.content || '', isStreaming: !event.done }
              : msg
          );
        } else {
          // Cria nova mensagem
          return [...msgs, {
            content: event.content || '',
            role: 'assistant',
            isStreaming: !event.done,
            id: null
          }];
        }
      });
    };
    
    EventsOn('chat:stream', handleStream);
    this._backendUnsubscribers.push(() => {
      if (EventsOff) EventsOff('chat:stream', handleStream);
    });
    
    // Evento: chat:stream_end (fim do streaming)
    const handleStreamEnd = (event) => {
      const currentConvId = get(this.stores.conversationId);
      // Só processa se for desta conversa OU se ainda não temos conversationId (nova conversa)
      if (currentConvId !== null && event.conversationId !== currentConvId) return;
      
      console.log(`[MessageController ${this._instanceId}] Stream finalizado`);
      this.stores.isStreaming.set(false);
    };
    
    EventsOn('chat:stream_end', handleStreamEnd);
    this._backendUnsubscribers.push(() => {
      if (EventsOff) EventsOff('chat:stream_end', handleStreamEnd);
    });
    
    // Evento: chat:conversation_created (nova conversa criada)
    const handleConversationCreated = async (event) => {
      console.log(`[MessageController ${this._instanceId}] Conversa criada:`, event);
      
      // Atualiza stores
      this.stores.conversationId.set(event.id);
      this.stores.conversationTitle.set(event.title || 'Nova conversa');
      
      // NÃO recarrega conversa aqui - as mensagens virão via chat:stream
      // Se recarregar agora, vai sobrescrever mensagens que já estão sendo streamadas
    };
    
    EventsOn('chat:conversation_created', handleConversationCreated);
    this._backendUnsubscribers.push(() => {
      if (EventsOff) EventsOff('chat:conversation_created', handleConversationCreated);
    });
    
    // Evento: chat:tools_start (início de execução de tools)
    const handleToolsStart = (event) => {
      const currentConvId = get(this.stores.conversationId);
      // Só processa se for desta conversa OU se ainda não temos conversationId (nova conversa)
      if (currentConvId !== null && event.conversationId !== currentConvId) return;
      
      console.log(`[MessageController ${this._instanceId}] Tools iniciando:`, event.tools);
      this.stores.executingTools.set(event.tools || []);
      this.stores.toolsMessage.set('Executando ferramentas...');
    };
    
    EventsOn('chat:tools_start', handleToolsStart);
    this._backendUnsubscribers.push(() => {
      if (EventsOff) EventsOff('chat:tools_start', handleToolsStart);
    });
    
    // Evento: chat:tools_end (fim de execução de tools)
    const handleToolsEnd = (event) => {
      const currentConvId = get(this.stores.conversationId);
      // Só processa se for desta conversa OU se ainda não temos conversationId (nova conversa)
      if (currentConvId !== null && event.conversationId !== currentConvId) return;
      
      console.log(`[MessageController ${this._instanceId}] Tools finalizadas`);
      this.stores.executingTools.set([]);
      this.stores.toolsMessage.set(null);
    };
    
    EventsOn('chat:tools_end', handleToolsEnd);
    this._backendUnsubscribers.push(() => {
      if (EventsOff) EventsOff('chat:tools_end', handleToolsEnd);
    });
    
    this._boundToBackend = true;
    console.log(`[MessageController ${this._instanceId}] Eventos backend conectados`);
  }
  
  /**
   * Carrega conversa do banco de dados
   * 
   * @param {number} conversationId - ID da conversa
   */
  async loadConversation(conversationId) {
    if (!GetConversationInfo || !GetMessages) {
      throw new Error('Funções Wails não disponíveis');
    }
    
    console.log(`[MessageController ${this._instanceId}] Carregando conversa ${conversationId}...`);
    
    try {
      // Busca info da conversa
      const info = await GetConversationInfo(conversationId);
      console.log(`[MessageController ${this._instanceId}] Info da conversa:`, info);
      
      // Atualiza stores
      this.stores.conversationId.set(info.id);
      this.stores.conversationTitle.set(info.title || 'Conversa');
      
      // Busca mensagens
      const messages = await GetMessages(conversationId);
      console.log(`[MessageController ${this._instanceId}] Mensagens recebidas:`, messages?.length);
      
      // Substitui mensagens completamente
      this.stores.messages.set(messages || []);
      
      console.log(`[MessageController ${this._instanceId}] Conversa ${conversationId} carregada`);
    } catch (err) {
      console.error(`[MessageController ${this._instanceId}] Erro ao carregar conversa:`, err);
      throw err;
    }
  }
  
  /**
   * Limpa state (nova conversa)
   */
  clear() {
    console.log(`[MessageController ${this._instanceId}] Limpando state...`);
    
    this.stores.messages.set([]);
    this.stores.conversationId.set(null);
    this.stores.conversationTitle.set('');
    this.stores.isStreaming.set(false);
    this.stores.executingTools.set([]);
    this.stores.toolsMessage.set(null);
    this.stores.streamingMessageId.set(null);
    this.stores.streamingContent.set('');
  }
  
  /**
   * Atualiza modelo da conversa
   * 
   * @param {string} model - Nome do modelo
   */
  async updateModel(model) {
    const convId = get(this.stores.conversationId);
    if (!convId || !UpdateConversationModel) return;
    
    console.log(`[MessageController ${this._instanceId}] Atualizando modelo para: ${model}`);
    await UpdateConversationModel(convId, model);
  }
  
  /**
   * Atualiza configurações da conversa
   * 
   * @param {boolean} showInternal - Mostrar mensagens internas
   */
  async updateSettings(showInternal) {
    const convId = get(this.stores.conversationId);
    if (!convId || !UpdateConversationSettings) return;
    
    console.log(`[MessageController ${this._instanceId}] Atualizando settings`);
    await UpdateConversationSettings(convId, { show_internal_messages: showInternal });
  }
  
  /**
   * Carrega mensagens filhas de uma mensagem (lazy loading para threads)
   * 
   * @param {number} messageId - ID da mensagem pai
   * @returns {Promise<Array>} MessageNodes dos filhos
   */
  async loadChildren(messageId) {
    if (!GetMessages) {
      console.warn(`[MessageController ${this._instanceId}] GetMessages não disponível`);
      return [];
    }
    
    try {
      console.log(`[MessageController ${this._instanceId}] Carregando filhos de mensagem ${messageId}`);
      const children = await GetMessages(0, messageId);
      return children || [];
    } catch (error) {
      console.error(`[MessageController ${this._instanceId}] Erro ao carregar filhos:`, error);
      return [];
    }
  }
  
  /**
   * Marca conversa como última acessada
   */
  async setAsLastConversation() {
    const convId = get(this.stores.conversationId);
    if (!convId || !SetLastConversation) return;
    
    await SetLastConversation(convId);
  }
  
  /**
   * Limpa recursos (event listeners)
   */
  destroy() {
    console.log(`[MessageController ${this._instanceId}] Destruindo...`);
    
    // Remove event listeners do backend
    this._backendUnsubscribers.forEach(unsub => unsub());
    this._backendUnsubscribers = [];
    this._boundToBackend = false;
    
    console.log(`[MessageController ${this._instanceId}] Destruído`);
  }
}
