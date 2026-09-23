import { useEffect, useLayoutEffect, useRef } from 'react';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, expect, it, vi } from 'vitest';
import { Topbar } from './Topbar';
import { Modal, useModalId } from '../ui/Modal';
import { ConfirmHost } from '../ui/ConfirmHost';
import { CommandContextProvider, useCommandContextScope } from '../../lib/commandContextReact';
import { captureEditorMermaidTarget } from '../../lib/commandEditorMermaid';
import * as mermaidRegistry from '../../lib/commandEditorMermaid';
import { useMermaidSession } from '../../pages/useMermaidSession';
import { useEditorStore } from '../../store/editorStore';
import { useAuthStore } from '../../store/authStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useConfirmStore } from '../../store/confirmStore';
import { registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';
import { Editor } from '@tiptap/core';
import { buildRichTextExtensions } from '../editor/buildRichTextExtensions';

const state = vi.hoisted(() => ({ events: new Map<string, (payload?: unknown) => void>(), map: vi.fn(), flush: vi.fn(), announce: vi.fn(), navigate: vi.fn(), pathname: '/', version: 'v1' }));
vi.mock('react-router-dom', () => ({ useNavigate: () => state.navigate, useLocation: () => ({ pathname: state.pathname, search: '', hash: '', key: '/' }) }));
vi.mock('../../lib/commandGlobalOwnershipWails', () => ({ acquireGlobalCommandOwnership: () => ({ isReady: () => true, owns: () => false, dispose: () => {}, ready: Promise.resolve() }) }));
vi.mock('../../services/commandCatalog', () => ({ listCommandCatalog: vi.fn(async () => []) }));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({ createCommandLocalKeyboardWailsPort: () => ({ loadMap: state.map, beginLocalCommandUIKey: vi.fn(), resetLocalCommandKeyboard: vi.fn(), dispatchLocalCommandKey: vi.fn() }) }));
vi.mock('../../store/workspaceStore', async original => ({ ...await original<typeof import('../../store/workspaceStore')>(), flushWorkspaceNavigation: () => state.flush() }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: state.announce, announceRequest: vi.fn() }) }));
vi.mock('../../services/audioFeedback', () => ({ playSound: vi.fn(), SOUND_TYPES: { ALERT: 'alert' } }));
vi.mock('../../store/workspaceChatModalStore', () => ({ canPrepareWorkspaceChatOpen: () => false, registerWorkspaceChatCommandDispatcher: () => () => {}, prepareWorkspaceChatOpen: vi.fn() }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, callback: (payload?: unknown) => void) => { state.events.set(name, callback); return () => state.events.delete(name); } }));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn(async () => null) }));
vi.mock('../pickers/ProfilePicker', () => ({ ProfilePicker: () => null }));

