import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ChatToolbar } from './ChatToolbar';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
}));

vi.mock('../../store/chatStore', () => ({
  useChatStore: () => ({
    getActiveTab: () => ({ title: 'Conversa', conversationId: 1 }),
    clearActiveTab: vi.fn(),
    isLoading: false,
    loadConversationInActiveTab: vi.fn(),
  }),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: vi.fn() }),
}));

vi.mock('../../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({
    menu: { visible: false, x: 0, y: 0, items: [], ariaLabel: '' },
    openAtPoint: vi.fn(),
    closeMenu: vi.fn(),
    onSelectItem: vi.fn(),
  }),
}));

vi.mock('../pickers', () => ({
  HistoryPicker: () => <div data-testid="history-picker" />,
}));

vi.mock('../pickers/ProfilePicker', () => ({
  ProfilePicker: () => <div data-testid="profile-picker" />,
}));

vi.mock('../ui/Toolbar', () => ({
  Toolbar: ({ left, right }: { left: React.ReactNode; right: React.ReactNode }) => (
    <div>
      <div>{left}</div>
      <div>{right}</div>
    </div>
  ),
  ToolbarButton: ({ label, onClick }: { label: string; onClick?: () => void }) => (
    <button onClick={onClick}>{label}</button>
  ),
  ToolbarSeparator: () => <span data-testid="sep" />,
}));

vi.mock('../menu', () => ({
  Menu: () => <div data-testid="menu" />,
}));

vi.mock('./TokenStatsButton', () => ({
  TokenStatsButton: ({ onOpenModal }: { onOpenModal: () => void }) => (
    <button onClick={onOpenModal}>token</button>
  ),
}));

vi.mock('./TokenStatsModal', () => ({
  TokenStatsModal: () => <div data-testid="token-modal" />,
}));

describe('ChatToolbar', () => {
  it('aciona nova conversa', () => {
    const onNewConversation = vi.fn();

    render(<ChatToolbar onNewConversation={onNewConversation} />);

    fireEvent.click(screen.getByRole('button', { name: 'chat.newBtn' }));
    expect(onNewConversation).toHaveBeenCalled();
  });
});
