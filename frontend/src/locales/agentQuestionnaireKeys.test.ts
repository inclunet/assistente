import { describe, expect, it } from 'vitest';
import en from './en';
import es from './es';
import ptBR from './pt-BR';

/**
 * Chaves dos diálogos que o backend abre (AEP-0085): os do agente de código
 * (Fase 2), a confirmação de alteração de arquivo (Fase 3) e o updater com o
 * wizard de boas-vindas (Fase 4). O risco que o AEP nomeia é justamente este:
 * chave criada no Go e esquecida em `en.ts` ou `es.ts`. O fallback pt-BR salva o
 * diálogo de sair em branco, mas quem lê em inglês ou espanhol recebe em
 * português um pedido de autorização que precisa entender *antes* de responder.
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

const editConfirmationKeys = [
  'titleEdit',
  'titleOverwrite',
  'description',
  'descriptionNotes',
  'beforePrompt',
  'afterPrompt',
  'submit',
  'cancel',
  'rejectReasonLabel',
  'rejectReasonPlaceholder',
].map((sufixo) => `app.questionnaire.editConfirmation.${sufixo}`);

const updateKeys = [
  'title',
  'description',
  'descriptionNotes',
  'descriptionSize',
  'descriptionNotesSize',
  'prompt',
  'submit',
  'cancel',
].map((sufixo) => `app.questionnaire.update.${sufixo}`);

const elevationKeys = ['title', 'description', 'prompt', 'submit', 'cancel'].map(
  (sufixo) => `app.questionnaire.updateElevation.${sufixo}`
);

/** Confirmação de execução de comando shell (AEP-0091, kind=decision). */
const shellKeys = [
  'title',
  'prompt',
  'workDir',
  'submit',
  'cancel',
].map((sufixo) => `app.questionnaire.shell.${sufixo}`);

/** Confirmação HTTP mutável (AEP-0091 Fase 3). */
const httpKeys = ['title', 'prompt', 'submit', 'cancel'].map(
  (sufixo) => `app.questionnaire.http.${sufixo}`,
);

/** Exclusão de mensagem (AEP-0091 Fase 3). */
const deleteMessageKeys = ['title', 'description', 'submit', 'cancel'].map(
  (sufixo) => `app.questionnaire.deleteMessage.${sufixo}`,
);

/** Consentimento de acesso a host bloqueado pelo anti-SSRF (AEP-0082). */
const networkKeys = [
  'title',
  'description',
  'submit',
  'cancel',
  'detailsPrompt',
  'skillHostMatch',
  'scopePrompt',
  'reasonPrompt',
  'reasonPlaceholder',
  'scope.once',
  'scope.session',
  'scope.workspace',
  'scope.profile',
  'scope.global',
].map((sufixo) => `app.questionnaire.network.${sufixo}`);

const welcomeKeys = [
  'submitContinue',
  'submitNext',
  'submitFinish',
  'cancel',
  'back',
  'passwordTitle',
  'passwordDescription',
  'passwordPrompt',
  'passwordPlaceholder',
  'passwordConfirmPrompt',
  'passwordConfirmPlaceholder',
  'passwordMismatch',
  'recoveryTitle',
  'recoveryDescription',
  'recoveryPrompt',
  'recoveryConfirmPrompt',
  'providerTitle',
  'providerDescription',
  'providerPrompt',
  'providerOptionOther',
  'urlTitle',
  'urlDescription',
  'urlPrompt',
  'urlInvalid',
  'urlUnreachable',
  'apiKeyTitle',
  'apiKeyDescription',
  'apiKeyDescriptionLocal',
  'apiKeyPrompt',
  'connectionFailed',
  'authRequired',
  'authInvalid',
  'serverError',
  'modelTitle',
  'modelDescription',
  'modelPrompt',
  'modelManualTitle',
  'modelManualDescription',
  'modelManualPrompt',
].map((sufixo) => `app.questionnaire.welcome.${sufixo}`);

/**
 * Valores que o backend manda interpolar. A tradução que não os usa perde o
 * dado do pedido: "Assunto:" sem assunto, "Pergunta de" sem número, a
 * confirmação sem o arquivo que ela vai alterar.
 */
const requiredPlaceholders: Record<string, string[]> = {
  'app.questionnaire.agentQuestion.descriptionSubject': ['{{subject}}'],
  'app.questionnaire.agentQuestion.promptLabelNumbered': ['{{position}}', '{{total}}'],
  'app.questionnaire.agentPlan.descriptionSteps': ['{{steps}}'],
  'app.questionnaire.agentPlan.descriptionProjectSteps': ['{{steps}}'],
  'app.questionnaire.editConfirmation.description': ['{{path}}'],
  'app.questionnaire.editConfirmation.descriptionNotes': ['{{path}}', '{{notes}}'],
  'app.questionnaire.update.description': ['{{current}}', '{{latest}}'],
  'app.questionnaire.update.descriptionNotes': ['{{current}}', '{{latest}}', '{{notes}}'],
  'app.questionnaire.update.descriptionSize': ['{{current}}', '{{latest}}', '{{size}}'],
  'app.questionnaire.update.descriptionNotesSize': [
    '{{current}}',
    '{{latest}}',
    '{{notes}}',
    '{{size}}',
  ],
  'app.questionnaire.welcome.urlInvalid': ['{{detail}}'],
  'app.questionnaire.welcome.urlUnreachable': ['{{detail}}'],
  'app.questionnaire.welcome.connectionFailed': ['{{provider}}', '{{url}}', '{{detail}}'],
  'app.questionnaire.welcome.authRequired': ['{{detail}}'],
  'app.questionnaire.welcome.authInvalid': ['{{detail}}'],
  'app.questionnaire.welcome.serverError': ['{{detail}}'],
  'app.questionnaire.welcome.modelDescription': ['{{models}}'],
  'app.questionnaire.network.description': ['{{category}}'],
  'app.questionnaire.network.skillHostMatch': ['{{pattern}}'],
  'app.questionnaire.shell.workDir': ['{{workDir}}'],
};

/**
 * Nomes que o i18next reserva: um parâmetro assim mudaria a pluralização, a
 * variante ou o idioma da frase, e não o valor interpolado (AEP-0085 D2). O
 * backend não os usa, e a tradução também não pode esperá-los.
 */
const reservedPlaceholders = ['{{count}}', '{{context}}', '{{lng}}'];

function getLocaleValue(locale: unknown, key: string): unknown {
  const root = (locale as { translation: Record<string, unknown> }).translation;
  return key.split('.').reduce<unknown>((current, part) => {
    if (!current || typeof current !== 'object') return undefined;
    return (current as Record<string, unknown>)[part];
  }, root);
}

describe('chaves dos diálogos que o backend monta', () => {
  const keys = [
    ...permissionKeys,
    ...questionKeys,
    ...planKeys,
    ...editConfirmationKeys,
    ...updateKeys,
    ...elevationKeys,
    ...shellKeys,
    ...httpKeys,
    ...deleteMessageKeys,
    ...networkKeys,
    ...welcomeKeys,
  ];

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

  it.each(Object.entries(localeModules))('não espera parâmetro reservado em %s', (_nome, locale) => {
    for (const key of keys) {
      const value = getLocaleValue(locale, key) as string;
      for (const reserved of reservedPlaceholders) {
        expect(value, `${key} interpola ${reserved}`).not.toContain(reserved);
      }
    }
  });
});
