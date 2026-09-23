import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { Topbar } from './Topbar';

// This fixture has no native hotkeys. The shared ownership bridge is exercised
// independently with real reservation frames in commandGlobalOwnershipWails tests.
vi.mock('../../lib/commandGlobalOwnershipWails', () => ({
  acquireGlobalCommandOwnership: () => ({
    isReady: () => true, owns: () => false, dispose: () => {}, ready: Promise.resolve(),
  }),
}));
import { registerEditorMermaidSurface, requestEditorMermaidCommand, type EditorMermaidCommandID } from '../../lib/commandEditorMermaid';
import type { CommandShortcut } from '../../lib/commandShortcut';

const state = vi.hoisted(() => ({
  pathname: '/', mapReady: false, commandID: 'editor.mermaid.apply',
  begin: vi.fn(), beginKey: vi.fn(), take: vi.fn(), complete: vi.fn(), cancel: vi.fn(),
  getResult: vi.fn(), commit: vi.fn(), execute: vi.fn(), prepareAdmission: vi.fn(),
  announce: vi.fn(), navigate: vi.fn(),
  events: new Map<string, (payload: unknown) => void>(),
  menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' },
  noop: () => undefined, t: (key: string) => key,
}));
const commandID = 'editor.mermaid.apply';
const commandIDs = ['editor.mermaid.open', 'editor.mermaid.apply', 'editor.mermaid.remove'] as const;
type Source = 'button' | 'keyboard' | 'palette' | 'deck';
const sources: Source[] = ['button', 'keyboard', 'palette', 'deck'];
const auth = { isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a', role: 'user' } };
const workspace = {
  id: 'workspace-a', name: 'Workspace', profile: '', activeTabId: 'chat-tab',
  tabs: [{ id: 'chat-tab', type: 'chat' as const, conversationId: 'conversation-a' }],
};

vi.mock('react-router-dom', async importOriginal => ({
  ...await importOriginal<typeof import('react-router-dom')>(),
  useNavigate: () => state.navigate,
  useLocation: () => ({ pathname: state.pathname, search: '', hash: '', key: 'chat' }),
}));
vi.mock('../../store/authStore', () => ({
  useAuthStore: Object.assign((selector?: (value: typeof auth) => unknown) => selector ? selector(auth) : auth, {
    getState: () => auth, subscribe: () => () => undefined,
  }),
}));
vi.mock('../../store/workspaceStore', () => ({
  flushWorkspaceNavigation: vi.fn(async () => true),
  useWorkspaceStore: Object.assign((selector?: (value: { workspace: typeof workspace; workspaces: never[]; setActiveTab: () => void }) => unknown) =>
    selector ? selector({ workspace, workspaces: [], setActiveTab: state.noop }) : { workspace, workspaces: [] }, {
    getState: () => ({ workspace, workspaces: [] }), subscribe: () => () => undefined,
  }),
}));
vi.mock('../../store/settingsStore', () => ({ useSettingsStore: (selector: (value: { updateConfig: () => void }) => unknown) => selector({ updateConfig: state.noop }) }));
vi.mock('../../store/uiStore', () => ({ useUIStore: (selector: (value: { addToast: () => void }) => unknown) => selector({ addToast: state.noop }) }));
const shortcutsHelp = vi.hoisted(() => ({ isOpen: false, open: vi.fn(), close: vi.fn() }));
vi.mock('../../store/shortcutsHelpStore', () => ({
  useShortcutsHelpStore: Object.assign((selector?: (value: typeof shortcutsHelp) => unknown) => selector ? selector(shortcutsHelp) : shortcutsHelp, { getState: () => shortcutsHelp }),
}));
vi.mock('../../store/workspaceChatModalStore', () => ({
  canPrepareWorkspaceChatOpen: () => false, prepareWorkspaceChatOpen: vi.fn(),
  registerWorkspaceChatCommandDispatcher: () => () => undefined,
}));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn() }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, callback: (payload: unknown) => void) => {
  state.events.set(name, callback);
  return () => { if (state.events.get(name) === callback) state.events.delete(name); };
} }));
vi.mock('react-i18next', async importOriginal => ({ ...await importOriginal<typeof import('react-i18next')>(), useTranslation: () => ({ t: state.t, i18n: { language: 'en' } }) }));
vi.mock('../ui/KeyboardShortcutsHelp', () => ({ KeyboardShortcutsHelp: () => null }));
vi.mock('./ConnectionStatusIndicator', () => ({ ConnectionStatusIndicator: () => null }));
vi.mock('../menu', () => ({ Menu: () => null }));
vi.mock('./MenuButton', () => ({ MenuButton: () => null }));
vi.mock('../../hooks/useAnchoredContextMenu', () => ({ useAnchoredContextMenu: () => ({ menu: state.menu, openForTrigger: state.noop, openAtPoint: state.noop, closeMenu: state.noop, onSelectItem: state.noop }) }));
vi.mock('../../hooks/useToolbarKeyboardNav', () => ({ useToolbarKeyboardNav: state.noop }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: state.announce, announceRequest: () => true }) }));
vi.mock('../../hooks/useDefaultFocus', () => ({ restoreDefaultFocus: state.noop }));
vi.mock('../../lib/commandContextReact', () => ({ useCommandContextScope: () => null }));
vi.mock('../../services/commandCatalog', () => ({ listCommandCatalog: vi.fn(async () => commandIDs.map(id => ({ id, name: id, available: true }))) }));
vi.mock('../../lib/commandBackendExecutionWails', () => ({ createCommandBackendExecutionWailsPort: () => ({ executeCommand: vi.fn() }) }));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({ createCommandLocalKeyboardWailsPort: () => ({
  loadMap: async () => {
    state.mapReady = true;
    return {
      generation: 'generation-1', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      localPaletteCommands: ['editor.mermaid.open'],
      bindings: [{ shortcut: { version: 1 as const, code: 'KeyJ', modifiers: ['Control' as const] }, commandId: state.commandID, handler: state.commandID === 'editor.mermaid.open' ? 'local_ui' as const : 'contextual' as const }],
    };
  },
  dispatchLocalCommandKey: vi.fn(async () => null), beginLocalCommandUIKey: state.beginKey,
  resetLocalCommandKeyboard: vi.fn(async () => undefined),
}) }));
function port() {
  return { beginUICommand: state.begin, takeUICommand: state.take, completeUICommand: state.complete,
    cancelUICommand: state.cancel, getUICommandResult: state.getResult, commitBackendCommand: state.commit };
}
vi.mock('../../lib/commandUIExecutionWails', () => ({ createCommandUIExecutionWailsPort: () => port() }));
vi.mock('../../lib/commandWorkspaceTabWails', () => ({ createCommandWorkspaceTabWailsPort: () => port() }));


