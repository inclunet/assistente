import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { axe } from '../../test/a11yAxe';

import { AuthGate } from './AuthGate';
import { useAuthStore, __testing__ as authTesting } from '../../store/authStore';

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    // Em testes a key vence — replica o comportamento de `t()` quando
    // o resource bundle existe e o fallback é só safety-net. Garante
    // que asserts em chaves i18n batam de forma estável.
    t: (key: string) => key,
    i18n: { language: 'en', changeLanguage: vi.fn() },
  }),
}));

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
    // mockReset zera tanto chamadas quanto implementações (incluindo
    // mockResolvedValue). vi.clearAllMocks só zera chamadas, então
    // implementações vazadas de testes anteriores corrompiam os
    // status iniciais aqui.
    mockGetAuthStatus.mockReset();
    mockRefreshAuth.mockReset();
    mockSetupVault.mockReset();
    mockUnlockVault.mockReset();
    mockCreateAdminUser.mockReset();
    mockLogin.mockReset();
    mockLogout.mockReset();
    localStorage.clear();
    authTesting.resetGuards();
    useAuthStore.setState({
      status: null,
      user: null,
      isLoading: false,
      error: null,
      isAuthenticated: false,
    });
  });

  it('mostra estado de autenticação indisponível quando o backend falha', async () => {
    mockGetAuthStatus.mockRejectedValueOnce(new Error('cofre indisponível: DEK ausente'));

    render(
      <AuthGate>
        <div>App</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'auth.titles.unavailable' })).toBeInTheDocument();
    // M28 do Bloco 5: a mensagem crua do backend NÃO aparece na UI;
    // só a chave i18n mapeada.
    expect(screen.queryByText('cofre indisponível: DEK ausente')).not.toBeInTheDocument();
    expect(screen.getByText('auth.errors.vaultUnavailable')).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'auth.titles.setup' })).not.toBeInTheDocument();
    expect(mockSetupVault).not.toHaveBeenCalled();
  });

  it('nunca renderiza a mensagem crua do backend para erros desconhecidos', async () => {
    mockGetAuthStatus.mockRejectedValueOnce(new Error('panic: stack trace internal/auth/foo.go:42'));

    render(
      <AuthGate>
        <div>App</div>
      </AuthGate>,
    );

    await screen.findByRole('heading', { name: 'auth.titles.unavailable' });
    expect(screen.getByText('auth.errors.unknown')).toBeInTheDocument();
    expect(screen.queryByText(/panic: stack trace/)).not.toBeInTheDocument();
  });

  it('mostra estado neutro enquanto o status inicial ainda não carregou', () => {
    mockGetAuthStatus.mockImplementationOnce(() => new Promise(() => undefined));

    render(
      <AuthGate>
        <div>App</div>
      </AuthGate>,
    );

    expect(screen.getByRole('heading', { name: 'auth.titles.loading' })).toBeInTheDocument();
    expect(screen.queryByRole('heading', { name: 'auth.titles.setup' })).not.toBeInTheDocument();
  });

  it('permite tentar carregar status novamente após falha', async () => {
    mockGetAuthStatus
      .mockRejectedValueOnce(new Error('falha temporária'))
      .mockResolvedValueOnce({ vaultConfigured: true, vaultUnlocked: false, hasUsers: true });

    render(
      <AuthGate>
        <div>App</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'auth.titles.unavailable' })).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'auth.buttons.retry' }));

    await waitFor(() => {
      expect(screen.getByRole('heading', { name: 'auth.titles.unlock' })).toBeInTheDocument();
    });
    expect(screen.queryByRole('heading', { name: 'auth.titles.setup' })).not.toBeInTheDocument();
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
        <div>App</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'auth.titles.setup' })).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText('auth.labels.masterPassword'), 'senha-mestre');
    await userEvent.type(screen.getByLabelText('auth.labels.confirmPassword'), 'senha-mestre');
    await userEvent.click(screen.getByRole('button', { name: 'auth.buttons.continue' }));

    expect(await screen.findByText('RECOVERY-KEY')).toBeInTheDocument();
    expect(await screen.findByRole('heading', { name: 'auth.titles.createAdmin' })).toBeInTheDocument();
    expect(mockSetupVault).toHaveBeenCalledWith('senha-mestre');

    await userEvent.type(screen.getByLabelText('auth.labels.username'), 'admin');
    await userEvent.type(screen.getByLabelText('auth.labels.adminPassword'), 'senha-admin');
    await userEvent.type(screen.getByLabelText('auth.labels.confirmPassword'), 'senha-admin');
    await userEvent.click(screen.getByRole('button', { name: 'auth.buttons.continue' }));

    await waitFor(() => {
      expect(mockCreateAdminUser).toHaveBeenCalledWith({
        username: 'admin',
        password: 'senha-admin',
        displayName: 'admin',
      });
    });
    expect(await screen.findByRole('heading', { name: 'auth.titles.signIn' })).toBeInTheDocument();

    await userEvent.clear(screen.getByLabelText('auth.labels.password'));
    await userEvent.type(screen.getByLabelText('auth.labels.password'), 'senha-admin');
    await userEvent.click(screen.getByRole('button', { name: 'auth.buttons.continue' }));

    expect(await screen.findByText('App')).toBeInTheDocument();
    expect(mockLogin).toHaveBeenCalledWith({
      username: 'admin',
      password: 'senha-admin',
      clientLabel: 'Wails desktop',
    });
  });

  it('envia o usuário digitado no login para permitir alternância entre contas locais', async () => {
    mockGetAuthStatus.mockResolvedValue({ vaultConfigured: true, vaultUnlocked: true, hasUsers: true });
    mockRefreshAuth.mockRejectedValue(new Error('sem sessão'));
    mockLogin
      .mockResolvedValueOnce({ userId: 'ana-id', sessionId: 'session-ana', role: 'user' })
      .mockResolvedValueOnce({ userId: 'leo-id', sessionId: 'session-leo', role: 'user' });

    const view = render(
      <AuthGate>
        <div>App</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'auth.titles.signIn' })).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText('auth.labels.username'), 'ana');
    await userEvent.type(screen.getByLabelText('auth.labels.password'), 'senha-ana');
    await userEvent.click(screen.getByRole('button', { name: 'auth.buttons.continue' }));
    expect(await screen.findByText('App')).toBeInTheDocument();

    act(() => {
      useAuthStore.setState({ user: null, isAuthenticated: false, error: null });
    });
    view.rerender(
      <AuthGate>
        <div>App</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'auth.titles.signIn' })).toBeInTheDocument();
    await userEvent.clear(screen.getByLabelText('auth.labels.username'));
    await userEvent.clear(screen.getByLabelText('auth.labels.password'));
    await userEvent.type(screen.getByLabelText('auth.labels.username'), 'leo');
    await userEvent.type(screen.getByLabelText('auth.labels.password'), 'senha-leo');
    await userEvent.click(screen.getByRole('button', { name: 'auth.buttons.continue' }));

    expect(mockLogin).toHaveBeenNthCalledWith(1, {
      username: 'ana',
      password: 'senha-ana',
      clientLabel: 'Wails desktop',
    });
    expect(mockLogin).toHaveBeenNthCalledWith(2, {
      username: 'leo',
      password: 'senha-leo',
      clientLabel: 'Wails desktop',
    });
  });

  it('mantém o gate de login quando refresh falha no carregamento inicial', async () => {
    mockGetAuthStatus.mockResolvedValueOnce({ vaultConfigured: true, vaultUnlocked: true, hasUsers: true });
    mockRefreshAuth.mockRejectedValueOnce(new Error('refresh expirado'));

    render(
      <AuthGate>
        <div>App</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'auth.titles.signIn' })).toBeInTheDocument();
    expect(screen.queryByText('App')).not.toBeInTheDocument();
    expect(useAuthStore.getState().user).toBeNull();
    expect(useAuthStore.getState().isAuthenticated).toBe(false);
  });

  it('purga refresh token legado de localStorage no boot do store', async () => {
    localStorage.setItem('assistente-auth-refresh-token', 'legacy-leak');
    // Re-importar reaplica purgeLegacyTokenStorage no topo do módulo,
    // mas como o setup do teste já carregou o módulo, simulamos
    // chamando logout (que também purga).
    mockGetAuthStatus.mockResolvedValueOnce({
      vaultConfigured: true,
      vaultUnlocked: true,
      hasUsers: true,
    });
    mockLogout.mockResolvedValueOnce(undefined);

    await act(async () => {
      await useAuthStore.getState().logout();
    });

    expect(localStorage.getItem('assistente-auth-refresh-token')).toBeNull();
  });

  it('valida senhas que não conferem antes de chamar o backend', async () => {
    mockGetAuthStatus.mockResolvedValueOnce({
      vaultConfigured: false,
      vaultUnlocked: false,
      hasUsers: false,
    });

    render(
      <AuthGate>
        <div>App</div>
      </AuthGate>,
    );

    expect(await screen.findByRole('heading', { name: 'auth.titles.setup' })).toBeInTheDocument();

    await userEvent.type(screen.getByLabelText('auth.labels.masterPassword'), 'senha-1');
    await userEvent.type(screen.getByLabelText('auth.labels.confirmPassword'), 'senha-2');
    await userEvent.click(screen.getByRole('button', { name: 'auth.buttons.continue' }));

    expect(screen.getByRole('alert')).toHaveTextContent('auth.validation.passwordsDoNotMatch');
    expect(mockSetupVault).not.toHaveBeenCalled();
  });

  it('é acessível (axe-core) na tela de login', async () => {
    mockGetAuthStatus.mockResolvedValueOnce({
      vaultConfigured: true,
      vaultUnlocked: true,
      hasUsers: true,
    });
    mockRefreshAuth.mockRejectedValueOnce(new Error('sem sessão'));

    const { container } = render(
      <AuthGate>
        <div>App</div>
      </AuthGate>,
    );

    await screen.findByRole('heading', { name: 'auth.titles.signIn' });

    const results = await axe(container);
    expect(results).toHaveNoViolations();
  });

  it('é acessível (axe-core) na tela de setup', async () => {
    mockGetAuthStatus.mockResolvedValueOnce({
      vaultConfigured: false,
      vaultUnlocked: false,
      hasUsers: false,
    });

    const { container } = render(
      <AuthGate>
        <div>App</div>
      </AuthGate>,
    );

    await screen.findByRole('heading', { name: 'auth.titles.setup' });

    const results = await axe(container);
    expect(results).toHaveNoViolations();
  });
});
