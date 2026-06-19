import { create } from 'zustand';
import { persist } from 'zustand/middleware';

// AppConfig guarda apenas preferências de UI persistidas localmente.
// Configuração de LLM/modelo/voz/STT vive em profiles + provider registry
// (teardown do config.json legado — #299).
export interface AppConfig {
  theme: 'assistente' | 'amethyst' | 'midnight' | 'light' | 'high-contrast';
  language: 'pt-BR' | 'en' | 'es';
}

interface SettingsState {
  config: AppConfig;
  updateConfig: (partial: Partial<AppConfig>) => void;
}

const defaultConfig: AppConfig = {
  theme: 'assistente',
  language: 'pt-BR',
};

export const useSettingsStore = create<SettingsState>()(
  persist(
    (set) => ({
      config: defaultConfig,

      updateConfig: (partial) =>
        set((state) => ({
          config: { ...state.config, ...partial },
        })),
    }),
    {
      name: 'assistente-settings',
      version: 1,
      migrate: (persisted: unknown, version: number) => {
        const persistedState =
          typeof persisted === 'object' && persisted !== null
            ? (persisted as { config?: Partial<AppConfig> | null })
            : {};

        const config: AppConfig = { ...defaultConfig, ...(persistedState.config ?? {}) };

        if (version === 0 && config.theme === 'amethyst') {
          config.theme = 'assistente';
        }

        return { config };
      },
      partialize: (state) => ({ config: state.config }),
    }
  )
);
