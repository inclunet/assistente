import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from '@testing-library/react';

import { useAuthStore, __testing__ as authTesting } from './authStore';
import { useEditorStore } from './editorStore';

const mockGetAuthStatus = vi.fn();
const mockRefreshAuth = vi.fn();
const mockLogin = vi.fn();
const mockLogout = vi.fn();

vi.mock('@wailsjs/go/app/App', () => ({
  GetAuthStatus: () => mockGetAuthStatus(),
  RefreshAuth: (payload: unknown) => mockRefreshAuth(payload),
  SetupVault: vi.fn(),
  UnlockVault: vi.fn(),
  CreateAdminUser: vi.fn(),
  Login: (payload: unknown) => mockLogin(payload),
  Logout: (payload: unknown) => mockLogout(payload),
}));

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  mockGetAuthStatus.mockReset();
  mockRefreshAuth.mockReset();
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
  useEditorStore.getState().clearUser();
});

describe('authStore.refresh', () => {
  it('serializa chamadas concorrentes em uma única RPC (mutex)', async () => {
    const gate = deferred<{ userId: string; sessionId: string; role: string }>();
    mockRefreshAuth.mockImplementation(() => gate.promise);

    const a = useAuthStore.getState().refresh();
    const b = useAuthStore.getState().refresh();
    const c = useAuthStore.getState().refresh();

    expect(mockRefreshAuth).toHaveBeenCalledTimes(1);

    await act(async () => {
      gate.resolve({ userId: 'u-1', sessionId: 's-1', role: 'user' });
      await Promise.all([a, b, c]);
    });

    expect(useAuthStore.getState().isAuthenticated).toBe(true);
    expect(useAuthStore.getState().user?.userId).toBe('u-1');
  });

  it('descarta o resultado do refresh se logout ocorrer em flight (M36)', async () => {
    const gate = deferred<{ userId: string; sessionId: string; role: string }>();
    mockRefreshAuth.mockImplementation(() => gate.promise);
    mockLogout.mockResolvedValue(undefined);

    const refreshPromise = useAuthStore.getState().refresh();

    await act(async () => {
      await useAuthStore.getState().logout();
    });

    await act(async () => {
      gate.resolve({ userId: 'u-zumbi', sessionId: 's-zumbi', role: 'user' });
      await refreshPromise;
    });

    // Mesmo com refresh "bem-sucedido", o estado deve permanecer
    // deslogado: o usuário pediu logout, sessão remota pode ter sido
    // revogada. Nunca ressuscitar.
    expect(useAuthStore.getState().isAuthenticated).toBe(false);
    expect(useAuthStore.getState().user).toBeNull();
  });

  it('descarta o erro do refresh se logout ocorrer em flight', async () => {
    const gate = deferred<{ userId: string; sessionId: string; role: string }>();
    mockRefreshAuth.mockImplementation(() => gate.promise);
    mockLogout.mockResolvedValue(undefined);

    // Estado inicial autenticado para garantir que o resultado do refresh
    // não pode "regredir" o usuário a desautenticado depois do logout.
    useAuthStore.setState({
      isAuthenticated: false,
      user: null,
    });

    const refreshPromise = useAuthStore.getState().refresh();

    await act(async () => {
      await useAuthStore.getState().logout();
    });

    await act(async () => {
      gate.reject(new Error('refresh expired'));
      await refreshPromise;
    });

    expect(useAuthStore.getState().isAuthenticated).toBe(false);
    expect(useAuthStore.getState().user).toBeNull();
  });

  it('libera o mutex após resolver para permitir refresh subsequente', async () => {
    mockRefreshAuth
      .mockResolvedValueOnce({ userId: 'u-1', sessionId: 's-1', role: 'user' })
      .mockResolvedValueOnce({ userId: 'u-2', sessionId: 's-2', role: 'user' });

    await act(async () => {
      await useAuthStore.getState().refresh();
    });
    expect(useAuthStore.getState().user?.userId).toBe('u-1');
    useEditorStore.getState().createDocument({
      id: 'privado-u1',
      markdown: 'não pode chegar ao usuário 2',
    });

    await act(async () => {
      await useAuthStore.getState().refresh();
    });
    expect(useAuthStore.getState().user?.userId).toBe('u-2');
    expect(useEditorStore.getState().ownerUserId).toBe('u-2');
    expect(useEditorStore.getState().documents).toEqual({});
    expect(mockRefreshAuth).toHaveBeenCalledTimes(2);
  });
});

describe('authStore.error mapping (M28)', () => {
  it('mapeia mensagens conhecidas para chaves i18n estáveis', () => {
    expect(authTesting.mapBackendError(new Error('credenciais inválidas')).i18nKey).toBe(
      'auth.errors.invalidCredentials',
    );
    expect(authTesting.mapBackendError(new Error('user inactive')).i18nKey).toBe(
      'auth.errors.inactiveUser',
    );
    expect(authTesting.mapBackendError(new Error('refresh token inválido')).i18nKey).toBe(
      'auth.errors.sessionExpired',
    );
    expect(authTesting.mapBackendError(new Error('cofre indisponível')).i18nKey).toBe(
      'auth.errors.vaultUnavailable',
    );
    expect(authTesting.mapBackendError(new Error('coisa esdrúxula')).i18nKey).toBe(
      'auth.errors.unknown',
    );
  });

  it('preserva a mensagem original em `detail` para telemetria/log', () => {
    const result = authTesting.mapBackendError(new Error('credenciais inválidas'));
    expect(result.detail).toBe('credenciais inválidas');
  });
});

describe('authStore.login (M28)', () => {
  it('grava AuthError mapeada no estado quando login falha', async () => {
    mockLogin.mockRejectedValueOnce(new Error('credenciais inválidas'));

    await expect(useAuthStore.getState().login('admin', 'wrong')).rejects.toThrow();

    const error = useAuthStore.getState().error;
    expect(error).not.toBeNull();
    expect(error?.i18nKey).toBe('auth.errors.invalidCredentials');
    expect(useAuthStore.getState().isAuthenticated).toBe(false);
  });
});

describe('authStore boot purge (B25 adaptado)', () => {
  it('limpa refresh token legado de localStorage ao logout', async () => {
    localStorage.setItem('assistente-auth-refresh-token', 'leak');
    mockLogout.mockResolvedValue(undefined);

    await act(async () => {
      await useAuthStore.getState().logout();
    });

    expect(localStorage.getItem('assistente-auth-refresh-token')).toBeNull();
    expect(useEditorStore.getState().ownerUserId).toBeNull();
    expect(useEditorStore.getState().documents).toEqual({});
  });
});
