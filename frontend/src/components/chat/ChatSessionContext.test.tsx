import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';

const currentConversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
const targetConversationId = '01926b90-7a5a-7c4e-8d3f-000000000002';
const ensureConversationSurfaceSessionMock = vi.fn();
const removeConversationSurfaceSessionMock = vi.fn();
const retryMessageToConversationMock = vi.fn().mockResolvedValue(undefined);
const setConversationDraftMessageMock = vi.fn();
const setConversationDraftMediaFilesMock = vi.fn();

const chatStoreState = {
  sessionsByConversationId: {
    [currentConversationId]: {
      conversation: { id: currentConversationId, title: 'Conversa', threadedMessages: [] },
      isLoading: false,
      hasOlderMessages: true,
      isLoadingOlderMessages: false,
      queuedTurnCount: 0,
      draftMessage: '',
      draftMediaFiles: [],
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
  setConversationDraftMessage: setConversationDraftMessageMock,
  setConversationDraftMediaFiles: setConversationDraftMediaFilesMock,
  clearConversationDraft: vi.fn(),
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

const embeddedSurfaceId = 'embedded:workspace-chat-modal:tab-chat';
const embeddedSessionKey = `${embeddedSurfaceId}:${currentConversationId}`;

function Probe() {
  const { retryMessageToConversation, surface } = useChatSession();
  return (
    <button
      type="button"
      onClick={() => retryMessageToConversation(targetConversationId, 'message-1')}
    >
      {surface.sessionKey === embeddedSessionKey ? 'retry' : surface.sessionKey}
    </button>
  );
}

function DraftProbe() {
  const { draftMessage, setDraftMessage } = useChatSession();
  return (
    <button
      type="button"
      onClick={() => setDraftMessage('rascunho local')}
    >
      {draftMessage || 'sem rascunho'}
    </button>
  );
}

describe('ChatSessionProvider', () => {
  beforeEach(() => {
    ensureConversationSurfaceSessionMock.mockClear();
    removeConversationSurfaceSessionMock.mockClear();
    retryMessageToConversationMock.mockClear();
    setConversationDraftMessageMock.mockClear();
    setConversationDraftMediaFilesMock.mockClear();
    chatStoreState.surfaceSessionsByKey = {};
  });

  it('materializa sessão de superfície ao montar sessionKey não padrão', async () => {
    const { unmount } = render(
      <ChatSessionProvider
        conversationId={currentConversationId}
        surfaceType="embedded"
        surfaceId={embeddedSurfaceId}
      >
        <Probe />
      </ChatSessionProvider>,
    );

    await waitFor(() => {
      expect(ensureConversationSurfaceSessionMock).toHaveBeenCalledWith(
        currentConversationId,
        embeddedSessionKey,
        expect.objectContaining({
          conversationId: currentConversationId,
          sessionKey: embeddedSessionKey,
          surfaceId: embeddedSurfaceId,
          surfaceType: 'embedded',
          tabId: 'tab-chat',
        }),
      );
    });

    unmount();

    expect(removeConversationSurfaceSessionMock).toHaveBeenCalledWith(
      embeddedSessionKey,
    );
  });

  it('aceita identidade de superfície como contrato principal', async () => {
    render(
      <ChatSessionProvider
        surface={{
          conversationId: currentConversationId,
          sessionKey: embeddedSessionKey,
          surfaceId: embeddedSurfaceId,
          surfaceType: 'embedded',
          tabId: 'tab-chat',
        }}
      >
        <Probe />
      </ChatSessionProvider>,
    );

    await waitFor(() => {
      expect(ensureConversationSurfaceSessionMock).toHaveBeenCalledWith(
        currentConversationId,
        embeddedSessionKey,
        expect.objectContaining({
          conversationId: currentConversationId,
          sessionKey: embeddedSessionKey,
          surfaceId: embeddedSurfaceId,
          surfaceType: 'embedded',
          tabId: 'tab-chat',
        }),
      );
    });
    expect(screen.getByRole('button', { name: 'retry' })).toBeInTheDocument();
  });

  it('rematerializa superfície quando ela desaparece da store enquanto provider segue montado', async () => {
    const { rerender } = render(
      <ChatSessionProvider
        conversationId={currentConversationId}
        surfaceType="embedded"
        surfaceId={embeddedSurfaceId}
      >
        <Probe />
      </ChatSessionProvider>,
    );

    await waitFor(() => {
      expect(ensureConversationSurfaceSessionMock).toHaveBeenCalledTimes(1);
    });

    chatStoreState.surfaceSessionsByKey = {
      [embeddedSessionKey]: {
        sessionKey: embeddedSessionKey,
        conversationId: currentConversationId,
        isLoading: false,
        hasOlderMessages: false,
        isLoadingOlderMessages: false,
        streamingMessageId: null,
        streamingReasoning: null,
        isThinking: false,
        activeToolCalls: [],
        completedSegments: [],
        draftMessage: '',
        draftMediaFiles: [],
        expandedThreads: new Set(),
        expandedReasonings: new Set(),
        editingMessageId: null,
        readingMessageId: null,
        skipFocusRestore: false,
      },
    };
    rerender(
      <ChatSessionProvider
        conversationId={currentConversationId}
        surfaceType="embedded"
        surfaceId={embeddedSurfaceId}
      >
        <Probe />
      </ChatSessionProvider>,
    );

    chatStoreState.surfaceSessionsByKey = {};
    rerender(
      <ChatSessionProvider
        conversationId={currentConversationId}
        surfaceType="embedded"
        surfaceId={embeddedSurfaceId}
      >
        <Probe />
      </ChatSessionProvider>,
    );

    await waitFor(() => {
      expect(ensureConversationSurfaceSessionMock).toHaveBeenCalledTimes(2);
    });
  });

  it('normaliza origin do retry usando a conversa alvo', async () => {
    const user = userEvent.setup();
    render(
      <ChatSessionProvider
        conversationId={currentConversationId}
        surfaceType="embedded"
        surfaceId={embeddedSurfaceId}
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
          sessionKey: `${embeddedSurfaceId}:${targetConversationId}`,
          surfaceId: embeddedSurfaceId,
          surfaceType: 'embedded',
          tabId: 'tab-chat',
        }),
      },
    );
  });

  it('escopa rascunho pela sessionKey da superfície', async () => {
    const user = userEvent.setup();
    chatStoreState.surfaceSessionsByKey = {
      [embeddedSessionKey]: {
        sessionKey: embeddedSessionKey,
        conversationId: currentConversationId,
        isLoading: false,
        hasOlderMessages: false,
        isLoadingOlderMessages: false,
        streamingMessageId: null,
        streamingReasoning: null,
        isThinking: false,
        activeToolCalls: [],
        completedSegments: [],
        draftMessage: '',
        draftMediaFiles: [],
        expandedThreads: new Set(),
        expandedReasonings: new Set(),
        editingMessageId: null,
        readingMessageId: null,
        skipFocusRestore: false,
      },
    };

    render(
      <ChatSessionProvider
        conversationId={currentConversationId}
        surfaceType="embedded"
        surfaceId={embeddedSurfaceId}
      >
        <DraftProbe />
      </ChatSessionProvider>,
    );

    await user.click(screen.getByRole('button', { name: 'sem rascunho' }));

    expect(setConversationDraftMessageMock).toHaveBeenCalledWith(
      currentConversationId,
      'rascunho local',
      embeddedSessionKey,
    );
  });
});
