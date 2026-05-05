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

import { ChatSessionProvider, useChatSession } from './ChatSessionContext';
import type { ChatSurfaceIdentity } from '../../services/chatSessionRegistry';

const embeddedSurfaceId = 'embedded:workspace-chat-modal:tab-chat';
const embeddedSessionKey = `${embeddedSurfaceId}:${currentConversationId}`;
const embeddedSurface = (overrides: Partial<ChatSurfaceIdentity> = {}): ChatSurfaceIdentity => ({
  conversationId: currentConversationId,
  sessionKey: embeddedSessionKey,
  surfaceId: embeddedSurfaceId,
  surfaceType: 'embedded',
  tabId: 'tab-chat',
  ...overrides,
});

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

function NamedDraftProbe({ nextDraft }: { nextDraft: string }) {
  const { draftMessage, setDraftMessage } = useChatSession();
  return (
    <button
      type="button"
      onClick={() => setDraftMessage(nextDraft)}
    >
      {draftMessage}
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
      <ChatSessionProvider surface={embeddedSurface()}>
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
      <ChatSessionProvider surface={embeddedSurface()}>
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
      <ChatSessionProvider surface={embeddedSurface()}>
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
      <ChatSessionProvider surface={embeddedSurface()}>
        <Probe />
      </ChatSessionProvider>,
    );

    chatStoreState.surfaceSessionsByKey = {};
    rerender(
      <ChatSessionProvider surface={embeddedSurface()}>
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
      <ChatSessionProvider surface={embeddedSurface()}>
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

  it('usa tabId da identidade de superfície ao normalizar retry', async () => {
    const user = userEvent.setup();
    render(
      <ChatSessionProvider
        surface={embeddedSurface({ tabId: 'tab-from-surface' })}
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
          tabId: 'tab-from-surface',
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
      <ChatSessionProvider surface={embeddedSurface()}>
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

  it('mantém duas superfícies montadas da mesma conversa com rascunhos independentes', async () => {
    const user = userEvent.setup();
    chatStoreState.surfaceSessionsByKey = {
      [`editor-a:${currentConversationId}`]: {
        sessionKey: `editor-a:${currentConversationId}`,
        conversationId: currentConversationId,
        isLoading: false,
        hasOlderMessages: false,
        isLoadingOlderMessages: false,
        streamingMessageId: null,
        streamingReasoning: null,
        isThinking: false,
        activeToolCalls: [],
        completedSegments: [],
        draftMessage: 'rascunho editor A',
        draftMediaFiles: [],
        expandedThreads: new Set(),
        expandedReasonings: new Set(),
        editingMessageId: null,
        readingMessageId: null,
        skipFocusRestore: false,
      },
      [`editor-b:${currentConversationId}`]: {
        sessionKey: `editor-b:${currentConversationId}`,
        conversationId: currentConversationId,
        isLoading: false,
        hasOlderMessages: false,
        isLoadingOlderMessages: false,
        streamingMessageId: null,
        streamingReasoning: null,
        isThinking: false,
        activeToolCalls: [],
        completedSegments: [],
        draftMessage: 'rascunho editor B',
        draftMediaFiles: [],
        expandedThreads: new Set(),
        expandedReasonings: new Set(),
        editingMessageId: null,
        readingMessageId: null,
        skipFocusRestore: false,
      },
    };

    render(
      <>
        <ChatSessionProvider
          surface={{
            conversationId: currentConversationId,
            sessionKey: `editor-a:${currentConversationId}`,
            surfaceId: 'editor-a',
            surfaceType: 'embedded',
            tabId: 'tab-editor-a',
          }}
        >
          <NamedDraftProbe nextDraft="novo A" />
        </ChatSessionProvider>
        <ChatSessionProvider
          surface={{
            conversationId: currentConversationId,
            sessionKey: `editor-b:${currentConversationId}`,
            surfaceId: 'editor-b',
            surfaceType: 'embedded',
            tabId: 'tab-editor-b',
          }}
        >
          <NamedDraftProbe nextDraft="novo B" />
        </ChatSessionProvider>
      </>,
    );

    await user.click(screen.getByRole('button', { name: 'rascunho editor A' }));
    await user.click(screen.getByRole('button', { name: 'rascunho editor B' }));

    expect(setConversationDraftMessageMock).toHaveBeenNthCalledWith(
      1,
      currentConversationId,
      'novo A',
      `editor-a:${currentConversationId}`,
    );
    expect(setConversationDraftMessageMock).toHaveBeenNthCalledWith(
      2,
      currentConversationId,
      'novo B',
      `editor-b:${currentConversationId}`,
    );
  });
});
