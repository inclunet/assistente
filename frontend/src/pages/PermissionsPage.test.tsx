import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent } from '@testing-library/react';
import { axe } from '../test/a11yAxe';

const mockGetPermissions = vi.fn();
const mockAddToast = vi.fn();
const mockRevoke = vi.fn();
const mockConfirm = vi.fn();
const mockAnnounce = vi.fn();
const mockAnnounceRequest = vi.fn();
const tStable = (key: string, fb?: string) => (typeof fb === 'string' ? fb : key);

vi.mock('@wailsjs/go/wailsapi/Permissions', () => ({
  GetPermissions: (...args: unknown[]) => mockGetPermissions(...args),
  RevokePermission: (...args: unknown[]) => mockRevoke(...args), GetPermissionMigration: vi.fn(), AssignPermissionOrigin: vi.fn(),
}));

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: tStable,
    i18n: { language: 'pt-BR', changeLanguage: vi.fn() },
  }),
}));

vi.mock('../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: mockAnnounce, announceRequest: mockAnnounceRequest }),
  announce: (...args: unknown[]) => mockAnnounce(...args),
}));
vi.mock('../hooks/useConfirm', () => ({ useConfirm: () => mockConfirm }));
vi.mock('../hooks/useGridFocus', () => ({ useGridFocus: () => ({ handleGridReady: vi.fn() }) }));
vi.mock('../hooks/useGridPageLandmarks', () => ({ useGridPageLandmarks: vi.fn() }));
vi.mock('../services/audioFeedback', () => ({ playBumpSound: vi.fn() }));
vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: typeof mockAddToast }) => unknown) =>
    selector({ addToast: mockAddToast }),
}));
vi.mock('../components/ui/PageLoading', () => ({
  PageLoading: ({ message }: { message: string }) => <div>{message}</div>,
}));

vi.mock('./AllowlistPage', () => ({ default: () => <div /> }));
vi.mock('./ProfilesPage', () => ({ default: () => <div /> }));
vi.mock('../store/authStore', () => ({ useAuthStore: (selector: (state: { user: null }) => unknown) => selector({ user: null }) }));
import PermissionsPage from './PermissionsPage';

const entradaDeWorkspace = { id:'network-rule', domain:'network', resource:'api.internal', operation:'connect', scope:'workspace', scopeKey:'project', effect:'allow', origin:'consent', createdAt:'2026-08-01T10:00:00Z', revokedAt:'' };
describe('PermissionsPage — acessibilidade', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('a tela com hosts autorizados não tem violações', async () => {
    mockGetPermissions.mockResolvedValue({rules:[entradaDeWorkspace],migrationErrors:[]});

    const { container } = render(<PermissionsPage />);
    await waitFor(() => expect(screen.getByRole('grid')).toBeInTheDocument());

    expect(await axe(container)).toHaveNoViolations();
  });

  it('a tela sem host autorizado não tem violações', async () => {
    mockGetPermissions.mockResolvedValue({rules:[],migrationErrors:[]});

    const { container } = render(<PermissionsPage />);
    await waitFor(() => {
      expect(screen.getByText('Nenhum item para exibir')).toBeInTheDocument();
    });

    expect(await axe(container)).toHaveNoViolations();
  });
});

describe('PermissionsPage — gestão', () => {
  beforeEach(() => { vi.clearAllMocks(); mockConfirm.mockResolvedValue(true); mockRevoke.mockResolvedValue(undefined); });
  it('filtra por domínio e escopo sem esconder a identidade do contexto', async () => {
    mockGetPermissions.mockResolvedValue({ rules: [entradaDeWorkspace, { ...entradaDeWorkspace, id:'session-rule', resource:'temporary.internal', scope:'session', scopeKey:'conversation-123' }], migrationErrors:[] });
    render(<PermissionsPage />);
    await screen.findByText('temporary.internal');
    fireEvent.change(screen.getByLabelText('permissions.scopeLabel'), { target: { value:'session' } });
    expect(screen.queryByText('api.internal')).not.toBeInTheDocument();
    expect(screen.getByText('conversation-123')).toBeInTheDocument();
  });
  it('informa falha de leitura sem dizer que a lista está vazia', async () => {
    mockGetPermissions.mockRejectedValue(new Error('db unavailable'));
    render(<PermissionsPage />);
    await screen.findByText('permissions.loadFailed');
    expect(screen.queryByText('Nenhum item para exibir')).not.toBeInTheDocument();
  });
  it('expõe origens pendentes junto às regras já conhecidas', async () => {
    mockGetPermissions.mockResolvedValue({ rules:[entradaDeWorkspace], migrationErrors:['pending-source'] });
    render(<PermissionsPage />);
    await screen.findByText('api.internal');
    expect(screen.getByText('permissions.incomplete')).toBeInTheDocument();
    expect(screen.getByText('pending-source')).toBeInTheDocument();
  });
  it('revoga a identidade da linha após a confirmação', async () => {
    mockGetPermissions.mockResolvedValueOnce({rules:[entradaDeWorkspace],migrationErrors:[]}).mockResolvedValue({rules:[],migrationErrors:[]});
    render(<PermissionsPage />);
    await screen.findByText('api.internal');
    const cell = screen.getByText('api.internal').closest('[role="gridcell"]');
    expect(cell).not.toBeNull();
    fireEvent.focus(cell!);
    const button = screen.getByRole('button', { name:'permissions.revoke' });
    await waitFor(() => expect(button).toBeEnabled());
    fireEvent.click(button);
    await waitFor(() => expect(mockRevoke).toHaveBeenCalledWith('network-rule'));
    expect(mockConfirm).toHaveBeenCalledOnce();
  });
});
