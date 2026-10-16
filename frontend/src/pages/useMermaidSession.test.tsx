import { useEffect } from 'react';
import { act, render, waitFor, screen } from '@testing-library/react';
import { Editor } from '@tiptap/core';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { Modal, useModalId } from '../components/ui/Modal';
import { ConfirmHost } from '../components/ui/ConfirmHost';
import { buildRichTextExtensions } from '../components/editor/buildRichTextExtensions';
import { captureEditorMermaidTarget } from '../lib/commandEditorMermaid';
import { registerOpenModal, unregisterOpenModal } from '../lib/modalRegistry';
import { useAuthStore } from '../store/authStore';
import { useEditorStore } from '../store/editorStore';
import { useWorkspaceStore } from '../store/workspaceStore';
import { useConfirmStore } from '../store/confirmStore';
import { useMermaidSession } from './useMermaidSession';
import type { MonacoCodeEditor, MonacoNamespace } from './editorTypes';
vi.mock('../services/audioFeedback', () => ({ playSound: vi.fn(), SOUND_TYPES: { ALERT: 'alert' } }));

const cleanups: Array<() => void> = [];
beforeEach(() => { vi.spyOn(document, 'hasFocus').mockReturnValue(true); });
afterEach(() => { cleanups.splice(0).reverse().forEach(off => off()); vi.restoreAllMocks(); });
function Scope({ scope, onId }: { scope: string; onId: (id: string | null) => void }) {
  const id = useModalId();
  useEffect(() => { onId(id); return () => onId(null); }, [id, onId]);
  return <div data-mermaid-command-scope={scope}><button>Diagram</button></div>;
}
function fixture(mode: 'rich' | 'view' | 'markdown' = 'rich') {
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'u', sessionId: 's', role: 'user' } });
  const doc = { id: 'doc', title: 'Diagram', markdown: '```mermaid\ngraph TD\nA-->B\n```\n\n```mermaid\ngraph TD\nC-->D\n```', mode };
  useEditorStore.setState({ ownerUserId: 'u', documents: { doc } });
  useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'Workspace', activeTabId: 'doc', tabs: [{ id: 'doc', type: 'editor', title: 'Diagram', position: 0 }] } });
  const root = document.createElement('div'); document.body.append(root);
  const listeners = new Map<string, Set<() => void>>();
  const listen = (event: string) => (callback: () => void) => { const set = listeners.get(event) ?? new Set(); set.add(callback); listeners.set(event, set); return { dispose: () => { set.delete(callback); } }; };
  const emit = (event: string) => listeners.get(event)?.forEach(callback => callback());
  const monacoState = { text: doc.markdown, version: 1, readonly: false, result: 'success' as 'success' | 'false' | 'unchanged' };
  const model = { getValue: () => monacoState.text, getVersionId: () => monacoState.version, isDisposed: () => false,
    getOffsetAt: () => doc.markdown.indexOf('C-->D'), getFullModelRange: () => ({ startLineNumber: 1, startColumn: 1, endLineNumber: 9, endColumn: 4 }), onDidChangeContent: listen('content') };
  let liveModel = model;
  const monacoElement = document.createElement('textarea'); root.append(monacoElement);
  const monacoEditor = { getModel: () => liveModel, getPosition: () => ({ lineNumber: 8, column: 1 }), getDomNode: () => monacoElement,
    getOption: () => monacoState.readonly, onDidChangeModel: listen('model'), onDidDispose: listen('dispose'), onDidCompositionStart: listen('composition'), onDidChangeConfiguration: listen('configuration'),
    pushUndoStop: vi.fn(), focus: () => monacoElement.focus(), executeEdits: vi.fn((_source: string, edits: Array<{ text: string }>) => {
      if (monacoState.result === 'false') return false;
      if (monacoState.result === 'success') { monacoState.text = edits[0].text; monacoState.version++; emit('content'); }
      return true;
    }) };
  const editorRef = { current: mode === 'markdown' ? monacoEditor as unknown as MonacoCodeEditor : null };
  const monacoRef = { current: mode === 'markdown' ? { editor: { EditorOption: { readOnly: 1 } } } as unknown as MonacoNamespace : null };
  const rich = new Editor({ element: root, extensions: buildRichTextExtensions({ placeholder: '', imageFallbackLabel: '', imageLabelPrefix: '' }),
    content: { type: 'doc', content: [
      { type: 'codeBlock', attrs: { language: 'mermaid', mermaidBlockId: 'a' }, content: [{ type: 'text', text: 'graph TD\nA-->B' }] },
      { type: 'codeBlock', attrs: { language: 'mermaid', mermaidBlockId: 'b' }, content: [{ type: 'text', text: 'graph TD\nC-->D' }] },
    ] } });
  rich.commands.setTextSelection(rich.state.doc.firstChild!.nodeSize + 1);
  const richEditorRef = { current: rich };
  const flush = vi.fn();
  const richEditorHandleRef = { current: { getMarkdown: () => '', flushMarkdown: flush, openLinkDialog: async () => {} } };
  let api!: ReturnType<typeof useMermaidSession>;
  function Harness() {
    api = useMermaidSession({ activeTab: doc, rootRef: { current: root }, active: true, asking: false, editorRef, monacoRef, richEditorRef, richEditorHandleRef,
      setDocMarkdown: useEditorStore.getState().setDocMarkdown, updateLatestMarkdownForTab: vi.fn(), schedulePersistForTab: vi.fn() });
    return <><Modal isOpen={api.isMermaidModalOpen} title="Mermaid" onClose={api.cancelMermaidModal}><Scope scope={api.mermaidModalSessionKey} onId={api.setMermaidModalId} /></Modal><ConfirmHost /></>;
  }
  const mounted = render(<Harness />);
  cleanups.push(() => { mounted.unmount(); rich.destroy(); root.remove(); });
  return { get api() { return api; }, rich, root, richEditorRef, richEditorHandleRef, flush, doc, unmount: mounted.unmount, monacoState, monacoEditor, editorRef,
    changeModel: () => { liveModel = { ...model }; emit('model'); }, changeContent: () => { monacoState.version++; emit('content'); } };
}
function open(f: ReturnType<typeof fixture>, index?: number) {
  const target = captureEditorMermaidTarget('editor.mermaid.open', f.doc, index === undefined ? undefined : { index });
  expect(target).toBeDefined();
  act(() => { expect(target!.execute('editor.mermaid.open')).toBe(true); }); target!.dispose();
  expect(f.api.isMermaidModalOpen).toBe(true);
}
function diagrams(editor: Editor) {
  const codes: string[] = [];
  editor.state.doc.descendants(node => { if (node.type.name === 'codeBlock' && node.attrs.language === 'mermaid') codes.push(node.textContent); });
  return codes;
}
it('open síncrono captura segundo bloco selecionado; apply ACK real uma única vez', async () => {
  const f = fixture(); open(f); expect(f.api.mermaidModalInitialCode).toBe('graph TD\nC-->D');
  f.api.updateMermaidDraft('graph TD\nX-->Y');
  const target = captureEditorMermaidTarget('editor.mermaid.apply', f.doc)!;
  await act(async () => { expect(await target.prepare('editor.mermaid.apply')).toBe(true); });
  expect(f.api.isMermaidModalOpen).toBe(false); expect(document.activeElement).toBe(f.rich.view.dom);
  act(() => { expect(target.execute('editor.mermaid.apply')).toBe(true); });
  expect(f.rich.state.doc.firstChild!.textContent).toBe('graph TD\nA-->B');
  expect(diagrams(f.rich)).toEqual(['graph TD\nA-->B', 'graph TD\nX-->Y']);
  expect(target.execute('editor.mermaid.apply')).toBe(false); expect(f.flush).toHaveBeenCalledOnce(); target.dispose();
});
it('draft ABA após capture recusa e não fecha o formulário', async () => {
  const f = fixture(); open(f); f.api.updateMermaidDraft('new');
  const target = captureEditorMermaidTarget('editor.mermaid.apply')!;
  f.api.updateMermaidDraft('other'); f.api.updateMermaidDraft('new');
  expect(await target.prepare('editor.mermaid.apply')).toBe(false);
  expect(f.api.isMermaidModalOpen).toBe(true); target.dispose();
});
it.each(['document', 'instance', 'handle', 'workspace'] as const)('%s alterado após abertura não retargeta', kind => {
  const f = fixture(); open(f);
  if (kind === 'document') act(() => { f.rich.commands.insertContent('changed'); });
  if (kind === 'instance') f.richEditorRef.current = new Editor({ extensions: buildRichTextExtensions({ placeholder: '', imageFallbackLabel: '', imageLabelPrefix: '' }) });
  if (kind === 'instance') cleanups.push(() => f.richEditorRef.current.destroy());
  if (kind === 'handle') f.richEditorHandleRef.current = { ...f.richEditorHandleRef.current };
  if (kind === 'workspace') {
    const ws = useWorkspaceStore.getState().workspace!;
    act(() => { useWorkspaceStore.setState({ workspace: { ...ws, activeTabId: 'other' } }); useWorkspaceStore.setState({ workspace: ws }); });
  }
  expect(captureEditorMermaidTarget('editor.mermaid.apply', f.doc, { code: 'new' })).toBeUndefined();
  expect(f.flush).not.toHaveBeenCalled();
});
it('view abre bloco focado e aplica somente fence capturada sem exigir editor ativo', async () => {
  const f = fixture('view');
  const button = document.createElement('button'); button.dataset.mermaidIndex = '1'; f.root.append(button); button.focus();
  open(f); f.api.updateMermaidDraft('graph LR\nE-->F');
  const target = captureEditorMermaidTarget('editor.mermaid.apply')!;
  await act(async () => { expect(await target.prepare('editor.mermaid.apply')).toBe(true); });
  expect(document.activeElement).toBe(button);
  act(() => { expect(target.execute('editor.mermaid.apply')).toBe(true); });
  expect(useEditorStore.getState().documents.doc.markdown).toContain('A-->B');
  expect(useEditorStore.getState().documents.doc.markdown).toContain('E-->F'); target.dispose();
});
it('readonly e seleção fora de bloco não abrem', () => {
  const f = fixture('view'); expect(captureEditorMermaidTarget('editor.mermaid.open')).toBeUndefined();
  useEditorStore.setState({ documents: { doc: { ...f.doc, readOnly: true } } });
  expect(captureEditorMermaidTarget('editor.mermaid.open', undefined, { index: 0 })).toBeUndefined();
});
it.each([false, true])('remove usa confirmação real após fechar Mermaid; confirmado=%s', async confirmed => {
  const f = fixture(); open(f);
  const target = captureEditorMermaidTarget('editor.mermaid.remove')!;
  let preparation!: Promise<boolean>; act(() => { preparation = target.prepare('editor.mermaid.remove'); });
  await waitFor(() => expect(screen.getByRole('alertdialog')).toBeInTheDocument());
  expect(f.api.isMermaidModalOpen).toBe(false);
  act(() => { useConfirmStore.getState().respond(confirmed); });
  await act(async () => { expect(await preparation).toBe(confirmed); });
  if (!confirmed) expect(document.activeElement).toBe(f.rich.view.dom);
  act(() => { expect(target.execute('editor.mermaid.remove')).toBe(confirmed); });
  expect(diagrams(f.rich)).toEqual(confirmed ? ['graph TD\nA-->B'] : ['graph TD\nA-->B', 'graph TD\nC-->D']); target.dispose();
});
it.each(['success', 'false', 'unchanged'] as const)('Monaco ACK exige executeEdits e versão alterada: %s', async result => {
  const f = fixture('markdown'); open(f); f.api.updateMermaidDraft('graph LR\nX-->Y');
  const target = captureEditorMermaidTarget('editor.mermaid.apply')!;
  await act(async () => { expect(await target.prepare('editor.mermaid.apply')).toBe(true); });
  f.monacoState.result = result;
  act(() => { expect(target.execute('editor.mermaid.apply')).toBe(result === 'success'); });
  expect(f.monacoEditor.executeEdits).toHaveBeenCalledTimes(1);
  expect(f.monacoState.text).toContain('A-->B');
  expect(f.monacoState.text.includes('X-->Y')).toBe(result === 'success');
  expect(useEditorStore.getState().documents.doc.markdown).toBe(f.doc.markdown);
  expect(target.execute('editor.mermaid.apply')).toBe(false); target.dispose();
});
it.each(['readonly', 'model', 'instance', 'version'] as const)('Monaco %s antes Begin recusa sem mutação', async kind => {
  const f = fixture('markdown'); open(f); f.api.updateMermaidDraft('different');
  const target = captureEditorMermaidTarget('editor.mermaid.apply')!;
  if (kind === 'readonly') f.monacoState.readonly = true;
  if (kind === 'model') f.changeModel();
  if (kind === 'version') f.changeContent();
  if (kind === 'instance') f.editorRef.current = { ...f.monacoEditor } as unknown as MonacoCodeEditor;
  expect(await target.prepare('editor.mermaid.apply')).toBe(false);
  expect(target.execute('editor.mermaid.apply')).toBe(false); expect(f.monacoEditor.executeEdits).not.toHaveBeenCalled(); target.dispose();
});
it('cancel restaura somente foco da instância capturada', async () => {
  const f = fixture(); open(f); act(() => f.api.cancelMermaidModal());
  await waitFor(() => expect(document.activeElement).toBe(f.rich.view.dom));
  expect(f.flush).not.toHaveBeenCalled();
});
it('cancel em preview restaura wrapper exato e não raiz sem tabIndex', async () => {
  const f = fixture('view'); const wrapper = document.createElement('div'); wrapper.tabIndex = 0; wrapper.dataset.mermaidIndex = '1'; f.root.append(wrapper); wrapper.focus();
  open(f); act(() => f.api.cancelMermaidModal());
  await waitFor(() => expect(document.activeElement).toBe(wrapper));
});
it('dispose cancela decisão própria pendente sem editar', async () => {
  const f = fixture(); open(f); const target = captureEditorMermaidTarget('editor.mermaid.remove')!;
  let preparation!: Promise<boolean>; act(() => { preparation = target.prepare('editor.mermaid.remove'); });
  await waitFor(() => expect(useConfirmStore.getState().active).not.toBeNull());
  act(() => target.dispose());
  await act(async () => { expect(await preparation).toBe(false); });
  expect(useConfirmStore.getState().active).toBeNull(); expect(diagrams(f.rich)).toHaveLength(2);
});
it('cancel remove após trocar contexto não rouba foco do destino novo', async () => {
  const f = fixture(); open(f); const target = captureEditorMermaidTarget('editor.mermaid.remove')!;
  let preparation!: Promise<boolean>; act(() => { preparation = target.prepare('editor.mermaid.remove'); });
  await waitFor(() => expect(screen.getByRole('alertdialog')).toBeInTheDocument());
  const other = document.createElement('button'); document.body.append(other); cleanups.push(() => other.remove());
  act(() => {
    useWorkspaceStore.setState(state => ({ workspace: { ...state.workspace!, activeTabId: 'other' } }));
    useConfirmStore.getState().respond(false); other.focus();
  });
  await act(async () => { expect(await preparation).toBe(false); });
  expect(document.activeElement).toBe(other); expect(diagrams(f.rich)).toHaveLength(2); target.dispose();
});
it('modal ABA após prepare impede aplicação mesmo com stack novamente vazia', async () => {
  const f = fixture(); open(f); f.api.updateMermaidDraft('changed');
  const target = captureEditorMermaidTarget('editor.mermaid.apply')!;
  await act(async () => { expect(await target.prepare('editor.mermaid.apply')).toBe(true); });
  const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; overlay.dataset.modalId = 'other'; document.body.append(overlay);
  registerOpenModal('other'); unregisterOpenModal('other'); overlay.remove();
  expect(target.canExecute('editor.mermaid.apply')).toBe(false); expect(target.execute('editor.mermaid.apply')).toBe(false);
  expect(diagrams(f.rich)).toEqual(['graph TD\nA-->B', 'graph TD\nC-->D']); target.dispose();
});
it('remove fora do modal captura segundo bloco selecionado sem escolher primeiro', async () => {
  const f = fixture(); const target = captureEditorMermaidTarget('editor.mermaid.remove', f.doc)!;
  expect(target).toBeDefined(); let preparation!: Promise<boolean>;
  act(() => { preparation = target.prepare('editor.mermaid.remove'); });
  await waitFor(() => expect(screen.getByRole('alertdialog')).toBeInTheDocument());
  act(() => useConfirmStore.getState().confirm());
  await act(async () => { expect(await preparation).toBe(true); });
  act(() => { expect(target.execute('editor.mermaid.remove')).toBe(true); });
  expect(diagrams(f.rich)).toEqual(['graph TD\nA-->B']); target.dispose();
});
