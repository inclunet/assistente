import { describe, expect, it } from 'vitest';
import { createInstance } from 'i18next';
import en from './en';
import es from './es';
import ptBR from './pt-BR';

describe('rótulos reais da confirmação de sobrescrita', () => {
  it.each(Object.entries({ en, es, 'pt-BR': ptBR }))('%s contém texto e interpola o destino', async (language, resource) => {
    const instance = createInstance();
    await instance.init({ lng: language, resources: { [language]: resource }, fallbackLng: false });
    for (const suffix of ['Title', 'Description', 'Confirm', 'Cancel']) {
      const key = `app.questionnaire.editConfirmation.overwrite${suffix}`;
      expect(instance.exists(key)).toBe(true);
      expect(instance.t(key, { path: 'arquivo-teste.md' })).not.toBe(key);
    }
    expect(instance.t('app.questionnaire.editConfirmation.overwriteDescription', { path: 'arquivo-teste.md' })).toContain('arquivo-teste.md');
  });
});
