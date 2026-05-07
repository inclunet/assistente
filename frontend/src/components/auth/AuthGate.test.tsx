import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/react';
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
});
