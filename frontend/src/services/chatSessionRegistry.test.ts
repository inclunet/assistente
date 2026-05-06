import { describe, expect, it } from 'vitest';
import {
  buildWorkspaceModalChatSurfaceId,
  createChatSurfaceIdentity,
  createChatSurfaceOrigin,
  createEmptyChatSession,
  getChatSurfaceSessionsForConversation,
  getChatSession,
  getConversationTimeline,
  isPersistedTimelineNode,
  normalizeChatSurfaceOrigin,
  patchChatConversation,
  patchChatSession,
  removeChatSession,
  TRANSIENT_TIMELINE_NODE_ID_PREFIXES,
  type ActiveConversation,
  type ChatSessionRegistryState,
} from './chatSessionRegistry';
import type { MessageNode } from '../lib/chatMessageTree';

const conversation = (id: string): ActiveConversation => ({
  id,
  title: `Conversa ${id}`,
  threadedMessages: [],
});

const messageNode = (id: string, overrides: Partial<MessageNode['message']> = {}): MessageNode => ({
  message: {
    id,
    conversationId: 'conversation-1',
    role: 'user',
    content: id,
    createdAt: new Date(2026, 0, 1, 0, Number(id.replace(/\D/g, '') || 0)).toISOString(),
    timestamp: Date.UTC(2026, 0, 1, 0, Number(id.replace(/\D/g, '') || 0)),
    ...overrides,
  },
  children: [],
  childCount: 0,
  level: 0,
} as unknown as MessageNode);

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
      sessionsByConversationId: {},
      timelinesByConversationId: {},
      surfaceSessionsByKey: {},
    };

    expect(getChatSession(state, 'missing')).toMatchObject({
      conversation: null,
      isLoading: false,
      activeToolCalls: [],
      completedSegments: [],
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
      sessionsByConversationId: {},
      timelinesByConversationId: {},
      surfaceSessionsByKey: {},
    };

    const next = patchChatSession(state, 'conversation-1', { isLoading: true });

    expect(next.sessionsByConversationId['conversation-1']).toMatchObject({
      isLoading: true,
      conversation: null,
    });
    expect(next.surfaceSessionsByKey?.['conversation:conversation-1']).toMatchObject({
      isLoading: true,
      conversationId: 'conversation-1',
    });
    expect(state.sessionsByConversationId['conversation-1']).toBeUndefined();
  });

  it('aplica updater na conversa quando ela existe', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {
        'conversation-1': {
          ...createEmptyChatSession('conversation-1'),
          conversation: conversation('conversation-1'),
        },
      },
      timelinesByConversationId: {},
      surfaceSessionsByKey: {},
    };

    const next = patchChatConversation(state, 'conversation-1', (current) => ({
      ...current,
      title: 'Renomeada',
    }));

    expect(next.sessionsByConversationId?.['conversation-1'].conversation?.title).toBe('Renomeada');
    expect(next.timelinesByConversationId?.['conversation-1'].title).toBe('Renomeada');
  });

  it('remove sessão sem mutar o estado original', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {
        'conversation-1': {
          ...createEmptyChatSession('conversation-1'),
          conversation: conversation('conversation-1'),
        },
      },
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {
        'conversation:conversation-1': createEmptyChatSession('conversation-1'),
      },
    };

    const next = removeChatSession(state, 'conversation-1');

    expect(next.sessionsByConversationId['conversation-1']).toBeUndefined();
    expect(next.timelinesByConversationId?.['conversation-1']).toBeUndefined();
    expect(next.surfaceSessionsByKey?.['conversation:conversation-1']).toBeUndefined();
    expect(state.sessionsByConversationId['conversation-1']).toBeDefined();
  });

  it('compõe timeline compartilhada com sessão visual por chave', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {
        'tab-a:conversation-1': {
          ...createEmptyChatSession('conversation-1', 'tab-a:conversation-1'),
          expandedThreads: new Set(['message-1']),
        },
        'tab-b:conversation-1': {
          ...createEmptyChatSession('conversation-1', 'tab-b:conversation-1'),
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
      sessionsByConversationId: {},
      timelinesByConversationId: {
        'conversation-1': {
          ...conversation('conversation-1'),
          title: 'Timeline compartilhada',
        },
      },
      surfaceSessionsByKey: {
        'tab-a:conversation-1': {
          ...createEmptyChatSession('conversation-1', 'tab-a:conversation-1'),
          draftMessage: 'rascunho A',
          draftMediaFiles: [{ id: 'media-a' } as unknown as import('./mediaService').MediaFile],
          scrollTop: 120,
          scrollAnchorMessageId: 'message-a',
          expandedThreads: new Set(['thread-a']),
          editingMessageId: 'edit-a',
          isLoading: true,
        },
        'tab-b:conversation-1': {
          ...createEmptyChatSession('conversation-1', 'tab-b:conversation-1'),
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
    expect(tabA.conversation?.threadedMessages).toBe(tabB.conversation?.threadedMessages);
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
      sessionsByConversationId: {
        'conversation-1': {
          ...createEmptyChatSession('conversation-1'),
          conversation: conversation('conversation-1'),
          expandedThreads: new Set(['message-from-default']),
          editingMessageId: 'editing-default',
        },
      },
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {},
    };

    const defaultSession = getChatSession(state, 'conversation-1');
    const surfaceSession = getChatSession(state, 'conversation-1', 'tab-a:conversation-1');

    expect(defaultSession.expandedThreads.has('message-from-default')).toBe(true);
    expect(surfaceSession.expandedThreads.has('message-from-default')).toBe(false);
    expect(surfaceSession.editingMessageId).toBeNull();
    expect(surfaceSession.conversation?.title).toBe('Conversa conversation-1');
  });

  it('não grava estado visual de sessionKey não padrão em sessionsByConversationId', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {
        'conversation-1': {
          ...createEmptyChatSession('conversation-1'),
          conversation: conversation('conversation-1'),
        },
      },
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {},
    };

    const next = {
      ...state,
      ...patchChatSession(state, 'conversation-1', {
        editingMessageId: 'message-from-tab',
        expandedThreads: new Set(['message-from-tab']),
      }, 'tab-a:conversation-1'),
    };

    expect(next.sessionsByConversationId['conversation-1'].editingMessageId).toBeNull();
    expect(next.sessionsByConversationId['conversation-1'].expandedThreads.has('message-from-tab')).toBe(false);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').editingMessageId).toBe('message-from-tab');
    expect(getConversationTimeline(next, 'conversation-1')?.title).toBe('Conversa conversation-1');
  });

  it('atualiza timeline uma vez sem sobrescrever sessões visuais interessadas', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {
        'tab-a:conversation-1': {
          ...createEmptyChatSession('conversation-1', 'tab-a:conversation-1'),
          expandedThreads: new Set(['message-1']),
        },
        'tab-b:conversation-1': {
          ...createEmptyChatSession('conversation-1', 'tab-b:conversation-1'),
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

    expect(getConversationTimeline(next, 'conversation-1')?.title).toBe('Timeline atualizada');
    expect(getChatSurfaceSessionsForConversation(next, 'conversation-1')).toHaveLength(2);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').expandedThreads.has('message-1')).toBe(true);
    expect(getChatSession(next, 'conversation-1', 'tab-b:conversation-1').expandedThreads.has('message-2')).toBe(true);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').conversation?.title).toBe('Timeline atualizada');
    expect(getChatSession(next, 'conversation-1', 'tab-b:conversation-1').conversation?.title).toBe('Timeline atualizada');
  });

  it('reconcilia timeline compartilhada por chave de turno', () => {
    const streamingNode = messageNode('streaming-conversation-1-1', {
      role: 'assistant',
      turnId: 'user-1',
      isStreaming: true,
    });
    const canonicalNode = messageNode('assistant-final', {
      role: 'assistant',
      turnId: 'user-1',
      isStreaming: false,
    });
    const localAssistantNode = messageNode('assistant-local', {
      role: 'assistant',
      turnId: 'user-1',
      isStreaming: false,
    });
    const userNode = messageNode('user-1');
    userNode.originalIndex = 0;
    canonicalNode.originalIndex = 1;
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        'conversation-1': {
          ...conversation('conversation-1'),
          threadedMessages: [userNode, streamingNode, localAssistantNode],
        },
      },
      surfaceSessionsByKey: {},
    };

    const next = {
      ...state,
      ...patchChatSession(state, 'conversation-1', {
        conversation: {
          ...conversation('conversation-1'),
          threadedMessages: [userNode, canonicalNode],
        },
      }, 'tab-a:conversation-1'),
    };

    const ids = getConversationTimeline(next, 'conversation-1')?.threadedMessages.map((node) => node.message.id);
    expect(ids).toEqual(['user-1', 'assistant-final']);
  });

  it('não vaza nós transitórios para superfície nova da mesma conversa', () => {
    const userNode = messageNode('user-1');
    const transientNodes = TRANSIENT_TIMELINE_NODE_ID_PREFIXES.map((prefix) => (
      messageNode(`${prefix}local-1`, { role: prefix === 'tool-' ? 'tool' : 'assistant', turnId: 'user-1' })
    ));
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {},
      surfaceSessionsByKey: {},
    };

    const next = {
      ...state,
      ...patchChatSession(state, 'conversation-1', {
        conversation: {
          ...conversation('conversation-1'),
          threadedMessages: [userNode, ...transientNodes],
        },
      }, 'tab-a:conversation-1'),
    };

    expect(getConversationTimeline(next, 'conversation-1')?.threadedMessages.map((node) => node.message.id))
      .toEqual(['user-1']);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').conversation?.threadedMessages.map((node) => node.message.id))
      .toEqual(['user-1', ...transientNodes.map((node) => node.message.id)]);
    expect(getChatSession(next, 'conversation-1', 'tab-b:conversation-1').conversation?.threadedMessages.map((node) => node.message.id))
      .toEqual(['user-1']);
  });

  it.each(TRANSIENT_TIMELINE_NODE_ID_PREFIXES)(
    'descarta nó com prefixo "%s" do timeline cache compartilhado',
    (prefix) => {
      const userNode = messageNode('user-1');
      const transientNode = messageNode(`${prefix}local-1`, {
        role: prefix === 'tool-' ? 'tool' : 'assistant',
        turnId: 'user-1',
      });
      const state: ChatSessionRegistryState = {
        sessionsByConversationId: {},
        timelinesByConversationId: {},
        surfaceSessionsByKey: {},
      };

      const next = {
        ...state,
        ...patchChatSession(state, 'conversation-1', {
          conversation: {
            ...conversation('conversation-1'),
            threadedMessages: [userNode, transientNode],
          },
        }, 'tab-a:conversation-1'),
      };

      expect(getConversationTimeline(next, 'conversation-1')?.threadedMessages.map((node) => node.message.id))
        .toEqual(['user-1']);
      expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').conversation?.threadedMessages.map((node) => node.message.id))
        .toEqual(['user-1', transientNode.message.id]);
    },
  );

  it('mantém persistente nó com prefixo desconhecido', () => {
    expect(isPersistedTimelineNode(messageNode('unknown-source-1'))).toBe(true);
  });

  it('não apaga timeline ao aplicar patch visual sem conversation', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {},
    };

    const next = {
      ...state,
      ...patchChatSession(state, 'conversation-1', { isThinking: true }, 'tab-a:conversation-1'),
    };

    expect(getConversationTimeline(next, 'conversation-1')?.title).toBe('Conversa conversation-1');
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').isThinking).toBe(true);
    expect(getChatSession(next, 'conversation-1', 'tab-a:conversation-1').conversation?.title).toBe('Conversa conversation-1');
  });

  it('reusa referência da timeline em patch visual', () => {
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        'conversation-1': conversation('conversation-1'),
      },
      surfaceSessionsByKey: {},
    };

    const patch = patchChatSession(state, 'conversation-1', { isThinking: true }, 'tab-a:conversation-1');

    expect(patch.timelinesByConversationId).toBe(state.timelinesByConversationId);
    expect(getChatSession({ ...state, ...patch }, 'conversation-1', 'tab-a:conversation-1').isThinking).toBe(true);
  });

  it('reconcilia metadata da janela ao atualizar conversa em superfície visível', () => {
    const nodes = Array.from({ length: 10 }, (_, index) => messageNode(`message-${index}`));
    nodes.forEach((node, index) => {
      node.originalIndex = index;
    });
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        'conversation-1': {
          ...conversation('conversation-1'),
          threadedMessages: nodes,
        },
      },
      surfaceSessionsByKey: {
        'tab-a:conversation-1': {
          ...createEmptyChatSession('conversation-1', 'tab-a:conversation-1'),
          hasOlderMessages: true,
          visibleThreadedMessages: nodes.slice(5),
          messageWindow: {
            scope: 'conversation',
            conversationId: 'conversation-1',
            totalCount: 10,
            startIndex: 5,
            endIndex: 9,
            hasBefore: true,
            hasAfter: false,
          },
        },
      },
    };

    const next = {
      ...state,
      ...patchChatConversation(state, 'conversation-1', (current) => ({
        ...current,
        threadedMessages: [],
      })),
    };
    const tabA = getChatSession(next, 'conversation-1', 'tab-a:conversation-1');

    expect(tabA.visibleThreadedMessages).toEqual([]);
    expect(tabA.messageWindow).toMatchObject({
      totalCount: 0,
      startIndex: 0,
      endIndex: -1,
      hasBefore: false,
      hasAfter: false,
    });
    expect(tabA.hasOlderMessages).toBe(false);
  });

  it('limita a superfície sem cortar o turno no início da janela visual', () => {
    const nodes = Array.from({ length: 242 }, (_, index) => messageNode(`message-${index}`, {
      turnId: index <= 2 ? 'turn-boundary' : `turn-${index}`,
    }));
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {},
      surfaceSessionsByKey: {},
    };

    const next = {
      ...state,
      ...patchChatSession(state, 'conversation-1', {
        conversation: {
          ...conversation('conversation-1'),
          threadedMessages: nodes,
        },
      }),
    };
    const session = getChatSession(next, 'conversation-1');

    expect(getConversationTimeline(next, 'conversation-1')?.threadedMessages).toHaveLength(242);
    expect(session.visibleThreadedMessages?.map((node) => node.message.id).slice(0, 3)).toEqual([
      'message-0',
      'message-1',
      'message-2',
    ]);
  });

  it('materializa superfície padrão a partir da timeline com janela limitada', () => {
    const nodes = Array.from({ length: 245 }, (_, index) => messageNode(`message-${index}`));
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {
        'conversation-1': {
          ...createEmptyChatSession('conversation-1'),
          conversation: {
            ...conversation('conversation-1'),
            threadedMessages: nodes,
          },
        },
      },
      timelinesByConversationId: {},
      surfaceSessionsByKey: {},
    };

    const session = getChatSession(state, 'conversation-1');

    expect(session.conversation?.threadedMessages).toHaveLength(240);
    expect(session.visibleThreadedMessages).toHaveLength(240);
    expect(session.visibleThreadedMessages?.[0]?.message.id).toBe('message-5');
  });

  it('preserva superfície ancorada no início durante patch de conversa', () => {
    const nodes = Array.from({ length: 245 }, (_, index) => messageNode(`message-${index}`));
    nodes.forEach((node, index) => {
      node.originalIndex = index;
    });
    const state: ChatSessionRegistryState = {
      sessionsByConversationId: {},
      timelinesByConversationId: {
        'conversation-1': {
          ...conversation('conversation-1'),
          threadedMessages: nodes,
        },
      },
      surfaceSessionsByKey: {
        'tab-a:conversation-1': {
          ...createEmptyChatSession('conversation-1', 'tab-a:conversation-1'),
          visibleThreadedMessages: nodes,
          messageWindow: {
            scope: 'conversation',
            conversationId: 'conversation-1',
            totalCount: 245,
            startIndex: 0,
            endIndex: 244,
            hasBefore: false,
            hasAfter: true,
          },
        },
      },
    };

    const next = {
      ...state,
      ...patchChatConversation(state, 'conversation-1', (current) => ({
        ...current,
        threadedMessages: current.threadedMessages,
      })),
    };
    const tabA = getChatSession(next, 'conversation-1', 'tab-a:conversation-1');

    expect(tabA.visibleThreadedMessages?.[0]?.message.id).toBe('message-0');
    expect(tabA.visibleThreadedMessages).toHaveLength(240);
  });
});