let root: HTMLDivElement;
let unregister: (() => void) | undefined;
let revision = 0;
let editable = true;
let modalOpen = true;
const documentA = { id: 'document-a' };
const order: string[] = [];
const pending: Array<() => void> = [];
const reservation = () => ({ ticket: 'ticket-1', invocationId: 'invocation-1', commandId: state.commandID });
function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>(done => { resolve = done; });
  pending.push(resolve); return { promise, resolve };
}
beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  state.mapReady = false; state.pathname = '/'; state.commandID = commandID;
  revision = 0; editable = true; modalOpen = true; order.length = 0;
  state.begin.mockImplementation(async () => { order.push('begin'); return reservation(); });
  state.beginKey.mockImplementation(async () => { order.push('begin-key'); return reservation(); });
  state.take.mockImplementation(async () => { order.push('take'); return { ...reservation(), handoffId: 'handoff-1' }; });
  state.complete.mockImplementation(async () => { order.push('complete'); });
  state.cancel.mockResolvedValue(undefined);
  state.getResult.mockResolvedValue({ invocationId: 'invocation-1', status: 'succeeded' });
  state.prepareAdmission.mockImplementation(async () => { order.push('prepare'); return true; });
  state.execute.mockImplementation(() => { order.push('execute'); return true; });
});
afterEach(async () => {
  cleanup(); unregister?.(); unregister = undefined; root?.remove();
  await act(async () => { pending.splice(0).forEach(resolve => resolve()); await Promise.resolve(); });
  state.events.clear(); vi.restoreAllMocks(); vi.clearAllMocks(); vi.unstubAllGlobals();
});
async function mount(id: EditorMermaidCommandID = 'editor.mermaid.apply') {
  state.commandID = id;
  const view = render(<Topbar />);
  await waitFor(() => expect(state.mapReady).toBe(true));
  await act(async () => { await new Promise<void>(resolve => setTimeout(resolve, 50)); });
  root = document.createElement('div'); root.tabIndex = 0; document.body.appendChild(root); root.focus();
  unregister = registerEditorMermaidSurface({
    documentId: 'document-a',
    capture(command, expectedDocument) {
      if (expectedDocument && expectedDocument !== documentA || !editable || command !== 'editor.mermaid.open' && !modalOpen) return undefined;
      const capturedRevision = revision; let disposed = false;
      const current = () => !disposed && editable && capturedRevision === revision;
      return {
        isCurrent: current, canExecute: () => current(),
        prepare: async () => state.prepareAdmission(command, capturedRevision),
        execute: () => state.execute(command, capturedRevision), dispose: () => { disposed = true; },
      };
    },
  });
  return view;
}
async function openPalette() {
  const user = userEvent.setup();
  await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
  await user.type(await screen.findByRole('combobox'), state.commandID);
  return user;
}
async function trigger(source: Source) {
  if (source === 'button') act(() => { requestEditorMermaidCommand(state.commandID as EditorMermaidCommandID, { code: 'graph TD; A-->B' }, documentA); });
  if (source === 'keyboard') fireEvent.keyDown(root, { key: 'j', code: 'KeyJ', ctrlKey: true });
  if (source === 'palette') { const user = await openPalette(); await user.click(await screen.findByRole('option', { name: state.commandID })); }
  if (source === 'deck') act(() => {
    if (state.commandID === 'editor.mermaid.open') state.events.get('command:deck-local-ui')?.({
      commandId: state.commandID, generation: 'generation-1', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
    });
    else state.events.get('command:deck-ui-reservation')?.(reservation());
  });
}
describe('Topbar Mermaid: registry targets and real executor', () => {
  it.each<CommandShortcut>([
    { version: 1, code: 'KeyS', modifiers: ['Control'] },
    { version: 1, code: 'KeyS', modifiers: ['Meta'] },
    { version: 1, code: 'Enter', modifiers: ['Control'] },
  ])('modal shortcut $code/$modifiers preserves keyboard provenance and buffered release', async shortcut => {
    const beginModal = vi.fn(async () => reservation());
    const release = vi.fn(async () => undefined);
    vi.stubGlobal('go', { app: { App: { BeginEditorMermaidUIKey: beginModal, DispatchLocalCommandKey: release } } });
    await mount(); const ready = deferred();
    state.prepareAdmission.mockImplementationOnce(async () => { await ready.promise; return true; });
    act(() => { expect(requestEditorMermaidCommand('editor.mermaid.apply', { code: 'graph TD;' }, documentA, shortcut)).toBe(true); });
    await waitFor(() => expect(state.prepareAdmission).toHaveBeenCalledTimes(1));
    fireEvent.keyUp(window, { code: shortcut.code });
    expect(beginModal).not.toHaveBeenCalled(); expect(release).not.toHaveBeenCalled();
    await act(async () => ready.resolve());
    await waitFor(() => expect(state.complete).toHaveBeenCalledExactlyOnceWith('ticket-1', 'handoff-1', 'succeeded'));
    expect(beginModal).toHaveBeenCalledExactlyOnceWith('generation-1', shortcut, false);
    expect(release).toHaveBeenCalledExactlyOnceWith('generation-1', shortcut, 'up', false);
    expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled();
    expect(state.execute).toHaveBeenCalledTimes(1);
  });

  describe.each(['editor.mermaid.apply', 'editor.mermaid.remove'] as const)('%s', id => {
    it.each(sources)('%s awaits preparation before Begin and executes exactly once', async source => {
      await mount(id); const ready = deferred();
      state.prepareAdmission.mockImplementationOnce(async () => { order.push('prepare'); await ready.promise; return true; });
      await trigger(source); await waitFor(() => expect(state.prepareAdmission).toHaveBeenCalledTimes(1));
      expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled(); expect(state.take).not.toHaveBeenCalled();
      await act(async () => ready.resolve());
      await waitFor(() => expect(state.complete).toHaveBeenCalledExactlyOnceWith('ticket-1', 'handoff-1', 'succeeded'));
      expect(state.execute).toHaveBeenCalledExactlyOnceWith(id, 0);
      expect(order).toEqual(['prepare', ...(source === 'deck' ? [] : [source === 'keyboard' ? 'begin-key' : 'begin']), 'take', 'execute', 'complete']);
      if (source === 'deck') { expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled(); }
      else if (source === 'keyboard') expect(state.beginKey).toHaveBeenCalledTimes(1);
      else expect(state.begin).toHaveBeenCalledExactlyOnceWith(id);
    });
  });

  it.each(sources)('%s open remains local without Prepare or ledger', async source => {
    await mount('editor.mermaid.open'); await trigger(source);
    await waitFor(() => expect(state.execute).toHaveBeenCalledExactlyOnceWith('editor.mermaid.open', 0));
    expect(state.prepareAdmission).not.toHaveBeenCalled(); expect(state.begin).not.toHaveBeenCalled();
    expect(state.beginKey).not.toHaveBeenCalled(); expect(state.take).not.toHaveBeenCalled(); expect(state.complete).not.toHaveBeenCalled();
  });

  it('apply outside its modal is unavailable in palette', async () => {
    await mount(); modalOpen = false; await openPalette();
    const option = screen.queryByRole('option', { name: commandID });
    if (option) expect(option).toHaveAttribute('aria-disabled', 'true');
    expect(state.prepareAdmission).not.toHaveBeenCalled(); expect(state.begin).not.toHaveBeenCalled();
  });
  it('palette rejects source ABA without capturing a replacement', async () => {
    await mount(); const user = await openPalette(); revision += 2;
    await user.click(await screen.findByRole('option', { name: commandID }));
    expect(state.begin).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });
  it.each(['aba', 'readonly'] as const)('%s during prepare prevents Begin', async change => {
    await mount(); const ready = deferred(); state.prepareAdmission.mockImplementationOnce(async () => { await ready.promise; return true; });
    await trigger('button'); await waitFor(() => expect(state.prepareAdmission).toHaveBeenCalledTimes(1));
    if (change === 'aba') revision += 2; else editable = false;
    await act(async () => ready.resolve());
    expect(state.begin).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });
  it('cancelled preparation creates no reservation', async () => {
    await mount(); state.prepareAdmission.mockResolvedValueOnce(false); await trigger('button');
    await waitFor(() => expect(state.prepareAdmission).toHaveBeenCalledTimes(1));
    expect(state.begin).not.toHaveBeenCalled(); expect(state.take).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });
  it('false synchronous effect never completes succeeded', async () => {
    await mount(); state.execute.mockReturnValueOnce(false); state.getResult.mockResolvedValue({ invocationId: 'invocation-1', status: 'failed' });
    await trigger('button'); await waitFor(() => expect(state.cancel).toHaveBeenCalledWith('ticket-1'));
    expect(state.complete).not.toHaveBeenCalledWith('ticket-1', 'handoff-1', 'succeeded'); expect(state.execute).toHaveBeenCalledTimes(1);
  });
  it('lost completion acknowledgement reconciles once without replay', async () => {
    await mount(); state.complete.mockRejectedValueOnce(new Error('ack lost'));
    state.getResult.mockResolvedValue({ invocationId: 'invocation-1', status: 'outcome_unknown' });
    await trigger('button'); await waitFor(() => expect(state.announce).toHaveBeenCalledWith('commandPalette.executionUnknown'));
    expect(state.execute).toHaveBeenCalledTimes(1); expect(state.begin).toHaveBeenCalledTimes(1); expect(state.getResult).toHaveBeenCalledTimes(1);
  });
  it('duplicate Deck while preparing uses the original reservation only', async () => {
    await mount(); const ready = deferred(); state.prepareAdmission.mockImplementationOnce(async () => { await ready.promise; return true; });
    await trigger('deck'); await waitFor(() => expect(state.prepareAdmission).toHaveBeenCalledTimes(1)); await trigger('deck');
    await act(async () => ready.resolve()); await waitFor(() => expect(state.execute).toHaveBeenCalledTimes(1));
    expect(state.prepareAdmission).toHaveBeenCalledTimes(1); expect(state.take).toHaveBeenCalledTimes(1); expect(state.begin).not.toHaveBeenCalled();
  });
  it('repeat and IME do not prepare or begin', async () => {
    await mount();
    for (const guard of [{ repeat: true }, { isComposing: true }, { keyCode: 229 }]) fireEvent.keyDown(root, { key: 'j', code: 'KeyJ', ctrlKey: true, ...guard });
    await act(async () => { await Promise.resolve(); });
    expect(state.prepareAdmission).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled();
  });
});
