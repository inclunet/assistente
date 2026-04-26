import type { ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetChannelConfig = vi.fn();
const mockGetMessagingStatus = vi.fn();
const mockGetChannelTemplates = vi.fn();
const mockListCredentials = vi.fn();
const mockSaveChannelConfig = vi.fn();
const mockRestartChannel = vi.fn();
const mockDeleteCredential = vi.fn();
const mockAddToast = vi.fn();
const mockAnnounce = vi.fn();

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string, fallback?: string) =>
      ({
        'channels.tabs.channels': 'Canais',
        'channels.tabs.contacts': 'Contatos',
        'channels.actions.edit': 'Editar',
        'channels.actions.reconnectChannel': 'Reconectar',
        'channels.actions.removeContact': 'Remover',
        'channels.buttons.reload': 'Recarregar',
        'channels.status.disconnected': 'Desconectado',
      } as Record<string, string>)[key] ?? fallback ?? key,
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetChannelConfig: (name: string) => mockGetChannelConfig(name),
  SaveChannelConfig: (name: string, payload: unknown) => mockSaveChannelConfig(name, payload),
  GetMessagingStatus: () => mockGetMessagingStatus(),
  RestartChannel: (name: string) => mockRestartChannel(name),
  GetChannelTemplates: () => mockGetChannelTemplates(),
  ListCredentials: () => mockListCredentials(),
  DeleteCredential: (pattern: string) => mockDeleteCredential(pattern),
}));

vi.mock('@wailsjs/go/models', () => ({
  channels: {
    ChannelConfig: {
      createFrom: (data: unknown) => data,
    },
  },
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { addToast: mockAddToast };
    return selector ? selector(s) : s;
  },
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: mockAnnounce,
  }),
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    handleGridReady: vi.fn(),
  }),
}));

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => vi.fn(() => Promise.resolve(true)),
}));

vi.mock('../components/ui/Toolbar', async () => {
  const React = await import('react');
  return {
    Toolbar: ({
      left,
      right,
      actions,
    }: {
      left?: ReactNode;
      right?: ReactNode;
      actions?: Array<{ key: string; label: string; onClick?: () => void; disabled?: boolean }>;
    }) => (
      <div>
        {left}
        {right}
        {actions?.map((action) => (
          <button key={action.key} onClick={action.onClick} disabled={action.disabled}>
            {action.label}
          </button>
        ))}
      </div>
    ),
    ToolbarButton: React.forwardRef<HTMLButtonElement, { label: string; onClick?: () => void }>(
      ({ label, onClick }, ref) => (
        <button ref={ref} type="button" onClick={onClick}>{label}</button>
      )
    ),
  };
});

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({
    items,
    onFocusChange,
    getRowActions,
  }: {
    items?: Array<{ id: string; name: string; label: string }>;
    onFocusChange?: (item: { id: string; name: string; label: string } | null) => void;
    getRowActions?: (item: { id: string; name: string; label: string }) => Array<{ id: string; label?: string; onClick?: () => void }>;
  }) => (
    <div>
      <button type="button" onClick={() => onFocusChange?.(items?.[0] ?? null)}>
        focus-first
      </button>
      {items?.map((item) => (
        <div key={item.id}>
          <span>{item.label}</span>
          {getRowActions?.(item)?.map((action) => (
            <button key={action.id} type="button" onClick={action.onClick}>
              {action.label}
            </button>
          ))}
        </div>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/Modal', () => ({
  Modal: ({ isOpen, title, children }: { isOpen: boolean; title?: string; children?: ReactNode }) => (
    isOpen ? <div>{title}{children}</div> : null
  ),
  isModalOpen: () => false,
}));

vi.mock('../components/ui/EditorPanel', () => ({
  EditorPanelFields: ({ children, className }: { children?: ReactNode; className?: string }) => (
    <div className={className}>{children}</div>
  ),
  EditorPanelFooter: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
}));

vi.mock('../components/channels', () => ({
  ChannelsTelegramSection: () => <div>Telegram</div>,
  ChannelsSignalSection: () => <div>Signal</div>,
  ChannelsSlackSection: () => <div>Slack</div>,
}));

vi.mock('../components/menu', () => ({
  ContextMenu: () => null,
}));

vi.mock('../components/layout/MenuButton', () => ({
  MenuButton: () => <button type="button">Menu</button>,
}));

vi.mock('../components/modals/CreateChannelModal', () => ({
  default: () => null,
}));

import ChannelsPage from './ChannelsPage';

describe('ChannelsPage', () => {
  beforeEach(() => {
    mockGetChannelConfig.mockResolvedValue({
      enabled: false,
      bot_token: '',
      api_url: '',
      account: '',
      profile: '',
      max_history: 50,
      max_contacts: 1,
    });
    mockGetMessagingStatus.mockResolvedValue({});
    mockGetChannelTemplates.mockResolvedValue([
      { type: 'telegram', display_name: 'Telegram', icon: '📨' },
    ]);
    mockListCredentials.mockResolvedValue([]);
    mockSaveChannelConfig.mockResolvedValue(undefined);
    mockRestartChannel.mockResolvedValue(undefined);
    mockDeleteCredential.mockResolvedValue(undefined);
    mockAddToast.mockReset();
    mockAnnounce.mockReset();
  });

  it('reconecta canal via menu de acoes', async () => {
    const user = userEvent.setup();
    render(<ChannelsPage />);

    await waitFor(() => {
      expect(screen.getByText('Telegram')).toBeInTheDocument();
    });

    const reconnectButtons = screen.getAllByRole('button', { name: 'Reconectar' });
    const menuReconnect = reconnectButtons.find((button) => !button.hasAttribute('disabled'));
    expect(menuReconnect).toBeTruthy();
    await user.click(menuReconnect!);

    await waitFor(() => {
      expect(mockRestartChannel).toHaveBeenCalledWith('telegram');
    });
  });

  it('habilita acao da toolbar apos foco', async () => {
    const user = userEvent.setup();
    render(<ChannelsPage />);

    await waitFor(() => {
      expect(screen.getByText('Telegram')).toBeInTheDocument();
    });

    const reconnectButtons = screen.getAllByRole('button', { name: 'Reconectar' });
    const toolbarReconnect = reconnectButtons.find((button) => button.hasAttribute('disabled'));
    expect(toolbarReconnect).toBeTruthy();
    expect(toolbarReconnect).toBeDisabled();

    await user.click(screen.getByRole('button', { name: 'focus-first' }));
    await user.click(toolbarReconnect!);

    await waitFor(() => {
      expect(mockRestartChannel).toHaveBeenCalledWith('telegram');
    });
  });
});
