import { beforeEach, describe, expect, it, vi } from 'vitest';
import { handleExternalChatIncoming, type ExternalChatControllerAdapter } from './externalChatController';
import type { ChatEventControllerAdapter, ChatEventSession } from './chatEventController';

const mockStartChatEventController = vi.fn();
vi.mock('./chatEventController', () => ({
  startChatEventController: (...args: unknown[]) => mockStartChatEventController(...args),
}));

const emptySession: ChatEventSession = {
  conversation: null,
  activeToolCalls: [],
  completedSegments: [],
};

const chatEventAdapter: ChatEventControllerAdapter = {
  getSession: () => emptySession,
  patchSession: vi.fn(),
  patchConversation: vi.fn(),
  updateMessage: vi.fn(),
  updateReasoning: vi.fn(),
  setConversationLoading: vi.fn(),
};

function createAdapter(hasConversationSession: boolean): ExternalChatControllerAdapter {
  return {
    hasConversationSession: vi.fn(() => hasConversationSession),
    loadConversationSession: vi.fn().mockResolvedValue(undefined),
    chatEventAdapter,
  };
}

describe('externalChatController', () => {
  beforeEach(() => {
    mockStartChatEventController.mockClear();
  });

  it('ignora eventos sem conversationId', async () => {
    const adapter = createAdapter(false);

    await handleExternalChatIncoming({
      channel: 'telegram',
      from: 'Maria',
      text: 'oi',
      conversationId: '',
    }, adapter);

    expect(adapter.hasConversationSession).not.toHaveBeenCalled();
    expect(adapter.loadConversationSession).not.toHaveBeenCalled();
    expect(mockStartChatEventController).not.toHaveBeenCalled();
  });

  it('carrega sessão ausente e inicia controller externo', async () => {
    const adapter = createAdapter(false);

    await handleExternalChatIncoming({
      channel: 'telegram',
      from: 'Maria',
      text: 'oi',
      conversationId: 'conversation-1',
    }, adapter);

    expect(adapter.loadConversationSession).toHaveBeenCalledWith('conversation-1');
    expect(mockStartChatEventController).toHaveBeenCalledWith({
      conversationId: 'conversation-1',
      external: { channel: 'telegram', from: 'Maria', text: 'oi' },
      adapter: chatEventAdapter,
    });
  });

  it('não recarrega sessão já existente', async () => {
    const adapter = createAdapter(true);

    await handleExternalChatIncoming({
      channel: 'slack',
      from: 'Joao',
      text: 'ola',
      conversationId: 'conversation-2',
    }, adapter);

    expect(adapter.loadConversationSession).not.toHaveBeenCalled();
    expect(mockStartChatEventController).toHaveBeenCalledWith(expect.objectContaining({
      conversationId: 'conversation-2',
    }));
  });
});
