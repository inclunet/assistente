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
  pathname: '/', mapReady: false, commandID: 'chat.message.send_to_editor',
  begin: vi.fn(), beginKey: vi.fn(), take: vi.fn(), complete: vi.fn(), cancel: vi.fn(),
  getResult: vi.fn(), commit: vi.fn(), execute: vi.fn(), prepareAdmission: vi.fn(),
  announce: vi.fn(), navigate: vi.fn(),
  events: new Map<string, (payload: unknown) => void>(),
  menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' },
  noop: () => undefined, t: (key: string) => key,
}));
const commandID = 'chat.message.send_to_editor';
const commandIDs = [commandID] as const;
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
      localPaletteCommands: [],
      bindings: [{ shortcut: { version: 1 as const, code: 'KeyJ', modifiers: ['Control' as const] }, commandId: state.commandID, handler: 'contextual' as const }],
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

// This fixture models an open edit form; the production draft source is covered
// by the ChatSessionView/MessageNode suites, not replaced in those suites here.
const messageID = '01926b90-7a5a-7c4e-8d3f-000000000002';
const originalContent = 'Mensagem fonte original';
const newContent = 'Bloco para o editor';
const origin = { conversationId: 'conversation-a', sessionKey: 'page:tab:chat-tab:conversation-a', tabId: 'chat-tab' };
let root: HTMLDivElement;
let textarea: HTMLTextAreaElement;
let saveButton: HTMLButtonElement;
let unregister: (() => void) | undefined;
let selectedMessage: string | undefined;
let draft: string;
let revision: number;
let editing: boolean;
const listeners = new Set<() => void>();
const order: string[] = [];
const pending: Array<() => void> = [];

function changeMessage(id: string | undefined) { selectedMessage = id; revision++; listeners.forEach(changed => changed()); }

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>(done => { resolve = done; });
  pending.push(() => resolve());
  return { promise, resolve };
}
const reservation = (id = commandID) => ({ ticket: `ticket-${id}`, invocationId: 'invocation-1', commandId: id });

beforeEach(() => {
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  state.mapReady = false; state.pathname = '/'; state.commandID = commandID;
  order.length = 0; selectedMessage = messageID; draft = newContent; revision = 0; editing = true;
  state.begin.mockImplementation(async (id: string) => { order.push('begin'); return reservation(id); });
  state.beginKey.mockImplementation(async () => { order.push('begin-key'); return reservation(); });
  state.take.mockImplementation(async () => { order.push('take'); return { ...reservation(), handoffId: 'handoff-1' }; });
  state.complete.mockImplementation(async () => { order.push('complete'); }); state.cancel.mockResolvedValue(undefined);
  state.getResult.mockImplementation(async () => { order.push('result'); return { invocationId: 'invocation-1', status: 'succeeded' }; });
  state.prepareAdmission.mockImplementation(async () => { order.push('prepare'); });
  state.execute.mockImplementation(async () => { order.push('execute'); });
});
afterEach(async () => {
  cleanup(); unregister?.(); unregister = undefined; root?.remove();
  await act(async () => { pending.splice(0).forEach(resolve => resolve()); await Promise.resolve(); });
  listeners.clear(); state.events.clear(); vi.restoreAllMocks(); vi.clearAllMocks();
});

