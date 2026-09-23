import { afterEach, beforeEach, expect, it, vi } from 'vitest';
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
import {
  registerTerminalOperationSurface,
  requestTerminalOperation,
  requestTerminalSessionOperation,
  TERMINAL_INTERRUPT_COMMAND as id,
  TERMINAL_SESSION_CREATE_COMMAND,
  TERMINAL_SESSION_CLOSE_COMMAND,
} from '../../lib/commandTerminalOperation';
import { registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';

const state = vi.hoisted(() => ({
  pathname: '/', ready: false, flush: vi.fn(), reconcile: vi.fn(), begin: vi.fn(), beginKey: vi.fn(), prepare: vi.fn(), prepareSession: vi.fn(),
  keyCommand: 'terminal.command.interrupt',
  take: vi.fn(), commit: vi.fn(), result: vi.fn(), cancel: vi.fn(), complete: vi.fn(),
  events: new Map<string, (payload: unknown) => void>(), announce: vi.fn(), noop: () => undefined,
  t: (key: string) => key,
}));
const auth = { isAuthenticated: true, user: { userId: 'user-a', sessionId: 'auth-a', role: 'user' } };
const workspace = { id: 'workspace-a', name: 'Workspace', activeTabId: 'terminal-a',
  tabs: [{ id: 'terminal-a', type: 'terminal', state: { sessionId: 'pty-a' } }] };
const terminal = { sessions: [{ id: 'pty-a', state: 'running' }],
  historyBySession: { 'pty-a': [{ id: 'command-a', output: '', endedAt: '' }] },
  activeEntryBySession: { 'pty-a': 'command-a' } };
const listeners = new Set<() => void>();
const reservation = { ticket: 'ticket-a', invocationId: 'invocation-a', commandId: 'terminal.command.interrupt' };
const handoff = { ...reservation, handoffId: 'handoff-a' };

vi.mock('react-router-dom', async original => ({ ...await original<typeof import('react-router-dom')>(),
  useNavigate: () => state.noop, useLocation: () => ({ pathname: state.pathname, search: '', hash: '', key: state.pathname }),
}));
vi.mock('../../store/authStore', () => ({ useAuthStore: Object.assign(
  (select?: (value: typeof auth) => unknown) => select ? select(auth) : auth,
  { getState: () => auth, subscribe: () => () => undefined },
) }));
vi.mock('../../store/workspaceStore', () => ({ flushWorkspaceNavigation: state.flush,
  useWorkspaceStore: Object.assign(
    (select?: (value: { workspace: typeof workspace; workspaces: never[]; setActiveTab: () => void; reconcileActiveSelection: () => Promise<boolean> }) => unknown) =>
      select ? select({ workspace, workspaces: [], setActiveTab: state.noop, reconcileActiveSelection: state.reconcile }) : { workspace, workspaces: [] },
    { getState: () => ({ workspace, workspaces: [], reconcileActiveSelection: state.reconcile }), subscribe: () => () => undefined },
  ),
}));
vi.mock('../../store/terminalStore', () => ({ useTerminalStore: Object.assign(
  (select?: (value: typeof terminal) => unknown) => select ? select(terminal) : terminal,
  { getState: () => ({ ...terminal, loadSessions: state.reconcile }), subscribe: (fn: () => void) => { listeners.add(fn); return () => listeners.delete(fn); } },
) }));
vi.mock('../../store/settingsStore', () => ({ useSettingsStore: (select: (value: { updateConfig: () => void }) => unknown) => select({ updateConfig: state.noop }) }));
vi.mock('../../store/uiStore', () => ({ useUIStore: (select: (value: { addToast: () => void }) => unknown) => select({ addToast: state.noop }) }));
vi.mock('../../store/shortcutsHelpStore', () => ({ useShortcutsHelpStore: Object.assign(
  (select?: (value: { isOpen: boolean; open: () => void; close: () => void }) => unknown) => select ? select({ isOpen: false, open: state.noop, close: state.noop }) : { isOpen: false },
  { getState: () => ({ isOpen: false, open: state.noop, close: state.noop }) },
) }));
vi.mock('../../store/workspaceChatModalStore', () => ({ canPrepareWorkspaceChatOpen: () => false,
  prepareWorkspaceChatOpen: vi.fn(), registerWorkspaceChatCommandDispatcher: () => () => undefined,
}));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn() }));
vi.mock('@wailsjs/runtime/runtime', () => ({ EventsOn: (name: string, callback: (payload: unknown) => void) => {
  state.events.set(name, callback); return () => { if (state.events.get(name) === callback) state.events.delete(name); };
} }));
vi.mock('react-i18next', async original => ({ ...await original<typeof import('react-i18next')>(), useTranslation: () => ({ t: state.t, i18n: { language: 'en' } }) }));
vi.mock('../ui/KeyboardShortcutsHelp', () => ({ KeyboardShortcutsHelp: () => null }));
vi.mock('./ConnectionStatusIndicator', () => ({ ConnectionStatusIndicator: () => null }));
vi.mock('../menu', () => ({ Menu: () => null }));
vi.mock('./MenuButton', () => ({ MenuButton: () => null }));
vi.mock('../../hooks/useAnchoredContextMenu', () => ({ useAnchoredContextMenu: () => ({ menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' }, openForTrigger: state.noop, openAtPoint: state.noop, closeMenu: state.noop, onSelectItem: state.noop }) }));
vi.mock('../../hooks/useToolbarKeyboardNav', () => ({ useToolbarKeyboardNav: state.noop }));
vi.mock('../../hooks/useAnnouncer', () => ({ announce: state.announce, useAnnouncer: () => ({ announce: state.announce, announceRequest: () => true }) }));
vi.mock('../../lib/commandContextReact', () => ({ useCommandContextScope: () => null }));
vi.mock('../../services/commandCatalog', () => ({ listCommandCatalog: vi.fn(async () => [
  { id: 'terminal.command.interrupt', name: 'Interromper terminal', available: true },
  { id: 'terminal.session.create', name: 'Criar sessão terminal', available: true },
  { id: 'terminal.session.close', name: 'Fechar sessão terminal', available: true },
]) }));
vi.mock('../../lib/commandBackendExecutionWails', () => ({ createCommandBackendExecutionWailsPort: () => ({ executeCommand: vi.fn() }) }));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({ createCommandLocalKeyboardWailsPort: () => ({
  loadMap: async () => {
    state.ready = true;
    return { generation: 'generation-a', ownerId: 'user-a', sessionId: 'auth-a', workspaceId: 'workspace-a',
      localPaletteCommands: [], bindings: [{ shortcut: { version: 1, code: 'KeyJ', modifiers: ['Control'] }, commandId: state.keyCommand, handler: 'contextual' }] };
  },
  beginLocalCommandUIKey: state.beginKey, dispatchLocalCommandKey: vi.fn(), resetLocalCommandKeyboard: vi.fn(),
}) }));
function port() { return { beginUICommand: state.begin, takeUICommand: state.take, completeUICommand: state.complete,
  cancelUICommand: state.cancel, getUICommandResult: state.result, commitBackendCommand: state.commit,
  prepareTerminalInterruptCommand: state.prepare, prepareTerminalSessionCommand: state.prepareSession }; }
vi.mock('../../lib/commandUIExecutionWails', () => ({ createCommandUIExecutionWailsPort: () => port() }));
vi.mock('../../lib/commandWorkspaceTabWails', () => ({ createCommandWorkspaceTabWailsPort: () => port() }));
vi.mock('../../lib/commandTerminalOperationWails', () => ({ createCommandTerminalOperationWailsPort: () => port() }));

let root: HTMLElement;
let input: HTMLTextAreaElement;
let unregister: (() => void) | undefined;
beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  state.ready = false; state.pathname = '/'; state.events.clear(); state.keyCommand = id;
  workspace.activeTabId = 'terminal-a'; terminal.activeEntryBySession['pty-a'] = 'command-a';
  state.flush.mockResolvedValue(true); state.reconcile.mockResolvedValue(true);
  state.begin.mockResolvedValue(reservation); state.beginKey.mockResolvedValue(reservation);
  state.prepare.mockResolvedValue(undefined); state.prepareSession.mockResolvedValue(undefined);
  state.take.mockResolvedValue(handoff); state.commit.mockResolvedValue(undefined);
  state.result.mockResolvedValue({ invocationId: reservation.invocationId, status: 'succeeded' });
});
afterEach(() => {
  cleanup(); unregister?.(); root?.remove(); listeners.clear(); unregisterOpenModal('terminal-test-modal');
  vi.restoreAllMocks(); vi.clearAllMocks();
});
async function mount() {
  render(<Topbar />);
  await waitFor(() => expect(state.ready).toBe(true));
  await act(async () => { await Promise.resolve(); });
  root = document.createElement('main'); input = document.createElement('textarea'); root.appendChild(input); document.body.appendChild(root);
  unregister = registerTerminalOperationSurface({ root, instanceId: 'terminal-surface', tabId: 'terminal-a',
    isCurrent: () => true, canStart: () => true,
    subscribe: changed => { listeners.add(changed); return () => listeners.delete(changed); },
  });
  input.focus();
}
async function trigger(source: string) {
  if (source === 'button') act(() => { requestTerminalOperation('terminal-surface'); });
  if (source === 'keyboard') fireEvent.keyDown(input, { key: 'j', code: 'KeyJ', ctrlKey: true });
  if (source === 'deck') act(() => { state.events.get('command:deck-ui-reservation')?.(reservation); });
  if (source === 'palette') {
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await user.click(await screen.findByRole('option', { name: 'Interromper terminal' }));
  }
}

async function triggerSession(source: string, commandId: typeof TERMINAL_SESSION_CREATE_COMMAND | typeof TERMINAL_SESSION_CLOSE_COMMAND) {
  const admitted = { ...reservation, commandId };
  const admittedHandoff = { ...handoff, commandId };
  state.keyCommand = commandId;
  state.begin.mockResolvedValue(admitted);
  state.beginKey.mockResolvedValue(admitted);
  state.take.mockResolvedValue(admittedHandoff);
  state.result.mockResolvedValue({ invocationId: admitted.invocationId, status: 'succeeded' });
  if (source === 'native') act(() => { requestTerminalSessionOperation(commandId, 'terminal-surface'); });
  if (source === 'key') fireEvent.keyDown(input, { key: 'j', code: 'KeyJ', ctrlKey: true });
  if (source === 'deck') act(() => { state.events.get('command:deck-ui-reservation')?.(admitted); });
  if (source === 'palette') {
    const user = userEvent.setup();
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await user.click(await screen.findByRole('option', {
      name: commandId === TERMINAL_SESSION_CREATE_COMMAND ? 'Criar sessão terminal' : 'Fechar sessão terminal',
    }));
  }
}

it.each(['button', 'keyboard', 'palette', 'deck'])('interrupt via %s prepares the visible target and commits once', async source => {
  await mount(); await trigger(source);
  await waitFor(() => expect(state.result).toHaveBeenCalledOnce());
  expect(state.prepare).toHaveBeenCalledExactlyOnceWith('ticket-a', 'workspace-a', 'terminal-a', 'pty-a', 'command-a');
  expect(state.take).toHaveBeenCalledExactlyOnceWith('ticket-a');
  expect(state.commit).toHaveBeenCalledExactlyOnceWith('ticket-a', 'handoff-a');
  expect(state.complete).not.toHaveBeenCalled();
  expect(state.prepare.mock.invocationCallOrder[0]).toBeLessThan(state.take.mock.invocationCallOrder[0]);
  expect(state.take.mock.invocationCallOrder[0]).toBeLessThan(state.commit.mock.invocationCallOrder[0]);
  if (source === 'keyboard') {
    expect(state.beginKey).toHaveBeenCalledExactlyOnceWith('generation-a', { version: 1, code: 'KeyJ', modifiers: ['Control'] }, false);
    expect(state.begin).not.toHaveBeenCalled();
  } else if (source === 'deck') { expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled(); }
  else expect(state.begin).toHaveBeenCalledExactlyOnceWith(id);
});

it.each([
  ['native', TERMINAL_SESSION_CREATE_COMMAND],
  ['palette', TERMINAL_SESSION_CREATE_COMMAND],
  ['key', TERMINAL_SESSION_CREATE_COMMAND],
  ['deck', TERMINAL_SESSION_CREATE_COMMAND],
  ['native', TERMINAL_SESSION_CLOSE_COMMAND],
  ['palette', TERMINAL_SESSION_CLOSE_COMMAND],
  ['key', TERMINAL_SESSION_CLOSE_COMMAND],
  ['deck', TERMINAL_SESSION_CLOSE_COMMAND],
] as const)('%s executa %s pela entrada central sem Begin duplicado', async (source, commandId) => {
  if (source === 'key') state.keyCommand = commandId;
  await mount();
  await triggerSession(source, commandId);
  await waitFor(() => expect(state.result).toHaveBeenCalledOnce());
  expect(state.prepareSession).toHaveBeenCalledExactlyOnceWith('ticket-a', 'workspace-a', 'terminal-a', 'pty-a');
  expect(state.take).toHaveBeenCalledExactlyOnceWith('ticket-a');
  expect(state.commit).toHaveBeenCalledExactlyOnceWith('ticket-a', 'handoff-a');
  expect(state.prepare).not.toHaveBeenCalled();
  if (source === 'key') {
    expect(state.beginKey).toHaveBeenCalledTimes(1);
    expect(state.begin).not.toHaveBeenCalled();
  } else if (source === 'deck') {
    expect(state.begin).not.toHaveBeenCalled();
    expect(state.beginKey).not.toHaveBeenCalled();
  } else {
    expect(state.begin).toHaveBeenCalledExactlyOnceWith(commandId);
  }
});

it('rejects stale execution during preparation, including ABA, without retarget or commit', async () => {
  await mount(); let release!: () => void;
  state.prepare.mockImplementationOnce(() => new Promise<void>(done => { release = done; }));
  await trigger('button'); await waitFor(() => expect(state.prepare).toHaveBeenCalledOnce());
  act(() => {
    terminal.activeEntryBySession['pty-a'] = 'command-b'; listeners.forEach(fn => fn());
    terminal.activeEntryBySession['pty-a'] = 'command-a'; listeners.forEach(fn => fn()); release();
  });
  await waitFor(() => expect(state.cancel).toHaveBeenCalledWith('ticket-a'));
  expect(state.take).not.toHaveBeenCalled(); expect(state.commit).not.toHaveBeenCalled();
});
it('does not begin when workspace navigation could not be synchronized', async () => {
  await mount(); state.flush.mockResolvedValue(false); await trigger('button');
  await waitFor(() => expect(state.flush).toHaveBeenCalled());
  expect(state.begin).not.toHaveBeenCalled(); expect(state.commit).not.toHaveBeenCalled();
});
it('cancels Deck reservation behind a modal without sending an interrupt', async () => {
  await mount();
  const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; root.appendChild(overlay);
  act(() => { registerOpenModal('terminal-test-modal'); }); await trigger('deck');
  await waitFor(() => expect(state.cancel).toHaveBeenCalledWith('ticket-a'));
  expect(state.prepare).not.toHaveBeenCalled(); expect(state.commit).not.toHaveBeenCalled();
});
it('cancels Deck terminal close behind a modal without preparation or commit', async () => {
  await mount();
  const overlay = document.createElement('div'); overlay.className = 'modal-overlay'; root.appendChild(overlay);
  act(() => { registerOpenModal('terminal-test-modal'); });
  const admitted = { ...reservation, commandId: TERMINAL_SESSION_CLOSE_COMMAND };
  act(() => { state.events.get('command:deck-ui-reservation')?.(admitted); });
  await waitFor(() => expect(state.cancel).toHaveBeenCalledWith('ticket-a'));
  expect(state.prepareSession).not.toHaveBeenCalled(); expect(state.commit).not.toHaveBeenCalled();
});
it('does not retry or report success when the commit response is lost', async () => {
  await mount(); state.commit.mockRejectedValueOnce(new Error('response lost')); await trigger('button');
  await waitFor(() => expect(state.announce).toHaveBeenCalledWith('commandPalette.executionUnknown'));
  expect(state.commit).toHaveBeenCalledOnce(); expect(state.complete).not.toHaveBeenCalled(); expect(state.cancel).not.toHaveBeenCalled();
});
it('announces a backend preparation refusal without a commit or success', async () => {
  await mount(); state.prepare.mockRejectedValueOnce(new Error('target mismatch')); await trigger('button');
  await waitFor(() => expect(state.announce).toHaveBeenCalledWith('commandPalette.executionFailed'));
  expect(state.cancel).toHaveBeenCalledWith('ticket-a'); expect(state.commit).not.toHaveBeenCalled();
});
it('ignores a duplicate Deck delivery without cancelling its in-flight reservation', async () => {
  await mount(); let release!: () => void;
  state.prepare.mockImplementationOnce(() => new Promise<void>(done => { release = done; }));
  await trigger('deck'); await waitFor(() => expect(state.prepare).toHaveBeenCalledOnce());
  await trigger('deck');
  expect(state.cancel).not.toHaveBeenCalled(); expect(state.prepare).toHaveBeenCalledOnce();
  act(() => release());
  await waitFor(() => expect(state.result).toHaveBeenCalledOnce());
  expect(state.commit).toHaveBeenCalledOnce(); expect(state.cancel).not.toHaveBeenCalled();
});
