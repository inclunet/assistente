import type { ChangeEvent, ReactNode, KeyboardEventHandler, FocusEventHandler } from 'react';
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockList = vi.fn();
let mockLanguage = 'pt-BR';
const mockManagedList = vi.fn();
const mockNavigate = vi.fn();
const mockManagedAction = vi.fn();
const mockExecuteDeepLink = vi.fn();
vi.mock('react-router-dom', async (importOriginal) => ({ ...await importOriginal<typeof import('react-router-dom')>(), useNavigate: () => mockNavigate }));
vi.mock('../lib/deepLinks', () => ({ executeDeepLink: (...args: unknown[]) => mockExecuteDeepLink(...args) }));
vi.mock('../components/credentials/managedCredential', () => ({ managedCredentialAction: (...args: unknown[]) => mockManagedAction(...args) }));
const mockUpsert = vi.fn();
const mockDelete = vi.fn();
const mockListExternalSources = vi.fn();
const mockAnnounce = vi.fn();
const mockConfirm = vi.fn();

vi.mock('../hooks/useConfirm', () => ({
  useConfirm: () => mockConfirm,
}));

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) => {
      if (key === 'credentials.workflow.stored') return mockLanguage === 'pt-BR' ? 'Registro armazenado' : 'Stored record';
      const value =
      ({
        'credentials.pageTitle': 'Credenciais',
        'credentials.buttons.new': 'Nova',
        'credentials.buttons.edit': 'Editar',
        'credentials.buttons.duplicate': 'Duplicar',
        'credentials.buttons.delete': 'Excluir',
        'credentials.buttons.create': 'Criar',
        'credentials.labels.pattern': 'Pattern',
        'credentials.labels.type': 'Tipo',
        'credentials.labels.value': 'Valor',
        'credentials.labels.username': 'Usuário',
        'credentials.labels.password': 'Senha',
        'credentials.labels.header': 'Header',
        'credentials.labels.token': 'Token',
 'credentials.sourceFields.source': 'Fonte',
 'credentials.sourceFields.envName': 'Variável',
 'credentials.sourceFields.keyringName': 'Target',
        'credentials.modal.newTitle': 'Nova credencial',
        'credentials.modal.editTitle': 'Editar credencial',
        'credentials.placeholders.pattern': 'ex: *.github.com ou channel:slack:bot_token',
        'credentials.placeholders.token': 'Informe o token',
        'credentials.placeholders.token_ref': 'Token, keyring://service/user ou env://VAR',
        'credentials.aria.suggestions': 'Sugestões de referência',
        'credentials.aria.suggestionsAvailable': '{{count}} sugestões disponíveis',
        'credentials.hint.sensitive':
          'Os valores sensíveis não são exibidos após salvar. Para atualizar, informe novamente.',
        'credentials.types.bearer': 'Bearer token',
        'credentials.types.basic': 'Basic (usuário/senha)',
        'credentials.types.custom': 'Header customizado',
        'credentials.types.secret': 'Segredo (uso interno)',
        'credentials.aria.toolbar': 'Barra de ferramentas de credenciais',
        'credentials.labels.origin': 'Origem',
        'credentials.origin.system': 'Sistema',
        'credentials.origin.manual': 'Manual',
        'credentials.buttons.view': 'Visualizar',
        'credentials.modal.viewTitle': 'Credencial do sistema',
        'credentials.managed.badge': 'Gerenciada pelo sistema',
        'credentials.managed.description': 'Esta credencial é gerenciada automaticamente.',
        'common.cancel': 'Cancelar',
        'common.save': 'Salvar',
        'common.close': 'Fechar',
      } as Record<string, string>)[key] ?? key;
      return typeof opts?.count !== 'undefined'
        ? value.replace('{{count}}', String(opts.count))
        : value;
    },
  }),
}));

