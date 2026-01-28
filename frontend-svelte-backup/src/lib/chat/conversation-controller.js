import { get } from 'svelte/store';
import { EventRouter } from '../events/event-router.js';

let Wails = null;
let Loaded = false;
async function ensureWails() {
  if (Loaded) return;
  try {
    const api = await import('../../../wailsjs/go/main/App.js');
    Wails = api;
  } catch (e) {
    console.warn('[ConversationController] Funções Wails não disponíveis:', e);
    Wails = {};
  }
  Loaded = true;
}

/**
 * ConversationController - controla uma conversa por aba
 * - liga/desliga eventos escopados por conversa
 * - executa operações via Wails e atualiza stores
 */
export class ConversationController {
  constructor(stores, { instanceId = null } = {}) {
    this.stores = stores;
    this.instanceId = instanceId || crypto.randomUUID?.() || String(Date.now());
    this.router = new EventRouter();
    this._bound = false;
  }

  async init() {
    await ensureWails();
  }

  /**
   * Conecta eventos escopados para a conversa atual
   */
  async bind() {
    if (this._bound) return;
    await this.init();
    const convId = get(this.stores.conversationId);
    if (convId) {
      this.router.setFilter({ conversationId: convId });
    }

    // messages_ready: associa IDs e inicia streaming
    await this.router.on('chat:messages_ready', (event) => {
      if (!event) return;
      console.log('[ConversationController] messages_ready recebido:', {
        conversationId: event?.conversationId ?? event?.ConversationId,
        title: event?.title,
        userMessageId: event?.userMessageId
      });
      const id = event?.conversationId ?? event?.ConversationId ?? convId;
      const current = get(this.stores.conversationId);
      // Bootstrap: se ainda não temos conversationId, adota do evento
      if (!current && id) {
        this.stores.conversationId.set(id);
        this.router.setFilter({ conversationId: id });
      }
      if (!id || id !== get(this.stores.conversationId)) return;

      if (event?.title) this.stores.conversationTitle.set(event.title);
      if (event?.userMessageId) {
        this.stores.messages.update(msgs => {
          const arr = Array.isArray(msgs) ? [...msgs] : [];
          for (let i = arr.length - 1; i >= 0; i--) {
            const m = arr[i];
            if ((m?.role ?? m?.message?.role) === 'user' && (m?.id ?? m?.ID ?? m?.message?.id ?? m?.message?.ID) == null) {
              arr[i] = { ...(m?.message ? { ...m, message: { ...m.message, id: event.userMessageId, ID: event.userMessageId } } : { ...m, id: event.userMessageId, ID: event.userMessageId }) };
              break;
            }
          }
          return arr;
        });
      }
      this.stores.isStreaming.set(true);
    }, { fallbackGlobal: true });

    // conversa criada: define ID/título cedo
    await this.router.on('chat:conversation_created', (event) => {
      if (!event) return;
      const id = event?.id ?? event?.conversationId ?? event?.ConversationId;
      if (!id) return;
      const current = get(this.stores.conversationId);
      if (!current) {
        this.stores.conversationId.set(id);
        if (event?.title) this.stores.conversationTitle.set(event.title);
        this.router.setFilter({ conversationId: id });
      }
    }, { fallbackGlobal: true });

    // stream: atualiza última resposta
    await this.router.on('chat:stream', (event) => {
      if (!event) return;
      console.log('[ConversationController] stream evento:', {
        done: !!event.done,
        hasContent: !!event.content,
        contentLength: (event.content || '').length
      });
      const done = !!event.done;
      this.stores.isStreaming.set(!done);
      this.stores.messages.update(msgs => {
        const arr = Array.isArray(msgs) ? [...msgs] : [];
        const last = arr[arr.length - 1];
        if (last && (last.isStreaming || last.role === 'assistant')) {
          arr[arr.length - 1] = { ...last, content: event.content || '', isStreaming: !done };
        } else {
          arr.push({ role: 'assistant', content: event.content || '', isStreaming: !done, id: null });
        }
        return arr;
      });
    }, { fallbackGlobal: true });

    // done: finaliza estados
    await this.router.on('chat:done', (_event) => {
      this.stores.isStreaming.set(false);
      this.stores.streamingMessageId.set(null);
      this.stores.streamingContent.set('');
      this.stores.executingTools.set([]);
      this.stores.toolsMessage.set(null);
      this.stores.messages.update(arr => {
        if (!Array.isArray(arr) || arr.length === 0) return arr;
        const copy = [...arr];
        const idx = copy.length - 1;
        if (copy[idx]?.isStreaming) copy[idx] = { ...copy[idx], isStreaming: false };
        return copy;
      });
    }, { fallbackGlobal: true });

    // erro de streaming: finalizar estados
    await this.router.on('chat:error', (_event) => {
      this.stores.isStreaming.set(false);
      this.stores.streamingMessageId.set(null);
      this.stores.streamingContent.set('');
      this.stores.executingTools.set([]);
      this.stores.toolsMessage.set(null);
      this.stores.messages.update(arr => {
        if (!Array.isArray(arr) || arr.length === 0) return arr;
        const copy = [...arr];
        const idx = copy.length - 1;
        if (copy[idx]?.isStreaming) copy[idx] = { ...copy[idx], isStreaming: false };
        return copy;
      });
    }, { fallbackGlobal: true });

    // tools start/end (consolidado em 'chat:tools' e resultados em 'chat:tool_results')
    await this.router.on('chat:tools', (event) => {
      if (!event) return;
      const status = event.status || 'start';
      if (status === 'start') {
        this.stores.executingTools.set(event.tools || []);
        this.stores.toolsMessage.set(event.message || 'Executando ferramentas...');
      }
    }, { fallbackGlobal: true });
    await this.router.on('chat:tool_results', (event) => {
      if (!event) return;
      this.stores.executingTools.set([]);
      this.stores.toolsMessage.set(null);
    }, { fallbackGlobal: true });

    // mensagens internas (agentes/tools) - opcionalmente podem popular threads
    await this.router.on('chat:internal_message', (_event) => {
      // Mantemos leve: UI já suporta exibição condicional de internas
      // Se desejado, podemos anexar às threads via threadedMessages depois
    }, { fallbackGlobal: true });

    this._bound = true;
  }

