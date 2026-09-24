import { useMemo, useRef } from 'react';
import { useLandmarkNavigation, type Landmark } from '../../hooks/useLandmarkNavigation';
import { Modal, useModalIsTopmost } from '../ui/Modal';
import { registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';
import { WorkspaceChatModal } from '../workspace/WorkspaceChatModal';
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

const state = vi.hoisted(() => ({
  pathname: '/', mapReady: false, commandID: 'navigation.landmark.next',
  begin: vi.fn(), beginKey: vi.fn(), take: vi.fn(), complete: vi.fn(), cancel: vi.fn(),
  getResult: vi.fn(), commit: vi.fn(), execute: vi.fn(), prepareAdmission: vi.fn(),
  announce: vi.fn(), navigate: vi.fn(),
  events: new Map<string, (payload: unknown) => void>(),
  menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' },
  noop: () => undefined, t: (key: string) => key,
}));
const commandIDs = ['navigation.landmark.default', 'navigation.landmark.next', 'navigation.landmark.previous'] as const;

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
  useWorkspaceChatModalStore: Object.assign((selector?: (value: typeof modalState) => unknown) => selector ? selector(modalState) : modalState, { getState: () => modalState }),
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
vi.mock('../../services/commandCatalog', () => ({ listCommandCatalog: vi.fn(async () => commandIDs.map(id => ({ id, name: id, available: true }))) }));
vi.mock('../../lib/commandBackendExecutionWails', () => ({ createCommandBackendExecutionWailsPort: () => ({ executeCommand: vi.fn() }) }));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({ createCommandLocalKeyboardWailsPort: () => ({
  loadMap: async () => {
    state.mapReady = true;
    return {
      generation: 'generation-1', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      localPaletteCommands: [...commandIDs],
      bindings: [
        { shortcut: { version: 1, code: 'F6', modifiers: [] }, commandId: 'navigation.landmark.next', handler: 'local_ui' },
        { shortcut: { version: 1, code: 'F6', modifiers: ['Shift'] }, commandId: 'navigation.landmark.previous', handler: 'local_ui' },
        { shortcut: { version: 1, code: 'KeyJ', modifiers: ['Control'] }, commandId: 'navigation.landmark.default', handler: 'local_ui' },
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



const modalState = {
  isOpen: true, boundTabId: 'chat-tab', boundConversationId: 'conversation-a',
  boundSurface: { conversationId: 'conversation-a', sessionKey: 'modal-a', surfaceId: 'modal-a', surfaceType: 'modal', tabId: 'chat-tab' },
  contextDisplay: '', sessionMeta: null, boundSend: vi.fn(), focusNonce: 0, adapterError: null, close: vi.fn(), setBoundConversation: vi.fn(),
};
vi.mock('../chat/ChatPanel', () => ({
  useEffectiveProfileSlug: () => undefined,
  ChatPanel: () => <><div className="ws-content-toolbar"><button>Modal toolbar</button></div><div className="message-list"><div className="message-list__list" tabIndex={0} aria-label="Modal messages" /></div><div className="chat-input"><textarea className="chat-input__textarea" aria-label="Modal composer" /></div></>,
}));
vi.mock('../chat/ChatSurfaceController', () => ({
  sendChatSurfaceMessage: vi.fn(), useChatConversationTimeline: () => ({ id: 'conversation-a', title: 'Conversation', threadedMessages: [] }),
}));
function Regions({ prefix = 'page', modal = false, enabled = true }: { prefix?: string; modal?: boolean; enabled?: boolean }) {
  const root = useRef<HTMLDivElement>(null);
  const isTopmost = useModalIsTopmost();
  const landmarks = useMemo<Landmark[]>(() => ['first', 'second', 'third'].map(id => ({
    id, label: id,
    contains: () => root.current?.querySelector('[data-zone="' + id + '"]')?.contains(document.activeElement) === true,
    focus: () => { const element = root.current?.querySelector<HTMLElement>('[data-zone="' + id + '"]'); element?.focus(); return !!element && document.activeElement === element; },
  })), []);
  useLandmarkNavigation({ landmarks, enabled, defaultLandmarkId: 'second', allowWhenModalOpen: modal, shouldHandleKey: modal ? isTopmost : undefined });
  return <div ref={root}>{['first', 'second', 'third'].map(id => <textarea key={id} data-zone={id} aria-label={prefix + '-' + id} onKeyDown={event => {
    if (event.key === 'Escape' && event.currentTarget.value === 'consume') { event.preventDefault(); event.stopPropagation(); }
  }} />)}</div>;
}
beforeEach(() => {
  localStorage.removeItem('assistente.command-palette.v1.user-a.workspace-a');
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  state.mapReady = false; state.pathname = '/';
  modalState.close.mockClear();
});
afterEach(() => { cleanup(); state.events.clear(); vi.restoreAllMocks(); vi.clearAllMocks(); });
function expectNoLedger() {
  expect(state.begin).not.toHaveBeenCalled(); expect(state.beginKey).not.toHaveBeenCalled(); expect(state.take).not.toHaveBeenCalled(); expect(state.complete).not.toHaveBeenCalled(); expect(state.commit).not.toHaveBeenCalled();
  expect(state.cancel).not.toHaveBeenCalled(); expect(state.getResult).not.toHaveBeenCalled();
}
async function mount(extra?: React.ReactNode) {
  const view = render(<><Topbar /><Regions />{extra}</>);
  await waitFor(() => expect(state.mapReady).toBe(true));
  await act(async () => { await new Promise<void>(resolve => setTimeout(resolve, 50)); });
  return view;
}
function focus(name: string) { const element = screen.getByRole('textbox', { name }); act(() => element.focus()); return element; }
function key(code: string, shiftKey = false, extra = {}) {
  fireEvent.keyDown(document.activeElement!, { key: code, code, shiftKey, ...extra });
  fireEvent.keyUp(document.activeElement!, { key: code, code, shiftKey });
}
async function palette(id: string) {
  const user = userEvent.setup(); await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
  await user.type(await screen.findByRole('combobox'), id); return user;
}
function deck(id: string) {
  act(() => state.events.get('command:deck-local-ui')?.({ commandId: id, generation: 'generation-1', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' }));
}
describe('Topbar landmark navigation with real hook and registry', () => {
  it('F6 and Shift+F6 traverse regions once without ledger', async () => {
    await mount(); focus('page-first'); key('F6'); expect(screen.getByLabelText('page-second')).toHaveFocus();
    key('F6', true); expect(screen.getByLabelText('page-first')).toHaveFocus(); expectNoLedger();
  });
  it.each([
    ['navigation.landmark.next', 'page-first', 'page-second'],
    ['navigation.landmark.previous', 'page-third', 'page-second'],
    ['navigation.landmark.default', 'page-third', 'page-second'],
  ])('palette %s preserves origin captured before search gains focus', async (id, start, end) => {
    await mount(); focus(start); const user = await palette(id);
    expect(await screen.findByRole('combobox')).toHaveFocus();
    const shortcut = id === 'navigation.landmark.next' ? 'F6' : id === 'navigation.landmark.previous' ? 'Shift+F6' : 'Ctrl+J';
    await user.click(await screen.findByRole('option', { name: `${id}. ${shortcut}` }));
    await waitFor(() => expect(screen.getByLabelText(end)).toHaveFocus()); expectNoLedger();
  });
  it.each([
    ['navigation.landmark.next', 'page-first', 'page-second'],
    ['navigation.landmark.previous', 'page-third', 'page-second'],
    ['navigation.landmark.default', 'page-third', 'page-second'],
  ])('Deck %s follows the same local path', async (id, start, end) => {
    await mount(); focus(start); deck(id); await waitFor(() => expect(screen.getByLabelText(end)).toHaveFocus()); expectNoLedger();
  });
  it('Escape bubbles to default only when the component did not consume it', async () => {
    await mount(); const first = focus('page-first'); fireEvent.change(first, { target: { value: 'draft' } });
    key('Escape'); expect(screen.getByLabelText('page-second')).toHaveFocus(); expect(first).toHaveValue('draft');
    focus('page-first'); fireEvent.change(first, { target: { value: 'consume' } }); key('Escape');
    expect(first).toHaveFocus(); expectNoLedger();
  });
  it.each([{ repeat: true }, { isComposing: true }, { keyCode: 229 }])('blocks invalid F6 %j', async flags => {
    await mount(); const first = focus('page-first'); key('F6', false, flags); expect(first).toHaveFocus(); expectNoLedger();
  });
  it('already consumed F6 does not navigate', async () => {
    await mount(); const first = focus('page-first');
    const event = new KeyboardEvent('keydown', { key: 'F6', code: 'F6', bubbles: true, cancelable: true }); event.preventDefault(); fireEvent(first, event);
    expect(first).toHaveFocus(); expectNoLedger();
  });
  it('configured default shortcut uses local map and does not change input text', async () => {
    await mount(); const first = focus('page-first'); fireEvent.change(first, { target: { value: 'rascunho' } });
    fireEvent.keyDown(first, { key: 'j', code: 'KeyJ', ctrlKey: true });
    expect(screen.getByLabelText('page-second')).toHaveFocus(); expect(first).toHaveValue('rascunho'); expectNoLedger();
  });
  it.each([{ repeat: true }, { isComposing: true }, { keyCode: 229 }])('Escape %j does not leave the component', async flags => {
    await mount(); const first = focus('page-first'); key('Escape', false, flags);
    expect(first).toHaveFocus(); expectNoLedger();
  });
  it('modal barrier blocks page navigation and palette snapshot survives no ABA', async () => {
    await mount(); focus('page-first'); const user = await palette('navigation.landmark.next');
    act(() => { registerOpenModal('transient-test'); unregisterOpenModal('transient-test'); });
    await user.click(await screen.findByRole('option', { name: 'navigation.landmark.next. F6' }));
    expect(screen.getByLabelText('page-second')).not.toHaveFocus(); expectNoLedger();
  });
  it('topmost modal owns F6 and an unrelated modal blocks both surfaces', async () => {
    const view = await mount(<Modal isOpen title="Region modal" onClose={() => {}}><Regions prefix="modal" modal /></Modal>);
    focus('modal-first'); key('F6'); expect(screen.getByLabelText('modal-second')).toHaveFocus();
    view.rerender(<><Topbar /><Regions /><Modal isOpen title="Region modal" onClose={() => {}}><Regions prefix="modal" modal /></Modal><Modal isOpen title="Other modal" onClose={() => {}}><input aria-label="Other input" /></Modal></>);
    const other = screen.getByLabelText('Other input'); act(() => other.focus()); key('F6');
    expect(other).toHaveFocus(); expectNoLedger();
  });
  it('WorkspaceChatModal topmost uses its real landmark owner instead of page regions', async () => {
    await mount(<WorkspaceChatModal />); const composer = focus('Modal composer');
    key('F6'); expect(screen.getByRole('button', { name: 'Modal toolbar' })).toHaveFocus();
    key('F6', true); expect(composer).toHaveFocus(); expectNoLedger();
  });
  it('unmounting captured regions rejects pending palette navigation', async () => {
    const view = await mount(); focus('page-first'); const user = await palette('navigation.landmark.next');
    view.rerender(<Topbar />);
    await user.click(await screen.findByRole('option', { name: 'navigation.landmark.next. F6' })); expectNoLedger();
  });
});
