import { describe, it, expect } from 'vitest';
import ptBR from './pt-BR';
import en from './en';
import es from './es';

// Regressão: as chaves de MCP nativo são consumidas pelo ProfileToolsSection
// como `profiles.nativeMcp*` (nível flat), não como `profiles.chatSection.nativeMcp*`.
// Se ficarem no namespace errado, en/es não resolvem e caem no fallback PT do código.
// O mock de t() nos testes de componente retorna a própria chave, então NÃO pega
// esse mismatch — por isso validamos a presença real das chaves aqui.

type Locale = { translation: { profiles: Record<string, unknown> } };

const locales: Record<string, Locale> = {
  'pt-BR': ptBR as unknown as Locale,
  en: en as unknown as Locale,
  es: es as unknown as Locale,
};

const NATIVE_MCP_KEYS = [
  'nativeMcpLabel',
  'nativeMcpAuto',
  'nativeMcpOn',
  'nativeMcpOff',
  'nativeMcpHint',
] as const;

describe('i18n: chaves profiles.nativeMcp*', () => {
  for (const [name, locale] of Object.entries(locales)) {
    it(`existem no nível profiles.* (flat) em ${name}`, () => {
      const profiles = locale.translation.profiles;
      for (const key of NATIVE_MCP_KEYS) {
        expect(typeof profiles[key], `${name}: profiles.${key}`).toBe('string');
        expect((profiles[key] as string).length).toBeGreaterThan(0);
      }
    });

    it(`NÃO existem mais sob profiles.chatSection.* em ${name}`, () => {
      const chatSection = locale.translation.profiles.chatSection as
        | Record<string, unknown>
        | undefined;
      if (!chatSection) return;
      for (const key of NATIVE_MCP_KEYS) {
        expect(chatSection[key], `${name}: profiles.chatSection.${key} órfã`).toBeUndefined();
      }
    });
  }
});
