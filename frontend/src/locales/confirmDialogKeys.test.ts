import { describe, expect, it } from 'vitest';
import en from './en';
import es from './es';
import ptBR from './pt-BR';

/**
 * Chaves das confirmações que a Fase 3 do AEP-0091 tirou do `window.confirm` e
 * levou para o `useConfirm`/`DecisionDialog`. O `window.confirm` mostrava a
 * string que recebia; o `t()` mostra a própria chave quando ela não existe no
 * locale, e o `defaultValue` só cobre pt-BR. Os testes de componente mockam
 * `t()` devolvendo a chave, então quem checa a existência real é este arquivo.
 */
const localeModules = { en, es, 'pt-BR': ptBR } as const;

const editableListKeys = [
  'loadError',
  'createSuccess',
  'updateSuccess',
  'createError',
  'updateError',
  'cannotDelete',
  'deleteSuccess',
  'deleteError',
  'deleteConfirmTitle',
  'deleteConfirm',
].map((sufixo) => `editableList.${sufixo}`);

const jobsKeys = ['jobs.builder.deleteConfirmTitle', 'jobs.builder.deleteConfirm'];

const taskListKeys = [
  'tasklist.deleteTaskTitle',
  'tasklist.confirmDeleteTask',
  'tasklist.deleteConfirmTitle',
  'tasklist.confirmDelete',
];

/** Rótulos dos botões do rodapé (AEP-0090: primária antes de Cancelar). */
const actionKeys = ['common.delete', 'common.cancel'];

/**
 * Sem o `{{name}}` a pessoa lê "Tem certeza que deseja excluir?" sem saber o
 * que some — e é a última tela antes de uma exclusão.
 */
const requiredPlaceholders: Record<string, string[]> = {
  'editableList.loadError': ['{{name}}'],
  'editableList.createSuccess': ['{{name}}'],
  'editableList.updateSuccess': ['{{name}}'],
  'editableList.createError': ['{{name}}'],
  'editableList.updateError': ['{{name}}'],
  'editableList.cannotDelete': ['{{name}}'],
  'editableList.deleteSuccess': ['{{name}}'],
  'editableList.deleteError': ['{{name}}'],
  'editableList.deleteConfirmTitle': ['{{name}}'],
  'editableList.deleteConfirm': ['{{name}}'],
  'jobs.builder.deleteConfirm': ['{{name}}'],
};

function getLocaleValue(locale: unknown, key: string): unknown {
  const root = (locale as { translation: Record<string, unknown> }).translation;
  return key.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object') return undefined;
    return (current as Record<string, unknown>)[part];
  }, root);
}

describe('chaves das confirmações migradas do window.confirm', () => {
  const keys = [...editableListKeys, ...jobsKeys, ...taskListKeys, ...actionKeys];

  it.each(Object.entries(localeModules))('traduz título, mensagem e botões em %s', (_nome, locale) => {
    for (const key of keys) {
      const value = getLocaleValue(locale, key);
      expect(value, key).toEqual(expect.any(String));
      expect((value as string).trim(), key).not.toBe('');
    }
  });

  it.each(Object.entries(localeModules))('mantém o nome do item na frase em %s', (_nome, locale) => {
    for (const [key, placeholders] of Object.entries(requiredPlaceholders)) {
      const value = getLocaleValue(locale, key) as string;
      for (const placeholder of placeholders) {
        expect(value, `${key} sem ${placeholder}`).toContain(placeholder);
      }
    }
  });
});
