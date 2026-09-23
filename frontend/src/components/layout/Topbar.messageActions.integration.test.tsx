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
import {
  captureChatMessagingTarget,
  registerChatMessagingSurface,
  requestChatMessagingCommand,
  type ChatMessagingHandoff,
} from '../../lib/commandChatMessaging';

const state = vi.hoisted(() => ({
  pathname: '/', mapReady: false, commandID: 'chat.message.copy',
  begin: vi.fn(), beginKey: vi.fn(), take: vi.fn(), complete: vi.fn(), cancel: vi.fn(),
  getResult: vi.fn(), commit: vi.fn(), execute: vi.fn(), prepareAdmission: vi.fn(),
  announce: vi.fn(), navigate: vi.fn(),
  events: new Map<string, (payload: unknown) => void>(),
  menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' },
  noop: () => undefined, t: (key: string) => key,
}));
const commandIDs = [
  'chat.message.copy', 'chat.message.copy_markdown', 'chat.message.speak',
  'chat.message.edit.open', 'chat.message.pin.toggle', 'chat.message.delete',
] as const;
const auditedIDs = commandIDs.filter(id => id !== 'chat.message.edit.open');
type MessageCommand = typeof commandIDs[number];
type Source = 'button' | 'keyboard' | 'palette' | 'deck';
const sources: Source[] = ['button', 'keyboard', 'palette', 'deck'];
const isUI = (id: string) => id !== 'chat.message.pin.toggle' && id !== 'chat.message.delete';
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
      localPaletteCommands: ['chat.message.edit.open'],
      bindings: [{ shortcut: { version: 1 as const, code: 'KeyJ', modifiers: ['Control' as const] }, commandId: state.commandID, handler: state.commandID === 'chat.message.edit.open' ? 'local_ui' as const : isUI(state.commandID) ? 'ui' as const : 'contextual' as const }],
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
let message: HTMLButtonElement;
let unregister: (() => void) | undefined;
let selectedMessage: string | undefined;
const listeners = new Set<() => void>();
const order: string[] = [];
function select(id: string | undefined) {
  selectedMessage = id;
  listeners.forEach(changed => changed());
}
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(done => { resolve = done; });
  return { promise, resolve };
}
const reservation = (commandId: string) => ({ ticket: `ticket-${commandId}`, invocationId: 'invocation-1', commandId });

beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  state.mapReady = false; state.pathname = '/'; order.length = 0; selectedMessage = 'message-a';
  state.begin.mockImplementation(async (id: string) => { order.push('begin'); return reservation(id); });
  state.beginKey.mockImplementation(async () => { order.push('begin-key'); return reservation(state.commandID); });
  state.take.mockImplementation(async (ticket: string) => { order.push('take'); return { ...reservation(ticket.slice(7)), handoffId: 'handoff-1' }; });
  state.complete.mockImplementation(async () => { order.push('complete'); });
  state.cancel.mockResolvedValue(undefined);
  state.getResult.mockImplementation(async () => { order.push('result'); return { invocationId: 'invocation-1', status: 'succeeded' }; });
  state.prepareAdmission.mockImplementation(async () => { order.push('prepare'); });
  state.execute.mockImplementation(async () => { order.push('execute'); });
});
afterEach(() => {
  cleanup(); unregister?.(); unregister = undefined; root?.remove(); listeners.clear(); state.events.clear();
  vi.restoreAllMocks(); vi.clearAllMocks();
});

async function mount(commandID: MessageCommand) {
  state.commandID = commandID;
  render(<Topbar />);
  await waitFor(() => expect(state.mapReady).toBe(true));
  await act(async () => { await new Promise<void>(resolve => setTimeout(resolve, 50)); });
  root = document.createElement('div'); message = document.createElement('button');
  message.textContent = 'Selected message'; root.appendChild(message); document.body.appendChild(root);
  unregister = registerChatMessagingSurface({
    root, instanceId: 'chat-actions', isCurrent: () => true,
    canStart: (id, keyboardTarget) => commandIDs.some(candidate => candidate === id) && Boolean(selectedMessage) &&
      (!keyboardTarget || keyboardTarget === message),
    subscribe(changed) { listeners.add(changed); return () => { listeners.delete(changed); }; },
    prepare(id) {
      const captured = selectedMessage;
      if (!captured) return undefined;
      return {
        executionKind: isUI(id) ? 'ui' as const : 'backend' as const,
        prepareAdmission: (ticket: string) => state.prepareAdmission(ticket, captured, id),
        isCurrent: () => selectedMessage === captured,
        canCommit: () => selectedMessage === captured,
        execute: (handoff: ChatMessagingHandoff) => state.execute(handoff, captured, id),
        dispose: state.noop,
      };
    },
  });
  message.focus();
}
async function openPalette(id: MessageCommand) {
  const user = userEvent.setup();
  await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
  await user.type(await screen.findByRole('combobox'), id);
  return user;
}
async function trigger(source: Source, id: MessageCommand) {
  if (source === 'button') act(() => { requestChatMessagingCommand(id, 'chat-actions'); });
  if (source === 'keyboard') fireEvent.keyDown(message, { key: 'j', code: 'KeyJ', ctrlKey: true });
  if (source === 'deck') act(() => {
    if (id === 'chat.message.edit.open') state.events.get('command:deck-local-ui')?.({
      commandId: id, generation: 'generation-1', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
    });
    else state.events.get('command:deck-ui-reservation')?.(reservation(id));
  });
  if (source === 'palette') { const user = await openPalette(id); await user.click(await screen.findByRole('option', { name: id })); }
}

