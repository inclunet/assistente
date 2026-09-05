import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { ChatSessionContextValue } from './ChatSessionContext';
import type { ChatSurfaceOrigin } from '../../services/chatSessionRegistry';

const sendMessageToConversationMock = vi.fn();
const useChatSessionMock = vi.fn();

vi.mock('../../store/chatStore', () => ({
  useChatStore: (selector: (state: { sendMessageToConversation: typeof sendMessageToConversationMock }) => unknown) => selector({
    sendMessageToConversation: sendMessageToConversationMock,
  }),
}));

vi.mock('./ChatSessionContext', () => ({
  useChatSession: () => useChatSessionMock(),
}));

vi.mock('i18next', () => ({
  default: {
    t: (key: string) => key,
  },
}));

import {
  useChatSurfaceController,
  type ChatSurfaceSendHandler,
} from './ChatSurfaceController';

const baseOrigin: ChatSurfaceOrigin = {
  sessionKey: 'embedded:surface-a:conversation-a',
  conversationId: 'conversation-a',
  surfaceId: 'embedded:surface-a',
  surfaceType: 'embedded',
};

function createSession(overrides: Partial<ChatSessionContextValue> = {}): ChatSessionContextValue {
  return {
    surface: {
      conversationId: baseOrigin.conversationId,
      sessionKey: baseOrigin.sessionKey,
      surfaceId: baseOrigin.surfaceId,
      surfaceType: baseOrigin.surfaceType,
    },
    origin: baseOrigin,
    conversationId: 'conversation-a',
    session: null,
    conversation: null,
    threadedMessages: [],
    isLoading: false,
    hasOlderMessages: false,
    isLoadingOlderMessages: false,
    isLoadingMessageWindow: false,
    hasNewerMessages: false,
    draftMessage: '',
    draftMediaFiles: [],
    scrollTop: 0,
    scrollAnchorMessageId: null,
    setDraftMessage: vi.fn(),
    setDraftMediaFiles: vi.fn(),
    clearDraft: vi.fn(),
    setScrollState: vi.fn(),
    loadOlderMessages: vi.fn(),
    loadNewerMessages: vi.fn(),
    loadStartMessages: vi.fn(),
    loadEndMessages: vi.fn(),
    loadConversationSession: vi.fn(),
    loadMessageChildren: vi.fn(),
    retryMessageToConversation: vi.fn(),
    updateConversationMessage: vi.fn(),
    updateConversationMessagePinned: vi.fn(),
    clearConversationMessages: vi.fn(),
    startConversationEditing: vi.fn(),
    startConversationReading: vi.fn(),
    setConversationEditingMessageId: vi.fn(),
    setConversationReadingMessageId: vi.fn(),
    toggleConversationThreadExpanded: vi.fn(),
    toggleConversationReasoningExpanded: vi.fn(),
    isConversationReasoningExpanded: vi.fn(),
    ...overrides,
  };
}

function SendProbe() {
  const controller = useChatSurfaceController();
  return (
    <button type="button" onClick={() => void controller.sendMessage('oi')}>
      send
    </button>
  );
}

function SendWithAdapterProbe({ onSend }: { onSend: ChatSurfaceSendHandler }) {
  const controller = useChatSurfaceController({ onSend });
  return (
    <button type="button" onClick={() => void controller.sendMessage('oi')}>
      send
    </button>
  );
}

function ErrorProbe() {
  const controller = useChatSurfaceController();
  const handleClick = async () => {
    try {
      await controller.sendMessage('oi');
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      document.body.setAttribute('data-error', message);
    }
  };
  return (
    <button type="button" onClick={() => void handleClick()}>
      send
    </button>
  );
}

describe('useChatSurfaceController', () => {
  beforeEach(() => {
    sendMessageToConversationMock.mockReset();
    useChatSessionMock.mockReset();
    document.body.removeAttribute('data-error');
  });

  it('envia pelo caminho padrão com origem normalizada', async () => {
    const user = userEvent.setup();
    useChatSessionMock.mockReturnValue(createSession({
      origin: {
        ...baseOrigin,
        conversationId: null,
      },
    }));

    render(<SendProbe />);
    await user.click(screen.getByRole('button', { name: 'send' }));

    await waitFor(() => {
      expect(sendMessageToConversationMock).toHaveBeenCalledWith(
        'conversation-a',
        'oi',
        undefined,
        undefined,
        {
          origin: expect.objectContaining({
            conversationId: 'conversation-a',
            sessionKey: 'embedded:surface-a:conversation-a',
            surfaceId: 'embedded:surface-a',
            surfaceType: 'embedded',
          }),
        },
      );
    });
  });

  it('lança erro traduzido quando não há conversa ativa', async () => {
    const user = userEvent.setup();
    useChatSessionMock.mockReturnValue(createSession({
      conversationId: null,
      origin: {
        ...baseOrigin,
        conversationId: null,
      },
    }));

    render(<ErrorProbe />);
    await user.click(screen.getByRole('button', { name: 'send' }));

    await waitFor(() => {
      expect(document.body).toHaveAttribute('data-error', 'chat.errors.chatTabNotReady');
    });
    expect(sendMessageToConversationMock).not.toHaveBeenCalled();
  });

  it('usa adapter de envio quando onSend é fornecido', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn().mockResolvedValue(undefined);
    useChatSessionMock.mockReturnValue(createSession());

    render(<SendWithAdapterProbe onSend={onSend} />);
    await user.click(screen.getByRole('button', { name: 'send' }));

    await waitFor(() => {
      expect(onSend).toHaveBeenCalledWith('oi', undefined, {
        conversationId: 'conversation-a',
        origin: baseOrigin,
      });
    });
    expect(sendMessageToConversationMock).not.toHaveBeenCalled();
  });
});
