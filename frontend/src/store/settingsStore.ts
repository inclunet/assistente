import { create } from 'zustand';
import { persist } from 'zustand/middleware';

// AppConfig guarda apenas preferências de UI persistidas localmente.
// Configuração de LLM/modelo/voz/STT vive em profiles + provider registry
// (teardown do config.json legado — #299).
export interface AppConfig {
  theme: 'assistente' | 'amethyst' | 'midnight' | 'light' | 'high-contrast';
  language: 'pt-BR' | 'en' | 'es';
  /** Som de alerta na abertura de DecisionDialog (AEP-0091). Default: true. */
  decisionAlertSound: boolean;
  /** Impede bloqueio/suspensão da tela enquanto a janela está em foco. Default: true. */
  preventScreenLock: boolean;
  editor: {
    /** Tratamento de mudança externa segura. Default: autoReload. */
    externalChange: 'autoReload' | 'prompt';
  };
}

interface SettingsState {
  config: AppConfig;
  updateConfig: (partial: Partial<AppConfig>) => void;
}

const validThemes: AppConfig['theme'][] = [
  'assistente',
  'amethyst',
  'midnight',
  'light',
  'high-contrast',
];
const validLanguages: AppConfig['language'][] = ['pt-BR', 'en', 'es'];

export const defaultConfig: AppConfig = {
  theme: 'assistente',
  language: 'pt-BR',
  decisionAlertSound: true,
  preventScreenLock: true,
  editor: {
    externalChange: 'autoReload',
  },
};

export function sanitizeConfig(persistedConfig: Partial<AppConfig> | null | undefined): AppConfig {
  const config: AppConfig = {
    ...defaultConfig,
    editor: { ...defaultConfig.editor },
  };
  if (validThemes.includes(persistedConfig?.theme as AppConfig['theme'])) {
    config.theme = persistedConfig!.theme as AppConfig['theme'];
  }
  if (validLanguages.includes(persistedConfig?.language as AppConfig['language'])) {
    config.language = persistedConfig!.language as AppConfig['language'];
  }
  if (typeof persistedConfig?.decisionAlertSound === 'boolean') {
    config.decisionAlertSound = persistedConfig.decisionAlertSound;
  }
  if (typeof persistedConfig?.preventScreenLock === 'boolean') {
    config.preventScreenLock = persistedConfig.preventScreenLock;
  }
  const externalChange = persistedConfig?.editor?.externalChange;
  if (externalChange === 'autoReload' || externalChange === 'prompt') {
    config.editor.externalChange = externalChange;
  }
  return config;
}

export const useSettingsStore = create<SettingsState>()(
  persist(
    (set) => ({
      config: defaultConfig,

      updateConfig: (partial) =>
        set((state) => ({
          config: {
            ...state.config,
            ...partial,
            editor: partial.editor
              ? { ...state.config.editor, ...partial.editor }
              : state.config.editor,
          },
        })),
    }),
    {
      name: 'assistente-settings',
      version: 4,
      migrate: (persisted: unknown, version: number) => {
        const persistedState =
          typeof persisted === 'object' && persisted !== null
            ? (persisted as { config?: Partial<AppConfig> | null })
            : {};
        const config = sanitizeConfig(persistedState.config);

        // Whitelist explícito: só migramos as chaves suportadas.
        // Versões antigas persistiam LLM/credenciais neste mesmo objeto;
        // spread cego as manteria no localStorage (#299).

        if (version === 0 && config.theme === 'amethyst') {
          config.theme = 'assistente';
        }

        // v4: editor.externalChange default autoReload se ausente (já no sanitize).
        return { config };
      },
      partialize: (state) => ({ config: state.config }),
    }
  )
);
