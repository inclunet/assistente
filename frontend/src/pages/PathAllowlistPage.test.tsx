import type { ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetPathAllowlist = vi.fn();
const mockRemovePathAllowlistEntry = vi.fn();
const mockConfirm = vi.fn();
const mockAnnounce = vi.fn();
const mockAddToast = vi.fn();

vi.mock('@wailsjs/go/wailsapi/FSTrust', () => ({
  GetPathAllowlist: (...args: unknown[]) => mockGetPathAllowlist(...args),
  RemovePathAllowlistEntry: (...args: unknown[]) => mockRemovePathAllowlistEntry(...args),
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
    <div role="toolbar" aria-label="pathAllowlist.toolbarLabel">
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

import PathAllowlistPage from './PathAllowlistPage';

const entradaDeDocs = {
  path: '/tmp/projeto/docs/readme.md',
  kind: 'file',
  operation: 'read',
  scope: 'workspace',
  createdBy: 'user',
  createdAt: '2026-08-17T12:00:00Z',
  reason: 'ler docs fora do sandbox',
};

describe('PathAllowlistPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConfirm.mockResolvedValue(true);
    mockGetPathAllowlist.mockResolvedValue([entradaDeDocs]);
    mockRemovePathAllowlistEntry.mockResolvedValue(undefined);
  });

  it('mostra o path autorizado com tipo, operação e escopo', async () => {
    render(<PathAllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('/tmp/projeto/docs/readme.md')).toBeInTheDocument();
    });
    expect(screen.getByText('pathAllowlist.kind.file')).toBeInTheDocument();
    expect(screen.getByText('read')).toBeInTheDocument();
    expect(screen.getByText('pathAllowlist.scope.workspace')).toBeInTheDocument();
    expect(screen.getByText('user')).toBeInTheDocument();
  });

  it('nomeia o escopo e o tipo que a interface não conhece sem mostrar código cru', async () => {
    mockGetPathAllowlist.mockResolvedValue([{ ...entradaDeDocs, scope: 'universo', kind: 'portal' }]);

    render(<PathAllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('pathAllowlist.scope.unknown')).toBeInTheDocument();
    });
    expect(screen.getByText('pathAllowlist.kind.unknown')).toBeInTheDocument();
  });

  it('remove depois de confirmar, anuncia e recarrega a lista', async () => {
    const user = userEvent.setup();
    mockGetPathAllowlist.mockResolvedValueOnce([entradaDeDocs]).mockResolvedValueOnce([]);
    render(<PathAllowlistPage />);
    await waitFor(() => expect(screen.getByText('/tmp/projeto/docs/readme.md')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'pathAllowlist.actions.remove' }));

    await waitFor(() => {
      expect(mockRemovePathAllowlistEntry).toHaveBeenCalledWith(
        'workspace',
        '/tmp/projeto/docs/readme.md',
        'file',
        'read',
      );
    });
    expect(mockConfirm).toHaveBeenCalled();
    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringContaining('pathAllowlist.announce.removed'),
    );
    await waitFor(() => {
      expect(screen.getByText('pathAllowlist.empty')).toBeInTheDocument();
    });
  });

  it('não remove nada quando a pessoa desiste da confirmação', async () => {
    const user = userEvent.setup();
    mockConfirm.mockResolvedValue(false);
    render(<PathAllowlistPage />);
    await waitFor(() => expect(screen.getByText('/tmp/projeto/docs/readme.md')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'pathAllowlist.actions.remove' }));

    expect(mockRemovePathAllowlistEntry).not.toHaveBeenCalled();
  });

  it('mostra erro i18n quando a remoção falha', async () => {
    const user = userEvent.setup();
    mockRemovePathAllowlistEntry.mockRejectedValue(
      new Error('entrada de allowlist de path não encontrada'),
    );
    render(<PathAllowlistPage />);
    await waitFor(() => expect(screen.getByText('/tmp/projeto/docs/readme.md')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'pathAllowlist.actions.remove' }));

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith('pathAllowlist.error.removeFailed', 'error');
    });
  });

  it('após remover, falha de sincronização não contradiz o sucesso', async () => {
    const user = userEvent.setup();
    mockGetPathAllowlist
      .mockResolvedValueOnce([entradaDeDocs])
      .mockRejectedValueOnce(new Error('rede caiu'));
    render(<PathAllowlistPage />);
    await waitFor(() => expect(screen.getByText('/tmp/projeto/docs/readme.md')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'pathAllowlist.actions.remove' }));

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith('pathAllowlist.toast.removed', 'success', undefined, undefined, {
        suppressAnnounce: true,
      });
    });
    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith(
        'pathAllowlist.error.reloadAfterRemoveFailed',
        'warning',
      );
    });
    expect(screen.queryByText('pathAllowlist.loadFailedBody')).not.toBeInTheDocument();
    expect(screen.getByText('pathAllowlist.empty')).toBeInTheDocument();
  });

  it('diz que continua pedindo consentimento quando não há path autorizado', async () => {
    mockGetPathAllowlist.mockResolvedValue([]);

    render(<PathAllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('pathAllowlist.empty')).toBeInTheDocument();
    });
  });

  it('falha de carga não diz que não há path autorizado', async () => {
    mockGetPathAllowlist.mockRejectedValue(new Error('sem acesso'));

    render(<PathAllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('pathAllowlist.loadFailedBody')).toBeInTheDocument();
    });
    expect(screen.queryByText('pathAllowlist.empty')).not.toBeInTheDocument();
    expect(mockAddToast).toHaveBeenCalledWith('pathAllowlist.error.loadFailed', 'error');
  });

  it('avisa que a lista não cobre escopos efêmeros nem outros perfis', async () => {
    render(<PathAllowlistPage />);

    await waitFor(() => {
      expect(screen.getByText('pathAllowlist.sessionNote')).toBeInTheDocument();
    });
  });

  it('a barra deixa de oferecer remover quando a linha sai da lista', async () => {
    const user = userEvent.setup();
    mockGetPathAllowlist.mockResolvedValueOnce([entradaDeDocs]).mockResolvedValueOnce([]);
    render(<PathAllowlistPage />);
    await waitFor(() => expect(screen.getByText('/tmp/projeto/docs/readme.md')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'focar:workspace:file:read:/tmp/projeto/docs/readme.md' }));
    const naBarra = screen.getByRole('button', { name: 'toolbar:pathAllowlist.actions.remove' });
    expect(naBarra).toBeEnabled();

    await user.click(naBarra);

    await waitFor(() => {
      expect(screen.getByText('pathAllowlist.empty')).toBeInTheDocument();
    });
    expect(
      screen.getByRole('button', { name: 'toolbar:pathAllowlist.actions.remove' }),
    ).toBeDisabled();
  });
});
