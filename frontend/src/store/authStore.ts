import { create } from 'zustand';
import {
  CreateAdminUser,
  GetAuthStatus,
  Login,
  Logout,
  RefreshAuth,
  SetupVault,
  UnlockVault,
} from '@wailsjs/go/app/App';

export interface AuthStatus {
  vaultConfigured: boolean;
  vaultUnlocked: boolean;
  hasUsers: boolean;
}

interface AuthUser {
  userId: string;
  sessionId: string;
  role: string;
}

/**
 * `i18nKey` referencia uma chave de `auth.errors.*` nos locales. O store
 * NUNCA renderiza uma string crua do backend — o caller usa a chave para
 * lookup com `useTranslation()`. Isso resolve M28 do review do Bloco 5
 * (mensagens hardcoded em pt-BR vazando para a UI quando o backend muda)
 * e a regra do CLAUDE.md (i18n obrigatório).
 *
 * `detail` mantém o texto original do erro só para anúncios em live
 * regions / log estruturado quando faz sentido (raríssimo) — a UI
 * sempre prefere a tradução de `i18nKey`.
 */
export interface AuthError {
  i18nKey: string;
  detail?: string;
}

interface AuthState {
  status: AuthStatus | null;
  user: AuthUser | null;
  isLoading: boolean;
  error: AuthError | null;
  isAuthenticated: boolean;
  loadStatus: () => Promise<void>;
  setupVault: (masterPassword: string) => Promise<string>;
  unlockVault: (kind: string, secret: string) => Promise<void>;
  createAdmin: (username: string, password: string, displayName?: string) => Promise<void>;
  login: (username: string, password: string) => Promise<void>;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
}

/**
 * Versões antigas do app guardavam o refresh token em `localStorage`.
 * O fluxo atual (AEP-0052) persiste o refresh **só no backend**:
 * cifrado pela DEK do vault em `internal-auth:refresh-token` e
 * espelhado no keychain do SO. O frontend não toca mais nesse token.
 *
 * A constante existe apenas para uma migração defensiva: ao boot do app
 * apagamos qualquer resíduo legado para que extensões/scripts não tenham
 * mais acesso ao token via `localStorage.getItem`. Após +1 release sem
 * nenhuma instalação reportar resíduo, este bloco pode ser removido.
 */
const LEGACY_REFRESH_TOKEN_KEY = 'assistente-auth-refresh-token';

function purgeLegacyTokenStorage() {
  try {
    if (typeof localStorage !== 'undefined') {
      localStorage.removeItem(LEGACY_REFRESH_TOKEN_KEY);
    }
  } catch {
    /* localStorage indisponível em algum sandbox — ignore. */
  }
}

/**
 * Mapeia mensagens conhecidas do backend para chaves de i18n. O backend
 * de auth core (Fatia 1) padronizou ErrInvalidCredentials,
 * ErrInactiveUser, ErrInvalidRefreshToken, ErrInvalidVaultSecret. Quando
 * a string não bate com nenhum padrão conhecido caímos no fallback
 * genérico — nunca renderizamos a mensagem crua, evitando vazar
 * estrutura interna do servidor.
 */
function mapBackendError(error: unknown): AuthError {
  const detail = error instanceof Error ? error.message : String(error);
  const lower = detail.toLowerCase();
  if (lower.includes('credenciais') || lower.includes('invalid credential') || lower.includes('senha')) {
    return { i18nKey: 'auth.errors.invalidCredentials', detail };
  }
  if (lower.includes('inactive') || lower.includes('inativ')) {
    return { i18nKey: 'auth.errors.inactiveUser', detail };
  }
  if (lower.includes('refresh')) {
    return { i18nKey: 'auth.errors.sessionExpired', detail };
  }
  if (lower.includes('vault') || lower.includes('cofre') || lower.includes('dek')) {
    return { i18nKey: 'auth.errors.vaultUnavailable', detail };
  }
  if (lower.includes('admin') && lower.includes('já')) {
    return { i18nKey: 'auth.errors.adminAlreadyExists', detail };
  }
  return { i18nKey: 'auth.errors.unknown', detail };
}

/**
 * `refreshGuard` serializa execuções concorrentes de `refresh()`. Antes,
 * múltiplos `loadStatus()` em paralelo (ex: alt-tab → focus event) podiam
 * disparar refreshes simultâneos que se sobrescreviam no setState. Agora
 * o segundo caller espera a mesma promise e ambos enxergam o mesmo
 * resultado — equivalente ao mutex pedido no M36 do review.
 */
let refreshGuard: Promise<void> | null = null;

/**
 * Counter monotônico de logout. Quando um logout acontece DURANTE um
 * refresh em flight, o resultado do refresh é descartado para evitar o
 * cenário "token zumbi": refresh retorna access novo de uma sessão que
 * já foi revogada pelo logout do usuário. Combinada com `refreshGuard`,
 * esta defesa cobre M36 do Bloco 5.
 */
