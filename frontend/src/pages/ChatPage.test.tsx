import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const sendMessageMock = vi.fn();
const updateMessageMock = vi.fn();
const showMenuMock = vi.fn();
const hideMenuMock = vi.fn();
const copyMessageMock = vi.fn();
const speakMessageMock = vi.fn();
const ensureWorkspaceTabHasConversationMock = vi.fn().mockResolvedValue('01926b90-7a5a-7c4e-8d3f-000000000001');
const conversationId = '01926b90-7a5a-7c4e-8d3f-000000000001';
const activeConversation = { id: conversationId, title: 'Conversa', threadedMessages: [] };
const workspaceStoreState = {
  getActiveTab: () => ({ id: 'chat-tab', type: 'chat' as const, conversationId, title: 'Conversa', position: 0 }),
};

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

vi.mock('../services/tts', () => ({
  ttsService: {
    isEnabled: () => false,
    hasVoiceConfig: () => false,
    on: vi.fn(),
    off: vi.fn(),
  },
}));

const chatStoreState = {
  sendMessageToConversation: sendMessageMock,
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

vi.mock('../store/chatStore', () => ({
  useChatStore: (selector?: (s: typeof chatStoreState) => unknown) => {
    if (typeof selector === 'function') {
      return selector(chatStoreState);
    }
    return chatStoreState;
  },
}));

vi.mock('../store/workspaceStore', () => ({
  useWorkspaceStore: (selector?: (s: typeof workspaceStoreState) => unknown) => {
    if (typeof selector === 'function') {
      return selector(workspaceStoreState);
    }
    return workspaceStoreState;
  },
  useActiveTab: () => workspaceStoreState.getActiveTab(),
}));

vi.mock('../lib/workspaceConversation', () => ({
  ensureWorkspaceTabHasConversation: (...args: unknown[]) => ensureWorkspaceTabHasConversationMock(...args),
}));

vi.mock('../store/editorStore', () => ({
  useEditorStore: {
    getState: () => ({
      requestInsert: vi.fn(),
      activeTabId: 'chat-tab',
      tabs: [{ id: 'chat-tab', conversationId: '01926b90-7a5a-7c4e-8d3f-00000000000a' }],
    }),
  },
}));

vi.mock('../hooks/useChatKeyboardNav', () => ({
  useChatKeyboardNav: () => {},
}));

vi.mock('../hooks/useTabsKeyboardShortcuts', () => ({
  useTabsKeyboardShortcuts: () => {},
}));

vi.mock('../hooks/useContextMenu', () => ({
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

vi.mock('../components/chat/ChatToolbar', () => ({
  ChatToolbar: ({ inputRef }: { inputRef?: React.RefObject<HTMLTextAreaElement> }) => (
    <div>Toolbar {inputRef ? 'ok' : 'no-ref'}</div>
  ),
}));

vi.mock('../components/chat/MessageList', () => ({
  MessageList: ({ onContextMenu }: { onContextMenu?: (event: MouseEvent, message: { id: string; role: string }) => void }) => (
    <button
      type="button"
      onClick={() => onContextMenu?.(new MouseEvent('contextmenu'), { id: 'm1', role: 'user' })}
    >
      open-menu
    </button>
  ),
}));

vi.mock('../components/chat/ChatInput', async () => {
  const React = await import('react');
  return {
    ChatInput: React.forwardRef<HTMLTextAreaElement, { onSend: (value: string) => void }>(
      ({ onSend }, ref) => (
        <button ref={ref as React.RefObject<HTMLButtonElement>} type="button" onClick={() => onSend('oi')}>
          send
        </button>
      )
    ),
  };
});

vi.mock('../components/menu', () => ({
  ContextMenu: ({ visible, items }: { visible: boolean; items: Array<{ id: string; label: string }> }) => (
    <div>{visible ? items.map((item) => item.label).join(',') : 'closed'}</div>
  ),
}));

vi.mock('../components/ui/KeyboardShortcutsHelp', () => ({
  KeyboardShortcutsHelp: ({ isOpen }: { isOpen: boolean }) => <div>{isOpen ? 'help-open' : 'help-closed'}</div>,
}));

vi.mock('../hooks/useAnnouncer', () => ({
  announce: vi.fn(),
}));

vi.mock('../utils/errorHandler', () => ({
  ErrorSeverity: { RECOVERABLE: 'recoverable' },
  ErrorMessages: { CHAT: { SEND_FAILED: 'Falha ao enviar', DELETE_FAILED: 'Falha ao deletar' } },
  handleError: vi.fn(),
}));

import ChatPage from './ChatPage';

describe('ChatPage', () => {
  beforeEach(() => {
    workspaceStoreState.getActiveTab = () => ({
      id: 'chat-tab',
      type: 'chat' as const,
      conversationId: '01926b90-7a5a-7c4e-8d3f-000000000001',
      title: 'Conversa',
      position: 0,
    });
    sendMessageMock.mockReset();
    showMenuMock.mockReset();
    hideMenuMock.mockReset();
    ensureWorkspaceTabHasConversationMock.mockClear();
    sendMessageMock.mockRejectedValueOnce(new Error('erro'));
  });

  it('abre menu de contexto ao acionar MessageList', async () => {
    render(<ChatPage />);
    await userEvent.click(screen.getByRole('button', { name: 'open-menu' }));

    expect(showMenuMock).toHaveBeenCalled();
    expect(screen.getByText('Copiar')).toBeInTheDocument();
  });

  it('garante a conversa da aba antes de enviar quando ainda não existe conversationId', async () => {
    const user = userEvent.setup();
    workspaceStoreState.getActiveTab = () => ({
      id: 'chat-tab',
      type: 'chat' as const,
      conversationId: '',
      title: 'Conversa',
      position: 0,
    });
    ensureWorkspaceTabHasConversationMock.mockResolvedValue('01926b90-7a5a-7c4e-8d3f-00000000002a');
    sendMessageMock.mockResolvedValueOnce(undefined);

    render(<ChatPage />);

    await user.click(screen.getByRole('button', { name: 'send' }));

    await waitFor(() => {
      expect(ensureWorkspaceTabHasConversationMock).toHaveBeenCalled();
      expect(sendMessageMock).toHaveBeenCalledWith(
        '01926b90-7a5a-7c4e-8d3f-00000000002a',
        'oi',
        undefined,
        undefined,
        {
          origin: expect.objectContaining({
            conversationId: '01926b90-7a5a-7c4e-8d3f-00000000002a',
            sessionKey: 'page:tab:chat-tab:01926b90-7a5a-7c4e-8d3f-00000000002a',
            surfaceId: 'page:tab:chat-tab',
            surfaceType: 'page',
            tabId: 'chat-tab',
          }),
        },
      );
    });
  });

  it('mostra banner de erro e permite retry', async () => {
    const user = userEvent.setup();
    render(<ChatPage />);

    await user.click(screen.getByRole('button', { name: 'send' }));

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('Falha ao enviar');

    sendMessageMock.mockResolvedValueOnce(undefined);
    // Mock i18n returns translation keys as literal strings
    await user.click(screen.getByRole('button', { name: 'chat.retryAriaLabel' }));

    await waitFor(() => {
      expect(sendMessageMock).toHaveBeenCalledTimes(2);
    });
  });
});
