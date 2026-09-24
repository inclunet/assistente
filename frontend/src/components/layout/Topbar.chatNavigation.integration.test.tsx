import { useEffect, useRef } from 'react';
import { Modal, useModalId } from '../ui/Modal';
import { registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';
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
import { listCommandCatalog } from '../../services/commandCatalog';

const state = vi.hoisted(() => ({
  pathname: '/', mapReady: false, commandID: 'navigation.landmark.next',
  begin: vi.fn(), beginKey: vi.fn(), take: vi.fn(), complete: vi.fn(), cancel: vi.fn(),
  getResult: vi.fn(), commit: vi.fn(), execute: vi.fn(), prepareAdmission: vi.fn(),
  announce: vi.fn(), navigate: vi.fn(),
  events: new Map<string, (payload: unknown) => void>(),
  menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' },
  noop: () => undefined, t: (key: string) => key,
}));
const commandIDs = CHAT_NAVIGATION_COMMAND_IDS;
import { CHAT_NAVIGATION_COMMAND_IDS, registerChatNavigationSurface, requestChatNavigationCommand } from '../../lib/commandChatNavigation';

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
  useActiveTab: () => workspace.tabs[0], useWorkspaceTabs: () => workspace.tabs,
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
  useWorkspaceChatModalStore: Object.assign((selector?: (value: typeof modalState) => unknown) => selector ? selector(modalState) : modalState, { getState: () => modalState, subscribe: () => () => undefined }),
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
vi.mock('../../hooks/useAnnouncer', () => ({ announce: state.announce, useAnnouncer: () => ({ announce: state.announce, announceRequest: () => true }) }));

vi.mock('../../lib/commandContextReact', () => ({ useCommandContextScope: () => null }));
vi.mock('../../services/commandCatalog', () => ({ listCommandCatalog: vi.fn(async () => commandIDs.map(id => ({
  id, name: id, available: true, description: '', category: 'chat', aliases: [], icon: '',
  effect: 'ui', risk: 'none', decision: 'allow', availabilityStatus: 'available', availabilityReason: '',
  allowedSources: ['keyboard.local', 'palette', 'streamdeck.key', 'ui.action'], scopes: ['chat'], presentationVersion: '1',
}))) }));
vi.mock('../../lib/commandBackendExecutionWails', () => ({ createCommandBackendExecutionWailsPort: () => ({ executeCommand: vi.fn() }) }));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({ createCommandLocalKeyboardWailsPort: () => ({
  loadMap: async () => {
    state.mapReady = true;
    return {
      generation: 'generation-1', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      localPaletteCommands: [...commandIDs],
      bindings: [
        ...commandIDs.map((commandId, index) => ({ shortcut: { version: 1, code: 'Digit' + (index + 1), modifiers: ['Control'] }, commandId, handler: 'local_ui' })),
        { shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] }, commandId: 'navigation.palette.open', handler: 'local_ui' },
      ],
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


const modalState = { isOpen: true, boundTabId: 'chat-tab', boundConversationId: 'conversation-a' };
const calls: string[] = [];
const listeners = new Set<() => void>();
function Surface({ name = 'a', current = true }: { name?: string; current?: boolean }) {
  const root = useRef<HTMLDivElement>(null);
  const modalId = useModalId();
  const live = useRef(current); live.current = current;
  useEffect(() => {
    const element = root.current!;
    const message = { id: name, content: name };
    return registerChatNavigationSurface({
      root: element, instanceId: name, allowedCommands: commandIDs,
      readContext: () => ({ pathname: '/', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
        tabId: 'chat-tab', conversationId: 'conversation-a', chatSessionKey: 'surface-a',
        modalId: modalId ?? undefined, messageId: name, message }),
      isCurrent: () => live.current,
      subscribe: changed => { listeners.add(changed); return () => { listeners.delete(changed); }; },
      canOpen: () => live.current,
      open: id => { calls.push(name + ':' + id); return true; },
    });
  }, [name, modalId]);
  return <div className="message-node" ref={root} tabIndex={0} aria-label={'Message ' + name}>
    <button onClick={() => requestChatNavigationCommand('chat.message.menu.open', name)}>Menu {name}</button>
  </div>;
}
beforeEach(() => {
  localStorage.removeItem('assistente.command-palette.v1.user-a.workspace-a');
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  state.mapReady = false; state.pathname = '/'; auth.user.sessionId = 'session-a'; calls.length = 0;
});
afterEach(() => { cleanup(); listeners.clear(); state.events.clear(); vi.restoreAllMocks(); vi.clearAllMocks(); });
function noLedger() {
  for (const spy of [state.begin, state.beginKey, state.take, state.complete, state.cancel, state.getResult, state.commit]) expect(spy).not.toHaveBeenCalled();
}
async function mount(extra?: React.ReactNode) {
  const view = render(<><Topbar /><Surface />{extra}</>);
  await waitFor(() => expect(state.mapReady).toBe(true));
  await act(async () => { await new Promise<void>(resolve => setTimeout(resolve, 50)); });
  act(() => screen.getByLabelText('Message a').focus());
  return view;
}
function deck(id: string) {
  act(() => state.events.get('command:deck-local-ui')?.({ commandId: id, generation: 'generation-1', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' }));
}
async function palette(id: string, keyboard = false) {
  const user = userEvent.setup();
  if (keyboard) {
    fireEvent.keyDown(document.activeElement!, { key: 'k', code: 'KeyK', ctrlKey: true });
    fireEvent.keyUp(document.activeElement!, { key: 'k', code: 'KeyK', ctrlKey: true });
  } else await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
  await user.type(await screen.findByRole('combobox'), id);
  return user;
}
describe('Topbar chat navigation with real registry', () => {
  it('catalog rejection disposes all captured listeners without opening palette', async () => {
    await mount();
    vi.mocked(listCommandCatalog).mockRejectedValueOnce(new Error('catalog unavailable'));
    await userEvent.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await waitFor(() => expect(listeners.size).toBe(0));
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument(); expect(calls).toEqual([]); noLedger();
  });
  it('invalidated opening guard disposes captures after catalog resolves', async () => {
    await mount();
    const catalog = await listCommandCatalog();
    vi.mocked(listCommandCatalog).mockImplementationOnce(async () => {
      state.pathname = '/profiles';
      return catalog;
    });
    await userEvent.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await waitFor(() => expect(listeners.size).toBe(0));
    expect(screen.queryByRole('combobox')).not.toBeInTheDocument(); expect(calls).toEqual([]); noLedger();
  });
  it.each(commandIDs)('configured keyboard %s executes once without ledger', async id => {
    await mount();
    const code = 'Digit' + (commandIDs.indexOf(id) + 1);
    fireEvent.keyDown(document.activeElement!, { code, key: String(commandIDs.indexOf(id) + 1), ctrlKey: true });
    fireEvent.keyUp(document.activeElement!, { code, ctrlKey: true });
    expect(calls).toEqual(['a:' + id]); noLedger();
  });
  it.each(commandIDs)('Deck %s executes through local path', async id => {
    await mount(); deck(id); expect(calls).toEqual(['a:' + id]); noLedger();
  });
  it.each(commandIDs)('mouse palette %s keeps node captured before button focus', async id => {
    await mount(); const user = await palette(id);
    await user.click(await screen.findByRole('option', { name: `${id}. Ctrl+${commandIDs.indexOf(id) + 1}` }));
    await waitFor(() => expect(calls).toEqual(['a:' + id])); noLedger();
  });
  it.each(commandIDs)('CtrlK palette %s keeps node captured before search', async id => {
    await mount(); const user = await palette(id, true);
    await user.click(await screen.findByRole('option', { name: `${id}. Ctrl+${commandIDs.indexOf(id) + 1}` }));
    await waitFor(() => expect(calls).toEqual(['a:' + id])); noLedger();
  });
  it('node gesture requests its exact instance even if another node had focus', async () => {
    await mount(<Surface name="b" />);
    fireEvent.click(screen.getByRole('button', { name: 'Menu b' }));
    expect(calls).toEqual(['b:chat.message.menu.open']); noLedger();
  });
  it('palette does not recapture the latest node', async () => {
    await mount(<Surface name="b" />);
    const user = await palette('chat.message.read.open');
    act(() => screen.getByLabelText('Message b').focus());
    await user.click(await screen.findByRole('option', { name: 'chat.message.read.open. Ctrl+3' }));
    await waitFor(() => expect(calls).toEqual(['a:chat.message.read.open'])); noLedger();
  });
  it('unmounted captured node does not retarget another node', async () => {
    const view = await mount(); const user = await palette('chat.message.read.open');
    view.rerender(<><Topbar /><Surface key="b" name="b" /></>);
    await user.click(await screen.findByRole('option', { name: 'chat.message.read.open. Ctrl+3' }));
    expect(calls).toEqual([]); noLedger();
  });
  it('modal ABA invalidates palette target', async () => {
    await mount(); const user = await palette('chat.message.read.open');
    act(() => { registerOpenModal('decision'); unregisterOpenModal('decision'); });
    await user.click(await screen.findByRole('option', { name: 'chat.message.read.open. Ctrl+3' }));
    expect(calls).toEqual([]); noLedger();
  });
  it('session ABA invalidates permanently after subscription notification', async () => {
    await mount(); const user = await palette('chat.message.read.open');
    act(() => {
      auth.user.sessionId = 'other'; listeners.forEach(fn => fn());
      auth.user.sessionId = 'session-a'; listeners.forEach(fn => fn());
    });
    await user.click(await screen.findByRole('option', { name: 'chat.message.read.open. Ctrl+3' }));
    expect(calls).toEqual([]); noLedger();
  });
  it('route change rejects captured action', async () => {
    await mount(); const user = await palette('chat.message.read.open'); state.pathname = '/profiles';
    await user.click(await screen.findByRole('option', { name: 'chat.message.read.open. Ctrl+3' }));
    expect(calls).toEqual([]); noLedger();
  });
  it.each([{ repeat: true }, { isComposing: true }, { keyCode: 229 }])('invalid keyboard %j does not execute', async flags => {
    await mount(); fireEvent.keyDown(document.activeElement!, { key: '3', code: 'Digit3', ctrlKey: true, ...flags });
    expect(calls).toEqual([]); noLedger();
  });
  it('consumed keyboard does not execute', async () => {
    await mount(); const event = new KeyboardEvent('keydown', { key: '3', code: 'Digit3', ctrlKey: true, bubbles: true, cancelable: true });
    event.preventDefault(); fireEvent(document.activeElement!, event);
    expect(calls).toEqual([]); noLedger();
  });
  it('no focused node means no fallback to last message', async () => {
    await mount(); act(() => screen.getByLabelText('Message a').blur()); deck('chat.message.read.open');
    expect(calls).toEqual([]); noLedger();
  });
  it('owning workspace modal permits navigation and overlay blocks it', async () => {
    const view = await mount(<Modal isOpen title="Chat modal" onClose={() => {}}><Surface name="modal" /></Modal>);
    act(() => screen.getByLabelText('Message modal').focus()); deck('chat.message.read.open');
    expect(calls).toEqual(['modal:chat.message.read.open']);
    view.rerender(<><Topbar /><Surface /><Modal isOpen title="Chat modal" onClose={() => {}}><Surface name="modal" /></Modal>
      <Modal isOpen title="Decision" onClose={() => {}}><button>Decision action</button></Modal></>);
    act(() => screen.getByRole('button', { name: 'Decision action' }).focus()); deck('chat.message.read.open');
    expect(calls).toHaveLength(1); noLedger();
  });
});
