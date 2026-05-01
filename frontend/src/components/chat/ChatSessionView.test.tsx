import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const updateMessageMock = vi.fn();
const showMenuMock = vi.fn();
const hideMenuMock = vi.fn();
const copyMessageMock = vi.fn();
const speakMessageMock = vi.fn();
const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
const activeConversation = { id: conversationId, title: 'Conversa', threadedMessages: [] };

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock('../../services/tts', () => ({
  ttsService: {
    isEnabled: () => false,
    hasVoiceConfig: () => false,
    on: vi.fn(),
    off: vi.fn(),
  },
}));

const chatStoreState = {
  retryMessageToConversation: vi.fn(),
  sessionsByConversationId: {
    [conversationId]: {
      conversation: activeConversation,
      isLoading: false,
      hasOlderMessages: false,
      isLoadingOlderMessages: false,
    },
  },
  loadMessageChildren: vi.fn(),
  loadConversationSession: vi.fn(),
  updateConversationMessage: updateMessageMock,
  toggleConversationReasoningExpanded: vi.fn(),
  isConversationReasoningExpanded: () => false,
  startConversationEditing: vi.fn(),
  startConversationReading: vi.fn(),
};

vi.mock('../../store/chatStore', () => ({
  useChatStore: (selector?: (s: typeof chatStoreState) => unknown) => {
    if (typeof selector === 'function') {
      return selector(chatStoreState);
    }
    return chatStoreState;
  },
}));

vi.mock('../../store/editorStore', () => ({
  useEditorStore: {
    getState: () => ({
      requestInsert: vi.fn(),
      activeTabId: 'chat-tab',
      tabs: [{ id: 'chat-tab', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a' }],
    }),
  },
}));

vi.mock('../../hooks/useChatKeyboardNav', () => ({
  useChatKeyboardNav: () => {},
}));

vi.mock('../../hooks/useContextMenu', () => ({
  useContextMenu: () => ({
    menuVisible: true,
    menuPosition: { x: 1, y: 2 },
    menuItems: [{ id: 'copy', label: 'Copiar' }],
    showMenu: showMenuMock,
    hideMenu: hideMenuMock,
  }),
  useMessageActions: () => ({
    copyMessage: copyMessageMock,
    speakMessage: speakMessageMock,
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  DeleteMessage: vi.fn(),
}));

vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: () => () => {},
}));

vi.mock('./ChatToolbar', () => ({
  ChatToolbar: ({ inputRef }: { inputRef?: React.RefObject<HTMLTextAreaElement> }) => (
    <div>Toolbar {inputRef ? 'ok' : 'no-ref'}</div>
  ),
}));

vi.mock('./MessageList', () => ({
  MessageList: ({ onContextMenu }: { onContextMenu?: (event: MouseEvent, message: { id: string; role: string }) => void }) => (
    <button
      type="button"
      onClick={() => onContextMenu?.(new MouseEvent('contextmenu'), { id: 'm1', role: 'user' })}
    >
      open-menu
    </button>
  ),
}));

vi.mock('./ChatInput', async () => {
  const React = await import('react');
  return {
    ChatInput: React.forwardRef<HTMLTextAreaElement, { onSend: (value: string) => void; disabled?: boolean }>(
      ({ onSend, disabled }, ref) => (
        <button
          ref={ref as React.RefObject<HTMLButtonElement>}
          type="button"
          disabled={disabled}
          onClick={() => onSend('oi')}
        >
          send
        </button>
      )
    ),
  };
});

vi.mock('../menu', () => ({
  ContextMenu: ({ visible, items }: { visible: boolean; items: Array<{ id: string; label: string }> }) => (
    <div>{visible ? items.map((item) => item.label).join(',') : 'closed'}</div>
  ),
}));

vi.mock('../ui/KeyboardShortcutsHelp', () => ({
  KeyboardShortcutsHelp: ({ isOpen }: { isOpen: boolean }) => <div>{isOpen ? 'help-open' : 'help-closed'}</div>,
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  announce: vi.fn(),
}));

vi.mock('../../utils/errorHandler', () => ({
  ErrorSeverity: { RECOVERABLE: 'recoverable' },
  ErrorMessages: { CHAT: { SEND_FAILED: 'Falha ao enviar', DELETE_FAILED: 'Falha ao deletar' } },
  handleError: vi.fn(),
}));

import { ChatSessionView } from './ChatSessionView';

describe('ChatSessionView', () => {
  beforeEach(() => {
    showMenuMock.mockReset();
    hideMenuMock.mockReset();
    chatStoreState.sessionsByConversationId[conversationId].isLoading = false;
  });

  it('embedded: aciona menu de contexto via MessageList', async () => {
    const onSend = vi.fn();
    render(<ChatSessionView variant="embedded" conversationId={conversationId} onSend={onSend} showShortcutsHelp={false} />);

    await userEvent.click(screen.getByRole('button', { name: 'open-menu' }));

    expect(showMenuMock).toHaveBeenCalled();
    expect(screen.getByText('Copiar')).toBeInTheDocument();
  });

  it('embedded: mostra banner de erro e retry quando onSend falha', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn().mockRejectedValueOnce(new Error('fail'));
    render(<ChatSessionView variant="embedded" conversationId={conversationId} onSend={onSend} showShortcutsHelp={false} />);

    await user.click(screen.getByRole('button', { name: 'send' }));

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Falha ao enviar');

    onSend.mockResolvedValueOnce(undefined);
    await user.click(screen.getByRole('button', { name: 'chat.retryAriaLabel' }));

    await waitFor(() => {
      expect(onSend).toHaveBeenCalledTimes(2);
    });
  });

  it('embedded: mantém envio habilitado mesmo com isLoading global ativo', async () => {
    const user = userEvent.setup();
    const onSend = vi.fn().mockResolvedValue(undefined);
    chatStoreState.sessionsByConversationId[conversationId].isLoading = true;
    render(
      <ChatSessionView
        variant="embedded"
        conversationId={conversationId}
        surfaceId="embedded:workspace-chat-modal:tab-1"
        onSend={onSend}
        showShortcutsHelp={false}
      />,
    );

    await user.click(screen.getByRole('button', { name: 'send' }));

    expect(onSend).toHaveBeenCalledWith('oi', undefined, expect.objectContaining({
      conversationId,
      sessionKey: `embedded:workspace-chat-modal:tab-1:${conversationId}`,
      surfaceId: 'embedded:workspace-chat-modal:tab-1',
      surfaceType: 'embedded',
    }));
  });
});
