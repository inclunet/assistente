import type { ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetProviders = vi.fn();
const mockCreateProvider = vi.fn();
const mockDeleteProvider = vi.fn();
const mockCanRemoveAgent = vi.fn();
const mockRemoveAgent = vi.fn();
const mockConfirm = vi.fn();
const mockAddToast = vi.fn();
const mockAnnounce = vi.fn();

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string, fallback?: string | Record<string, unknown>) =>
      typeof fallback === 'string' ? fallback : key,
    i18n: { language: 'pt-BR' },
  }),
}));

vi.mock('@wailsjs/go/app/App', () => ({
  GetLLMProvidersWithStatus: () => mockGetProviders(),
  CreateLLMProvider: (payload: unknown) => mockCreateProvider(payload),
  CanRemoveACPAgent: (agentID: string) => mockCanRemoveAgent(agentID),
  DeleteLLMProvider: (_ctx: unknown, id: string) => mockDeleteProvider(id),
  RemoveACPAgent: (agentID: string) => mockRemoveAgent(agentID),
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    handleGridReady: vi.fn(),
  }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: mockAnnounce,
  }),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { addToast: mockAddToast };
    return selector ? selector(s) : s;
  },
}));

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => mockConfirm,
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, actions }: { left?: ReactNode; actions?: Array<{ key: string; label: string; onClick?: () => void; disabled?: boolean }> }) => (
    <div>
      {left}
      {actions?.map((action) => (
        <button
          key={action.key}
          data-testid={`toolbar-action-${action.key}`}
          onClick={action.onClick}
          disabled={action.disabled}
        >
          {action.label}
        </button>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({
    items,
    onFocusChange,
    getRowActions,
  }: {
    items?: Array<{ id: string; name: string; type: string; base_url: string }>;
    onFocusChange?: (item: { id: string; name: string; type: string; base_url: string } | null) => void;
    getRowActions?: (item: { id: string; name: string; type: string; base_url: string }) => Array<{ id: string; label?: string; onClick?: () => void }>;
  }) => (
    <div>
      <button type="button" onClick={() => onFocusChange?.(items?.[0] ?? null)}>
        focus-first
      </button>
      {items?.map((item) => (
        <div key={item.id}>
          <span>{item.name}</span>
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
  Modal: ({ isOpen, children }: { isOpen: boolean; children?: ReactNode }) => (isOpen ? <div>{children}</div> : null),
  isModalOpen: () => false,
}));

// O dublê mostra o que recebeu: é a única forma de um teste de página provar que
// a configuração salva chega ao formulário, em vez de ser montada à mão nele.
vi.mock('../components/settings/ProviderForm', () => ({
  ProviderForm: ({
    provider,
    onSave,
    onCancel,
  }: {
    provider?: { acp_command?: string; acp_args?: string[] };
    onSave: () => void;
    onCancel: () => void;
  }) => (
    <div>
      <span data-testid="form-acp-command">{provider?.acp_command ?? ''}</span>
      <span data-testid="form-acp-args">{JSON.stringify(provider?.acp_args ?? [])}</span>
      <button type="button" onClick={onSave}>Salvar</button>
      <button type="button" onClick={onCancel}>Cancelar</button>
    </div>
  ),
}));

import ProvidersPage from './ProvidersPage';

describe('ProvidersPage', () => {
  let nowSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    nowSpy = vi.spyOn(Date, 'now');
    mockGetProviders.mockResolvedValue([
      {
        id: 'openai-1',
        name: 'OpenAI',
        type: 'openai',
        base_url: 'https://api.openai.com',
        credential_required: true,
        credential_status: 'configured',
      },
    ]);
    mockCreateProvider.mockReset();
    mockDeleteProvider.mockReset();
    mockCanRemoveAgent.mockReset();
    mockRemoveAgent.mockReset();
    mockConfirm.mockReset();
    mockCreateProvider.mockResolvedValue(undefined);
    mockDeleteProvider.mockResolvedValue(undefined);
    mockCanRemoveAgent.mockResolvedValue(false);
    mockRemoveAgent.mockResolvedValue(undefined);
    mockConfirm.mockResolvedValue(true);
    mockAddToast.mockReset();
    mockAnnounce.mockReset();
    nowSpy.mockReturnValue(123);
  });

  afterEach(() => {
    nowSpy.mockRestore();
  });

  it('duplica provedor via menu de acoes', async () => {
    const user = userEvent.setup();
    render(<ProvidersPage />);

    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument();
    });

    const duplicateButtons = screen.getAllByRole('button', { name: 'Duplicar' });
    const menuDuplicate = duplicateButtons.find((button) => !button.hasAttribute('disabled'));
    expect(menuDuplicate).toBeTruthy();
    await user.click(menuDuplicate!);

    await waitFor(() => {
      expect(mockCreateProvider).toHaveBeenCalledWith(expect.objectContaining({
        id: 'openai-123',
        name: 'OpenAI (Copia)',
        type: 'openai',
        base_url: 'https://api.openai.com',
      }));
    });
  });

  it('leva o comando salvo do agente ao formulario de edicao', async () => {
    // Um provedor de agente é endereçado pelo comando, e não por URL: sem ele o
    // formulário mostraria menos do que está salvo e a validação barraria até
    // quem só queria renomear.
    mockGetProviders.mockResolvedValue([
      {
        id: 'cursor-1',
        name: 'Cursor local',
        type: 'cursor',
        api_format: 'acp',
        base_url: '',
        credential_required: false,
        credential_status: 'none',
        acp_command: '/opt/cursor/agente',
        acp_args: ['acp', '--forcar'],
      },
    ]);
    const user = userEvent.setup();
    render(<ProvidersPage />);

    await waitFor(() => {
      expect(screen.getByText('Cursor local')).toBeInTheDocument();
    });

    const editButtons = screen.getAllByRole('button', { name: 'Editar' });
    const rowEdit = editButtons.find((button) => !button.hasAttribute('disabled'));
    expect(rowEdit).toBeTruthy();
    await user.click(rowEdit!);

    expect(await screen.findByTestId('form-acp-command')).toHaveTextContent('/opt/cursor/agente');
    expect(screen.getByTestId('form-acp-args')).toHaveTextContent('["acp","--forcar"]');
  });

  it('duplica provedor de agente com o comando que o sobe', async () => {
    mockGetProviders.mockResolvedValue([
      {
        id: 'cursor-1',
        name: 'Cursor local',
        type: 'cursor',
        api_format: 'acp',
        base_url: '',
        credential_required: false,
        credential_status: 'none',
        acp_command: '/opt/cursor/agente',
        acp_args: ['acp'],
      },
    ]);
    const user = userEvent.setup();
    render(<ProvidersPage />);

    await waitFor(() => {
      expect(screen.getByText('Cursor local')).toBeInTheDocument();
    });

    const duplicateButtons = screen.getAllByRole('button', { name: 'Duplicar' });
    const rowDuplicate = duplicateButtons.find((button) => !button.hasAttribute('disabled'));
    await user.click(rowDuplicate!);

    await waitFor(() => {
      expect(mockCreateProvider).toHaveBeenCalledWith(expect.objectContaining({
        type: 'cursor',
        api_format: 'acp',
        acp_command: '/opt/cursor/agente',
        acp_args: ['acp'],
      }));
    });
    // Sem erro: o backend recusa o formato acp sem comando, e a cópia sem ele
    // morreria em toast de erro.
    expect(mockAddToast).not.toHaveBeenCalledWith(expect.anything(), 'error');
  });

  it('habilita acao de excluir na toolbar apos foco', async () => {
    const user = userEvent.setup();
    render(<ProvidersPage />);

    await waitFor(() => {
      expect(screen.getByText('OpenAI')).toBeInTheDocument();
    });

    const deleteButton = screen.getByTestId('toolbar-action-delete');
    expect(deleteButton).toBeDisabled();

    await user.click(screen.getByRole('button', { name: 'focus-first' }));
    await user.click(deleteButton);

    await waitFor(() => {
      expect(mockDeleteProvider).toHaveBeenCalledWith('openai-1');
    });
  });

  it('oferece desinstalar depois de remover o ultimo provedor do agente', async () => {
    mockGetProviders.mockResolvedValue([
      {
        id: 'cursor-1',
        name: 'Cursor local',
        type: 'acp',
        api_format: 'acp',
        base_url: '',
        credential_required: false,
        credential_status: 'none',
        acp_agent_id: 'cursor',
        acp_command: 'cursor-agent',
        acp_args: ['acp'],
      },
    ]);
    mockCanRemoveAgent.mockResolvedValue(true);
    const user = userEvent.setup();
    render(<ProvidersPage />);

    await screen.findByText('Cursor local');
    const deleteButtons = screen.getAllByRole('button', { name: 'Excluir' });
    const rowDelete = deleteButtons.find((button) => !button.hasAttribute('disabled'));
    await user.click(rowDelete!);

    await waitFor(() => {
      expect(mockDeleteProvider).toHaveBeenCalledWith('cursor-1');
      expect(mockCanRemoveAgent).toHaveBeenCalledWith('cursor');
      expect(mockRemoveAgent).toHaveBeenCalledWith('cursor');
    });
    expect(mockConfirm).toHaveBeenCalledTimes(2);
    expect(mockConfirm.mock.calls[1]?.[0]).toEqual(expect.objectContaining({
      title: 'providers.confirm.removeUnusedAgentTitle',
      confirmText: 'providers.confirm.removeUnusedAgentConfirm',
      cancelText: 'providers.confirm.keepAgent',
    }));
  });
});
