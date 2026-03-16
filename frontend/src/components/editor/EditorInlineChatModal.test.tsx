import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { EditorInlineChatModal } from './EditorInlineChatModal';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('../ui/Modal', () => ({
  Modal: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
}));

vi.mock('../chat/MessageList', () => ({
  MessageList: () => <div data-testid="message-list" />,
}));

vi.mock('../chat/ChatInput', () => ({
  ChatInput: ({ onSend }: { onSend: (msg: string) => void }) => (
    <button onClick={() => onSend('Oi')}>Enviar</button>
  ),
}));

vi.mock('../menu', () => ({
  ContextMenu: () => <div data-testid="context-menu" />,
}));

vi.mock('../ui/Toolbar', () => ({
  Toolbar: ({ left }: { left: React.ReactNode }) => <div>{left}</div>,
}));

vi.mock('../../hooks/useContextMenu', () => ({
  useContextMenu: () => ({
    showMenu: vi.fn(),
    hideMenu: vi.fn(),
    menuItems: [],
    menuPosition: { x: 0, y: 0 },
    menuVisible: false,
  }),
  useMessageActions: () => ({
    copyMessage: vi.fn(),
    speakMessage: vi.fn(),
  }),
}));

vi.mock('../../store/chatStore', () => ({
  useChatStore: () => ({
    isLoading: false,
    getThreadedMessages: () => [],
    loadMessageChildren: vi.fn(),
    getActiveTab: () => ({ title: 'Conversa', conversationId: 1 }),
    loadConversationInActiveTab: vi.fn(),
  }),
}));

describe('EditorInlineChatModal', () => {
  it('renderiza erro e envia mensagem', () => {
    const onSend = vi.fn().mockResolvedValue(undefined);

    render(
      <EditorInlineChatModal
        isOpen={true}
        selectedText="texto"
        error="Falha"
        onClose={() => {}}
        onSend={onSend}
      />
    );

    expect(screen.getByText('Falha')).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'Enviar' }));
    expect(onSend).toHaveBeenCalledWith('Oi', undefined);
  });
});
