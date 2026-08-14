import type { ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetAgentPermissions = vi.fn();
const mockRevokeAgentPermission = vi.fn();
const mockConfirm = vi.fn();
const mockAnnounce = vi.fn();
const mockAddToast = vi.fn();

vi.mock('@wailsjs/go/wailsapi/ACPTrust', () => ({
  GetAgentPermissions: (...args: unknown[]) => mockGetAgentPermissions(...args),
  RevokeAgentPermission: (...args: unknown[]) => mockRevokeAgentPermission(...args),
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
    <div role="toolbar" aria-label="agentPermissions.toolbarLabel">
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

import AgentPermissionsPage from './AgentPermissionsPage';

const autorizacaoDeExecutar = {
  profileSlug: 'cursor',
  profileName: 'Cursor',
  action: 'execute',
  grantedAt: '2026-07-30T12:00:00Z',
};

describe('AgentPermissionsPage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockConfirm.mockResolvedValue(true);
    mockGetAgentPermissions.mockResolvedValue([autorizacaoDeExecutar]);
    mockRevokeAgentPermission.mockResolvedValue(undefined);
  });

  it('mostra o que cada perfil autorizou', async () => {
    render(<AgentPermissionsPage />);

    await waitFor(() => {
      expect(screen.getByText('Cursor')).toBeInTheDocument();
    });
    expect(screen.getByText('agentPermissions.action.execute')).toBeInTheDocument();
  });

  it('mostra o slug quando o perfil já não existe', async () => {
    // A autorização sobrevive ao perfil e voltaria a valer se ele fosse
    // recriado: esconder a linha deixaria a pessoa sem como revogá-la.
    mockGetAgentPermissions.mockResolvedValue([
      { ...autorizacaoDeExecutar, profileName: '', profileSlug: 'perfil-antigo' },
    ]);

    render(<AgentPermissionsPage />);

    await waitFor(() => {
      expect(screen.getByText('perfil-antigo')).toBeInTheDocument();
    });
  });

  it('nomeia a classe que a interface não conhece sem mostrar código cru', async () => {
    mockGetAgentPermissions.mockResolvedValue([{ ...autorizacaoDeExecutar, action: 'faz-tudo' }]);

    render(<AgentPermissionsPage />);

    await waitFor(() => {
      expect(screen.getByText('agentPermissions.action.unknown')).toBeInTheDocument();
    });
  });

  it('revoga depois de confirmar, anuncia e recarrega a lista', async () => {
    const user = userEvent.setup();
    mockGetAgentPermissions.mockResolvedValueOnce([autorizacaoDeExecutar]).mockResolvedValueOnce([]);
    render(<AgentPermissionsPage />);
    await waitFor(() => expect(screen.getByText('Cursor')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'agentPermissions.actions.revoke' }));

    await waitFor(() => {
      expect(mockRevokeAgentPermission).toHaveBeenCalledWith('cursor', 'execute');
    });
    expect(mockConfirm).toHaveBeenCalled();
    // O toast não anuncia sozinho: quem anuncia é a frase que diz o que saiu.
    expect(mockAnnounce).toHaveBeenCalledWith(
      expect.stringContaining('agentPermissions.announce.revoked'),
    );
    await waitFor(() => {
      expect(screen.getByText('agentPermissions.empty')).toBeInTheDocument();
    });
  });

  it('não revoga nada quando a pessoa desiste da confirmação', async () => {
    const user = userEvent.setup();
    mockConfirm.mockResolvedValue(false);
    render(<AgentPermissionsPage />);
    await waitFor(() => expect(screen.getByText('Cursor')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'agentPermissions.actions.revoke' }));

    expect(mockRevokeAgentPermission).not.toHaveBeenCalled();
  });

  it('traduz o código de autorização inexistente em vez de exibir o texto do backend', async () => {
    // Dizer "revogado" sem ter revogado faria a pessoa acreditar que fechou
    // uma porta que continua aberta.
    const user = userEvent.setup();
    mockRevokeAgentPermission.mockRejectedValue(new Error('agent_permission_not_found'));
    render(<AgentPermissionsPage />);
    await waitFor(() => expect(screen.getByText('Cursor')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'agentPermissions.actions.revoke' }));

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith('agentPermissions.error.notFound', 'error');
    });
  });

  it('não joga na tela a mensagem crua de um erro qualquer da revogação', async () => {
    const user = userEvent.setup();
    mockRevokeAgentPermission.mockRejectedValue(new Error('open trust.json: permission denied'));
    render(<AgentPermissionsPage />);
    await waitFor(() => expect(screen.getByText('Cursor')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'agentPermissions.actions.revoke' }));

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith('agentPermissions.error.revokeFailed', 'error');
    });
  });

  it('diz que o agente pergunta antes de tudo quando não há autorização', async () => {
    mockGetAgentPermissions.mockResolvedValue([]);

    render(<AgentPermissionsPage />);

    await waitFor(() => {
      expect(screen.getByText('agentPermissions.empty')).toBeInTheDocument();
    });
  });

  it('avisa quando não conseguiu carregar as autorizações', async () => {
    mockGetAgentPermissions.mockRejectedValue(new Error('sem acesso'));

    render(<AgentPermissionsPage />);

    await waitFor(() => {
      expect(mockAddToast).toHaveBeenCalledWith('agentPermissions.error.loadFailed', 'error');
    });
  });

  it('falha de carga não diz que não há autorização nenhuma', async () => {
    // As que existirem continuam valendo; quem acreditasse na lista vazia não
    // iria procurá-las.
    mockGetAgentPermissions.mockRejectedValue(new Error('sem acesso'));

    render(<AgentPermissionsPage />);

    await waitFor(() => {
      expect(screen.getByText('agentPermissions.loadFailedBody')).toBeInTheDocument();
    });
    expect(screen.queryByText('agentPermissions.empty')).not.toBeInTheDocument();
  });

  it('a barra deixa de oferecer revogar quando a linha sai da lista', async () => {
    // Sem isso, o botão continuaria habilitado sobre uma autorização que já não
    // existe, e quem navega por teclado só descobriria pelo erro.
    const user = userEvent.setup();
    mockGetAgentPermissions.mockResolvedValueOnce([autorizacaoDeExecutar]).mockResolvedValueOnce([]);
    render(<AgentPermissionsPage />);
    await waitFor(() => expect(screen.getByText('Cursor')).toBeInTheDocument());

    await user.click(screen.getByRole('button', { name: 'focar:cursor:execute' }));
    const naBarra = screen.getByRole('button', { name: 'toolbar:agentPermissions.actions.revoke' });
    expect(naBarra).toBeEnabled();

    await user.click(naBarra);

    await waitFor(() => {
      expect(screen.getByText('agentPermissions.empty')).toBeInTheDocument();
    });
    expect(screen.getByRole('button', { name: 'toolbar:agentPermissions.actions.revoke' })).toBeDisabled();
  });
});
