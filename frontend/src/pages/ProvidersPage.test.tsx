import { useState, type ReactNode } from 'react';
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockGetProviders = vi.fn();
const mockCreateProvider = vi.fn();
const mockUpdateProvider = vi.fn();
const mockDeleteProvider = vi.fn();
const mockCanRemoveAgent = vi.fn();
const mockRemoveAgent = vi.fn();
const mockAgentInstallPlan = vi.fn();
const mockUpdateAgent = vi.fn();
const mockConfirm = vi.fn();
const mockAddToast = vi.fn();
const mockAnnounce = vi.fn();
const mockAnnounceRequest = vi.fn();

const mockGetConnection = vi.fn();
const mockDisconnectChatGPT = vi.fn();
const mockCreateChatGPT = vi.fn();
const mockAuthorizeChatGPT = vi.fn();
const mockCancelChatGPT = vi.fn();
const mockT = (key: string, fallback?: string | Record<string, unknown>) => typeof fallback === 'string' ? fallback : key;

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: mockT,
    i18n: { language: 'pt-BR' },
  }),
}));

vi.mock('@wailsjs/go/wailsapi/ACPInstall', () => ({
  ACPAgentInstallPlan: (agentID: string) => mockAgentInstallPlan(agentID),
  CanRemoveACPAgent: (agentID: string) => mockCanRemoveAgent(agentID),
  RemoveACPAgent: (agentID: string) => mockRemoveAgent(agentID),
  UpdateACPAgent: (agentID: string, confirmation: unknown) => mockUpdateAgent(agentID, confirmation),
}));

vi.mock('@wailsjs/go/wailsapi/LLMProviders', () => ({
  GetLLMProvidersWithStatus: () => mockGetProviders(),
  GetLLMProvider: vi.fn().mockResolvedValue({ name: 'ChatGPT', default_model: 'account-model', is_default: false }),
  UpdateLLMProvider: (...args: unknown[]) => mockUpdateProvider(...args),
  CreateLLMProvider: (payload: unknown) => mockCreateProvider(payload),
  DeleteLLMProvider: (id: string) => mockDeleteProvider(id),
  SetDefaultProvider: vi.fn(),
  ChatGPTConnection: (...args: unknown[]) => mockGetConnection(...args),
  DisconnectChatGPT: (...args: unknown[]) => mockDisconnectChatGPT(...args),
  CreateChatGPTConnection: (...args: unknown[]) => mockCreateChatGPT(...args),
  AuthorizeChatGPT: (...args: unknown[]) => mockAuthorizeChatGPT(...args),
  CancelChatGPT: (...args: unknown[]) => mockCancelChatGPT(...args),
}));

vi.mock('@wailsjs/go/wailsapi/LLMModels', () => ({
  GetModelsByProvider: vi.fn().mockResolvedValue(['account-model']),
}));

