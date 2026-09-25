import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, render, screen, fireEvent, waitFor } from '@testing-library/react';
import { StrictMode, useEffect, useLayoutEffect, useRef } from 'react';
import userEvent from '@testing-library/user-event';
import { Topbar } from './Topbar';

// This fixture has no native hotkeys. The shared ownership bridge is exercised
// independently with real reservation frames in commandGlobalOwnershipWails tests.
vi.mock('../../lib/commandGlobalOwnershipWails', () => ({
  acquireGlobalCommandOwnership: () => ({
    isReady: () => true, owns: () => false, dispose: () => {}, ready: Promise.resolve(),
  }),
}));
import { useShortcutsHelpStore } from '../../store/shortcutsHelpStore';
import { flushWorkspaceNavigation } from '../../store/workspaceStore';
import { registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';
import { registerWorkspacePanelFocus } from '../workspace/workspacePanelFocusRegistry';
import { captureWorkspaceTabTarget } from '../../lib/commandWorkspaceTabTarget';
import { WORKSPACE_TAB_NAVIGATION_COMMAND_IDS, isWorkspaceTabNavigationCommand } from '../../lib/commandWorkspaceTabNavigation';
import { CommandContextProvider, useCommandContextScope } from '../../lib/commandContextReact';
import { WorkspaceTabCreationMenuProvider, useWorkspaceTabCreationMenu, type WorkspaceTabCreationMenuRequest } from '../../lib/workspaceTabCreationMenu';
import { WorkspaceToolbar } from '../workspace/WorkspaceToolbar';
import { useWorkspaceKeyboardShortcuts } from '../../hooks/useWorkspaceKeyboardShortcuts';
import type { SurfaceContextGetter } from '../../lib/commandContextProviders';
import type { BackendCommandExecutionResult } from '../../lib/commandBackendExecution';
import type { MenuItem } from '../menu';
import { COMMAND_NAVIGATION_ROUTES } from '../../lib/commandNavigation';
import { ReadFocusContext } from '../../lib/commandContextProviders';

const keyboardState = vi.hoisted(() => ({
  realExecution: false,
  executions: [] as import('../../lib/commandBackendExecution').CommandBackendExecution[],
  handlers: new Map<string, (payload?: unknown) => void>(),
  loadMap: vi.fn<() => Promise<import('../../lib/commandLocalKeyboard').LocalCommandKeyboardMap>>(async () => ({ generation: 'g1', bindings: [{
    shortcut: { version: 1 as const, code: 'KeyK', modifiers: ['Control' as const] },
    commandId: 'navigation.palette.open', handler: 'local_ui' as const,
  }, {
    shortcut: { version: 1 as const, code: 'F1', modifiers: [] },
    commandId: 'navigation.help.open', handler: 'local_ui' as const,
  }] as Array<{ shortcut: import('../../lib/commandShortcut').CommandKeyboardTrigger; commandId: string; handler: 'backend' | 'ui' | 'contextual' | 'local_ui' }> })),
  dispatch: vi.fn<(...args: [string, import('../../lib/commandShortcut').CommandShortcut, 'down' | 'up', boolean]) => Promise<BackendCommandExecutionResult | null>>(async () => null),
  beginUI: vi.fn(async (): Promise<import('../../lib/commandUIExecution').UICommandBeginResponse | null> => null),
  reset: vi.fn(async (_generation: string) => undefined),
  palette: vi.fn(),
}));
const workspaceChatState = vi.hoisted(() => ({
  canPrepare: vi.fn(() => true),
  prepare: vi.fn(),
  register: vi.fn(),
  getActiveWorkspace: vi.fn(),
  lease: {
    isCurrent: vi.fn(() => true),
    dispose: vi.fn(),
    present: vi.fn(),
  },
}));
const chatPickerState = vi.hoisted(() => ({
  available: false,
  current: true,
  open: vi.fn(() => true),
  captures: [] as Array<{ valid: boolean }>,
}));
vi.mock('../../lib/commandChatPickers', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/commandChatPickers')>();
  return {
    ...actual,
    captureChatPickerTarget: () => {
      if (!chatPickerState.available) return undefined;
      const capture = { valid: true };
      chatPickerState.captures.push(capture);
      return {
        isCurrent: () => capture.valid && chatPickerState.current,
        canOpen: () => capture.valid && chatPickerState.current && chatPickerState.available,
        open: chatPickerState.open,
        dispose: () => { capture.valid = false; },
      };
    },
  };
});
vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (name: string, callback: (payload?: unknown) => void) => {
    keyboardState.handlers.set(name, callback);
    return () => { if (keyboardState.handlers.get(name) === callback) keyboardState.handlers.delete(name); };
  },
}));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({
  GetActiveWorkspace: () => workspaceChatState.getActiveWorkspace(),
}));
vi.mock('../../store/workspaceChatModalStore', () => ({
  canPrepareWorkspaceChatOpen: () => workspaceChatState.canPrepare(),
  prepareWorkspaceChatOpen: () => workspaceChatState.prepare(),
  registerWorkspaceChatCommandDispatcher: () => {
    workspaceChatState.register();
    return () => undefined;
  },
}));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({
  createCommandLocalKeyboardWailsPort: () => ({
    loadMap: async () => ({
      ...(await keyboardState.loadMap()),
      ownerId: 'user-a',
      sessionId: 'session-a',
      workspaceId: 'workspace-a',
      localPaletteCommands: [
        'workspace.tab.next', 'workspace.tab.previous', 'workspace.tab.first', 'workspace.tab.second',
        'workspace.tab.third', 'workspace.tab.fourth', 'workspace.tab.fifth', 'workspace.tab.sixth',
        'workspace.tab.seventh', 'workspace.tab.eighth', 'workspace.tab.ninth', 'workspace.tab.go_to',
        'navigation.workspace.open', 'navigation.history.open', 'navigation.memories.open',
        'navigation.tasklists.open', 'navigation.jobs.open', 'navigation.profiles.open',
        'navigation.settings.open', 'navigation.help.open', 'navigation.about.open',
        'navigation.data.export.open', 'navigation.data.import.open', 'navigation.palette.open',
        'navigation.menu.open', 'help.shortcuts.show', 'workspace.panel.focus',
        'chat.model.open', 'chat.history.open', 'chat.profile.open',
      ],
    }),
    dispatchLocalCommandKey: keyboardState.dispatch,
    beginLocalCommandUIKey: keyboardState.beginUI,
    resetLocalCommandKeyboard: keyboardState.reset,
  }),
}));

function CommandSourcePanel({ getter, surfaceID = 'source-panel' }: { getter: SurfaceContextGetter; surfaceID?: string }) {
  const scope = useCommandContextScope();
  const root = useRef<HTMLDivElement>(null);
  useLayoutEffect(() => scope?.registerSurface(surfaceID, root, getter), [scope, getter, surfaceID]);
  return <div ref={root}><button>source control</button></div>;
}

const navigateSpy = vi.fn();
const navigationRouteCases = Object.entries(COMMAND_NAVIGATION_ROUTES) as Array<[string, string]>;
const navigationKeyboardCases = navigationRouteCases.map(([commandID, route], index) => [
  commandID,
  route,
  `Key${String.fromCharCode(65 + index)}`,
  String.fromCharCode(97 + index),
] as const);
const toggleMenuSpy = vi.fn();
const announceSpy = vi.fn();
const commandOpenSpy = vi.fn();
const modalState = vi.hoisted(() => ({ open: false }));
const locationState = vi.hoisted(() => ({ pathname: '/history', search: '', hash: '', key: 'history' }));
const catalogState = vi.hoisted(() => ({ list: vi.fn() }));
const authState = vi.hoisted(() => ({
  isAuthenticated: true,
  user: { userId: 'user-a', sessionId: 'session-a', role: 'user' },
  listeners: new Set<() => void>(),
}));
const menuHookState = vi.hoisted(() => ({
  afterSelect: undefined as (() => void) | undefined,
  afterDismisses: [] as Array<(() => void) | undefined>,
}));
const contextMenuState = vi.hoisted(() => ({
  openAtPoint: vi.fn(),
  items: [] as Array<{ id: string; action?: () => void }>,
  afterSelect: undefined as (() => void) | undefined,
}));
const executionState = vi.hoisted(() => ({
  takeResolve: undefined as ((response: unknown) => void) | undefined,
  port: {
    beginUICommand: vi.fn().mockResolvedValue({ ticket: 'ticket-1', invocationId: 'inv-1', commandId: 'help.shortcuts.show' }),
    takeUICommand: vi.fn(() => new Promise((resolve) => { executionState.takeResolve = resolve; })),
    completeUICommand: vi.fn().mockResolvedValue(undefined),
    getUICommandResult: vi.fn().mockResolvedValue({ invocationId: 'inv-1', status: 'succeeded' }),
    cancelUICommand: vi.fn().mockResolvedValue(undefined),
    commitBackendCommand: vi.fn().mockResolvedValue(undefined),
  },
}));
const backendExecutionState = vi.hoisted(() => ({
  execute: vi.fn(),
  cancelPresentation: vi.fn(),
  dispose: vi.fn(),
}));
const workspaceState = vi.hoisted(() => ({
  workspace: { id: 'workspace-a', name: 'Test Workspace', profile: '', activeTabId: 'tab-a', tabs: [{ id: 'tab-a', type: 'chat' as const }] as Array<{ id: string; type: 'chat'; profileOverride?: Record<string, unknown> }> },
  workspaces: [] as Array<{ id: string; name: string; is_active: boolean; tab_count: number }>,
  switchWorkspace: vi.fn(),
  setActiveTab: vi.fn((tabId: string) => {
    workspaceState.workspace = { ...workspaceState.workspace, activeTabId: tabId };
    workspaceState.listeners.forEach((listener) => listener());
  }),
  createWorkspace: vi.fn(),
  renameWorkspace: vi.fn(),
  exportWorkspace: vi.fn(),
  importWorkspace: vi.fn(),
  listeners: new Set<() => void>(),
}));
const workspacePanelState = vi.hoisted(() => ({
  current: true,
  useReal: false,
  focus: vi.fn(() => { (document.querySelector('.ws-content__panel') as HTMLElement | null)?.focus(); }),
  dispose: vi.fn(),
}));
function setupNavigationTabs() {
  workspaceState.workspace = {
    ...workspaceState.workspace,
    activeTabId: 'tab-a',
    tabs: Array.from({ length: 9 }, (_, index) => ({
      id: index === 0 ? 'tab-a' : `nav-${index + 1}`,
      type: 'chat' as const,
      title: `Tab ${index + 1}`,
      position: index,
    })),
  };
}
function navigationTargetId(commandId: string) {
  if (!isWorkspaceTabNavigationCommand(commandId)) throw new Error('Expected navigation command');
  if (commandId === 'workspace.tab.go_to') return 'nav-2';
  const position = commandId === 'workspace.tab.next' ? 2
    : commandId === 'workspace.tab.previous' ? 9
    : WORKSPACE_TAB_NAVIGATION_COMMAND_IDS.indexOf(commandId) - 1;
  return position === 1 ? 'tab-a' : `nav-${position}`;
}

function CreationMenuHost({
  onShow,
  onClose,
}: {
  onShow: (request: WorkspaceTabCreationMenuRequest, onSelect: (commandID: string, intent: WorkspaceTabCreationMenuRequest['intent']) => void) => void;
  onClose?: (options: { restoreFocus: boolean }) => void;
}) {
  const menu = useWorkspaceTabCreationMenu();
  useEffect(() => menu?.registerHost({
    show: (request, onSelect) => {
      onShow(request, onSelect);
      return true;
    },
    close: (options = { restoreFocus: false }) => onClose?.(options),
    focusTrigger: vi.fn(),
    isOpen: () => true,
  }), [menu, onClose, onShow]);
  return null;
}

function LegacyKeyboardListenerProbe() {
  useWorkspaceKeyboardShortcuts();
  return null;
}

function activeWorkspaceSnapshot(conversationID = 'conversation-a') {
  return {
    id: 'workspace-a',
    name: 'Test Workspace',
    snapshot_epoch: 'epoch-1',
    snapshot_sequence: '1',
    tabs: {
      active: 'tab-a',
      items: [{
        id: 'tab-a',
        type: 'editor',
        title: 'Editor',
        position: 0,
        conversation_id: conversationID,
      }],
    },
  };
}
let unregisterWorkspacePanelFocus: (() => void) | undefined;
catalogState.list.mockResolvedValue([]);

vi.mock('../../services/commandCatalog', () => ({
  listCommandCatalog: (query: unknown) => catalogState.list(query),
}));

vi.mock('../../store/authStore', () => ({
  useAuthStore: Object.assign(
    (selector?: (state: typeof authState) => unknown) => selector ? selector(authState) : authState,
    {
      getState: () => authState,
      subscribe: (listener: () => void) => { authState.listeners.add(listener); return () => authState.listeners.delete(listener); },
    },
  ),
}));

vi.mock('../../lib/commandUIExecutionWails', () => ({
  createCommandUIExecutionWailsPort: () => executionState.port,
}));

vi.mock('../../lib/commandWorkspaceTabWails', () => ({
  createCommandWorkspaceTabWailsPort: () => executionState.port,
}));

vi.mock('../../lib/commandWorkspacePanel', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/commandWorkspacePanel')>();
  return {
    ...actual,
    captureWorkspacePanelTarget: (readPathname: () => string) => {
      if (workspacePanelState.useReal) return actual.captureWorkspacePanelTarget(readPathname);
      if (readPathname() !== '/' || !document.querySelector('.ws-content__panel')) return undefined;
      return {
        isCurrent: () => workspacePanelState.current,
        focus: workspacePanelState.focus,
        dispose: workspacePanelState.dispose,
      };
    },
  };
});

vi.mock('../../lib/commandBackendExecutionWails', () => ({
  createCommandBackendExecutionWailsPort: () => ({ executeCommand: keyboardState.palette }),
}));

vi.mock('../../lib/commandBackendExecution', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../../lib/commandBackendExecution')>();
  return {
    ...actual,
    createCommandBackendExecution: (...args: Parameters<typeof actual.createCommandBackendExecution>) => {
      if (!keyboardState.realExecution) return backendExecutionState;
      const real = actual.createCommandBackendExecution(...args);
      const executor = { ...real, cancelPresentation: vi.fn(real.cancelPresentation) };
      keyboardState.executions.push(executor);
      return executor;
    },
  };
});

afterEach(() => {
  // O unmount da Topbar não fecha o store global aberto pelos testes de ajuda.
  act(() => useShortcutsHelpStore.getState().close());
  chatPickerState.available = false;
  chatPickerState.current = true;
  chatPickerState.open.mockClear();
  chatPickerState.captures = [];
  unregisterWorkspacePanelFocus?.();
  unregisterWorkspacePanelFocus = undefined;
  keyboardState.realExecution = false;
  keyboardState.executions = [];
  keyboardState.loadMap.mockReset().mockResolvedValue({ generation: 'g1', bindings: [{
    shortcut: { version: 1 as const, code: 'KeyK', modifiers: ['Control' as const] },
    commandId: 'navigation.palette.open', handler: 'local_ui' as const,
  }, {
    shortcut: { version: 1 as const, code: 'F1', modifiers: [] },
    commandId: 'navigation.help.open', handler: 'local_ui' as const,
  }] });
  keyboardState.dispatch.mockReset().mockResolvedValue(null);
  keyboardState.beginUI.mockReset().mockResolvedValue(null);
  keyboardState.reset.mockClear();
  keyboardState.palette.mockReset();
  keyboardState.handlers.clear();
  workspaceState.switchWorkspace.mockClear();
  workspaceState.setActiveTab.mockClear();
  modalState.open = false;
	vi.restoreAllMocks();
	locationState.pathname = '/history';
	locationState.search = '';
	locationState.hash = '';
	locationState.key = 'history';
  catalogState.list.mockReset();
  catalogState.list.mockResolvedValue([]);
  commandOpenSpy.mockReset();
  authState.isAuthenticated = true;
  authState.user = { userId: 'user-a', sessionId: 'session-a', role: 'user' };
  authState.listeners.clear();
  workspaceState.workspace = { id: 'workspace-a', name: 'Test Workspace', profile: '', activeTabId: 'tab-a', tabs: [{ id: 'tab-a', type: 'chat' as const }] };
  workspaceState.listeners.clear();
  workspacePanelState.current = true;
  workspaceChatState.canPrepare.mockReset().mockReturnValue(true);
  workspaceChatState.prepare.mockReset();
  workspaceChatState.register.mockReset();
  workspaceChatState.getActiveWorkspace.mockReset();
  workspaceChatState.lease.isCurrent.mockReset().mockReturnValue(true);
  workspaceChatState.lease.dispose.mockReset();
  workspaceChatState.lease.present.mockReset();
  workspacePanelState.useReal = false;
  workspacePanelState.focus.mockClear();
  workspacePanelState.dispose.mockClear();
  menuHookState.afterSelect = undefined;
  menuHookState.afterDismisses = [];
  contextMenuState.openAtPoint.mockClear();
  contextMenuState.items = [];
  contextMenuState.afterSelect = undefined;
  executionState.takeResolve = undefined;
  backendExecutionState.execute.mockReset();
  backendExecutionState.cancelPresentation.mockClear();
  backendExecutionState.dispose.mockClear();
  backendExecutionState.execute.mockResolvedValue({
    execution: { invocationId: 'inv-workspace-1', status: 'succeeded' },
    presented: true,
    reason: 'presented',
  });
  Object.values(executionState.port).forEach((method) => method.mockClear());
  executionState.port.beginUICommand.mockResolvedValue({ ticket: 'ticket-1', invocationId: 'inv-1', commandId: 'help.shortcuts.show' });
  executionState.port.completeUICommand.mockResolvedValue(undefined);
  executionState.port.getUICommandResult.mockResolvedValue({ invocationId: 'inv-1', status: 'succeeded' });
  executionState.port.commitBackendCommand.mockResolvedValue(undefined);
});

vi.mock('../ui/Modal', async (importOriginal) => {
  const actual = await importOriginal<typeof import('../ui/Modal')>();
  return { ...actual, isModalOpen: () => modalState.open };
});

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: 'en', changeLanguage: vi.fn() },
  }),
}));

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual<typeof import('react-router-dom')>('react-router-dom');
  return {
    ...actual,
    useNavigate: () => navigateSpy,
    useLocation: () => locationState,
  };
});

vi.mock('../../store/settingsStore', () => ({
  useSettingsStore: (selector: (state: { updateConfig: (cfg: unknown) => void }) => unknown) =>
    selector({ updateConfig: vi.fn() }),
}));

vi.mock('../../store/workspaceStore', () => ({
  flushWorkspaceNavigation: vi.fn(async () => true),
  useWorkspaceStore: Object.assign(
    (selector?: (state: typeof workspaceState) => unknown) => selector ? selector(workspaceState) : workspaceState,
    {
      getState: () => workspaceState,
    },
    { subscribe: (listener: () => void) => { workspaceState.listeners.add(listener); return () => workspaceState.listeners.delete(listener); } },
  ),
}));

vi.mock('../../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: (options: { onAfterSelect?: () => void; onAfterDismiss?: () => void } = {}) => ({
    ...(menuHookState.afterSelect = options.onAfterSelect, menuHookState.afterDismisses.push(options.onAfterDismiss), {}),
    menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' },
    openForTrigger: commandOpenSpy,
    openAtPoint: vi.fn((_x: number, _y: number, _ariaLabel: string, items: Array<{ id: string; action?: () => void }>) => {
      contextMenuState.openAtPoint(_x, _y, _ariaLabel, items);
      contextMenuState.items = items;
      contextMenuState.afterSelect = options.onAfterSelect;
    }),
    closeMenu: vi.fn(),
    onSelectItem: vi.fn(),
  }),
}));

