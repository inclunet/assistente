import { useEffect } from 'react';
import { useSettingsStore } from '../store/settingsStore';

export type ThemeId = 'assistente' | 'amethyst' | 'midnight' | 'light' | 'high-contrast';

export const THEMES: { id: ThemeId; label: string; description: string }[] = [
  { id: 'assistente',     label: 'Assistente',     description: 'Tema escuro azul vibrante (padrão)' },
  { id: 'amethyst',       label: 'Ametista',       description: 'Tema escuro violeta' },
  { id: 'midnight',       label: 'Meia-Noite',     description: 'Tema escuro cinza-azulado' },
  { id: 'light',          label: 'Claro',           description: 'Tema claro com acentos roxos' },
  { id: 'high-contrast',  label: 'Alto Contraste',  description: 'Máximo contraste para acessibilidade' },
];

const LEGACY_MAP: Record<string, ThemeId> = {
  dark:      'assistente',
  system:    'assistente',
  inclunet:  'assistente',
};

function applyTheme(themeId: string) {
  const resolved = LEGACY_MAP[themeId] ?? themeId;
  document.documentElement.setAttribute('data-theme', resolved);
}

export function useTheme() {
  const theme = useSettingsStore((s) => s.config?.theme ?? 'assistente');
  const updateConfig = useSettingsStore((s) => s.updateConfig);

  useEffect(() => {
    applyTheme(theme);
  }, [theme]);

  const setTheme = (id: ThemeId) => {
    updateConfig({ theme: id });
    applyTheme(id);
  };

  return { theme: (LEGACY_MAP[theme] ?? theme) as ThemeId, setTheme, themes: THEMES };
}
