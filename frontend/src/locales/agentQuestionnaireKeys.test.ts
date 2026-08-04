import { describe, expect, it } from 'vitest';
import en from './en';
import es from './es';
import ptBR from './pt-BR';

/**
 * Chaves dos diálogos que o agente de código faz o backend abrir (AEP-0085
 * Fase 2). O risco que o AEP nomeia é justamente este: chave criada no Go e
 * esquecida em `en.ts` ou `es.ts`. O fallback pt-BR salva o diálogo de sair em
 * branco, mas quem lê em inglês ou espanhol recebe em português um pedido de
 * autorização que precisa entender *antes* de responder.
 */
const localeModules = { en, es, 'pt-BR': ptBR } as const;

/** Classes de ação do protocolo (backend: `acp.ToolKind`). */
const actionClasses = [
  'read',
  'edit',
  'delete',
  'move',
  'search',
  'execute',
  'think',
  'fetch',
  'switchMode',
  'other',
] as const;

const permissionKeys = [
  'title',
  'submit',
  'cancel',
  'actionPrompt',
  'choicePrompt',
  ...actionClasses.map((classe) => `description.${classe}`),
  ...actionClasses.map((classe) => `descriptionAlways.${classe}`),
].map((sufixo) => `app.questionnaire.agentPermission.${sufixo}`);

const questionKeys = [
  'title',
  'submit',
  'cancel',
  'description',
  'descriptionSubject',
  'promptLabel',
  'promptLabelNumbered',
  'answerPrompt',
  'answerPromptMultiple',
].map((sufixo) => `app.questionnaire.agentQuestion.${sufixo}`);

const planKeys = [
  'title',
  'submit',
  'cancel',
  'contentPrompt',
  'choicePrompt',
  'approve',
  'reject',
  'description',
  'descriptionSteps',
  'descriptionProject',
  'descriptionProjectSteps',
].map((sufixo) => `app.questionnaire.agentPlan.${sufixo}`);

/**
 * Valores que o backend manda interpolar. A tradução que não os usa perde o
 * dado do pedido: "Assunto:" sem assunto, "Pergunta de" sem número.
 */
const requiredPlaceholders: Record<string, string[]> = {
  'app.questionnaire.agentQuestion.descriptionSubject': ['{{subject}}'],
  'app.questionnaire.agentQuestion.promptLabelNumbered': ['{{position}}', '{{total}}'],
  'app.questionnaire.agentPlan.descriptionSteps': ['{{steps}}'],
  'app.questionnaire.agentPlan.descriptionProjectSteps': ['{{steps}}'],
};

function getLocaleValue(locale: unknown, key: string): unknown {
  const root = (locale as { translation: Record<string, unknown> }).translation;
  return key.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object') return undefined;
    return (current as Record<string, unknown>)[part];
  }, root);
}

describe('chaves dos diálogos do agente de código', () => {
  const keys = [...permissionKeys, ...questionKeys, ...planKeys];

  it.each(Object.entries(localeModules))('traduz todos os campos visíveis em %s', (_nome, locale) => {
    for (const key of keys) {
      const value = getLocaleValue(locale, key);
      expect(value, key).toEqual(expect.any(String));
      expect((value as string).trim(), key).not.toBe('');
    }
  });

  it.each(Object.entries(localeModules))('mantém os valores do pedido em %s', (_nome, locale) => {
    for (const [key, placeholders] of Object.entries(requiredPlaceholders)) {
      const value = getLocaleValue(locale, key) as string;
      for (const placeholder of placeholders) {
        expect(value, `${key} sem ${placeholder}`).toContain(placeholder);
      }
    }
  });
});