let commandId = 'editor.mermaid.apply';
const reservation = () => ({ commandId, ticket: 'ticket', invocationId: 'invocation' });
const handoff = () => ({ ...reservation(), handoffId: 'handoff' });
const api = { BeginContextualDeckUICommand: vi.fn(), BeginUICommand: vi.fn(), BeginContextualPaletteUICommand: vi.fn(), TakeUICommand: vi.fn(), CompleteUICommand: vi.fn(), CancelUICommand: vi.fn(), GetUICommandResult: vi.fn(), CommitWorkspaceTabCommand: vi.fn() };
const doc = { id: 'doc', title: 'Diagram', mode: 'view' as 'view' | 'rich', markdown: '```mermaid\ngraph TD\nA-->B\n```', sessionHydrated: true };
let richEditor: Editor | null = null;
let mermaid!: ReturnType<typeof useMermaidSession>;
let unregister: (() => void) | undefined;
function Scope() {
  const id = useModalId();
  useEffect(() => { mermaid.setMermaidModalId(id); return () => mermaid.setMermaidModalId(null); }, [id]);
  return <div data-mermaid-command-scope={mermaid.mermaidModalSessionKey}><textarea aria-label="Mermaid draft" /></div>;
}
function Source({ provider = true }: { provider?: boolean }) {
  const scope = useCommandContextScope(); const root = useRef<HTMLDivElement>(null);
  const rich = useRef<Editor | null>(null);
  const handle = useRef({ getMarkdown: () => '', flushMarkdown: vi.fn(), openLinkDialog: async () => {} });
  useLayoutEffect(() => {
    if (doc.mode !== 'rich' || !root.current) return;
    const editor = new Editor({ element: root.current, extensions: buildRichTextExtensions({ placeholder: '', imageFallbackLabel: '', imageLabelPrefix: '' }),
      content: { type: 'doc', content: [{ type: 'codeBlock', attrs: { language: 'mermaid', mermaidBlockId: 'block' }, content: [{ type: 'text', text: 'graph TD\nA-->B' }] }] } });
    rich.current = editor; richEditor = editor; editor.commands.setTextSelection(1);
    return () => { editor.destroy(); rich.current = null; richEditor = null; };
  }, []);
  useLayoutEffect(() => {
    if (!provider) return;
    unregister = scope?.registerSurface('doc', root, () => ({ surfaceId: 'doc', surfaceType: 'editor', snapshotVersion: state.version }));
    return unregister;
  }, [scope, provider]);
  mermaid = useMermaidSession({ activeTab: doc, rootRef: root, active: true, asking: false,
    editorRef: { current: null }, monacoRef: { current: null }, richEditorRef: rich, richEditorHandleRef: handle,
    setDocMarkdown: useEditorStore.getState().setDocMarkdown, updateLatestMarkdownForTab: vi.fn(), schedulePersistForTab: vi.fn() });
  return <><div ref={root} className="ws-content__panel"><button data-testid="block" data-mermaid-index="0">Diagram block</button></div>
    <Modal isOpen={mermaid.isMermaidModalOpen} title="Mermaid" onClose={mermaid.cancelMermaidModal}><Scope /></Modal><ConfirmHost /></>;
}
const envelope = { offerId: 'offer', generation: 'map', userId: 'u', sessionId: 's', workspaceId: 'ws' };
function condition(id = commandId, enabled = true) { return { commandId: id, bySurface: { editor: enabled }, fallback: false }; }
function emit(conditions: unknown = [condition()]) { state.events.get('command:deck-contextual-ui')?.({ ...envelope, conditions }); }
async function mount(provider = true, modal = true) {
  const root = document.createElement('div'); root.id = 'root'; document.body.append(root);
  render(<CommandContextProvider><Topbar /><Source provider={provider} /></CommandContextProvider>, { container: root });
  await act(async () => { await Promise.resolve(); });
  screen.getByTestId('block').focus();
  if (modal) {
    const target = captureEditorMermaidTarget('editor.mermaid.open', doc, { index: 0 });
    expect(target).toBeDefined(); act(() => { expect(target!.execute('editor.mermaid.open')).toBe(true); }); target!.dispose();
    await waitFor(() => expect(screen.getByLabelText('Mermaid draft')).toBeInTheDocument());
    screen.getByLabelText('Mermaid draft').focus();
    fireEvent.compositionStart(screen.getByLabelText('Mermaid draft'));
    fireEvent.compositionEnd(screen.getByLabelText('Mermaid draft'));
    if (commandId === 'editor.mermaid.apply') mermaid.updateMermaidDraft('graph LR\nX-->Y');
    expect(root).toHaveAttribute('inert');
  }
  return root;
}
beforeEach(() => {
  state.events.clear(); state.flush.mockReset(); state.flush.mockResolvedValue(true); state.pathname = '/'; state.version = 'v1'; state.navigate.mockClear();
  commandId = 'editor.mermaid.apply';
  doc.mode = 'view';
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'u', sessionId: 's', role: 'user' } });
  useEditorStore.setState({ ownerUserId: 'u', documents: { doc } });
  useWorkspaceStore.setState({ workspace: { id: 'ws', name: 'Workspace', profile: 'focused', activeTabId: 'doc', tabs: [{ id: 'doc', type: 'editor', title: 'Diagram', position: 0 }] } });
  Object.values(api).forEach(spy => spy.mockReset());
  api.BeginContextualDeckUICommand.mockImplementation(async () => reservation()); api.TakeUICommand.mockImplementation(async () => handoff());
  api.GetUICommandResult.mockResolvedValue({ invocationId: 'invocation', status: 'succeeded' });
  state.map.mockResolvedValue({ generation: 'map', ownerId: 'u', sessionId: 's', workspaceId: 'ws', bindings: [], validUntil: Date.now() + 300000 });
  Object.assign(window, { go: { app: { App: api } } }); vi.spyOn(document, 'hasFocus').mockReturnValue(true);
});
afterEach(() => { cleanup(); unregisterOpenModal('foreign'); document.getElementById('root')?.remove(); Reflect.deleteProperty(window, 'go'); vi.restoreAllMocks(); });

