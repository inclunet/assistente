import { describe, expect, it } from 'vitest';
import {
  buildWorkspaceModalChatSurfaceId,
  createChatSurfaceIdentity,
  createChatSurfaceOrigin,
  createEmptyChatSurfaceSession,
  getChatSurfaceSessionsForConversation,
  getChatSession,
  getConversationInfo,
  normalizeChatSurfaceOrigin,
  patchChatConversation,
  patchChatSession,
  removeChatSession,
  type ConversationInfo,
  type ChatSessionRegistryState,
} from './chatSessionRegistry';

const conversation = (id: string): ConversationInfo => ({ id, title: `Conversa ${id}` });

describe('chatSessionRegistry', () => {
  it('cria identidade canônica para superfície de aba', () => {
    const identity = createChatSurfaceIdentity({
      conversationId: 'conversation-1',
      surfaceType: 'page',
      tabId: 'tab-chat-1',
    });

    expect(identity).toEqual({
      conversationId: 'conversation-1',
      sessionKey: 'page:tab:tab-chat-1:conversation-1',
      surfaceId: 'page:tab:tab-chat-1',
      surfaceType: 'page',
      tabId: 'tab-chat-1',
    });
    expect(createChatSurfaceOrigin(identity)).toEqual(identity);
  });

  it('cria identidade canônica para modal do workspace vinculado ao painel', () => {
    const surfaceId = buildWorkspaceModalChatSurfaceId('tab-terminal-1');
    const identity = createChatSurfaceIdentity({
      conversationId: 'conversation-2',
      surfaceId,
      surfaceType: 'modal',
      tabId: 'tab-terminal-1',
    });

    expect(identity.surfaceId).toBe('modal:workspace-chat:tab-terminal-1');
    expect(identity.sessionKey).toBe('modal:workspace-chat:tab-terminal-1:conversation-2');
    expect(identity.tabId).toBe('tab-terminal-1');
  });

  it('cria identidade standalone somente quando não há tabId explícito', () => {
    expect(createChatSurfaceIdentity({ surfaceType: 'embedded' })).toMatchObject({
      conversationId: null,
      sessionKey: 'embedded:standalone:none',
      surfaceId: 'embedded:standalone',
      surfaceType: 'embedded',
    });
  });

  it('preserva sessionKey explícita durante migração de superfícies existentes', () => {
    expect(createChatSurfaceIdentity({
      conversationId: 'conversation-1',
      sessionKey: 'legacy-surface:conversation-1',
      surfaceId: 'modal:workspace-chat:tab-a',
      surfaceType: 'modal',
      tabId: 'tab-a',
    })).toMatchObject({
      conversationId: 'conversation-1',
      sessionKey: 'legacy-surface:conversation-1',
      surfaceId: 'modal:workspace-chat:tab-a',
      surfaceType: 'modal',
      tabId: 'tab-a',
    });
  });

  it('retorna sessão vazia para conversa ausente', () => {
    const state: ChatSessionRegistryState = {
      conversationInfoById: {},
      surfaceSessionsByKey: {},
    };

    expect(getChatSession(state, 'missing')).toMatchObject({
      conversation: null,
      isLoading: false,
      draftMessage: '',
      queuedTurnCount: 0,
    });
  });

  it('normaliza origin recalculando sessionKey quando a conversa muda', () => {
    const origin = {
      sessionKey: 'tab-a:old-conversation',
      conversationId: 'old-conversation',
      surfaceId: 'tab-a',
      surfaceType: 'page' as const,
    };

    expect(normalizeChatSurfaceOrigin(origin, 'new-conversation')).toMatchObject({
      conversationId: 'new-conversation',
      sessionKey: 'tab-a:new-conversation',
    });
  });

  it('normaliza origin preservando sessionKey quando a conversa já corresponde', () => {
    const origin = {
      sessionKey: 'tab-a:conversation-1',
      conversationId: 'conversation-1',
      surfaceId: 'tab-a',
      surfaceType: 'page' as const,
    };

    expect(normalizeChatSurfaceOrigin(origin, 'conversation-1')).toMatchObject({
      conversationId: 'conversation-1',
      sessionKey: 'tab-a:conversation-1',
    });
  });

  it('aplica patch criando sessão quando necessário', () => {
    const state: ChatSessionRegistryState = {
      conversationInfoById: {},
      surfaceSessionsByKey: {},
    };

    const next = patchChatSession(state, 'conversation-1', { isLoading: true });

    expect(next.surfaceSessionsByKey['conversation:' + 'conversation-1']).toMatchObject({
      isLoading: true,
      conversationId: 'conversation-1',
    });
    expect(next.surfaceSessionsByKey?.['conversation:conversation-1']).toMatchObject({
      isLoading: true,
      conversationId: 'conversation-1',
    });
    expect(state.surfaceSessionsByKey['conversation:' + 'conversation-1']).toBeUndefined();
  });

  it('aplica updater na conversa quando ela existe', () => {
    const state: ChatSessionRegistryState = {
      conversationInfoById: { 'conversation-1': conversation('conversation-1') },
      surfaceSessionsByKey: {
        ['conversation:' + 'conversation-1']: {
          ...createEmptyChatSurfaceSession('conversation-1'),
        },
      },
    };

    const next = patchChatConversation(state, 'conversation-1', (current) => ({
      ...current,
      title: 'Renomeada',
    }));

    expect(state.conversationInfoById['conversation-1'].title).toBe('Conversa conversation-1');
    expect(next.conversationInfoById?.['conversation-1'].title).toBe('Renomeada');
  });

  it('remove sessão sem mutar o estado original', () => {
    const state: ChatSessionRegistryState = {
      conversationInfoById: { 'conversation-1': conversation('conversation-1') },
      surfaceSessionsByKey: {
        'conversation:conversation-1': createEmptyChatSurfaceSession('conversation-1'),
      },
    };

    const next = removeChatSession(state, 'conversation-1');

    expect(next.surfaceSessionsByKey['conversation:' + 'conversation-1']).toBeUndefined();
    expect(next.conversationInfoById?.['conversation-1']).toBeUndefined();
    expect(next.surfaceSessionsByKey?.['conversation:conversation-1']).toBeUndefined();
    expect(state.surfaceSessionsByKey['conversation:' + 'conversation-1']).toBeDefined();
  });

  it('compõe timeline compartilhada com sessão visual por chave', () => {
    const state: ChatSessionRegistryState = {
      conversationInfoById: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {
        'tab-a:conversation-1': {
          ...createEmptyChatSurfaceSession('conversation-1', 'tab-a:conversation-1'),
          expandedThreads: new Set(['message-1']),
        },
        'tab-b:conversation-1': {
          ...createEmptyChatSurfaceSession('conversation-1', 'tab-b:conversation-1'),
          expandedThreads: new Set(['message-2']),
        },
      },
    };

    expect(getChatSession(state, 'conversation-1', 'tab-a:conversation-1').conversation?.title).toBe('Conversa conversation-1');
    expect(getChatSession(state, 'conversation-1', 'tab-a:conversation-1').expandedThreads.has('message-1')).toBe(true);
    expect(getChatSession(state, 'conversation-1', 'tab-b:conversation-1').expandedThreads.has('message-1')).toBe(false);
    expect(getChatSession(state, 'conversation-1', 'tab-b:conversation-1').expandedThreads.has('message-2')).toBe(true);
  });

  it('mantém painéis independentes com timeline compartilhada', () => {
    const state: ChatSessionRegistryState = {
      conversationInfoById: {
        'conversation-1': {
          ...conversation('conversation-1'),
          title: 'Timeline compartilhada',
        },
      },
      surfaceSessionsByKey: {
        'tab-a:conversation-1': {
          ...createEmptyChatSurfaceSession('conversation-1', 'tab-a:conversation-1'),
          draftMessage: 'rascunho A',
          draftMediaFiles: [{ id: 'media-a' } as unknown as import('./mediaService').MediaFile],
          scrollTop: 120,
          scrollAnchorMessageId: 'message-a',
          expandedThreads: new Set(['thread-a']),
          editingMessageId: 'edit-a',
          isLoading: true,
        },
        'tab-b:conversation-1': {
          ...createEmptyChatSurfaceSession('conversation-1', 'tab-b:conversation-1'),
          draftMessage: 'rascunho B',
          draftMediaFiles: [],
          scrollTop: 540,
          scrollAnchorMessageId: 'message-b',
          expandedThreads: new Set(['thread-b']),
          editingMessageId: null,
          isLoading: false,
        },
      },
    };

    const tabA = getChatSession(state, 'conversation-1', 'tab-a:conversation-1');
    const tabB = getChatSession(state, 'conversation-1', 'tab-b:conversation-1');

    expect(tabA.conversation).toStrictEqual(tabB.conversation);
    expect(tabA.conversation).not.toHaveProperty('threadedMessages');
    expect(tabA.conversation).toBe(state.conversationInfoById['conversation-1']);
    expect(tabA.conversation?.title).toBe('Timeline compartilhada');
    expect(tabA.draftMessage).toBe('rascunho A');
    expect(tabB.draftMessage).toBe('rascunho B');
    expect(tabA.draftMediaFiles).toHaveLength(1);
    expect(tabB.draftMediaFiles).toHaveLength(0);
    expect(tabA.scrollTop).toBe(120);
    expect(tabB.scrollTop).toBe(540);
    expect(tabA.scrollAnchorMessageId).toBe('message-a');
    expect(tabB.scrollAnchorMessageId).toBe('message-b');
    expect(tabA.expandedThreads.has('thread-a')).toBe(true);
    expect(tabA.expandedThreads.has('thread-b')).toBe(false);
    expect(tabB.expandedThreads.has('thread-b')).toBe(true);
    expect(tabA.editingMessageId).toBe('edit-a');
    expect(tabB.editingMessageId).toBeNull();
    expect(tabA.isLoading).toBe(true);
    expect(tabB.isLoading).toBe(false);
  });

  it('não herda estado visual de compatibilidade em sessionKey não padrão', () => {
    const state: ChatSessionRegistryState = {
      conversationInfoById: { 'conversation-1': conversation('conversation-1') },
      surfaceSessionsByKey: {
        ['conversation:' + 'conversation-1']: {
          ...createEmptyChatSurfaceSession('conversation-1'),
          expandedThreads: new Set(['message-from-default']),
          editingMessageId: 'editing-default',
        },
      },
    };

    const defaultSession = getChatSession(state, 'conversation-1');
    const surfaceSession = getChatSession(state, 'conversation-1', 'tab-a:conversation-1');

    expect(defaultSession.expandedThreads.has('message-from-default')).toBe(true);
    expect(surfaceSession.expandedThreads.has('message-from-default')).toBe(false);
    expect(surfaceSession.editingMessageId).toBeNull();
    expect(surfaceSession.conversation?.title).toBe('Conversa conversation-1');
  });

  it('não grava estado visual de sessionKey não padrão na sessão padrão', () => {
    const state: ChatSessionRegistryState = {
      conversationInfoById: { 'conversation-1': conversation('conversation-1') },
      surfaceSessionsByKey: {
        ['conversation:' + 'conversation-1']: {
          ...createEmptyChatSurfaceSession('conversation-1'),
        },
      },
    };

    const next = {
      ...state,
      ...patchChatSession(state, 'conversation-1', {
        editingMessageId: 'message-from-tab',
        expandedThreads: new Set(['message-from-tab']),
      }, 'tab-a:conversation-1'),
    };

    expect(next.surfaceSessionsByKey['conversation:' + 'conversation-1'].editingMessageId).toBeNull();
    expect(next.surfaceSessionsByKey['conversation:' + 'conversation-1'].expandedThreads.has('message-from-tab')).toBe(false);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').editingMessageId).toBe('message-from-tab');
    expect(getConversationInfo(next, 'conversation-1')?.title).toBe('Conversa conversation-1');
  });

  it('atualiza timeline uma vez sem sobrescrever sessões visuais interessadas', () => {
    const state: ChatSessionRegistryState = {
      conversationInfoById: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {
        'tab-a:conversation-1': {
          ...createEmptyChatSurfaceSession('conversation-1', 'tab-a:conversation-1'),
          expandedThreads: new Set(['message-1']),
        },
        'tab-b:conversation-1': {
          ...createEmptyChatSurfaceSession('conversation-1', 'tab-b:conversation-1'),
          expandedThreads: new Set(['message-2']),
        },
      },
    };

    const next = {
      ...state,
      ...patchChatConversation(state, 'conversation-1', (current) => ({
        ...current,
        title: 'Timeline atualizada',
      })),
    };

    expect(getConversationInfo(next, 'conversation-1')?.title).toBe('Timeline atualizada');
    expect(getChatSurfaceSessionsForConversation(next, 'conversation-1')).toHaveLength(2);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').expandedThreads.has('message-1')).toBe(true);
    expect(getChatSession(next, 'conversation-1', 'tab-b:conversation-1').expandedThreads.has('message-2')).toBe(true);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').conversation?.title).toBe('Timeline atualizada');
    expect(getChatSession(next, 'conversation-1', 'tab-b:conversation-1').conversation?.title).toBe('Timeline atualizada');
  });

  it('não apaga timeline ao aplicar patch visual sem conversation', () => {
    const state: ChatSessionRegistryState = {
      conversationInfoById: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {},
    };

    const next = {
      ...state,
      ...patchChatSession(state, 'conversation-1', { isLoading: true }, 'tab-a:conversation-1'),
    };

    expect(getConversationInfo(next, 'conversation-1')?.title).toBe('Conversa conversation-1');
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').isLoading).toBe(true);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').conversation?.title).toBe('Conversa conversation-1');
  });

  it('reusa referência da timeline em patch visual', () => {
    const state: ChatSessionRegistryState = {
      conversationInfoById: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {},
    };

    const patch = patchChatSession(state, 'conversation-1', { isLoading: true }, 'tab-a:conversation-1');

    expect({ ...state, ...patch }.conversationInfoById).toBe(state.conversationInfoById);
    expect(getChatSession({ ...state, ...patch }, 'conversation-1', 'tab-a:conversation-1').isLoading).toBe(true);
  });

});
