import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const currentConversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
const targetConversationId = '01926b90-7a5a-7c4e-8d3f-000000000002';
const ensureConversationSurfaceSessionMock = vi.fn();
const removeConversationSurfaceSessionMock = vi.fn();
const retryMessageToConversationMock = vi.fn().mockResolvedValue(undefined);

const chatStoreState = {
  sessionsByConversationId: {
    [currentConversationId]: {
      conversation: { id: currentConversationId, title: 'Conversa', threadedMessages: [] },
      isLoading: false,
      hasOlderMessages: true,
      isLoadingOlderMessages: false,
      queuedTurnCount: 0,
    },
  },
  timelinesByConversationId: {},
  surfaceSessionsByKey: {},
  ensureConversationSurfaceSession: ensureConversationSurfaceSessionMock,
  removeConversationSurfaceSession: removeConversationSurfaceSessionMock,
  retryMessageToConversation: retryMessageToConversationMock,
  loadOlderMessagesForConversation: vi.fn(),
  loadMessageChildren: vi.fn(),
  loadConversationSession: vi.fn(),
  updateConversationMessage: vi.fn(),
  clearConversationMessages: vi.fn(),
  startConversationEditing: vi.fn(),
  startConversationReading: vi.fn(),
  setConversationEditingMessageId: vi.fn(),
  setConversationReadingMessageId: vi.fn(),
  toggleConversationThreadExpanded: vi.fn(),
  toggleConversationReasoningExpanded: vi.fn(),
  isConversationReasoningExpanded: vi.fn(() => false),
};

vi.mock('../../store/chatStore', () => ({
  useChatStore: (selector?: (state: typeof chatStoreState) => unknown) => {
    if (typeof selector === 'function') {
      return selector(chatStoreState);
    }
    return chatStoreState;
  },
}));

vi.mock('../workspace/WorkspacePanelContext', () => ({
  useWorkspacePanel: () => ({
    tab: { id: 'tab-chat', type: 'chat', title: 'Chat', position: 0 },
    isActive: true,
  }),
}));

import { ChatSessionProvider, useChatSession } from './ChatSessionContext';

function Probe() {
  const { retryMessageToConversation } = useChatSession();
  return (
    <button
      type="button"
      onClick={() => retryMessageToConversation(targetConversationId, 'message-1')}
    >
      retry
    </button>
  );
}

describe('ChatSessionProvider', () => {
  beforeEach(() => {
    ensureConversationSurfaceSessionMock.mockClear();
    removeConversationSurfaceSessionMock.mockClear();
    retryMessageToConversationMock.mockClear();
  });

  it('materializa sessão de superfície ao montar sessionKey não padrão', async () => {
    const { unmount } = render(
      <ChatSessionProvider
        conversationId={currentConversationId}
        surfaceType="embedded"
        surfaceId="embedded:workspace-chat-modal:tab-chat"
      >
        <Probe />
      </ChatSessionProvider>,
    );

    await waitFor(() => {
      expect(ensureConversationSurfaceSessionMock).toHaveBeenCalledWith(
        currentConversationId,
        `embedded:workspace-chat-modal:tab-chat:${currentConversationId}`,
        expect.objectContaining({
          conversationId: currentConversationId,
          sessionKey: `embedded:workspace-chat-modal:tab-chat:${currentConversationId}`,
          surfaceId: 'embedded:workspace-chat-modal:tab-chat',
          surfaceType: 'embedded',
          tabId: 'tab-chat',
        }),
      );
    });

    unmount();

    expect(removeConversationSurfaceSessionMock).toHaveBeenCalledWith(
      `embedded:workspace-chat-modal:tab-chat:${currentConversationId}`,
    );
  });

  it('normaliza origin do retry usando a conversa alvo', async () => {
    const user = userEvent.setup();
    render(
      <ChatSessionProvider
        conversationId={currentConversationId}
        surfaceType="embedded"
        surfaceId="embedded:workspace-chat-modal:tab-chat"
      >
        <Probe />
      </ChatSessionProvider>,
    );

    await user.click(screen.getByRole('button', { name: 'retry' }));

    expect(retryMessageToConversationMock).toHaveBeenCalledWith(
      targetConversationId,
      'message-1',
      undefined,
      {
        origin: expect.objectContaining({
          conversationId: targetConversationId,
          sessionKey: `embedded:workspace-chat-modal:tab-chat:${targetConversationId}`,
          surfaceId: 'embedded:workspace-chat-modal:tab-chat',
          surfaceType: 'embedded',
          tabId: 'tab-chat',
        }),
      },
    );
  });
});
