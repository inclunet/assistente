import { act, renderHook } from '@testing-library/react';
import { Editor } from '@tiptap/core';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { buildRichTextExtensions } from '../components/editor/buildRichTextExtensions';
import { captureEditorFormatting } from '../lib/commandEditorFormatting';
import { EDITOR_SLIDE_COMMAND_IDS } from '../lib/commandEditorSlideTemplates';
import { useAuthStore } from '../store/authStore';
import { useEditorStore, type EditorDocument } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useEditorSlideSurface } from './useEditorSlideSurface';
import { useEditorFormattingSurface } from './useEditorFormattingSurface';

const cleanups: Array<() => void> = [];
function mountSurface() {
  const doc: EditorDocument = { id: 'doc', title: 'Deck', markdown: '# Existing', mode: 'rich' };
  useAuthStore.setState({ user: { userId: 'u', sessionId: 's', role: 'user' }, isAuthenticated: true });
  useEditorStore.setState({ ownerUserId: 'u', documents: { doc } });
  useWorkspaceStore.setState({ workspace: { id: 'w', name: 'Workspace', activeTabId: 'doc', tabs: [{ id: 'doc', title: 'Deck', type: 'editor', position: 0 }] } });
  const root = document.createElement('div');
  document.body.append(root);
  const editor = new Editor({ extensions: buildRichTextExtensions({ placeholder: '', imageFallbackLabel: '', imageLabelPrefix: '' }), content: 'Existing' });
  const rootRef = { current: root };
  const rich = { current: editor };
  const composing = { current: false };
  const flushRich = vi.fn(() => {
    useEditorStore.getState().setDocMarkdown('doc', '# Latest flushed slide');
    return true;
  });
  const commitRich = vi.fn((id: string, text: string) => useEditorStore.getState().setDocMarkdown(id, text));
  const appended = vi.fn();
  const hook = renderHook(({ active, asking }) => {
    useEditorFormattingSurface(rootRef, rich, 'doc', active, 1, asking);
    useEditorSlideSurface({ root: rootRef, rich, markdown: { current: null }, monaco: { current: null }, documentId: 'doc', active, ready: 1,
      asking, composing, flushRich, commitRich, appended });
  }, { initialProps: { active: true, asking: false } });
  cleanups.push(() => { hook.unmount(); editor.destroy(); root.remove(); });
  return { ...hook, doc, root, editor, composing, flushRich, commitRich, appended };
}
afterEach(() => {
  cleanups.splice(0).forEach(cleanup => cleanup());
  useAuthStore.setState({ user: null, isAuthenticated: false });
  useEditorStore.setState({ ownerUserId: null, documents: {} });
  useWorkspaceStore.setState({ workspace: null });
});

describe('slides capturados e commit auditado', () => {
  it.each(EDITOR_SLIDE_COMMAND_IDS)('prepara %s sem inserir, commita uma única vez no documento completo', async id => {
    const surface = mountSurface();
    const target = captureEditorFormatting();
    expect(target?.canExecute('editor.format.bold')).toBe(true);
    expect(target?.canExecute(id)).toBe(true);
    expect(surface.flushRich).not.toHaveBeenCalled();
    expect(target?.execute(id)).toBe(false);
    expect(await target?.prepare(id)).toBe(true);
    expect(surface.flushRich).toHaveBeenCalledTimes(1);
    expect(surface.commitRich).not.toHaveBeenCalled();
    expect(target?.execute(id)).toBe(true);
    expect(surface.commitRich).toHaveBeenCalledWith('doc', expect.stringContaining('# Latest flushed slide\n\n---\n\n<!-- .slide:'));
    expect(surface.appended).toHaveBeenCalledTimes(1);
    expect(target?.execute(id)).toBe(false);
    target?.dispose();
  });
  it('o alvo composto preserva os comandos TipTap sem transformar slides em formatação rica', async () => {
    const surface = mountSurface();
    surface.editor.commands.setTextSelection({ from: 1, to: 5 });
    const target = captureEditorFormatting();
    expect(await target?.prepare('editor.format.bold')).toBe(true);
    expect(target?.execute('editor.format.bold')).toBe(true);
    expect(surface.editor.isActive('bold')).toBe(true);
    expect(surface.flushRich).not.toHaveBeenCalled();
    target?.dispose();
  });
  it.each(['selection', 'document', 'session', 'workspace', 'readonly', 'mode', 'composition', 'unmount'] as const)('cancela captura após mudança %s', async kind => {
    const surface = mountSurface();
    const target = captureEditorFormatting(surface.doc);
    expect(await target?.prepare('editor.slide.insert.basic')).toBe(true);
    act(() => {
      if (kind === 'selection') { surface.editor.commands.setTextSelection(3); surface.editor.commands.setTextSelection(1); }
      if (kind === 'document') useEditorStore.setState(state => ({ documents: { doc: { ...state.documents.doc } } }));
      if (kind === 'session') useAuthStore.setState({ user: { userId: 'u', sessionId: 'other', role: 'user' } });
      if (kind === 'workspace') {
        const workspace = useWorkspaceStore.getState().workspace!;
        useWorkspaceStore.setState({ workspace: { ...workspace, activeTabId: 'other' } });
        useWorkspaceStore.setState({ workspace });
      }
      if (kind === 'readonly') useEditorStore.setState(state => ({ documents: { doc: { ...state.documents.doc, readOnly: true } } }));
      if (kind === 'mode') useEditorStore.setState(state => ({ documents: { doc: { ...state.documents.doc, mode: 'markdown' } } }));
      if (kind === 'composition') surface.root.dispatchEvent(new Event('compositionstart', { bubbles: true }));
      if (kind === 'unmount') surface.unmount();
    });
    expect(target?.execute('editor.slide.insert.basic')).toBe(false);
    expect(surface.commitRich).not.toHaveBeenCalled();
    target?.dispose();
  });
  it('recusa documento errado, preparação com dados arbitrários e falha de flush', async () => {
    const surface = mountSurface();
    expect(captureEditorFormatting({ ...surface.doc })).toBeUndefined();
    const target = captureEditorFormatting(surface.doc);
    expect(await target?.prepare('editor.slide.insert.basic', { content: 'injected' })).toBe(false);
    surface.flushRich.mockReturnValue(false);
    expect(await target?.prepare('editor.slide.insert.basic')).toBe(false);
    expect(target?.execute('editor.slide.insert.basic')).toBe(false);
    expect(surface.commitRich).not.toHaveBeenCalled();
    target?.dispose();
  });
  it('troca de sessão durante flush não recebe exceção à proteção de contexto', async () => {
    const surface = mountSurface();
    const target = captureEditorFormatting(surface.doc);
    surface.flushRich.mockImplementation(() => {
      useAuthStore.setState({ isAuthenticated: false });
      return true;
    });
    expect(await target?.prepare('editor.slide.insert.basic')).toBe(false);
    expect(target?.execute('editor.slide.insert.basic')).toBe(false);
    expect(surface.commitRich).not.toHaveBeenCalled();
    target?.dispose();
  });
});