async function mount() {
  const view = render(<Topbar />);
  await waitFor(() => expect(state.mapReady).toBe(true));
  await act(async () => { await new Promise<void>(resolve => setTimeout(resolve, 50)); });
  root = document.createElement('div'); textarea = document.createElement('textarea');
  root.className = 'message-node'; root.tabIndex = 0; root.setAttribute('data-message-node', ''); root.setAttribute('data-message-id', messageID);
  textarea.setAttribute('aria-label', 'Rascunho da mensagem'); textarea.value = draft;
  saveButton = document.createElement('button'); saveButton.textContent = 'Enviar ao editor';
  root.append(textarea, saveButton); document.body.appendChild(root);
  unregister = registerChatMessagingSurface({
    root, instanceId: 'editor-transfer', isCurrent: () => true,
    canStart: (id, keyboardTarget) => id === commandID && editing && Boolean(selectedMessage) && Boolean(draft.trim()) &&
      (!keyboardTarget || root.contains(keyboardTarget as Node)),
    subscribe(changed) { listeners.add(changed); return () => { listeners.delete(changed); }; },
    prepare() {
      if (!editing || !selectedMessage || !draft.trim()) return undefined;
      const captured = { messageId: selectedMessage, originalContent, content: draft, revision, origin: { ...origin } };
      const isCurrent = () => editing && selectedMessage === captured.messageId && revision === captured.revision;
      return {
        executionKind: 'ui', isCurrent, canCommit: isCurrent,
        prepareAdmission: (ticket: string) => state.prepareAdmission(ticket, captured.messageId, captured.originalContent, captured.content),
        execute: (handoff: ChatMessagingHandoff) => state.execute(handoff, captured), dispose: state.noop,
      };
    },
  });
  saveButton.addEventListener('click', () => {
    const target = captureChatMessagingTarget(() => state.pathname, commandID, 'editor-transfer');
    if (target) requestChatMessagingCommand(commandID, 'editor-transfer', target);
  });
  root.focus();
  return view;
}
async function openPalette() {
  const user = userEvent.setup();
  await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
  await user.type(await screen.findByRole('combobox'), commandID);
  return user;
}
async function trigger(source: Source) {
  if (source === 'button') await userEvent.click(saveButton);
  if (source === 'keyboard') fireEvent.keyDown(root, { key: 'j', code: 'KeyJ', ctrlKey: true });
  if (source === 'deck') act(() => state.events.get('command:deck-ui-reservation')?.(reservation()));
  if (source === 'palette') { const user = await openPalette(); await user.click(await screen.findByRole('option', { name: commandID })); }
}


