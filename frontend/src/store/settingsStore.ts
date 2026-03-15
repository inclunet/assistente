import { create } from 'zustand';
import { persist } from 'zustand/middleware';

export interface AppConfig {
  apiKey: string;
  baseURL: string;
  defaultModel: string;
  temperature: number;
  maxTokens: number;
  streamEnabled: boolean;
  theme: 'assistente' | 'amethyst' | 'midnight' | 'light' | 'high-contrast';
  language: 'pt-BR' | 'en' | 'es';
  // Nota: voz/TTS e STT agora vêm do perfil global (ttsService / useInteractionProfile)
}

interface SettingsState {
  config: AppConfig | null;
  isLoading: boolean;
  error: string | null;
  
  // Actions
  setConfig: (config: AppConfig) => void;
  updateConfig: (partial: Partial<AppConfig>) => void;
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  reset: () => void;
}

const defaultConfig: AppConfig = {
  apiKey: '',
  baseURL: 'https://api.openai.com/v1',
  defaultModel: 'gpt-4',
  temperature: 0.7,
  maxTokens: 2000,
  streamEnabled: true,
  theme: 'assistente',
  language: 'pt-BR',
  // Nota: voz/TTS e STT agora vêm do perfil global
};

export const useSettingsStore = create<SettingsState>()(
  persist(
    (set) => ({
      config: null,
      isLoading: false,
      error: null,

      setConfig: (config) => set({ config, error: null }),
      
      updateConfig: (partial) =>
        set((state) => ({
          config: state.config ? { ...state.config, ...partial } : null,
        })),
      
      setLoading: (isLoading) => set({ isLoading }),
      
      setError: (error) => set({ error, isLoading: false }),
      
      reset: () => set({ config: defaultConfig, error: null, isLoading: false }),
    }),
    {
      name: 'assistente-settings',
      version: 1,
      migrate: (persisted: unknown, version: number) => {
        const persistedState =
          typeof persisted === 'object' && persisted !== null
            ? (persisted as { config?: AppConfig | null })
            : {};

        if (version === 0 && persistedState.config?.theme === 'amethyst') {
          persistedState.config = { ...persistedState.config, theme: 'assistente' };
        }

        return persistedState as { config: AppConfig | null };
      },
      partialize: (state) => ({ config: state.config }),
    }
  )
);