let logoutGeneration = 0;
let loadStatusInFlight: Promise<void> | null = null;

purgeLegacyTokenStorage();

export const useAuthStore = create<AuthState>()((set, get) => ({
  status: null,
  user: null,
  isLoading: false,
  error: null,
  isAuthenticated: false,

  loadStatus: async () => {
    if (loadStatusInFlight && get().isLoading) {
      return loadStatusInFlight;
    }
    loadStatusInFlight = null;
    loadStatusInFlight = (async () => {
      set({ isLoading: true, error: null });
      try {
        const status = (await GetAuthStatus()) as AuthStatus;
        set({ status });
        if (status.vaultUnlocked && status.hasUsers) {
          await get().refresh();
        }
        set({ isLoading: false });
      } catch (error) {
        console.error('[authStore] loadStatus failed', error);
        set({
          error: mapBackendError(error),
          isLoading: false,
          user: null,
          isAuthenticated: false,
        });
      } finally {
        loadStatusInFlight = null;
      }
    })();
    return loadStatusInFlight;
  },

  setupVault: async (masterPassword) => {
    set({ isLoading: true, error: null });
    try {
      const recoveryKey = await SetupVault(masterPassword);
      const status = (await GetAuthStatus()) as AuthStatus;
      set({ status, isLoading: false });
      return recoveryKey;
    } catch (error) {
      console.error('[authStore] setupVault failed', error);
      set({ error: mapBackendError(error), isLoading: false });
      throw error;
    }
  },

  unlockVault: async (kind, secret) => {
    set({ isLoading: true, error: null });
    try {
      await UnlockVault(kind, secret);
      const status = (await GetAuthStatus()) as AuthStatus;
      set({ status });
      if (status.vaultUnlocked && status.hasUsers) {
        await get().refresh();
      }
      set({ isLoading: false });
    } catch (error) {
      console.error('[authStore] unlockVault failed', error);
      set({ error: mapBackendError(error), isLoading: false });
      throw error;
    }
  },

  createAdmin: async (username, password, displayName) => {
    set({ isLoading: true, error: null });
    try {
      await CreateAdminUser({ username, password, displayName: displayName || username });
      const status = (await GetAuthStatus()) as AuthStatus;
      set({ status, isLoading: false });
    } catch (error) {
      console.error('[authStore] createAdmin failed', error);
      set({ error: mapBackendError(error), isLoading: false });
      throw error;
    }
  },

  login: async (username, password) => {
    set({ isLoading: true, error: null });
    try {
      const user = parseAuthUser(await Login({ username, password, clientLabel: 'Wails desktop' }));
      set({
        user,
        isAuthenticated: true,
        isLoading: false,
        error: null,
      });
    } catch (error) {
      console.error('[authStore] login failed', error);
      set({
        error: mapBackendError(error),
        isLoading: false,
        user: null,
        isAuthenticated: false,
      });
      throw error;
    }
  },

  refresh: async () => {
    if (refreshGuard) {
      await refreshGuard;
      return;
    }
    const generationAtStart = logoutGeneration;
    refreshGuard = (async () => {
      try {
        const user = parseAuthUser(await RefreshAuth({}));
        if (logoutGeneration !== generationAtStart) {
          // Logout aconteceu durante o refresh — descartamos o resultado
          // para não ressuscitar a sessão (M36 do review).
          console.warn('[authStore] refresh result discarded: logout in flight');
          return;
        }
        set({ user, isAuthenticated: true, error: null });
      } catch (error) {
        if (logoutGeneration !== generationAtStart) {
          // Mesma defesa: o erro do refresh não importa se o usuário já
          // pediu logout — o estado correto é "deslogado".
          return;
        }
        console.warn('[authStore] refresh failed', error);
        set({ user: null, isAuthenticated: false });
      }
    })();
    try {
      await refreshGuard;
    } finally {
      refreshGuard = null;
    }
  },

  logout: async () => {
    logoutGeneration += 1;
    try {
      await Logout({});
    } catch (error) {
      // Backend já trata logout como best-effort (M23 do Bloco 4); aqui
      // apenas logamos para telemetria e seguimos limpando o estado.
      console.warn('[authStore] logout RPC failed', error);
    }
    purgeLegacyTokenStorage();
    set({ user: null, isAuthenticated: false, error: null });
  },
}));

function parseAuthUser(value: unknown): AuthUser {
  if (!isAuthUser(value)) {
    throw new Error('sessão inválida retornada pelo backend');
  }
  return value;
}

function isAuthUser(value: unknown): value is AuthUser {
  if (!value || typeof value !== 'object') {
    return false;
  }
  const user = value as Partial<AuthUser>;
  return Boolean(user.userId && user.sessionId && user.role);
}

export const __testing__ = {
  resetGuards() {
    refreshGuard = null;
    logoutGeneration = 0;
    loadStatusInFlight = null;
  },
  mapBackendError,
};
