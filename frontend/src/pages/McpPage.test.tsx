import { GetCredentialForURL, UpsertCredential } from '@wailsjs/go/wailsapi/Credentials';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { act, type ReactNode } from 'react';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { SaveMCPServerAuth, DeleteMCPServerAuth, GetMCPServerAuthInfo } from '@wailsjs/go/wailsapi/MCP';

const mockSave = vi.fn();
const mockConnect = vi.fn();
const mockToast = vi.fn();
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
    connect: mockConnect,
    disconnect: vi.fn(),
    reconnect: vi.fn(),
    save: mockSave,
    remove: vi.fn(),
    getConfig: mockGetConfig,
    setupEventListeners: () => () => {},
  }),
}));

vi.mock('@wailsjs/go/wailsapi/Credentials', () => ({ GetCredentialForURL: vi.fn(async () => null), UpsertCredential: vi.fn(async () => {}), ListExternalSources: vi.fn(async () => []) }));

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
  announce: vi.fn(),
  useAnnouncer: () => ({
    announce: vi.fn(),
  }),
}));

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => vi.fn(() => Promise.resolve(true)),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { addToast: mockToast };
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
    credentialEditor: ReactNode;
    oauth2AuthUrl: string;
    oauth2TokenUrl: string;
    oauth2Scopes: string;
    oauth2ClientSecret: string;
    oauth2ClientId: string;
    oauthDCRRegistered: boolean;
    onOAuth2ClientIdChange: (value: string) => void;
    onOAuth2ClientSecretChange: (value: string) => void;
    discoveryStatus: string;
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
      {['bearer', 'basic'].includes(props.authType) && props.credentialEditor}
      <input aria-label="Client ID" value={props.oauth2ClientId} onChange={(e) => props.onOAuth2ClientIdChange(e.target.value)} />
      <span data-testid="dcr-registered">{String(props.oauthDCRRegistered)}</span>
      <input aria-label="Client Secret" value={props.oauth2ClientSecret} onChange={(e) => props.onOAuth2ClientSecretChange(e.target.value)} />
      <input aria-label="Server URL" value={props.url} onChange={(e) => props.onUrlChange(e.target.value)} />
      <input aria-label="Authorization URL" value={props.oauth2AuthUrl} onChange={(e) => props.onOAuth2AuthUrlChange(e.target.value)} />
      <input aria-label="Token URL" value={props.oauth2TokenUrl} onChange={(e) => props.onOAuth2TokenUrlChange(e.target.value)} />
      <input aria-label="OAuth Scopes" value={props.oauth2Scopes} onChange={(e) => props.onOAuth2ScopesChange(e.target.value)} />
      <span data-testid="discovery-status-value">{props.discoveryStatus}</span>
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
          <option value="bearer">Bearer</option>
          <option value="basic">Basic</option>
          <option value="oauth2_pkce">PKCE</option>
          <option value="oauth2_client_credentials">Client Credentials</option>
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
import { executeDeepLink } from '../lib/deepLinks';

