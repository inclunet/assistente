import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { captureEditorFormatting } from '../lib/commandEditorFormatting';
import {
  appendEditorSlideMarkdown,
  buildEditorSlideTemplate,
  EDITOR_SLIDE_COMMAND_IDS,
} from '../lib/commandEditorSlideTemplates';
import { useAuthStore } from '../store/authStore';
import { useEditorStore, type EditorDocument } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useEditorSlideSurface } from './useEditorSlideSurface';

type Disposable = { dispose: () => void };
type Selection = { lineNumber: number; column: number; equalsSelection: (other: Selection) => boolean };

function makeSelection(lineNumber = 1, column = 1): Selection {
  return {
    lineNumber,
    column,
    equalsSelection(other) { return this.lineNumber === other.lineNumber && this.column === other.column; },
  };
}

function makeMonacoFixture(initial = '# Existing') {
  const root = document.createElement('div');
  root.className = 'monaco-editor';
  document.body.append(root);

  let value = initial;
  let version = 1;
  let readonly = false;
  let model: ReturnType<typeof createModel>;
  let selection = makeSelection();
  const modelListeners = new Set<() => void>();
  const selectionListeners = new Set<() => void>();
  const compositionListeners = new Set<() => void>();
  const disposeListeners = new Set<() => void>();
  const contentListeners = new Set<() => void>();
  let executeEditsResult = true;
  const executeEdits = vi.fn((_source: string, edits: Array<{ range: unknown; text: string }>) => {
    if (!executeEditsResult) return false;
    value = edits[0]?.text ?? value;
    version += 1;
    contentListeners.forEach(listener => listener());
    return true;
  });

  function createModel() {
    let modelDisposed = false;
    const current = {
      getVersionId: () => version,
      getValue: () => value,
      getFullModelRange: () => ({ startLineNumber: 1, startColumn: 1, endLineNumber: 1, endColumn: value.length + 1 }),
      getPositionAt: (offset: number) => ({ lineNumber: 1, column: offset + 1 }),
      isDisposed: () => modelDisposed,
      dispose: () => {
        modelDisposed = true;
        disposeListeners.forEach(listener => listener());
      },
    };
    return current;
  }

  model = createModel();
  const editor = {
    getModel: () => model,
    getSelection: () => ({ ...selection, equalsSelection: selection.equalsSelection }),
    getDomNode: () => root,
    getOption: () => readonly,
    onDidChangeModel: (listener: () => void): Disposable => {
      modelListeners.add(listener);
      return { dispose: () => modelListeners.delete(listener) };
    },
    onDidChangeCursorSelection: (listener: () => void): Disposable => {
      selectionListeners.add(listener);
      return { dispose: () => selectionListeners.delete(listener) };
    },
    onDidCompositionStart: (listener: () => void): Disposable => {
      compositionListeners.add(listener);
      return { dispose: () => compositionListeners.delete(listener) };
    },
    onDidDispose: (listener: () => void): Disposable => {
      disposeListeners.add(listener);
      return { dispose: () => disposeListeners.delete(listener) };
    },
    onDidChangeModelContent: (listener: () => void): Disposable => {
      contentListeners.add(listener);
      return { dispose: () => contentListeners.delete(listener) };
    },
    pushUndoStop: vi.fn(() => true),
    executeEdits,
    setPosition: vi.fn(),
    revealPositionInCenter: vi.fn(),
    focus: vi.fn(),
    replaceSelection(next: Selection = makeSelection(1, 2)) {
      selection = next;
      selectionListeners.forEach(listener => listener());
    },
    replaceModel(next = createModel()) {
      model = next;
      modelListeners.forEach(listener => listener());
    },
    emitCompositionStart() {
      compositionListeners.forEach(listener => listener());
    },
    setReadonly(next: boolean) { readonly = next; },
    disposeModel() { model.dispose(); },
  };
  return {
    root,
    editor,
    model: () => model,
    monaco: { editor: { EditorOption: { readOnly: 'readOnly' } } },
    getValue: () => value,
    setExecuteEditsResult: (next: boolean) => { executeEditsResult = next; },
  };
}

function seedStore(document: EditorDocument = {
  id: 'markdown-slide-doc',
  title: 'Slides',
  markdown: '# Existing',
  mode: 'markdown',
  readOnly: false,
  loadError: false,
}) {
  useAuthStore.setState({ user: { userId: 'user', sessionId: 'session', role: 'user' }, isAuthenticated: true });
  useEditorStore.setState({ ownerUserId: 'user', documents: { [document.id]: document } });
  useWorkspaceStore.setState({ workspace: {
    id: 'workspace',
    name: 'Workspace',
    activeTabId: document.id,
    tabs: [{ id: document.id, title: document.title, type: 'editor', position: 0 }],
  } });
}

