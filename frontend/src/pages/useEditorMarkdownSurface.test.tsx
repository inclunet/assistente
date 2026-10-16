import { act, renderHook } from '@testing-library/react';
import { afterEach, describe, expect, it } from 'vitest';
import { captureEditorFormatting } from '../lib/commandEditorFormatting';
import { useAuthStore } from '../store/authStore';
import { useEditorStore, type EditorDocument } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useEditorMarkdownSurface } from './useEditorMarkdownSurface';
import { useQuestionnaireUIStore } from '../store/questionnaireUIStore';

const documentId = 'markdown-doc';
const userId = 'markdown-user';
const sessionId = 'markdown-session';
const workspaceId = 'markdown-workspace';

function fixture() {
  let value = 'alpha';
  let version = 1;
  const modelListeners = new Set<() => void>();
  const selectionListeners = new Set<() => void>();
  const contentListeners = new Set<() => void>();
  const dom = document.createElement('div');
  dom.className = 'monaco-editor';
  document.body.append(dom);
  const model = {
    isDisposed: () => false,
    getVersionId: () => version,
    getValueInRange: () => value,
    isReadonly: () => false,
    onDidChangeContent: (listener: () => void) => { contentListeners.add(listener); return { dispose: () => contentListeners.delete(listener) }; },
  };
  const selection = { startLineNumber: 1, startColumn: 1, endLineNumber: 1, endColumn: 6 };
  const editor = {
    onDidDispose: () => ({ dispose() {} }),
    onDidChangeConfiguration: () => ({ dispose() {} }),
    getModel: () => model,
    getSelection: () => ({ ...selection }),
    getDomNode: () => dom,
    getOption: () => false,
    onDidChangeCursorSelection: (listener: () => void) => { selectionListeners.add(listener); return { dispose: () => selectionListeners.delete(listener) }; },
    onDidChangeModel: (listener: () => void) => { modelListeners.add(listener); return { dispose: () => modelListeners.delete(listener) }; },
    onDidCompositionStart: () => ({ dispose: () => undefined }),
    executeEdits: () => { version += 1; contentListeners.forEach(listener => listener()); return true; },
    pushUndoStop: () => true,
    replaceModel: () => modelListeners.forEach(listener => listener()),
    replaceSelection: () => selectionListeners.forEach(listener => listener()),
  };
  const monaco = { editor: { EditorOption: { readOnly: 'readOnly' } } };
  return { editor, monaco, dom, replaceModel: editor.replaceModel, replaceSelection: editor.replaceSelection };
}

function seed(document: EditorDocument = {
  id: documentId, title: 'Markdown', markdown: 'alpha', mode: 'markdown', readOnly: false, loadError: false,
}) {
  useAuthStore.setState({ user: { userId, sessionId, role: 'user' }, isAuthenticated: true });
  useEditorStore.setState({ ownerUserId: userId, documents: { [documentId]: document } });
  useWorkspaceStore.setState({ workspace: {
    id: workspaceId, name: 'Markdown', activeTabId: documentId,
    tabs: [{ id: documentId, type: 'editor', title: 'Markdown', position: 0 }],
  } });
}

afterEach(() => {
  useAuthStore.setState({ user: null, isAuthenticated: false });
  useEditorStore.setState({ ownerUserId: null, documents: {} });
  useWorkspaceStore.setState({ workspace: null });
  document.body.replaceChildren();
});

