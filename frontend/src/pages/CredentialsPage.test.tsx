import type { ChangeEvent, ReactNode, KeyboardEventHandler, FocusEventHandler } from 'react';
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest';
import type { MockInstance } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

const mockList = vi.fn();
const mockUpsert = vi.fn();
const mockDelete = vi.fn();
const mockListExternalSources = vi.fn();

vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init: () => {} },
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) => {
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
        'credentials.modal.newTitle': 'Nova credencial',
        'credentials.modal.editTitle': 'Editar credencial',
        'credentials.placeholders.pattern': 'ex: *.github.com ou channel:slack:bot_token',
        'credentials.placeholders.token': 'Informe o token',
        'credentials.placeholders.token_ref': 'Token, keyring://service/user ou env://VAR',
        'credentials.aria.suggestions': 'Sugestões de referência',
        'credentials.aria.suggestionsAvailable': '{{count}} sugestões disponíveis',
        'credentials.aria.noSuggestions': 'Nenhuma sugestão disponível',
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

vi.mock('@wailsjs/go/app/App', () => ({
  ListCredentials: () => mockList(),
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
    announce: vi.fn(),
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
      <input aria-label={label} value={value} onChange={onChange} type={type} onKeyDown={onKeyDown} onBlur={onBlur} role={rest.role as string} aria-expanded={rest['aria-expanded'] as boolean} aria-controls={rest['aria-controls'] as string} aria-activedescendant={rest['aria-activedescendant'] as string} aria-autocomplete={rest['aria-autocomplete'] as 'list' | 'none' | 'inline' | 'both' | undefined} />
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

describe('CredentialsPage', () => {
  let confirmSpy: MockInstance<(message?: string) => boolean>;

  beforeEach(() => {
    mockList.mockResolvedValue([
      { pattern: '*.github.com', type: 'bearer', masked: '••••1234', managed: false },
    ]);
    mockUpsert.mockResolvedValue(undefined);
    mockDelete.mockResolvedValue(undefined);
    mockListExternalSources.mockResolvedValue([]);
    confirmSpy = vi.spyOn(window, 'confirm').mockReturnValue(true);
  });

  afterEach(() => {
    confirmSpy.mockRestore();
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

  describe('autocomplete de referências externas', () => {
    const keyringResults = [
      { value: 'keyring://github-token', label: 'github-token' },
      { value: 'keyring://aws-secret', label: 'aws-secret' },
    ];

    beforeEach(() => {
      mockListExternalSources.mockImplementation((prefix: string) => {
        if (prefix === 'keyring://') return Promise.resolve(keyringResults);
        if (prefix === 'env://') return Promise.resolve([{ value: 'env://HOME', label: 'HOME' }]);
        return Promise.resolve([]);
      });
    });

    it('mostra sugestões ao digitar keyring://', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      const tokenInput = screen.getByLabelText('Token');
      await userEvent.clear(tokenInput);
      await userEvent.type(tokenInput, 'keyring://');

      await waitFor(() => {
        expect(screen.getByRole('listbox')).toBeInTheDocument();
      });

      expect(screen.getByText('github-token')).toBeInTheDocument();
      expect(screen.getByText('aws-secret')).toBeInTheDocument();
    });

    it('anuncia a quantidade de sugestões para leitores de tela (aria-live)', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      const tokenInput = screen.getByLabelText('Token');
      await userEvent.clear(tokenInput);
      await userEvent.type(tokenInput, 'keyring://');

      await waitFor(() => {
        expect(screen.getByRole('status')).toHaveTextContent('2 sugestões disponíveis');
      });
    });

    it('seleciona sugestão com Enter após navegar com seta', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      const tokenInput = screen.getByLabelText('Token');
      await userEvent.clear(tokenInput);
      await userEvent.type(tokenInput, 'keyring://');

      await waitFor(() => {
        expect(screen.getByRole('listbox')).toBeInTheDocument();
      });

      await userEvent.keyboard('{ArrowDown}');
      await userEvent.keyboard('{Enter}');

      expect((tokenInput as HTMLInputElement).value).toBe('keyring://github-token');
    });

    it('fecha sugestões com Escape', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      const tokenInput = screen.getByLabelText('Token');
      await userEvent.clear(tokenInput);
      await userEvent.type(tokenInput, 'keyring://');

      await waitFor(() => {
        expect(screen.getByRole('listbox')).toBeInTheDocument();
      });

      await userEvent.keyboard('{Escape}');

      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    });

    it('seleciona sugestão com mouse', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      const tokenInput = screen.getByLabelText('Token');
      await userEvent.clear(tokenInput);
      await userEvent.type(tokenInput, 'keyring://');

      await waitFor(() => {
        expect(screen.getByRole('listbox')).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText('aws-secret'));

      expect((tokenInput as HTMLInputElement).value).toBe('keyring://aws-secret');
      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    });

    it('campo muda de password para text quando valor é referência externa', async () => {
      render(<CredentialsPage />);
      await userEvent.click(screen.getByText('Nova'));

      const tokenInput = screen.getByLabelText('Token') as HTMLInputElement;
      expect(tokenInput.type).toBe('password');

      await userEvent.clear(tokenInput);
      await userEvent.type(tokenInput, 'keyring://');

      await waitFor(() => {
        expect(screen.getByRole('listbox')).toBeInTheDocument();
      });

      await userEvent.click(screen.getByText('github-token'));

      expect(tokenInput.type).toBe('text');
      expect(tokenInput.value).toBe('keyring://github-token');
    });
  });
});
