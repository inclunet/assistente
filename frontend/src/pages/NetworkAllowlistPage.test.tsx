import type { ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetNetworkAllowlist = vi.fn();
const mockRemoveNetworkAllowlistEntry = vi.fn();
const mockConfirm = vi.fn();
const mockAnnounce = vi.fn();
const mockAddToast = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  GetNetworkAllowlist: (...args: unknown[]) => mockGetNetworkAllowlist(...args),
  RemoveNetworkAllowlistEntry: (...args: unknown[]) => mockRemoveNetworkAllowlistEntry(...args),
}));

vi.mock('react-i18next', () => {
  const t = (key: string, values?: Record<string, unknown>) => {
    if (!values) return key;
    const parts = Object.entries(values).map(([name, value]) => `${name}=${String(value)}`);
    return `${key}(${parts.join(',')})`;
  };
  return { useTranslation: () => ({ t, i18n: { language: 'pt-BR' } }) };
});

vi.mock('@ant-design/icons', () => ({
  DeleteOutlined: () => <span data-testid="delete-icon" />,
  ReloadOutlined: () => <span data-testid="reload-icon" />,
}));

vi.mock('../hooks/useConfirm', () => ({ useConfirm: () => mockConfirm }));
vi.mock('../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: mockAnnounce }) }));
vi.mock('../hooks/useGridFocus', () => ({ useGridFocus: () => ({ handleGridReady: vi.fn() }) }));
vi.mock('../hooks/useGridPageLandmarks', () => ({ useGridPageLandmarks: vi.fn() }));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: typeof mockAddToast }) => unknown) =>
    selector({ addToast: mockAddToast }),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, actions = [] }: {
    left?: ReactNode;
    actions?: Array<{ key: string; label: string; onClick: () => void; disabled?: boolean }>;
  }) => (
    <div role="toolbar" aria-label="networkAllowlist.toolbarLabel">
      {left}
      {actions.map((action) => (
        <button key={action.key} type="button" onClick={action.onClick} disabled={action.disabled}>
          {`toolbar:${action.label}`}
        </button>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/PageLoading', () => ({
  PageLoading: ({ message }: { message: string }) => <div>{message}</div>,
}));

vi.mock('../components/layout/MenuButton', () => ({
  MenuButton: ({ items }: { items: Array<{ id: string; label: string; onClick: () => void }> }) => (
    <>
      {items.map((item) => (
        <button key={item.id} type="button" onClick={item.onClick}>{item.label}</button>
      ))}
    </>
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({ items, columns, onFocusChange }: {
    items: Array<Record<string, unknown>>;
    columns: Array<{ key: string; format?: (value: unknown, item: Record<string, unknown>) => ReactNode }>;
    onFocusChange?: (item: Record<string, unknown> | null) => void;
  }) => (
    <div role="grid">
      {items.map((item) => (
        <div key={String(item.id)} role="row">
          {columns.map((column) => (
            <span key={column.key}>
              {column.format ? column.format(item[column.key], item) : String(item[column.key] ?? '')}
            </span>
          ))}
          <button type="button" onClick={() => onFocusChange?.(item)}>
            {`focar:${String(item.id)}`}
          </button>
        </div>
      ))}
    </div>
  ),
}));

import NetworkAllowlistPage from './NetworkAllowlistPage';

const hostDeWorkflows = {
  host: 'api.nu.workflows.dev',
  port: '',
  scope: 'workspace',
  category: 'cgnat',
  resolvedIps: ['100.64.1.112'],
  createdBy: 'skill:workflows-api',
  createdAt: '2026-08-01T10:00:00Z',
  reason: 'API interna de workflows',
};

describe('NetworkAllowlistPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConfirm.mockResolvedValue(true);
    mockGetNetworkAllowlist.mockResolvedValue([hostDeWorkflows]);
    mockRemoveNetworkAllowlistEntry.mockResolvedValue(undefined);
  });

  it('mostra o host autorizado com escopo, categoria e quem autorizou', async () => {
    render(<NetworkAllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('api.nu.workflows.dev')).toBeInTheDocument();
    });
    expect(screen.getByText('networkAllowlist.scope.workspace')).toBeInTheDocument();
    expect(screen.getByText('cgnat')).toBeInTheDocument();
    expect(screen.getByText('skill:workflows-api')).toBeInTheDocument();
    expect(screen.getByText('100.64.1.112')).toBeInTheDocument();
  });

  it('diz que a entrada sem porta vale só para as portas default', async () => {
    // Célula vazia seria lida como "qualquer porta", e a autorização por host
    // cobre apenas 80/443 (AEP-0082).
    render(<NetworkAllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('networkAllowlist.defaultPorts')).toBeInTheDocument();
    });
  });

  it('mostra a porta quando a autorização foi daquela porta', async () => {
    mockGetNetworkAllowlist.mockResolvedValue([{ ...hostDeWorkflows, port: '8443' }]);

    render(<NetworkAllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('8443')).toBeInTheDocument();
    });
  });

  it('nomeia o escopo que a interface não conhece sem mostrar código cru', async () => {
    mockGetNetworkAllowlist.mockResolvedValue([{ ...hostDeWorkflows, scope: 'universo' }]);

    render(<NetworkAllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('networkAllowlist.scope.unknown')).toBeInTheDocument();
    });
  });

  it('remove depois de confirmar, anuncia e recarrega a lista', async () => {
    const user = userEvent.setup();
    mockGetNetworkAllowlist.mockResolvedValueOnce([hostDeWorkflows]).mockResolvedValueOnce([]);
    render(<NetworkAllowlistPage />);
    await waitFor(() => expect(screen.getByText('api.nu.workflows.dev')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'networkAllowlist.actions.remove' }));

    await waitFor(() => {
      expect(mockRemoveNetworkAllowlistEntry).toHaveBeenCalledWith(
        'workspace',
        'api.nu.workflows.dev',
        '',
      );
    });
    expect(mockConfirm).toHaveBeenCalled();
    // O toast não anuncia sozinho: quem anuncia é a frase que diz o que saiu.
    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringContaining('networkAllowlist.announce.removed'),
    );
    await waitFor(() => {
      expect(screen.getByText('networkAllowlist.empty')).toBeInTheDocument();
    });
  });

  it('não remove nada quando a pessoa desiste da confirmação', async () => {
    const user = userEvent.setup();
    mockConfirm.mockResolvedValue(false);
    render(<NetworkAllowlistPage />);
    await waitFor(() => expect(screen.getByText('api.nu.workflows.dev')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'networkAllowlist.actions.remove' }));

    expect(mockRemoveNetworkAllowlistEntry).not.toHaveBeenCalled();
  });

  it('mostra erro i18n quando a remoção falha', async () => {
    // Dizer "removida" sem ter removido faria a pessoa acreditar que fechou um
    // acesso que continua aberto. A mensagem vem da chave i18n, não do backend.
    const user = userEvent.setup();
    mockRemoveNetworkAllowlistEntry.mockRejectedValue(
      new Error('entrada de allowlist de rede não encontrada'),
    );
    render(<NetworkAllowlistPage />);
    await waitFor(() => expect(screen.getByText('api.nu.workflows.dev')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'networkAllowlist.actions.remove' }));

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith('networkAllowlist.error.removeFailed', 'error');
    });
  });

  it('após remover, falha de sincronização não contradiz o sucesso', async () => {
    const user = userEvent.setup();
    mockGetNetworkAllowlist
      .mockResolvedValueOnce([hostDeWorkflows])
      .mockRejectedValueOnce(new Error('rede caiu'));
    render(<NetworkAllowlistPage />);
    await waitFor(() => expect(screen.getByText('api.nu.workflows.dev')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'networkAllowlist.actions.remove' }));

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith('networkAllowlist.toast.removed', 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
    });
    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith(
        'networkAllowlist.error.reloadAfterRemoveFailed',
        'warning',
      );
    });
    expect(screen.queryByText('networkAllowlist.loadFailedBody')).not.toBeInTheDocument();
    expect(screen.getByText('networkAllowlist.empty')).toBeInTheDocument();
  });

  it('diz que tudo segue bloqueado quando não há host autorizado', async () => {
    mockGetNetworkAllowlist.mockResolvedValue([]);

    render(<NetworkAllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('networkAllowlist.empty')).toBeInTheDocument();
    });
  });

  it('falha de carga não diz que não há host autorizado', async () => {
    // As autorizações que existirem continuam valendo; quem acreditasse na
    // lista vazia não iria procurá-las.
    mockGetNetworkAllowlist.mockRejectedValue(new Error('sem acesso'));

    render(<NetworkAllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('networkAllowlist.loadFailedBody')).toBeInTheDocument();
    });
    expect(screen.queryByText('networkAllowlist.empty')).not.toBeInTheDocument();
    expect(mockAddToast).toHaveBeenCalledWith('networkAllowlist.error.loadFailed', 'error');
  });

  it('avisa que a lista não cobre escopos efêmeros nem outros perfis', async () => {
    // Sem esse aviso, a lista pareceria a relação completa do que está liberado.
    render(<NetworkAllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('networkAllowlist.sessionNote')).toBeInTheDocument();
    });
  });

  it('a barra deixa de oferecer remover quando a linha sai da lista', async () => {
    const user = userEvent.setup();
    mockGetNetworkAllowlist.mockResolvedValueOnce([hostDeWorkflows]).mockResolvedValueOnce([]);
    render(<NetworkAllowlistPage />);
    await waitFor(() => expect(screen.getByText('api.nu.workflows.dev')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'focar:workspace:api.nu.workflows.dev:' }));
    const naBarra = screen.getByRole('button', { name: 'toolbar:networkAllowlist.actions.remove' });
    expect(naBarra).toBeEnabled();

    await user.click(naBarra);

    await waitFor(() => {
      expect(screen.getByText('networkAllowlist.empty')).toBeInTheDocument();
    });
    expect(
      screen.getByRole('button', { name: 'toolbar:networkAllowlist.actions.remove' }),
    ).toBeDisabled();
  });
});
