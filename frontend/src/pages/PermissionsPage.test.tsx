import { describe, expect, it, vi, beforeEach } from 'vitest';
import { render, screen, waitFor, fireEvent, act } from '@testing-library/react';
import { axe } from '../test/a11yAxe';
import { useNavigationStore } from '../store/navigationStore';
import { MemoryRouter } from 'react-router-dom';

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
vi.mock('../hooks/useGridPageLandmarks', () => ({ useGridPageLandmarks: vi.fn() }));
vi.mock('../services/audioFeedback', () => ({ playBumpSound: vi.fn() }));
vi.mock('../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: typeof mockAddToast }) => unknown) =>
    selector({ addToast: mockAddToast }),
}));
vi.mock('../components/ui/PageLoading', () => ({
  PageLoading: ({ message }: { message: string }) => <div>{message}</div>,
}));

vi.mock('./AllowlistPage', () => ({ default: () => <div>policies-editor</div> }));
vi.mock('../store/authStore', () => ({ useAuthStore: (selector: (state: { user: null }) => unknown) => selector({ user: null }) }));
import PermissionsPage from './PermissionsPage';

function renderPermissions() {
  return render(<MemoryRouter><PermissionsPage /></MemoryRouter>);
}

const entradaDeWorkspace = { id:'network-rule', domain:'network', resource:'api.internal', operation:'connect', scope:'workspace', scopeKey:'project', effect:'allow', origin:'consent', createdAt:'2026-08-01T10:00:00Z', revokedAt:'' };
describe('PermissionsPage — acessibilidade', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it('a tela com hosts autorizados não tem violações', async () => {
    mockGetPermissions.mockResolvedValue({rules:[entradaDeWorkspace],migrationErrors:[]});

    const { container } = renderPermissions();
    await waitFor(() => expect(screen.getByRole('grid')).toBeInTheDocument());

    expect(await axe(container)).toHaveNoViolations();
  });

  it('a tela sem host autorizado não tem violações', async () => {
    mockGetPermissions.mockResolvedValue({rules:[],migrationErrors:[]});

    const { container } = renderPermissions();
    await waitFor(() => {
      expect(screen.getByText('Nenhum item para exibir')).toBeInTheDocument();
    });

    expect(await axe(container)).toHaveNoViolations();
  });
});

describe('PermissionsPage — gestão', () => {
  beforeEach(() => { vi.clearAllMocks(); useNavigationStore.getState().clearPendingEdit(); mockConfirm.mockResolvedValue(true); mockRevoke.mockResolvedValue(undefined); });
  it('abre o editor de políticas para o pedido de deep link sem consumi-lo antes do editor', async () => {
    mockGetPermissions.mockResolvedValue({rules:[],migrationErrors:[]});
    useNavigationStore.getState().requestResourceEdit('allowlists', 'project-policy', 'edit');
    renderPermissions();
    await screen.findByText('policies-editor');
    expect(useNavigationStore.getState().pendingEdit?.id).toBe('project-policy');
    act(() => useNavigationStore.getState().clearPendingEdit());
  });
  it('filtra por domínio e escopo sem esconder a identidade do contexto', async () => {
    mockGetPermissions.mockResolvedValue({ rules: [entradaDeWorkspace, { ...entradaDeWorkspace, id:'session-rule', resource:'temporary.internal', scope:'session', scopeKey:'conversation-123' }], migrationErrors:[] });
    renderPermissions();
    await screen.findByText('temporary.internal');
    fireEvent.change(screen.getByLabelText('permissions.scopeLabel'), { target: { value:'session' } });
    expect(screen.queryByText('api.internal')).not.toBeInTheDocument();
    expect(screen.getByText('conversation-123')).toBeInTheDocument();
  });
  it('informa falha de leitura sem dizer que a lista está vazia', async () => {
    mockGetPermissions.mockRejectedValue(new Error('db unavailable'));
    renderPermissions();
    await screen.findByText('permissions.loadFailed');
    expect(screen.queryByText('Nenhum item para exibir')).not.toBeInTheDocument();
  });
  it('expõe origens pendentes junto às regras já conhecidas', async () => {
    mockGetPermissions.mockResolvedValue({ rules:[entradaDeWorkspace], migrationErrors:[{ code: 'owner_pending', domain: 'network', profile: 'padrao' }] });
    renderPermissions();
    await screen.findByText('api.internal');
    expect(screen.getByText('permissions.incomplete')).toBeInTheDocument();
    expect(screen.getByText('permissions.diagnostic.owner_pending')).toBeInTheDocument();
  });
  it('revoga a identidade da linha após a confirmação', async () => {
    mockGetPermissions.mockResolvedValueOnce({rules:[entradaDeWorkspace],migrationErrors:[]}).mockResolvedValue({rules:[],migrationErrors:[]});
    renderPermissions();
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
  it.each([true, false])('preserva foco após revogar e recarregar (linha restante: %s)', async (remaining) => {
    const next = { ...entradaDeWorkspace, id: 'next', resource: 'next.internal' };
    let finishRevoke!: () => void;
    let finishReload!: (value: unknown) => void;
    mockGetPermissions.mockResolvedValueOnce({ rules: [entradaDeWorkspace, ...(remaining ? [next] : [])], migrationErrors: [] })
      .mockImplementationOnce(() => new Promise((resolve) => { finishReload = resolve; }));
    mockRevoke.mockImplementationOnce(() => new Promise<void>((resolve) => { finishRevoke = resolve; }));
    renderPermissions();
    const cell = (await screen.findByText('api.internal')).closest('[role="row"]')!.querySelector<HTMLElement>('[role="gridcell"]')!;
    const grid = screen.getByRole('grid');
    act(() => cell.focus());
    fireEvent.contextMenu(cell);
    fireEvent.click(await screen.findByRole('menuitem', { name: 'permissions.revoke' }));
    await waitFor(() => expect(mockRevoke).toHaveBeenCalledWith('network-rule'));
    // Simula o retorno do foco da confirmação antes da resposta assíncrona.
    act(() => cell.focus());
    await act(async () => finishRevoke());
    await waitFor(() => expect(mockGetPermissions).toHaveBeenCalledTimes(2));
    const expected = remaining
      ? screen.getByText('next.internal').closest('[role="row"]')!.querySelector('[role="gridcell"]')
      : screen.getByRole('button', { name: 'permissions.policies' });
    await waitFor(() => expect(expected).toHaveFocus());
    if (remaining) expect(screen.getByRole('grid')).toBe(grid);
    else expect(screen.getByText('Nenhum item para exibir')).toBeInTheDocument();
    await act(async () => finishReload({ rules: remaining ? [next] : [], migrationErrors: [] }));
    await waitFor(() => expect(expected).toHaveFocus());
    if (remaining) expect(screen.getByRole('grid')).toBe(grid);
    else expect(screen.getByText('Nenhum item para exibir')).toBeInTheDocument();
  });
});
