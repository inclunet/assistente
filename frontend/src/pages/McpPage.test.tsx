import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, type ReactNode } from 'react';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockSave = vi.fn();
const mockGetConfig = vi.fn();
const mockLoadServers = vi.fn();
const mockDuplicate = vi.fn();
const mockDiscover = vi.hoisted(() =>
  vi.fn(async (_url?: string): Promise<Record<string, unknown>> => ({ found: false }))
);
let mockServers: Array<Record<string, unknown>> = [];

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string) =>
      ({
        'mcp.buttons.newServer': 'Novo Servidor',
        'mcp.actions.duplicate': 'Duplicar',
        'mcp.buttons.delete': 'Excluir',
        'common.save': 'Salvar',
      } as Record<string, string>)[key] ?? key,
  }),
}));

vi.mock('../store/mcpStore', () => ({
  useMCPStore: () => ({
    servers: mockServers,
    isLoading: false,
    loadServers: mockLoadServers,
    connect: vi.fn(),
    disconnect: vi.fn(),
    reconnect: vi.fn(),
    save: mockSave,
    remove: vi.fn(),
    getConfig: mockGetConfig,
    setupEventListeners: () => () => {},
  }),
}));

vi.mock('@wailsjs/go/wailsapi/MCP', () => ({
  SaveMCPServerAuth: vi.fn(() => Promise.resolve()),
  DeleteMCPServerAuth: vi.fn(() => Promise.resolve()),
  GetMCPServerAuthInfo: vi.fn(() => Promise.resolve({ hasAuth: false })),
  DiscoverMCPServerAuth: mockDiscover,
  DuplicateMCPServer: (slug: string) => mockDuplicate(slug),
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    handleGridReady: vi.fn(),
  }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: vi.fn(),
  }),
}));

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => vi.fn(() => Promise.resolve(true)),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { addToast: vi.fn() };
    return selector ? selector(s) : s;
  },
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ actions }: { actions?: Array<{ key: string; label: string; onClick: () => void }> }) => (
    <div>
      {actions?.map((a) => (
        <button key={a.key} onClick={a.onClick}>{a.label}</button>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({
    items,
    getRowActions,
  }: {
    items?: Array<{ id: string; name: string }>;
    getRowActions?: (item: { id: string; name: string }) => Array<{ id: string; label: string; onClick: () => void }>;
  }) => (
    <div>
      {items?.map((item) => (
        <div key={item.id}>
          <span>{item.name}</span>
          {getRowActions?.(item)?.map((action) => (
            <button key={action.id} onClick={action.onClick}>{action.label}</button>
          ))}
        </div>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/Modal', () => ({
  Modal: ({ isOpen, children }: { isOpen: boolean; children: ReactNode }) =>
    (isOpen ? <div role="dialog">{children}</div> : null),
  isModalOpen: () => false,
}));

vi.mock('../components/ui/EditorPanel', () => ({
  EditorPanelFooter: ({ children }: { children: ReactNode }) => <div>{children}</div>,
}));

vi.mock('../components', () => ({
  Button: ({ children, onClick }: { children?: ReactNode; onClick?: () => void }) => (
    <button onClick={onClick}>{children}</button>
  ),
}));

vi.mock('../components/mcp/McpGeneralSection', () => ({
  McpGeneralSection: ({
    name,
    transport,
    onNameChange,
    onTransportChange,
  }: {
    name: string;
    transport: string;
    onNameChange: (value: string) => void;
    onTransportChange: (value: string) => void;
  }) => (
    <div>
      <label>
        Nome
        <input aria-label="Nome" value={name} onChange={(e) => onNameChange(e.target.value)} />
      </label>
      <label>
        Tipo
        <select aria-label="Tipo" value={transport} onChange={(e) => onTransportChange(e.target.value)}>
          <option value="stdio">Local</option>
          <option value="streamable">Remoto</option>
        </select>
      </label>
    </div>
  ),
}));

vi.mock('../components/mcp/McpConnectionSection', () => ({
  McpConnectionSection: (props: {
    url: string;
    oauth2AuthUrl: string;
    oauth2TokenUrl: string;
    oauth2Scopes: string;
    discoveryRegistrationUrl: string;
    oauth2CallbackHost: string;
    oauth2CallbackPort: string;
    authType: string;
    onUrlChange: (value: string) => void;
    onAuthTypeChange: (value: string) => void;
    onOAuth2AuthUrlChange: (value: string) => void;
    onOAuth2TokenUrlChange: (value: string) => void;
    onOAuth2ScopesChange: (value: string) => void;
    onOAuth2CallbackHostChange: (value: string) => void;
    onOAuth2CallbackPortChange: (value: string) => void;
    onUrlBlur: () => void;
    onManualOverride: () => void;
  }) => (
    <div data-testid="connection-section">
      <input aria-label="Server URL" value={props.url} onChange={(e) => props.onUrlChange(e.target.value)} />
      <input aria-label="Authorization URL" value={props.oauth2AuthUrl} onChange={(e) => props.onOAuth2AuthUrlChange(e.target.value)} />
      <input aria-label="Token URL" value={props.oauth2TokenUrl} onChange={(e) => props.onOAuth2TokenUrlChange(e.target.value)} />
      <input aria-label="OAuth Scopes" value={props.oauth2Scopes} onChange={(e) => props.onOAuth2ScopesChange(e.target.value)} />
      <span data-testid="registration-url-value">{props.discoveryRegistrationUrl}</span>
      <span data-testid="callback-host-value">{props.oauth2CallbackHost}</span>
      <span data-testid="callback-port-value">{props.oauth2CallbackPort}</span>
      <label>
        Auth Type
        <select
          aria-label="Auth Type"
          value={props.authType}
          onChange={(e) => props.onAuthTypeChange(e.target.value)}
        >
          <option value="none">None</option>
          <option value="oauth2_pkce">PKCE</option>
        </select>
      </label>
      <label>
        Callback Host
        <select
          aria-label="Callback Host"
          value={props.oauth2CallbackHost}
          onChange={(e) => props.onOAuth2CallbackHostChange(e.target.value)}
        >
          <option value="">default</option>
          <option value="localhost">localhost</option>
          <option value="127.0.0.1">127.0.0.1</option>
          <option value="[::1]">[::1]</option>
        </select>
      </label>
      <label>
        Callback Port
        <input
          aria-label="Callback Port"
          value={props.oauth2CallbackPort}
          onChange={(e) => props.onOAuth2CallbackPortChange(e.target.value)}
        />
      </label>
      <button type="button" onClick={props.onUrlBlur}>Descobrir OAuth</button>
      <button type="button" onClick={props.onManualOverride}>Configurar manualmente</button>
    </div>
  ),
}));

import McpPage from './McpPage';

describe('McpPage — oauth2_callback_host', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSave.mockResolvedValue(undefined);
    mockLoadServers.mockResolvedValue(undefined);
    mockDiscover.mockResolvedValue({ found: false });
    mockServers = [];
  });

  async function openNewServerForm() {
    render(<McpPage />);
    await userEvent.click(screen.getByText('Novo Servidor'));
    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument();
    });
  }

  it('inicializa oauth2CallbackHost vazio para novo servidor', async () => {
    await openNewServerForm();
    expect(screen.getByTestId('callback-host-value')).toHaveTextContent('');
  });

  it('propaga mudança de callback host para o componente', async () => {
    await openNewServerForm();

    await userEvent.selectOptions(screen.getByLabelText('Callback Host'), '127.0.0.1');

    expect(screen.getByTestId('callback-host-value')).toHaveTextContent('127.0.0.1');
  });

  it('inclui oauth2_callback_host no config ao salvar com PKCE', async () => {
    await openNewServerForm();

    await userEvent.type(screen.getByLabelText('Nome'), 'Test Server');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    await userEvent.selectOptions(screen.getByLabelText('Auth Type'), 'oauth2_pkce');
    await userEvent.selectOptions(screen.getByLabelText('Callback Host'), '127.0.0.1');
    await userEvent.type(screen.getByLabelText('Callback Port'), '3118');

    await userEvent.click(screen.getByText('Salvar'));

    await waitFor(() => {
      expect(mockSave).toHaveBeenCalledTimes(1);
    });

    const [slug, config] = mockSave.mock.calls[0];
    expect(slug).toBe('test-server');
    expect(config.oauth2_callback_host).toBe('127.0.0.1');
    expect(config.oauth2_callback_port).toBe(3118);
  });

  it('não inclui oauth2_callback_host quando authType não é PKCE', async () => {
    await openNewServerForm();

    await userEvent.type(screen.getByLabelText('Nome'), 'No PKCE');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');

    await userEvent.click(screen.getByText('Salvar'));

    await waitFor(() => {
      expect(mockSave).toHaveBeenCalledTimes(1);
    });

    const [, config] = mockSave.mock.calls[0];
    expect(config.oauth2_callback_host).toBeUndefined();
  });

  it('duplica servidor MCP via menu de acoes', async () => {
    mockServers = [
      {
        slug: 'mcp-teste',
        name: 'MCP Teste',
        description: '',
        transport: 'stdio',
        status: 'disconnected',
        toolCount: 0,
        enabled: true,
        autoConnect: false,
      },
    ];

    mockDuplicate.mockResolvedValue('mcp-teste-copia');
    mockGetConfig.mockResolvedValue({
      name: 'MCP Teste (Copia)',
      transport: 'stdio',
      enabled: true,
      auto_connect: false,
    });

    render(<McpPage />);

    await waitFor(() => {
      expect(screen.getByText('MCP Teste')).toBeInTheDocument();
    });

    const row = screen.getByText('MCP Teste').parentElement;
    if (!row) {
      throw new Error('Linha do servidor MCP nao encontrada');
    }

    await userEvent.click(within(row).getByRole('button', { name: 'Duplicar' }));

    await waitFor(() => {
      expect(mockDuplicate).toHaveBeenCalledWith('mcp-teste');
    });
  });

  it('não inclui oauth2_callback_host quando vazio (usa default do backend)', async () => {
    await openNewServerForm();

    await userEvent.type(screen.getByLabelText('Nome'), 'Default Host');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    await userEvent.selectOptions(screen.getByLabelText('Auth Type'), 'oauth2_pkce');

    await userEvent.click(screen.getByText('Salvar'));

    await waitFor(() => {
      expect(mockSave).toHaveBeenCalledTimes(1);
    });

    const [, config] = mockSave.mock.calls[0];
    expect(config.oauth2_callback_host).toBeUndefined();
  });

  it('preserva endpoints OAuth preenchidos manualmente durante discovery', async () => {
    mockDiscover.mockResolvedValue({
      found: true,
      status: 'complete',
      authType: 'oauth2_pkce',
      authUrl: 'https://descoberto.example/authorize',
      tokenUrl: 'https://descoberto.example/token',
      scopes: ['openid'],
      registrationUrl: '',
    });
    await openNewServerForm();

    await userEvent.type(screen.getByLabelText('Server URL'), 'https://mcp.example/caminho');
    await userEvent.selectOptions(screen.getByLabelText('Auth Type'), 'oauth2_pkce');
    await userEvent.type(screen.getByLabelText('Authorization URL'), 'https://manual.example/authorize');
    await userEvent.type(screen.getByLabelText('Token URL'), 'https://manual.example/token');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    await userEvent.click(screen.getByText('Descobrir OAuth'));

    await waitFor(() => expect(mockDiscover).toHaveBeenCalled());
    expect(screen.getByLabelText('Authorization URL')).toHaveValue('https://manual.example/authorize');
    expect(screen.getByLabelText('Token URL')).toHaveValue('https://manual.example/token');
  });

  it('aproveita scopes do PRM em discovery parcial sem bloquear configuração manual', async () => {
    mockDiscover.mockResolvedValue({
      found: false,
      status: 'partial',
      protectedResourceFound: true,
      resourceName: 'Recurso parcial',
      scopes: ['files:read', 'files:write'],
    });
    await openNewServerForm();

    await userEvent.type(screen.getByLabelText('Server URL'), 'https://mcp.example/caminho');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');

    await waitFor(() => {
      expect(screen.getByLabelText('OAuth Scopes')).toHaveValue('files:read files:write');
    });
    await userEvent.type(screen.getByLabelText('OAuth Scopes'), ' custom');
    expect(screen.getByLabelText('OAuth Scopes')).toHaveValue('files:read files:write custom');
  });

  it('permite repetir discovery da mesma URL após escolher configuração manual', async () => {
    await openNewServerForm();
    await userEvent.type(screen.getByLabelText('Server URL'), 'https://mcp.example/caminho');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    await waitFor(() => expect(mockDiscover).toHaveBeenCalledTimes(1));

    await userEvent.click(screen.getByText('Configurar manualmente'));
    await userEvent.type(screen.getByLabelText('OAuth Scopes'), 'manual:scope');
    expect(mockDiscover).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByText('Descobrir OAuth'));

    await waitFor(() => expect(mockDiscover).toHaveBeenCalledTimes(2));
    expect(mockDiscover).toHaveBeenLastCalledWith('https://mcp.example/caminho');
  });

  it('aceita scheme HTTPS sem depender de caixa', async () => {
    await openNewServerForm();
    await userEvent.type(screen.getByLabelText('Server URL'), 'HTTPS://mcp.example/caminho');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');

    await waitFor(() => {
      expect(mockDiscover).toHaveBeenCalledWith('HTTPS://mcp.example/caminho');
    });
  });

  it('não dispara discovery a cada alteração da URL no modo HTTP', async () => {
    await openNewServerForm();
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    await userEvent.type(screen.getByLabelText('Server URL'), 'https://mcp.example/caminho');

    expect(mockDiscover).not.toHaveBeenCalled();
    await userEvent.click(screen.getByText('Descobrir OAuth'));
    await waitFor(() => expect(mockDiscover).toHaveBeenCalledTimes(1));
  });

  it('limpa registration URL descoberto ao descobrir outro servidor sem DCR', async () => {
    mockDiscover
      .mockResolvedValueOnce({
        found: true,
        status: 'complete',
        authType: 'oauth2_pkce',
        authUrl: 'https://auth.example/authorize',
        tokenUrl: 'https://auth.example/token',
        scopes: [],
        registrationUrl: 'https://auth.example/register',
      })
      .mockResolvedValue({
        found: true,
        status: 'complete',
        authType: 'oauth2_pkce',
        authUrl: 'https://other.example/authorize',
        tokenUrl: 'https://other.example/token',
        scopes: [],
        registrationUrl: '',
      });
    await openNewServerForm();

    fireEvent.change(screen.getByLabelText('Server URL'), {
      target: { value: 'https://first.example/mcp' },
    });
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    await waitFor(() => {
      expect(screen.getByTestId('registration-url-value')).toHaveTextContent(
        'https://auth.example/register'
      );
    });

    fireEvent.change(screen.getByLabelText('Server URL'), {
      target: { value: 'https://second.example/mcp' },
    });
    await waitFor(() => {
      expect(screen.getByTestId('registration-url-value')).toBeEmptyDOMElement();
    });
  });

  it('ignora resposta atrasada de discovery para URL anterior', async () => {
    let resolveFirst: (value: Record<string, unknown>) => void = () => {};
    let resolveSecond: (value: Record<string, unknown>) => void = () => {};
    const first = new Promise<Record<string, unknown>>((resolve) => {
      resolveFirst = resolve;
    });
    const second = new Promise<Record<string, unknown>>((resolve) => {
      resolveSecond = resolve;
    });
    mockDiscover.mockImplementation((url?: string) =>
      url?.includes('first.example') ? first : second
    );
    await openNewServerForm();
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');

    fireEvent.change(screen.getByLabelText('Server URL'), {
      target: { value: 'https://first.example/mcp' },
    });
    await userEvent.click(screen.getByText('Descobrir OAuth'));
    await waitFor(() => {
      expect(mockDiscover).toHaveBeenCalledWith('https://first.example/mcp');
    });
    fireEvent.change(screen.getByLabelText('Server URL'), {
      target: { value: 'https://second.example/mcp' },
    });
    await userEvent.click(screen.getByText('Descobrir OAuth'));
    await waitFor(() => {
      expect(mockDiscover).toHaveBeenCalledWith('https://second.example/mcp');
    });

    await act(async () => {
      resolveSecond({
        found: true,
        status: 'complete',
        authType: 'oauth2_pkce',
        authUrl: 'https://second.example/authorize',
        tokenUrl: 'https://second.example/token',
        scopes: [],
      });
    });
    await waitFor(() => {
      expect(screen.getByLabelText('Token URL')).toHaveValue('https://second.example/token');
    });

    await act(async () => {
      resolveFirst({
        found: true,
        status: 'complete',
        authType: 'oauth2_pkce',
        authUrl: 'https://first.example/authorize',
        tokenUrl: 'https://first.example/token',
        scopes: [],
      });
    });
    expect(screen.getByLabelText('Token URL')).toHaveValue('https://second.example/token');
  });

  it('não aplica registration URL manual de um servidor após trocar sua URL', async () => {
    mockServers = [{
      slug: 'remote',
      name: 'Remote',
      description: '',
      transport: 'streamable',
      status: 'disconnected',
      toolCount: 0,
      enabled: true,
      autoConnect: false,
      url: 'https://old.example/mcp',
    }];
    mockGetConfig.mockResolvedValue({
      name: 'Remote',
      transport: 'streamable',
      url: 'https://old.example/mcp',
      auth_type: 'oauth2_pkce',
      oauth2_registration_url: 'https://old.example/register',
      enabled: true,
      auto_connect: false,
    });

    render(<McpPage />);
    const editButtons = await screen.findAllByRole('button', { name: 'mcp.actions.edit' });
    await userEvent.click(editButtons[editButtons.length - 1]);
    await waitFor(() => {
      expect(screen.getByTestId('registration-url-value')).toHaveTextContent(
        'https://old.example/register'
      );
    });

    fireEvent.change(screen.getByLabelText('Server URL'), {
      target: { value: 'https://old.example/mcp/?view=config#oauth' },
    });
    expect(screen.getByTestId('registration-url-value')).toHaveTextContent(
      'https://old.example/register'
    );

    fireEvent.change(screen.getByLabelText('Server URL'), {
      target: { value: 'https://new.example/mcp' },
    });
    expect(screen.getByTestId('registration-url-value')).toBeEmptyDOMElement();

    await userEvent.click(screen.getByText('Salvar'));
    await waitFor(() => expect(mockSave).toHaveBeenCalled());
    const [, config] = mockSave.mock.calls[mockSave.mock.calls.length - 1];
    expect(config.oauth2_registration_url).toBeUndefined();
  });
});
