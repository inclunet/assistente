import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockSave = vi.fn();
const mockGetConfig = vi.fn();
const mockLoadServers = vi.fn();

vi.mock('../store/mcpStore', () => ({
  useMCPStore: () => ({
    servers: [],
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

vi.mock('@wailsjs/go/main/App', () => ({
  SaveMCPServerAuth: vi.fn(() => Promise.resolve()),
  DeleteMCPServerAuth: vi.fn(() => Promise.resolve()),
  GetMCPServerAuthInfo: vi.fn(() => Promise.resolve({ hasAuth: false })),
  DiscoverMCPServerAuth: vi.fn(() => Promise.resolve({ found: false })),
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    focusFirstCell: vi.fn(),
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
  useUIStore: () => ({
    addToast: vi.fn(),
  }),
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ actions }: any) => (
    <div>
      {actions?.map((a: any) => (
        <button key={a.key} onClick={a.onClick}>{a.label}</button>
      ))}
    </div>
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: () => <div />,
}));

vi.mock('../components/ui/Modal', () => ({
  Modal: ({ isOpen, children }: any) => (isOpen ? <div role="dialog">{children}</div> : null),
}));

vi.mock('../components/ui/EditorPanel', () => ({
  EditorPanelFooter: ({ children }: any) => <div>{children}</div>,
}));

vi.mock('../components', () => ({
  Button: ({ children, onClick }: any) => <button onClick={onClick}>{children}</button>,
}));

vi.mock('../components/mcp/McpGeneralSection', () => ({
  McpGeneralSection: ({ isNew, slug, name, transport, onSlugChange, onNameChange, onTransportChange }: any) => (
    <div>
      {isNew && (
        <label>
          Slug
          <input aria-label="Slug" value={slug} onChange={(e) => onSlugChange(e.target.value)} />
        </label>
      )}
      <label>
        Nome
        <input aria-label="Nome" value={name} onChange={(e) => onNameChange(e.target.value)} />
      </label>
      <label>
        Transporte
        <select aria-label="Transporte" value={transport} onChange={(e) => onTransportChange(e.target.value)}>
          <option value="stdio">STDIO</option>
          <option value="streamable">Streamable</option>
          <option value="sse">SSE</option>
        </select>
      </label>
    </div>
  ),
}));

vi.mock('../components/mcp/McpConnectionSection', () => ({
  McpConnectionSection: (props: any) => (
    <div data-testid="connection-section">
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
    </div>
  ),
}));

import McpPage from './McpPage';

describe('McpPage — oauth2_callback_host', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSave.mockResolvedValue(undefined);
    mockLoadServers.mockResolvedValue(undefined);
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

    await userEvent.type(screen.getByLabelText('Slug'), 'test-server');
    await userEvent.type(screen.getByLabelText('Nome'), 'Test Server');
    await userEvent.selectOptions(screen.getByLabelText('Transporte'), 'streamable');
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

    await userEvent.type(screen.getByLabelText('Slug'), 'no-pkce');
    await userEvent.type(screen.getByLabelText('Nome'), 'No PKCE');
    await userEvent.selectOptions(screen.getByLabelText('Transporte'), 'streamable');

    await userEvent.click(screen.getByText('Salvar'));

    await waitFor(() => {
      expect(mockSave).toHaveBeenCalledTimes(1);
    });

    const [, config] = mockSave.mock.calls[0];
    expect(config.oauth2_callback_host).toBeUndefined();
  });

  it('não inclui oauth2_callback_host quando vazio (usa default do backend)', async () => {
    await openNewServerForm();

    await userEvent.type(screen.getByLabelText('Slug'), 'default-host');
    await userEvent.type(screen.getByLabelText('Nome'), 'Default Host');
    await userEvent.selectOptions(screen.getByLabelText('Transporte'), 'streamable');
    await userEvent.selectOptions(screen.getByLabelText('Auth Type'), 'oauth2_pkce');

    await userEvent.click(screen.getByText('Salvar'));

    await waitFor(() => {
      expect(mockSave).toHaveBeenCalledTimes(1);
    });

    const [, config] = mockSave.mock.calls[0];
    expect(config.oauth2_callback_host).toBeUndefined();
  });
});
