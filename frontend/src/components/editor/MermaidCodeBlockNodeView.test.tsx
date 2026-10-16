import { useCallback, useEffect, useRef, useState } from 'react';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Editor } from '@tiptap/core';
import i18n from 'i18next';
import { RichTextEditor, type RichTextEditorHandle } from './RichTextEditor';
import { buildRichTextExtensions } from './buildRichTextExtensions';
import { useMermaidSession } from '../../pages/useMermaidSession';
import { useAuthStore } from '../../store/authStore';
import { useEditorStore } from '../../store/editorStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useConfirmStore } from '../../store/confirmStore';
import { Modal, useModalId } from '../ui/Modal';
import { ConfirmHost } from '../ui/ConfirmHost';
import { registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';
import { EDITOR_MERMAID_COMMAND_EVENT, captureEditorMermaidTarget, requestEditorMermaidCommand, type EditorMermaidCommandRequest } from '../../lib/commandEditorMermaid';

vi.mock('../ui/MarkdownRenderer', () => ({ MarkdownRenderer: ({ content }: { content: string }) => <div data-testid="markdown-preview">{content}</div> }));
vi.mock('../../services/audioFeedback', () => ({ playSound: vi.fn(), SOUND_TYPES: { ALERT: 'alert' } }));
const cleanups: Array<() => void> = [];
beforeEach(() => { vi.spyOn(document, 'hasFocus').mockReturnValue(true); });
afterEach(() => { cleanups.splice(0).reverse().forEach(off => off()); vi.restoreAllMocks(); });

function Scope({ scope, onId }: { scope: string; onId: (id: string | null) => void }) {
  const id = useModalId();
  useEffect(() => { onId(id); return () => onId(null); }, [id, onId]);
  return <div data-mermaid-command-scope={scope}><button>Diagram editor</button></div>;
}
async function fixture(readOnly = false) {
  const fence = String.fromCharCode(96).repeat(3);
  const markdown = [fence + 'mermaid', 'graph TD', 'A-->B', fence, '', fence + 'mermaid', 'graph TD', 'C-->D', fence].join('\n');
  const doc = { id: 'doc', title: 'Diagram', markdown, mode: 'rich' as const, readOnly };
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'user' } });
  useEditorStore.setState({ ownerUserId: 'owner', documents: { doc } });
  useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'Workspace', activeTabId: 'doc', tabs: [{ id: 'doc', type: 'editor', title: 'Diagram', position: 0 }] } });
  let api!: ReturnType<typeof useMermaidSession>;
  let editor!: Editor;
  let handle!: RichTextEditorHandle;
  let rerender!: () => void;
  let enableEditing!: () => void;
  const requests: EditorMermaidCommandRequest[] = [];
  const results: boolean[] = [];
  const listener = (event: Event) => {
    const detail = (event as CustomEvent<EditorMermaidCommandRequest>).detail;
    requests.push(detail);
    const target = captureEditorMermaidTarget(detail.commandID, detail.expectedDocument, detail.input);
    if (!target || !target.canExecute(detail.commandID)) { target?.dispose(); return; }
    event.preventDefault();
    if (detail.commandID === 'editor.mermaid.open') { results.push(target.execute(detail.commandID)); target.dispose(); return; }
    void (async () => {
      try { results.push(await target.prepare(detail.commandID) && target.execute(detail.commandID)); }
      finally { target.dispose(); }
    })();
  };
  window.addEventListener(EDITOR_MERMAID_COMMAND_EVENT, listener);
  cleanups.push(() => window.removeEventListener(EDITOR_MERMAID_COMMAND_EVENT, listener));
  function Harness() {
    const rootRef = useRef<HTMLDivElement>(null);
    const editorRef = useRef<Editor | null>(null);
    const handleRef = useRef<RichTextEditorHandle | null>(null);
    const [, setRender] = useState(0);
    const [readonly, setReadonly] = useState(readOnly);
    enableEditing = () => setReadonly(false);
    rerender = () => setRender(value => value + 1);
    const ready = useCallback((value: Editor | null) => { editorRef.current = value; if (value) { editor = value; setRender(n => n + 1); } }, []);
    api = useMermaidSession({ activeTab: doc, rootRef, active: true, asking: false,
      editorRef: { current: null }, monacoRef: { current: null }, richEditorRef: editorRef, richEditorHandleRef: handleRef,
      setDocMarkdown: useEditorStore.getState().setDocMarkdown, updateLatestMarkdownForTab: vi.fn(), schedulePersistForTab: vi.fn() });
    useEffect(() => { if (handleRef.current) handle = handleRef.current; });
    return <><div ref={rootRef}><RichTextEditor ref={handleRef} markdown={markdown} onMarkdownChange={() => {}} readOnly={readonly} onEditorReady={ready} onRequestEditMermaid={api.requestEditRichMermaid} /></div>
      <Modal isOpen={api.isMermaidModalOpen} title="Diagram" onClose={api.cancelMermaidModal}>
        <Scope scope={api.mermaidModalSessionKey} onId={api.setMermaidModalId} />
      </Modal><ConfirmHost /></>;
  }
  const mounted = render(<Harness />);
  cleanups.push(mounted.unmount);
  await waitFor(() => expect(screen.getAllByRole('button', { name: i18n.t('editor.mermaid.editDiagram') })).toHaveLength(2));
  if (!readOnly) await waitFor(() => expect(blocks(editor).every(block => !!block.id)).toBe(true));
  return { get editor() { return editor; }, get handle() { return handle; }, get api() { return api; }, rerender: () => rerender(), enableEditing: () => enableEditing(), requests, results, doc, container: mounted.container };
}
function blocks(editor: Editor) {
  const blocks: Array<{ id: string; code: string }> = [];
  editor.state.doc.descendants(node => { if (node.type.name === 'codeBlock' && node.attrs.language === 'mermaid') blocks.push({ id: node.attrs.mermaidBlockId, code: node.textContent }); });
  return blocks;
}
const previews = (f: Awaited<ReturnType<typeof fixture>>) => f.container.querySelectorAll<HTMLElement>('.rich-mermaid-block__preview');

