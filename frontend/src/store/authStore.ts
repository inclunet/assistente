import { create } from 'zustand';
import {
  ClearAuthRefreshToken,
  CreateAdminUser,
  GetAuthStatus,
  GetAuthUser,
  LoadAuthRefreshToken,
  Login,
  Logout,
  RefreshAuth,
  SetupVault,
  StoreAuthRefreshToken,
  UnlockVault,
} from '@wailsjs/go/app/App';

export interface AuthStatus {
  vaultConfigured: boolean;
  vaultUnlocked: boolean;
  hasUsers: boolean;
}

interface TokenPair {
  accessToken: string;
  refreshToken: string;
}

interface AuthUser {
  userId: string;
  sessionId: string;
  role: string;
}

interface AuthState {
  status: AuthStatus | null;
  user: AuthUser | null;
  accessToken: string;
  refreshToken: string;
  isLoading: boolean;
  error: string | null;
  isAuthenticated: boolean;
  loadStatus: () => Promise<void>;
  setupVault: (masterPassword: string) => Promise<string>;
  unlockVault: (kind: string, secret: string) => Promise<void>;
  createAdmin: (username: string, password: string, displayName?: string) => Promise<void>;
  login: (username: string, password: string) => Promise<void>;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
}

const refreshTokenFallbackKey = 'assistente-auth-refresh-token';

export const useAuthStore = create<AuthState>()((set, get) => ({
  status: null,
  user: null,
  accessToken: '',
  refreshToken: '',
  isLoading: false,
  error: null,
  isAuthenticated: false,

  loadStatus: async () => {
    set({ isLoading: true, error: null });
    try {
      const status = (await GetAuthStatus()) as AuthStatus;
      const refreshToken = await loadStoredRefreshToken();
      set({ status, refreshToken, isLoading: false });
      if (refreshToken && status.vaultUnlocked && status.hasUsers) {
        await get().refresh();
      }
    } catch (error) {
      set({ error: errorMessage(error), isLoading: false, isAuthenticated: false });
    }
  },

  setupVault: async (masterPassword) => {
    set({ isLoading: true, error: null });
    try {
      const recoveryKey = await SetupVault(masterPassword);
      const status = (await GetAuthStatus()) as AuthStatus;
      set({ status, isLoading: false });
      return recoveryKey;
    } catch (error) {
      set({ error: errorMessage(error), isLoading: false });
      throw error;
    }
  },

  unlockVault: async (kind, secret) => {
    set({ isLoading: true, error: null });
    try {
      await UnlockVault(kind, secret);
      const status = (await GetAuthStatus()) as AuthStatus;
      set({ status, isLoading: false });
    } catch (error) {
      set({ error: errorMessage(error), isLoading: false });
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
      set({ error: errorMessage(error), isLoading: false });
      throw error;
    }
  },

  login: async (username, password) => {
    set({ isLoading: true, error: null });
    try {
      const pair = (await Login({ username, password, clientLabel: 'Wails desktop' })) as TokenPair;
      const user = (await GetAuthUser(pair.accessToken)) as AuthUser;
      await storeRefreshToken(pair.refreshToken);
      set({
        accessToken: pair.accessToken,
        refreshToken: pair.refreshToken,
        user,
        isAuthenticated: true,
        isLoading: false,
      });
    } catch (error) {
      set({ error: errorMessage(error), isLoading: false, isAuthenticated: false });
      throw error;
    }
  },

  refresh: async () => {
    const refreshToken = get().refreshToken;
    if (!refreshToken) return;
    try {
      const pair = (await RefreshAuth({ refreshToken })) as TokenPair;
      const user = (await GetAuthUser(pair.accessToken)) as AuthUser;
      await storeRefreshToken(pair.refreshToken);
      set({
        accessToken: pair.accessToken,
        refreshToken: pair.refreshToken,
        user,
        isAuthenticated: true,
        error: null,
      });
    } catch {
      await clearStoredRefreshToken();
      set({ accessToken: '', refreshToken: '', user: null, isAuthenticated: false });
    }
  },

  logout: async () => {
    const refreshToken = get().refreshToken;
    if (refreshToken) {
      await Logout({ refreshToken }).catch(() => undefined);
    }
    await clearStoredRefreshToken();
    set({ accessToken: '', refreshToken: '', user: null, isAuthenticated: false });
  },
}));

async function loadStoredRefreshToken(): Promise<string> {
  const keyringToken = await LoadAuthRefreshToken().catch(() => '');
  if (keyringToken) return keyringToken;
  return localStorage.getItem(refreshTokenFallbackKey) || '';
}

async function storeRefreshToken(refreshToken: string): Promise<void> {
  try {
    await StoreAuthRefreshToken(refreshToken);
    localStorage.removeItem(refreshTokenFallbackKey);
  } catch {
    localStorage.setItem(refreshTokenFallbackKey, refreshToken);
  }
}

async function clearStoredRefreshToken(): Promise<void> {
  localStorage.removeItem(refreshTokenFallbackKey);
  await ClearAuthRefreshToken().catch(() => undefined);
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
