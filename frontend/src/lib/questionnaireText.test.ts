import { describe, expect, it } from 'vitest';
import i18n from 'i18next';

import {
  questionnaireOptionValue,
  resolveQuestionnaireText,
  type QuestionnaireText,
} from './questionnaireText';

/** Instância isolada com apenas as chaves que os testes precisam. */
async function traduzindoEm(lng: string, resources: Record<string, unknown>) {
  const instance = i18n.createInstance();
  await instance.init({
    lng,
    fallbackLng: 'en',
    resources: { [lng]: { translation: resources } },
    interpolation: { escapeValue: false },
    initImmediate: false,
  });
  return instance.t.bind(instance);
}

describe('resolveQuestionnaireText', () => {
  it('traduz quando a chave existe no idioma', async () => {
    const t = await traduzindoEm('en', {
      app: { questionnaire: { shell: { title: 'Confirm command execution' } } },
    });

    const texto: QuestionnaireText = {
      key: 'app.questionnaire.shell.title',
      fallback: 'Confirmar execução de comando',
    };

    expect(resolveQuestionnaireText(t, texto)).toBe('Confirm command execution');
  });

  it('cai no texto do backend quando a chave não existe', async () => {
    const t = await traduzindoEm('en', {});

    const texto: QuestionnaireText = {
      key: 'app.questionnaire.shell.title',
      fallback: 'Confirmar execução de comando',
    };

    // Nunca vazio: o diálogo é lido por leitor de telas e um título em branco
    // esconderia o que está sendo autorizado.
    expect(resolveQuestionnaireText(t, texto)).toBe('Confirmar execução de comando');
  });

  it('interpola os parâmetros na tradução', async () => {
    const t = await traduzindoEm('en', {
      app: {
        questionnaire: {
          http: { title: 'Confirm {{method}} operation' },
        },
      },
    });

    const texto: QuestionnaireText = {
      key: 'app.questionnaire.http.title',
      params: { method: 'DELETE' },
      fallback: 'Confirmar operação DELETE',
    };

    expect(resolveQuestionnaireText(t, texto)).toBe('Confirm DELETE operation');
  });

  it('parâmetro com nome reservado do i18next não muda a tradução', async () => {
    const t = await traduzindoEm('en', {
      app: { questionnaire: { shell: { prompt: 'Allow running {{count}}?' } } },
    });

    const texto: QuestionnaireText = {
      key: 'app.questionnaire.shell.prompt',
      params: { count: 'this command' },
      fallback: 'Permitir a execução deste comando?',
    };

    expect(resolveQuestionnaireText(t, texto)).toBe('Allow running this command?');
  });

  it('texto puro do backend vai direto para a tela', async () => {
    const t = await traduzindoEm('en', {});

    expect(resolveQuestionnaireText(t, 'Antes')).toBe('Antes');
  });

  it('sem texto nenhum usa o padrão de quem chamou', async () => {
    const t = await traduzindoEm('en', { ui: { questionnaire: { submit: 'Send' } } });

    expect(resolveQuestionnaireText(t, undefined, 'padrão')).toBe('padrão');
    expect(resolveQuestionnaireText(t, '', 'padrão')).toBe('padrão');
    expect(resolveQuestionnaireText(t, { fallback: '' }, 'padrão')).toBe('padrão');
  });

  it('chave sem fallback e sem tradução ainda diz algo', async () => {
    const t = await traduzindoEm('en', {});

    expect(resolveQuestionnaireText(t, { key: 'app.questionnaire.shell.title' })).toBe(
      'app.questionnaire.shell.title'
    );
  });
});

describe('questionnaireOptionValue', () => {
  it('devolve o texto do backend, não a tradução', () => {
    expect(
      questionnaireOptionValue({
        key: 'app.questionnaire.network.scope.session',
        fallback: 'session — Durante esta conversa',
      })
    ).toBe('session — Durante esta conversa');
  });

  it('opção de texto puro é o próprio valor', () => {
    expect(questionnaireOptionValue('Permitir uma vez')).toBe('Permitir uma vez');
  });

  it('opção só com chave usa a chave como valor', () => {
    expect(questionnaireOptionValue({ key: 'x.y' })).toBe('x.y');
  });
});
