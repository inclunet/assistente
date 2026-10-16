import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor } from '@testing-library/react';
import { useLayoutEffect, useRef } from 'react';
import { Topbar } from './Topbar';
import { CommandContextProvider, useCommandContextScope } from '../../lib/commandContextReact';
import { registerChatClearSurface } from '../../lib/commandChatClear';
import { registerEditorFileSurface } from '../../lib/commandEditorFile';
import { registerEditorFormatAdapter } from '../../lib/commandEditorFormatting';
import { registerChatMessagingSurface } from '../../lib/commandChatMessaging';
import { registerTerminalOperationSurface } from '../../lib/commandTerminalOperation';
import { registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';

const state = vi.hoisted(() => ({
  map: vi.fn(), events: new Map<string, (payload?: unknown) => void>(), listeners: new Set<() => void>(),
  surfaceVersion: 'v1',
  navigate: vi.fn(), announce: vi.fn(),
  auth: { isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a' } },
  workspace: { workspace: { id: 'workspace-a', profile: 'focused', activeTabId: 'tab-a', tabs: [{ id: 'tab-a', type: 'chat' }] }, workspaces: [] },
  terminal: { sessions: [{ id: 'pty-a', state: 'running' }], historyBySession: { 'pty-a': [{ id: 'command-a', output: '', endedAt: '' }] }, activeEntryBySession: { 'pty-a': 'command-a' }, loadSessions: vi.fn(async () => undefined) },
}));
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key, i18n: { language: 'pt-BR' } }) }));
vi.mock('react-router-dom', () => ({ useNavigate: () => state.navigate, useLocation: () => ({ pathname: '/', search: '', hash: '', key: '/' }) }));
vi.mock('../../lib/commandGlobalOwnershipWails', () => ({ acquireGlobalCommandOwnership: () => ({ isReady: () => true, owns: () => false, dispose: () => {}, ready: Promise.resolve() }) }));
vi.mock('../../services/commandCatalog', () => ({ listCommandCatalog: vi.fn(async () => []) }));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({ createCommandLocalKeyboardWailsPort: () => ({ loadMap: state.map, beginLocalCommandUIKey: vi.fn(), resetLocalCommandKeyboard: vi.fn(), dispatchLocalCommandKey: vi.fn() }) }));
vi.mock('../../store/authStore', () => ({ useAuthStore: Object.assign((selector?: (value: typeof state.auth) => unknown) => selector ? selector(state.auth) : state.auth, { getState: () => state.auth, subscribe: () => () => {} }) }));
vi.mock('../../store/workspaceStore', () => ({ flushWorkspaceNavigation: vi.fn(async () => true), useWorkspaceStore: Object.assign((selector?: (value: typeof state.workspace) => unknown) => selector ? selector(state.workspace) : state.workspace, { getState: () => state.workspace, subscribe: (fn: () => void) => { state.listeners.add(fn); return () => state.listeners.delete(fn); } }) }));
vi.mock('../../store/shortcutsHelpStore', () => ({ useShortcutsHelpStore: (selector: (value: { isOpen: boolean; open: () => void; close: () => void }) => unknown) => selector({ isOpen: false, open: vi.fn(), close: vi.fn() }) }));
vi.mock('../../store/uiStore', () => ({ useUIStore: (selector: (value: { addToast: () => void }) => unknown) => selector({ addToast: vi.fn() }) }));
vi.mock('../../store/terminalStore', () => ({ useTerminalStore: Object.assign((selector?: (value: typeof state.terminal) => unknown) => selector ? selector(state.terminal) : state.terminal, { getState: () => state.terminal, subscribe: () => () => undefined }) }));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: state.announce }) }));
vi.mock('../../hooks/useDefaultFocus', () => ({ restoreDefaultFocus: vi.fn() }));
vi.mock('../../store/workspaceChatModalStore', () => ({ canPrepareWorkspaceChatOpen: () => false, registerWorkspaceChatCommandDispatcher: () => () => {}, prepareWorkspaceChatOpen: vi.fn() }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, fn: (payload?: unknown) => void) => { state.events.set(name, fn); return () => state.events.delete(name); } }));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn(async () => null) }));
vi.mock('../pickers/ProfilePicker', () => ({ ProfilePicker: () => null }));