function mountSurface(fixture = makeMonacoFixture()) {
  seedStore();
  const root = { current: fixture.root };
  const rich = { current: null };
  const markdown = { current: fixture.editor as never };
  const monaco = { current: fixture.monaco as never };
  const composing = { current: false };
  const flushRich = vi.fn(() => true);
  const commitRich = vi.fn();
  const appended = vi.fn();
  const hook = renderHook(({ active, asking, ready }) => useEditorSlideSurface({
    root,
    rich,
    markdown,
    monaco,
    documentId: 'markdown-slide-doc',
    active,
    ready,
    asking,
    composing,
    flushRich,
    commitRich,
    appended,
  }), { initialProps: { active: true, asking: false, ready: 1 } });
  return { ...fixture, ...hook, root, rich, markdown, monaco, composing, flushRich, commitRich, appended };
}

afterEach(() => {
  useAuthStore.setState({ user: null, isAuthenticated: false });
  useEditorStore.setState({ ownerUserId: null, documents: {} });
  useWorkspaceStore.setState({ workspace: null });
  document.body.replaceChildren();
});

describe('useEditorSlideSurface em Markdown com Monaco', () => {
  it.each(EDITOR_SLIDE_COMMAND_IDS)('insere %s no documento completo, sem usar seleção', async (commandID) => {
    const surface = mountSurface();
    const original = surface.getValue();
    const target = captureEditorFormatting();
    const template = buildEditorSlideTemplate(commandID)!;

    expect(await target?.prepare(commandID)).toBe(true);
    expect(target?.execute(commandID)).toBe(true);
    expect(surface.getValue()).toBe(appendEditorSlideMarkdown(original, template));
    expect(surface.editor.executeEdits).toHaveBeenCalledWith('command-slide-insert', [{
      range: expect.any(Object),
      text: appendEditorSlideMarkdown(original, template),
      forceMoveMarkers: true,
    }]);
    expect(surface.editor.pushUndoStop).toHaveBeenCalledTimes(2);
    expect(surface.editor.setPosition).toHaveBeenCalledWith({ lineNumber: 1, column: surface.getValue().length + 1 });
    expect(surface.commitRich).not.toHaveBeenCalled();
    expect(surface.appended).toHaveBeenCalledTimes(1);
    expect(useEditorStore.getState().documents['markdown-slide-doc']?.markdown).toBe(original);
    target?.dispose();
    surface.unmount();
  });

  it('não notifica quando executeEdits retorna false', async () => {
    const surface = mountSurface();
    surface.setExecuteEditsResult(false);
    const target = captureEditorFormatting();

    expect(await target?.prepare('editor.slide.insert.basic')).toBe(true);
    expect(target?.execute('editor.slide.insert.basic')).toBe(false);
    expect(surface.appended).not.toHaveBeenCalled();
    expect(surface.commitRich).not.toHaveBeenCalled();
    expect(surface.editor.pushUndoStop).toHaveBeenCalledTimes(2);
    target?.dispose();
    surface.unmount();
  });

  it.each(['readonlyEditor', 'modeldispose', 'ABAmodel', 'selection', 'store', 'session', 'unmount'] as const)(
    'invalida a captura após %s', async (kind) => {
      const surface = mountSurface();
      const target = captureEditorFormatting();
      expect(await target?.prepare('editor.slide.insert.basic')).toBe(true);

      act(() => {
        if (kind === 'readonlyEditor') surface.editor.setReadonly(true);
        if (kind === 'modeldispose') surface.editor.disposeModel();
        if (kind === 'ABAmodel') surface.editor.replaceModel();
        if (kind === 'selection') surface.editor.replaceSelection();
        if (kind === 'store') useEditorStore.setState(state => ({ documents: { ...state.documents, 'markdown-slide-doc': { ...state.documents['markdown-slide-doc']! } } }));
        if (kind === 'session') useAuthStore.setState({ user: { userId: 'user', sessionId: 'other-session', role: 'user' } });
        if (kind === 'unmount') surface.unmount();
      });

      expect(target?.isCurrent()).toBe(false);
      expect(target?.execute('editor.slide.insert.basic')).toBe(false);
      expect(surface.appended).not.toHaveBeenCalled();
      target?.dispose();
      if (kind !== 'unmount') surface.unmount();
    },
  );

  it('não captura sem editor Monaco, mesmo com Markdown no store', () => {
    seedStore();
    const root = { current: document.body.appendChild(document.createElement('div')) };
    const hook = renderHook(() => useEditorSlideSurface({
      root,
      rich: { current: null },
      markdown: { current: null },
      monaco: { current: { editor: { EditorOption: { readOnly: 'readOnly' } } } as never },
      documentId: 'markdown-slide-doc',
      active: true,
      ready: 1,
      asking: false,
      composing: { current: false },
      flushRich: () => true,
      commitRich: vi.fn(),
      appended: vi.fn(),
    }));
    expect(captureEditorFormatting()).toBeUndefined();
    hook.unmount();
  });
});