  async loadConversation(conversationId) {
    await this.init();
    if (!Wails?.GetConversationInfo || !Wails?.GetMessages) {
      throw new Error('APIs Wails indisponíveis');
    }
    const info = await Wails.GetConversationInfo(conversationId);
    this.stores.conversationId.set(info.id);
    this.stores.conversationTitle.set(info.title || 'Conversa');
    // Marca como recentemente usada mesmo sem enviar mensagem
    try { if (Wails?.TouchConversation) await Wails.TouchConversation(conversationId); } catch (e) { /* opcional: log leve */ }
    const messages = await Wails.GetMessages(conversationId);
    this.stores.messages.set(messages || []);
    // Reajusta filtro do router e faz bind
    this.router.setFilter({ conversationId });
    await this.bind();
  }

  async send({ text, media = [], params }) {
    await this.init();
    if (!Wails?.SendMessage) throw new Error('SendMessage indisponível');
    const convId = get(this.stores.conversationId) || 0;
    const mediaJson = media.length > 0 ? JSON.stringify(media) : '';
    return Wails.SendMessage(convId, text || '', mediaJson, params || {});
  }

  async updateSettings(showInternal) {
    await this.init();
    const convId = get(this.stores.conversationId);
    if (!convId || !Wails?.UpdateConversationSettings) return;
    await Wails.UpdateConversationSettings(convId, { show_internal_messages: !!showInternal });
  }

  async loadChildren(messageId) {
    await this.init();
    if (!Wails?.GetMessages) return [];
    try {
      const children = await Wails.GetMessages(0, messageId);
      return children || [];
    } catch (e) {
      console.warn('[ConversationController] Erro ao carregar filhos:', e);
      return [];
    }
  }

  /**
   * Limpa estado para iniciar uma nova conversa
   */
  clear() {
    this.stores.messages.set([]);
    this.stores.conversationId.set(null);
    this.stores.conversationTitle.set('');
    this.stores.isStreaming.set(false);
    this.stores.executingTools.set([]);
    this.stores.toolsMessage.set(null);
    this.stores.streamingMessageId.set(null);
    this.stores.streamingContent.set('');
    // Remove escopo atual de eventos
    try { this.router.setFilter({ conversationId: null }); } catch {}
  }

  destroy() { this.router.dispose(); this._bound = false; }
}
