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
  pathname: '/', mapReady: false, commandID: 'chat.message.edit.save',
  begin: vi.fn(), beginKey: vi.fn(), take: vi.fn(), complete: vi.fn(), cancel: vi.fn(),
  getResult: vi.fn(), commit: vi.fn(), execute: vi.fn(), prepareAdmission: vi.fn(),
  announce: vi.fn(), navigate: vi.fn(),
  events: new Map<string, (payload: unknown) => void>(),
  menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' },
  noop: () => undefined, t: (key: string) => key,
}));
const commandID = 'chat.message.edit.save';
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
const originalContent = 'Original ao abrir edição';
const newContent = 'Rascunho editado';
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
function changeDraft(content: string) { draft = content; revision++; listeners.forEach(changed => changed()); }
function changeMessage(id: string | undefined) { selectedMessage = id; revision++; listeners.forEach(changed => changed()); }
function setEditing(value: boolean) { editing = value; revision++; listeners.forEach(changed => changed()); }
function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>(done => { resolve = done; });
  pending.push(() => resolve());
  return { promise, resolve };
}
const reservation = (id = commandID) => ({ ticket: `ticket-${id}`, invocationId: 'invocation-1', commandId: id });

beforeEach(() => {
  localStorage.removeItem('assistente.command-palette.v1.user-a.workspace-a');
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  state.mapReady = false; state.pathname = '/'; state.commandID = commandID;
  order.length = 0; selectedMessage = messageID; draft = newContent; revision = 0; editing = true;
  state.begin.mockImplementation(async (id: string) => { order.push('begin'); return reservation(id); });
  state.beginKey.mockImplementation(async () => { order.push('begin-key'); return reservation(); });
  state.take.mockImplementation(async () => { order.push('take'); return { ...reservation(), handoffId: 'handoff-1' }; });
  state.complete.mockResolvedValue(undefined); state.cancel.mockResolvedValue(undefined);
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
  root.className = 'message-node'; root.setAttribute('data-message-node', ''); root.setAttribute('data-message-id', messageID);
  textarea.setAttribute('aria-label', 'Rascunho da mensagem'); textarea.value = draft;
  saveButton = document.createElement('button'); saveButton.textContent = 'Salvar edição';
  root.append(textarea, saveButton); document.body.appendChild(root);
  unregister = registerChatMessagingSurface({
    root, instanceId: 'message-edit', isCurrent: () => true,
    canStart: (id, keyboardTarget) => id === commandID && editing && Boolean(selectedMessage) && Boolean(draft.trim()) &&
      (!keyboardTarget || root.contains(keyboardTarget as Node)),
    subscribe(changed) { listeners.add(changed); return () => { listeners.delete(changed); }; },
    prepare() {
      if (!editing || !selectedMessage || !draft.trim()) return undefined;
      const captured = { messageId: selectedMessage, originalContent, content: draft, revision, origin: { ...origin } };
      const isCurrent = () => editing && selectedMessage === captured.messageId && revision === captured.revision;
      return {
        executionKind: 'backend', isCurrent, canCommit: isCurrent,
        prepareAdmission: (ticket: string) => state.prepareAdmission(ticket, captured.messageId, captured.originalContent, captured.content),
        execute: (handoff: ChatMessagingHandoff) => state.execute(handoff, captured), dispose: state.noop,
      };
    },
  });
  saveButton.addEventListener('click', () => {
    const target = captureChatMessagingTarget(() => state.pathname, commandID, 'message-edit');
    if (target) requestChatMessagingCommand(commandID, 'message-edit', target);
  });
  textarea.focus();
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
  if (source === 'keyboard') fireEvent.keyDown(textarea, { key: 'j', code: 'KeyJ', ctrlKey: true });
  if (source === 'deck') act(() => state.events.get('command:deck-ui-reservation')?.(reservation()));
  if (source === 'palette') { const user = await openPalette(); await user.click(await screen.findByRole('option', { name: `${commandID}. Ctrl+J` })); }
}

describe('Topbar chat.message.edit.save: real registry and audited executor', () => {
  it.each(sources)('%s captures original content, draft and origin before admission; prepares before Take and commits once', async source => {
    await mount(); const prepared = deferred();
    state.prepareAdmission.mockImplementationOnce(async () => { order.push('prepare'); return prepared.promise; });
    await trigger(source);
    await waitFor(() => expect(state.prepareAdmission).toHaveBeenCalledExactlyOnceWith(`ticket-${commandID}`, messageID, originalContent, newContent));
    expect(state.take).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled(); expect(state.getResult).not.toHaveBeenCalled();
    await act(async () => prepared.resolve());
    await waitFor(() => expect(state.getResult).toHaveBeenCalledTimes(1));
    expect(state.execute).toHaveBeenCalledExactlyOnceWith({ ticket: `ticket-${commandID}`, handoffId: 'handoff-1' }, {
      messageId: messageID, originalContent, content: newContent, revision: 0, origin,
    });
    expect(state.take).toHaveBeenCalledExactlyOnceWith(`ticket-${commandID}`);
    expect(order.indexOf('prepare')).toBeLessThan(order.indexOf('take'));
    expect(order.indexOf('take')).toBeLessThan(order.indexOf('execute'));
    expect(state.complete).not.toHaveBeenCalled(); expect(state.commit).not.toHaveBeenCalled();
    if (source === 'keyboard') {
      expect(state.beginKey).toHaveBeenCalledExactlyOnceWith('generation-1', { version: 1, code: 'KeyJ', modifiers: ['Control'] }, false);
      expect(state.begin).not.toHaveBeenCalled();
    } else if (source === 'deck') {
      expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled();
    } else {
      expect(state.begin).toHaveBeenCalledExactlyOnceWith(commandID); expect(state.beginKey).not.toHaveBeenCalled();
    }
  });

  it.each(['draft', 'message', 'editor'] as const)('palette refuses %s ABA rather than recapturing on selection', async kind => {
    await mount(); const user = await openPalette();
    if (kind === 'draft') { changeDraft('outro texto'); changeDraft(newContent); }
    if (kind === 'message') { changeMessage('message-b'); changeMessage(messageID); }
    if (kind === 'editor') { setEditing(false); setEditing(true); }
    await user.click(await screen.findByRole('option', { name: `${commandID}. Ctrl+J` }));
    await act(async () => { await Promise.resolve(); });
    expect(state.begin).not.toHaveBeenCalled(); expect(state.prepareAdmission).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });

  it.each(['begin', 'prepare', 'take'] as const)('draft changed during %s cancels the original lease without saving a replacement', async stage => {
    await mount(); const blocked = deferred();
    if (stage === 'begin') state.begin.mockImplementationOnce(async () => { await blocked.promise; return reservation(); });
    if (stage === 'prepare') state.prepareAdmission.mockReturnValueOnce(blocked.promise);
    if (stage === 'take') state.take.mockImplementationOnce(async () => { await blocked.promise; return { ...reservation(), handoffId: 'handoff-1' }; });
    await trigger('button');
    await waitFor(() => expect(stage === 'begin' ? state.begin : stage === 'prepare' ? state.prepareAdmission : state.take).toHaveBeenCalledTimes(1));
    changeDraft('Novo rascunho após capturar');
    await act(async () => blocked.resolve());
    if (stage === 'take') await waitFor(() => expect(state.complete).toHaveBeenCalledWith(`ticket-${commandID}`, 'handoff-1', 'cancelled'));
    else await waitFor(() => expect(state.cancel).toHaveBeenCalledWith(`ticket-${commandID}`));
    expect(state.execute).not.toHaveBeenCalled(); expect(draft).toBe('Novo rascunho após capturar');
  });

  it.each(sources)('%s refuses missing edit target without starting a new reservation', async source => {
    await mount(); setEditing(false);
    if (source === 'palette') {
      const user = await openPalette();
      const unavailable = await screen.findByRole('option', { name: `${commandID}. Ctrl+J. commandPalette.unavailable` });
      expect(unavailable).toHaveAttribute('aria-disabled', 'true');
      await user.click(unavailable);
    }
    else await trigger(source);
    await act(async () => { await Promise.resolve(); });
    expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled();
    expect(state.prepareAdmission).not.toHaveBeenCalled(); expect(state.take).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
    if (source === 'deck') expect(state.cancel).toHaveBeenCalledWith(`ticket-${commandID}`);
  });

  it.each(['failed', 'denied', 'outcome_unknown'])('%s preserves the draft and never retries or reports UI success', async status => {
    await mount(); state.getResult.mockResolvedValueOnce({ invocationId: 'invocation-1', status });
    await trigger('button');
    await waitFor(() => expect(state.announce).toHaveBeenCalledWith(status === 'outcome_unknown' ? 'commandPalette.executionUnknown' : 'commandPalette.executionFailed'));
    expect(state.begin).toHaveBeenCalledTimes(1); expect(state.prepareAdmission).toHaveBeenCalledTimes(1);
    expect(state.execute).toHaveBeenCalledTimes(1); expect(state.getResult).toHaveBeenCalledTimes(1);
    expect(state.complete).not.toHaveBeenCalled(); expect(draft).toBe(newContent);
  });

  it('lost commit acknowledgement consults the ledger once without replaying save', async () => {
    await mount(); state.execute.mockRejectedValueOnce(new Error('connection lost'));
    state.getResult.mockResolvedValueOnce({ invocationId: 'invocation-1', status: 'outcome_unknown' });
    await trigger('button');
    await waitFor(() => expect(state.announce).toHaveBeenCalledWith('commandPalette.executionUnknown'));
    expect(state.execute).toHaveBeenCalledTimes(1); expect(state.getResult).toHaveBeenCalledTimes(1);
    expect(state.begin).toHaveBeenCalledTimes(1); expect(state.complete).not.toHaveBeenCalled();
  });

  it('original-content conflict in preparation cannot Take or Commit', async () => {
    await mount(); state.prepareAdmission.mockRejectedValueOnce(new Error('message changed since edit opened'));
    await trigger('button');
    await waitFor(() => expect(state.cancel).toHaveBeenCalledWith(`ticket-${commandID}`));
    expect(state.prepareAdmission).toHaveBeenCalledExactlyOnceWith(`ticket-${commandID}`, messageID, originalContent, newContent);
    expect(state.take).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled(); expect(draft).toBe(newContent);
  });

  it('an explicit stale button target cannot retarget the currently selected message', async () => {
    await mount(); const target = captureChatMessagingTarget(() => '/', commandID, 'message-edit');
    expect(target).toBeDefined(); changeMessage('message-b');
    expect(requestChatMessagingCommand(commandID, 'message-edit', target)).toBe(false);
    expect(state.begin).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });

  it('unmount during Take prevents saving and completes only cancellation', async () => {
    const view = await mount(); const taken = deferred();
    state.take.mockImplementationOnce(async () => { await taken.promise; return { ...reservation(), handoffId: 'handoff-1' }; });
    await trigger('button'); await waitFor(() => expect(state.take).toHaveBeenCalledTimes(1));
    view.unmount(); await act(async () => taken.resolve());
    await waitFor(() => expect(state.complete).toHaveBeenCalledWith(`ticket-${commandID}`, 'handoff-1', 'cancelled'));
    expect(state.execute).not.toHaveBeenCalled();
  });

  it('repeat, IME, consumed keys and unrelated textareas do not save', async () => {
    await mount();
    fireEvent.keyDown(textarea, { key: 'j', code: 'KeyJ', ctrlKey: true, repeat: true });
    fireEvent.keyDown(textarea, { key: 'j', code: 'KeyJ', ctrlKey: true, isComposing: true });
    const consumed = new KeyboardEvent('keydown', { key: 'j', code: 'KeyJ', ctrlKey: true, cancelable: true, bubbles: true });
    consumed.preventDefault(); fireEvent(textarea, consumed);
    const outside = document.createElement('textarea'); document.body.appendChild(outside);
    fireEvent.keyDown(outside, { key: 'j', code: 'KeyJ', ctrlKey: true }); outside.remove();
    await act(async () => { await Promise.resolve(); });
    expect(state.beginKey).not.toHaveBeenCalled(); expect(state.execute).not.toHaveBeenCalled();
  });

  it('edit.open remains local with no ledger while save uses audited admission', async () => {
    await mount(); unregister?.();
    unregister = registerChatMessagingSurface({
      root, instanceId: 'message-open', isCurrent: () => true, canStart: id => id === 'chat.message.edit.open',
      subscribe: () => () => {}, prepare: () => ({ isCurrent: () => true, canCommit: () => true, execute: state.execute, dispose: state.noop }),
    });
    expect(requestChatMessagingCommand('chat.message.edit.open', 'message-open')).toBe(true);
    await waitFor(() => expect(state.execute).toHaveBeenCalledTimes(1));
    expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled();
    expect(state.prepareAdmission).not.toHaveBeenCalled(); expect(state.take).not.toHaveBeenCalled();
    expect(state.complete).not.toHaveBeenCalled(); expect(state.getResult).not.toHaveBeenCalled();
  });
});