it.each(['editor.mermaid.apply', 'editor.mermaid.remove'])('%s consumes before real hook preparation, closes owned modal and audits once', async id => {
  commandId = id; await mount();
  api.BeginContextualDeckUICommand.mockImplementation(async () => { expect(mermaid.isMermaidModalOpen).toBe(true); return reservation(); });
  await act(async () => { emit(); emit(); });
  if (id.endsWith('remove')) {
    await screen.findByRole('alertdialog'); expect(api.BeginContextualDeckUICommand).toHaveBeenCalledOnce(); expect(api.TakeUICommand).not.toHaveBeenCalled();
    act(() => useConfirmStore.getState().respond(true));
  }
  await waitFor(() => expect(api.CompleteUICommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff', 'succeeded'));
  expect(mermaid.isMermaidModalOpen).toBe(false); expect(document.activeElement).toBe(screen.getByTestId('block'));
  expect(useEditorStore.getState().documents.doc.markdown).toBe(id.endsWith('remove') ? '' : '```mermaid\ngraph LR\nX-->Y\n```');
  expect(api.BeginContextualDeckUICommand).toHaveBeenCalledExactlyOnceWith('offer', 'map', { surfaceType: 'editor', surfaceId: 'doc', profile: 'focused' });
  expect(api.BeginUICommand).not.toHaveBeenCalled(); expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled(); expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled();
});

it('restores actual TipTap focus after owned modal preparation without treating it as the original textarea', async () => {
  doc.mode = 'rich'; await mount();
  await act(async () => emit());
  await waitFor(() => expect(api.CompleteUICommand).toHaveBeenCalledWith('ticket', 'handoff', 'succeeded'));
  expect(document.activeElement).toBe(richEditor!.view.dom);
  expect(richEditor!.state.doc.firstChild!.textContent).toBe('graph LR\nX-->Y');
});

it.each(['blur', 'ime', 'other-control'])('TipTap prepared-focus continuation rejects %s during Take', async mode => {
  doc.mode = 'rich'; await mount(); let resolve!: (value: ReturnType<typeof handoff>) => void;
  api.TakeUICommand.mockImplementation(() => new Promise(done => { resolve = done; }));
  await act(async () => emit()); await waitFor(() => expect(api.TakeUICommand).toHaveBeenCalledOnce());
  expect(document.activeElement).toBe(richEditor!.view.dom);
  act(() => {
    if (mode === 'blur') { fireEvent(window, new FocusEvent('blur')); fireEvent(window, new FocusEvent('focus')); }
    if (mode === 'ime') { fireEvent.compositionStart(richEditor!.view.dom); fireEvent.compositionEnd(richEditor!.view.dom); }
    if (mode === 'other-control') screen.getByTestId('block').focus();
  });
  await act(async () => resolve(handoff()));
  await waitFor(() => expect(api.CompleteUICommand).toHaveBeenCalledWith('ticket', 'handoff', 'cancelled'));
  expect(richEditor!.state.doc.firstChild!.textContent).toBe('graph TD\nA-->B');
});

it.each(['cancel', 'direct', 'after-offer-ttl'])('remove confirmation: %s preserves target and reservation budget', async mode => {
  commandId = 'editor.mermaid.remove'; await mount(true, mode !== 'direct');
  const now = Date.now(); await act(async () => emit()); await screen.findByRole('alertdialog');
  expect(api.BeginContextualDeckUICommand).toHaveBeenCalledOnce();
  if (mode === 'after-offer-ttl') vi.spyOn(Date, 'now').mockReturnValue(now + 15000);
  act(() => useConfirmStore.getState().respond(mode !== 'cancel'));
  await waitFor(() => expect(mode === 'cancel' ? api.CancelUICommand : api.CompleteUICommand).toHaveBeenCalled());
  expect(api.TakeUICommand).toHaveBeenCalledTimes(mode === 'cancel' ? 0 : 1);
  expect(useEditorStore.getState().documents.doc.markdown).toBe(mode === 'cancel' ? doc.markdown : '');
});

it('throwing preparation cancels the already-consumed reservation and disposes without retry', async () => {
  await mount();
  const capture = mermaidRegistry.captureContextualEditorMermaidTarget;
  const dispose = vi.fn();
  vi.spyOn(mermaidRegistry, 'captureContextualEditorMermaidTarget').mockImplementation(id => {
    const captured = capture(id); if (!captured) return;
    const target = captured.target;
    return { documentId: captured.documentId, target: { ...target, prepare: async () => { throw new Error('preparation-failed'); }, dispose: () => { dispose(); target.dispose(); } } };
  });
  await act(async () => emit());
  await waitFor(() => expect(api.CancelUICommand).toHaveBeenCalledExactlyOnceWith('ticket'));
  await act(async () => emit());
  expect(api.BeginContextualDeckUICommand).toHaveBeenCalledOnce(); expect(dispose).toHaveBeenCalled();
  expect(api.TakeUICommand).not.toHaveBeenCalled(); expect(useEditorStore.getState().documents.doc.markdown).toBe(doc.markdown);
});

it.each(['missing-provider', 'foreign-modal', 'unknown-ime', 'flush-false', 'wrong-command', 'revision', 'expired-offer', 'local-under-modal', 'layer-under-modal', 'durable-under-modal', 'ambiguous'])('rejects %s without fallback or effect', async mode => {
  await mount(mode !== 'missing-provider');
  if (mode === 'foreign-modal') registerOpenModal('foreign');
  if (mode === 'unknown-ime') { screen.getByTestId('block').focus(); screen.getByLabelText('Mermaid draft').focus(); }
  if (mode === 'flush-false') state.flush.mockResolvedValue(false);
  if (mode === 'revision') state.flush.mockImplementation(async () => { mermaid.updateMermaidDraft('other'); mermaid.updateMermaidDraft('graph LR\nX-->Y'); return true; });
  if (mode === 'expired-offer') state.flush.mockImplementation(async () => { const afterLease = Date.now(); vi.spyOn(Date, 'now').mockReturnValue(afterLease + 10001); return true; });
  if (mode === 'wrong-command') api.BeginContextualDeckUICommand.mockResolvedValue({ ...reservation(), commandId: 'workspace.tab.close' });
  const other = mode === 'local-under-modal' ? 'navigation.settings.open' : mode === 'layer-under-modal' ? 'layer.toggle' : 'workspace.tab.close';
  await act(async () => emit(mode.endsWith('under-modal') ? [condition(commandId, false), condition(other)] : mode === 'ambiguous' ? [condition(), condition('editor.mermaid.remove')] : undefined));
  if (mode === 'wrong-command') await waitFor(() => expect(api.CancelUICommand).toHaveBeenCalledWith('ticket'));
  expect(api.BeginContextualDeckUICommand).toHaveBeenCalledTimes(mode === 'wrong-command' ? 1 : 0);
  expect(api.TakeUICommand).not.toHaveBeenCalled(); expect(api.CompleteUICommand).not.toHaveBeenCalled(); expect(state.navigate).not.toHaveBeenCalled();
  expect(useEditorStore.getState().documents.doc.markdown).toBe(doc.markdown);
});

it.each(['blur-aba', 'ime-aba', 'provider', 'profile-aba', 'map', 'deadline'])('rejects %s during deferred Take without retarget/replay', async mode => {
  await mount(); let resolve!: (value: ReturnType<typeof handoff>) => void;
  api.TakeUICommand.mockImplementation(() => new Promise(done => { resolve = done; }));
  await act(async () => emit()); await waitFor(() => expect(api.TakeUICommand).toHaveBeenCalledOnce());
  const block = screen.getByTestId('block');
  act(() => {
    if (mode === 'blur-aba') { fireEvent(window, new FocusEvent('blur')); fireEvent(window, new FocusEvent('focus')); }
    if (mode === 'ime-aba') { fireEvent.compositionStart(block); fireEvent.compositionEnd(block); }
    if (mode === 'provider') unregister?.();
    if (mode === 'profile-aba') { const ws = useWorkspaceStore.getState().workspace!; useWorkspaceStore.setState({ workspace: { ...ws, profile: 'other' } }); useWorkspaceStore.setState({ workspace: ws }); }
    if (mode === 'map') state.events.get('command:keyboard-map-changed')?.();
    if (mode === 'deadline') vi.spyOn(Date, 'now').mockReturnValue(Date.now() + 300001);
  });
  await act(async () => resolve(handoff()));
  await waitFor(() => expect(api.CompleteUICommand).toHaveBeenCalledWith('ticket', 'handoff', 'cancelled'));
  expect(api.CompleteUICommand).not.toHaveBeenCalledWith('ticket', 'handoff', 'succeeded');
  expect(useEditorStore.getState().documents.doc.markdown).toBe(doc.markdown);
});