// Unit tests observe the controlled picker contract; the real Combobox and
// its focus/keyboard behavior are exercised in Topbar.palette.integration.
vi.mock('../pickers/Combobox', () => {
  const React = require('react');
  return { Combobox: (props: {
    items: Array<{ value: string; label: string; searchText?: string; accessibleLabel?: string; disabled?: boolean }>;
    open?: boolean;
    label: string;
    triggerAriaLabel?: string;
    shortcut?: string;
    triggerRef?: React.Ref<HTMLButtonElement>;
    busy?: boolean;
    onOpenChange?: (open: boolean) => void;
    onSelect: (value: string) => void;
    onAfterSelect?: () => void;
    onAfterDismiss?: () => void;
  }) => {
    menuHookState.afterSelect = props.onAfterSelect;
    menuHookState.afterDismisses.push(props.onAfterDismiss);
    React.useEffect(() => {
      if (!props.open) return;
      commandOpenSpy(document.activeElement, props.triggerAriaLabel ?? props.label, props.items.map(item => ({
        id: item.value, label: item.label, searchText: item.searchText,
        ariaLabel: item.accessibleLabel, disabled: item.disabled,
        action: () => {
          if (item.disabled) return;
          props.onSelect(item.value);
          props.onOpenChange?.(false);
        },
      })));
    }, [props.open, props.items]);
    return <button ref={props.triggerRef} aria-label={props.triggerAriaLabel ?? props.label} data-shortcut={props.shortcut}
      aria-expanded={props.open} aria-busy={props.busy}
      onClick={() => props.onOpenChange?.(!props.open)}>{props.label}</button>;
  } };
});

vi.mock('../pickers/ProfilePicker', () => {
  const React = require('react');
  return { ProfilePicker: () => React.createElement('div', { 'data-testid': 'profile-picker-fixture' }) };
});

vi.mock('../../hooks/useAnnouncer', () => ({
  useAnnouncer: () => ({ announce: announceSpy, announceRequest: vi.fn(() => true) }),
  announce: (...args: unknown[]) => announceSpy(...args),
}));

vi.mock('./MenuButton', () => {
  const React = require('react');
  const MenuButton = React.forwardRef((props: { items: Array<{ id: string; shortcut?: string; onClick?: () => void }>; buttonLabel: string; currentItemId: string; onAfterSelect?: () => void }, ref: React.Ref<{ toggleMenu: () => void }>) => {
    React.useImperativeHandle(ref, () => ({ toggleMenu: toggleMenuSpy }));

    return (
      <div>
        <button onClick={() => props.items[0]?.onClick?.()}>{props.buttonLabel}</button>
        {props.items.map((item) => (
          <button key={item.id} data-testid={`main-menu-${item.id}`} data-shortcut={item.shortcut} onClick={() => { item.onClick?.(); props.onAfterSelect?.(); }}>
            {item.id}
          </button>
        ))}
        <div data-testid="current-item">{props.currentItemId}</div>
        <div data-testid="menu-items">{props.items.map((item) => item.id).join(',')}</div>
      </div>
    );
  });

  return { MenuButton, MenuItem: {}, MenuButtonRef: {} };
});

describe('Topbar — integração dos comandos de picker com o mapa efetivo', () => {
  const cases = [
    ['chat.model.open', 'KeyM', 'm'],
    ['chat.history.open', 'KeyH', 'h'],
    ['chat.profile.open', 'KeyP', 'p'],
  ] as const;

  async function mount(commandId: string, code: string) {
    locationState.pathname = '/';
    chatPickerState.available = true;
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'g-chat', bindings: [{
      shortcut: { version: 1, code, modifiers: ['Control'] }, commandId, handler: 'local_ui',
    }] });
    const view = render(<><Topbar /><textarea data-testid="chat-picker-input" /></>);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    await act(async () => { await Promise.resolve(); });
    const input = screen.getByTestId('chat-picker-input');
    input.focus();
    return { view, input };
  }

  function expectNoDurableExecution() {
    expect(keyboardState.dispatch).not.toHaveBeenCalled();
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
    expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
  }

  it.each(cases)('%s usa teclado e Deck no chat modal autorizado, sem ledger ou autorepeat', async (commandId, code, key) => {
    const { view, input } = await mount(commandId, code);
    try {
      modalState.open = true;
      fireEvent.keyDown(input, { key, code, ctrlKey: true });
      expect(chatPickerState.open).toHaveBeenCalledExactlyOnceWith(commandId);
      fireEvent.keyDown(input, { key, code, ctrlKey: true, repeat: true });
      expect(chatPickerState.open).toHaveBeenCalledTimes(1);
      fireEvent.keyUp(input, { key, code, ctrlKey: true });
      act(() => keyboardState.handlers.get('command:deck-local-ui')?.({
        commandId, generation: 'g-chat', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      }));
      expect(chatPickerState.open).toHaveBeenCalledTimes(2);
      expectNoDurableExecution();
    } finally { view.unmount(); }
  });

  it.each(['scope-missing', 'stale-target', 'ime', 'blur', 'owner', 'generation'])('recusa picker do Deck: %s', async (failure) => {
    const { view, input } = await mount('chat.model.open', 'KeyM');
    try {
      modalState.open = true;
      if (failure === 'scope-missing') chatPickerState.available = false;
      if (failure === 'stale-target') chatPickerState.current = false;
      if (failure === 'ime') fireEvent.compositionStart(input);
      if (failure === 'blur') vi.mocked(document.hasFocus).mockReturnValue(false);
      act(() => keyboardState.handlers.get('command:deck-local-ui')?.({
        commandId: 'chat.model.open', generation: failure === 'generation' ? 'old' : 'g-chat',
        userId: failure === 'owner' ? 'other-user' : 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      }));
      expect(chatPickerState.open).not.toHaveBeenCalled();
      expectNoDurableExecution();
    } finally { view.unmount(); }
  });

  it('remapeia picker e suprime a tecla antiga sem fallback', async () => {
    const { view, input } = await mount('chat.model.open', 'KeyY');
    try {
      fireEvent.keyDown(input, { key: 'm', code: 'KeyM', ctrlKey: true });
      expect(chatPickerState.open).not.toHaveBeenCalled();
      fireEvent.keyDown(input, { key: 'y', code: 'KeyY', ctrlKey: true });
      expect(chatPickerState.open).toHaveBeenCalledExactlyOnceWith('chat.model.open');
      expectNoDurableExecution();
    } finally { view.unmount(); }
  });

  it('remapear a mesma tecla para navegação não autoriza executar atrás de modal', async () => {
    const { view, input } = await mount('navigation.settings.open', 'KeyM');
    try {
      modalState.open = true;
      fireEvent.keyDown(input, { key: 'm', code: 'KeyM', ctrlKey: true });
      expect(navigateSpy).not.toHaveBeenCalled();
      expect(chatPickerState.open).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each([false, true])('paleta usa somente a instância capturada antes da busca; substituída=%s', async (replaced) => {
    const { view } = await mount('chat.model.open', 'KeyM');
    catalogState.list.mockResolvedValue([{ id: 'chat.model.open', name: 'Modelos', available: true }]);
    try {
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      await waitFor(() => expect(commandOpenSpy).toHaveBeenCalled());
      const item = (commandOpenSpy.mock.calls[commandOpenSpy.mock.calls.length - 1]?.[2] as Array<{ action?: () => void; disabled?: boolean }>)[0];
      expect(item.disabled).toBe(false);
      if (replaced) chatPickerState.captures.forEach(capture => { capture.valid = false; });
      await act(async () => { item.action?.(); menuHookState.afterSelect?.(); });
      expect(chatPickerState.open).toHaveBeenCalledTimes(replaced ? 0 : 1);
      expectNoDurableExecution();
    } finally { view.unmount(); }
  });
});

function changeKeyboardProfile(profile: string) {
  workspaceState.workspace = { ...workspaceState.workspace, profile };
  workspaceState.listeners.forEach(listener => listener());
}

describe('Topbar teclado local — controller e guard reais', () => {

  it.each([1, 2] as const)('perfil efetivo seleciona navegação local v%s sem backend ou rebuild', async (version) => {
    locationState.pathname = '/';
    workspaceState.workspace.profile = 'profile-a';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const shortcut: import('../../lib/commandShortcut').CommandKeyboardTrigger = version === 1
      ? { version: 1, code: 'KeyY', modifiers: ['Control', 'Shift'] }
      : { version: 2, steps: [{ code: 'KeyY', modifiers: ['Control', 'Shift'] }, { code: 'KeyL', modifiers: [] }] };
    const leaf = (commandId: string): import('../../lib/commandLocalKeyboard').LocalCommandContextualBinding => ({
      shortcut, bySurface: { chat: { shortcut, commandId, handler: 'local_ui' } }, fallback: null,
    });
    keyboardState.loadMap.mockResolvedValue({ generation: 'profile-map', bindings: [], contextualBindings: [{
      shortcut, bySurface: { chat: null }, fallback: null, byProfile: {
        'profile-a': leaf('navigation.settings.open'), 'profile-b': leaf('navigation.history.open'),
      },
    }] });
    const view = render(<CommandContextProvider><CommandSourcePanel surfaceID="tab-a" getter={() => ({
      surfaceId: 'tab-a', surfaceType: 'chat', snapshotVersion: 'chat-v1',
    })} /><Topbar /></CommandContextProvider>);
    const control = screen.getByRole('button', { name: 'source control' });
    const press = async () => {
      control.focus();
      await act(async () => {
        fireEvent.keyDown(control, { code: 'KeyY', key: 'Y', ctrlKey: true, shiftKey: true });
        fireEvent.keyUp(control, { code: 'KeyY' });
        if (version === 2) {
          fireEvent.keyDown(control, { code: 'KeyL', key: 'l' });
          fireEvent.keyUp(control, { code: 'KeyL' });
        }
      });
    };
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      const loads = keyboardState.loadMap.mock.calls.length;
      await press();
      expect(navigateSpy).toHaveBeenLastCalledWith('/settings');
      await act(async () => changeKeyboardProfile('profile-b'));
      await press();
      expect(navigateSpy).toHaveBeenLastCalledWith('/history');
      // Override da aba tem precedência sobre o perfil do workspace.
      workspaceState.workspace = { ...workspaceState.workspace, tabs: [{ id: 'tab-a', type: 'chat', profileOverride: { slug: 'profile-a' } }] };
      workspaceState.listeners.forEach(listener => listener());
      await press();
      expect(navigateSpy).toHaveBeenLastCalledWith('/settings');
      expect(keyboardState.loadMap).toHaveBeenCalledTimes(loads);
      expect(keyboardState.dispatch).not.toHaveBeenCalled();
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(keyboardState.palette).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it('sequência por perfil não sobrevive à transição A-B-A entre etapas', async () => {
    locationState.pathname = '/';
    workspaceState.workspace.profile = 'profile-a';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const shortcut: import('../../lib/commandShortcut').CommandKeyboardTrigger = {
      version: 2, steps: [{ code: 'KeyY', modifiers: ['Control', 'Shift'] }, { code: 'KeyL', modifiers: [] }],
    };
    keyboardState.loadMap.mockResolvedValue({ generation: 'profile-sequence', bindings: [], contextualBindings: [{
      shortcut, bySurface: { chat: null }, fallback: null, byProfile: {
        'profile-a': { shortcut, bySurface: { chat: { shortcut, commandId: 'navigation.settings.open', handler: 'local_ui' } }, fallback: null },
      },
    }] });
    const view = render(<CommandContextProvider><CommandSourcePanel surfaceID="tab-a" getter={() => ({
      surfaceId: 'tab-a', surfaceType: 'chat', snapshotVersion: 'chat-v1',
    })} /><Topbar /></CommandContextProvider>);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      const control = screen.getByRole('button', { name: 'source control' });
      control.focus();
      await act(async () => {
        fireEvent.keyDown(control, { code: 'KeyY', key: 'Y', ctrlKey: true, shiftKey: true });
        fireEvent.keyUp(control, { code: 'KeyY' });
        changeKeyboardProfile('profile-b');
        changeKeyboardProfile('profile-a');
        fireEvent.keyDown(control, { code: 'KeyL', key: 'l' });
      });
      expect(navigateSpy).not.toHaveBeenCalled();
      expect(keyboardState.dispatch).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it.each([1, 2] as const)('aciona consulta contextual v%s com observação da surface real', async (version) => {
    locationState.pathname = '/';
    keyboardState.realExecution = true;
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const trigger: import('../../lib/commandShortcut').CommandKeyboardTrigger = version === 1
      ? { version: 1, code: 'KeyY', modifiers: ['Control', 'Shift'] }
      : { version: 2, steps: [{ code: 'KeyY', modifiers: ['Control', 'Shift'] }, { code: 'KeyL', modifiers: [] }] };
    keyboardState.loadMap.mockResolvedValue({ generation: 'context-map', bindings: [], contextualBindings: [{
      shortcut: trigger, bySurface: { chat: { shortcut: trigger, commandId: 'workspace.list', handler: 'backend' } }, fallback: null,
    }] });
    keyboardState.dispatch.mockResolvedValue({ invocationId: 'context-run', status: 'succeeded', output: { kind: 'workspace.list', workspaces: [] } });
    const showCreation = vi.fn();
    const view = render(<WorkspaceTabCreationMenuProvider><CommandContextProvider><CreationMenuHost onShow={showCreation} /><CommandSourcePanel surfaceID="tab-a" getter={() => ({
      surfaceId: 'tab-a', surfaceType: 'chat', snapshotVersion: 'chat-v1',
    })} /><Topbar /></CommandContextProvider></WorkspaceTabCreationMenuProvider>);
    try {
      const control = screen.getByRole('button', { name: 'source control' });
      control.focus();
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      await act(async () => { fireEvent.keyDown(control, { code: 'KeyY', key: 'Y', ctrlKey: true, shiftKey: true }); });
      if (version === 2) {
        expect(keyboardState.dispatch).not.toHaveBeenCalled();
        fireEvent.keyUp(control, { code: 'KeyY' });
        await act(async () => { fireEvent.keyDown(control, { code: 'KeyL', key: 'l' }); });
      }
      await waitFor(() => expect(keyboardState.dispatch).toHaveBeenCalledTimes(1));
      expect(keyboardState.dispatch).toHaveBeenCalledWith('context-map', trigger, 'down', false,
        expect.objectContaining({ surfaceId: 'tab-a', surfaceType: 'chat', isCurrent: expect.any(Function) }));
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(showCreation).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  const shortcut = { version: 1 as const, code: 'KeyK', modifiers: ['Control' as const, 'Shift' as const] };
  const output: BackendCommandExecutionResult = {
    invocationId: 'keyboard-invocation', status: 'succeeded',
    output: { kind: 'workspace.list', workspaces: [
      { id: 'workspace-b', name: 'Workspace B', profile: '', tab_count: 2, is_active: false },
    ] },
  };

  async function mountKeyboard() {
    keyboardState.realExecution = true;
    keyboardState.loadMap.mockResolvedValue({ generation: 'g1', bindings: [{ shortcut, commandId: 'workspace.list', handler: 'backend' }] });
    keyboardState.dispatch.mockResolvedValue(output);
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<Topbar />);
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    return view;
  }

  async function press(target: Window | Element = document.activeElement!) {
    await act(async () => {
      fireEvent.keyDown(target, { key: 'K', code: 'KeyK', ctrlKey: true, shiftKey: true });
    });
  }

  function changeContext(change: string) {
    if (change === 'auth') authState.isAuthenticated = false;
    if (change === 'session') authState.user = { ...authState.user, sessionId: 'session-b' };
    if (change === 'user') authState.user = { ...authState.user, userId: 'user-b' };
    if (change === 'workspace') workspaceState.workspace = { ...workspaceState.workspace, id: 'workspace-c' };
    if (change === 'tab') workspaceState.workspace = { ...workspaceState.workspace, activeTabId: 'tab-b' };
    if (change === 'route') locationState.pathname = '/about';
    if (change === 'modal') modalState.open = true;
  }

  it.each(['layer.activate', 'layer.toggle', 'layer.back'])('despacha %s pelo teclado sem apresentar output de workspace', async (commandId) => {
    keyboardState.realExecution = true;
    keyboardState.loadMap.mockResolvedValue({ generation: 'g-layer', bindings: [{ shortcut, commandId, handler: 'backend' }] });
    keyboardState.dispatch.mockResolvedValue({ invocationId: 'layer-invocation', status: 'succeeded' });
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<Topbar />);
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    await press();
    expect(keyboardState.dispatch).toHaveBeenCalledExactlyOnceWith('g-layer', shortcut, 'down', false);
    expect(commandOpenSpy).not.toHaveBeenCalled();
    expect(keyboardState.palette).not.toHaveBeenCalled();
    view.unmount();
  });

  it('Ctrl+Shift+K despacha teclado e apresenta picker; up ignora resultado e não executa paleta', async () => {
    const view = await mountKeyboard();
    await act(async () => { fireEvent.keyDown(window, { key: 'K', code: 'KeyK', ctrlKey: true, shiftKey: true }); });
    expect(keyboardState.dispatch).toHaveBeenCalledExactlyOnceWith('g1', shortcut, 'down', false);
    expect(commandOpenSpy).toHaveBeenCalledTimes(1);
    const items: MenuItem[] = commandOpenSpy.mock.calls[0][2];
    expect(items[0]).toMatchObject({ id: 'ws-command-workspace-b', label: 'Workspace B' });
    await act(async () => { fireEvent.keyUp(window, { key: 'K', code: 'KeyK' }); });
    expect(keyboardState.dispatch).toHaveBeenLastCalledWith('g1', shortcut, 'up', false);
    expect(commandOpenSpy).toHaveBeenCalledTimes(1);
    items[0].action?.();
    items[0].action?.();
    expect(workspaceState.switchWorkspace).toHaveBeenCalledExactlyOnceWith('workspace-b');
    expect(keyboardState.palette).not.toHaveBeenCalled();
    expect(catalogState.list).not.toHaveBeenCalled();
    view.unmount();
  });

  it('executa UI local de navegação imediatamente e não usa handoff', async () => {
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-ui',
      bindings: [{ shortcut, commandId: 'navigation.settings.open', handler: 'local_ui' }],
    });
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<Topbar />);
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    await press();
    expect(navigateSpy).toHaveBeenCalledWith('/settings');
    expect(keyboardState.dispatch).not.toHaveBeenCalled();
    expect(keyboardState.palette).not.toHaveBeenCalled();
    await act(async () => { fireEvent.keyUp(window, { key: 'K', code: 'KeyK' }); });
    expect(keyboardState.dispatch).not.toHaveBeenCalled();
    expect(keyboardState.palette).not.toHaveBeenCalled();
    view.unmount();
  });

  it('confirma UI local após a navegação desmontar a Topbar, sem Cancel tardio', async () => {
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-own-route',
      bindings: [{ shortcut, commandId: 'navigation.settings.open', handler: 'local_ui' }],
    });
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    let view!: ReturnType<typeof render>;
    navigateSpy.mockImplementationOnce(() => { view.unmount(); });
    view = render(<Topbar />);
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    await press();
    expect(navigateSpy).toHaveBeenCalledWith('/settings');
    expect(navigateSpy).toHaveBeenCalledWith('/settings');
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
  });

  it('executa UI local sem criar handoff quando a rota muda', async () => {
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-stale',
      bindings: [{ shortcut, commandId: 'navigation.settings.open', handler: 'local_ui' }],
    });
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<Topbar />);
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    navigateSpy.mockClear();
    locationState.pathname = '/about';
    await act(async () => { view.rerender(<Topbar />); });
    await press();
    expect(navigateSpy).toHaveBeenCalledWith('/settings');
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    view.unmount();
  });

  it('não cria handoff UI local ao pressionar Escape após mudança de rota', async () => {
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-escape',
      bindings: [{ shortcut, commandId: 'navigation.settings.open', handler: 'local_ui' }],
    });
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<Topbar />);
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    locationState.pathname = '/about';
    await act(async () => { view.rerender(<Topbar />); });
    navigateSpy.mockClear();
    await press();
    expect(navigateSpy).toHaveBeenCalledWith('/settings');
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    view.unmount();
  });

  it.each(['modal', 'editable'])('não despacha durante %s', async (kind) => {
    const view = await mountKeyboard();
    const input = document.createElement('input');
    document.body.appendChild(input);
    try {
      if (kind === 'modal') modalState.open = true;
      else input.focus();
      await press();
      expect(keyboardState.dispatch).not.toHaveBeenCalled();
      expect(commandOpenSpy).not.toHaveBeenCalled();
    } finally { input.remove(); view.unmount(); }
  });

  it.each(['blur', 'map', 'auth', 'session', 'workspace', 'tab', 'route', 'modal', 'unmount'])(
    'descarta resposta pendente após %s',
    async (change) => {
      const view = await mountKeyboard();
      let resolve!: (value: BackendCommandExecutionResult) => void;
      keyboardState.dispatch.mockReturnValueOnce(new Promise((done) => { resolve = done; }));
      await press();
      expect(keyboardState.dispatch).toHaveBeenCalledTimes(1);
      await act(async () => {
        if (change === 'blur') window.dispatchEvent(new Event('blur'));
        else if (change === 'map') keyboardState.handlers.get('command:keyboard-map-changed')!();
        else if (change === 'unmount') view.unmount();
        else {
          changeContext(change);
          view.rerender(<Topbar />);
        }
      });
      await act(async () => { resolve(output); });
      if (change === 'blur' || change === 'map') {
        expect(keyboardState.executions[1].cancelPresentation).toHaveBeenCalled();
      }
      expect(commandOpenSpy).not.toHaveBeenCalled();
      expect(keyboardState.palette).not.toHaveBeenCalled();
      view.unmount();
    },
  );

  it.each(['auth', 'session', 'user', 'workspace', 'tab', 'route', 'modal'])(
    'revalida ação do picker após %s sem depender de notificação do store',
    async (change) => {
      const view = await mountKeyboard();
      await press();
      const items: MenuItem[] = commandOpenSpy.mock.calls[0][2];
      changeContext(change);
      if (change === 'route') view.rerender(<Topbar />);
      items[0].action?.();
      expect(workspaceState.switchWorkspace).not.toHaveBeenCalled();
      view.unmount();
    },
  );

  it.each(['auth', 'route', 'workspace'])('limpa geração, listeners e pendência após %s', async (change) => {
    const view = await mountKeyboard();
    const previousHandler = keyboardState.handlers.get('command:keyboard-map-changed')!;
    changeContext(change);
    await act(async () => { view.rerender(<Topbar />); });
    expect(keyboardState.reset).toHaveBeenCalledWith('g1');
    expect(keyboardState.handlers.get('command:keyboard-map-changed')).not.toBe(previousHandler);
    const loads = keyboardState.loadMap.mock.calls.length;
    await act(async () => { previousHandler(); });
    expect(keyboardState.loadMap).toHaveBeenCalledTimes(loads);
    view.unmount();
    await act(async () => {});
    expect(keyboardState.handlers.size).toBe(0);
    expect(authState.listeners.size).toBe(0);
    expect(workspaceState.listeners.size).toBe(0);
    await press(window);
    fireEvent.focus(window);
    expect(keyboardState.dispatch).not.toHaveBeenCalled();
    expect(keyboardState.loadMap).toHaveBeenCalledTimes(loads);
  });

  it('não anuncia feedback antigo quando a autenticação muda antes do cleanup do efeito', async () => {
    const view = await mountKeyboard();
    const feedback = keyboardState.handlers.get('command:deck-feedback');
    expect(feedback).toBeDefined();

    announceSpy.mockClear();
    feedback?.({
      invocationId: 'deck-feedback-auth-a', state: 'waiting', title: 'Open chat',
      userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      generation: 'g1', expiresAt: Date.now() + 3_000,
    });
    expect(announceSpy).toHaveBeenCalledWith('commandDeckFeedback.waiting');

    announceSpy.mockClear();
    authState.user = { ...authState.user, userId: 'user-b' };
    // Deliberately do not notify the store listener: the callback must still
    // compare the live auth principal with the keyboard map owner.
    feedback?.({
      invocationId: 'deck-feedback-auth-b', state: 'waiting', title: 'Open chat',
      userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      generation: 'g1', expiresAt: Date.now() + 3_000,
    });
    expect(announceSpy).not.toHaveBeenCalled();
    view.unmount();
  });

  it('recarrega no focus da janela e ignora blur de um filho para reset do mapa', async () => {
    const view = await mountKeyboard();
    const loads = keyboardState.loadMap.mock.calls.length;
    fireEvent.blur(document.activeElement!);
    expect(keyboardState.reset).not.toHaveBeenCalled();
    await act(async () => { window.dispatchEvent(new Event('blur')); window.dispatchEvent(new Event('focus')); });
    expect(keyboardState.reset).toHaveBeenCalledWith('g1');
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalledTimes(loads + 1));
    view.unmount();
  });
});

