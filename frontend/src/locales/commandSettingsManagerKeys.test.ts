import { describe, expect, it } from 'vitest';
import en from './en';
import es from './es';
import ptBR from './pt-BR';

const localeModules = { en, es, 'pt-BR': ptBR } as const;

const requiredKeys = [
  'commandSettings.managers.settings',
  'commandSettings.managers.commands',
  'commandSettings.managers.hint',
  'commandSettings.managers.commandsCount',
  'commandSettings.managers.rulesCount',
  'commandSettings.rules.title',
  'commandSettings.dialog.layerTitle',
  'commandSettings.dialog.bindingTitle',
  'commandSettings.dialog.ruleTitle',
] as const;

const requiredPlaceholders: Record<string, string> = {
  'commandSettings.managers.commandsCount': '{{count}}',
  'commandSettings.managers.rulesCount': '{{count}}',
};

function getLocaleValue(locale: unknown, key: string): unknown {
  const root = (locale as { translation: Record<string, unknown> }).translation;
  return key.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object') return undefined;
    return (current as Record<string, unknown>)[part];
  }, root);
}

describe('chaves dos gerenciadores de camadas de comandos', () => {
  it.each(Object.entries(localeModules))('declara títulos e textos usados pela interface em %s', (_localeName, locale) => {
    for (const key of requiredKeys) {
      const value = getLocaleValue(locale, key);
      expect(value, key).toEqual(expect.any(String));
      expect((value as string).trim(), key).not.toBe('');
    }
  });

  it.each(Object.entries(localeModules))('preserva a contagem interpolada em %s', (_localeName, locale) => {
    for (const [key, placeholder] of Object.entries(requiredPlaceholders)) {
      expect(getLocaleValue(locale, key), key).toContain(placeholder);
    }
  });
});
