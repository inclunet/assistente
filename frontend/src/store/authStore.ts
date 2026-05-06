import { create } from 'zustand';
import {
  CreateAdminUser,
  GetAuthStatus,
  GetAuthUser,
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

interface TokenPair {
  accessToken: string;
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

const legacyRefreshTokenKey = 'assistente-auth-refresh-token';

export const useAuthStore = create<AuthState>()((set, get) => ({
  status: null,
  user: null,
  accessToken: '',
  isLoading: false,
  error: null,
  isAuthenticated: false,

  loadStatus: async () => {
    set({ isLoading: true, error: null });
    try {
      const status = (await GetAuthStatus()) as AuthStatus;
      set({ status, isLoading: false });
      if (status.vaultUnlocked && status.hasUsers) {
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
      set({
        accessToken: pair.accessToken,
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
    try {
      const legacyRefreshToken = localStorage.getItem(legacyRefreshTokenKey) || '';
      const pair = (await RefreshAuth(legacyRefreshToken ? { refreshToken: legacyRefreshToken } : {})) as TokenPair;
      const user = (await GetAuthUser(pair.accessToken)) as AuthUser;
      localStorage.removeItem(legacyRefreshTokenKey);
      set({
        accessToken: pair.accessToken,
        user,
        isAuthenticated: true,
        error: null,
      });
    } catch {
      localStorage.removeItem(legacyRefreshTokenKey);
      set({ accessToken: '', user: null, isAuthenticated: false });
    }
  },

  logout: async () => {
    await Logout({}).catch(() => undefined);
    localStorage.removeItem(legacyRefreshTokenKey);
    set({ accessToken: '', user: null, isAuthenticated: false });
  },
}));

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
