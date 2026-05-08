import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';

import { AuthGate } from './AuthGate';
import { useAuthStore } from '../../store/authStore';

const mockGetAuthStatus = vi.fn();
const mockRefreshAuth = vi.fn();
const mockSetupVault = vi.fn();
const mockUnlockVault = vi.fn();
const mockCreateAdminUser = vi.fn();
const mockLogin = vi.fn();
const mockLogout = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  GetAuthStatus: () => mockGetAuthStatus(),
  RefreshAuth: (payload: unknown) => mockRefreshAuth(payload),
  SetupVault: (password: string) => mockSetupVault(password),
  UnlockVault: (kind: string, secret: string) => mockUnlockVault(kind, secret),
  CreateAdminUser: (payload: unknown) => mockCreateAdminUser(payload),
  Login: (payload: unknown) => mockLogin(payload),
  Logout: (payload: unknown) => mockLogout(payload),
}));

describe('AuthGate', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    useAuthStore.setState({
      status: null,
      user: null,
      isLoading: false,
      error: null,
      isAuthenticated: false,
    });
  });

  it('não mostra inicialização do cofre quando o carregamento de status falha', async () => {
    mockGetAuthStatus.mockRejectedValueOnce(new Error('serviços de autenticação não inicializados'));

    render(
      <AuthGate>
        <div>Aplicação</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'Autenticação indisponível' })).toBeInTheDocument();
    expect(screen.getByText('serviços de autenticação não inicializados')).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Inicializar cofre' })).not.toBeInTheDocument();
    expect(mockSetupVault).not.toHaveBeenCalled();
  });

  it('mostra estado neutro enquanto o status inicial ainda não carregou', () => {
    mockGetAuthStatus.mockImplementationOnce(() => new Promise(() => undefined));

    render(
      <AuthGate>
        <div>Aplicação</div>
      </AuthGate>,
    );

    expect(screen.getByRole('heading', { name: 'Carregando autenticação' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'Inicializar cofre' })).not.toBeInTheDocument();
  });

  it('permite tentar carregar status novamente após falha', async () => {
    mockGetAuthStatus
      .mockRejectedValueOnce(new Error('falha temporária'))
      .mockResolvedValueOnce({ vaultConfigured: true, vaultUnlocked: false, hasUsers: true });

    render(
      <AuthGate>
        <div>Aplicação</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'Autenticação indisponível' })).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Tentar novamente' }));

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'Desbloquear cofre' })).toBeInTheDocument();
    });
    expect(screen.queryByRole('heading', { name: 'Inicializar cofre' })).not.toBeInTheDocument();
  });

  it('percorre setup do cofre, criação do admin e login local', async () => {
    mockGetAuthStatus
      .mockResolvedValueOnce({ vaultConfigured: false, vaultUnlocked: false, hasUsers: false })
      .mockResolvedValueOnce({ vaultConfigured: true, vaultUnlocked: true, hasUsers: false })
      .mockResolvedValueOnce({ vaultConfigured: true, vaultUnlocked: true, hasUsers: true });
    mockSetupVault.mockResolvedValueOnce('RECOVERY-KEY');
    mockCreateAdminUser.mockResolvedValueOnce(undefined);
    mockLogin.mockResolvedValueOnce({ userId: 'admin-id', sessionId: 'session-id', role: 'admin' });

    render(
      <AuthGate>
        <div>Aplicação</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'Inicializar cofre' })).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText('Senha mestre'), 'senha-mestre');
    await userEvent.type(screen.getByLabelText('Confirmar senha'), 'senha-mestre');
    await userEvent.click(screen.getByRole('button', { name: 'Continuar' }));

    expect(await screen.findByText('RECOVERY-KEY')).toBeInTheDocument();
    expect(await screen.findByRole('heading', { name: 'Criar admin local' })).toBeInTheDocument();
    expect(mockSetupVault).toHaveBeenCalledWith('senha-mestre');

    await userEvent.type(screen.getByLabelText('Usuário'), 'admin');
    await userEvent.type(screen.getByLabelText('Senha do admin'), 'senha-admin');
    await userEvent.type(screen.getByLabelText('Confirmar senha'), 'senha-admin');
    await userEvent.click(screen.getByRole('button', { name: 'Continuar' }));

    await waitFor(() => {
      expect(mockCreateAdminUser).toHaveBeenCalledWith({
        username: 'admin',
        password: 'senha-admin',
        displayName: 'admin',
      });
    });
    expect(await screen.findByRole('heading', { name: 'Entrar' })).toBeInTheDocument();

    await userEvent.clear(screen.getByLabelText('Senha'));
    await userEvent.type(screen.getByLabelText('Senha'), 'senha-admin');
    await userEvent.click(screen.getByRole('button', { name: 'Continuar' }));

    expect(await screen.findByText('Aplicação')).toBeInTheDocument();
    expect(mockLogin).toHaveBeenCalledWith({ username: 'admin', password: 'senha-admin', clientLabel: 'Wails desktop' });
  });

  it('envia o usuário digitado no login para permitir alternância entre contas locais', async () => {
    mockGetAuthStatus.mockResolvedValue({ vaultConfigured: true, vaultUnlocked: true, hasUsers: true });
    mockRefreshAuth.mockRejectedValue(new Error('sem sessão'));
    mockLogin
      .mockResolvedValueOnce({ userId: 'ana-id', sessionId: 'session-ana', role: 'user' })
      .mockResolvedValueOnce({ userId: 'leo-id', sessionId: 'session-leo', role: 'user' });

    const view = render(
      <AuthGate>
        <div>Aplicação</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'Entrar' })).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText('Usuário'), 'ana');
    await userEvent.type(screen.getByLabelText('Senha'), 'senha-ana');
    await userEvent.click(screen.getByRole('button', { name: 'Continuar' }));
    expect(await screen.findByText('Aplicação')).toBeInTheDocument();

    act(() => {
      useAuthStore.setState({ user: null, isAuthenticated: false, error: null });
    });
    view.rerender(
      <AuthGate>
        <div>Aplicação</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'Entrar' })).toBeInTheDocument();
    await userEvent.clear(screen.getByLabelText('Usuário'));
    await userEvent.clear(screen.getByLabelText('Senha'));
    await userEvent.type(screen.getByLabelText('Usuário'), 'leo');
    await userEvent.type(screen.getByLabelText('Senha'), 'senha-leo');
    await userEvent.click(screen.getByRole('button', { name: 'Continuar' }));

    expect(mockLogin).toHaveBeenNthCalledWith(1, { username: 'ana', password: 'senha-ana', clientLabel: 'Wails desktop' });
    expect(mockLogin).toHaveBeenNthCalledWith(2, { username: 'leo', password: 'senha-leo', clientLabel: 'Wails desktop' });
  });

  it('mantém o gate de login quando refresh falha no carregamento inicial', async () => {
    mockGetAuthStatus.mockResolvedValueOnce({ vaultConfigured: true, vaultUnlocked: true, hasUsers: true });
    mockRefreshAuth.mockRejectedValueOnce(new Error('refresh expirado'));

    render(
      <AuthGate>
        <div>Aplicação</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'Entrar' })).toBeInTheDocument();
    expect(screen.queryByText('Aplicação')).not.toBeInTheDocument();
    expect(useAuthStore.getState().user).toBeNull();
    expect(useAuthStore.getState().isAuthenticated).toBe(false);
  });
});