describe('Topbar chat.message.send_to_editor', () => {
  it.each(sources)('%s prepares before Take and completes only after editor acknowledgement', async source => {
    await mount();
    const prepared = deferred(); const acknowledged = deferred();
    state.prepareAdmission.mockImplementationOnce(async () => { order.push('prepare'); await prepared.promise; });
    state.execute.mockImplementationOnce(async () => { order.push('execute'); await acknowledged.promise; });
    await trigger(source);
    await waitFor(() => expect(state.prepareAdmission).toHaveBeenCalledExactlyOnceWith(`ticket-${commandID}`, messageID, originalContent, newContent));
    expect(state.take).not.toHaveBeenCalled();
    await act(async () => prepared.resolve());
    await waitFor(() => expect(state.execute).toHaveBeenCalledTimes(1));
    expect(state.complete).not.toHaveBeenCalled(); expect(state.getResult).not.toHaveBeenCalled();
    expect(state.execute).toHaveBeenCalledWith({ ticket: `ticket-${commandID}`, handoffId: 'handoff-1' }, {
      messageId: messageID, originalContent, content: newContent, revision: 0, origin,
    });
    await act(async () => acknowledged.resolve());
    await waitFor(() => expect(state.complete).toHaveBeenCalledExactlyOnceWith(`ticket-${commandID}`, 'handoff-1', 'succeeded'));
    expect(order).toEqual([...(source === 'deck' ? [] : [source === 'keyboard' ? 'begin-key' : 'begin']), 'prepare', 'take', 'execute', 'complete', 'result']);
    expect(state.commit).not.toHaveBeenCalled(); expect(state.cancel).not.toHaveBeenCalled();
    if (source === 'keyboard') {
      expect(state.beginKey).toHaveBeenCalledExactlyOnceWith('generation-1', { version: 1, code: 'KeyJ', modifiers: ['Control'] }, false);
      expect(state.begin).not.toHaveBeenCalled();
    } else if (source === 'deck') {
      expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled();
    } else expect(state.begin).toHaveBeenCalledExactlyOnceWith(commandID);
  });

  it.each(sources)('%s refuses missing source rather than transferring a latest message', async source => {
    await mount(); changeMessage(undefined);
    if (source === 'palette') {
      await openPalette(); expect(screen.queryByRole('option', { name: commandID })).not.toBeInTheDocument();
    } else await trigger(source);
    await act(async () => { await Promise.resolve(); });
    expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled();
    expect(state.prepareAdmission).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
    if (source === 'deck') expect(state.cancel).toHaveBeenCalledWith(`ticket-${commandID}`);
  });

  it('palette preserves the captured source and rejects selection ABA', async () => {
    await mount(); const user = await openPalette();
    changeMessage('message-b'); changeMessage(messageID);
    await user.click(await screen.findByRole('option', { name: commandID }));
    expect(state.begin).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });

  it.each(['begin', 'prepare', 'take'] as const)('source changed during %s never executes on the replacement', async stage => {
    await mount(); const blocked = deferred();
    if (stage === 'begin') state.begin.mockImplementationOnce(async () => { await blocked.promise; return reservation(); });
    if (stage === 'prepare') state.prepareAdmission.mockReturnValueOnce(blocked.promise);
    if (stage === 'take') state.take.mockImplementationOnce(async () => { await blocked.promise; return { ...reservation(), handoffId: 'handoff-1' }; });
    await trigger('button');
    await waitFor(() => expect(stage === 'begin' ? state.begin : stage === 'prepare' ? state.prepareAdmission : state.take).toHaveBeenCalledTimes(1));
    changeMessage('message-b');
    await act(async () => blocked.resolve());
    await waitFor(() => expect(stage === 'take' ? state.complete : state.cancel).toHaveBeenCalledTimes(1));
    expect(state.execute).not.toHaveBeenCalled();
    expect(state.complete).not.toHaveBeenCalledWith(`ticket-${commandID}`, 'handoff-1', 'succeeded');
  });

  it('stale explicit button target cannot recapture the current message', async () => {
    await mount(); const target = captureChatMessagingTarget(() => '/', commandID, 'editor-transfer');
    expect(target).toBeDefined(); changeMessage('message-b');
    expect(requestChatMessagingCommand(commandID, 'editor-transfer', target)).toBe(false);
    expect(state.begin).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });

  it.each(sources)('%s transfer failure after Take cancels to unknown without Complete failed or replay', async source => {
    await mount(); state.execute.mockRejectedValueOnce(new Error('editor ack lost'));
    state.getResult.mockResolvedValue({ invocationId: 'invocation-1', status: 'outcome_unknown' });
    await trigger(source);
    await waitFor(() => expect(state.cancel).toHaveBeenCalledExactlyOnceWith(`ticket-${commandID}`));
    await waitFor(() => expect(state.getResult).toHaveBeenCalledTimes(1));
    expect(state.complete).not.toHaveBeenCalled();
    expect(state.execute).toHaveBeenCalledTimes(1); expect(state.take).toHaveBeenCalledTimes(1);
    expect(state.prepareAdmission).toHaveBeenCalledTimes(1);
    expect(state.begin.mock.calls.length + state.beginKey.mock.calls.length).toBe(source === 'deck' ? 0 : 1);
  });

  it('preparation rejection never Takes or executes', async () => {
    await mount(); state.prepareAdmission.mockRejectedValueOnce(new Error('source changed'));
    await trigger('button');
    await waitFor(() => expect(state.cancel).toHaveBeenCalledTimes(1));
    expect(state.take).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled(); expect(state.complete).not.toHaveBeenCalled();
  });

  it('duplicate Deck delivery does not execute twice while awaiting acknowledgement', async () => {
    await mount(); const ack = deferred(); state.execute.mockReturnValueOnce(ack.promise);
    await trigger('deck'); await waitFor(() => expect(state.execute).toHaveBeenCalledTimes(1));
    await trigger('deck');
    await act(async () => ack.resolve());
    await waitFor(() => expect(state.complete).toHaveBeenCalledTimes(1));
    expect(state.prepareAdmission).toHaveBeenCalledTimes(1); expect(state.take).toHaveBeenCalledTimes(1);
    expect(state.execute).toHaveBeenCalledTimes(1);
  });

  it('repeat, IME and consumed shortcuts never reserve or execute', async () => {
    await mount();
    fireEvent.keyDown(root, { key: 'j', code: 'KeyJ', ctrlKey: true, repeat: true });
    fireEvent.keyDown(root, { key: 'j', code: 'KeyJ', ctrlKey: true, isComposing: true });
    fireEvent.keyDown(root, { key: 'j', code: 'KeyJ', ctrlKey: true, keyCode: 229 });
    const event = new KeyboardEvent('keydown', { key: 'j', code: 'KeyJ', ctrlKey: true, bubbles: true, cancelable: true });
    event.preventDefault(); fireEvent(root, event);
    fireEvent.keyDown(textarea, { key: 'j', code: 'KeyJ', ctrlKey: true });
    await act(async () => { await Promise.resolve(); });
    expect(state.beginKey).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });
});