vi.mock('@wailsjs/go/wailsapi/Credentials', () => ({
  ListCredentials: () => mockList(),
 ListManagedCredentials: () => mockManagedList(),
  UpsertCredential: (payload: unknown) => mockUpsert(payload),
  DeleteCredential: (pattern: string) => mockDelete(pattern),
  ListExternalSources: (prefix: string) => mockListExternalSources(prefix),
}));

vi.mock('../hooks/useGridFocus', () => ({
  useGridFocus: () => ({
    handleGridReady: vi.fn(),
  }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({
    announce: mockAnnounce,
    announceRequest: vi.fn(),
  }),
}));

vi.mock('../store/uiStore', () => ({
  useUIStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    const s = { addToast: vi.fn() };
    return selector ? selector(s) : s;
  },
}));

vi.mock('../components/ui/Toolbar', () => ({
  Toolbar: ({ left, right, actions }: { left?: ReactNode; right?: ReactNode; actions?: Array<{ key: string; label: string; onClick?: () => void }> }) => (
    <div>
      {left}
      {right}
      <div>
        {actions?.map((action) => (
          <button key={action.key} onClick={action.onClick}>
            {action.label}
          </button>
        ))}
      </div>
    </div>
  ),
}));

vi.mock('../components/ui/DataGrid', () => ({
  DataGrid: ({
    items,
    onActivate,
    getRowActions,
  }: {
    items?: Array<{ id: string; pattern: string }>;
    onActivate?: (row: { id: string; pattern: string }) => void;
    getRowActions?: (row: { id: string; pattern: string }) => Array<{ id: string; label?: string; onClick?: () => void }>;
  }) => (
    <div>
      {items?.map((row) => (
        <div key={row.id}>
          <button onClick={() => onActivate?.(row)}>{row.pattern}</button>
          {getRowActions?.(row)?.map((action) => (
            <button key={action.id} onClick={action.onClick}>{action.label}</button>
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

vi.mock('../components/ui/EditorPanel', () => ({
  EditorPanelFooter: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
}));

vi.mock('../components', () => ({
  Button: ({ children, onClick }: { children?: ReactNode; onClick?: () => void }) => <button onClick={onClick}>{children}</button>,
  Input: ({ label, value, onChange, type, onKeyDown, onBlur, ...rest }: { label: string; value: string; onChange: (event: ChangeEvent<HTMLInputElement>) => void; type?: string; onKeyDown?: KeyboardEventHandler<HTMLInputElement>; onBlur?: FocusEventHandler<HTMLInputElement>; [key: string]: unknown }) => (
    <label>
      {label}
      <input aria-label={label} value={value} onChange={onChange} type={type} onKeyDown={onKeyDown} onBlur={onBlur} onFocus={rest.onFocus as FocusEventHandler<HTMLInputElement>} role={rest.role as string} aria-expanded={rest['aria-expanded'] as boolean} aria-controls={rest['aria-controls'] as string} aria-activedescendant={rest['aria-activedescendant'] as string} aria-autocomplete={rest['aria-autocomplete'] as 'list' | 'none' | 'inline' | 'both' | undefined} />
    </label>
  ),
  Select: ({ label, value, options, onChange }: { label: string; value: string; options: Array<{ value: string; label: string }>; onChange: (event: ChangeEvent<HTMLSelectElement>) => void }) => (
    <label>
      {label}
      <select aria-label={label} value={value} onChange={onChange}>
        {options.map((opt) => (
          <option key={opt.value} value={opt.value}>{opt.label}</option>
        ))}
      </select>
    </label>
  ),
}));

import CredentialsPage from './CredentialsPage';
import { useAuthStore } from '../store/authStore';
import { useNavigationStore } from '../store/navigationStore';

describe('CredentialsPage', () => {
  beforeEach(() => {
    mockList.mockResolvedValue([
      { pattern: '*.github.com', type: 'bearer', masked: '••••1234', managed: false },
    ]);
    mockUpsert.mockResolvedValue(undefined);
    mockDelete.mockResolvedValue(undefined);
    mockManagedList.mockResolvedValue([]);
    mockListExternalSources.mockResolvedValue([]);
    mockAnnounce.mockClear();
    mockConfirm.mockReset();
    mockConfirm.mockResolvedValue(true);
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  it('carrega credenciais e abre editor', async () => {
    render(<CredentialsPage />);

    await waitFor(() => {
      expect(screen.getByText('*.github.com')).toBeInTheDocument();
    });

    await userEvent.click(screen.getByText('*.github.com'));

    expect(screen.getByLabelText('Pattern')).toBeInTheDocument();
    expect(screen.getByLabelText('Tipo')).toBeInTheDocument();
  });

  it('cria nova credencial', async () => {
    render(<CredentialsPage />);

    await userEvent.click(screen.getByText('Nova'));

    await userEvent.type(screen.getByLabelText('Pattern'), 'api.example.com');
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'bearer');
    await userEvent.type(screen.getByLabelText('Token'), 'tok_123');

    await userEvent.click(screen.getByText('Criar'));

    expect(mockUpsert).toHaveBeenCalledWith(expect.objectContaining({
      pattern: 'api.example.com',
      type: 'bearer',
      token: 'tok_123',
    }));
  });

  it('deep link pré-preenche pattern e tipo na criação', async () => {
    useNavigationStore.setState({
      pendingEdit: {
        resource: 'credentials',
        id: '',
        action: 'new',
        initial: { pattern: 'api.search.brave.com', type: 'bearer' },
        timestamp: Date.now(),
      },
    });

    render(<CredentialsPage />);

    await waitFor(() => {
      expect(screen.getByLabelText('Pattern')).toHaveValue('api.search.brave.com');
    });
    expect(screen.getByLabelText('Tipo')).toHaveValue('bearer');
    // Token continua vazio: segredo nunca vem por deep link.
    expect(screen.getByLabelText('Token')).toHaveValue('');
  });

  it('deep link abre o modal mesmo sem credenciais cadastradas', async () => {
    mockList.mockResolvedValue([]);
    useNavigationStore.setState({
      pendingEdit: {
        resource: 'credentials',
        id: '',
        action: 'new',
        initial: { pattern: 'api.tavily.com', type: 'bearer' },
        timestamp: Date.now(),
      },
    });

    render(<CredentialsPage />);

    await waitFor(() => {
      expect(screen.getByLabelText('Pattern')).toHaveValue('api.tavily.com');
    });
  });

  it('exclui credencial via menu de acoes', async () => {
    render(<CredentialsPage />);

    await waitFor(() => {
      expect(screen.getByText('*.github.com')).toBeInTheDocument();
    });

    const deleteButtons = screen.getAllByRole('button', { name: 'Excluir' });
    await userEvent.click(deleteButtons[deleteButtons.length - 1]);

    expect(mockDelete).toHaveBeenCalledWith('*.github.com');
  });

  it('credencial gerenciada mostra Visualizar em vez de Editar/Excluir', async () => {
    mockList.mockResolvedValue([
      { pattern: 'mcp-client:atlassian', type: 'oauth2', masked: '••••abcd', managed: true },
    ]);

    render(<CredentialsPage />);

    await waitFor(() => {
      expect(screen.getByText('mcp-client:atlassian')).toBeInTheDocument();
    });

    expect(screen.getByText('Visualizar')).toBeInTheDocument();
    expect(screen.queryAllByRole('button', { name: 'Excluir' }).filter(
      (btn) => btn.closest('[data-row]') !== null
    )).toHaveLength(0);
  });

  it('credencial gerenciada abre modal de visualizacao ao clicar', async () => {
    mockList.mockResolvedValue([
      { pattern: 'mcp-tokens:my-server', type: 'oauth2', masked: '••••xyz', managed: true },
    ]);

    render(<CredentialsPage />);

    await waitFor(() => {
      expect(screen.getByText('mcp-tokens:my-server')).toBeInTheDocument();
    });

    await userEvent.click(screen.getByText('mcp-tokens:my-server'));

    expect(screen.getByText('Gerenciada pelo sistema')).toBeInTheDocument();
    expect(screen.getByText('Fechar')).toBeInTheDocument();
  });


  it('salva command com argumentos estruturados e não duplica Basic', async () => {
    render(<CredentialsPage />);
    await userEvent.click(screen.getByText('Nova'));
    await userEvent.type(screen.getByLabelText('Pattern'), 'command.example');
    await userEvent.selectOptions(screen.getByLabelText('Fonte'), 'command');
    await userEvent.type(screen.getByLabelText('credentials.sourceFields.commandName'), 'wsl.exe');
    const args = screen.getByLabelText('credentials.sourceFields.args');
    await userEvent.clear(args);
    await userEvent.click(args);
    await userEvent.paste('["--", "nu", "genai", "ai-gateway", "token"]');
    await userEvent.click(screen.getByText('Criar'));
    expect(mockUpsert).toHaveBeenCalledWith(expect.objectContaining({ source: 'command', sourceConfig: expect.objectContaining({ command: 'wsl.exe', args: ['--', 'nu', 'genai', 'ai-gateway', 'token'] }) }));
    await userEvent.click(screen.getByText('Nova'));
    await userEvent.selectOptions(screen.getByLabelText('Tipo'), 'basic');
    expect(screen.getAllByLabelText('Usuário')).toHaveLength(1);
    expect(screen.getAllByLabelText('Senha')).toHaveLength(1);
    await userEvent.selectOptions(screen.getByLabelText('Fonte'), 'env');
    expect(screen.getAllByLabelText('Usuário')).toHaveLength(1);
    expect(screen.queryByLabelText('Senha')).not.toBeInTheDocument();
  });

  it('carrega configuração command para edição sem materializar segredo', async () => {
    mockList.mockResolvedValue([{ pattern: 'command.example', type: 'bearer', source: 'command', sourceConfig: { command: 'nu', args: ['genai', 'token'], timeoutSeconds: 12 }, masked: 'command', managed: false }]);
    render(<CredentialsPage />);
    await userEvent.click(await screen.findByText('command.example'));
    expect(screen.getByLabelText('credentials.sourceFields.commandName')).toHaveValue('nu');
    expect(screen.getByText('credentials.sourceFields.cacheHint')).toBeInTheDocument();
    expect(screen.getByLabelText('credentials.sourceFields.args')).toHaveValue('["genai","token"]');
    expect(screen.queryByLabelText('Token')).not.toBeInTheDocument();
  });

  describe('autocomplete de referências externas', () => {
    const keyringResults = [
      { value: 'github-token', label: 'github-token' },
      { value: 'aws-secret', label: 'aws-secret' },
    ];

    beforeEach(() => {
      mockListExternalSources.mockImplementation((prefix: string) => {
        if (prefix === 'keyring') return Promise.resolve(keyringResults);
        if (prefix === 'env') return Promise.resolve([{ value: 'HOME', label: 'HOME' }]);
        return Promise.resolve([]);
      });
    });

    it('mostra sugestões ao selecionar a fonte keyring', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      await userEvent.selectOptions(screen.getByLabelText('Fonte'), 'keyring');
      const tokenInput = screen.getByLabelText('Target') as HTMLInputElement;
      await userEvent.click(tokenInput);

      await waitFor(() => {
        expect(screen.getByRole('listbox')).toBeInTheDocument();
      });

      expect(screen.getByText('github-token')).toBeInTheDocument();
      expect(screen.getByText('aws-secret')).toBeInTheDocument();
    });

    it('anuncia a quantidade de sugestões via announcer global', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      await userEvent.selectOptions(screen.getByLabelText('Fonte'), 'keyring');
      const tokenInput = screen.getByLabelText('Target') as HTMLInputElement;
      await userEvent.click(tokenInput);

      await waitFor(() => {
        expect(mockAnnounce).toHaveBeenCalledWith('2 sugestões disponíveis');
      });
    });

    it('seleciona sugestão com Enter após navegar com seta', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      await userEvent.selectOptions(screen.getByLabelText('Fonte'), 'keyring');
      const tokenInput = screen.getByLabelText('Target') as HTMLInputElement;
      await userEvent.click(tokenInput);

      await waitFor(() => {
        expect(screen.getByRole('listbox')).toBeInTheDocument();
      });

      await userEvent.keyboard('{ArrowDown}');
      await userEvent.keyboard('{Enter}');

      expect((tokenInput as HTMLInputElement).value).toBe('github-token');
    });

    it('fecha sugestões com Escape', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      await userEvent.selectOptions(screen.getByLabelText('Fonte'), 'keyring');
      const tokenInput = screen.getByLabelText('Target') as HTMLInputElement;
      await userEvent.click(tokenInput);

      await waitFor(() => {
        expect(screen.getByRole('listbox')).toBeInTheDocument();
      });

      await userEvent.keyboard('{Escape}');

      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    });

    it('seleciona sugestão com mouse', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      await userEvent.selectOptions(screen.getByLabelText('Fonte'), 'keyring');
      const tokenInput = screen.getByLabelText('Target') as HTMLInputElement;
      await userEvent.click(tokenInput);

      await waitFor(() => {
        expect(screen.getByRole('listbox')).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText('aws-secret'));

      expect((tokenInput as HTMLInputElement).value).toBe('aws-secret');
      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    });

    it('mostra e seleciona sugestões ao selecionar a fonte env', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      await userEvent.selectOptions(screen.getByLabelText('Fonte'), 'env');
      const tokenInput = screen.getByLabelText('Variável') as HTMLInputElement;
      await userEvent.click(tokenInput);

      await waitFor(() => {
        expect(screen.getByRole('listbox')).toBeInTheDocument();
      });

      expect(mockListExternalSources).toHaveBeenCalledWith('env');
      expect(screen.getByText('HOME')).toBeInTheDocument();

      await userEvent.click(screen.getByText('HOME'));

      expect(tokenInput.value).toBe('HOME');
      expect(tokenInput.type).toBe('text');
      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    });

    it('campo muda de password para text quando a fonte selecionada é externa', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      expect((screen.getByLabelText('Token') as HTMLInputElement).type).toBe('password');
      await userEvent.selectOptions(screen.getByLabelText('Fonte'), 'keyring');
      const tokenInput = screen.getByLabelText('Target') as HTMLInputElement;
      await userEvent.click(tokenInput);

      await waitFor(() => {
        expect(screen.getByRole('listbox')).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText('github-token'));

      expect(tokenInput.type).toBe('text');
      expect(tokenInput.value).toBe('github-token');
    });
  });
});


describe('autorizações compostas', () => {
  const authorization = { id: 'auth-id', pattern: 'oauth:auth-id', source: 'oauth', integration: 'mcp', consumerId: 'server', state: 'connected', unreadable: false };
  beforeEach(() => {
    mockList.mockResolvedValue([]);
    mockManagedList.mockResolvedValue([authorization]);
    mockManagedAction.mockReset();
    mockExecuteDeepLink.mockReset();
    mockUpsert.mockClear();
    mockDelete.mockClear();
  });
  it('atualiza a tradução do estado sem reconsultar', async () => {
    mockLanguage = 'pt-BR';
    const { rerender } = render(<CredentialsPage />);
    await userEvent.click(await screen.findByText('oauth:auth-id'));
    expect(screen.getByLabelText('Valor')).toHaveValue('Registro armazenado');
    const calls = mockManagedList.mock.calls.length;
    mockLanguage = 'en';
    rerender(<CredentialsPage />);
    expect(screen.getByLabelText('Valor')).toHaveValue('Stored record');
    expect(mockManagedList).toHaveBeenCalledTimes(calls);
    mockLanguage = 'pt-BR';
  });
  it('descarta navegação quando a sessão muda', async () => {
    let resolve!: (value: unknown) => void;
    mockManagedAction.mockReturnValue(new Promise((done) => { resolve = done; }));
    render(<CredentialsPage />);
    await userEvent.click(await screen.findByText('oauth:auth-id'));
    await userEvent.click(screen.getByText('credentials.workflow.configure'));
    act(() => useAuthStore.setState({ user: { userId: 'other', sessionId: 'other-session', role: 'user' } }));
    await act(async () => resolve({ type: 'resource:edit', resource: 'mcp', resourceId: 'slack' }));
    expect(mockExecuteDeepLink).not.toHaveBeenCalled();
    act(() => useAuthStore.setState({ user: null }));
  });
  it('criação OAuth encaminha para MCP sem persistir uma credencial vazia', async () => {
    render(<CredentialsPage />);
    await userEvent.click(screen.getByText('Nova'));
    await userEvent.selectOptions(screen.getByLabelText('Fonte'), 'oauth');
    await userEvent.click(screen.getByText('credentials.workflow.newMcp'));
    expect(mockExecuteDeepLink).toHaveBeenCalledWith({ type: 'resource:new', resource: 'mcp' }, { navigate: mockNavigate });
    expect(mockUpsert).not.toHaveBeenCalled();
  });
  it('identifica conexão estática ilegível sem inventar uma integração OAuth', async () => {
    mockManagedList.mockResolvedValue([{ ...authorization, pattern: 'connection:broken', source: 'static', integration: '', consumerId: '', unreadable: true }]);
    render(<CredentialsPage />);
    await userEvent.click(await screen.findByText('connection:broken'));
    expect(screen.getByLabelText('Tipo')).toHaveValue('credentials.workflow.staticConnection');
    expect(screen.queryByText('credentials.workflow.configure')).not.toBeInTheDocument();
  });
  it('abre o fluxo vinculado sem usar o CRUD genérico', async () => {
    const action = { type: 'resource:edit', resource: 'mcp', resourceId: 'slack' };
    mockManagedAction.mockResolvedValue(action);
    render(<CredentialsPage />);
    await userEvent.click(await screen.findByText('oauth:auth-id'));
    await userEvent.click(screen.getByText('credentials.workflow.configure'));
    await waitFor(() => expect(mockExecuteDeepLink).toHaveBeenCalledWith(action, { navigate: mockNavigate }));
    expect(mockUpsert).not.toHaveBeenCalled();
    expect(mockDelete).not.toHaveBeenCalled();
  });
  it('mostra erro seguro se o vínculo deixou de existir', async () => {
    mockManagedAction.mockRejectedValue(new Error('private backend error'));
    render(<CredentialsPage />);
    await userEvent.click(await screen.findByText('oauth:auth-id'));
    await userEvent.click(screen.getByText('credentials.workflow.configure'));
    expect(await screen.findByText('credentials.workflow.unavailable')).toBeInTheDocument();
    expect(screen.queryByText('private backend error')).not.toBeInTheDocument();
    expect(mockExecuteDeepLink).not.toHaveBeenCalled();
  });
  it('descarta abertura pendente após fechar', async () => {
    let resolve!: (value: unknown) => void;
    mockManagedAction.mockReturnValue(new Promise((done) => { resolve = done; }));
    render(<CredentialsPage />);
    await userEvent.click(await screen.findByText('oauth:auth-id'));
    await userEvent.click(screen.getByText('credentials.workflow.configure'));
    await userEvent.click(screen.getByText('Fechar'));
    resolve({ type: 'resource:edit', resource: 'mcp', resourceId: 'slack' });
    await waitFor(() => expect(screen.queryByText('credentials.workflow.configure')).not.toBeInTheDocument());
    expect(mockExecuteDeepLink).not.toHaveBeenCalled();
  });
});
