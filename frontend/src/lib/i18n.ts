import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';
import ptBR from '../locales/pt-BR';
import en from '../locales/en';
import es from '../locales/es';

export type LanguageId = 'pt-BR' | 'en' | 'es';

export const LANGUAGES: { id: LanguageId; label: string; nativeLabel: string }[] = [
  { id: 'pt-BR', label: 'Português (Brasil)', nativeLabel: 'Português (Brasil)' },
  { id: 'en',    label: 'English',             nativeLabel: 'English' },
  { id: 'es',    label: 'Español',             nativeLabel: 'Español' },
];

function detectLanguage(): LanguageId {
  const stored = (() => {
    try {
      const raw = localStorage.getItem('assistente-settings');
      if (raw) {
        const parsed = JSON.parse(raw);
        return parsed?.state?.config?.language as string | undefined;
      }
    } catch { /* ignore */ }
    return undefined;
  })();

  if (stored && LANGUAGES.some((l) => l.id === stored)) {
    return stored as LanguageId;
  }

  const browserLang = navigator.language || (navigator as any).userLanguage || '';
  if (browserLang.startsWith('pt')) return 'pt-BR';
  if (browserLang.startsWith('es')) return 'es';
  return 'en';
}

const resources = {
  'pt-BR': ptBR,
  'en': en,
  'es': es,
};

i18n
  .use(initReactI18next)
  .init({
    resources,
    lng: detectLanguage(),
    fallbackLng: 'en',
    interpolation: {
      escapeValue: false,
    },
  });

export default i18n;