describe('MermaidCodeBlockNodeView — ingressos reais', () => {
  it('chama abertura por ID gerado no schema real e mantém handle após rerender inócuo', async () => {
    const f = await fixture(); const handle = f.handle; const editor = f.editor; const selected = blocks(editor)[1];
    fireEvent.click(screen.getAllByRole('button', { name: i18n.t('editor.mermaid.editDiagram') })[1]);
    expect(f.api.mermaidModalInitialCode).toBe(selected.code);
    expect(f.requests[0].input?.mermaidBlockId).toBe(selected.id);
    expect(f.requests[0].input?.expectedEditor === editor).toBe(true);
    expect(f.requests[0].input).not.toHaveProperty('apply'); expect(f.requests[0].input).not.toHaveProperty('remove');
    expect(f.handle === handle).toBe(true); act(() => f.rerender()); expect(f.editor === editor).toBe(true); expect(f.handle === handle).toBe(true);
    f.api.updateMermaidDraft('graph LR\nX-->Y');
    const target = captureEditorMermaidTarget('editor.mermaid.apply', f.doc)!;
    expect(target).toBeDefined(); act(() => f.rerender()); expect(target.isCurrent()).toBe(true);
    await act(async () => { expect(await target.prepare('editor.mermaid.apply')).toBe(true); });
    act(() => { expect(target.execute('editor.mermaid.apply')).toBe(true); }); target.dispose();
    expect(blocks(editor).map(block => block.code)).toEqual(['graph TD\nA-->B', 'graph LR\nX-->Y']);
    expect(editor.getHTML()).not.toContain(selected.id);
    expect(handle.getMarkdown()).not.toContain(selected.id);
    expect(handle).not.toHaveProperty('applyMermaidById'); expect(handle).not.toHaveProperty('removeMermaidById');
  });
  it('internacionaliza título e aria-label do bloco', async () => {
    await fixture();
    expect(screen.getAllByRole('group', { name: i18n.t('editor.mermaid.blockLabel', 'Bloco Mermaid') })).toHaveLength(2);
    expect(screen.getAllByText(i18n.t('editor.mermaid.title', 'Mermaid'))).toHaveLength(2);
  });
  it.each(['button', 'Delete', 'Backspace', 'ShiftDelete'])('%s passa pela mesma confirmação e remove somente ID capturado', async gesture => {
    const f = await fixture(); const id = blocks(f.editor)[1].id;
    if (gesture === 'button') fireEvent.click(screen.getAllByRole('button', { name: i18n.t('editor.mermaid.removeBtnLabel') })[1]);
    else fireEvent.keyDown(previews(f)[1], { key: gesture === 'ShiftDelete' ? 'Delete' : gesture, shiftKey: gesture === 'ShiftDelete' });
    await waitFor(() => expect(screen.getByRole('alertdialog')).toBeInTheDocument());
    expect(blocks(f.editor)).toHaveLength(2); expect(f.requests).toHaveLength(1);
    expect(f.requests[0].input?.mermaidBlockId).toBe(id);
    expect(f.requests[0].input?.expectedEditor === f.editor).toBe(true);
    act(() => useConfirmStore.getState().confirm());
    await waitFor(() => expect(f.results).toEqual([true]));
    expect(blocks(f.editor).map(block => block.code)).toEqual(['graph TD\nA-->B']);
  });
  it.each(['Enter', 'F2', 'doubleClick', 'x'])('abertura %s preserva origem e não edita diretamente', async gesture => {
    const f = await fixture();
    if (gesture === 'doubleClick') fireEvent.doubleClick(previews(f)[1]);
    else fireEvent.keyDown(previews(f)[1], { key: gesture });
    expect(f.api.isMermaidModalOpen).toBe(true); expect(f.requests).toHaveLength(1);
    expect(f.api.mermaidModalInitialCode).toBe('graph TD\nC-->D');
    expect(f.api.mermaidModalInitialInsertText).toBe(gesture === 'x' ? 'x' : '');
    expect(blocks(f.editor).map(block => block.code)).toEqual(['graph TD\nA-->B', 'graph TD\nC-->D']);
  });
  it.each(['repeat', 'ime', '229', 'consumed', 'modal', 'readonly'])('nega %s sem despachar nem remover', async reason => {
    const f = await fixture(reason === 'readonly');
    let cleanupOverlay = () => {};
    if (reason === 'modal') {
      const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; overlay.dataset.modalId = 'other'; document.body.append(overlay);
      registerOpenModal('other'); cleanupOverlay = () => { unregisterOpenModal('other'); overlay.remove(); };
    }
    try {
      const event = new KeyboardEvent('keydown', { key: 'Delete', bubbles: true, cancelable: true, repeat: reason === 'repeat', isComposing: reason === 'ime', keyCode: reason === '229' ? 229 : 46 });
      if (reason === 'consumed') event.preventDefault();
      fireEvent(previews(f)[0], event);
      expect(f.requests).toHaveLength(0); expect(blocks(f.editor)).toHaveLength(2);
      expect(useConfirmStore.getState().active).toBeNull();
    } finally { cleanupOverlay(); }
  });
  it('editor diferente com mesmo ID não pode abrir ou remover no ativo', async () => {
    const f = await fixture(); const id = blocks(f.editor)[0].id;
    const other = new Editor({ extensions: buildRichTextExtensions({ placeholder: '', imageFallbackLabel: '', imageLabelPrefix: '' }),
      content: { type: 'doc', content: [{ type: 'codeBlock', attrs: { language: 'mermaid', mermaidBlockId: id }, content: [{ type: 'text', text: 'other' }] }] } });
    cleanups.push(() => other.destroy());
    expect(requestEditorMermaidCommand('editor.mermaid.remove', { mermaidBlockId: id, expectedEditor: other })).toBe(false);
    act(() => { f.api.requestEditRichMermaid({ mermaidBlockId: id, expectedEditor: other }); });
    expect(f.api.isMermaidModalOpen).toBe(false); expect(useConfirmStore.getState().active).toBeNull(); expect(blocks(f.editor)).toHaveLength(2);
  });
  it('readonly sem ID passa a gerar ID ao habilitar mesma instância', async () => {
    const f = await fixture(true); const editor = f.editor;
    expect(blocks(editor).every(block => !block.id)).toBe(true);
    act(() => f.enableEditing());
    await waitFor(() => expect(blocks(editor).every(block => !!block.id)).toBe(true));
    expect(f.editor === editor).toBe(true);
    expect(screen.getAllByRole('button', { name: i18n.t('editor.mermaid.editDiagram') })[0]).not.toBeDisabled();
  });
});
