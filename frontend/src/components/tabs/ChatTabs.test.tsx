import { describe, expect, it, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ChatTabs } from './ChatTabs';

const deleteTabSpy = vi.fn();

vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

vi.mock('@wailsjs/go/main/App', () => ({
  GetAvailableChannels: () => Promise.resolve([]),
}));

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: vi.fn() }),
}));

vi.mock('../../store/uiStore', () => ({
  useUIStore: () => ({ addToast: vi.fn() }),
}));

vi.mock('../../store/chatStore', () => ({
  useChatStore: () => ({
    tabs: [
      { id: '1', title: 'Aba 1', conversationId: 1 },
      { id: '2', title: 'Aba 2', conversationId: 2 },
    ],
    activeTabId: '1',
    isLoading: false,
    deleteTab: deleteTabSpy,
    setActiveTab: vi.fn(),
    updateTabTitle: vi.fn(),
    assignChannelToTab: vi.fn(),
    unassignChannelFromTab: vi.fn(),
  }),
}));

vi.mock('../menu', () => ({
  ContextMenu: () => <div data-testid="menu" />,
}));

vi.mock('../ui/tabs', () => ({
  Tabs: ({ children }: { children: React.ReactNode }) => <div>{children}</div>,
  TabList: ({ children, listRef, onKeyDown }: { children: React.ReactNode; listRef?: React.Ref<HTMLDivElement>; onKeyDown?: (e: React.KeyboardEvent<HTMLDivElement>) => void }) => (
    <div role="tablist" ref={listRef} onKeyDown={onKeyDown}>{children}</div>
  ),
  Tab: ({ children, value, onClick, onDoubleClick }: { children: React.ReactNode; value: string; onClick?: () => void; onDoubleClick?: () => void }) => (
    <button role="tab" data-tab-value={value} onClick={onClick} onDoubleClick={onDoubleClick}>{children}</button>
  ),
}));

describe('ChatTabs', () => {
  it('renderiza abas e permite fechar', () => {
    render(<ChatTabs />);

    expect(screen.getByRole('tab', { name: /Aba 1/i })).toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: 'chatTabs.close Aba 2' }));

    expect(deleteTabSpy).toHaveBeenCalledWith('2');
  });
});
