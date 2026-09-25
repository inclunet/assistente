import { afterEach, beforeAll, describe, expect, it } from 'vitest';
import i18next from 'i18next';
import ptBR from '../locales/pt-BR';
import en from '../locales/en';
import es from '../locales/es';
import {
  appendEditorSlideMarkdown,
  buildEditorSlideTemplate,
  EDITOR_SLIDE_COMMAND_IDS,
  isEditorSlideCommand,
} from './commandEditorSlideTemplates';

describe('commandEditorSlideTemplates', () => {
  beforeAll(async () => {
    i18next.addResourceBundle('pt-BR', 'translation', ptBR.translation, true, true);
    i18next.addResourceBundle('en', 'translation', en.translation, true, true);
    i18next.addResourceBundle('es', 'translation', es.translation, true, true);
    await i18next.changeLanguage('pt-BR');
  });

  afterEach(async () => {
    await i18next.changeLanguage('pt-BR');
  });

  it('expõe exatamente os onze comandos de slide', () => {
    expect(EDITOR_SLIDE_COMMAND_IDS).toHaveLength(11);
    expect(new Set(EDITOR_SLIDE_COMMAND_IDS).size).toBe(11);
    for (const id of EDITOR_SLIDE_COMMAND_IDS) {
      expect(isEditorSlideCommand(id)).toBe(true);
      expect(buildEditorSlideTemplate(id)).toBeTruthy();
    }
  });

  it.each(['pt-BR', 'en', 'es'] as const)('constrói todos os templates em %s', async (language) => {
    await i18next.changeLanguage(language);
    const localized = {
      'pt-BR': { basic: 'Novo slide', title: 'Título', diagramStart: 'Início' },
      en: { basic: 'New slide', title: 'Title', diagramStart: 'Start' },
      es: { basic: 'Nueva diapositiva', title: 'Título', diagramStart: 'Inicio' },
    }[language];
    for (const id of EDITOR_SLIDE_COMMAND_IDS) {
      const template = buildEditorSlideTemplate(id);
      expect(template).toEqual(expect.any(String));
      expect(template?.trim()).not.toBe('');
    }
    expect(buildEditorSlideTemplate('editor.slide.insert.basic')).toContain(localized.basic);
    expect(buildEditorSlideTemplate('editor.slide.insert.title')).toContain(localized.title);
    expect(buildEditorSlideTemplate('editor.slide.insert.diagram')).toContain(localized.diagramStart);
  });

  it('rejeita IDs inválidos', () => {
    expect(isEditorSlideCommand('editor.slide.insert.unknown')).toBe(false);
    expect(buildEditorSlideTemplate('editor.slide.insert.unknown')).toBeUndefined();
    expect(buildEditorSlideTemplate('')).toBeUndefined();
  });

  it('anexa ao documento vazio com newline final', () => {
    expect(appendEditorSlideMarkdown('', '  slide  ')).toBe('slide\n');
    expect(appendEditorSlideMarkdown('\n', '\nslide\n')).toBe('slide\n');
  });

  it('insere um separador entre slides sem duplicá-lo', () => {
    expect(appendEditorSlideMarkdown('# Um\n', '# Dois')).toBe('# Um\n\n---\n\n# Dois\n');
    expect(appendEditorSlideMarkdown('# Um\n\n---\n', '# Dois')).toBe('# Um\n\n---\n\n# Dois\n');
    expect(appendEditorSlideMarkdown('# Um\n\n----\n\n', '# Dois')).toBe('# Um\n\n----\n\n# Dois\n');
  });
});
