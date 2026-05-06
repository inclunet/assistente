import { create } from 'zustand';
import { persist } from 'zustand/middleware';
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

export const useAuthStore = create<AuthState>()(
  persist(
    (set, get) => ({
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
          set({ status, isLoading: false });
          if (get().refreshToken && status.vaultUnlocked && status.hasUsers) {
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
          set({
            accessToken: pair.accessToken,
            refreshToken: pair.refreshToken,
            user,
            isAuthenticated: true,
            error: null,
          });
        } catch {
          set({ accessToken: '', refreshToken: '', user: null, isAuthenticated: false });
        }
      },

      logout: async () => {
        const refreshToken = get().refreshToken;
        if (refreshToken) {
          await Logout({ refreshToken }).catch(() => undefined);
        }
        set({ accessToken: '', refreshToken: '', user: null, isAuthenticated: false });
      },
    }),
    {
      name: 'assistente-auth',
      partialize: (state) => ({ refreshToken: state.refreshToken }),
    },
  ),
);

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