describe('Topbar audited message actions: real registry and executor', () => {
  it.each(sources)('edit.open via %s is local presentation without a reservation or ledger', async source => {
    const id = 'chat.message.edit.open'; await mount(id);
    await trigger(source, id);
    await waitFor(() => expect(state.execute).toHaveBeenCalledTimes(1));
    expect(state.execute.mock.calls[0].slice(1)).toEqual(['message-a', id]);
    expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled();
    expect(state.prepareAdmission).not.toHaveBeenCalled(); expect(state.take).not.toHaveBeenCalled();
    expect(state.complete).not.toHaveBeenCalled(); expect(state.getResult).not.toHaveBeenCalled();
    expect(state.cancel).not.toHaveBeenCalled(); expect(state.commit).not.toHaveBeenCalled();
  });

  it.each(auditedIDs.flatMap(id => sources.map(source => ({ id, source }))))(
    '$id via $source prepares before Take, executes the captured message once and completes by kind', async ({ id, source }) => {
      await mount(id);
      const preparation = deferred<void>();
      state.prepareAdmission.mockImplementationOnce(async () => { order.push('prepare'); return preparation.promise; });
      await trigger(source, id);
      await waitFor(() => expect(state.prepareAdmission).toHaveBeenCalledExactlyOnceWith(`ticket-${id}`, 'message-a', id));
      expect(state.take).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
      expect(state.complete).not.toHaveBeenCalled(); expect(state.getResult).not.toHaveBeenCalled();
      await act(async () => preparation.resolve());
      await waitFor(() => expect(state.getResult).toHaveBeenCalledTimes(1));
      expect(state.execute).toHaveBeenCalledExactlyOnceWith({ ticket: `ticket-${id}`, handoffId: 'handoff-1' }, 'message-a', id);
      expect(state.take).toHaveBeenCalledExactlyOnceWith(`ticket-${id}`);
      expect(order.indexOf('prepare')).toBeLessThan(order.indexOf('take'));
      expect(order.indexOf('take')).toBeLessThan(order.indexOf('execute'));
      if (isUI(id)) {
        expect(state.complete).toHaveBeenCalledExactlyOnceWith(`ticket-${id}`, 'handoff-1', 'succeeded');
        expect(order.indexOf('execute')).toBeLessThan(order.indexOf('complete'));
      } else expect(state.complete).not.toHaveBeenCalled();
      // Pin/delete are submitted by the captured action, not generic workspace Commit.
      expect(state.commit).not.toHaveBeenCalled();
      if (source === 'keyboard') {
        expect(state.beginKey).toHaveBeenCalledExactlyOnceWith('generation-1', { version: 1, code: 'KeyJ', modifiers: ['Control'] }, false);
        expect(state.begin).not.toHaveBeenCalled();
      } else if (source === 'deck') {
        expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled();
      } else {
        expect(state.begin).toHaveBeenCalledExactlyOnceWith(id); expect(state.beginKey).not.toHaveBeenCalled();
      }
    },
  );

  it.each(commandIDs)('%s rejects palette selection ABA without recapturing', async id => {
    await mount(id);
    const user = await openPalette(id);
    select('message-b'); select('message-a');
    await user.click(await screen.findByRole('option', { name: id }));
    await act(async () => { await Promise.resolve(); });
    expect(state.begin).not.toHaveBeenCalled(); expect(state.prepareAdmission).not.toHaveBeenCalled();
    expect(state.take).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });

  it.each(auditedIDs)('%s cancels after selection ABA during admission preparation', async id => {
    await mount(id);
    const preparation = deferred<void>(); state.prepareAdmission.mockReturnValueOnce(preparation.promise);
    await trigger('button', id);
    await waitFor(() => expect(state.prepareAdmission).toHaveBeenCalledTimes(1));
    select('message-b'); select('message-a');
    await act(async () => preparation.resolve());
    await waitFor(() => expect(state.cancel).toHaveBeenCalledWith(`ticket-${id}`));
    expect(state.take).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });

  it.each(sources)('%s refuses commands without a contextual selected message', async source => {
    const id = 'chat.message.delete'; await mount(id); select(undefined);
    if (source === 'palette') {
      await openPalette(id);
      expect(screen.queryByRole('option', { name: id })).not.toBeInTheDocument();
    } else await trigger(source, id);
    await act(async () => { await Promise.resolve(); });
    expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled();
    expect(state.prepareAdmission).not.toHaveBeenCalled(); expect(state.take).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
    if (source === 'deck') expect(state.cancel).toHaveBeenCalledWith(`ticket-${id}`);
  });

  it('rejects a captured button target after replacement instead of using the newly selected message', async () => {
    const id = 'chat.message.pin.toggle'; await mount(id);
    const target = captureChatMessagingTarget(() => '/', id, 'chat-actions');
    expect(target).toBeDefined(); select('message-b');
    expect(requestChatMessagingCommand(id, 'chat-actions', target)).toBe(false);
    expect(state.begin).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });

  it.each(['failed', 'denied', 'outcome_unknown'])('reports backend %s without duplicate execution or UI success completion', async status => {
    const id = 'chat.message.delete'; await mount(id);
    state.getResult.mockResolvedValueOnce({ invocationId: 'invocation-1', status });
    await trigger('button', id);
    await waitFor(() => expect(state.announce).toHaveBeenCalledWith(status === 'outcome_unknown' ? 'commandPalette.executionUnknown' : 'commandPalette.executionFailed'));
    expect(state.execute).toHaveBeenCalledTimes(1); expect(state.begin).toHaveBeenCalledTimes(1);
    expect(state.complete).not.toHaveBeenCalled();
  });

  it('admission failure cancels the reservation without Take or effects', async () => {
    const id = 'chat.message.copy'; await mount(id);
    state.prepareAdmission.mockRejectedValueOnce(new Error('prepare denied'));
    await trigger('button', id);
    await waitFor(() => expect(state.cancel).toHaveBeenCalledWith(`ticket-${id}`));
    expect(state.take).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled(); expect(state.complete).not.toHaveBeenCalled();
  });

  it('a failed UI effect completes failed once, never succeeded or replayed', async () => {
    const id = 'chat.message.copy'; await mount(id);
    state.execute.mockRejectedValueOnce(new Error('clipboard denied'));
    state.getResult.mockResolvedValueOnce({ invocationId: 'invocation-1', status: 'failed' });
    await trigger('button', id);
    await waitFor(() => expect(state.announce).toHaveBeenCalledWith('commandPalette.executionFailed'));
    expect(state.execute).toHaveBeenCalledTimes(1);
    expect(state.complete).toHaveBeenCalledExactlyOnceWith(`ticket-${id}`, 'handoff-1', 'failed');
    expect(state.getResult).toHaveBeenCalledExactlyOnceWith(`ticket-${id}`);
  });

  it('duplicate requests while preparing do not reserve or execute twice', async () => {
    const id = 'chat.message.pin.toggle'; await mount(id);
    const preparation = deferred<void>(); state.prepareAdmission.mockReturnValueOnce(preparation.promise);
    await trigger('button', id);
    await waitFor(() => expect(state.prepareAdmission).toHaveBeenCalledTimes(1));
    await trigger('button', id);
    expect(state.begin).toHaveBeenCalledTimes(1); expect(state.execute).not.toHaveBeenCalled();
    await act(async () => preparation.resolve());
    await waitFor(() => expect(state.getResult).toHaveBeenCalledTimes(1));
    expect(state.take).toHaveBeenCalledTimes(1); expect(state.execute).toHaveBeenCalledTimes(1);
  });

  it('repeat, IME and unrelated keyboard targets never begin admission', async () => {
    await mount('chat.message.copy');
    fireEvent.keyDown(message, { key: 'j', code: 'KeyJ', ctrlKey: true, repeat: true });
    fireEvent.keyDown(message, { key: 'j', code: 'KeyJ', ctrlKey: true, isComposing: true });
    fireEvent.keyDown(document.body, { key: 'j', code: 'KeyJ', ctrlKey: true });
    await act(async () => { await Promise.resolve(); });
    expect(state.beginKey).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });
});
