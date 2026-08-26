import { describe, it, expect } from 'vitest';
import i18next from 'i18next';

import ptBR from '../locales/pt-BR';
import en from '../locales/en';
import es from '../locales/es';

// Prova com instância real do i18next: `count` é seletor de plural no
// i18next (procura streamRetry_one/streamRetry_other). A chave de retry
// não tem sufixo plural — o teste garante que a resolução cai na chave
// base e interpola, em vez de devolver a chave crua ou vazia.
describe('chatNotice.streamRetry com i18next real', () => {
  it.each([
    ['pt-BR', ptBR, 'tentativa 2'],
    ['en', en, 'attempt 2'],
    ['es', es, 'intento 2'],
  ] as const)('%s resolve a chave base sem sufixo plural', async (lng, resource, esperado) => {
    const i18n = i18next.createInstance();
    await i18n.init({
      resources: { [lng]: resource },
      lng,
      fallbackLng: 'en',
      interpolation: { escapeValue: false },
    });

    const mensagem = i18n.t('app.chatNotice.streamRetry', { count: 2 });
    expect(mensagem).toContain(esperado);
    expect(mensagem).not.toContain('undefined');
    expect(mensagem).not.toContain('{{count}}');
  });
});