vi.mock('react-router-dom', async importOriginal => ({ ...await importOriginal<typeof import('react-router-dom')>(), useLocation: () => ({ pathname: '/settings/providers' }) }));
vi.mock('../lib/commandShortcutHints', () => ({ useCommandShortcutHint: () => 'Ctrl+N' }));
// Page tests exercise its forms; the real command/menu pipeline has its own integration tests.
vi.mock('../lib/providerCreationCommands', () => ({
  useProviderCreationCommands: ({ open }: { open: (kind: 'chatgpt' | 'acp' | 'api') => void }) => {
    const [shown, show] = useState(false);
    return {
      buttonRef: { current: null }, requestOpen: () => show(true),
      menu: shown ? <div role="menu">{(['chatgpt', 'acp', 'api'] as const).map(kind =>
        <button role="menuitem" key={kind} onClick={() => { show(false); open(kind); }}>
          {kind === 'chatgpt' ? 'chatgpt.add' : 'providers.creation.' + kind}
        </button>)}</div> : null,
    };
  },
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    handleGridReady: vi.fn(),
  }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: mockAnnounce,
    announceRequest: mockAnnounceRequest,
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
    getRowActions?: (item: { id: string; name: string; type: string; base_url: string }) => Array<{ id: string; label?: string; disabled?: boolean; onClick?: () => void }>;
  }) => (
    <div>
      <button type="button" onClick={() => onFocusChange?.(items?.[0] ?? null)}>
        focus-first
      </button>
      {items?.map((item) => (
        <div key={item.id}>
          <span>{item.name}</span>
          {getRowActions?.(item)?.map((action) => (
            <button key={action.id} type="button" onClick={action.onClick} disabled={action.disabled}>
              {action.label}
            </button>
          ))}
        </div>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/Modal', () => ({
  Modal: ({ isOpen, children, allowClose = true, onClose }: { isOpen: boolean; children?: ReactNode; allowClose?: boolean; onClose: () => void }) => (isOpen ? <div><button disabled={!allowClose} onClick={onClose}>modal-close</button>{children}</div> : null),
  isModalOpen: () => false,
  useModalId: () => null,
  useModalIsTopmost: () => () => true,
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
import { useNavigationStore } from '../store/navigationStore';

describe('ProvidersPage', () => {
  let nowSpy: ReturnType<typeof vi.spyOn>;

  beforeEach(() => {
    useNavigationStore.getState().clearPendingEdit();
    nowSpy = vi.spyOn(Date, 'now');
    mockGetConnection.mockReset().mockResolvedValue({ id: 'chatgpt', state: 'connected' });
    mockDisconnectChatGPT.mockReset();
    mockCreateChatGPT.mockReset();
    mockAuthorizeChatGPT.mockReset().mockReturnValue(new Promise(() => undefined));
    mockCancelChatGPT.mockReset().mockResolvedValue(undefined);
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
    mockUpdateProvider.mockReset().mockResolvedValue({});
    mockCreateProvider.mockReset();
    mockDeleteProvider.mockReset();
    mockCanRemoveAgent.mockReset();
    mockRemoveAgent.mockReset();
    mockAgentInstallPlan.mockReset();
    mockUpdateAgent.mockReset();
    mockConfirm.mockReset();
    mockCreateProvider.mockResolvedValue(undefined);
    mockDeleteProvider.mockResolvedValue(undefined);
    mockCanRemoveAgent.mockResolvedValue(false);
    mockRemoveAgent.mockResolvedValue(undefined);
    mockUpdateAgent.mockResolvedValue({ version: '2.0.0' });
    mockConfirm.mockResolvedValue(true);
    mockAddToast.mockReset();
    mockAnnounce.mockReset();
    mockAnnounceRequest.mockReset();
    nowSpy.mockReturnValue(123);
  });

  afterEach(() => {
    nowSpy.mockRestore();
  });

  it('preserva edição pendente quando a carga de provedores falha', async () => {
    mockGetProviders.mockRejectedValueOnce(new Error('offline'));
    useNavigationStore.getState().requestResourceEdit('providers', 'openai-1');
    render(<ProvidersPage />);
    await waitFor(() => expect(mockAddToast).toHaveBeenCalledWith('Erro ao carregar provedores', 'error'));
    expect(useNavigationStore.getState().pendingEdit?.id).toBe('openai-1');
    expect(screen.queryByTestId('form-acp-command')).not.toBeInTheDocument();
  });

  it('abre criação pedida por navegação mesmo com lista vazia', async () => {
    mockGetProviders.mockResolvedValueOnce([]);
    useNavigationStore.getState().requestResourceEdit('providers', '', 'new');
    render(<ProvidersPage />);
    await screen.findByRole('menu');
    expect(useNavigationStore.getState().pendingEdit).toBeNull();
  });

  it('blocks closing and disconnecting while account preferences are being saved', async () => {
    let complete!: () => void;
    mockUpdateProvider.mockReturnValueOnce(new Promise<void>(resolve => { complete = resolve; }));
    mockGetProviders.mockResolvedValue([{ id: 'chatgpt', name: 'Account', type: 'chatgpt',
      base_url: '', credential_required: true, credential_status: 'configured' }]);
    const user = userEvent.setup();
    render(<ProvidersPage />);
    await screen.findByText('Account');
    await user.click(screen.getAllByRole<HTMLButtonElement>('button', { name: 'Editar' }).find(button => !button.disabled)!);
    await screen.findByDisplayValue('ChatGPT');
    await user.click(screen.getByRole('button', { name: 'chatgpt.savePreferences' }));
    expect(screen.getByRole('button', { name: 'modal-close' })).toBeDisabled();
    expect(screen.getByRole('button', { name: 'chatgpt.disconnect' })).toBeDisabled();
    await act(async () => { complete(); });
    await waitFor(() => expect(screen.getByRole('button', { name: 'modal-close' })).toBeEnabled());
  });

  it('blocks the modal close control until the connection record is created', async () => {
    let complete!: (value: { id: string }) => void;
    mockCreateChatGPT.mockReturnValueOnce(new Promise(resolve => { complete = resolve; }));
    const user = userEvent.setup();
    render(<ProvidersPage />);
    await screen.findByText('OpenAI');
    await user.click(screen.getByTestId('toolbar-action-add'));
    await user.click(screen.getByRole('menuitem', { name: 'chatgpt.add' }));
    await user.type(screen.getByLabelText('chatgpt.label'), 'Account');
    await user.click(screen.getByRole('button', { name: 'chatgpt.connect' }));
    const close = screen.getByRole('button', { name: 'modal-close' });
    expect(close).toBeDisabled();
    await user.click(close);
    expect(screen.getByRole('button', { name: 'common.cancel' })).toBeDisabled();
    await act(async () => complete({ id: 'created' }));
    expect(close).toBeEnabled();
    await user.click(close);
    expect(screen.queryByRole('button', { name: 'chatgpt.connect' })).not.toBeInTheDocument();
    expect(mockCancelChatGPT).toHaveBeenCalledWith('created');
  });

  it.each(['unconfirmed', 'failed'])('preserves the disconnection result while refreshing the list: %s', async outcome => {
    const providers = [{ id: 'chatgpt', name: 'ChatGPT', type: 'chatgpt', base_url: 'https://api.openai.com/v1', credential_status: 'configured' }];
    mockGetProviders.mockResolvedValue(providers);
    let complete!: (value: boolean) => void, fail!: (error: Error) => void;
    mockDisconnectChatGPT.mockReturnValueOnce(new Promise((resolve, reject) => { complete = resolve; fail = reject; }));
    const user = userEvent.setup();
    render(<ProvidersPage />);
    await screen.findByText('ChatGPT');
    await user.click(screen.getByRole('button', { name: 'focus-first' }));
    await user.click(screen.getByTestId('toolbar-action-edit'));
    await screen.findByRole('button', { name: 'chatgpt.reconnect' });
    await user.click(screen.getByRole('button', { name: 'chatgpt.disconnect' }));
    expect(screen.getByRole('button', { name: 'modal-close' })).toBeDisabled();
    let refreshed!: (value: typeof providers) => void;
    mockGetProviders.mockReturnValueOnce(new Promise(resolve => { refreshed = resolve; }));
    await act(async () => { if (outcome === 'unconfirmed') complete(false); else fail(new Error('failure')); });
    const message = outcome === 'unconfirmed' ? 'chatgpt.revocationUnconfirmed' : 'chatgpt.connectionError';
    expect(screen.getByText(message)).toBeInTheDocument();
    expect(screen.getByText('Carregando...')).toBeInTheDocument();
    await act(async () => refreshed(providers));
    expect(screen.getByText(message)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'modal-close' })).toBeEnabled();
    expect(mockGetConnection).toHaveBeenCalledTimes(1);
    expect(mockAnnounce.mock.calls.some(call => call[0] === message)).toBe(true);
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

  it.each([['custom', 'none'], ['ollama', 'required']])('preserva auth_mode ao duplicar %s/%s', async (type, auth_mode) => {
    mockGetProviders.mockResolvedValue([{ id: 'api', name: 'API original', type, auth_mode,
      api_format: 'openai', base_url: 'https://example.com', credential_pattern: 'shared-alias', credential_status: 'configured' }]);
    const user = userEvent.setup();
    render(<ProvidersPage />);
    await screen.findByText('API original');
    await user.click(screen.getAllByRole('button', { name: 'Duplicar' }).find(button => !button.hasAttribute('disabled'))!);
    await waitFor(() => expect(mockCreateProvider).toHaveBeenCalledWith(expect.objectContaining({ type, auth_mode, credential_from_provider_id: 'api' })));
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

  it.each([['oauth_authorization_changed', 'chatgpt.authorizationChanged'], ['oauth_vault_persistence_required', 'chatgpt.vaultUnavailable'], ['oauth_vault_unavailable', 'chatgpt.vaultUnavailable'], ['chatgpt_disconnect_before_delete', 'chatgpt.disconnectBeforeDelete'], ['chatgpt_authorization_in_progress', 'chatgpt.authorizationInProgress']])('traduz recusa de exclusao %s', async (code, key) => {
    mockDeleteProvider.mockRejectedValueOnce(new Error(code));
    const user = userEvent.setup();
    render(<ProvidersPage />);
    await screen.findByText('OpenAI');
    await user.click(screen.getByRole('button', { name: 'focus-first' }));
    await user.click(screen.getByTestId('toolbar-action-delete'));
    await waitFor(() => expect(mockAddToast).toHaveBeenCalledWith(key, 'error'));
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

  it('mantem Atualizar agente desabilitado para provedor que nao e ACP', async () => {
    render(<ProvidersPage />);

    await screen.findByText('OpenAI');

    expect(screen.getByRole('button', { name: 'Atualizar agente' })).toBeDisabled();
    expect(mockAgentInstallPlan).not.toHaveBeenCalled();
  });

  it('mantem Atualizar agente desabilitado quando o catalogo nao tem versao nova', async () => {
    mockGetProviders.mockResolvedValue([{
      id: 'cursor-1',
      name: 'Cursor local',
      type: 'acp',
      api_format: 'acp',
      base_url: '',
      credential_required: false,
      credential_status: 'none',
      acp_agent_id: 'cursor',
    }]);
    mockAgentInstallPlan.mockResolvedValue({
      agent_id: 'cursor',
      installed: { version: '1.0.0' },
      version: '1.0.0',
      update: false,
      can_update: false,
    });

    render(<ProvidersPage />);

    await waitFor(() => {
      expect(mockAgentInstallPlan).toHaveBeenCalledWith('cursor');
    });
    expect(screen.getByRole('button', { name: 'Atualizar agente' })).toBeDisabled();
  });

  it('atualiza agente ACP pelo fluxo Wails existente e anuncia sucesso', async () => {
    mockGetProviders.mockResolvedValue([{
      id: 'cursor-1',
      name: 'Cursor local',
      type: 'acp',
      api_format: 'acp',
      base_url: '',
      credential_required: false,
      credential_status: 'none',
      acp_agent_id: 'cursor',
    }]);
    mockAgentInstallPlan.mockResolvedValue({
      agent_id: 'cursor',
      name: 'Cursor',
      installed: { version: '1.0.0' },
      version: '2.0.0',
      distribution: 'binary',
      origin: 'https://example.test/cursor.zip',
      sha256: 'abc123',
      update: true,
      can_update: true,
      unverified: false,
    });
    const user = userEvent.setup();
    render(<ProvidersPage />);

    const update = await screen.findByRole('button', { name: 'Atualizar agente' });
    await waitFor(() => expect(update).toBeEnabled());
    await user.click(update);
    const confirmUpdate = await screen.findByRole('button', {
      name: 'providerForm.agent.catalog.confirm.confirmUpdateBtn',
    });
    expect(confirmUpdate).toHaveAttribute(
      'aria-keyshortcuts',
      expect.stringContaining('Control+Enter'),
    );
    expect(screen.getByRole('button', {
      name: 'providerForm.agent.catalog.confirm.cancelBtn',
    })).toHaveAttribute(
      'aria-keyshortcuts',
      expect.stringContaining('Control+Backspace'),
    );
    await user.click(confirmUpdate);

    await waitFor(() => {
      expect(mockUpdateAgent).toHaveBeenCalledWith('cursor', {
        distribution: 'binary',
        origin: 'https://example.test/cursor.zip',
        sha256: 'abc123',
        accept_unverified: false,
      });
      expect(mockAnnounce).toHaveBeenCalledWith('providers.toast.agentUpdated');
    });
  });

  it('anuncia de forma assertiva quando a atualização do agente falha', async () => {
    mockGetProviders.mockResolvedValue([{
      id: 'cursor-1',
      name: 'Cursor local',
      type: 'acp',
      api_format: 'acp',
      base_url: '',
      credential_required: false,
      credential_status: 'none',
      acp_agent_id: 'cursor',
    }]);
    mockAgentInstallPlan.mockResolvedValue({
      agent_id: 'cursor',
      name: 'Cursor',
      installed: { version: '1.0.0' },
      version: '2.0.0',
      distribution: 'binary',
      origin: 'https://example.test/cursor.zip',
      sha256: 'abc123',
      update: true,
      can_update: true,
    });
    mockUpdateAgent.mockRejectedValue(new Error('turno em andamento'));
    const user = userEvent.setup();
    render(<ProvidersPage />);

    const update = await screen.findByRole('button', { name: 'Atualizar agente' });
    await waitFor(() => expect(update).toBeEnabled());
    await user.click(update);
    await user.click(await screen.findByRole('button', {
      name: 'providerForm.agent.catalog.confirm.confirmUpdateBtn',
    }));

    await waitFor(() => {
      expect(mockAnnounce).toHaveBeenCalledWith(
        'providers.error.updateAgentFailed',
        'assertive',
      );
      expect(mockAddToast).toHaveBeenCalledWith(
        'providers.error.updateAgentFailed',
        'error',
        undefined,
        undefined,
        { suppressAnnounce: true },
      );
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

