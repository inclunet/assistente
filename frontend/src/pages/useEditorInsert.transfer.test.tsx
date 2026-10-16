import { act, renderHook } from '@testing-library/react';
import { Editor } from '@tiptap/core';
import { afterEach, expect, it, vi } from 'vitest';
import { buildRichTextExtensions } from '../components/editor/buildRichTextExtensions';
import { captureEditorTransferDestination, transferEditorContent } from '../lib/commandEditorTransfer';
import { useAuthStore } from '../store/authStore';
import { useEditorStore } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useEditorInsert } from './useEditorInsert';

const cleanup: Array<() => void> = [];
afterEach(() => { cleanup.splice(0).reverse().forEach(off => off()); vi.useRealTimers(); });
function fixture(ready = true, active = true) {
  useAuthStore.setState({ user: { userId: 'u', sessionId: 's', role: 'user' }, isAuthenticated: true });
  const doc = { id: 'doc', title: 'Doc', markdown: 'hello world', mode: 'rich' as const, readOnly: false, loadError: false };
  useEditorStore.setState({ ownerUserId: 'u', documents: { doc } });
  useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'WS', activeTabId: 'doc', tabs: [{ id: 'doc', type: 'editor', title: 'Doc', position: 0 }] } });
  const root = document.createElement('div'); document.body.append(root);
  const editor = new Editor({ element: root, extensions: buildRichTextExtensions({ placeholder: '', imageFallbackLabel: 'image', imageLabelPrefix: 'image' }), content: { type: 'doc', content: [{ type: 'paragraph', content: [{ type: 'text', text: 'hello world' }] }] } });
  editor.commands.setTextSelection({ from: 1, to: 6 });
  const richEditorRef: { current: Editor | null } = { current: ready ? editor : null };
  const rootRef = { current: root };
  const flush = vi.fn();
  const hook = renderHook(({ active, nonce }) => useEditorInsert({ commandSurface: { root: rootRef, active, asking: false }, activeTab: doc, currentDocumentId: 'doc', sessionLoaded: true, editorReadyNonce: nonce, editorRef: { current: null }, monacoRef: { current: null }, richEditorRef, flushActiveRichMarkdownNow: flush }), { initialProps: { active, nonce: 0 } });
  cleanup.push(() => { editor.destroy(); root.remove(); }, hook.unmount);
  return { editor, richEditorRef, hook, flush };
}
const request = () => ({ targetDocumentId: 'doc', content: 'changed', format: 'plain' as const, isCurrent: () => true });
it('Tiptap real substitui somente seleção capturada e ACK após alteração', async () => {
  const f = fixture(); await act(async () => { await transferEditorContent(request()); });
  expect(f.editor.getText()).toBe('changed world'); expect(f.flush).toHaveBeenCalledTimes(1);
  expect(document.activeElement).toBe(f.editor.view.dom);
});
it('captura inativo e mantém lease durante ativação autorizada', async () => {
  const f = fixture(true, false); const destination = captureEditorTransferDestination('doc');
  expect(destination).toBeDefined(); f.hook.rerender({ active: true, nonce: 0 });
  await act(async () => { await transferEditorContent({ ...request(), destination }); });
  expect(f.editor.getText()).toBe('changed world');
});
it('sessionLoaded sem instância aguarda montagem real', async () => {
  vi.useFakeTimers(); const f = fixture(false); const done = transferEditorContent(request());
  await act(async () => { await vi.advanceTimersByTimeAsync(100); });
  expect(f.editor.getText()).toBe('hello world');
  await act(async () => { f.richEditorRef.current = f.editor; f.hook.rerender({ active: true, nonce: 1 }); });
  await done; expect(f.editor.getText()).toBe('changed world');
});
it('não captura sample provisório antes da hidratação do documento', async () => {
  vi.useFakeTimers(); const f = fixture();
  act(() => { useEditorStore.setState(state => ({ documents: { doc: { ...state.documents.doc, sessionHydrated: false } } })); });
  const done = transferEditorContent(request());
  await act(async () => { await vi.advanceTimersByTimeAsync(40); });
  expect(f.editor.getText()).toBe('hello world');
  await act(async () => {
    f.editor.commands.setContent({ type: 'doc', content: [{ type: 'paragraph' }] });
    useEditorStore.setState(state => ({ documents: { doc: { ...state.documents.doc, markdown: '', sessionHydrated: true } } }));
    await vi.advanceTimersByTimeAsync(20);
  });
  await done; expect(f.editor.getText()).toBe('changed');
});
it('seleção ABA invalida mesmo retornando ao intervalo original', async () => {
  const f = fixture(); const destination = captureEditorTransferDestination('doc');
  f.editor.commands.setTextSelection(7); f.editor.commands.setTextSelection({ from: 1, to: 6 });
  await expect(transferEditorContent({ ...request(), destination })).rejects.toMatchObject({ code: 'stale' });
  expect(f.editor.getText()).toBe('hello world');
});
it('readonly nunca é tornado editável', async () => {
  const f = fixture(); f.editor.setEditable(false); const setEditable = vi.spyOn(f.editor, 'setEditable');
  await expect(transferEditorContent(request())).rejects.toMatchObject({ code: 'unavailable' });
  expect(setEditable).not.toHaveBeenCalled(); expect(f.editor.getText()).toBe('hello world');
});
it('run sem mudança documental não gera ACK de sucesso', async () => {
  const f = fixture();
  await expect(transferEditorContent({ ...request(), content: 'hello' })).rejects.toMatchObject({ code: 'unavailable' });
  expect(f.editor.getText()).toBe('hello world'); expect(f.flush).not.toHaveBeenCalled();
});
it('desmontagem durante validação aborta alvo capturado', async () => {
  const f = fixture();
  await expect(transferEditorContent({ ...request(), beforeApply: async () => { f.hook.unmount(); } })).rejects.toMatchObject({ code: 'stale' });
  expect(f.editor.getText()).toBe('hello world');
});
it('não rouba foco se contexto muda durante flush após edição confirmada', async () => {
  const f = fixture(); const button = document.createElement('button'); document.body.append(button);
  cleanup.push(() => button.remove());
  f.flush.mockImplementation(() => {
    useWorkspaceStore.setState(state => ({ workspace: { ...state.workspace!, activeTabId: 'other' } }));
    button.focus();
  });
  await act(async () => { await transferEditorContent(request()); });
  expect(f.editor.getText()).toBe('changed world'); expect(document.activeElement).toBe(button);
});
it('documento ABA invalida referência original antes do efeito', async () => {
  const f = fixture(); const destination = captureEditorTransferDestination('doc');
  const original = useEditorStore.getState().documents.doc;
  act(() => {
    useEditorStore.setState({ documents: { doc: { ...original, markdown: 'other' } } });
    useEditorStore.setState({ documents: { doc: original } });
  });
  await expect(transferEditorContent({ ...request(), destination })).rejects.toMatchObject({ code: 'stale' });
  expect(f.editor.getText()).toBe('hello world');
});
