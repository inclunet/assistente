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

interface AuthState {
  status: AuthStatus | null;
  user: AuthUser | null;
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
      const user = (await Login({ username, password, clientLabel: 'Wails desktop' })) as AuthUser;
      set({
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
      const user = (await RefreshAuth(legacyRefreshToken ? { refreshToken: legacyRefreshToken } : {})) as AuthUser;
      localStorage.removeItem(legacyRefreshTokenKey);
      set({
        user,
        isAuthenticated: true,
        error: null,
      });
    } catch {
      localStorage.removeItem(legacyRefreshTokenKey);
      set({ user: null, isAuthenticated: false });
    }
  },

  logout: async () => {
    await Logout({}).catch(() => undefined);
    localStorage.removeItem(legacyRefreshTokenKey);
    set({ user: null, isAuthenticated: false });
  },
}));

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
