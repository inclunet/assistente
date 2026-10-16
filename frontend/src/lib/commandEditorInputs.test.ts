import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';
import { useUIStore } from '../store/uiStore';
import {
  requestEditorCommandInput,
  validateEditorCommandInput,
  type EditorInputContext,
} from './commandEditorInputs';

const linkContext: EditorInputContext = {
  existingHref: '',
  selectedText: 'texto selecionado',
  selectionEmpty: false,
};

function resetStores() {
  useQuestionnaireUIStore.setState({ active: null, activeScope: null, queue: [], _activeResolve: null });
  useUIStore.setState({ toasts: [] });
}

async function answerActive(answers: Record<string, unknown>) {
  const active = useQuestionnaireUIStore.getState().active;
  expect(active).not.toBeNull();
  useQuestionnaireUIStore.getState().submit(answers);
}

beforeEach(resetStores);
afterEach(resetStores);

describe('commandEditorInputs', () => {
  it('Markdown pede somente dimensões e fixa cabeçalho obrigatório', async () => {
    const pending = requestEditorCommandInput('editor.format.table.insert', { ...linkContext, tableHeaderRequired: true }, new AbortController().signal);
    expect(useQuestionnaireUIStore.getState().active?.questions.map(question => question.id)).toEqual(['rows', 'cols']);
    await answerActive({ rows: '3', cols: '4' });
    await expect(pending).resolves.toEqual({ kind: 'table', rows: 3, cols: 4, withHeaderRow: true });
  });
  it('valida schemas estritos de link e tabela', () => {
    expect(validateEditorCommandInput('editor.format.link.set', {
      kind: 'link', href: 'https://example.com', text: 'site',
    })).toBe(true);
    expect(validateEditorCommandInput('editor.format.link.set', {
      kind: 'link', href: 'javascript:alert(1)',
    })).toBe(false);
    expect(validateEditorCommandInput('editor.format.link.set', {
      kind: 'link', href: 'https://example.com', extra: true,
    })).toBe(false);
    expect(validateEditorCommandInput('editor.format.table.insert', {
      kind: 'table', rows: 2, cols: 6, withHeaderRow: false,
    })).toBe(true);
    expect(validateEditorCommandInput('editor.format.table.insert', {
      kind: 'table', rows: 1, cols: 2, withHeaderRow: false,
    })).toBe(false);
    expect(validateEditorCommandInput('editor.format.table.insert', {
      kind: 'table', rows: 2.5, cols: 2, withHeaderRow: false,
    })).toBe(false);
    expect(validateEditorCommandInput('editor.format.table.insert', {
      kind: 'table', rows: 2, cols: '2', withHeaderRow: false,
    })).toBe(false);
    expect(validateEditorCommandInput('editor.format.table.insert', {
      kind: 'table', rows: 2, cols: 2, withHeaderRow: 'false',
    })).toBe(false);
  });

  it('abre o questionnaire real e retorna link sem editar editor', async () => {
    const pending = requestEditorCommandInput('editor.format.link.set', linkContext, new AbortController().signal);
    const active = useQuestionnaireUIStore.getState().active;
    expect(active?.questions.map(question => question.id)).toEqual(['href']);
    await answerActive({ href: ' https://example.com/path ' });
    await expect(pending).resolves.toEqual({ kind: 'link', href: 'https://example.com/path' });
  });

  it('não pede texto ao editar link existente e usa defaults do contexto', async () => {
    const pending = requestEditorCommandInput('editor.format.link.set', {
      existingHref: 'https://old.example', selectedText: '', selectionEmpty: true,
    }, new AbortController().signal);
    const active = useQuestionnaireUIStore.getState().active;
    expect(active?.questions.map(question => question.id)).toEqual(['href']);
    expect(active?.questions[0].default).toBe('https://old.example');
    await answerActive({ href: 'https://new.example' });
    await expect(pending).resolves.toEqual({ kind: 'link', href: 'https://new.example' });
  });

  it('converte somente escolhas exatas da tabela e preserva booleano real', async () => {
    const pending = requestEditorCommandInput('editor.format.table.insert', linkContext, new AbortController().signal);
    const active = useQuestionnaireUIStore.getState().active;
    expect(active?.questions[0].options?.map(option => typeof option === 'string' ? option : option.fallback)).toEqual(['2', '3', '4', '5', '6']);
    await answerActive({ rows: '3', cols: '6', withHeaderRow: true });
    await expect(pending).resolves.toEqual({ kind: 'table', rows: 3, cols: 6, withHeaderRow: true });
  });

  it('recusa URL insegura e bounds/tipos inválidos com toast', async () => {
    const invalidLink = requestEditorCommandInput('editor.format.link.set', linkContext, new AbortController().signal);
    await answerActive({ href: 'javascript:alert(1)' });
    await expect(invalidLink).resolves.toBeUndefined();
    expect(useUIStore.getState().toasts).toHaveLength(1);

    const invalidTable = requestEditorCommandInput('editor.format.table.insert', linkContext, new AbortController().signal);
    await answerActive({ rows: '1', cols: '2', withHeaderRow: 'false' });
    await expect(invalidTable).resolves.toBeUndefined();
    expect(useUIStore.getState().toasts).toHaveLength(2);
  });

  it('cancela request ativo e request enfileirado por AbortSignal', async () => {
    const firstController = new AbortController();
    const first = requestEditorCommandInput('editor.format.link.set', linkContext, firstController.signal);
    const secondController = new AbortController();
    const second = requestEditorCommandInput('editor.format.table.insert', linkContext, secondController.signal);
    expect(useQuestionnaireUIStore.getState().queue).toHaveLength(1);

    secondController.abort();
    await expect(second).resolves.toBeUndefined();
    expect(useQuestionnaireUIStore.getState().queue).toHaveLength(0);
    expect(useQuestionnaireUIStore.getState().active).not.toBeNull();

    firstController.abort();
    await expect(first).resolves.toBeUndefined();
    expect(useQuestionnaireUIStore.getState().active).toBeNull();
  });

  it('ignora resposta tardia depois do abort e não mantém pending UI', async () => {
    const controller = new AbortController();
    const pending = requestEditorCommandInput('editor.format.table.insert', linkContext, controller.signal);
    const id = useQuestionnaireUIStore.getState().active?.id;
    expect(id).toBeTruthy();
    controller.abort();
    await expect(pending).resolves.toBeUndefined();

    useQuestionnaireUIStore.getState().submit({ rows: '6', cols: '6', withHeaderRow: true });
    expect(useQuestionnaireUIStore.getState().active).toBeNull();
    expect(useQuestionnaireUIStore.getState().queue).toHaveLength(0);
  });
});
