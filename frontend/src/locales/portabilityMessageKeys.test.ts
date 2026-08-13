import { describe, it, expect } from 'vitest';
import ptBR from './pt-BR';
import en from './en';
import es from './es';

// Os avisos e erros de importação chegam do backend como código
// (internal/portability/messages.go) e são traduzidos por
// `portability.messages.<código>`. Chave que existe em um idioma e falta em
// outro devolve a lista pela metade em português, que é exatamente o que a
// tradução veio resolver.

type Messages = Record<string, unknown>;
type Locale = { translation: { portability: { messages: Messages } } };

const locales: Record<string, Locale> = {
  'pt-BR': ptBR as unknown as Locale,
  en: en as unknown as Locale,
  es: es as unknown as Locale,
};

function flatten(value: Messages, prefix = ''): string[] {
  return Object.entries(value).flatMap(([key, entry]) => {
    const path = prefix ? `${prefix}.${key}` : key;
    if (entry && typeof entry === 'object') return flatten(entry as Messages, path);
    return [path];
  });
}

const reference = flatten(locales['pt-BR'].translation.portability.messages).sort();

describe('i18n: chaves portability.messages.*', () => {
  it('pt-BR cobre os grupos que o backend emite', () => {
    for (const code of [
      'import.unsupportedResources',
      'import.emptyConversations',
      'acp.commandNotFound',
      'acp.credentialMissing',
      'provider.missingBaseUrl',
      'mcpServer.invalidTransport',
      'taskList.workflowWithoutStatuses',
      'memoryRecord.missingId',
      'credential.vaultUnavailableForImport',
      'conflict.mcpServerSlug',
    ]) {
      expect(reference, `pt-BR: ${code}`).toContain(code);
    }
  });

  for (const [name, locale] of Object.entries(locales)) {
    it(`${name} traduz exatamente as mesmas chaves`, () => {
      expect(flatten(locale.translation.portability.messages).sort()).toEqual(reference);
    });

    it(`${name} não deixa mensagem vazia`, () => {
      const messages = locale.translation.portability.messages;
      for (const code of reference) {
        const text = code.split('.').reduce<unknown>(
          (node, part) => (node as Messages | undefined)?.[part],
          messages,
        );
        expect(typeof text, `${name}: ${code}`).toBe('string');
        expect((text as string).trim().length, `${name}: ${code}`).toBeGreaterThan(0);
      }
    });
  }
});
