import { describe, expect, it } from 'vitest';
import en from './en';
import es from './es';
import ptBR from './pt-BR';

const localeModules = {
  en,
  es,
  'pt-BR': ptBR,
} as const;

const requiredAccessibilityKeys = [
  'channels.signalLink.generateQr',
  'channels.signalLink.regionLabel',
  'channels.signalLink.scanQr',
  'channels.signalLink.qrAlt',
  'channels.signalLink.generating',
  'channels.signalLink.waiting',
  'jobs.builder.testSuccess',
  'ui.imageViewer.zoomLevel',
] as const;

function getLocaleValue(locale: unknown, key: string): unknown {
  const root = (locale as { translation: Record<string, unknown> }).translation;
  return key.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object') return undefined;
    return (current as Record<string, unknown>)[part];
  }, root);
}

describe('accessibility locale keys', () => {
  it.each(Object.entries(localeModules))('declara chaves acessíveis em %s', (_localeName, locale) => {
    for (const key of requiredAccessibilityKeys) {
      expect(getLocaleValue(locale, key), key).toEqual(expect.any(String));
    }
  });
});