describe('Topbar', () => {
  const creationCases = [
    ['C', 'chat'],
    ['E', 'editor'],
    ['R', 'terminal'],
    ['T', 'tasklist'],
  ] as const;

  it('não deixa o listener reservado legado bloquear workspace.chat.open no pipeline contextual', async () => {
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    document.body.appendChild(root);
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-chat-open',
      bindings: [{
        shortcut: { version: 1, code: 'KeyI', modifiers: ['Control', 'Shift'] },
        commandId: 'workspace.chat.open',
        handler: 'contextual',
      }],
    });
    keyboardState.beginUI.mockResolvedValue({
      ticket: 'chat-key-ticket', invocationId: 'chat-key-inv', commandId: 'workspace.chat.open',
    });
    executionState.port.takeUICommand.mockResolvedValue({
      ticket: 'chat-key-ticket', invocationId: 'chat-key-inv', commandId: 'workspace.chat.open',
      handoffId: 'chat-key-handoff',
    });
    executionState.port.getUICommandResult.mockResolvedValue({
      invocationId: 'chat-key-inv', status: 'succeeded',
    });
    workspaceChatState.prepare.mockResolvedValue(workspaceChatState.lease);
    workspaceChatState.getActiveWorkspace.mockResolvedValue(activeWorkspaceSnapshot());

    const view = render(
      <CommandContextProvider>
        <LegacyKeyboardListenerProbe />
        <Topbar />
      </CommandContextProvider>,
    );
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      const trigger = screen.getByRole('button', { name: 'commandPalette.title' });
      trigger.focus();
      const event = new KeyboardEvent('keydown', {
        key: 'I', code: 'KeyI', ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true,
      });
      await act(async () => {
        window.dispatchEvent(event);
        await Promise.resolve();
      });

      await waitFor(() => expect(executionState.port.commitBackendCommand)
        .toHaveBeenCalledExactlyOnceWith('chat-key-ticket', 'chat-key-handoff'));
      expect(event.defaultPrevented).toBe(true);
      expect(keyboardState.beginUI).toHaveBeenCalledOnce();
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
      await waitFor(() => expect(workspaceChatState.lease.present).toHaveBeenCalledExactlyOnceWith('conversation-a'));
    } finally {
      view.unmount();
      root.remove();
    }
  });

  it('seleciona workspace.chat.open na própria paleta e apresenta pelo snapshot readonly', async () => {
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    document.body.appendChild(root);
    catalogState.list.mockResolvedValueOnce([{
      id: 'workspace.chat.open', name: 'Open chat', available: true,
    }]);
    executionState.port.beginUICommand.mockResolvedValue({
      ticket: 'chat-palette-ticket', invocationId: 'chat-palette-inv', commandId: 'workspace.chat.open',
    });
    executionState.port.takeUICommand.mockResolvedValue({
      ticket: 'chat-palette-ticket', invocationId: 'chat-palette-inv', commandId: 'workspace.chat.open',
      handoffId: 'chat-palette-handoff',
    });
    executionState.port.getUICommandResult.mockResolvedValue({
      invocationId: 'chat-palette-inv', status: 'succeeded',
    });
    workspaceChatState.prepare.mockResolvedValue(workspaceChatState.lease);
    workspaceChatState.getActiveWorkspace.mockResolvedValue(activeWorkspaceSnapshot('conversation-from-readonly-snapshot'));
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await userEvent.setup().click(screen.getByRole('button', { name: 'commandPalette.title' }));
      await waitFor(() => expect(commandOpenSpy).toHaveBeenCalledOnce());
      const items = commandOpenSpy.mock.calls[0]?.[2] as Array<{ id: string; action?: () => void }>;
      expect(items).toHaveLength(1);
      expect(items[0]).toMatchObject({ id: 'command-workspace.chat.open' });
      await act(async () => {
        items[0].action?.();
        menuHookState.afterSelect?.();
        await Promise.resolve();
      });
      await waitFor(() => expect(executionState.port.commitBackendCommand)
        .toHaveBeenCalledExactlyOnceWith('chat-palette-ticket', 'chat-palette-handoff'));
      await waitFor(() => expect(workspaceChatState.lease.present)
        .toHaveBeenCalledExactlyOnceWith('conversation-from-readonly-snapshot'));
      expect(executionState.port.beginUICommand).toHaveBeenCalledExactlyOnceWith('workspace.chat.open');
    } finally {
      view.unmount();
      root.remove();
    }
  });

  it.each(['monaco', 'contenteditable'] as const)(
    'aceita workspace.chat.open contextual no %s sem tecla prévia, mas não transforma unknown em autorização local',
    async (surface) => {
      locationState.pathname = '/';
      vi.spyOn(document, 'hasFocus').mockReturnValue(true);
      const root = document.createElement('div');
      root.className = 'workspace-layout';
      const editor = document.createElement('div');
      if (surface === 'monaco') editor.className = 'monaco-editor';
      else editor.contentEditable = 'true';
      editor.tabIndex = -1;
      root.appendChild(editor);
      document.body.appendChild(root);
      keyboardState.loadMap.mockResolvedValue({ generation: 'g-chat-deck', bindings: [] });
      executionState.port.takeUICommand.mockImplementation(async (ticket = `chat-deck-${surface}-ticket`) => ({
        ticket,
        invocationId: `chat-deck-${surface}-inv`,
        commandId: 'workspace.chat.open',
        handoffId: `chat-deck-${surface}-handoff`,
      }));
      executionState.port.getUICommandResult.mockResolvedValue({
        invocationId: `chat-deck-${surface}-inv`, status: 'succeeded',
      });
      workspaceChatState.prepare.mockResolvedValue(workspaceChatState.lease);
      workspaceChatState.getActiveWorkspace.mockResolvedValue(activeWorkspaceSnapshot());
      const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
      try {
        editor.focus();
        await waitFor(() => expect(keyboardState.handlers.has('command:deck-ui-reservation')).toBe(true));
        keyboardState.handlers.get('command:deck-ui-reservation')?.({
          ticket: `chat-deck-${surface}-ticket`,
          invocationId: `chat-deck-${surface}-inv`,
          commandId: 'workspace.chat.open',
        });

        await waitFor(() => expect(executionState.port.commitBackendCommand)
          .toHaveBeenCalledExactlyOnceWith(`chat-deck-${surface}-ticket`, expect.any(String)));
        expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
        await waitFor(() => expect(workspaceChatState.lease.present).toHaveBeenCalledExactlyOnceWith('conversation-a'));
      } finally {
        view.unmount();
        root.remove();
      }
    },
  );

  function creationBindings() {
    return creationCases.map(([letter, type]) => ({
      shortcut: {
        version: 2 as const,
        steps: [
          { code: 'KeyN', modifiers: ['Control'] },
          { code: `Key${letter}`, modifiers: [] },
        ] as [{ code: string; modifiers: ('Control' | 'Alt' | 'Shift' | 'Meta')[] }, { code: string; modifiers: ('Control' | 'Alt' | 'Shift' | 'Meta')[] }],
      } as import('../../lib/commandShortcut').CommandKeyboardTrigger,
      commandId: `workspace.tab.${type}.create`,
      handler: 'contextual' as const,
    }));
  }

  async function mountCreationIntegration(options: {
    onShow?: (request: WorkspaceTabCreationMenuRequest, onSelect: (commandID: string, intent: WorkspaceTabCreationMenuRequest['intent']) => void) => void;
    onClose?: (options: { restoreFocus: boolean }) => void;
    includeToolbar?: boolean;
    commandID?: string;
    sourceGetter?: SurfaceContextGetter;
  }) {
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    document.body.appendChild(root);
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-creation',
      bindings: creationBindings(),
    });
    catalogState.list.mockResolvedValue(creationCases.map(([letter, type]) => ({
      id: `workspace.tab.${type}.create`,
      name: `New ${type}`,
      available: true,
      shortcut: letter,
    })));
    executionState.port.beginUICommand.mockImplementation(async () => ({
      ticket: 'palette-ticket', invocationId: 'palette-invocation', commandId: options.commandID ?? 'workspace.tab.chat.create',
    }));
    executionState.port.takeUICommand.mockImplementation(async (ticket = 'palette-ticket') => ({
      ticket,
      invocationId: ticket === 'keyboard-ticket' ? 'keyboard-invocation' : 'palette-invocation',
      commandId: options.commandID ?? 'workspace.tab.chat.create',
      handoffId: ticket === 'keyboard-ticket' ? 'keyboard-handoff' : 'palette-handoff',
    }));
    executionState.port.getUICommandResult.mockImplementation(async (invocationId = 'palette-invocation') => ({ invocationId, status: 'succeeded' }));
    keyboardState.beginUI.mockResolvedValue({
      ticket: 'keyboard-ticket', invocationId: 'keyboard-invocation', commandId: options.commandID ?? 'workspace.tab.chat.create',
    });

    const view = render(
      <WorkspaceTabCreationMenuProvider>
        <CommandContextProvider>
          {options.sourceGetter && <CommandSourcePanel surfaceID="tab-a" getter={options.sourceGetter} />}
          {options.onShow && <CreationMenuHost onShow={options.onShow} onClose={options.onClose} />}
          <Topbar />
          {options.includeToolbar && <WorkspaceToolbar />}
        </CommandContextProvider>
      </WorkspaceTabCreationMenuProvider>,
    );
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    return { view, root };
  }

  it.each([false, true])('mutação contextual revalida a surface entre Begin e commit; mudou=%s', async (changed) => {
    let version = 'chat-v1';
    const { view, root } = await mountCreationIntegration({ sourceGetter: () => ({ surfaceType: 'chat', surfaceId: 'tab-a', snapshotVersion: version }) });
    let release!: (response: import('../../lib/commandUIExecution').UICommandTakeResponse) => void;
    executionState.port.takeUICommand.mockImplementationOnce(() => new Promise(resolve => { release = resolve; }));
    const shortcut = { version: 1 as const, code: 'KeyY', modifiers: ['Control' as const, 'Shift' as const] };
    keyboardState.loadMap.mockResolvedValue({ generation: 'context-create', bindings: [], contextualBindings: [{
      shortcut, bySurface: { chat: { shortcut, commandId: 'workspace.tab.chat.create', handler: 'contextual' } }, fallback: null,
    }] });
    try {
      await act(async () => { keyboardState.handlers.get('command:keyboard-map-changed')?.(); });
      const control = screen.getByRole('button', { name: 'source control' });
      control.focus();
      await act(async () => { fireEvent.keyDown(control, { code: 'KeyY', key: 'Y', ctrlKey: true, shiftKey: true }); });
      await waitFor(() => expect(executionState.port.takeUICommand).toHaveBeenCalledOnce());
      if (changed) version = 'chat-v2';
      await act(async () => { release({ ticket: 'keyboard-ticket', invocationId: 'keyboard-invocation',
        commandId: 'workspace.tab.chat.create', handoffId: 'keyboard-handoff' }); });
      if (changed) expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
      else await waitFor(() => expect(executionState.port.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('keyboard-ticket', 'keyboard-handoff'));
    } finally { view.unmount(); root.remove(); }
  });

  it.each(['stable', 'different', 'aba'] as const)('revalida perfil antes do commit contextual: %s', async (change) => {
    workspaceState.workspace.profile = 'profile-a';
    const { view, root } = await mountCreationIntegration({ sourceGetter: () => ({ surfaceType: 'chat', surfaceId: 'tab-a', snapshotVersion: 'chat-v1' }) });
    let release!: (response: import('../../lib/commandUIExecution').UICommandTakeResponse) => void;
    executionState.port.takeUICommand.mockImplementationOnce(() => new Promise(resolve => { release = resolve; }));
    const shortcut = { version: 1 as const, code: 'KeyY', modifiers: ['Control' as const, 'Shift' as const] };
    keyboardState.loadMap.mockResolvedValue({ generation: 'profile-create', bindings: [], contextualBindings: [{
      shortcut, bySurface: { chat: null }, fallback: null, byProfile: {
        'profile-a': { shortcut, bySurface: { chat: { shortcut, commandId: 'workspace.tab.chat.create', handler: 'contextual' } }, fallback: null },
      },
    }] });
    try {
      await act(async () => { keyboardState.handlers.get('command:keyboard-map-changed')?.(); });
      const control = screen.getByRole('button', { name: 'source control' });
      control.focus();
      await act(async () => { fireEvent.keyDown(control, { code: 'KeyY', key: 'Y', ctrlKey: true, shiftKey: true }); });
      await waitFor(() => expect(executionState.port.takeUICommand).toHaveBeenCalledOnce());
      expect(keyboardState.beginUI).toHaveBeenCalledWith('profile-create', shortcut, false,
        expect.objectContaining({ profile: 'profile-a', surfaceId: 'tab-a', surfaceType: 'chat' }));
      await act(async () => {
        if (change !== 'stable') changeKeyboardProfile('profile-b');
        if (change === 'aba') changeKeyboardProfile('profile-a');
        release({ ticket: 'keyboard-ticket', invocationId: 'keyboard-invocation', commandId: 'workspace.tab.chat.create', handoffId: 'keyboard-handoff' });
      });
      if (change === 'stable') await waitFor(() => expect(executionState.port.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('keyboard-ticket', 'keyboard-handoff'));
      else expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
    } finally { view.unmount(); root.remove(); }
  });

  it('WorkspaceToolbar mostra o prefixo efetivo e o retira quando as sequências são suprimidas', async () => {
    const { view, root } = await mountCreationIntegration({ includeToolbar: true });
    try {
      const trigger = screen.getByRole('button', { name: /workspace\.newTab/ });
      expect(trigger).toHaveAccessibleName('workspace.newTab, Ctrl+N');
      keyboardState.loadMap.mockResolvedValue({ generation: 'new-prefix', bindings: [{
        commandId: 'workspace.tab.chat.create', handler: 'contextual',
        shortcut: { version: 2, steps: [{ code: 'KeyB', modifiers: ['Control'] }, { code: 'KeyC', modifiers: [] }] },
      }] });
      await act(async () => { keyboardState.handlers.get('command:keyboard-map-changed')?.(); });
      await waitFor(() => expect(trigger).toHaveAccessibleName('workspace.newTab, Ctrl+B'));
      keyboardState.loadMap.mockResolvedValue({ generation: 'no-prefix', bindings: [] });
      await act(async () => { keyboardState.handlers.get('command:keyboard-map-changed')?.(); });
      await waitFor(() => expect(trigger).toHaveAccessibleName('workspace.newTab'));
    } finally { view.unmount(); root.remove(); }
  });

  it.each(creationCases)('Ctrl+N + %s envia exatamente a sequência v2 de %s sem Begin no prefixo', async (letter, type) => {
    const { view, root } = await mountCreationIntegration({
      includeToolbar: true,
      commandID: `workspace.tab.${type}.create`,
    });
    try {
      const trigger = screen.getByRole('button', { name: /workspace\.newTab/ });
      trigger.focus();
      await act(async () => { fireEvent.keyDown(window, { key: 'n', code: 'KeyN', ctrlKey: true }); });
      await waitFor(() => expect(commandOpenSpy).toHaveBeenCalledOnce());
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      fireEvent.keyUp(window, { key: 'n', code: 'KeyN' });

      await act(async () => { fireEvent.keyDown(window, { key: letter.toLowerCase(), code: `Key${letter}` }); });
      await waitFor(() => expect(keyboardState.beginUI).toHaveBeenCalledOnce());
      expect(keyboardState.beginUI).toHaveBeenCalledWith(
        'g-creation',
        expect.objectContaining({
          version: 2,
          steps: [
            { code: 'KeyN', modifiers: ['Control'] },
            { code: `Key${letter}`, modifiers: [] },
          ],
        }),
        false,
      );
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
      await waitFor(() => expect(executionState.port.commitBackendCommand).toHaveBeenCalledWith(
        'keyboard-ticket', 'keyboard-handoff',
      ));
      expect(type).toBeTruthy();
    } finally {
      view.unmount();
      root.remove();
    }
  });

  it('botão do WorkspaceToolbar e seleção do item usam o executor contextual, sem BeginLocal nem createWorkspaceTab', async () => {
    const { view, root } = await mountCreationIntegration({
      includeToolbar: true,
      commandID: 'workspace.tab.editor.create',
    });
    try {
      fireEvent.click(screen.getByRole('button', { name: /workspace\.newTab/ }));
      await waitFor(() => expect(catalogState.list).toHaveBeenCalledOnce());
      const lastCall = commandOpenSpy.mock.calls[commandOpenSpy.mock.calls.length - 1];
      const items = lastCall?.[2] as Array<{ action?: () => void }> | undefined;
      expect(items?.[1]?.action).toBeDefined();
      items?.[1]?.action?.();
      await waitFor(() => expect(executionState.port.beginUICommand).toHaveBeenCalled());
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(executionState.port.beginUICommand).toHaveBeenCalledWith('workspace.tab.editor.create');
    } finally {
      view.unmount();
      root.remove();
    }
  });

  it('cria workspace uma vez pelo menu contextual do picker, sem bypass do store nem troca de workspace', async () => {
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    executionState.port.beginUICommand.mockResolvedValue({
      ticket: 'workspace-menu-ticket', invocationId: 'workspace-menu-inv', commandId: 'workspace.create',
    });
    executionState.port.takeUICommand.mockResolvedValue({
      ticket: 'workspace-menu-ticket', invocationId: 'workspace-menu-inv', commandId: 'workspace.create',
      handoffId: 'workspace-menu-handoff',
    });
    executionState.port.getUICommandResult.mockResolvedValue({ invocationId: 'workspace-menu-inv', status: 'succeeded' });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    document.body.appendChild(root);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      const picker = screen.getByRole('button', { name: 'workspace.workspaceList' });
      picker.focus();
      await act(async () => {
        fireEvent.contextMenu(picker, { clientX: 24, clientY: 16 });
      });
      expect(contextMenuState.openAtPoint).toHaveBeenCalledOnce();
      const item = contextMenuState.items.find((candidate) => candidate.id === 'new-workspace');
      expect(item?.action).toBeDefined();

      await act(async () => {
        item?.action?.();
        // O hook real fecha o menu e só então entrega onAfterSelect; o mock
        // precisa preservar essa ordem para não validar um atalho síncrono.
        contextMenuState.afterSelect?.();
      });
      await waitFor(() => expect(executionState.port.commitBackendCommand)
        .toHaveBeenCalledExactlyOnceWith('workspace-menu-ticket', 'workspace-menu-handoff'));
      expect(executionState.port.beginUICommand).toHaveBeenCalledExactlyOnceWith('workspace.create');
      expect(workspaceState.createWorkspace).not.toHaveBeenCalled();
      expect(workspaceState.switchWorkspace).not.toHaveBeenCalled();
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
    } finally {
      view.unmount();
      root.remove();
    }
  });

  it('Escape cancela o menu/chord, restaura foco e não inicia execução', async () => {
    const { view, root } = await mountCreationIntegration({ includeToolbar: true });
    try {
      const trigger = screen.getByRole('button', { name: /workspace\.newTab/ });
      trigger.focus();
      fireEvent.keyDown(window, { key: 'n', code: 'KeyN', ctrlKey: true });
      await waitFor(() => expect(commandOpenSpy).toHaveBeenCalledOnce());
      fireEvent.keyUp(window, { key: 'n', code: 'KeyN' });
      fireEvent.keyDown(window, { key: 'Escape', code: 'Escape' });
      expect(document.activeElement).toBe(trigger);
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    } finally {
      view.unmount();
      root.remove();
    }
  });

  it.each(['modal', 'blur', 'owner', 'workspace', 'active-tab'] as const)(
    'cancela Ctrl+N antes da segunda tecla após mudança de contexto: %s',
    async (change) => {
      const { view, root } = await mountCreationIntegration({ includeToolbar: true });
      try {
        const trigger = screen.getByRole('button', { name: /workspace\.newTab/ });
        trigger.focus();
        await act(async () => { fireEvent.keyDown(window, { key: 'n', code: 'KeyN', ctrlKey: true }); });
        await waitFor(() => expect(commandOpenSpy).toHaveBeenCalledOnce());
        fireEvent.keyUp(window, { key: 'n', code: 'KeyN' });

        if (change === 'modal') modalState.open = true;
        if (change === 'blur') window.dispatchEvent(new Event('blur'));
        if (change === 'owner') authState.user = { ...authState.user, userId: 'user-b' };
        if (change === 'workspace') workspaceState.workspace = { ...workspaceState.workspace, id: 'workspace-b' };
        if (change === 'active-tab') workspaceState.workspace = { ...workspaceState.workspace, activeTabId: 'tab-b' };
        workspaceState.listeners.forEach((listener) => listener());

        fireEvent.keyDown(window, { key: 'c', code: 'KeyC' });
        await act(async () => { await Promise.resolve(); });
        expect(keyboardState.beginUI).not.toHaveBeenCalled();
        expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
      } finally {
        view.unmount();
        root.remove();
      }
    },
  );

  it.each(navigationKeyboardCases)('leva %s pelo teclado configurado até %s, uma vez e sem ledger', async (commandID, route, code, key) => {
    keyboardState.loadMap.mockResolvedValue({
      generation: `g-keyboard-${commandID}`,
      bindings: [{ shortcut: { version: 1, code, modifiers: ['Alt'] }, commandId: commandID, handler: 'local_ui' }],
    });
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      screen.getByRole('button', { name: 'commandPalette.title' }).focus();
      const first = new KeyboardEvent('keydown', { key, code, altKey: true, bubbles: true, cancelable: true });
      await act(async () => { window.dispatchEvent(first); });
      await waitFor(() => expect(navigateSpy).toHaveBeenCalledExactlyOnceWith(route));
      expect(first.defaultPrevented).toBe(true);

      await act(async () => {
        window.dispatchEvent(new KeyboardEvent('keydown', { key, code, altKey: true, repeat: true, bubbles: true, cancelable: true }));
      });
      expect(navigateSpy).toHaveBeenCalledExactlyOnceWith(route);
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
      expect(keyboardState.dispatch).not.toHaveBeenCalled();
    } finally {
      view.unmount();
    }
  });

  it.each([
    { name: 'failed panel', focusOutside: false },
    { name: 'toolbar', focusOutside: true },
  ])('recupera foco no rollback apenas quando o foco ainda pertence à origem: %s', async ({ focusOutside }) => {
    locationState.pathname = '/';
    setupNavigationTabs();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    const tabs = document.createElement('div');
    tabs.className = 'ws-tabs';
    for (const id of workspaceState.workspace.tabs.map((tab) => tab.id)) {
      const button = document.createElement('button');
      button.type = 'button';
      button.setAttribute('role', 'tab');
      button.dataset.tabValue = id;
      tabs.append(button);
    }
    const failedPanel = document.createElement('section');
    failedPanel.className = 'ws-content__panel';
    failedPanel.dataset.tabId = 'nav-2';
    const failedFocus = document.createElement('input');
    failedPanel.append(failedFocus);
    root.append(tabs, failedPanel);
    document.body.append(root);
    const rollbackPanel = document.createElement('section');
    rollbackPanel.tabIndex = -1;
    const rollbackFocus = vi.fn(() => { rollbackPanel.focus(); return true; });
    unregisterWorkspacePanelFocus = registerWorkspacePanelFocus('tab-a', rollbackFocus, rollbackFocus, () => true);
    const view = render(<Topbar />);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      await act(async () => {
        if (focusOutside) screen.getByRole('button', { name: 'commandPalette.title' }).focus();
        else failedFocus.focus();
        root.append(rollbackPanel);
        window.dispatchEvent(new CustomEvent('workspace:tab-activation-rollback', {
          detail: { failedTabId: 'nav-2', rollbackTabId: 'tab-a' },
        }));
      });
      if (focusOutside) {
        expect(rollbackFocus).not.toHaveBeenCalled();
      } else {
        expect(rollbackFocus).toHaveBeenCalledOnce();
        expect(document.activeElement).toBe(rollbackPanel);
      }
    } finally {
      view.unmount();
      unregisterWorkspacePanelFocus?.();
      unregisterWorkspacePanelFocus = undefined;
      root.remove();
    }
  });

  it.each([false, true])('relê a superfície da origem de Ctrl+K, alterada=%s', async (changed) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    let version = 'v1';
    const getter = () => ({ surfaceType: 'editor', surfaceId: 'source-panel', snapshotVersion: version });
    let resolve!: (items: unknown[]) => void;
    catalogState.list.mockReturnValueOnce(new Promise((done) => { resolve = done; }));
    const view = render(<CommandContextProvider><Topbar /><CommandSourcePanel getter={getter} /></CommandContextProvider>);
    screen.getByRole('button', { name: 'source control' }).focus();
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    fireEvent.keyDown(document.activeElement!, { key: 'k', code: 'KeyK', ctrlKey: true });
    await waitFor(() => expect(catalogState.list).toHaveBeenCalledTimes(1));
    if (changed) version = 'v2';
    await act(async () => { resolve([{ id: 'help.shortcuts.show', available: true }]); });
    expect(commandOpenSpy).toHaveBeenCalledTimes(changed ? 0 : 1);
    view.unmount();
    expect(authState.listeners.size).toBe(0);
    expect(workspaceState.listeners.size).toBe(0);
  });

  it('não substitui a origem registrada sem dados pela superfície da toolbar', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const getter = () => null;
    const view = render(<CommandContextProvider><Topbar /><CommandSourcePanel getter={getter} /></CommandContextProvider>);
    screen.getByRole('button', { name: 'source control' }).focus();
    fireEvent.keyDown(document.activeElement!, { key: 'k', code: 'KeyK', ctrlKey: true });
    expect(catalogState.list).not.toHaveBeenCalled();
    view.unmount();
  });

  it('não usa fallback da toolbar quando a origem fica oculta sem blur', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const getter = () => ({ surfaceType: 'editor', surfaceId: 'source-panel', snapshotVersion: 'v1' });
    const view = render(<CommandContextProvider><Topbar /><CommandSourcePanel getter={getter} /></CommandContextProvider>);
    const button = screen.getByRole('button', { name: 'source control' });
    button.focus();
    button.parentElement!.hidden = true;
    fireEvent.keyDown(button, { key: 'k', code: 'KeyK', ctrlKey: true });
    expect(catalogState.list).not.toHaveBeenCalled();
    view.unmount();
  });

  it('aguarda provider do painel real antes de iniciar Ctrl+K', () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<CommandContextProvider><Topbar /><div className="ws-content__panel"><button>loading panel</button></div></CommandContextProvider>);
    const button = screen.getByRole('button', { name: 'loading panel' });
    button.focus();
    fireEvent.keyDown(button, { key: 'k', code: 'KeyK', ctrlKey: true });
    expect(catalogState.list).not.toHaveBeenCalled();
    view.unmount();
  });
  it('renderiza titulo da página e configura item atual', () => {
    render(<Topbar />);

    expect(screen.getByRole('button', { name: 'menu.navLabelPlain' })).toBeInTheDocument();
    // On /history sub-route, the h1 shows the page title
    expect(screen.getByRole('heading', { level: 1 })).toHaveTextContent('menu.history');
    expect(screen.getByTestId('current-item')).toHaveTextContent('history');

    const items = screen.getByTestId('menu-items').textContent || '';
    expect(items).toContain('memories');
    expect(items).toContain('settings');
    expect(items).not.toContain('export-data');
    expect(items).not.toContain('import-data');
    expect(items).not.toContain('theme');
    expect(items).not.toContain('language');
  });

  it('mostra botão voltar em sub-rota', () => {
    render(<Topbar />);

    const backButton = screen.getByRole('button', { name: 'menu.backToWorkspace' });
    expect(backButton).toBeInTheDocument();

    fireEvent.click(backButton);
    expect(navigateSpy).toHaveBeenCalledWith('/');
  });

  it('não navega com Alt+Backspace quando o foco está em campo editável', () => {
    render(<Topbar />);
    navigateSpy.mockClear();

    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();

    fireEvent.keyDown(input, { key: 'Backspace', altKey: true });
    expect(navigateSpy).not.toHaveBeenCalled();

    document.body.removeChild(input);
  });

  it('abre menu com Alt+M pelo binding UI do mapa, sem listener legado', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-alt-m',
      bindings: [{ shortcut: { version: 1, code: 'KeyM', modifiers: ['Alt'] }, commandId: 'navigation.menu.open', handler: 'local_ui' }],
    });
    render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });

    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    const event = new KeyboardEvent('keydown', { key: 'm', code: 'KeyM', altKey: true, bubbles: true, cancelable: true });
    await act(async () => {
      window.dispatchEvent(event);
      await waitFor(() => expect(toggleMenuSpy).toHaveBeenCalledTimes(1));
      await new Promise<void>((resolve) => setTimeout(resolve, 0));
    });
    expect(event.defaultPrevented).toBe(true);
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
  });

  it.each([
    ['navigation.settings.open', 'KeyC', '/settings'],
    ['navigation.jobs.open', 'KeyJ', '/jobs'],
    ['navigation.menu.open', 'KeyM', null],
  ])('executa %s pelo teclado no campo de mensagem sem alterar o texto', async (commandId, code, route) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'g-text', bindings: [
      { shortcut: { version: 1, code, modifiers: ['Alt'] }, commandId, handler: 'local_ui' },
    ] });
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    const input = document.createElement('textarea');
    input.value = 'rascunho preservado';
    document.body.appendChild(input);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      input.focus();
      navigateSpy.mockClear();
      await act(async () => {
        input.dispatchEvent(new KeyboardEvent('keydown', { key: code.slice(-1).toLowerCase(), code, altKey: true, bubbles: true, cancelable: true }));
      });
      if (route) expect(navigateSpy).toHaveBeenCalledWith(route);
      else expect(toggleMenuSpy).toHaveBeenCalledTimes(1);
      expect(input.value).toBe('rascunho preservado');
    } finally { input.remove(); view.unmount(); }
  });

  it.each(['composing', '229', 'active-composition', 'modal', 'monaco', 'repeat', 'altgraph'])('bloqueia navegação pelo teclado durante %s', async (state) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'g-blocked', bindings: [
      { shortcut: { version: 1, code: 'KeyM', modifiers: ['Alt'] }, commandId: 'navigation.menu.open', handler: 'local_ui' },
    ] });
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    const wrapper = document.createElement('div');
    if (state === 'monaco') wrapper.className = 'monaco-editor';
    const input = document.createElement('textarea');
    wrapper.appendChild(input);
    document.body.appendChild(wrapper);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      input.focus();
      if (state === 'active-composition') fireEvent.compositionStart(input);
      if (state === 'modal') modalState.open = true;
      const event = new KeyboardEvent('keydown', { key: 'm', code: 'KeyM', altKey: true, bubbles: true, cancelable: true,
        isComposing: state === 'composing', keyCode: state === '229' ? 229 : 0, repeat: state === 'repeat' });
      if (state === 'altgraph') vi.spyOn(event, 'getModifierState').mockImplementation((key) => key === 'AltGraph');
      await act(async () => { input.dispatchEvent(event); });
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(toggleMenuSpy).not.toHaveBeenCalled();
    } finally { wrapper.remove(); view.unmount(); }
  });

  it.each(['fresh', 'composing', 'modal', 'monaco', 'stale-focus'])('navegação do Deck no campo de mensagem: %s', async (state) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    const wrapper = document.createElement('div');
    if (state === 'monaco') wrapper.className = 'monaco-editor';
    const input = document.createElement('textarea');
    wrapper.appendChild(input);
    document.body.appendChild(wrapper);
    try {
      input.focus();
      if (state === 'composing') fireEvent.compositionStart(input);
      if (state === 'modal') modalState.open = true;
      await waitFor(() => expect(keyboardState.handlers.has('command:deck-local-ui')).toBe(true));
      await act(async () => {
        keyboardState.handlers.get('command:deck-local-ui')?.({ commandId: 'navigation.menu.open', generation: 'g1', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' });
      });
      if (state === 'fresh') {
        await waitFor(() => expect(toggleMenuSpy).toHaveBeenCalledTimes(1));
      } else if (state !== 'stale-focus') {
        expect(toggleMenuSpy).not.toHaveBeenCalled();
      }
      if (state === 'stale-focus') expect(toggleMenuSpy).toHaveBeenCalledTimes(1);
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    } finally { wrapper.remove(); view.unmount(); }
  });

  it('consome reservation do Stream Deck sem BeginUICommand e usa o mesmo handler', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });

    await act(async () => {
      keyboardState.handlers.get('command:deck-local-ui')?.({
        commandId: 'navigation.menu.open', generation: 'g1', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      });
      await waitFor(() => expect(toggleMenuSpy).toHaveBeenCalledTimes(1));
      await new Promise<void>((resolve) => setTimeout(resolve, 0));
    });

    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
  });

  it.each([
    ['navigation.data.export.open', '/settings/data?action=export'],
    ['navigation.data.import.open', '/settings/data?action=import'],
  ] as const)('executa %s pelo evento local do Deck uma única vez', async (commandId, route) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    await waitFor(() => expect(keyboardState.handlers.has('command:deck-local-ui')).toBe(true));
    await act(async () => {
      keyboardState.handlers.get('command:deck-local-ui')?.({
        commandId, generation: 'g1', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      });
    });
    await waitFor(() => expect(navigateSpy).toHaveBeenCalledExactlyOnceWith(route));
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
    expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
    view.unmount();
  });

  it.each(navigationRouteCases)('leva %s pelo evento local do Deck até %s sem BeginUICommand', async (commandID, route) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      screen.getByRole('button', { name: 'commandPalette.title' }).focus();
      await waitFor(() => expect(keyboardState.handlers.has('command:deck-local-ui')).toBe(true));
      await act(async () => {
        keyboardState.handlers.get('command:deck-local-ui')?.({
          commandId: commandID, generation: 'g1', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
        });
      });
      await waitFor(() => expect(navigateSpy).toHaveBeenCalledExactlyOnceWith(route));
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
      expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
    } finally {
      view.unmount();
    }
  });

  it.each(['modal', 'generation', 'owner', 'workspace', 'blur'] as const)('recusa navigation.about.open do Deck com contexto %s', async (guard) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      screen.getByRole('button', { name: 'commandPalette.title' }).focus();
      await waitFor(() => expect(keyboardState.handlers.has('command:deck-local-ui')).toBe(true));
      if (guard === 'modal') modalState.open = true;
      if (guard === 'blur') vi.mocked(document.hasFocus).mockReturnValue(false);
      await act(async () => {
        keyboardState.handlers.get('command:deck-local-ui')?.({
          commandId: 'navigation.about.open',
          generation: guard === 'generation' ? 'stale-generation' : 'g1',
          userId: guard === 'owner' ? 'other-user' : 'user-a',
          sessionId: 'session-a',
          workspaceId: guard === 'workspace' ? 'other-workspace' : 'workspace-a',
        });
      });

      expect(navigateSpy).not.toHaveBeenCalled();
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    } finally {
      modalState.open = false;
      view.unmount();
    }
  });

  it('executa navigation.palette.open pelo Deck local sem ledger e abre a paleta uma vez', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    catalogState.list.mockResolvedValueOnce([{ id: 'help.shortcuts.show', name: 'Atalhos', available: true }]);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    await waitFor(() => expect(keyboardState.handlers.has('command:deck-local-ui')).toBe(true));
    await act(async () => {
      keyboardState.handlers.get('command:deck-local-ui')?.({
        commandId: 'navigation.palette.open', generation: 'g1', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      });
    });
    await waitFor(() => expect(catalogState.list).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(commandOpenSpy).toHaveBeenCalledTimes(1));
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
    view.unmount();
  });

  it('executa workspace.panel.focus pelo teclado somente no painel contextual ativo', async () => {
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-panel',
      bindings: [{ shortcut: { version: 1, code: 'KeyP', modifiers: ['Alt'] }, commandId: 'workspace.panel.focus', handler: 'local_ui' }],
    });
    keyboardState.beginUI.mockResolvedValue({ ticket: 'panel-ticket', invocationId: 'panel-inv', commandId: 'workspace.panel.focus' });
    executionState.port.takeUICommand.mockResolvedValue({ ticket: 'panel-ticket', invocationId: 'panel-inv', commandId: 'workspace.panel.focus', handoffId: 'panel-handoff' });
    executionState.port.getUICommandResult.mockResolvedValue({ invocationId: 'panel-inv', status: 'succeeded' });
    render(<><div className="ws-content__panel" tabIndex={-1} /><Topbar /></>);
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });

    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    await act(async () => { await Promise.resolve(); });
    const event = new KeyboardEvent('keydown', { key: 'p', code: 'KeyP', altKey: true, bubbles: true, cancelable: true });
    await act(async () => { window.dispatchEvent(event); await Promise.resolve(); });

    await waitFor(() => expect(workspacePanelState.focus).toHaveBeenCalledTimes(1));
    expect(document.activeElement).toBe(document.querySelector('.ws-content__panel'));
    expect(event.defaultPrevented).toBe(true);
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
  });

  it('não rouba o atalho global workspace.panel.focus fora da rota de workspace', async () => {
    locationState.pathname = '/history';
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-panel',
      bindings: [{ shortcut: { version: 1, code: 'KeyP', modifiers: ['Alt'] }, commandId: 'workspace.panel.focus', handler: 'local_ui' }],
    });
    render(<CommandContextProvider><Topbar /></CommandContextProvider>);

    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    const event = new KeyboardEvent('keydown', { key: 'p', code: 'KeyP', altKey: true, bubbles: true, cancelable: true });
    window.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    expect(workspacePanelState.focus).not.toHaveBeenCalled();
  });

  it('desabilita workspace.panel.focus na paleta fora do contexto suportado', async () => {
    locationState.pathname = '/history';
    catalogState.list.mockResolvedValueOnce([{ id: 'workspace.panel.focus', name: 'Focar painel', available: true }]);
    const user = userEvent.setup();
    render(<Topbar />);

    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await waitFor(() => expect(commandOpenSpy).toHaveBeenCalledTimes(1));
    const item = (commandOpenSpy.mock.calls[0]?.[2] as Array<{ disabled?: boolean }>)[0];
    expect(item.disabled).toBe(true);
  });

  it('rejeita workspace.panel.focus do Deck quando o painel já mudou', async () => {
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const panel = document.createElement('div');
    panel.className = 'ws-content__panel';
    panel.tabIndex = -1;
    document.body.appendChild(panel);
    render(<Topbar />);
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });

    await act(async () => {
      await waitFor(() => expect(keyboardState.handlers.has('command:deck-local-ui')).toBe(true));
    });
    workspacePanelState.current = false;
    await act(async () => {
      keyboardState.handlers.get('command:deck-local-ui')?.({ commandId: 'workspace.panel.focus', generation: 'g1', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' });
      await Promise.resolve();
    });

    expect(workspacePanelState.focus).not.toHaveBeenCalled();
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
    panel.remove();
  });

  it('executa workspace.panel.focus pela paleta com o helper e registry reais', async () => {
    locationState.pathname = '/';
    workspacePanelState.useReal = true;
    const panel = document.createElement('div');
    panel.className = 'ws-content__panel';
    panel.dataset.tabId = 'tab-a';
    panel.dataset.tabType = 'chat';
    panel.dataset.active = 'true';
    const input = document.createElement('input');
    panel.appendChild(input);
    document.body.appendChild(panel);
    const immediateFocus = () => { input.focus(); return true; };
    unregisterWorkspacePanelFocus = registerWorkspacePanelFocus('tab-a', immediateFocus, immediateFocus);
    catalogState.list.mockResolvedValueOnce([{ id: 'workspace.panel.focus', name: 'Focar painel', available: true }]);
    const user = userEvent.setup();
    render(<Topbar />);

    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await waitFor(() => expect(commandOpenSpy).toHaveBeenCalledTimes(1));
    const item = (commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void; disabled?: boolean }>)[0];
    expect(item.disabled).toBe(false);
    await act(async () => {
      item.action?.();
      menuHookState.afterSelect?.();
      await Promise.resolve();
    });
    expect(document.activeElement).toBe(input);
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    panel.remove();
  });

  it('rejeita reservation de workspace.panel.focus do Deck fora da rota global', async () => {
    locationState.pathname = '/history';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    workspacePanelState.useReal = true;
    render(<Topbar />);
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });

    await act(async () => {
      await waitFor(() => expect(keyboardState.handlers.has('command:deck-ui-reservation')).toBe(true));
      keyboardState.handlers.get('command:deck-ui-reservation')?.({
        ticket: 'outside-panel-ticket', invocationId: 'outside-panel-inv', commandId: 'workspace.panel.focus',
      });
      await Promise.resolve();
    });
    expect(executionState.port.cancelUICommand).toHaveBeenCalledWith('outside-panel-ticket');
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
  });

  it('cancela reservation real quando a aba do painel muda antes do Take', async () => {
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    workspacePanelState.useReal = true;
    const panel = document.createElement('div');
    panel.className = 'ws-content__panel';
    panel.dataset.tabId = 'tab-a';
    panel.dataset.tabType = 'chat';
    panel.dataset.active = 'true';
    const input = document.createElement('input');
    panel.appendChild(input);
    document.body.appendChild(panel);
    const immediateFocus = () => { input.focus(); return true; };
    unregisterWorkspacePanelFocus = registerWorkspacePanelFocus('tab-a', immediateFocus, immediateFocus);
    render(<Topbar />);
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });

    await act(async () => {
      await waitFor(() => expect(keyboardState.handlers.has('command:deck-local-ui')).toBe(true));
    });
    workspaceState.workspace = {
      ...workspaceState.workspace,
      activeTabId: 'tab-b',
      tabs: [{ id: 'tab-b', type: 'chat' as const }],
    };
    workspaceState.listeners.forEach((listener) => listener());
    panel.dataset.active = 'false';
    await act(async () => {
      keyboardState.handlers.get('command:deck-local-ui')?.({ commandId: 'workspace.panel.focus', generation: 'g1', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' });
      await Promise.resolve();
    });

    expect(document.activeElement).not.toBe(input);
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
    panel.remove();
  });

  it.each([
    ['workspace.create', 'workspace'],
    ['workspace.tab.chat.create', 'chat'],
    ['workspace.tab.editor.create', 'editor'],
    ['workspace.tab.tasklist.create', 'tasklist'],
    ['workspace.tab.terminal.create', 'terminal'],
    ['workspace.tab.close', 'close'],
    ...WORKSPACE_TAB_NAVIGATION_COMMAND_IDS.map((id) => [id, id] as const),
  ] as const)('roteia binding configurado para %s pela política correta em campo editável', async (commandId, label) => {
    if (isWorkspaceTabNavigationCommand(commandId)) setupNavigationTabs();
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const goToArguments = { workspace_id: 'workspace-a', target_mode: 'position', position: 2 };
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-chat-create',
      bindings: [{ shortcut: { version: 1, code: 'KeyT', modifiers: ['Control'] }, commandId,
        handler: isWorkspaceTabNavigationCommand(commandId) ? 'local_ui' : 'contextual',
        ...(commandId === 'workspace.tab.go_to' ? { arguments: goToArguments } : {}) }],
    });
    keyboardState.beginUI.mockResolvedValue({ ticket: `${label}-ticket`, invocationId: `${label}-inv`, commandId });
    executionState.port.takeUICommand.mockResolvedValue({ ticket: `${label}-ticket`, invocationId: `${label}-inv`, commandId, handoffId: `${label}-handoff` });
    executionState.port.getUICommandResult.mockResolvedValue({ invocationId: `${label}-inv`, status: 'succeeded' });
    const input = document.createElement('textarea');
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    root.appendChild(input);
    document.body.appendChild(root);
    render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    input.focus();
    const targetLease = captureWorkspaceTabTarget(() => locationState.pathname);
    expect(targetLease).toBeDefined();
    targetLease?.dispose();

    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    await act(async () => {
      await new Promise<void>((resolve) => setTimeout(resolve, 0));
      const event = new KeyboardEvent('keydown', { key: 't', code: 'KeyT', ctrlKey: true, bubbles: true, cancelable: true });
      input.dispatchEvent(event);
      expect(event.defaultPrevented).toBe(true);
      if (isWorkspaceTabNavigationCommand(commandId)) {
        expect(workspaceState.setActiveTab).toHaveBeenCalledExactlyOnceWith(navigationTargetId(commandId));
      } else {
        await waitFor(() => expect(keyboardState.beginUI).toHaveBeenCalled());
        await waitFor(() => expect(executionState.port.takeUICommand).toHaveBeenCalledWith(`${label}-ticket`));
        await waitFor(() => expect(executionState.port.commitBackendCommand).toHaveBeenCalledWith(`${label}-ticket`, `${label}-handoff`));
      }
    });
    if (isWorkspaceTabNavigationCommand(commandId)) {
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
      expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
      expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
      expect(executionState.port.getUICommandResult).not.toHaveBeenCalled();
    } else {
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
      expect(executionState.port.completeUICommand).not.toHaveBeenCalledWith(`${label}-ticket`, `${label}-handoff`, 'succeeded');
      expect(executionState.port.takeUICommand).toHaveBeenCalledWith(`${label}-ticket`);
    }
    root.remove();
  });

  it.each([
    'workspace.tab.chat.create', 'workspace.tab.editor.create',
    'workspace.tab.tasklist.create', 'workspace.tab.terminal.create', 'workspace.tab.close',
  ])('aguarda persistência e cancela %s após falha, Escape ou mudança de aba', async (commandId) => {
    for (const outcome of ['failed', 'changed', 'cancelled', 'confirmed'] as const) {
      locationState.pathname = '/';
      workspaceState.workspace = { ...workspaceState.workspace, activeTabId: 'tab-a', tabs: [
        { id: 'tab-a', type: 'chat' }, { id: 'tab-b', type: 'chat' },
      ] };
      keyboardState.loadMap.mockResolvedValue({ generation: `barrier-${outcome}`, bindings: [
        { shortcut: { version: 1, code: 'KeyT', modifiers: ['Control'] }, commandId, handler: 'contextual' },
      ] });
      keyboardState.beginUI.mockClear().mockResolvedValue({ ticket: 'barrier', invocationId: 'barrier-inv', commandId });
      executionState.port.takeUICommand.mockClear().mockResolvedValue({ ticket: 'barrier', invocationId: 'barrier-inv', commandId, handoffId: 'barrier-hand' });
      executionState.port.commitBackendCommand.mockClear();
      executionState.port.getUICommandResult.mockResolvedValue({ invocationId: 'barrier-inv', status: 'succeeded' });
      let release!: (value: boolean) => void;
      vi.mocked(flushWorkspaceNavigation).mockClear().mockImplementationOnce(() => new Promise<boolean>((resolve) => { release = resolve; }));
      vi.spyOn(document, 'hasFocus').mockReturnValue(true);
      const root = document.createElement('div');
      root.className = 'workspace-layout';
      const input = document.createElement('textarea');
      root.append(input);
      document.body.append(root);
      const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
      try {
        await act(async () => { await Promise.resolve(); });
        input.focus();
        await act(async () => { input.dispatchEvent(new KeyboardEvent('keydown', {
          key: 't', code: 'KeyT', ctrlKey: true, bubbles: true, cancelable: true,
        })); });
        await waitFor(() => expect(flushWorkspaceNavigation).toHaveBeenCalledOnce());
        expect(keyboardState.beginUI).not.toHaveBeenCalled();
        expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
        if (outcome === 'changed') {
          workspaceState.workspace = { ...workspaceState.workspace, activeTabId: 'tab-b' };
          workspaceState.listeners.forEach((listener) => listener());
        }
        if (outcome === 'cancelled') {
          fireEvent.keyDown(input, { key: 'Escape', code: 'Escape' });
        }
        await act(async () => { release(outcome !== 'failed'); });
        if (outcome === 'confirmed') {
          await waitFor(() => expect(executionState.port.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('barrier', 'barrier-hand'));
        } else {
          expect(keyboardState.beginUI).not.toHaveBeenCalled();
          expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
          expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
        }
      } finally {
        view.unmount();
        root.remove();
        vi.mocked(flushWorkspaceNavigation).mockReset().mockResolvedValue(true);
      }
    }
  });

  it('executa navegação de aba do Deck pelo evento local versionado, sem reservation', async () => {
    setupNavigationTabs();
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'g-tabdeck', bindings: [] });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    const panel = document.createElement('div');
    panel.className = 'ws-content__panel';
    panel.dataset.tabId = 'tab-a';
    panel.dataset.tabType = 'chat';
    panel.dataset.active = 'true';
    const tab = document.createElement('button');
    tab.setAttribute('role', 'tab');
    tab.dataset.tabValue = 'nav-2';
    root.append(panel, tab);
    document.body.appendChild(root);
    render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    await waitFor(() => expect(keyboardState.handlers.has('command:deck-local-ui')).toBe(true));
    keyboardState.handlers.get('command:deck-local-ui')?.({
      commandId: 'workspace.tab.next', generation: 'g-tabdeck', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
    });
    await waitFor(() => expect(workspaceState.setActiveTab).toHaveBeenCalledWith('nav-2'));
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
    expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
    root.remove();
  });

  it.each([
    ['w', 'KeyW'], ['F4', 'F4'],
  ])('fecha aba por Ctrl+%s no protocolo contextual sem confirmação visual de sucesso', async (key, code) => {
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'g-close', bindings: [
      { shortcut: { version: 1, code, modifiers: ['Control'] }, commandId: 'workspace.tab.close', handler: 'contextual' },
    ] });
    keyboardState.beginUI.mockResolvedValue({ ticket: 'close-ticket', invocationId: 'close-inv', commandId: 'workspace.tab.close' });
    executionState.port.takeUICommand.mockResolvedValue({ ticket: 'close-ticket', invocationId: 'close-inv', commandId: 'workspace.tab.close', handoffId: 'close-handoff' });
    executionState.port.getUICommandResult.mockResolvedValue({ invocationId: 'close-inv', status: 'succeeded' });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    const input = document.createElement('textarea');
    root.appendChild(input);
    document.body.appendChild(root);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      input.focus();
      const event = new KeyboardEvent('keydown', { key, code, ctrlKey: true, bubbles: true, cancelable: true });
      await act(async () => { input.dispatchEvent(event); });
      expect(event.defaultPrevented).toBe(true);
      await waitFor(() => expect(executionState.port.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('close-ticket', 'close-handoff'));
      expect(executionState.port.completeUICommand).not.toHaveBeenCalledWith('close-ticket', 'close-handoff', 'succeeded');
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    } finally { root.remove(); view.unmount(); }
  });

  it('restaura foco na sucessora somente depois da confirmação real de fechamento', async () => {
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    workspaceState.workspace.tabs.push({ id: 'tab-b', type: 'chat' });
    keyboardState.loadMap.mockResolvedValue({ generation: 'g-close-focus', bindings: [
      { shortcut: { version: 1, code: 'KeyW', modifiers: ['Control'] }, commandId: 'workspace.tab.close', handler: 'contextual' },
    ] });
    keyboardState.beginUI.mockResolvedValue({ ticket: 'close-focus', invocationId: 'close-focus-inv', commandId: 'workspace.tab.close' });
    executionState.port.takeUICommand.mockResolvedValue({ ticket: 'close-focus', invocationId: 'close-focus-inv', commandId: 'workspace.tab.close', handoffId: 'close-focus-hand' });
    executionState.port.getUICommandResult.mockResolvedValue({ invocationId: 'close-focus-inv', status: 'succeeded' });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    const source = document.createElement('div');
    source.className = 'ws-content__panel';
    source.dataset.tabId = 'tab-a';
    source.dataset.tabType = 'chat';
    source.dataset.active = 'true';
    const input = document.createElement('textarea');
    source.appendChild(input);
    const successor = document.createElement('button');
    successor.setAttribute('role', 'tab');
    successor.dataset.tabValue = 'tab-b';
    const tablist = document.createElement('div');
    tablist.className = 'ws-tabs';
    tablist.append(successor);
    root.append(source, tablist);
    document.body.appendChild(root);
    let finish!: () => void;
    executionState.port.commitBackendCommand.mockImplementationOnce(() => {
      workspaceState.workspace = { ...workspaceState.workspace, tabs: [{ id: 'tab-b', type: 'chat' }], activeTabId: 'tab-b' };
      workspaceState.listeners.forEach((listener) => listener());
      source.remove();
      return new Promise<void>((resolve) => { finish = resolve; });
    });
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      input.focus();
      await act(async () => {
        input.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', code: 'KeyW', ctrlKey: true, bubbles: true, cancelable: true }));
      });
      await waitFor(() => expect(executionState.port.commitBackendCommand).toHaveBeenCalled());
      expect(document.activeElement).not.toBe(successor);
      await act(async () => { finish(); });
      await waitFor(() => expect(document.activeElement).toBe(successor));
      expect(announceSpy).toHaveBeenCalledWith(expect.any(String));
    } finally { root.remove(); view.unmount(); }
  });

  it('continua navegando por teclado após a página anterior perder seu controle focado', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'route-focus-transition', bindings: [
      { shortcut: { version: 1, code: 'KeyC', modifiers: ['Alt'] }, commandId: 'navigation.settings.open', handler: 'local_ui' },
      { shortcut: { version: 1, code: 'KeyJ', modifiers: ['Alt'] }, commandId: 'navigation.jobs.open', handler: 'local_ui' },
      { shortcut: { version: 1, code: 'KeyH', modifiers: ['Alt'] }, commandId: 'navigation.history.open', handler: 'local_ui' },
    ] });
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await act(async () => { await Promise.resolve(); });
      await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });
      for (const [code, route] of [['KeyC', '/settings'], ['KeyJ', '/jobs'], ['KeyH', '/history']]) {
        await act(async () => {
          document.activeElement!.dispatchEvent(new KeyboardEvent('keydown', { code, key: code.slice(-1).toLowerCase(), altKey: true, bubbles: true, cancelable: true }));
          document.activeElement!.dispatchEvent(new KeyboardEvent('keyup', { code, bubbles: true }));
        });
        expect(navigateSpy).toHaveBeenLastCalledWith(route);
        await act(async () => {
          (document.activeElement as HTMLElement).blur();
          locationState.pathname = route;
          locationState.key = route;
          view.rerender(<CommandContextProvider><Topbar /></CommandContextProvider>);
        });
        expect(document.activeElement).toBe(document.body);
      }
      expect(navigateSpy).toHaveBeenCalledTimes(3);
      expect(keyboardState.dispatch).not.toHaveBeenCalled();
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); }
  });

  it('mantém Deck e Ctrl+Tab operantes entre abas quando o painel anterior perde o foco', async () => {
    setupNavigationTabs();
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'deck-tab-focus-transition', bindings: [
      { shortcut: { version: 1, code: 'Tab', modifiers: ['Control'] }, commandId: 'workspace.tab.next', handler: 'local_ui' },
    ] });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    root.innerHTML = '<div class="ws-content__panel" data-tab-id="tab-a"><textarea></textarea></div>';
    document.body.append(root);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    const deck = (commandId: string) => keyboardState.handlers.get('command:deck-local-ui')?.({
      commandId, generation: 'deck-tab-focus-transition', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
    });
    try {
      await act(async () => { await Promise.resolve(); });
      root.querySelector('textarea')!.focus();
      await act(async () => { deck('workspace.tab.second'); });
      expect(workspaceState.workspace.activeTabId).toBe('nav-2');
      // WorkspaceContent blurs the old panel when it becomes hidden. The next
      // command must not require a window blur/focus or a map reload to work.
      await act(async () => {
        (root.firstElementChild as HTMLElement).hidden = true;
        root.querySelector('textarea')!.blur();
      });
      expect(document.activeElement).toBe(document.body);
      await act(async () => { deck('workspace.tab.first'); });
      expect(workspaceState.workspace.activeTabId).toBe('tab-a');
      await act(async () => {
        document.body.dispatchEvent(new KeyboardEvent('keydown', { key: 'Tab', code: 'Tab', ctrlKey: true, bubbles: true, cancelable: true }));
        document.body.dispatchEvent(new KeyboardEvent('keyup', { key: 'Tab', code: 'Tab', bubbles: true }));
      });
      expect(workspaceState.workspace.activeTabId).toBe('nav-2');
      await act(async () => { deck('workspace.tab.first'); deck('workspace.tab.second'); });
      expect(workspaceState.workspace.activeTabId).toBe('nav-2');
      expect(workspaceState.setActiveTab).toHaveBeenCalledTimes(5);
      expect(keyboardState.dispatch).not.toHaveBeenCalled();
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    } finally { view.unmount(); root.remove(); }
  });

  it.each(['modal', 'window-unfocused', 'session', 'generation', 'expired'] as const)(
    'navegação do Deck com foco no documento preserva a barreira %s', async (barrier) => {
      const now = Date.now();
      const focused = vi.spyOn(document, 'hasFocus').mockReturnValue(true);
      keyboardState.loadMap.mockResolvedValue({ generation: 'body-guard', validUntil: now + 60_000, bindings: [] });
      const view = render(<Topbar />);
      const payload = { commandId: 'navigation.settings.open', generation: 'body-guard', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' };
      try {
        await act(async () => { await Promise.resolve(); });
        expect(document.activeElement).toBe(document.body);
        await act(async () => { keyboardState.handlers.get('command:deck-local-ui')?.(payload); });
        expect(navigateSpy).toHaveBeenCalledExactlyOnceWith('/settings');
        navigateSpy.mockClear();
        if (barrier === 'modal') modalState.open = true;
        if (barrier === 'window-unfocused') focused.mockReturnValue(false);
        if (barrier === 'session') payload.sessionId = 'other-session';
        if (barrier === 'generation') payload.generation = 'old-map';
        if (barrier === 'expired') vi.spyOn(Date, 'now').mockReturnValue(now + 60_001);
        await act(async () => { keyboardState.handlers.get('command:deck-local-ui')?.(payload); });
        expect(navigateSpy).not.toHaveBeenCalled();
        expect(keyboardState.dispatch).not.toHaveBeenCalled();
        expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
      } finally { modalState.open = false; view.unmount(); }
    },
  );

  it('foca o destino da navegação após a atualização local imediata', async () => {
    setupNavigationTabs();
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const commandId = 'workspace.tab.next';
    keyboardState.loadMap.mockResolvedValue({ generation: 'nav-focus', bindings: [
      { shortcut: { version: 1, code: 'KeyL', modifiers: ['Control'] }, commandId, handler: 'local_ui' },
    ] });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    const source = document.createElement('div');
    source.className = 'ws-content__panel';
    source.dataset.tabId = 'tab-a';
    const input = document.createElement('textarea');
    source.appendChild(input);
    const tablist = document.createElement('div');
    tablist.className = 'ws-tabs';
    const destination = document.createElement('button');
    destination.setAttribute('role', 'tab');
    destination.dataset.tabValue = 'nav-2';
    tablist.appendChild(destination);
    root.append(source, tablist);
    document.body.appendChild(root);
    const focusDestination = vi.fn(() => { destination.focus(); return true; });
    const unregisterDestination = registerWorkspacePanelFocus('nav-2', focusDestination, focusDestination, () => true);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      input.focus();
      await act(async () => {
        input.dispatchEvent(new KeyboardEvent('keydown', { key: 'l', code: 'KeyL', ctrlKey: true, bubbles: true, cancelable: true }));
      });
      await waitFor(() => expect(document.activeElement).toBe(destination));
      expect(workspaceState.setActiveTab).toHaveBeenCalledWith('nav-2');
      expect(focusDestination).toHaveBeenCalledOnce();
      expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
    } finally { unregisterDestination(); root.remove(); view.unmount(); }
  });

  it.each(['nested', 'datagrid', 'modal', 'ime'])('preserva o contexto %s em binding pessoal de navegação', async (kind) => {
    setupNavigationTabs();
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'nav-guard', bindings: [
      { shortcut: { version: 1, code: 'KeyL', modifiers: ['Control'] }, commandId: 'workspace.tab.next', handler: 'local_ui' },
    ] });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    const container = document.createElement('div');
    if (kind === 'nested') container.setAttribute('data-tab-scope', '');
    if (kind === 'datagrid') container.className = 'datagrid-container';
    if (kind === 'monaco') container.className = 'monaco-editor';
    const input = document.createElement('textarea');
    container.append(input);
    root.append(container);
    document.body.append(root);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      input.focus();
      modalState.open = kind === 'modal';
      const event = new KeyboardEvent('keydown', { key: 'l', code: 'KeyL', ctrlKey: true,
        isComposing: kind === 'ime', bubbles: true, cancelable: true });
      input.dispatchEvent(event);
      expect(event.defaultPrevented).toBe(false);
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
      expect(workspaceState.setActiveTab).not.toHaveBeenCalled();
      // Positive control: the same loaded binding works when the competing
      // control/modal/composition is gone, so absence of a map cannot pass this test.
      container.removeAttribute('data-tab-scope');
      container.className = '';
      modalState.open = false;
      input.dispatchEvent(new CompositionEvent('compositionend', { bubbles: true }));
      const allowed = new KeyboardEvent('keydown', { key: 'l', code: 'KeyL', ctrlKey: true, bubbles: true, cancelable: true });
      input.dispatchEvent(allowed);
      expect(allowed.defaultPrevented).toBe(true);
      expect(workspaceState.setActiveTab).toHaveBeenCalledExactlyOnceWith('nav-2');
    } finally { modalState.open = false; root.remove(); view.unmount(); }
  });

  it('permite Ctrl+Shift+Tab em Monaco com IME unknown após troca de foco e mantém guardas IME/modal', async () => {
    setupNavigationTabs();
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'monaco-tab-nav', bindings: [
      { shortcut: { version: 1, code: 'Tab', modifiers: ['Control', 'Shift'] }, commandId: 'workspace.tab.previous', handler: 'local_ui' },
    ] });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    const panel = document.createElement('div');
    panel.className = 'ws-content__panel';
    panel.dataset.tabId = 'tab-a';
    const previousFocus = document.createElement('textarea');
    const monaco = document.createElement('div');
    monaco.className = 'monaco-editor';
    const input = document.createElement('div');
    input.className = 'native-edit-context';
    input.setAttribute('contenteditable', 'true');
    input.tabIndex = 0;
    monaco.append(input);
    panel.append(previousFocus, monaco);
    root.append(panel);
    document.body.append(root);
    const focusPanel = () => { input.focus(); return true; };
    const unregisterFocus = registerWorkspacePanelFocus('tab-a', focusPanel, focusPanel, () => true);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      previousFocus.focus();
      input.focus();
      expect(ReadFocusContext().composition).toBe('unknown');
      modalState.open = false;
      const allowed = new KeyboardEvent('keydown', { key: 'Tab', code: 'Tab', ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true });
      input.dispatchEvent(allowed);
      expect(allowed.defaultPrevented).toBe(true);
      await waitFor(() => expect(workspaceState.setActiveTab).toHaveBeenCalledExactlyOnceWith('nav-9'));

      modalState.open = true;
      const blocked = new KeyboardEvent('keydown', { key: 'Tab', code: 'Tab', ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true });
      input.dispatchEvent(blocked);
      expect(blocked.defaultPrevented).toBe(false);
      expect(workspaceState.setActiveTab).toHaveBeenCalledTimes(1);

      modalState.open = false;
      input.dispatchEvent(new CompositionEvent('compositionstart', { bubbles: true }));
      expect(ReadFocusContext().composition).toBe('active');
      const activeComposition = new KeyboardEvent('keydown', { key: 'Tab', code: 'Tab', ctrlKey: true, shiftKey: true, bubbles: true, cancelable: true });
      input.dispatchEvent(activeComposition);
      expect(activeComposition.defaultPrevented).toBe(false);
      const composing = new KeyboardEvent('keydown', { key: 'Tab', code: 'Tab', ctrlKey: true, shiftKey: true, isComposing: true, bubbles: true, cancelable: true });
      input.dispatchEvent(composing);
      expect(composing.defaultPrevented).toBe(false);
      const ime229 = new KeyboardEvent('keydown', { key: 'Tab', code: 'Tab', ctrlKey: true, shiftKey: true, keyCode: 229, bubbles: true, cancelable: true });
      input.dispatchEvent(ime229);
      expect(ime229.defaultPrevented).toBe(false);
      expect(workspaceState.setActiveTab).toHaveBeenCalledTimes(1);
      expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
    } finally { modalState.open = false; unregisterFocus(); root.remove(); view.unmount(); }
  });

  it('no Monaco, respeita remapeamento/supressão e não amplia para rich editable', async () => {
    setupNavigationTabs();
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'monaco-tab-remap', bindings: [
      { shortcut: { version: 1, code: 'PageDown', modifiers: ['Control'] }, commandId: 'workspace.tab.next', handler: 'local_ui' },
    ] });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    const panel = document.createElement('div');
    panel.className = 'ws-content__panel';
    panel.dataset.tabId = 'tab-a';
    const monaco = document.createElement('div');
    monaco.className = 'monaco-editor';
    const input = document.createElement('div');
    input.className = 'native-edit-context';
    input.setAttribute('contenteditable', 'true');
    input.tabIndex = 0;
    monaco.append(input);
    panel.append(monaco);
    const rich = document.createElement('div');
    rich.className = 'rich-text-editor';
    rich.setAttribute('contenteditable', 'true');
    const richInput = document.createElement('textarea');
    rich.append(richInput);
    root.append(panel, rich);
    document.body.append(root);
    const focusPanel = () => { input.focus(); return true; };
    const unregisterFocus = registerWorkspacePanelFocus('tab-a', focusPanel, focusPanel, () => true);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      input.focus();
      const suppressed = new KeyboardEvent('keydown', { key: 'Tab', code: 'Tab', ctrlKey: true, bubbles: true, cancelable: true });
      input.dispatchEvent(suppressed);
      expect(suppressed.defaultPrevented).toBe(false);
      expect(workspaceState.setActiveTab).not.toHaveBeenCalled();

      const remapped = new KeyboardEvent('keydown', { key: 'PageDown', code: 'PageDown', ctrlKey: true, bubbles: true, cancelable: true });
      input.dispatchEvent(remapped);
      expect(remapped.defaultPrevented).toBe(true);
      await waitFor(() => expect(workspaceState.setActiveTab).toHaveBeenCalledExactlyOnceWith('nav-2'));

      workspaceState.setActiveTab.mockClear();
      richInput.focus();
      const richDenied = new KeyboardEvent('keydown', { key: 'PageDown', code: 'PageDown', ctrlKey: true, bubbles: true, cancelable: true });
      richInput.dispatchEvent(richDenied);
      expect(richDenied.defaultPrevented).toBe(false);
      expect(workspaceState.setActiveTab).not.toHaveBeenCalled();
      expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
    } finally { unregisterFocus(); root.remove(); view.unmount(); }
  });

  it('executa bursts e repetição de Ctrl+Tab, PageUp/Down e posições sem esperar render ou IPC', async () => {
    setupNavigationTabs();
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const entries = [
      ['Tab', false, 'workspace.tab.next'], ['Tab', true, 'workspace.tab.previous'],
      ['PageDown', false, 'workspace.tab.next'], ['PageUp', false, 'workspace.tab.previous'],
      ['Digit9', false, 'workspace.tab.ninth'], ['Digit1', false, 'workspace.tab.first'],
    ] as const;
    keyboardState.loadMap.mockResolvedValue({ generation: 'burst', bindings: entries.map(([code, shift, commandId]) => ({
      shortcut: { version: 1, code, modifiers: shift ? ['Control', 'Shift'] : ['Control'] }, commandId, handler: 'local_ui',
    })) });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    const input = document.createElement('textarea');
    root.append(input);
    document.body.append(root);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await act(async () => { await Promise.resolve(); });
      input.focus();
      const down = (code: string, shiftKey = false, repeat = false) => input.dispatchEvent(new KeyboardEvent('keydown', {
        key: code, code, ctrlKey: true, shiftKey, repeat, bubbles: true, cancelable: true,
      }));
      const up = (code: string) => input.dispatchEvent(new KeyboardEvent('keyup', { key: code, code, bubbles: true }));
      act(() => {
        down('Tab'); down('Tab', false, true); down('Tab', false, true); up('Tab');
        down('Tab', true); up('Tab');
        down('PageDown'); up('PageDown');
        down('PageUp'); up('PageUp');
        down('Digit9'); up('Digit9');
        down('Digit1'); up('Digit1');
      });
      expect(workspaceState.setActiveTab.mock.calls.map(([id]) => id)).toEqual([
        'nav-2', 'nav-3', 'nav-4', 'nav-3', 'nav-4', 'nav-3', 'nav-9', 'tab-a',
      ]);
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(keyboardState.dispatch).not.toHaveBeenCalled();
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
      expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
      expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
      expect(executionState.port.getUICommandResult).not.toHaveBeenCalled();
    } finally { view.unmount(); root.remove(); }
  });

  it.each(['w', 'F4'])('não usa fallback de fechamento quando Ctrl+%s não está no mapa', async (key) => {
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'g-empty-close', bindings: [] });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    document.body.appendChild(root);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      const event = new KeyboardEvent('keydown', { key, code: key === 'w' ? 'KeyW' : 'F4', ctrlKey: true, bubbles: true, cancelable: true });
      await act(async () => { window.dispatchEvent(event); });
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
    } finally { root.remove(); view.unmount(); }
  });

  it.each(['modal', 'settings', 'datagrid', 'monaco', 'composing'])('não fecha aba pelo teclado em %s', async (kind) => {
    locationState.pathname = kind === 'settings' ? '/settings' : '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'g-close-block', bindings: [
      { shortcut: { version: 1, code: 'KeyW', modifiers: ['Control'] }, commandId: 'workspace.tab.close', handler: 'contextual' },
    ] });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    const wrapper = document.createElement('div');
    wrapper.className = kind === 'datagrid' ? 'datagrid-container' : kind === 'monaco' ? 'monaco-editor' : '';
    const input = document.createElement('textarea');
    wrapper.appendChild(input);
    root.appendChild(wrapper);
    document.body.appendChild(root);
    const view = render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    try {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
      input.focus();
      modalState.open = kind === 'modal';
      await act(async () => {
        input.dispatchEvent(new KeyboardEvent('keydown', { key: 'w', code: 'KeyW', ctrlKey: true, isComposing: kind === 'composing', bubbles: true, cancelable: true }));
      });
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
    } finally { root.remove(); view.unmount(); }
  });

  it('não captura Ctrl+T em Monaco, durante IME ou com modal aberto', async () => {
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-chat-create-negative',
      bindings: [{ shortcut: { version: 1, code: 'KeyT', modifiers: ['Control'] }, commandId: 'workspace.tab.chat.create', handler: 'contextual' }],
    });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    const monaco = document.createElement('div');
    monaco.className = 'monaco-editor';
    monaco.tabIndex = 0;
    const input = document.createElement('textarea');
    root.append(monaco, input);
    document.body.appendChild(root);
    render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());

    monaco.focus();
    const monacoEvent = new KeyboardEvent('keydown', { key: 't', code: 'KeyT', ctrlKey: true, bubbles: true, cancelable: true });
    monaco.dispatchEvent(monacoEvent);
    expect(monacoEvent.defaultPrevented).toBe(false);

    const monacoInput = document.createElement('textarea');
    monaco.appendChild(monacoInput);
    monacoInput.focus();
    const monacoInputEvent = new KeyboardEvent('keydown', { key: 't', code: 'KeyT', ctrlKey: true, bubbles: true, cancelable: true });
    monacoInput.dispatchEvent(monacoInputEvent);
    expect(monacoInputEvent.defaultPrevented).toBe(false);

    input.focus();
    const composingEvent = new KeyboardEvent('keydown', { key: 't', code: 'KeyT', ctrlKey: true, isComposing: true, bubbles: true, cancelable: true });
    input.dispatchEvent(composingEvent);
    expect(composingEvent.defaultPrevented).toBe(false);
    const imeEvent = new KeyboardEvent('keydown', { key: 't', code: 'KeyT', ctrlKey: true, bubbles: true, cancelable: true });
    Object.defineProperty(imeEvent, 'keyCode', { value: 229 });
    input.dispatchEvent(imeEvent);
    expect(imeEvent.defaultPrevented).toBe(false);

    modalState.open = true;
    const modalEvent = new KeyboardEvent('keydown', { key: 't', code: 'KeyT', ctrlKey: true, bubbles: true, cancelable: true });
    input.dispatchEvent(modalEvent);
    expect(modalEvent.defaultPrevented).toBe(false);
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    root.remove();
  });

  it('não inicia novamente uma execução contextual em repetição', async () => {
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-chat-create-repeat',
      bindings: [{ shortcut: { version: 1, code: 'KeyT', modifiers: ['Control'] }, commandId: 'workspace.tab.chat.create', handler: 'contextual' }],
    });
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    const input = document.createElement('textarea');
    root.appendChild(input);
    document.body.appendChild(root);
    render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    input.focus();
    const event = new KeyboardEvent('keydown', { key: 't', code: 'KeyT', ctrlKey: true, repeat: true, bubbles: true, cancelable: true });
    input.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    root.remove();
  });

  it.each([
    ['workspace.create', 'workspace'],
    ['workspace.tab.chat.create', 'chat'],
    ['workspace.tab.editor.create', 'editor'],
    ['workspace.tab.tasklist.create', 'tasklist'],
    ['workspace.tab.terminal.create', 'terminal'],
    ['workspace.tab.close', 'close'],
    ...WORKSPACE_TAB_NAVIGATION_COMMAND_IDS.map((id) => [id, id] as const),
  ] as const)('executa %s pela paleta e não usa criação local', async (commandId, label) => {
    if (isWorkspaceTabNavigationCommand(commandId)) setupNavigationTabs();
    locationState.pathname = '/';
    const goToArguments = { workspace_id: 'workspace-a', target_mode: 'position', position: 2 };
    if (commandId === 'workspace.tab.go_to') {
      keyboardState.loadMap.mockResolvedValue({
        generation: 'palette-go-to',
        bindings: [],
        localPaletteCommands: ['workspace.tab.go_to'],
        localPaletteArguments: { 'workspace.tab.go_to': goToArguments },
      });
    }
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    document.body.appendChild(root);
    catalogState.list.mockResolvedValueOnce([{ id: commandId, name: `Nova aba de ${label}`, available: true }]);
    executionState.port.beginUICommand.mockResolvedValue({ ticket: `palette-${label}-ticket`, invocationId: `palette-${label}-inv`, commandId });
    executionState.port.takeUICommand.mockResolvedValue({ ticket: `palette-${label}-ticket`, invocationId: `palette-${label}-inv`, commandId, handoffId: `palette-${label}-handoff` });
    executionState.port.getUICommandResult.mockResolvedValue({ invocationId: `palette-${label}-inv`, status: 'succeeded' });
    const user = userEvent.setup();
    render(<CommandContextProvider><Topbar /></CommandContextProvider>);

    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await waitFor(() => expect(commandOpenSpy).toHaveBeenCalledTimes(1));
    const item = (commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void; disabled?: boolean }>)[0];
    expect(item.disabled).toBe(false);
    await act(async () => {
      item.action?.();
      menuHookState.afterSelect?.();
      if (isWorkspaceTabNavigationCommand(commandId)) {
        expect(workspaceState.setActiveTab).toHaveBeenCalledExactlyOnceWith(navigationTargetId(commandId));
      } else {
        await waitFor(() => expect(executionState.port.commitBackendCommand).toHaveBeenCalledWith(`palette-${label}-ticket`, `palette-${label}-handoff`));
      }
    });
    if (isWorkspaceTabNavigationCommand(commandId)) {
      expect(keyboardState.beginUI).not.toHaveBeenCalled();
      expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
      expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
      expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
      expect(executionState.port.getUICommandResult).not.toHaveBeenCalled();
    } else {
      expect(executionState.port.completeUICommand).not.toHaveBeenCalledWith(`palette-${label}-ticket`, `palette-${label}-handoff`, 'succeeded');
    }
    root.remove();
  });

  it.each([
    ['workspace.tab.editor.create', 'editor'],
    ['workspace.tab.tasklist.create', 'tasklist'],
    ['workspace.tab.terminal.create', 'terminal'],
    ['workspace.tab.close', 'close'],
    ...WORKSPACE_TAB_NAVIGATION_COMMAND_IDS.map((id) => [id, id] as const),
  ] as const)('mantém %s indisponível em settings', async (commandId, label) => {
    if (isWorkspaceTabNavigationCommand(commandId)) setupNavigationTabs();
    locationState.pathname = '/settings';
    catalogState.list.mockResolvedValueOnce([{
      id: commandId,
      name: `Nova aba de ${label}`,
      available: true,
    }]);
    const user = userEvent.setup();
    render(<CommandContextProvider><Topbar /></CommandContextProvider>);

    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });
    const item = (commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void; disabled?: boolean }>)[0];
    expect(item.disabled).toBe(true);
    item.action?.();
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
  });

  it.each([
    ['workspace.create', 'workspace'],
    ['workspace.tab.chat.create', 'chat'],
    ['workspace.tab.editor.create', 'editor'],
    ['workspace.tab.tasklist.create', 'tasklist'],
    ['workspace.tab.terminal.create', 'terminal'],
    ['workspace.tab.close', 'close'],
  ] as const)('cancela reservation contextual do Deck para %s quando a aba muda antes do commit', async (commandId, label) => {
    if (isWorkspaceTabNavigationCommand(commandId)) setupNavigationTabs();
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    document.body.appendChild(root);
    let resolveTake!: (response: unknown) => void;
    executionState.port.takeUICommand.mockImplementation(() => new Promise((resolve) => { resolveTake = resolve; }));
    render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    const targetLease = captureWorkspaceTabTarget(() => locationState.pathname);
    expect(targetLease).toBeDefined();
    targetLease?.dispose();

    await act(async () => {
      await waitFor(() => expect(keyboardState.handlers.has('command:deck-ui-reservation')).toBe(true));
      keyboardState.handlers.get('command:deck-ui-reservation')?.({
        ticket: `deck-${label}-ticket`, invocationId: `deck-${label}-inv`, commandId,
      });
      await waitFor(() => expect(executionState.port.takeUICommand).toHaveBeenCalledWith(`deck-${label}-ticket`));
    });
    workspaceState.workspace = { ...workspaceState.workspace, activeTabId: 'tab-b', tabs: [{ id: 'tab-b', type: 'chat' as const }] };
    workspaceState.listeners.forEach((listener) => listener());
    await act(async () => {
      resolveTake({ ticket: `deck-${label}-ticket`, invocationId: `deck-${label}-inv`, commandId, handoffId: `deck-${label}-handoff` });
      await Promise.resolve();
    });
    expect(executionState.port.commitBackendCommand).not.toHaveBeenCalled();
    expect(executionState.port.completeUICommand).toHaveBeenCalledWith(`deck-${label}-ticket`, `deck-${label}-handoff`, 'cancelled');
    root.remove();
  });

  it.each([
    ['workspace.tab.chat.create', 'chat'],
    ['workspace.tab.editor.create', 'editor'],
    ['workspace.tab.tasklist.create', 'tasklist'],
    ['workspace.tab.terminal.create', 'terminal'],
    ['workspace.tab.close', 'close'],
  ] as const)('executa reservation contextual positiva do Deck para %s sem BeginUICommand', async (commandId, label) => {
    if (isWorkspaceTabNavigationCommand(commandId)) setupNavigationTabs();
    locationState.pathname = '/';
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    document.body.appendChild(root);
    executionState.port.takeUICommand.mockResolvedValue({ ticket: `deck-${label}-ticket`, invocationId: `deck-${label}-inv`, commandId, handoffId: `deck-${label}-handoff` });
    executionState.port.getUICommandResult.mockResolvedValue({ invocationId: `deck-${label}-inv`, status: 'succeeded' });
    render(<CommandContextProvider><Topbar /></CommandContextProvider>);
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    await act(async () => {
      await waitFor(() => expect(keyboardState.handlers.has('command:deck-ui-reservation')).toBe(true));
      keyboardState.handlers.get('command:deck-ui-reservation')?.({
        ticket: `deck-${label}-ticket`, invocationId: `deck-${label}-inv`, commandId,
      });
      await waitFor(() => expect(executionState.port.takeUICommand).toHaveBeenCalledWith(`deck-${label}-ticket`));
      await waitFor(() => expect(executionState.port.commitBackendCommand).toHaveBeenCalledWith(`deck-${label}-ticket`, `deck-${label}-handoff`));
    });
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    expect(executionState.port.completeUICommand).not.toHaveBeenCalledWith(`deck-${label}-ticket`, `deck-${label}-handoff`, 'succeeded');
    root.remove();
  });

  it.each([
    ['e', '/settings/data?action=export'],
    ['i', '/settings/data?action=import'],
  ] as const)('navega com Alt+%s para %s pelo binding explícito', async (key, route) => {
    const commandId = key === 'e' ? 'navigation.data.export.open' : 'navigation.data.import.open';
    const code = key === 'e' ? 'KeyE' : 'KeyI';
    keyboardState.loadMap.mockResolvedValue({ generation: `g-data-${key}`, bindings: [{
      shortcut: { version: 1, code, modifiers: ['Alt'] }, commandId, handler: 'local_ui',
    }] });
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<Topbar />);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    navigateSpy.mockClear();
    announceSpy.mockClear();

    await act(async () => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key, code, altKey: true, bubbles: true, cancelable: true }));
    });
    expect(navigateSpy).toHaveBeenCalledWith(route);
    expect(navigateSpy).toHaveBeenCalledTimes(1);
    expect(announceSpy).not.toHaveBeenCalled();
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    await act(async () => { view.unmount(); await Promise.resolve(); });
  });

  it.each([
    ['w', 'KeyW', '/', 'navigation.workspace.open'],
    ['c', 'KeyC', '/settings', 'navigation.settings.open'],
    ['h', 'KeyH', '/history', 'navigation.history.open'],
    ['l', 'KeyL', '/memories', 'navigation.memories.open'],
    ['t', 'KeyT', '/tasklists', 'navigation.tasklists.open'],
    ['j', 'KeyJ', '/jobs', 'navigation.jobs.open'],
    ['p', 'KeyP', '/profiles', 'navigation.profiles.open'],
    ['Backspace', 'Backspace', '/', 'navigation.workspace.open'],
  ])('executa Alt+%s exclusivamente pelo binding UI do mapa e handoff', async (key, code, route, commandId) => {
    const shortcut = { version: 1 as const, code, modifiers: ['Alt' as const] };
    keyboardState.loadMap.mockResolvedValue({
      generation: 'g-migrated-default',
      bindings: [{ shortcut, commandId, handler: 'local_ui' as const }],
    });
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    let view!: ReturnType<typeof render>;
    act(() => { view = render(<Topbar />); });
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });
    await act(async () => {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    });
    navigateSpy.mockClear();

    const event = new KeyboardEvent('keydown', { key, code, altKey: true, bubbles: true, cancelable: true });
    await act(async () => { window.dispatchEvent(event); });
    expect(event.getModifierState('AltGraph')).toBe(false);
    expect(event.defaultPrevented).toBe(true);

    await waitFor(() => expect(navigateSpy).toHaveBeenCalledWith(route));
    expect(navigateSpy).toHaveBeenCalledTimes(1);
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    await act(async () => { view.unmount(); await Promise.resolve(); });
  });

  it.each([
    ['w', 'KeyW'], ['c', 'KeyC'], ['h', 'KeyH'], ['l', 'KeyL'],
    ['t', 'KeyT'], ['j', 'KeyJ'], ['p', 'KeyP'],
  ])('não usa rota legada para Alt+%s quando o mapa está vazio', async (key, code) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    let view!: ReturnType<typeof render>;
    act(() => { view = render(<Topbar />); });
    await act(async () => { screen.getByRole('button', { name: 'commandPalette.title' }).focus(); });
    await act(async () => {
      await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    });
    navigateSpy.mockClear();

    await act(async () => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key, code, altKey: true, bubbles: true, cancelable: true }));
    });

    expect(navigateSpy).not.toHaveBeenCalled();
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    await act(async () => { view.unmount(); await Promise.resolve(); });
  });

  it('não usa fallback de navegação para Alt+Backspace sem binding no mapa', () => {
    render(<Topbar />);
    navigateSpy.mockClear();
    announceSpy.mockClear();

    fireEvent.keyDown(window, { key: 'Backspace', altKey: true });
    expect(navigateSpy).not.toHaveBeenCalled();
  });

  it('não navega se Ctrl também está pressionado', () => {
    render(<Topbar />);
    navigateSpy.mockClear();

    fireEvent.keyDown(window, { key: 'h', altKey: true, ctrlKey: true });
    expect(navigateSpy).not.toHaveBeenCalled();
  });

  it.each(['input', 'textarea'])('abre Ctrl+K em %s nativo e preserva o rascunho', async (kind) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    catalogState.list.mockResolvedValueOnce([{ id: 'help.shortcuts.show', name: 'Atalhos', available: true }]);
    render(<Topbar />);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    const input = kind === 'input' ? document.createElement('input') : document.createElement('textarea');
    input.value = 'rascunho preservado';
    document.body.appendChild(input);
    input.focus();
    const event = new KeyboardEvent('keydown', { key: 'k', code: 'KeyK', ctrlKey: true, bubbles: true, cancelable: true });
    input.dispatchEvent(event);

    await waitFor(() => expect(catalogState.list).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(commandOpenSpy).toHaveBeenCalledTimes(1));
    expect(event.defaultPrevented).toBe(true);
    expect(input.value).toBe('rascunho preservado');
    input.remove();
  });

  it('não captura Ctrl+K durante composição IME nativa', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    render(<Topbar />);
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();

    const event = new KeyboardEvent('keydown', { key: 'k', code: 'KeyK', ctrlKey: true, isComposing: true, bubbles: true, cancelable: true });
    input.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
    expect(catalogState.list).not.toHaveBeenCalled();
    document.body.removeChild(input);
  });

  it.each(['monaco', 'contenteditable', 'modal'])('não captura Ctrl+K em contexto bloqueado: %s', async (kind) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const target = document.createElement(kind === 'contenteditable' ? 'div' : 'input');
    if (kind === 'monaco') target.className = 'monaco-editor';
    if (kind === 'contenteditable') target.contentEditable = 'true';
    document.body.appendChild(target);
    target.focus();
    if (kind === 'modal') modalState.open = true;
    render(<Topbar />);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    const event = new KeyboardEvent('keydown', { key: 'k', code: 'KeyK', ctrlKey: true, bubbles: true, cancelable: true });
    target.dispatchEvent(event);
    await Promise.resolve();
    expect(catalogState.list).not.toHaveBeenCalled();
    expect(commandOpenSpy).not.toHaveBeenCalled();
    target.remove();
  });

  it('descarta Ctrl+K quando o foco muda antes do catálogo chegar', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    let resolveCatalog!: (items: unknown[]) => void;
    catalogState.list.mockReturnValueOnce(new Promise<unknown[]>((resolve) => { resolveCatalog = resolve; }));
    render(<Topbar />);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    const input = document.createElement('input');
    const other = document.createElement('button');
    document.body.append(input, other);
    input.focus();
    input.dispatchEvent(new KeyboardEvent('keydown', { key: 'k', code: 'KeyK', ctrlKey: true, bubbles: true, cancelable: true }));
    await waitFor(() => expect(catalogState.list).toHaveBeenCalledTimes(1));
    other.focus();
    resolveCatalog([{ id: 'help.shortcuts.show', name: 'Atalhos', available: true }]);
    await act(async () => { await Promise.resolve(); });
    expect(commandOpenSpy).not.toHaveBeenCalled();
    input.remove();
    other.remove();
  });

  it('não captura Ctrl+K durante composição IME', () => {
    render(<Topbar />);
    const input = document.createElement('input');
    document.body.appendChild(input);
    input.focus();
    const event = new KeyboardEvent('keydown', { key: 'k', code: 'KeyK', ctrlKey: true, isComposing: true, bubbles: true, cancelable: true });
    input.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(false);
    document.body.removeChild(input);
  });

  it('não repete a leitura do catálogo ao segurar Ctrl+K', () => {
    render(<Topbar />);
    fireEvent.keyDown(window, { key: 'k', code: 'KeyK', ctrlKey: true, repeat: true, cancelable: true });

    expect(catalogState.list).not.toHaveBeenCalled();
  });

  it('não publica catálogo atrasado após mudança de foco e permite retry', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const user = userEvent.setup();
    let resolveFirst!: (items: unknown[]) => void;
    let resolveSecond!: (items: unknown[]) => void;
    catalogState.list
      .mockReturnValueOnce(new Promise<unknown[]>((resolve) => { resolveFirst = resolve; }))
      .mockReturnValueOnce(new Promise<unknown[]>((resolve) => { resolveSecond = resolve; }));
    render(<Topbar />);
    const button = screen.getByRole('button', { name: 'commandPalette.title' });

    await user.click(button);
    expect(catalogState.list).toHaveBeenCalledTimes(1);
    const other = document.createElement('button');
    document.body.appendChild(other);
    other.focus();
    await act(async () => { resolveFirst([{ id: 'first', name: 'First', available: true }]); });
    expect(commandOpenSpy).not.toHaveBeenCalled();
    expect(button).toHaveAttribute('aria-busy', 'false');

    await user.click(button);
    expect(catalogState.list).toHaveBeenCalledTimes(2);
    await act(async () => { resolveSecond([{ id: 'second', name: 'Second', available: true }]); });
    expect(commandOpenSpy).toHaveBeenCalledTimes(1);
    document.body.removeChild(other);
  });

  it('descarta catálogo atrasado após mudança de rota e permite nova leitura', async () => {
    const user = userEvent.setup();
    let resolveOld!: (items: unknown[]) => void;
    let resolveNew!: (items: unknown[]) => void;
    catalogState.list
      .mockReturnValueOnce(new Promise<unknown[]>((resolve) => { resolveOld = resolve; }))
      .mockReturnValueOnce(new Promise<unknown[]>((resolve) => { resolveNew = resolve; }));
    const view = render(<Topbar />);
    const oldButton = screen.getByRole('button', { name: 'commandPalette.title' });

    await user.click(oldButton);
    locationState.pathname = '/settings';
    view.rerender(<Topbar />);
    await act(async () => { resolveOld([{ id: 'old-route', name: 'Old route', available: true }]); });
    expect(commandOpenSpy).not.toHaveBeenCalled();

    const newButton = screen.getByRole('button', { name: 'commandPalette.title' });
    await user.click(newButton);
    expect(catalogState.list).toHaveBeenCalledTimes(2);
    await act(async () => { resolveNew([{ id: 'new-route', name: 'New route', available: true }]); });
    expect(commandOpenSpy).toHaveBeenCalledTimes(1);
    expect(commandOpenSpy.mock.calls[0][2][0]).toMatchObject({ id: 'command-new-route' });
  });

  it('descarta catálogo atrasado após mudança apenas de query', async () => {
    const user = userEvent.setup();
    let resolveOld!: (items: unknown[]) => void;
    let resolveNew!: (items: unknown[]) => void;
    catalogState.list
      .mockReturnValueOnce(new Promise<unknown[]>((resolve) => { resolveOld = resolve; }))
      .mockReturnValueOnce(new Promise<unknown[]>((resolve) => { resolveNew = resolve; }));
    const view = render(<Topbar />);
    const oldButton = screen.getByRole('button', { name: 'commandPalette.title' });

    await user.click(oldButton);
    locationState.search = '?tab=next';
    locationState.key = 'query-next';
    view.rerender(<Topbar />);
    await act(async () => { resolveOld([{ id: 'old-query', name: 'Old query', available: true }]); });
    expect(commandOpenSpy).not.toHaveBeenCalled();

    const newButton = screen.getByRole('button', { name: 'commandPalette.title' });
    expect(newButton).toHaveAttribute('aria-busy', 'false');
    await user.click(newButton);
    expect(catalogState.list).toHaveBeenCalledTimes(2);
    await act(async () => { resolveNew([{ id: 'new-query', name: 'New query', available: true }]); });
    expect(commandOpenSpy).toHaveBeenCalledTimes(1);
    expect(commandOpenSpy.mock.calls[0][2][0]).toMatchObject({ id: 'command-new-query' });
    view.unmount();
  });

  it('mantém comando local válido quando o header é desconectado sem blur', async () => {
    const user = userEvent.setup();
    const openHelp = vi.spyOn(useShortcutsHelpStore.getState(), 'open').mockClear();
    catalogState.list.mockResolvedValueOnce([{ id: 'help.shortcuts.show', name: 'Keyboard shortcuts', available: true }]);
    const view = render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });
    const item = (commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void }>)[0];
    await act(async () => {
      item.action?.();
      menuHookState.afterSelect?.();
      await Promise.resolve();
    });
    await act(async () => { await Promise.resolve(); });
    expect(openHelp).toHaveBeenCalledTimes(1);

    const parent = view.container.parentNode;
    const nextSibling = view.container.nextSibling;
    try {
      parent?.removeChild(view.container);
      expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
    } finally {
      if (parent) parent.insertBefore(view.container, nextSibling);
      view.unmount();
    }
  });

  it('mantém uma única sessão de surface durante StrictMode e remount', async () => {
    const user = userEvent.setup();
    catalogState.list.mockResolvedValueOnce([{ id: 'strict-mode', name: 'Strict mode', available: true }]);
    const first = render(<StrictMode><Topbar /></StrictMode>);
    const authSubscriptionCount = authState.listeners.size;
    const workspaceSubscriptionCount = workspaceState.listeners.size;
    expect(authSubscriptionCount).toBeGreaterThan(0);
    expect(workspaceSubscriptionCount).toBeGreaterThan(0);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });
    expect(commandOpenSpy).toHaveBeenCalledTimes(1);

    first.unmount();
    expect(authState.listeners.size).toBe(0);
    expect(workspaceState.listeners.size).toBe(0);
    catalogState.list.mockResolvedValueOnce([{ id: 'after-remount', name: 'After remount', available: true }]);
    const second = render(<StrictMode><Topbar /></StrictMode>);
    expect(authState.listeners.size).toBe(authSubscriptionCount);
    expect(workspaceState.listeners.size).toBe(workspaceSubscriptionCount);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });
    expect(commandOpenSpy).toHaveBeenCalledTimes(2);
    expect(commandOpenSpy.mock.calls[1][2][0]).toMatchObject({ id: 'command-after-remount' });
    second.unmount();
    expect(authState.listeners.size).toBe(0);
    expect(workspaceState.listeners.size).toBe(0);
  });

  it.each(['unmount', 'route'])('preserva comando local após %s sem RPC tardio', async (change) => {
    const user = userEvent.setup();
    const openHelp = vi.spyOn(useShortcutsHelpStore.getState(), 'open').mockClear();
    catalogState.list.mockResolvedValueOnce([{ id: 'help.shortcuts.show', name: 'Keyboard shortcuts', available: true }]);
    const view = render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });
    (commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void }>)[0].action?.();
    menuHookState.afterSelect?.();
    await act(async () => { await Promise.resolve(); });
    expect(openHelp).toHaveBeenCalledTimes(1);

    if (change === 'unmount') {
      view.unmount();
    } else {
      locationState.pathname = '/settings';
      view.rerender(<Topbar />);
    }
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
    expect(executionState.port.completeUICommand).not.toHaveBeenCalled();
  });

  it('não anuncia falha de comando local após mudança de rota com mesmo owner', async () => {
    const user = userEvent.setup();
    announceSpy.mockClear();
    catalogState.list.mockResolvedValueOnce([{ id: 'help.shortcuts.show', name: 'Keyboard shortcuts', available: true }]);
    const view = render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });
    expect(announceSpy).toHaveBeenCalledWith('commandPalette.results');
    announceSpy.mockClear();
    (commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void }>)[0].action?.();
    menuHookState.afterSelect?.();
    await act(async () => { await Promise.resolve(); });

    locationState.pathname = '/settings';
    locationState.key = 'same-owner-route';
    view.rerender(<Topbar />);
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
    view.unmount();
  });

  it.each([true, false])('anuncia a abertura da lista, vazia=%s, sem executar comando', async (empty) => {
    const user = userEvent.setup();
    catalogState.list.mockResolvedValueOnce(empty ? [] : [
      { id: 'navigation.settings.open', name: 'Configurações', available: false, readinessReason: 'not-ready' },
    ]);
    const view = render(<Topbar />);
    announceSpy.mockClear();
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await waitFor(() => expect(announceSpy).toHaveBeenCalledWith(empty ? 'commandPalette.empty' : 'commandPalette.results'));
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    expect(commandOpenSpy).toHaveBeenCalled();
    view.unmount();
  });

  it('executa help.shortcuts.show localmente e não abre handoff RPC', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const user = userEvent.setup();
    const openHelp = vi.spyOn(useShortcutsHelpStore.getState(), 'open').mockClear();
    catalogState.list.mockResolvedValueOnce([{
      id: 'help.shortcuts.show',
      name: 'Keyboard shortcuts',
      description: 'Show shortcuts',
      available: true,
    }]);
    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });

    const menuItems = commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void }>;
    expect(menuItems).toHaveLength(1);
    menuItems[0].action?.();
    menuHookState.afterSelect?.();
    await act(async () => { await Promise.resolve(); });
    expect(openHelp).toHaveBeenCalledTimes(1);
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
  });

  it('executa navegação catalogada localmente e pesquisa por alias', async () => {
    const user = userEvent.setup();
    catalogState.list.mockResolvedValueOnce([{
      id: 'navigation.settings.open',
      name: 'Configurações',
      description: 'Preferências do aplicativo',
      category: 'navegação',
      aliases: ['ajustes'],
      available: true,
    }]);
    render(<Topbar />);

    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });
    const item = (commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void; searchText?: string }>)[0];
    expect(item.searchText).toBe('Preferências do aplicativo navegação ajustes');

    await act(async () => {
      item.action?.();
      menuHookState.afterSelect?.();
      await Promise.resolve();
    });
    await waitFor(() => expect(navigateSpy).toHaveBeenCalledWith('/settings'));
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
    expect(executionState.port.takeUICommand).not.toHaveBeenCalled();
  });

  it('expõe readinessReason no nome acessível e impede seleção indisponível', async () => {
    const user = userEvent.setup();
    catalogState.list.mockResolvedValueOnce([{
      id: 'navigation.settings.open',
      name: 'Configurações',
      description: 'Preferências',
      available: false,
      availabilityStatus: 'available',
      readinessReason: 'runtime desconectado',
    }]);
    render(<Topbar />);

    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });
    const item = (commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void; ariaLabel?: string; disabled?: boolean }>)[0];
    expect(item.ariaLabel).toContain('runtime desconectado');
    expect(item.ariaLabel).not.toContain('available');
    expect(item.disabled).toBe(true);
    item.action?.();
    expect(screen.getByRole('button', { name: 'commandPalette.title' })).toHaveAttribute('aria-expanded', 'true');
    expect(navigateSpy).not.toHaveBeenCalled();
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
  });

  it.each([
    ['suppressed', 'commandPalette.executionSuppressed'],
    ['denied', 'commandPalette.unavailable'],
    ['rejected_stale', 'commandPalette.unavailable'],
  ])('anuncia %s sem abrir o picker de resultado', async (status, message) => {
    const user = userEvent.setup();
    announceSpy.mockClear();
    backendExecutionState.execute.mockResolvedValueOnce({
      execution: { invocationId: 'inv-workspace-1', status }, presented: false, reason: 'backend-status',
    });
    catalogState.list.mockResolvedValueOnce([{ id: 'workspace.list', name: 'List workspaces', available: true }]);
    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });
    const items = commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void }>;
    await act(async () => {
      items[0].action?.();
      menuHookState.afterSelect?.();
      await Promise.resolve();
    });
    expect(announceSpy).toHaveBeenCalledWith(message);
    expect(commandOpenSpy).toHaveBeenCalledTimes(1);
  });

  it.each(['layer.activate', 'layer.toggle', 'layer.back'])('seleciona %s na paleta pelo executor backend', async (commandID) => {
    const user = userEvent.setup();
    backendExecutionState.execute.mockResolvedValueOnce({ execution: { invocationId: 'layer-invocation', status: 'succeeded' }, presented: true, reason: 'presented' });
    catalogState.list.mockResolvedValueOnce([{ id: commandID, name: 'Ação de camada', available: true }]);
    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });
    const items = commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void }>;
    await act(async () => { items[0].action?.(); menuHookState.afterSelect?.(); await Promise.resolve(); });
    expect(backendExecutionState.execute).toHaveBeenCalledWith(commandID, expect.any(Function));
    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
  });

  it('apresenta workspace.list no picker compartilhado e revalida a intenção ao selecionar', async () => {
    const user = userEvent.setup();
    backendExecutionState.execute.mockImplementationOnce((_commandID: string, apply: (output: unknown) => undefined) => {
      apply({ kind: 'workspace.list', workspaces: [
        { id: 'workspace-b', name: 'Workspace B', profile: 'default', tab_count: 2, is_active: false },
      ] });
      return Promise.resolve({
        execution: { invocationId: 'inv-workspace-1', status: 'succeeded' }, presented: true, reason: 'presented',
      });
    });
    catalogState.list.mockResolvedValueOnce([{ id: 'workspace.list', name: 'List workspaces', available: true }]);
    render(<Topbar />);

    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });
    const commandItems = commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void }>;
    await act(async () => {
      commandItems[0].action?.();
      menuHookState.afterSelect?.();
      await Promise.resolve();
    });

    expect(backendExecutionState.execute).toHaveBeenCalledWith('workspace.list', expect.any(Function));
    const workspaceItems = commandOpenSpy.mock.calls[1]?.[2] as Array<{ label: string; action?: () => void }>;
    expect(workspaceItems).toHaveLength(1);
    expect(workspaceItems[0].label).toBe('Workspace B');
    await act(async () => { workspaceItems[0].action?.(); });
    expect(workspaceState.switchWorkspace).toHaveBeenCalledWith('workspace-b');
    await act(async () => { workspaceItems[0].action?.(); });
    expect(workspaceState.switchWorkspace).toHaveBeenCalledTimes(1);

    authState.user = { userId: 'user-a', sessionId: 'session-b', role: 'user' };
    authState.listeners.forEach((listener) => listener());
    await act(async () => { workspaceItems[0].action?.(); });
    expect(workspaceState.switchWorkspace).toHaveBeenCalledTimes(1);
  });

  it('descarta callback antigo do picker quando uma intenção nova já foi publicada', async () => {
    const user = userEvent.setup();
    const applies: Array<(output: unknown) => undefined> = [];
    backendExecutionState.execute.mockImplementation((_commandID: string, apply: (output: unknown) => undefined) => {
      applies.push(apply);
      apply({ kind: 'workspace.list', workspaces: [
        { id: applies.length === 1 ? 'workspace-old' : 'workspace-new', name: applies.length === 1 ? 'Old' : 'New', profile: 'default', tab_count: 1, is_active: false },
      ] });
      return Promise.resolve({
        execution: { invocationId: `inv-${applies.length}`, status: 'succeeded' }, presented: true, reason: 'presented',
      });
    });
    catalogState.list.mockResolvedValue([{ id: 'workspace.list', name: 'List workspaces', available: true }]);
    render(<Topbar />);

    const commandButton = screen.getByRole('button', { name: 'commandPalette.title' });
    await user.click(commandButton);
    await act(async () => { await Promise.resolve(); });
    await act(async () => {
      (commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void }>)[0].action?.();
      menuHookState.afterSelect?.();
      await Promise.resolve();
    });
    const oldItems = commandOpenSpy.mock.calls[1]?.[2] as Array<{ action?: () => void }>;

    // Reutiliza o item do catálogo: cada seleção captura uma intenção distinta.
    await act(async () => {
      (commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void }>)[0].action?.();
      menuHookState.afterSelect?.();
      await Promise.resolve();
    });

    workspaceState.switchWorkspace.mockClear();
    await act(async () => { oldItems[0].action?.(); });
    expect(workspaceState.switchWorkspace).not.toHaveBeenCalled();
  });

  it('restaura o foco no botão de comando ao fechar o picker compartilhado fora do workspace', async () => {
    const user = userEvent.setup();
    expect(useShortcutsHelpStore.getState().isOpen).toBe(false);
    backendExecutionState.execute.mockImplementationOnce((_commandID: string, apply: (output: unknown) => undefined) => {
      apply({ kind: 'workspace.list', workspaces: [] });
      return Promise.resolve({
        execution: { invocationId: 'inv-workspace-1', status: 'succeeded' }, presented: true, reason: 'presented',
      });
    });
    catalogState.list.mockResolvedValueOnce([{ id: 'workspace.list', name: 'List workspaces', available: true }]);
    render(<Topbar />);

    const commandButton = screen.getByRole('button', { name: 'commandPalette.title' });
    await user.click(commandButton);
    await act(async () => { await Promise.resolve(); });
    await act(async () => {
      (commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void }>)[0].action?.();
      menuHookState.afterSelect?.();
      await Promise.resolve();
    });

    await act(async () => { menuHookState.afterDismisses[0]?.(); });
    expect(document.activeElement).toBe(commandButton);
  });

  it('cancela somente a apresentação pendente de workspace.list com Escape', async () => {
    const user = userEvent.setup();
    let resolve!: (result: unknown) => void;
    backendExecutionState.execute.mockImplementationOnce(() => new Promise((done) => { resolve = done; }));
    catalogState.list.mockResolvedValueOnce([{ id: 'workspace.list', name: 'List workspaces', available: true }]);
    render(<Topbar />);

    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });
    await act(async () => {
      (commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void }>)[0].action?.();
      menuHookState.afterSelect?.();
      await Promise.resolve();
    });
    fireEvent.keyDown(window, { key: 'Escape' });
    expect(backendExecutionState.cancelPresentation).toHaveBeenCalledTimes(1);
    await act(async () => {
      resolve({ execution: { invocationId: 'inv-workspace-1', status: 'succeeded', output: { kind: 'workspace.list', workspaces: [] } }, presented: false, reason: 'context-stale' });
    });
  });

  it('descarta intenção enfileirada quando a sessão muda antes do callback de foco', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const user = userEvent.setup();
    catalogState.list.mockResolvedValueOnce([{
      id: 'help.shortcuts.show', name: 'Keyboard shortcuts', available: true,
    }]);
    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await act(async () => { await Promise.resolve(); });

    const menuItems = commandOpenSpy.mock.calls[0]?.[2] as Array<{ action?: () => void }>;
    menuItems[0].action?.();
    await act(async () => {
      authState.user = { userId: 'user-a', sessionId: 'session-b', role: 'user' };
      authState.listeners.forEach((listener) => listener());
      menuHookState.afterSelect?.();
      await Promise.resolve();
    });

    expect(executionState.port.beginUICommand).not.toHaveBeenCalled();
  });

  it.each(['logout', 'workspace', 'aba', 'foco ABA', 'modal', 'Escape', 'unmount'])
    ('descarta resposta pendente após %s', async (change) => {
      const user = userEvent.setup();
      let resolve!: (items: unknown[]) => void;
      catalogState.list.mockReturnValueOnce(new Promise<unknown[]>((done) => { resolve = done; }));
      const view = render(<Topbar />);
      const button = screen.getByRole('button', { name: 'commandPalette.title' });
      await user.click(button);
      expect(catalogState.list).toHaveBeenCalledTimes(1);
      expect(button).toHaveAttribute('aria-busy', 'true');
      let extra: HTMLElement | undefined;
      try {
        if (change === 'logout') authState.isAuthenticated = false;
        if (change === 'workspace') workspaceState.workspace = { ...workspaceState.workspace, id: 'workspace-b' };
        if (change === 'aba') workspaceState.workspace = { ...workspaceState.workspace, activeTabId: 'tab-b' };
        if (change === 'foco ABA') {
          extra = document.createElement('button');
          document.body.appendChild(extra);
          act(() => {
            extra?.focus();
            button.focus();
          });
        }
        if (change === 'modal') {
          extra = document.createElement('div');
          extra.className = 'modal-overlay';
          document.body.appendChild(extra);
          registerOpenModal('pending-catalog-modal');
        }
        if (change === 'Escape') fireEvent.keyDown(button, { key: 'Escape' });
        if (change === 'unmount') view.unmount();
        await act(async () => { resolve([{ id: 'late', name: 'Late', available: true }]); });
        expect(commandOpenSpy).not.toHaveBeenCalled();
        if (change !== 'unmount') expect(button).toHaveAttribute('aria-busy', 'false');
      } finally {
        if (change === 'modal') unregisterOpenModal('pending-catalog-modal');
        extra?.remove();
      }
    });

  it('somente a requisição mais recente pode publicar o catálogo', async () => {
    const user = userEvent.setup();
    let resolveFirst!: (items: unknown[]) => void;
    let resolveSecond!: (items: unknown[]) => void;
    catalogState.list
      .mockReturnValueOnce(new Promise<unknown[]>((done) => { resolveFirst = done; }))
      .mockReturnValueOnce(new Promise<unknown[]>((done) => { resolveSecond = done; }));
    render(<Topbar />);
    const button = screen.getByRole('button', { name: 'commandPalette.title' });
    await user.click(button);
    await user.click(button);
    expect(catalogState.list).toHaveBeenCalledTimes(2);
    await act(async () => { resolveFirst([{ id: 'old', name: 'Old', available: true }]); });
    expect(commandOpenSpy).not.toHaveBeenCalled();
    expect(button).toHaveAttribute('aria-busy', 'true');
    await act(async () => { resolveSecond([{ id: 'new', name: 'New', available: true }]); });
    expect(commandOpenSpy).toHaveBeenCalledTimes(1);
    expect(commandOpenSpy.mock.calls[0][2][0]).toMatchObject({ id: 'command-new', label: 'New' });
    expect(button).toHaveAttribute('aria-busy', 'false');
  });

  it('Ctrl+K consulta e publica após revalidar o foco atual', async () => {
    let resolve!: (items: unknown[]) => void;
    catalogState.list.mockReturnValueOnce(new Promise<unknown[]>((done) => { resolve = done; }));
    render(<Topbar />);
    const button = screen.getByRole('button', { name: 'commandPalette.title' });
    act(() => { button.focus(); });
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    fireEvent.keyDown(button, { key: 'k', code: 'KeyK', ctrlKey: true });
    await waitFor(() => expect(catalogState.list).toHaveBeenCalledTimes(1));
    expect(commandOpenSpy).not.toHaveBeenCalled();
    await act(async () => { resolve([{ id: 'keyboard', name: 'Keyboard', available: true }]); });
    expect(commandOpenSpy).toHaveBeenCalledTimes(1);
  });

  it('encaminha a abertura pelo item do menu principal após a seleção', async () => {
    render(<Topbar />);

    fireEvent.click(screen.getByTestId('main-menu-command-palette'));
    await act(async () => { await Promise.resolve(); });

    expect(document.activeElement).toBe(screen.getByRole('button', { name: 'commandPalette.title' }));
    expect(screen.getByRole('button', { name: 'commandPalette.title' })).toHaveAttribute('aria-expanded', 'true');
    expect(catalogState.list).toHaveBeenCalledOnce();
    expect(commandOpenSpy).toHaveBeenCalledOnce();
  });

  it('mantém o ingresso pelo botão de teclado sem desabilitá-lo durante a carga', () => {
    render(<Topbar />);
    const button = screen.getByRole('button', { name: 'commandPalette.title' });

    fireEvent.keyDown(button, { key: 'Enter' });
    expect(button).not.toBeDisabled();
  });

  it('não inicia o picker quando um modal já está aberto', () => {
    modalState.open = true;
    try {
      render(<Topbar />);
      const button = screen.getByRole('button', { name: 'commandPalette.title' });

      fireEvent.click(button);

      expect(button).toHaveAttribute('aria-expanded', 'false');
    } finally {
      modalState.open = false;
    }
  });

  it('atualiza os rótulos da paleta e menus com o mapa aceito e remove os suprimidos', async () => {
    keyboardState.loadMap.mockResolvedValue({ generation: 'hints', bindings: [
      { shortcut: { version: 1, code: 'KeyP', modifiers: ['Control'] }, commandId: 'navigation.palette.open', handler: 'local_ui' },
      { shortcut: { version: 1, code: 'F1', modifiers: [] }, commandId: 'navigation.settings.open', handler: 'local_ui' },
      { shortcut: { version: 1, code: 'KeyU', modifiers: ['Alt'] }, commandId: 'navigation.menu.open', handler: 'local_ui' },
    ] });
    render(<Topbar />);
    const palette = screen.getByRole('button', { name: 'commandPalette.title' });
    await waitFor(() => expect(palette).toHaveAttribute('data-shortcut', 'Ctrl+P'));
    expect(screen.getByTestId('main-menu-command-palette')).toHaveAttribute('data-shortcut', 'Ctrl+P');
    expect(screen.getByTestId('main-menu-settings')).toHaveAttribute('data-shortcut', 'F1');
    expect(screen.getByTestId('main-menu-help')).not.toHaveAttribute('data-shortcut');
    expect(screen.getByRole('button', { name: 'menu.navLabelPlain — Alt+U' })).toBeInTheDocument();
    keyboardState.loadMap.mockResolvedValue({ generation: 'hints-suppressed', bindings: [] });
    await act(async () => { keyboardState.handlers.get('command:keyboard-map-changed')?.(); });
    await waitFor(() => expect(palette).not.toHaveAttribute('data-shortcut'));
    expect(screen.getByTestId('main-menu-settings')).not.toHaveAttribute('data-shortcut');
    expect(screen.getByRole('button', { name: 'menu.navLabelPlain' })).toBeInTheDocument();
  });

  it('navega para /help com F1 pelo mapa efetivo', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'f1-default', bindings: [{
      shortcut: { version: 1, code: 'F1', modifiers: [] }, commandId: 'navigation.help.open', handler: 'local_ui',
    }] });
    const view = render(<Topbar />);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    navigateSpy.mockClear();
    const event = new KeyboardEvent('keydown', { key: 'F1', code: 'F1', bubbles: true, cancelable: true });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(true);
    expect(navigateSpy).toHaveBeenCalledExactlyOnceWith('/help');
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    view.unmount();
  });

  it('não usa fallback legado de F1 quando o mapa suprime o comando', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'f1-suppressed', bindings: [] });
    const view = render(<Topbar />);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    navigateSpy.mockClear();
    const event = new KeyboardEvent('keydown', { key: 'F1', code: 'F1', bubbles: true, cancelable: true });
    window.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
    expect(navigateSpy).not.toHaveBeenCalled();
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    view.unmount();
  });

  it('respeita o remapeamento efetivo de F1', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'f1-remapped', bindings: [{
      shortcut: { version: 1, code: 'F1', modifiers: [] }, commandId: 'navigation.settings.open', handler: 'local_ui',
    }] });
    const view = render(<Topbar />);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    navigateSpy.mockClear();
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'F1', code: 'F1', bubbles: true, cancelable: true }));
    expect(navigateSpy).toHaveBeenCalledExactlyOnceWith('/settings');
    view.unmount();
  });

  it('permite apenas navigation.help.open no modal; outros comandos locais continuam bloqueados', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: 'f1-modal', bindings: [
      { shortcut: { version: 1, code: 'F1', modifiers: [] }, commandId: 'navigation.help.open', handler: 'local_ui' },
      { shortcut: { version: 1, code: 'KeyH', modifiers: ['Alt'] }, commandId: 'navigation.history.open', handler: 'local_ui' },
    ] });
    const view = render(<Topbar />);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    modalState.open = true;
    navigateSpy.mockClear();
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'F1', code: 'F1', bubbles: true, cancelable: true }));
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'h', code: 'KeyH', altKey: true, bubbles: true, cancelable: true }));
    expect(navigateSpy).toHaveBeenCalledExactlyOnceWith('/help');
    expect(navigateSpy).not.toHaveBeenCalledWith('/history');
    view.unmount();
  });

  it.each(['input', 'textarea'] as const)('permite F1 mapeado em campo editável: %s', async (tag) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: `f1-${tag}`, bindings: [{
      shortcut: { version: 1, code: 'F1', modifiers: [] }, commandId: 'navigation.help.open', handler: 'local_ui',
    }] });
    const view = render(<Topbar />);
    const field = document.createElement(tag);
    document.body.appendChild(field);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    field.focus();
    navigateSpy.mockClear();
    field.dispatchEvent(new KeyboardEvent('keydown', { key: 'F1', code: 'F1', bubbles: true, cancelable: true }));
    expect(navigateSpy).toHaveBeenCalledExactlyOnceWith('/help');
    field.remove();
    view.unmount();
  });

  it.each([
    ['composing', { isComposing: true }],
    ['repeat', { repeat: true }],
  ] as const)('não executa F1 durante %s', async (_label, options) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    keyboardState.loadMap.mockResolvedValue({ generation: `f1-${_label}`, bindings: [{
      shortcut: { version: 1, code: 'F1', modifiers: [] }, commandId: 'navigation.help.open', handler: 'local_ui',
    }] });
    const view = render(<Topbar />);
    const field = document.createElement('input');
    document.body.appendChild(field);
    await waitFor(() => expect(keyboardState.loadMap).toHaveBeenCalled());
    field.focus();
    navigateSpy.mockClear();
    field.dispatchEvent(new KeyboardEvent('keydown', { key: 'F1', code: 'F1', bubbles: true, cancelable: true, ...options }));
    expect(navigateSpy).not.toHaveBeenCalled();
    expect(keyboardState.beginUI).not.toHaveBeenCalled();
    field.remove();
    view.unmount();
  });

  it('Alt+Backspace previne o default mas não navega com um modal aberto', () => {
    modalState.open = true;
    try {
      render(<Topbar />);
      navigateSpy.mockClear();

    const event = new KeyboardEvent('keydown', { key: 'Backspace', altKey: true, bubbles: true, cancelable: true });
    window.dispatchEvent(event);

    expect(event.defaultPrevented).toBe(true);
    expect(navigateSpy).not.toHaveBeenCalled();
    } finally {
      modalState.open = false;
    }
  });

  it('Alt+M não age quando um modal está aberto', () => {
    modalState.open = true;
    try {
      render(<Topbar />);
      toggleMenuSpy.mockClear();

      fireEvent.keyDown(window, { key: 'm', altKey: true });

      expect(toggleMenuSpy).not.toHaveBeenCalled();
    } finally {
      modalState.open = false;
    }
  });
});