describe('McpPage — oauth2_callback_host', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSave.mockResolvedValue(undefined);
    mockLoadServers.mockResolvedValue(undefined);
    mockDiscover.mockResolvedValue({ found: false });
    mockServers = [];
    vi.mocked(GetCredentialForURL).mockResolvedValue(null as unknown as Awaited<ReturnType<typeof GetCredentialForURL>>);
  });

  async function openNewServerForm() {
    render(<McpPage />);
    await userEvent.click(screen.getByText('Novo Servidor'));
    await waitFor(() => {
      expect(screen.getByRole('dialog')).toBeInTheDocument();
    });
  }

  it('abre o primeiro cadastro pelo caminho de criação do CredManager', async () => {
    const navigate = vi.fn();
    await executeDeepLink({ type: 'resource:new', resource: 'mcp' }, { navigate });
    render(<McpPage />);
    expect(await screen.findByRole('dialog')).toBeInTheDocument();
    expect(screen.getByLabelText('Nome')).toHaveValue('');
    expect(navigate).toHaveBeenCalledWith('/settings/mcp');
  });

  it('salva command pelo mesmo contrato do CredManager sem gravar um token estático', async () => {
    await openNewServerForm();
    fireEvent.change(screen.getByLabelText('Nome'), {target:{value:'Command MCP'}});
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    fireEvent.change(screen.getByLabelText('Server URL'), {target:{value:'https://mcp.example.com/tools'}});
    await userEvent.selectOptions(screen.getByLabelText('Auth Type'), 'bearer');
    await waitFor(() => expect(screen.getByRole('button', {name:'credentials.mcp.configure'})).toBeEnabled());
    expect(UpsertCredential).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole('button', {name:'credentials.mcp.configure'}));
    await userEvent.selectOptions(screen.getByLabelText('credentials.sourceFields.source'), 'command');
    fireEvent.change(screen.getByLabelText('credentials.sourceFields.commandName'), {target:{value:'wsl.exe'}});
    fireEvent.change(screen.getByLabelText('credentials.sourceFields.args'), {target:{value:'["bash","-ic","nu genai api-gateway token"]'}});
    await userEvent.click(screen.getByText('Salvar'));
    await waitFor(() => expect(mockSave).toHaveBeenCalledWith(expect.any(String),expect.anything(),undefined,expect.objectContaining({pattern:'mcp.example.com',type:'bearer',source:'command',sourceConfig:expect.objectContaining({command:'wsl.exe',args:['bash','-ic','nu genai api-gateway token'],timeoutSeconds:30})})));
    expect(SaveMCPServerAuth).not.toHaveBeenCalled();
    expect(mockSave.mock.calls[0][3].token).toBeUndefined();
    expect(UpsertCredential).not.toHaveBeenCalled();
  });

  it('recusa comando inválido antes de salvar servidor e descarta edição ao mudar destino', async () => {
    await openNewServerForm();
    fireEvent.change(screen.getByLabelText('Nome'), {target:{value:'Test'}});
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    fireEvent.change(screen.getByLabelText('Server URL'), {target:{value:'https://first.example/tools'}});
    await userEvent.selectOptions(screen.getByLabelText('Auth Type'), 'bearer');
    await waitFor(() => expect(screen.getByRole('button', {name:'credentials.mcp.configure'})).toBeEnabled());
    await userEvent.click(screen.getByRole('button', {name:'credentials.mcp.configure'}));
    await userEvent.selectOptions(screen.getByLabelText('credentials.sourceFields.source'), 'command');
    fireEvent.change(screen.getByLabelText('credentials.sourceFields.commandName'), {target:{value:'tool'}});
    fireEvent.change(screen.getByLabelText('credentials.sourceFields.args'), {target:{value:'[1]'}});
    await userEvent.click(screen.getByText('Salvar'));
    expect(mockSave).not.toHaveBeenCalled();
    expect(UpsertCredential).not.toHaveBeenCalled();
    expect(mockToast).toHaveBeenCalledWith('credentials.sourceFields.invalidArgs', 'error');
    fireEvent.change(screen.getByLabelText('Server URL'), {target:{value:'https://second.example/tools'}});
    await userEvent.click(screen.getByText('Salvar'));
    await waitFor(() => expect(mockSave).toHaveBeenCalled());
    expect(UpsertCredential).not.toHaveBeenCalled();
  });

  it('preserva credencial externa existente ao editar apenas o nome do MCP', async () => {
    mockServers = [{slug:'existing',name:'Existing',transport:'streamable',status:'disconnected',enabled:true}];
    mockGetConfig.mockResolvedValue({name:'Existing',transport:'streamable',url:'https://example.com/tools',auth_type:'bearer'});
    vi.mocked(GetCredentialForURL).mockResolvedValue({pattern:'example.com',type:'bearer',source:'env',sourceConfig:{env:'EXISTING_TOKEN'},masked:'env',managed:false} as Awaited<ReturnType<typeof GetCredentialForURL>>);
    render(<McpPage />);
    const row = screen.getByText('Existing').closest('div')!;
    await userEvent.click(within(row).getByRole('button', {name:'mcp.actions.edit'}));
    await screen.findByLabelText('Nome');
    fireEvent.change(screen.getByLabelText('Nome'), {target:{value:'Renamed'}});
    await userEvent.click(screen.getByText('Salvar'));
    await waitFor(() => expect(mockSave).toHaveBeenCalled());
    expect(UpsertCredential).not.toHaveBeenCalled();
    expect(SaveMCPServerAuth).not.toHaveBeenCalled();
  });

  it.each([false, true])('preserva método público no rename e usa Post ao trocar para Client Credentials (%s)', async (clientCredentials) => {
    mockServers = [{slug: 'public', name: 'Public', transport: 'streamable', status: 'disconnected', enabled: true}];
    mockGetConfig.mockResolvedValue({name: 'Public', transport: 'streamable', url: 'https://example.com/mcp', auth_type: 'oauth2_pkce', oauth_managed: true, oauth_authorization_id: 'grant', oauth2_client_id: 'public-client', oauth2_token_auth_method: 'none', oauth2_client_method: 'manual'});
    render(<McpPage />);
    const row = screen.getByText('Public').closest('div');
    if (!row) throw new Error('Linha ausente');
    await userEvent.click(within(row).getByRole('button', {name: 'mcp.actions.edit'}));
    await screen.findByLabelText('Nome');
    fireEvent.change(screen.getByLabelText('Nome'), {target: {value: 'Renamed'}});
    if (clientCredentials) await userEvent.selectOptions(screen.getByLabelText('Auth Type'), 'oauth2_client_credentials');
    await userEvent.click(screen.getByText('Salvar'));
    await waitFor(() => expect(mockSave).toHaveBeenCalledWith('public', expect.objectContaining({name: 'Renamed', oauth2_token_auth_method: clientCredentials ? 'client_secret_post' : 'none'})));
  });

  it.each(['oauth2_pkce', 'bearer', 'basic'].flatMap((authType) => [false, true].map((fail) => ({ authType, fail }))))('delega remoção e configuração none a uma única gravação atômica ($authType, $fail)', async ({ authType, fail }) => {
    mockServers = [{ slug: 'legacy', name: 'Legacy', transport: 'streamable', status: 'disconnected', enabled: true }];
    mockGetConfig.mockResolvedValue({ name: 'Legacy', transport: 'streamable', url: 'https://example.com/mcp', auth_type: authType, oauth_managed: false });
    vi.mocked(GetMCPServerAuthInfo).mockResolvedValueOnce({ hasAuth: true } as Awaited<ReturnType<typeof GetMCPServerAuthInfo>>);
    if (fail) mockSave.mockRejectedValueOnce(new Error('oauth_transient'));
    render(<McpPage />);
    const row = screen.getByText('Legacy').closest('div');
    if (!row) throw new Error('Linha ausente');
    await userEvent.click(within(row).getByRole('button', { name: 'mcp.actions.edit' }));
    await screen.findByLabelText('Auth Type');
    await userEvent.selectOptions(screen.getByLabelText('Auth Type'), 'none');
    await userEvent.click(screen.getByText('Salvar'));
    await waitFor(() => expect(mockSave).toHaveBeenCalledWith('legacy', expect.objectContaining({ auth_type: 'none' })));
    expect(DeleteMCPServerAuth).not.toHaveBeenCalled();
    if (fail) {
      await waitFor(() => expect(mockToast).toHaveBeenCalled());
      expect(screen.getByRole('dialog')).toBeInTheDocument();
    } else {
      await waitFor(() => expect(mockSave).toHaveBeenCalledWith('legacy', expect.objectContaining({ auth_type: 'none' })));
    }
  });

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
    expect(config.oauth_managed).toBe(true);
  });

  it.each([false, true])('envia segredo na gravação atômica (falha=%s)', async (fail) => {
    await openNewServerForm();
    await userEvent.type(screen.getByLabelText('Nome'), 'Atomic');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    await userEvent.selectOptions(screen.getByLabelText('Auth Type'), 'oauth2_pkce');
    await userEvent.type(screen.getByLabelText('Client Secret'), 'secret');
    if (fail) mockSave.mockRejectedValueOnce(new Error('atomic failure'));
    await userEvent.click(screen.getByText('Salvar'));
    await waitFor(() => expect(mockSave).toHaveBeenCalledWith('atomic', expect.objectContaining({oauth_managed: true}), 'secret'));
    expect(SaveMCPServerAuth).not.toHaveBeenCalled();
    if (fail) await waitFor(() => expect(screen.getByLabelText('Nome')).toHaveValue('Atomic'));
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

  it('traduz negativa de rede na descoberta sem preencher endpoints', async () => {
    mockDiscover.mockResolvedValue({ found: false, status: 'partial', error: 'oauth_discovery_destination_blocked' });
    await openNewServerForm();
    await userEvent.type(screen.getByLabelText('Server URL'), 'https://mcp.example/internal');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    await waitFor(() => expect(mockToast).toHaveBeenCalledWith('mcp.error.networkAuthorizationFailed', 'error'));
    expect(screen.getByTestId('discovery-status-value')).toHaveTextContent('not_found');
  });

  it('autocompleta cadastro sem DCR e salva os endpoints após escolha explícita de PKCE', async () => {
    mockServers = [{ slug: 'legacy', name: 'Legacy', transport: 'streamable', status: 'disconnected', enabled: true }];
    mockGetConfig.mockResolvedValue({
      name: 'Legacy', transport: 'streamable', url: 'https://example.com/mcp',
      auth_type: 'oauth2_client_credentials', oauth_managed: false,
      oauth2_client_id: 'company-client',
    });
    mockDiscover.mockResolvedValue({
      found: true, status: 'complete', authType: 'oauth2_pkce',
      authUrl: 'https://auth.example/authorize', tokenUrl: 'https://auth.example/token',
      scopes: ['sql', 'offline_access'], registrationUrl: '',
    });
    render(<McpPage />);
    const row = screen.getByText('Legacy').parentElement!;
    await userEvent.click(within(row).getByRole('button', { name: 'mcp.actions.edit' }));
    await waitFor(() => expect(screen.getByTestId('discovery-status-value')).toHaveTextContent('found'));
    expect(screen.getByLabelText('Auth Type')).toHaveValue('oauth2_client_credentials');
    expect(screen.getByLabelText('Authorization URL')).toHaveValue('https://auth.example/authorize');
    expect(screen.getByLabelText('Token URL')).toHaveValue('https://auth.example/token');
    expect(screen.getByLabelText('OAuth Scopes')).toHaveValue('sql offline_access');
    expect(screen.getByLabelText('Client ID')).toHaveValue('company-client');
    expect(screen.getByLabelText('Client Secret')).toHaveValue('');
    expect(screen.getByLabelText('Callback Host')).toHaveValue('');
    expect(screen.getByLabelText('Callback Port')).toHaveValue('');
    await userEvent.selectOptions(screen.getByLabelText('Auth Type'), 'oauth2_pkce');
    await userEvent.click(screen.getByText('Salvar'));
    await waitFor(() => expect(mockSave).toHaveBeenCalledWith('legacy', expect.objectContaining({
      auth_type: 'oauth2_pkce', oauth_managed: false, oauth2_client_id: 'company-client',
      oauth2_auth_url: 'https://auth.example/authorize', oauth2_token_url: 'https://auth.example/token',
      oauth2_scopes: ['sql', 'offline_access'],
    })));
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
    expect(screen.getByTestId('discovery-status-value')).toHaveTextContent('manual');
    await userEvent.type(screen.getByLabelText('OAuth Scopes'), 'manual:scope');
    expect(mockDiscover).toHaveBeenCalledTimes(1);
    await userEvent.click(screen.getByText('Descobrir OAuth'));

    await waitFor(() => expect(mockDiscover).toHaveBeenCalledTimes(2));
    expect(mockDiscover).toHaveBeenLastCalledWith('https://mcp.example/caminho');
  });

  it.each<[string, Record<string, unknown>]>([
    ['partial', { found: false, status: 'partial', protectedResourceFound: true }],
    ['not_found', { found: false, status: 'not_found' }],
  ])('permite retry explícito da mesma URL após resultado %s', async (status, result) => {
    mockDiscover.mockResolvedValue(result);
    await openNewServerForm();
    await userEvent.type(screen.getByLabelText('Server URL'), 'https://mcp.example/retry');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    await waitFor(() => {
      expect(screen.getByTestId('discovery-status-value')).toHaveTextContent(status);
    });

    await userEvent.click(screen.getByText('Descobrir OAuth'));
    await waitFor(() => expect(mockDiscover).toHaveBeenCalledTimes(2));
  });

  it('permite retry explícito da mesma URL após erro', async () => {
    mockDiscover
      .mockRejectedValueOnce(new Error('falha transitória'))
      .mockResolvedValue({ found: false, status: 'not_found' });
    await openNewServerForm();
    await userEvent.type(screen.getByLabelText('Server URL'), 'https://mcp.example/retry');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    await waitFor(() => {
      expect(screen.getByTestId('discovery-status-value')).toHaveTextContent('not_found');
    });

    await userEvent.click(screen.getByText('Descobrir OAuth'));
    await waitFor(() => expect(mockDiscover).toHaveBeenCalledTimes(2));
  });

  it('não duplica discovery durante loading nem após resultado completo', async () => {
    let resolveDiscovery: (value: Record<string, unknown>) => void = () => {};
    mockDiscover.mockImplementation(() => new Promise((resolve) => {
      resolveDiscovery = resolve;
    }));
    await openNewServerForm();
    await userEvent.type(screen.getByLabelText('Server URL'), 'https://mcp.example/complete');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'streamable');
    await waitFor(() => expect(mockDiscover).toHaveBeenCalledTimes(1));

    await userEvent.click(screen.getByText('Descobrir OAuth'));
    expect(mockDiscover).toHaveBeenCalledTimes(1);

    await act(async () => {
      resolveDiscovery({
        found: true,
        status: 'complete',
        authType: 'oauth2_pkce',
        authUrl: 'https://auth.example/authorize',
        tokenUrl: 'https://auth.example/token',
        scopes: [],
      });
    });
    await waitFor(() => {
      expect(screen.getByTestId('discovery-status-value')).toHaveTextContent('found');
    });
    await userEvent.click(screen.getByText('Descobrir OAuth'));
    expect(mockDiscover).toHaveBeenCalledTimes(1);
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
    expect(screen.getByTestId('discovery-status-value')).toHaveTextContent('idle');
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
  it.each(['rename','change','revert'])('vincula Device ao recurso original (%s)', async (mode) => {
    mockServers = [{slug:'device',name:'Device',transport:'streamable',status:'disconnected',toolCount:0,enabled:true}];
    mockGetConfig.mockResolvedValue({name:'Device',transport:'streamable',url:'https://old.example/mcp',auth_type:'oauth2_pkce',oauth_managed:true,oauth2_device_auth_url:'https://old.example/device',enabled:true});
    render(<McpPage />);
    const buttons = await screen.findAllByRole('button',{name:'mcp.actions.edit'});
    await userEvent.click(buttons[buttons.length-1]);
    await waitFor(() => expect(screen.getByLabelText('Server URL')).toHaveValue('https://old.example/mcp'));
    if (mode !== 'rename') fireEvent.change(screen.getByLabelText('Server URL'),{target:{value:'https://new.example/mcp'}});
    else fireEvent.change(screen.getByLabelText('Nome'),{target:{value:'Renamed'}});
    if (mode === 'revert') fireEvent.change(screen.getByLabelText('Server URL'),{target:{value:'https://old.example/mcp'}});
    await userEvent.click(screen.getByText('Salvar'));
    await waitFor(() => expect(mockSave).toHaveBeenCalled());
    const [,config]=mockSave.mock.calls[mockSave.mock.calls.length-1];
    expect(config.oauth2_device_auth_url).toBe(mode === 'change' ? undefined : 'https://old.example/device');
  });

});

it('mostra erro OAuth localizado sem toast de sucesso quando Conectar falha', async () => {
 mockServers = [{slug:'managed', name:'Managed', status:'disconnected', enabled:true, transport:'streamable', authType:'oauth2_pkce', tools:[]}];
 mockToast.mockClear();
 mockConnect.mockRejectedValueOnce(new Error('oauth_consent_declined'));
 render(<McpPage />);
 const row = screen.getByText('Managed').closest('div');
 if (!row) throw new Error('Linha do servidor ausente');
 await userEvent.click(within(row).getByRole('button', {name:'mcp.actions.connect'}));
 await waitFor(() => expect(mockToast).toHaveBeenCalledWith('mcp.error.consentDeclined','error'));
 expect(mockToast.mock.calls.some((call) => call[1] === 'success')).toBe(false);
});

it('mantém vínculo DCR em modo manual e ao acrescentar espaços no mesmo ID', async () => {
 mockServers = [{slug:'dcr',name:'Saved DCR',status:'disconnected',enabled:true,transport:'streamable',authType:'oauth2_pkce',tools:[]}];
 mockGetConfig.mockResolvedValue({slug:'dcr',name:'Saved DCR',transport:'streamable',url:'https://example.com/mcp',auth_type:'oauth2_pkce',oauth_managed:true,oauth2_client_id:'registered',oauth2_client_method:'dcr'});
 render(<McpPage />);
 const row = screen.getByText('Saved DCR').closest('div');
 if (!row) throw new Error('Linha ausente');
 await userEvent.click(within(row).getByRole('button',{name:'mcp.actions.edit'}));
 await screen.findByLabelText('Client ID');
 await userEvent.click(screen.getByRole('button',{name:'Configurar manualmente'}));
 fireEvent.change(screen.getByLabelText('Client ID'),{target:{value:' registered '}});
 expect(screen.getByTestId('dcr-registered')).toHaveTextContent('true');
 fireEvent.change(screen.getByLabelText('Client ID'),{target:{value:'manual-client'}});
 expect(screen.getByTestId('dcr-registered')).toHaveTextContent('false');
});