const api = { BeginContextualDeckUICommand: vi.fn(), BeginUICommand: vi.fn(), BeginContextualPaletteUICommand: vi.fn(),
  ExecuteContextualDeckLayerCommand: vi.fn(), ExecuteContextualPaletteLayerCommand: vi.fn(), ExecutePaletteCommand: vi.fn(),
  PrepareTerminalInterruptCommand: vi.fn(), PrepareTerminalSessionCommand: vi.fn(),
  TakeUICommand: vi.fn(), CompleteUICommand: vi.fn(), CancelUICommand: vi.fn(), GetUICommandResult: vi.fn(), CommitWorkspaceTabCommand: vi.fn() };
const files = { EditorPrepareCommand: vi.fn(), EditorCommitCommand: vi.fn() };
const releases: Array<() => void> = [];
let commandId = 'workspace.tab.chat.create';
function reservation(id = commandId) { return { ticket: 'ticket', invocationId: 'invocation', commandId: id }; }
function handoff() { return { ...reservation(), handoffId: 'handoff' }; }
function profileABA() {
  state.workspace.workspace.profile = 'other'; state.listeners.forEach(fn => fn());
  state.workspace.workspace.profile = 'focused'; state.listeners.forEach(fn => fn());
}
function Source({ type, registered }: { type: string; registered: boolean }) {
  const scope = useCommandContextScope(); const root = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => registered ? scope?.registerSurface('tab-a', root, () => ({ surfaceId: 'tab-a', surfaceType: type, snapshotVersion: state.surfaceVersion })) : undefined, [scope, type, registered]);
  return <div ref={root} className="ws-content__panel"><button data-testid="source">source</button><input data-testid="input" /></div>;
}
function condition(id = commandId, enabled = true, type = 'chat') { return { commandId: id, bySurface: { [type]: enabled }, fallback: false }; }
const envelope = { offerId: 'offer', generation: 'map', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' };
async function mount(type = 'chat', registered = true) {
  state.workspace.workspace.tabs[0].type = type;
  const view = render(<CommandContextProvider><div className="workspace-layout"><Topbar /><Source type={type} registered={registered} /></div></CommandContextProvider>);
  await act(async () => { await Promise.resolve(); });
  screen.getByTestId('source').focus();
  const event = (conditions: unknown = [condition(commandId, true, type)], extra = {}) => state.events.get('command:deck-contextual-ui')?.({ ...envelope, conditions, ...extra });
  return { view, event, root: screen.getByTestId('source').parentElement! };
}

describe('Topbar physical contextual Deck offers — real providers and ports', () => {
  beforeEach(() => {
    state.events.clear(); state.listeners.clear(); state.navigate.mockClear(); state.announce.mockClear();
    state.surfaceVersion = 'v1';
    state.auth = { isAuthenticated: true, user: { userId: 'user-a', sessionId: 'session-a' } };
    state.workspace.workspace = { id: 'workspace-a', profile: 'focused', activeTabId: 'tab-a', tabs: [{ id: 'tab-a', type: 'chat' }] };
    commandId = 'workspace.tab.chat.create';
    Object.values(api).forEach(fn => fn.mockReset()); Object.values(files).forEach(fn => fn.mockReset());
    api.BeginContextualDeckUICommand.mockImplementation(async () => reservation());
    api.TakeUICommand.mockImplementation(async () => handoff());
    api.GetUICommandResult.mockResolvedValue({ invocationId: 'invocation', status: 'succeeded' });
    files.EditorPrepareCommand.mockResolvedValue({ token: 'native', path: '/a.md', requiresOverwrite: false, cancelled: false });
    files.EditorCommitCommand.mockResolvedValue({ tabId: 'tab-a', path: '/a.md', written: true });
    state.map.mockResolvedValue({ generation: 'map', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a', bindings: [], localPaletteCommands: [] });
    Object.assign(window, { go: { app: { App: api }, wailsapi: { Editor: files } } });
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  });
  afterEach(() => { releases.splice(0).forEach(fn => fn()); unregisterOpenModal('deck-decision'); Reflect.deleteProperty(window, 'go'); vi.restoreAllMocks(); });

  it.each(['workspace.tab.chat.create', 'workspace.tab.editor.create', 'workspace.tab.close'])('%s uses offer Begin and existing Commit, never palette', async id => {
    commandId = id; const { view, event } = await mount();
    try {
      await act(async () => event());
      await waitFor(() => expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff'));
      expect(api.BeginContextualDeckUICommand).toHaveBeenCalledExactlyOnceWith('offer', 'map', { surfaceType: 'chat', surfaceId: 'tab-a', profile: 'focused', appPage: 'workspace' });
      expect(api.BeginUICommand).not.toHaveBeenCalled(); expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['layer.activate', 'layer.toggle', 'layer.back'])('%s executes once from the mixed event and survives self map invalidation', async id => {
    commandId = id;
    api.ExecuteContextualDeckLayerCommand.mockImplementation(async () => {
      state.events.get('command:keyboard-map-changed')?.();
      return { invocationId: 'layer-invocation', status: 'succeeded' };
    });
    const { view, event } = await mount();
    try {
      const conditions = [condition(id), condition('navigation.settings.open', false), condition('workspace.tab.close', false)];
      await act(async () => { event(conditions); event(conditions); });
      await waitFor(() => expect(state.announce).toHaveBeenCalledWith('commandSettings.layerActionCompleted'));
      expect(api.ExecuteContextualDeckLayerCommand).toHaveBeenCalledExactlyOnceWith('offer', 'map', { surfaceType: 'chat', surfaceId: 'tab-a', profile: 'focused', appPage: 'workspace' });
      expect(api.BeginContextualDeckUICommand).not.toHaveBeenCalled(); expect(api.TakeUICommand).not.toHaveBeenCalled();
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled(); expect(api.ExecutePaletteCommand).not.toHaveBeenCalled();
      expect(api.ExecuteContextualPaletteLayerCommand).not.toHaveBeenCalled(); expect(state.navigate).not.toHaveBeenCalled();
      expect(state.announce).not.toHaveBeenCalledWith('commandPalette.executionUnknown');
    } finally { view.unmount(); }
  });

  it.each(['local', 'durable', 'ambiguous'])('mixed layer/local/durable selection %s has exactly one route', async winner => {
    const { view, event } = await mount();
    try {
      await act(async () => event([
        condition('layer.toggle', winner === 'ambiguous'),
        condition('navigation.settings.open', winner === 'local' || winner === 'ambiguous'),
        condition(commandId, winner === 'durable'),
      ]));
      if (winner === 'durable') await waitFor(() => expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledOnce());
      expect(state.navigate).toHaveBeenCalledTimes(winner === 'local' ? 1 : 0);
      expect(api.BeginContextualDeckUICommand).toHaveBeenCalledTimes(winner === 'durable' ? 1 : 0);
      expect(api.ExecuteContextualDeckLayerCommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['profile-aba', 'session', 'ime', 'modal', 'modal-aba', 'provider', 'deadline', 'reload', 'missing-api'])('layer final submission guard rejects %s after bridge lookup', async mode => {
    commandId = 'layer.toggle'; const deadline = Date.now() + 60000; (await state.map()).validUntil = deadline;
    const { view, event } = await mount();
    const execute = api.ExecuteContextualDeckLayerCommand;
    if (mode === 'ime') {
      screen.getByTestId('input').focus();
      fireEvent.compositionStart(screen.getByTestId('input'));
      fireEvent.compositionEnd(screen.getByTestId('input'));
    }
    const readAPI = vi.fn(() => {
      if (mode === 'profile-aba') profileABA();
      if (mode === 'session') state.auth.user = { userId: 'user-a', sessionId: 'other' };
      if (mode === 'ime') fireEvent.compositionStart(screen.getByTestId('input'));
      if (mode === 'provider') state.surfaceVersion = 'v2';
      if (mode === 'deadline') vi.spyOn(Date, 'now').mockReturnValue(deadline);
      if (mode === 'reload') state.events.get('command:keyboard-map-changed')?.();
      if (mode === 'modal' || mode === 'modal-aba') {
        const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; document.body.append(overlay);
        registerOpenModal('deck-decision'); releases.push(() => overlay.remove());
        if (mode === 'modal-aba') { unregisterOpenModal('deck-decision'); overlay.remove(); }
      }
      return mode === 'missing-api' ? undefined : execute;
    });
    Object.defineProperty(api, 'ExecuteContextualDeckLayerCommand', { configurable: true, get: readAPI });
    try {
      await act(async () => event());
      expect(readAPI).toHaveBeenCalledOnce();
      expect(execute).not.toHaveBeenCalled(); expect(api.BeginContextualDeckUICommand).not.toHaveBeenCalled();
      expect(api.ExecuteContextualPaletteLayerCommand).not.toHaveBeenCalled(); expect(api.ExecutePaletteCommand).not.toHaveBeenCalled();
    } finally { Object.defineProperty(api, 'ExecuteContextualDeckLayerCommand', { configurable: true, writable: true, value: execute }); view.unmount(); }
  });

  it.each(['missing', 'page'])('layer rejects %s provider even with true fallback', async kind => {
    commandId = 'layer.back'; const { view, event } = await mount(kind === 'page' ? 'profiles' : 'chat', kind !== 'missing');
    try {
      await act(async () => event([{ commandId, bySurface: {}, fallback: true }]));
      expect(api.ExecuteContextualDeckLayerCommand).not.toHaveBeenCalled(); expect(api.BeginContextualDeckUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it('layer success after session drift cannot announce into the new owner or reexecute', async () => {
    commandId = 'layer.activate';
    api.ExecuteContextualDeckLayerCommand.mockImplementation(async () => {
      state.auth.user = { userId: 'other', sessionId: 'other' };
      return { invocationId: 'layer-invocation', status: 'succeeded' };
    });
    const { view, event } = await mount();
    try {
      await act(async () => event());
      expect(api.ExecuteContextualDeckLayerCommand).toHaveBeenCalledOnce();
      expect(state.announce).not.toHaveBeenCalledWith('commandSettings.layerActionCompleted');
      expect(state.announce).not.toHaveBeenCalledWith('commandPalette.executionUnknown');
      expect(api.BeginContextualDeckUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['profile-aba', 'provider-version', 'provider-disposed'])('layer backend success after %s suppresses stale presentation only', async mode => {
    commandId = 'layer.activate';
    let resolve!: (result: { invocationId: string; status: string }) => void;
    api.ExecuteContextualDeckLayerCommand.mockReturnValue(new Promise(done => { resolve = done; }));
    const { view, event } = await mount();
    try {
      await act(async () => event());
      expect(api.ExecuteContextualDeckLayerCommand).toHaveBeenCalledOnce();
      await act(async () => {
        if (mode === 'profile-aba') profileABA();
        if (mode === 'provider-version') state.surfaceVersion = 'v2';
        if (mode === 'provider-disposed') view.rerender(<CommandContextProvider><div className="workspace-layout"><Topbar /><Source type="chat" registered={false} /></div></CommandContextProvider>);
      });
      await act(async () => resolve({ invocationId: 'layer-invocation', status: 'succeeded' }));
      expect(state.announce).not.toHaveBeenCalledWith('commandSettings.layerActionCompleted');
      expect(state.announce).not.toHaveBeenCalledWith('commandPalette.executionUnknown');
      expect(state.announce).not.toHaveBeenCalledWith('commandPalette.executionFailed');
      expect(api.ExecuteContextualDeckLayerCommand).toHaveBeenCalledOnce();
      expect(api.BeginContextualDeckUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it('mixed local winner executes once without any Begin/ledger', async () => {
    const { view, event } = await mount();
    try {
      act(() => { event([condition(commandId, false), condition('navigation.settings.open')]); event([condition(commandId, false), condition('navigation.settings.open')]); });
      expect(state.navigate).toHaveBeenCalledExactlyOnceWith('/settings');
      expect(Object.values(api).some(fn => fn.mock.calls.length)).toBe(false);
    } finally { view.unmount(); }
  });

  it.each(['ambiguous', 'malformed', 'layer', 'page', 'mermaid', 'old-map', 'missing-provider', 'ime'])('rejects %s without Begin or local fallback', async mode => {
    const { view, event } = await mount('chat', mode !== 'missing-provider');
    try {
      if (mode === 'ime') { screen.getByTestId('input').focus(); fireEvent.compositionStart(screen.getByTestId('input')); }
      let conditions: unknown = [condition()];
      if (mode === 'ambiguous') conditions = [condition(), condition('navigation.settings.open')];
      if (mode === 'malformed') conditions = [{ ...condition(), fallback: 'true' }];
      if (mode === 'layer') conditions = [condition('layer.unknown')];
      if (mode === 'page') conditions = [condition('profiles.update')];
      if (mode === 'mermaid') conditions = [condition('editor.mermaid.apply')];
      await act(async () => event(conditions, mode === 'old-map' ? { generation: 'old' } : {}));
      expect(api.BeginContextualDeckUICommand).not.toHaveBeenCalled(); expect(api.BeginUICommand).not.toHaveBeenCalled();
      expect(state.navigate).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['begin', 'take', 'mismatch'])('cancels %s drift with no commit or fallback', async phase => {
    if (phase === 'begin') api.BeginContextualDeckUICommand.mockImplementation(async () => { profileABA(); return reservation(); });
    if (phase === 'take') api.TakeUICommand.mockImplementation(async () => { profileABA(); return handoff(); });
    if (phase === 'mismatch') api.BeginContextualDeckUICommand.mockResolvedValue(reservation('workspace.tab.close'));
    const { view, event } = await mount();
    try {
      await act(async () => event());
      await waitFor(() => expect(api.CancelUICommand.mock.calls.length + api.CompleteUICommand.mock.calls.length).toBeGreaterThan(0));
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled(); expect(api.BeginUICommand).not.toHaveBeenCalled();
      expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['accepted', 'cancelled'])('clear retains original target through decision %s', async mode => {
    commandId = 'chat.conversation.clear'; const { view, event, root } = await mount(); const succeeded = vi.fn();
    releases.push(registerChatClearSurface({ root, instanceId: 'chat', isCurrent: () => true, canStart: () => true, subscribe: () => () => {}, succeeded }));
    api.TakeUICommand.mockImplementation(async () => {
      const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; document.body.append(overlay);
      registerOpenModal('deck-decision');
      await Promise.resolve();
      unregisterOpenModal('deck-decision'); overlay.remove(); screen.getByTestId('source').focus();
      if (mode === 'cancelled') throw new Error('cancelled');
      return handoff();
    });
    try {
      await act(async () => event());
      if (mode === 'accepted') await waitFor(() => expect(succeeded).toHaveBeenCalledOnce());
      else { await waitFor(() => expect(api.CancelUICommand).toHaveBeenCalled()); expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled(); }
      expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['owner', 'tab', 'focus', 'reload', 'expiry', 'modal'])('guards %s after Take before Commit', async mode => {
    const deadline = Date.now() + 60000; (await state.map()).validUntil = deadline;
    const { view, event } = await mount();
    api.TakeUICommand.mockImplementation(async () => {
      if (mode === 'owner') state.auth.user = { userId: 'other', sessionId: 'session-a' };
      if (mode === 'tab') state.workspace.workspace.activeTabId = 'tab-b';
      if (mode === 'focus') screen.getByTestId('input').focus();
      if (mode === 'reload') state.events.get('command:keyboard-map-changed')?.();
      if (mode === 'expiry') vi.spyOn(Date, 'now').mockReturnValue(deadline);
      if (mode === 'modal') {
        const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; document.body.append(overlay);
        registerOpenModal('deck-decision'); releases.push(() => overlay.remove());
      }
      return handoff();
    });
    try {
      await act(async () => event());
      await waitFor(() => expect(api.CompleteUICommand).toHaveBeenCalledWith('ticket', 'handoff', 'cancelled'));
      expect(api.CommitWorkspaceTabCommand).not.toHaveBeenCalled(); expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['success', 'prepare-drift', 'take-drift'])('format keeps original adapter and guards %s', async mode => {
    commandId = 'editor.format.bold';
    const execute = vi.fn(() => true);
    releases.push(registerEditorFormatAdapter({ capture: () => ({ isCurrent: () => true, canExecute: () => true,
      prepare: async () => { if (mode === 'prepare-drift') profileABA(); return true; }, execute, dispose: () => {} }) }));
    api.TakeUICommand.mockImplementation(async () => { if (mode === 'take-drift') profileABA(); return handoff(); });
    const { view, event } = await mount('editor');
    try {
      await act(async () => event());
      if (mode === 'success') await waitFor(() => expect(execute).toHaveBeenCalledOnce());
      else if (mode === 'take-drift') await waitFor(() => expect(api.CompleteUICommand).toHaveBeenCalledWith('ticket', 'handoff', 'cancelled'));
      if (mode !== 'success') expect(execute).not.toHaveBeenCalled();
      if (mode === 'prepare-drift') expect(api.BeginContextualDeckUICommand).not.toHaveBeenCalled();
      expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['terminal.command.interrupt', 'terminal.session.create', 'terminal.session.close'])('terminal %s preserves its Prepare route', async id => {
    commandId = id; Object.assign(state.workspace.workspace.tabs[0], { state: { sessionId: 'pty-a' } });
    const { view, event, root } = await mount('terminal');
    releases.push(registerTerminalOperationSurface({ root, instanceId: 'terminal', tabId: 'tab-a', isCurrent: () => true, canStart: () => true, subscribe: () => () => {} }));
    try {
      await act(async () => event());
      await waitFor(() => expect(api.CommitWorkspaceTabCommand).toHaveBeenCalledOnce());
      expect(id === 'terminal.command.interrupt' ? api.PrepareTerminalInterruptCommand : api.PrepareTerminalSessionCommand).toHaveBeenCalledOnce();
      expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['success', 'prepare-drift', 'take-drift'])('messaging effect keeps target through %s', async mode => {
    commandId = 'chat.message.copy'; const execute = vi.fn(async () => undefined);
    const { view, event, root } = await mount();
    releases.push(registerChatMessagingSurface({ root, instanceId: 'message', isCurrent: () => true, canStart: () => true, subscribe: () => () => {},
      prepare: () => ({ isCurrent: () => true, canCommit: () => true, prepareAdmission: async () => { if (mode === 'prepare-drift') profileABA(); }, execute, dispose: () => {} }) }));
    api.TakeUICommand.mockImplementation(async () => { if (mode === 'take-drift') profileABA(); return handoff(); });
    try {
      await act(async () => event());
      await waitFor(() => expect(api.CompleteUICommand.mock.calls.length + api.CancelUICommand.mock.calls.length).toBeGreaterThan(0));
      expect(execute).toHaveBeenCalledTimes(mode === 'success' ? 1 : 0);
      expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each(['editor.file.save', 'editor.file.open', 'editor.file.save_copy', 'profile-aba'])('native file continuation %s survives only legitimate blur', async mode => {
    commandId = mode.startsWith('editor.file.') ? mode : 'editor.file.save';
    const { view, event, root } = await mount('editor');
    releases.push(registerEditorFileSurface({ root, ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a', tabId: 'tab-a',
      documentId: 'doc', instanceId: 'editor', generation: 'doc-v1', isActive: () => true, isCurrent: () => true, canExecute: () => true, canCommit: () => true,
      prepare: async () => ({ content: 'original', labels: {}, suggestedFilename: 'a.md', confirmOverwrite: false }), confirmOverwrite: async () => false }));
    files.EditorPrepareCommand.mockImplementation(async () => {
      fireEvent.blur(window); state.events.get('command:keyboard-map-changed')?.();
      if (mode === 'profile-aba') profileABA();
      fireEvent.focus(window);
      return { token: 'native', path: '/a.md', requiresOverwrite: false, cancelled: false };
    });
    try {
      await act(async () => event());
      if (mode === 'profile-aba') { await waitFor(() => expect(api.CompleteUICommand).toHaveBeenCalledWith('ticket', 'handoff', 'cancelled')); expect(files.EditorCommitCommand).not.toHaveBeenCalled(); }
      else await waitFor(() => expect(files.EditorCommitCommand).toHaveBeenCalledOnce());
      expect(api.BeginContextualDeckUICommand).toHaveBeenCalledOnce(); expect(api.BeginContextualPaletteUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });
});
