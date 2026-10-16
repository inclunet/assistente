import { act, renderHook } from '@testing-library/react';
import { Editor } from '@tiptap/core';
import { afterEach, describe, expect, it } from 'vitest';

import { buildRichTextExtensions } from '../components/editor/buildRichTextExtensions';
import { captureEditorFormatting } from '../lib/commandEditorFormatting';
import { useAuthStore } from '../store/authStore';
import { useEditorStore, type EditorDocument } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useEditorFormattingSurface } from './useEditorFormattingSurface';

const documentId = 'doc-formatting';
const userId = 'user-formatting';
const sessionId = 'session-formatting';
const workspaceId = 'workspace-formatting';

function createEditor(): Editor {
  return new Editor({
    extensions: buildRichTextExtensions({
      placeholder: 'placeholder',
      imageFallbackLabel: 'image',
      imageLabelPrefix: 'image',
    }),
    content: '<p>hello formatting</p>',
  });
}

function seedStores(document: EditorDocument = {
  id: documentId,
  title: 'Formatting',
  markdown: 'hello formatting',
  mode: 'rich',
  readOnly: false,
  loadError: false,
}) {
  useAuthStore.setState({
    user: { userId, sessionId, role: 'user' },
    isAuthenticated: true,
  });
  useEditorStore.setState({ ownerUserId: userId, documents: { [documentId]: document } });
  useWorkspaceStore.setState({
    workspace: {
      id: workspaceId,
      name: 'Formatting workspace',
      activeTabId: documentId,
      tabs: [{ id: documentId, type: 'editor', title: 'Formatting', position: 0 }],
    },
  });
}

function mountSurface(active = true) {
  const root = document.createElement('div');
  document.body.appendChild(root);
  const editor = createEditor();
  const rootRef = { current: root };
  const editorRef = { current: editor };
  const hook = renderHook(
    ({ isActive }) => useEditorFormattingSurface(rootRef, editorRef, documentId, isActive, 1, false),
    { initialProps: { isActive: active } },
  );
  return { ...hook, root, editor, rootRef, editorRef };
}

afterEach(() => {
  useAuthStore.setState({ user: null, isAuthenticated: false });
  useEditorStore.setState({ ownerUserId: null, documents: {} });
  useWorkspaceStore.setState({ workspace: null });
  document.body.replaceChildren();
});

describe('useEditorFormattingSurface', () => {
  it('captura e executa formatação com Tiptap real e extensões de produção', () => {
    seedStores();
    const surface = mountSurface();
    surface.editor.commands.setTextSelection({ from: 1, to: 6 });

    const target = captureEditorFormatting();
    expect(target).toBeDefined();
    expect(target?.isCurrent()).toBe(true);
    expect(target?.canExecute('editor.format.bold')).toBe(true);
    expect(target?.execute('editor.format.bold')).toBe(true);
    expect(surface.editor.isActive('bold')).toBe(true);

    target?.dispose();
    surface.unmount();
    surface.editor.destroy();
  });

  it.each([
    ['ownerUserId', () => useEditorStore.setState({ ownerUserId: 'other-user' })],
    ['different auth session', () => useAuthStore.setState({ user: { userId, sessionId: 'other-session', role: 'user' } })],
    ['active-tab ABA', () => {
      useWorkspaceStore.setState(state => ({ workspace: state.workspace && { ...state.workspace, activeTabId: 'other-tab' } }));
      useWorkspaceStore.setState(state => ({ workspace: state.workspace && { ...state.workspace, activeTabId: documentId } }));
    }],
    ['store document replacement with same ID', () => useEditorStore.setState(state => ({
      documents: { ...state.documents, [documentId]: { ...state.documents[documentId]!, title: 'replacement' } },
    }))],
    ['readonly', () => useEditorStore.setState(state => ({
      documents: { ...state.documents, [documentId]: { ...state.documents[documentId]!, readOnly: true } },
    }))],
  ] as const)('cancela captura após %s', (_name, invalidate) => {
    seedStores();
    const surface = mountSurface();
    surface.editor.commands.setTextSelection({ from: 1, to: 6 });
    const target = captureEditorFormatting();
    expect(target?.isCurrent()).toBe(true);

    act(invalidate);

    expect(target?.isCurrent()).toBe(false);
    expect(target?.execute('editor.format.bold')).toBe(false);
    surface.unmount();
    surface.editor.destroy();
  });

  it('cancela ao ficar inativo e limpa ao desmontar', () => {
    seedStores();
    const surface = mountSurface();
    surface.editor.commands.setTextSelection({ from: 1, to: 6 });
    const target = captureEditorFormatting();
    expect(target?.isCurrent()).toBe(true);

    act(() => surface.rerender({ isActive: false }));
    expect(target?.isCurrent()).toBe(false);
    expect(captureEditorFormatting()).toBeUndefined();

    act(() => surface.rerender({ isActive: true }));
    const activeTarget = captureEditorFormatting();
    expect(activeTarget?.isCurrent()).toBe(true);
    surface.unmount();
    expect(activeTarget?.isCurrent()).toBe(false);
    expect(captureEditorFormatting()).toBeUndefined();
    surface.editor.destroy();
  });
});
