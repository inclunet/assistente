import type { ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetAllChannelConfigs = vi.fn();
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
    t: (key: string, opts?: string | Record<string, unknown>) => {
      const map: Record<string, string> = {
        'channels.tabs.channels': 'Canais',
        'channels.tabs.contacts': 'Contatos',
        'channels.actions.edit': 'Editar',
        'channels.actions.reconnectChannel': 'Reconectar',
        'channels.actions.removeContact': 'Remover',
        'channels.buttons.reload': 'Recarregar',
        'channels.buttons.new': 'Novo',
        'channels.status.disconnected': 'Desconectado',
        'channels.empty.noChannels': 'Nenhum canal configurado. Use Novo para adicionar Telegram, Signal ou Slack.',
        'channels.aria.newChannel': 'Novo canal',
        'channels.aria.createChannel': 'Criar canal',
        'channels.aria.createMenu': 'Menu de criação',
        'channels.aria.toolbar': 'Toolbar',
        'channels.aria.gridLabel': 'Canais',
        'channels.announce.editorOpened': 'Editor aberto',
        'channels.announce.editorClosed': 'Editor fechado',
        'channels.title': 'Canais de Comunicação',
        'channels.modal.editorTitle': 'Editor de canal',
      };
      if (key === 'channels.modal.editorTitle' && opts && typeof opts === 'object' && 'title' in opts) {
        return `Editor de canal: ${String(opts.title)}`;
      }
      if (typeof opts === 'string') {
        return map[key] ?? opts;
      }
      return map[key] ?? key;
    },
  }),
}));

vi.mock('@wailsjs/go/wailsapi/Messaging', () => ({
  GetAllChannelConfigs: () => mockGetAllChannelConfigs(),
  SaveChannelConfig: (name: string, payload: unknown) => mockSaveChannelConfig(name, payload),
  GetMessagingStatus: () => mockGetMessagingStatus(),
  RestartChannel: (name: string) => mockRestartChannel(name),
  GetChannelTemplates: () => mockGetChannelTemplates(),
}));

vi.mock('@wailsjs/go/wailsapi/Credentials', () => ({
  ListCredentials: () => mockListCredentials(),
  DeleteCredential: (pattern: string) => mockDeleteCredential(pattern),
  UpsertCredential: vi.fn(),
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
    announceRequest: vi.fn(() => true),
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

vi.mock('../hooks/useGridPageLandmarks', () => ({
  useGridPageLandmarks: () => {},
}));

vi.mock('../hooks/useResourceEditRequest', () => ({
  useResourceEditRequest: () => {},
}));

vi.mock('../hooks/useSignalChannelController', () => ({
  useSignalChannelController: () => ({
    signalRegStep: 'idle',
    signalRegCode: '',
    signalRegCaptcha: '',
    signalRegError: '',
    signalSmsSent: false,
    signalCheckingAPI: false,
    signalAPIInfo: null,
    signalAPIReady: false,
    signalAccounts: [],
    signalConnectionMode: 'register',
    signalLinkQR: '',
    signalLinking: false,
    signalUnregistering: false,
    setSignalRegStep: vi.fn(),
    setSignalRegCode: vi.fn(),
    setSignalRegCaptcha: vi.fn(),
    setSignalRegError: vi.fn(),
    setSignalSmsSent: vi.fn(),
    setSignalAPIInfo: vi.fn(),
    setSignalAPIReady: vi.fn(),
    setSignalAccounts: vi.fn(),
    setSignalConnectionMode: vi.fn(),
    setSignalLinkQR: vi.fn(),
    setSignalLinking: vi.fn(),
    stopLinkPolling: vi.fn(),
    handleSignalCheckAPI: vi.fn(),
    handleSignalRegister: vi.fn(),
    handleSignalVerify: vi.fn(),
    handleSignalLink: vi.fn(),
    handleSignalUnregister: vi.fn(),
  }),
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

vi.mock('../components', () => ({
  Button: ({ children, onClick }: { children?: ReactNode; onClick?: () => void }) => (
    <button type="button" onClick={onClick}>{children}</button>
  ),
  PageLoading: ({ message }: { message?: string }) => <div>{message}</div>,
}));

vi.mock('../components/channels', () => ({
  ChannelsTelegramSection: () => <div>TelegramForm</div>,
  ChannelsSignalSection: () => <div>SignalForm</div>,
  ChannelsSlackSection: () => <div>SlackForm</div>,
}));

vi.mock('../components/menu', () => ({
  ContextMenu: ({
    visible,
    items,
  }: {
    visible?: boolean;
    items?: Array<{ id: string; label: string; action?: () => void }>;
  }) => (
    visible ? (
      <div role="menu">
        {items?.map((item) => (
          <button key={item.id} type="button" role="menuitem" onClick={item.action}>
            {item.label}
          </button>
        ))}
      </div>
    ) : null
  ),
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
    mockGetAllChannelConfigs.mockReset();
    mockGetAllChannelConfigs.mockResolvedValue({
      telegram: {
        enabled: false,
        bot_token: '',
        display_name: 'Telegram',
        max_history: 50,
        max_contacts: 1,
      },
    });
    mockGetMessagingStatus.mockResolvedValue({});
    mockGetChannelTemplates.mockResolvedValue([
      { type: 'telegram', display_name: 'Telegram Bot', icon: '📨', supported: true },
      { type: 'signal', display_name: 'Signal', icon: '📡', supported: true },
    ]);
    mockListCredentials.mockResolvedValue([]);
    mockSaveChannelConfig.mockResolvedValue(undefined);
    mockRestartChannel.mockResolvedValue(undefined);
    mockDeleteCredential.mockResolvedValue(undefined);
    mockAddToast.mockReset();
    mockAnnounce.mockReset();
  });

  it('mostra grid vazio quando não há canais no DB', async () => {
    mockGetAllChannelConfigs.mockResolvedValue({});
    render(<ChannelsPage />);

    await waitFor(() => {
      expect(screen.getByText(/Nenhum canal configurado/i)).toBeInTheDocument();
    });
    expect(screen.queryByText('Telegram')).not.toBeInTheDocument();
    expect(screen.queryByText('Signal')).not.toBeInTheDocument();
  });

  it('cria canal via Novo e abre o editor', async () => {
    const user = userEvent.setup();
    mockGetAllChannelConfigs
      .mockResolvedValueOnce({})
      .mockResolvedValueOnce({})
      .mockResolvedValue({
        telegram: {
          enabled: false,
          display_name: 'Telegram Bot',
          max_history: 50,
          max_contacts: 1,
        },
      });

    render(<ChannelsPage />);

    await waitFor(() => {
      expect(screen.getByText(/Nenhum canal configurado/i)).toBeInTheDocument();
    });

    await user.click(screen.getByRole('button', { name: 'Novo' }));
    await user.click(screen.getByRole('menuitem', { name: 'Telegram Bot' }));

    await waitFor(() => {
      expect(mockSaveChannelConfig).toHaveBeenCalledWith(
        'telegram',
        expect.objectContaining({ type: 'telegram' }),
      );
    });

    await waitFor(() => {
      expect(screen.getByText('TelegramForm')).toBeInTheDocument();
    });
  });

  it('reconecta canal via menu de acoes', async () => {
    const user = userEvent.setup();
    mockGetAllChannelConfigs.mockReset();
    mockGetAllChannelConfigs.mockResolvedValue({
      telegram: {
        enabled: false,
        bot_token: '',
        display_name: 'Telegram',
        max_history: 50,
        max_contacts: 1,
      },
    });
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