describe('useEditorMarkdownSurface', () => {
  it('desmontagem cancela formulário pendente mesmo sem retorno do executor', async () => {
    seed();
    const f = fixture();
    const hook = renderHook(() => useEditorMarkdownSurface({ current: f.dom }, { current: f.editor as never }, { current: f.monaco as never }, documentId, true, 1, false));
    const target = captureEditorFormatting();
    const preparing = target!.prepare('editor.format.table.insert');
    expect(useQuestionnaireUIStore.getState().active).not.toBeNull();
    hook.unmount();
    expect(await preparing).toBe(false);
    expect(useQuestionnaireUIStore.getState().active).toBeNull();
    expect(target?.isCurrent()).toBe(false);
  });
  it.each(['session', 'workspace', 'document'] as const)('invalida ABA de %s e aceita nova captura após edição normal', kind => {
    seed();
    const f = fixture();
    const hook = renderHook(() => useEditorMarkdownSurface({ current: f.dom }, { current: f.editor as never }, { current: f.monaco as never }, documentId, true, 1, false));
    const target = captureEditorFormatting();
    const auth = useAuthStore.getState().user;
    const ws = useWorkspaceStore.getState().workspace!;
    const original = useEditorStore.getState().documents[documentId];
    act(() => {
      if (kind === 'session') {
        useAuthStore.setState({ user: { ...auth!, sessionId: 'other' } });
        useAuthStore.setState({ user: auth });
      } else if (kind === 'workspace') {
        useWorkspaceStore.setState({ workspace: { ...ws, activeTabId: 'other' } });
        useWorkspaceStore.setState({ workspace: ws });
      } else {
        useEditorStore.setState({ documents: { [documentId]: { ...original, markdown: 'new' } } });
        useEditorStore.setState({ documents: { [documentId]: original } });
      }
    });
    expect(target?.isCurrent()).toBe(false);
    // The auth store deliberately clears editor ownership on session change.
    // Rehydrate the same session/document before testing a fresh capture.
    if (kind === 'session') seed(original);
    const next = captureEditorFormatting();
    expect(next?.isCurrent()).toBe(true);
    target?.dispose(); next?.dispose(); hook.unmount();
  });
  it('registra apenas o Monaco Markdown proprietário e captura o documento exato', () => {
    seed();
    const f = fixture();
    const rootRef = { current: f.dom };
    const editorRef = { current: f.editor as never };
    const monacoRef = { current: f.monaco as never };
    const hook = renderHook(() => useEditorMarkdownSurface(rootRef, editorRef, monacoRef, documentId, true, 1, false));
    const original = useEditorStore.getState().documents[documentId];
    const target = captureEditorFormatting(original);
    expect(target?.canExecute('editor.format.blockquote')).toBe(true);
    expect(captureEditorFormatting({ ...original })).toBeUndefined();
    target?.dispose();
    hook.unmount();
  });

  it.each([
    ['readonly', () => { useEditorStore.setState(state => ({ documents: { ...state.documents, [documentId]: { ...state.documents[documentId]!, readOnly: true } } })); }],
    ['wrong mode', () => { useEditorStore.setState(state => ({ documents: { ...state.documents, [documentId]: { ...state.documents[documentId]!, mode: 'rich' } } })); }],
    ['owner', () => { useEditorStore.setState({ ownerUserId: 'other-user' }); }],
    ['inactive', () => { /* rerender handles this case */ }],
  ] as const)('invalida quando muda %s', (_name, mutate) => {
    seed();
    const f = fixture();
    const rootRef = { current: f.dom };
    const editorRef = { current: f.editor as never };
    const monacoRef = { current: f.monaco as never };
    const hook = renderHook(({ active }) => useEditorMarkdownSurface(rootRef, editorRef, monacoRef, documentId, active, 1, false), { initialProps: { active: true } });
    const target = captureEditorFormatting(useEditorStore.getState().documents[documentId]);
    if (_name === 'inactive') act(() => hook.rerender({ active: false }));
    else act(mutate);
    expect(target?.isCurrent()).toBe(false);
    hook.unmount();
  });

  it('desmonta e invalida a inscrição sem retargetear', () => {
    seed();
    const f = fixture();
    const rootRef = { current: f.dom };
    const editorRef = { current: f.editor as never };
    const monacoRef = { current: f.monaco as never };
    const hook = renderHook(() => useEditorMarkdownSurface(rootRef, editorRef, monacoRef, documentId, true, 1, false));
    const target = captureEditorFormatting(useEditorStore.getState().documents[documentId]);
    hook.unmount();
    expect(target?.isCurrent()).toBe(false);
    expect(captureEditorFormatting()).toBeUndefined();
  });
});
