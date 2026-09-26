import { beforeEach, describe, expect, it, vi } from 'vitest';
import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { useEffect, useRef } from 'react';
import { Topbar } from './Topbar';
import { createCommandContextScope, type CommandContextScope } from '../../lib/commandContextReact';

let paletteContextScope: CommandContextScope | null = null;

// This fixture has no native hotkeys. The shared ownership bridge is exercised
// independently with real reservation frames in commandGlobalOwnershipWails tests.
vi.mock('../../lib/commandGlobalOwnershipWails', () => ({
  acquireGlobalCommandOwnership: () => ({
    isReady: () => true, owns: () => false, dispose: () => {}, ready: Promise.resolve(),
  }),
}));
import { WorkspaceToolbar } from '../workspace/WorkspaceToolbar';
import { WorkspaceTabCreationMenuProvider } from '../../lib/workspaceTabCreationMenu';
import { getModalRegistrySnapshot, registerOpenModal, unregisterOpenModal } from '../../lib/modalRegistry';
import { Modal, isModalOpen, useModalId } from '../ui/Modal';
import { registerChatPickerSurface, CHAT_PICKER_COMMAND_IDS, requestChatPresentationCommand } from '../../lib/commandChatPickers';
import { registerEditorPresentationSurface, EDITOR_PRESENTATION_COMMAND_IDS } from '../../lib/commandEditorPresentation';
import { COMMAND_NAVIGATION_ROUTES } from '../../lib/commandNavigation';
import { ExternalUIConnectionProvider } from '../../services/externalUIConnectionReact';
import type {
  ExternalUICommandOutcome,
  ExternalUICommandReadyEvent,
  ExternalUIConnectionService,
  ExternalUIConnectionStatus,
  ExternalUIDestination,
  ExternalUIOwnerProof,
  TakeExternalUICommandResult,
} from '../../services/externalUIConnection';

const state = vi.hoisted(() => ({
  workspaceListeners: new Set<() => void>(),
  navigate: vi.fn(),
  listCommandCatalog: vi.fn(),
  describeCommandCatalogItem: vi.fn(),
  getRuntimeToolCatalog: vi.fn(),
  genericExecute: vi.fn(),
  loadMap: vi.fn(),
  beginLocalCommandUIKey: vi.fn(),
  resetLocalCommandKeyboard: vi.fn(),
  dispatchLocalCommandKey: vi.fn(),
  beginUICommand: vi.fn(),
  takeUICommand: vi.fn(),
  completeUICommand: vi.fn(),
  getUICommandResult: vi.fn(),
  cancelUICommand: vi.fn(),
  commitBackendCommand: vi.fn(),
  announce: vi.fn(),
  auth: {
    isAuthenticated: true,
    user: { userId: 'user-a', sessionId: 'session-a' },
    subscribe: () => () => undefined,
  },
  workspace: {
    workspace: { id: 'workspace-a', name: 'Workspace', activeTabId: 'tab-a', tabs: [{ id: 'tab-a', type: 'chat' }] },
    workspaces: [],
    switchWorkspace: vi.fn(),
    createWorkspace: vi.fn(),
    renameWorkspace: vi.fn(),
    setActiveTab: vi.fn(),
  },
  chat: {
    canPrepare: vi.fn(),
    prepare: vi.fn(),
    register: vi.fn(),
    getActiveWorkspace: vi.fn(),
    lease: {
      isCurrent: vi.fn(),
      dispose: vi.fn(),
      present: vi.fn(),
    },
  },
}));
const locationState = vi.hoisted(() => ({ pathname: '/' }));
const deckEvents = vi.hoisted(() => new Map<string, (payload?: unknown) => void>());

const { navigate, listCommandCatalog, beginUICommand, takeUICommand, completeUICommand, getUICommandResult, cancelUICommand } = state;

type IntegrationAuthState = {
  isAuthenticated: boolean;
  user: { userId: string; sessionId: string };
  subscribe: () => () => undefined;
};

type IntegrationWorkspaceState = {
  workspace: { id: string; name: string; activeTabId: string };
  workspaces: never[];
  switchWorkspace: typeof state.workspace.switchWorkspace;
  createWorkspace: typeof state.workspace.createWorkspace;
  renameWorkspace: typeof state.workspace.renameWorkspace;
  setActiveTab: typeof state.workspace.setActiveTab;
};

const catalog = [
  ['navigation.workspace.open', 'Workspace'],
  ['navigation.history.open', 'Histórico'],
  ['navigation.memories.open', 'Memórias'],
  ['navigation.tasklists.open', 'Listas'],
  ['navigation.jobs.open', 'Jobs'],
  ['navigation.profiles.open', 'Perfis'],
  ['navigation.settings.open', 'Configurações'],
  ['navigation.data.export.open', 'Exportar dados'],
  ['navigation.data.import.open', 'Importar dados'],
  ['navigation.help.open', 'Ajuda'],
  ['navigation.about.open', 'Sobre'],
  ['navigation.palette.open', 'Abrir paleta'],
  ['navigation.menu.open', 'Menu'],
  ['workspace.list', 'Workspaces'],
  ['help.shortcuts.show', 'Atalhos de teclado'],
].map(([id, name]) => ({
  id,
  name,
  description: `Descrição de ${name}`,
  category: 'navigation',
  aliases: [name.toLowerCase()],
  risk: 'none',
  available: true,
  availabilityStatus: 'available',
  availabilityReason: '',
  readinessReason: '',
}));

const navigationPaletteCases = Object.entries(COMMAND_NAVIGATION_ROUTES).map(([commandID, route]) => {
  const item = catalog.find(candidate => candidate.id === commandID);
  if (!item) throw new Error(`Missing navigation catalog fixture for ${commandID}`);
  return [commandID, item.name, route] as const;
});

function activeWorkspaceSnapshot(conversationID = 'conversation-a') {
  return {
    id: 'workspace-a',
    name: 'Workspace',
    snapshot_epoch: 'epoch-1',
    snapshot_sequence: '1',
    tabs: {
      active: 'tab-a',
      items: [{ id: 'tab-a', type: 'editor', title: 'Editor', position: 0, conversation_id: conversationID }],
    },
  };
}

vi.mock('react-i18next', () => ({
  useTranslation: () => ({
    t: (key: string) => key,
    i18n: { language: 'pt-BR' },
  }),
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => navigate,
  useLocation: () => ({ pathname: locationState.pathname, search: '', hash: '', key: locationState.pathname }),
}));

vi.mock('../../services/commandCatalog', () => ({
  listCommandCatalog: (query: unknown) => listCommandCatalog(query),
  describeCommandCatalogItem: (...args: unknown[]) => state.describeCommandCatalogItem(...args),
}));
vi.mock('@wailsjs/go/wailsapi/Tools', () => ({
  GetRuntimeToolCatalog: (...args: unknown[]) => state.getRuntimeToolCatalog(...args),
}));

vi.mock('../../lib/commandLocalKeyboardWails', () => ({
  createCommandLocalKeyboardWailsPort: () => ({
    loadMap: state.loadMap,
    beginLocalCommandUIKey: state.beginLocalCommandUIKey,
    resetLocalCommandKeyboard: state.resetLocalCommandKeyboard,
    dispatchLocalCommandKey: state.dispatchLocalCommandKey,
  }),
}));

vi.mock('../../store/authStore', () => ({
  useAuthStore: Object.assign(
    (selector?: (value: IntegrationAuthState) => unknown) => selector ? selector(state.auth) : state.auth,
    { getState: () => state.auth, subscribe: state.auth.subscribe },
  ),
}));

vi.mock('../../store/workspaceStore', () => ({
  flushWorkspaceNavigation: vi.fn(async () => true),
  useWorkspaceStore: Object.assign(
    (selector?: (value: IntegrationWorkspaceState) => unknown) => selector ? selector(state.workspace) : state.workspace,
    { getState: () => state.workspace, subscribe: (listener: () => void) => {
      state.workspaceListeners.add(listener);
      return () => { state.workspaceListeners.delete(listener); };
    } },
  ),
}));

vi.mock('../../store/shortcutsHelpStore', () => ({
  useShortcutsHelpStore: (selector: (state: { isOpen: boolean; open: () => void; close: () => void }) => unknown) =>
    selector({ isOpen: false, open: vi.fn(), close: vi.fn() }),
}));

vi.mock('../../store/uiStore', () => ({
  useUIStore: (selector: (state: { addToast: () => void }) => unknown) => selector({ addToast: vi.fn() }),
}));

vi.mock('../ui/Modal', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../ui/Modal')>()),
}));
vi.mock('../../hooks/useAnnouncer', () => ({ useAnnouncer: () => ({ announce: state.announce }) }));
vi.mock('../../hooks/useDefaultFocus', () => ({ restoreDefaultFocus: vi.fn() }));
vi.mock('../../lib/commandContextReact', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../../lib/commandContextReact')>()),
  useCommandContextScope: () => paletteContextScope,
}));
vi.mock('../../lib/commandUIExecutionWails', () => ({
  createCommandUIExecutionWailsPort: () => ({
    beginUICommand,
    takeUICommand,
    completeUICommand,
    getUICommandResult,
    cancelUICommand,
  }),
}));
vi.mock('../../lib/commandBackendExecutionWails', () => ({
  createCommandBackendExecutionWailsPort: () => ({ executeCommand: (...args: unknown[]) => state.genericExecute(...args) }),
}));
vi.mock('../../lib/commandWorkspaceTabWails', () => ({
  createCommandWorkspaceTabWailsPort: () => ({
    beginUICommand, takeUICommand, completeUICommand, getUICommandResult, cancelUICommand,
    commitBackendCommand: state.commitBackendCommand,
  }),
}));
vi.mock('../pickers/ProfilePicker', () => ({ ProfilePicker: () => null }));
vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (name: string, callback: (payload?: unknown) => void) => {
    deckEvents.set(name, callback);
    return () => {
      if (deckEvents.get(name) === callback) deckEvents.delete(name);
    };
  },
}));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({
  GetActiveWorkspace: () => state.chat.getActiveWorkspace(),
}));
vi.mock('../../store/workspaceChatModalStore', () => ({
  canPrepareWorkspaceChatOpen: () => state.chat.canPrepare(),
  prepareWorkspaceChatOpen: () => state.chat.prepare(),
  registerWorkspaceChatCommandDispatcher: (callback: (tabID: string) => void) => {
    state.chat.register(callback);
    return () => undefined;
  },
}));

beforeEach(() => {
  paletteContextScope?.dispose();
  paletteContextScope = null;
  state.workspaceListeners.clear();
  vi.resetAllMocks();
  localStorage.clear();
  locationState.pathname = '/';
  state.auth.user = { userId: 'user-a', sessionId: 'session-a' };
  deckEvents.clear();
  state.loadMap.mockResolvedValue({ generation: 'g-default', bindings: [{
    shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] },
    commandId: 'navigation.palette.open', handler: 'local_ui',
  }, {
    shortcut: { version: 1, code: 'F1', modifiers: [] },
    commandId: 'navigation.help.open', handler: 'local_ui',
  }],
    ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
    localPaletteCommands: catalog.filter(item => item.id !== 'workspace.list').map(item => item.id),
  });
  listCommandCatalog.mockResolvedValue(catalog);
  state.describeCommandCatalogItem.mockReset();
  state.getRuntimeToolCatalog.mockReset().mockResolvedValue([]);
  state.genericExecute.mockReset();
  state.chat.canPrepare.mockReset().mockReturnValue(true);
  state.chat.prepare.mockReset();
  state.chat.register.mockReset();
  state.chat.getActiveWorkspace.mockReset();
  state.chat.lease.isCurrent.mockReset().mockReturnValue(true);
  state.chat.lease.dispose.mockReset();
  state.chat.lease.present.mockReset();
  state.workspace.setActiveTab.mockReset();
});

function createExternalUIServiceFixture() {
  const owner: ExternalUIOwnerProof = { userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' };
  const initialTarget: ExternalUIDestination = {
    workspaceId: owner.workspaceId,
    tabId: 'tab-a',
    surface: { surfaceType: 'toolbar', surfaceId: 'command-toolbar', snapshotVersion: 'stale-before-topbar' },
  };
  let revision = 0;
  let snapshot: ExternalUIConnectionStatus = {
    state: 'connected', owner, target: initialTarget, connectionId: 'connection-a', generation: '3',
    targetSnapshotId: 'target-stale', contextVersion: 'context-stale',
    expiresAt: new Date(Date.now() + 60_000).toISOString(),
  };
  const statusListeners = new Set<() => void>();
  const readyListeners = new Set<(event: ExternalUICommandReadyEvent) => void>();
  const order: string[] = [];
  const service: ExternalUIConnectionService = {
    refresh: vi.fn(async () => { statusListeners.forEach(listener => listener()); return snapshot; }),
    begin: vi.fn(async () => ({ invitation: 'opaque-invite', expiresAt: new Date(Date.now() + 60_000).toISOString() })),
    publishContext: vi.fn(async (target) => {
      revision += 1;
      order.push(`publish:${target.surface.snapshotVersion}`);
      snapshot = {
        ...snapshot,
        target,
        targetSnapshotId: `target-${revision}`,
        contextVersion: `context-${revision}`,
      };
      statusListeners.forEach(listener => listener());
      return snapshot;
    }),
    heartbeat: vi.fn(async () => snapshot),
    disconnect: vi.fn(async () => undefined),
    take: vi.fn(async (event) => {
      order.push('take');
      return {
        invocationId: event.invocationId,
        commandId: event.commandId,
        arguments: {},
        receiptId: `receipt-${event.invocationId}`,
        targetSnapshotId: event.targetSnapshotId,
        contextVersion: event.contextVersion,
        target: snapshot.target!,
      } satisfies TakeExternalUICommandResult;
    }),
    complete: vi.fn(async (_event, _take, outcome: ExternalUICommandOutcome) => {
      order.push(`complete:${outcome}`);
      return true;
    }),
    getSnapshot: () => snapshot,
    subscribe: listener => { statusListeners.add(listener); return () => statusListeners.delete(listener); },
    subscribeReady: listener => { readyListeners.add(listener); return () => readyListeners.delete(listener); },
    dispose: vi.fn(),
  };
  return {
    service,
    emitReady(commandId: string) {
      const event: ExternalUICommandReadyEvent = {
        connectionId: snapshot.connectionId!, generation: snapshot.generation!,
        invocationId: '018f2d3c-4b5a-7c8d-9e0f-123456789abc',
        targetSnapshotId: snapshot.targetSnapshotId!, contextVersion: snapshot.contextVersion!, commandId,
      };
      readyListeners.forEach(listener => listener(event));
      return event;
    },
    getSnapshot: () => snapshot,
    order,
  };
}

describe('Pickers de chat — paleta e registro reais', () => {
  function registerSurface(root: HTMLElement, open: (id: string) => boolean, conversationId = 'conversation-a', subscribe?: (onChange: () => void) => () => void) {
    return registerChatPickerSurface({
      root, workspaceId: 'workspace-a', tabId: 'tab-a', conversationId,
      ownerId: 'user-a', sessionId: 'session-a', allowedCommandIds: CHAT_PICKER_COMMAND_IDS,
      instanceId: 'chat-test-instance', generation: conversationId,
      isActive: () => true, isCurrent: () => true, isRouteCurrent: pathname => pathname === '/', canOpen: () => true, open, subscribe,
    });
  }

  async function mount() {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({
      generation: 'chat-map', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [], localPaletteCommands: [...CHAT_PICKER_COMMAND_IDS],
    });
    state.listCommandCatalog.mockResolvedValue(CHAT_PICKER_COMMAND_IDS.map(id => ({
      id, name: id, available: true,
    })));
    const view = render(<><Topbar /><div data-testid="chat-root"><textarea /></div></>);
    const root = screen.getByTestId('chat-root');
    const open = vi.fn(() => true);
    const unregister = registerSurface(root, open);
    await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
    return { view, root, open, unregister };
  }

  it.each(CHAT_PICKER_COMMAND_IDS)('abre %s uma vez depois de fechar a busca, sem execução durável', async commandID => {
    const { view, open, unregister } = await mount();
    try {
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
      await user.type(search, commandID);
      await user.keyboard('{ArrowDown}{Enter}');
      await waitFor(() => expect(open).toHaveBeenCalledExactlyOnceWith(commandID));
      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
      expect(beginUICommand).not.toHaveBeenCalled();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
      expect(state.dispatchLocalCommandKey).not.toHaveBeenCalled();
    } finally { unregister(); view.unmount(); }
  });

  it('dispõe o lease capturado ao tabular para fora das ações sem restaurar foco ao acionador', async () => {
    const unsubscribe = vi.fn();
    const subscribe = vi.fn((_onChange: () => void) => unsubscribe);
    const { view, root, unregister } = await mount();
    unregister();
    const unregisterWithSubscription = registerSurface(root, vi.fn(() => true), 'conversation-a', subscribe);
    try {
      const user = userEvent.setup();
      const trigger = screen.getByRole('button', { name: 'commandPalette.title' });
      await user.click(trigger);
      const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
      await waitFor(() => expect(search).toHaveFocus());
      expect(subscribe).toHaveBeenCalledOnce();

      await user.tab();
      await user.tab();
      await user.tab();
      const naturalTabTarget = document.activeElement;
      await waitFor(() => expect(screen.queryByRole('combobox')).not.toBeInTheDocument());
      expect(unsubscribe).toHaveBeenCalledOnce();
      expect(document.activeElement).toBe(naturalTabTarget);
      expect(document.activeElement).not.toBe(trigger);
    } finally {
      unregisterWithSubscription();
      unregister();
      view.unmount();
    }
  });

  it('troca de instância/conversa durante a busca não redireciona a seleção ao novo chat', async () => {
    const { view, root, open, unregister } = await mount();
    let unregisterReplacement: () => void = () => undefined;
    const replacementOpen = vi.fn(() => true);
    try {
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
      unregister();
      unregisterReplacement = registerSurface(root, replacementOpen, 'conversation-b');
      await user.type(search, 'chat.model.open');
      await user.keyboard('{ArrowDown}{Enter}');
      expect(open).not.toHaveBeenCalled();
      expect(replacementOpen).not.toHaveBeenCalled();
      expect(beginUICommand).not.toHaveBeenCalled();
    } finally { unregisterReplacement(); unregister(); view.unmount(); }
  });

  it.each(['chat.pinned.open', 'chat.tokens.open'] as const)('%s usa a mesma apresentação por botão, tecla e Deck sem ledger', async commandID => {
    const { view, root, open, unregister } = await mount();
    try {
      const input = root.querySelector('textarea')!;
      input.focus();
      expect(requestChatPresentationCommand(commandID, 'wrong-instance')).toBe(false);
      expect(open).not.toHaveBeenCalled();
      expect(requestChatPresentationCommand(commandID, 'chat-test-instance')).toBe(true);
      expect(open).toHaveBeenCalledExactlyOnceWith(commandID);
      state.loadMap.mockResolvedValue({
        generation: 'chat-map', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
        bindings: [{ shortcut: { version: 1, code: 'KeyY', modifiers: ['Control'] }, commandId: commandID, handler: 'local_ui' }],
        localPaletteCommands: [...CHAT_PICKER_COMMAND_IDS],
      });
      await act(async () => { deckEvents.get('command:keyboard-map-changed')?.(); await Promise.resolve(); });
      fireEvent.keyDown(input, { key: 'y', code: 'KeyY', ctrlKey: true });
      fireEvent.keyDown(input, { key: 'y', code: 'KeyY', ctrlKey: true, repeat: true });
      fireEvent.keyUp(input, { key: 'y', code: 'KeyY', ctrlKey: true });
      expect(open.mock.calls).toEqual([[commandID], [commandID]]);
      act(() => deckEvents.get('command:deck-local-ui')?.({ commandId: commandID,
        generation: 'chat-map', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' }));
      expect(open.mock.calls).toEqual([[commandID], [commandID], [commandID]]);
      expect(beginUICommand).not.toHaveBeenCalled();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
      expect(state.dispatchLocalCommandKey).not.toHaveBeenCalled();
      state.auth.user = { userId: 'other', sessionId: 'other' };
      expect(requestChatPresentationCommand(commandID, 'chat-test-instance')).toBe(false);
      expect(open).toHaveBeenCalledTimes(3);
    } finally { unregister(); view.unmount(); }
  });

  function ModalChatProbe({ open }: { open: (id: string) => boolean }) {
    const root = useRef<HTMLDivElement>(null);
    const modalId = useModalId();
    useEffect(() => {
      if (!root.current || !modalId) return;
      return registerChatPickerSurface({
        root: root.current, modalId, workspaceId: 'workspace-a', tabId: 'tab-a',
        ownerId: 'user-a', sessionId: 'session-a', allowedCommandIds: CHAT_PICKER_COMMAND_IDS,
        conversationId: 'conversation-a', instanceId: 'modal-chat-probe', generation: '1',
        isActive: () => true, isCurrent: () => true, isRouteCurrent: pathname => pathname === '/', canOpen: () => true, open,
      });
    }, [modalId, open]);
    return <div ref={root}><textarea aria-label="modal composer" /></div>;
  }

  it.each(['chat.pinned.open', 'chat.tokens.open'] as const)('%s respeita o chat modal e bloqueia uma segunda barreira', async commandID => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({
      generation: 'chat-map', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [{ shortcut: { version: 1, code: 'KeyY', modifiers: ['Control'] }, commandId: commandID, handler: 'local_ui' }],
      localPaletteCommands: [...CHAT_PICKER_COMMAND_IDS],
    });
    const open = vi.fn(() => true);
    const view = render(<><Topbar /><Modal isOpen title="Chat" onClose={() => {}}><ModalChatProbe open={open} /></Modal></>);
    let upper: ReturnType<typeof render> | undefined;
    try {
      await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
      const input = screen.getByRole('textbox', { name: 'modal composer' });
      input.focus();
      fireEvent.keyDown(input, { key: 'y', code: 'KeyY', ctrlKey: true });
      fireEvent.keyUp(input, { key: 'y', code: 'KeyY', ctrlKey: true });
      expect(open).toHaveBeenCalledExactlyOnceWith(commandID);
      expect(requestChatPresentationCommand(commandID, 'modal-chat-probe')).toBe(true);
      expect(open).toHaveBeenCalledTimes(2);
      upper = render(<Modal isOpen title="Other dialog" onClose={() => {}}><button>Other action</button></Modal>);
      fireEvent.keyDown(document.activeElement!, { key: 'y', code: 'KeyY', ctrlKey: true });
      fireEvent.keyUp(document.activeElement!, { key: 'y', code: 'KeyY', ctrlKey: true });
      expect(requestChatPresentationCommand(commandID, 'modal-chat-probe')).toBe(false);
      act(() => deckEvents.get('command:deck-local-ui')?.({ commandId: commandID,
        generation: 'chat-map', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' }));
      expect(open).toHaveBeenCalledTimes(2);
      expect(beginUICommand).not.toHaveBeenCalled();
      expect(state.dispatchLocalCommandKey).not.toHaveBeenCalled();
    } finally { upper?.unmount(); view.unmount(); }
  });

  it('modal real autoriza só os pickers do chat topmost e uma segunda barreira bloqueia teclado e Deck', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({
      generation: 'chat-map', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [
        { shortcut: { version: 1, code: 'KeyM', modifiers: ['Control'] }, commandId: 'chat.model.open', handler: 'local_ui' },
        { shortcut: { version: 1, code: 'KeyC', modifiers: ['Alt'] }, commandId: 'navigation.settings.open', handler: 'local_ui' },
      ], localPaletteCommands: [...CHAT_PICKER_COMMAND_IDS],
    });
    const open = vi.fn(() => true);
    const view = render(<><Topbar /><Modal isOpen title="Chat modal" onClose={() => undefined}>
      <ModalChatProbe open={open} />
    </Modal></>);
    let upper: ReturnType<typeof render> | undefined;
    try {
      await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
      const input = screen.getByRole('textbox', { name: 'modal composer' });
      input.focus();
      fireEvent.keyDown(input, { key: 'm', code: 'KeyM', ctrlKey: true });
      fireEvent.keyUp(input, { key: 'm', code: 'KeyM', ctrlKey: true });
      expect(open).toHaveBeenCalledExactlyOnceWith('chat.model.open');
      fireEvent.keyDown(input, { key: 'c', code: 'KeyC', altKey: true });
      expect(state.navigate).not.toHaveBeenCalled();
      const deck = () => deckEvents.get('command:deck-local-ui')?.({
        commandId: 'chat.history.open', generation: 'chat-map', userId: 'user-a',
        sessionId: 'session-a', workspaceId: 'workspace-a',
      });
      act(deck);
      expect(open).toHaveBeenLastCalledWith('chat.history.open');
      expect(open).toHaveBeenCalledTimes(2);
      upper = render(<Modal isOpen title="Confirmação" onClose={() => undefined}><button>confirmar</button></Modal>);
      const button = screen.getByRole('button', { name: 'confirmar' });
      button.focus();
      fireEvent.keyDown(button, { key: 'm', code: 'KeyM', ctrlKey: true });
      act(deck);
      expect(open).toHaveBeenCalledTimes(2);
      upper.unmount();
      upper = undefined;
      input.focus();
      act(deck);
      expect(open).toHaveBeenCalledTimes(3);
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
      expect(beginUICommand).not.toHaveBeenCalled();
    } finally { upper?.unmount(); view.unmount(); }
  });
});

describe('Apresentação do editor — dispatcher e paleta reais', () => {
  function registerEditor(root: HTMLElement, open: (id: string) => boolean, documentId = 'doc-a') {
    return registerEditorPresentationSurface({
      root, workspaceId: 'workspace-a', ownerId: 'user-a', sessionId: 'session-a',
      tabId: 'tab-a', documentId, instanceId: 'editor-test', generation: documentId,
      allowedCommandIds: EDITOR_PRESENTATION_COMMAND_IDS,
      isActive: () => true, isCurrent: () => true,
      isRouteCurrent: pathname => pathname === '/', canOpen: () => true, open,
    });
  }

  async function mountEditor(commandID: string, code = 'KeyY') {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({
      generation: 'editor-map', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [{ shortcut: { version: 1, code, modifiers: code === 'F5' ? [] : ['Control'] }, commandId: commandID, handler: 'local_ui' }],
      localPaletteCommands: [...EDITOR_PRESENTATION_COMMAND_IDS],
    });
    listCommandCatalog.mockResolvedValue(EDITOR_PRESENTATION_COMMAND_IDS.map(id => ({ id, name: id, available: true })));
    const view = render(<><Topbar /><section data-testid="editor-root"><textarea aria-label="editor text" /></section></>);
    const root = screen.getByTestId('editor-root');
    const input = screen.getByRole('textbox', { name: 'editor text' });
    const open = vi.fn(() => true);
    const unregister = registerEditor(root, open);
    await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
    return { view, root, input, open, unregister };
  }

  it.each(EDITOR_PRESENTATION_COMMAND_IDS)('%s executa pelo teclado e Deck sem ledger', async commandID => {
    const { view, input, open, unregister } = await mountEditor(commandID);
    try {
      input.focus();
      fireEvent.keyDown(input, { key: 'y', code: 'KeyY', ctrlKey: true });
      expect(open).toHaveBeenCalledExactlyOnceWith(commandID);
      fireEvent.keyDown(input, { key: 'y', code: 'KeyY', ctrlKey: true, repeat: true });
      fireEvent.keyUp(input, { key: 'y', code: 'KeyY', ctrlKey: true });
      // Navigation repeats by contract; presentation actions must not repeat.
      const keyboardCalls = ['editor.table.cell.next', 'editor.table.cell.previous'].includes(commandID) ? 2 : 1;
      expect(open.mock.calls).toEqual(Array.from({ length: keyboardCalls }, () => [commandID]));
      act(() => deckEvents.get('command:deck-local-ui')?.({ commandId: commandID,
        generation: 'editor-map', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' }));
      expect(open.mock.calls).toEqual(Array.from({ length: keyboardCalls + 1 }, () => [commandID]));
      expect(state.dispatchLocalCommandKey).not.toHaveBeenCalled();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
      expect(beginUICommand).not.toHaveBeenCalled();
    } finally { unregister(); view.unmount(); }
  });

  it.each(['editor', 'other', 'unknown', 'suppressed'] as const)('Alt+I usa projeção contextual do host: %s', async context => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const shortcut = { version: 1, code: 'KeyI', modifiers: ['Alt'] };
    state.loadMap.mockResolvedValue({
      generation: 'context-map', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [], localPaletteCommands: [...EDITOR_PRESENTATION_COMMAND_IDS, 'navigation.data.import.open'],
      contextualBindings: [{ shortcut, bySurface: { editor: context === 'suppressed' ? null : {
        shortcut, commandId: 'editor.menu.insert.open', handler: 'local_ui',
      } }, fallback: { shortcut, commandId: 'navigation.data.import.open', handler: 'local_ui' } }],
    });
    if (context === 'other') {
      locationState.pathname = '/settings';
      // Production obtains the route from CommandContextProvider; this fixture
      // mocks that provider, so it must supply the same trusted route explicitly.
      paletteContextScope = createCommandContextScope(undefined, locationState.pathname);
    }
    const view = render(<><Topbar /><section data-testid="context-editor"><textarea aria-label="context text" /></section></>);
    const root = screen.getByTestId('context-editor');
    const open = vi.fn(() => true);
    const unregister = context === 'editor' || context === 'suppressed' ? registerEditor(root, open) : () => {};
    try {
      await act(async () => { await Promise.resolve(); });
      const target = context === 'editor' || context === 'suppressed'
        ? screen.getByRole('textbox', { name: 'context text' })
        : screen.getByRole('button', { name: 'commandPalette.title' });
      target.focus();
      fireEvent.keyDown(target, { key: 'i', code: 'KeyI', altKey: true });
      fireEvent.keyUp(target, { key: 'i', code: 'KeyI', altKey: true });
      if (context === 'editor') expect(open).toHaveBeenCalledExactlyOnceWith('editor.menu.insert.open');
      else expect(open).not.toHaveBeenCalled();
      if (context === 'other') expect(navigate).toHaveBeenCalledExactlyOnceWith('/settings/data?action=import');
      else expect(navigate).not.toHaveBeenCalled();
      expect(state.dispatchLocalCommandKey).not.toHaveBeenCalled();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    } finally { unregister(); view.unmount(); }
  });

  it.each(EDITOR_PRESENTATION_COMMAND_IDS)('%s abre pela paleta após restaurar foco', async commandID => {
    const { view, open, unregister } = await mountEditor(commandID);
    try {
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
      await user.type(search, commandID);
      await user.keyboard('{ArrowDown}{Enter}');
      await waitFor(() => expect(open).toHaveBeenCalledExactlyOnceWith(commandID));
      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
      expect(beginUICommand).not.toHaveBeenCalled();
    } finally { unregister(); view.unmount(); }
  });

  it('não redireciona a paleta se o documento/registro muda durante a busca', async () => {
    const { view, root, open, unregister } = await mountEditor('editor.menu.file.open');
    let cleanupReplacement = () => {};
    const replacement = vi.fn(() => true);
    try {
      const user = userEvent.setup();
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
      unregister();
      cleanupReplacement = registerEditor(root, replacement, 'doc-b');
      await user.type(search, 'editor.menu.file.open');
      await user.keyboard('{ArrowDown}{Enter}');
      expect(open).not.toHaveBeenCalled();
      expect(replacement).not.toHaveBeenCalled();
    } finally { cleanupReplacement(); unregister(); view.unmount(); }
  });

  it('F5 configurado abre apresentação uma vez, sem repetir ou atravessar modal', async () => {
    const { view, input, open, unregister } = await mountEditor('editor.presentation.fullscreen', 'F5');
    let modal: ReturnType<typeof render> | undefined;
    try {
      input.focus();
      const first = new KeyboardEvent('keydown', { key: 'F5', code: 'F5', bubbles: true, cancelable: true });
      act(() => input.dispatchEvent(first));
      expect(first.defaultPrevented).toBe(true);
      expect(open).toHaveBeenCalledOnce();
      fireEvent.keyUp(input, { key: 'F5', code: 'F5' });
      modal = render(<Modal isOpen title="Barreira editor" onClose={() => undefined}><button>Decidir</button></Modal>);
      const button = screen.getByRole('button', { name: 'Decidir' });
      button.focus();
      fireEvent.keyDown(button, { key: 'F5', code: 'F5' });
      act(() => deckEvents.get('command:deck-local-ui')?.({ commandId: 'editor.presentation.fullscreen',
        generation: 'editor-map', userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' }));
      expect(open).toHaveBeenCalledOnce();
    } finally { modal?.unmount(); unregister(); view.unmount(); }
  });

  it.each([
    ['editor.slides.open', { key: 's', code: 'KeyS', altKey: true }],
    ['editor.presentation.fullscreen', { key: 'F5', code: 'F5' }],
  ] as const)('respeita remapeamento e supressão de %s sem fallback', async (commandID, previousKey) => {
    const { view, input, open, unregister } = await mountEditor(commandID);
    try {
      input.focus();
      fireEvent.keyDown(input, previousKey);
      fireEvent.keyUp(input, previousKey);
      expect(open).not.toHaveBeenCalled();
      fireEvent.keyDown(input, { key: 'y', code: 'KeyY', ctrlKey: true });
      fireEvent.keyUp(input, { key: 'y', code: 'KeyY', ctrlKey: true });
      expect(open).toHaveBeenCalledExactlyOnceWith(commandID);
      state.loadMap.mockResolvedValue({ generation: 'editor-suppressed', ownerId: 'user-a',
        sessionId: 'session-a', workspaceId: 'workspace-a', bindings: [], localPaletteCommands: [] });
      await act(async () => { deckEvents.get('command:keyboard-map-changed')?.(); });
      fireEvent.keyDown(input, { key: 'y', code: 'KeyY', ctrlKey: true });
      fireEvent.keyUp(input, { key: 'y', code: 'KeyY', ctrlKey: true });
      fireEvent.keyDown(input, previousKey);
      fireEvent.keyUp(input, previousKey);
      expect(open).toHaveBeenCalledOnce();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    } finally { unregister(); view.unmount(); }
  });

  it.each(['owner', 'generation', 'ime', 'route', 'detached'] as const)('recusa Deck com %s inválido', async fault => {
    const commandId = 'editor.slides.open';
    const { view, root, input, open, unregister } = await mountEditor(commandId);
    const originalParent = root.parentElement;
    try {
      input.focus();
      if (fault === 'ime') fireEvent.compositionStart(input);
      if (fault === 'route') { locationState.pathname = '/settings'; view.rerender(<Topbar />); }
      if (fault === 'detached') root.remove();
      act(() => deckEvents.get('command:deck-local-ui')?.({ commandId,
        generation: fault === 'generation' ? 'stale' : 'editor-map',
        userId: fault === 'owner' ? 'other-user' : 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a' }));
      expect(open).not.toHaveBeenCalled();
    } finally {
      if (fault === 'ime') fireEvent.compositionEnd(input);
      if (fault === 'detached') originalParent?.appendChild(root);
      unregister(); view.unmount();
    }
  });
});

describe('Criação de abas — Topbar, Toolbar e Menu reais', () => {
  const choices = ['chat', 'editor', 'terminal', 'tasklist'];

  it.each(['/','/settings'] as const)('Ctrl+Shift+N cria workspace uma vez no teclado em %s, sem store legado', async (pathname) => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    locationState.pathname = pathname;
    state.loadMap.mockResolvedValue({
      generation: `g-workspace-create-${pathname}`, ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [{ shortcut: { version: 1, code: 'KeyN', modifiers: ['Control', 'Shift'] }, commandId: 'workspace.create', handler: 'contextual' }],
      localPaletteCommands: catalog.map(item => item.id),
    });
    state.beginLocalCommandUIKey.mockResolvedValue({ ticket: 'workspace-ticket', invocationId: 'workspace-inv', commandId: 'workspace.create' });
    state.takeUICommand.mockResolvedValue({ ticket: 'workspace-ticket', invocationId: 'workspace-inv', commandId: 'workspace.create', handoffId: 'workspace-handoff' });
    state.getUICommandResult.mockResolvedValue({ invocationId: 'workspace-inv', status: 'succeeded' });
    state.commitBackendCommand.mockResolvedValue(undefined);
    render(<Topbar />);
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
    await user.keyboard('{Control>}{Shift>}n{/Shift}{/Control}');
    await waitFor(() => expect(state.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('workspace-ticket', 'workspace-handoff'));
    expect(state.workspace.createWorkspace).not.toHaveBeenCalled();
    expect(beginUICommand).not.toHaveBeenCalled();
    expect(state.beginLocalCommandUIKey).toHaveBeenCalledTimes(1);
    expect(state.announce).toHaveBeenCalledWith('workspace.announce.workspaceCreatedCommand');
  });

  it('seleciona Novo workspace no Menu real e mantém o handoff após fechar e restaurar foco', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.beginUICommand.mockResolvedValue({ ticket: 'menu-workspace-ticket', invocationId: 'menu-workspace-inv', commandId: 'workspace.create' });
    state.takeUICommand.mockResolvedValue({
      ticket: 'menu-workspace-ticket', invocationId: 'menu-workspace-inv', commandId: 'workspace.create', handoffId: 'menu-workspace-handoff',
    });
    state.getUICommandResult.mockResolvedValue({ invocationId: 'menu-workspace-inv', status: 'succeeded' });
    state.commitBackendCommand.mockResolvedValue(undefined);
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    document.body.appendChild(root);
    const view = render(<div className="workspace-layout"><Topbar /></div>);
    try {
      await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
      const picker = screen.getByRole('button', { name: 'workspace.workspaceList' });
      picker.focus();
      fireEvent.contextMenu(picker, { clientX: 12, clientY: 12 });
      const menu = await screen.findByRole('menu', { name: 'workspace.workspaceOptions' });
      await user.click(await within(menu).findByRole('menuitem', { name: /workspace\.newWorkspace/ }));

      await waitFor(() => expect(state.commitBackendCommand)
        .toHaveBeenCalledExactlyOnceWith('menu-workspace-ticket', 'menu-workspace-handoff'));
      expect(state.cancelUICommand).not.toHaveBeenCalled();
      expect(state.workspace.createWorkspace).not.toHaveBeenCalled();
      expect(state.workspace.switchWorkspace).not.toHaveBeenCalled();
      expect(state.announce).toHaveBeenCalledWith('workspace.announce.workspaceCreatedCommand');
      expect(screen.queryByRole('menu', { name: 'workspace.workspaceOptions' })).not.toBeInTheDocument();
    } finally {
      view.unmount();
      root.remove();
    }
  });

  it('seleciona Novo workspace no WorkspaceToolbar real e mantém o handoff após fechar o menu', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.beginUICommand.mockResolvedValue({ ticket: 'toolbar-workspace-ticket', invocationId: 'toolbar-workspace-inv', commandId: 'workspace.create' });
    state.takeUICommand.mockResolvedValue({
      ticket: 'toolbar-workspace-ticket', invocationId: 'toolbar-workspace-inv', commandId: 'workspace.create', handoffId: 'toolbar-workspace-handoff',
    });
    state.getUICommandResult.mockResolvedValue({ invocationId: 'toolbar-workspace-inv', status: 'succeeded' });
    state.commitBackendCommand.mockResolvedValue(undefined);
    const view = render(<WorkspaceTabCreationMenuProvider><div className="workspace-layout"><Topbar /><WorkspaceToolbar /></div></WorkspaceTabCreationMenuProvider>);
    try {
      await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
      const optionsButton = screen.getByRole('button', { name: 'workspace.workspaceOptions' });
      optionsButton.focus();
      await user.click(optionsButton);
      const menu = await screen.findByRole('menu', { name: 'workspace.workspaceOptions' });
      await user.click(await within(menu).findByRole('menuitem', { name: /workspace\.newWorkspace/ }));

      await waitFor(() => expect(state.commitBackendCommand)
        .toHaveBeenCalledExactlyOnceWith('toolbar-workspace-ticket', 'toolbar-workspace-handoff'));
      expect(state.cancelUICommand).not.toHaveBeenCalled();
      expect(state.workspace.createWorkspace).not.toHaveBeenCalled();
      expect(state.workspace.switchWorkspace).not.toHaveBeenCalled();
      expect(state.announce).toHaveBeenCalledWith('workspace.announce.workspaceCreatedCommand');
      expect(screen.queryByRole('menu', { name: 'workspace.workspaceOptions' })).not.toBeInTheDocument();
    } finally {
      view.unmount();
    }
  });

  it('descarta Novo workspace do WorkspaceToolbar quando o modal abre antes do handoff', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    let resolveTake!: (response: unknown) => void;
    state.beginUICommand.mockResolvedValue({ ticket: 'toolbar-stale-ticket', invocationId: 'toolbar-stale-inv', commandId: 'workspace.create' });
    state.takeUICommand.mockImplementation(() => new Promise(resolve => { resolveTake = resolve; }));
    state.getUICommandResult.mockResolvedValue({ invocationId: 'toolbar-stale-inv', status: 'succeeded' });
    state.commitBackendCommand.mockResolvedValue(undefined);
    const view = render(<WorkspaceTabCreationMenuProvider><div className="workspace-layout"><Topbar /><WorkspaceToolbar /></div></WorkspaceTabCreationMenuProvider>);
    try {
      await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
      await user.click(screen.getByRole('button', { name: 'workspace.workspaceOptions' }));
      const menu = await screen.findByRole('menu', { name: 'workspace.workspaceOptions' });
      await user.click(await within(menu).findByRole('menuitem', { name: /workspace\.newWorkspace/ }));
      await waitFor(() => expect(state.takeUICommand).toHaveBeenCalledWith('toolbar-stale-ticket'));

      registerOpenModal('workspace-create-stale');
      await act(async () => {
        resolveTake({ ticket: 'toolbar-stale-ticket', invocationId: 'toolbar-stale-inv', commandId: 'workspace.create', handoffId: 'toolbar-stale-handoff' });
        await Promise.resolve();
      });
      expect(state.commitBackendCommand).not.toHaveBeenCalled();
      expect(state.workspace.createWorkspace).not.toHaveBeenCalled();
      expect(state.workspace.switchWorkspace).not.toHaveBeenCalled();
    } finally {
      unregisterOpenModal('workspace-create-stale');
      view.unmount();
    }
  });

  it('anuncia sucesso de workspace.create pelo Deck sem Begin e sem duplicar commit', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.takeUICommand.mockResolvedValue({
      ticket: 'deck-workspace-ticket', invocationId: 'deck-workspace-inv', commandId: 'workspace.create', handoffId: 'deck-workspace-handoff',
    });
    state.getUICommandResult.mockResolvedValue({ invocationId: 'deck-workspace-inv', status: 'succeeded' });
    state.commitBackendCommand.mockResolvedValue(undefined);
    const root = document.createElement('div');
    root.className = 'workspace-layout';
    document.body.appendChild(root);
    const view = render(<div className="workspace-layout"><Topbar /></div>);
    try {
      await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
      screen.getByRole('button', { name: 'commandPalette.title' }).focus();
      const reservation = deckEvents.get('command:deck-ui-reservation');
      expect(reservation).toBeDefined();
      await act(async () => {
        reservation?.({ ticket: 'deck-workspace-ticket', invocationId: 'deck-workspace-inv', commandId: 'workspace.create' });
      });
      await waitFor(() => expect(state.commitBackendCommand)
        .toHaveBeenCalledExactlyOnceWith('deck-workspace-ticket', 'deck-workspace-handoff'));
      expect(beginUICommand).not.toHaveBeenCalled();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
      expect(state.announce).toHaveBeenCalledWith('workspace.announce.workspaceCreatedCommand');
    } finally {
      view.unmount();
      root.remove();
    }
  });
  async function mountCreation() {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({
      generation: 'g-create', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: choices.map((type, index) => ({
        commandId: `workspace.tab.${type}.create`, handler: 'contextual',
        shortcut: { version: 2, steps: [
          { code: 'KeyN', modifiers: ['Control'] },
          { code: ['KeyC', 'KeyE', 'KeyR', 'KeyT'][index], modifiers: [] },
        ] },
      })),
    });
    listCommandCatalog.mockResolvedValue(choices.map(type => ({ id: `workspace.tab.${type}.create`, name: type, available: true })));
    let admittedCommand = '';
    beginUICommand.mockImplementation(async (id: string) => {
      admittedCommand = id;
      return { ticket: 'ticket', invocationId: 'invocation', commandId: id };
    });
    state.beginLocalCommandUIKey.mockImplementation(async (_generation, shortcut) => {
      admittedCommand = `workspace.tab.${choices[['KeyC', 'KeyE', 'KeyR', 'KeyT'].indexOf(shortcut.steps[1].code)]}.create`;
      return { ticket: 'ticket', invocationId: 'invocation', commandId: admittedCommand };
    });
    takeUICommand.mockImplementation(async () => ({
      ticket: 'ticket', invocationId: 'invocation', commandId: admittedCommand, handoffId: 'handoff',
    }));
    getUICommandResult.mockResolvedValue({ invocationId: 'invocation', status: 'succeeded' });
    state.commitBackendCommand.mockResolvedValue(undefined);
    const view = render(<WorkspaceTabCreationMenuProvider><div className="workspace-layout">
      <Topbar /><WorkspaceToolbar />
    </div></WorkspaceTabCreationMenuProvider>);
    await act(async () => {});
    screen.getByRole('button', { name: 'workspace.newTab, Ctrl+N' }).focus();
    return view;
  }

  it.each(['KeyC', 'KeyE', 'KeyR', 'KeyT'])('conclui %s pelo ingresso teclado com Menu real', async (code) => {
    await mountCreation();
    fireEvent.keyDown(document.activeElement!, { key: 'n', code: 'KeyN', ctrlKey: true });
    const menu = await screen.findByRole('menu', { name: 'workspace.newTabMenu' });
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    fireEvent.keyUp(document.activeElement!, { key: 'n', code: 'KeyN' });
    fireEvent.keyDown(document.activeElement!, { key: code.slice(-1).toLowerCase(), code });
    fireEvent.keyUp(document.activeElement!, { code });
    await waitFor(() => expect(state.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff'));
    expect(state.beginLocalCommandUIKey).toHaveBeenCalledTimes(1);
    expect(beginUICommand).not.toHaveBeenCalled();
    expect(menu).not.toBeInTheDocument();
  });

  it('setas e Enter selecionam uma vez, sem timer roubar foco depois do commit', async () => {
    await mountCreation();
    fireEvent.keyDown(document.activeElement!, { key: 'n', code: 'KeyN', ctrlKey: true });
    await screen.findByRole('menu', { name: 'workspace.newTabMenu' });
    await waitFor(() => expect(screen.getByRole('menuitem', { name: /chat/ })).toHaveFocus());
    fireEvent.keyDown(document.activeElement!, { key: 'ArrowDown', code: 'ArrowDown' });
    await waitFor(() => expect(screen.getByRole('menuitem', { name: /editor/ })).toHaveFocus());
    fireEvent.keyDown(document.activeElement!, { key: 'Enter', code: 'Enter' });
    await waitFor(() => expect(state.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff'));
    expect(beginUICommand).toHaveBeenCalledExactlyOnceWith('workspace.tab.editor.create');
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    const destination = screen.getByRole('button', { name: 'commandPalette.title' });
    destination.focus();
    await act(async () => { await new Promise(resolve => setTimeout(resolve, 30)); });
    expect(destination).toHaveFocus();
  });

  it('botão e Escape usam o host compartilhado; clicar executa o comando escolhido', async () => {
    await mountCreation();
    const button = screen.getByRole('button', { name: 'workspace.newTab, Ctrl+N' });
    fireEvent.click(button);
    await screen.findByRole('menu', { name: 'workspace.newTabMenu' });
    fireEvent.keyDown(document.activeElement!, { key: 'Escape', code: 'Escape' });
    expect(screen.queryByRole('menu', { name: 'workspace.newTabMenu' })).not.toBeInTheDocument();
    expect(button).toHaveFocus();
    expect(beginUICommand).not.toHaveBeenCalled();
    fireEvent.click(button);
    fireEvent.click(await screen.findByRole('menuitem', { name: 'terminal, Ctrl+N R' }));
    await waitFor(() => expect(state.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('ticket', 'handoff'));
    expect(beginUICommand).toHaveBeenCalledExactlyOnceWith('workspace.tab.terminal.create');
  });

  it('segunda tecla rápida executa antes do catálogo e a resposta tardia não reabre o menu', async () => {
    await mountCreation();
    let resolveCatalog!: (items: unknown[]) => void;
    listCommandCatalog.mockReturnValueOnce(new Promise(resolve => { resolveCatalog = resolve; }));
    fireEvent.keyDown(document.activeElement!, { key: 'n', code: 'KeyN', ctrlKey: true });
    fireEvent.keyUp(document.activeElement!, { code: 'KeyN' });
    fireEvent.keyDown(document.activeElement!, { key: 'c', code: 'KeyC' });
    fireEvent.keyUp(document.activeElement!, { code: 'KeyC' });
    await waitFor(() => expect(state.commitBackendCommand).toHaveBeenCalledTimes(1));
    await act(async () => resolveCatalog([{ id: 'workspace.tab.chat.create', name: 'chat', available: true }]));
    expect(screen.queryByRole('menu', { name: 'workspace.newTabMenu' })).not.toBeInTheDocument();
    expect(state.beginLocalCommandUIKey).toHaveBeenCalledTimes(1);
  });

  it('abrir e fechar um modal entre os passos invalida a escolha, mesmo voltando à mesma tela', async () => {
    await mountCreation();
    fireEvent.keyDown(document.activeElement!, { key: 'n', code: 'KeyN', ctrlKey: true });
    await screen.findByRole('menu', { name: 'workspace.newTabMenu' });
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    document.body.append(overlay);
    registerOpenModal('creation-interruption');
    unregisterOpenModal('creation-interruption');
    overlay.remove();
    fireEvent.keyDown(document.activeElement!, { key: 'c', code: 'KeyC' });
    await act(async () => {});
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(state.commitBackendCommand).not.toHaveBeenCalled();
    expect(screen.queryByRole('menu', { name: 'workspace.newTabMenu' })).not.toBeInTheDocument();
  });

  it('timeout fecha a escolha e uma letra posterior não cria aba', async () => {
    await mountCreation();
    fireEvent.keyDown(document.activeElement!, { key: 'n', code: 'KeyN', ctrlKey: true });
    await screen.findByRole('menu', { name: 'workspace.newTabMenu' });
    await waitFor(() => expect(screen.queryByRole('menu', { name: 'workspace.newTabMenu' })).not.toBeInTheDocument(), { timeout: 2200 });
    fireEvent.keyDown(document.activeElement!, { key: 'c', code: 'KeyC' });
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(state.commitBackendCommand).not.toHaveBeenCalled();
  });
});

describe('Topbar palette — integração real do Combobox compartilhado', () => {
  it.each(['present', 'removed', 'disabled', 'owner-changed'] as const)('Escape restaura somente uma origem de foco ainda válida: %s', async mode => {
    const user = userEvent.setup();
    let usingFakeTimers = false;
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const source = document.createElement('textarea');
    document.body.append(source);
    const view = render(<Topbar />);
    try {
      await act(async () => { await Promise.resolve(); });
      source.focus();
      await user.keyboard('{Control>}k{/Control}');
      const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
      await waitFor(() => expect(search).toHaveFocus());
      if (mode === 'removed') source.remove();
      if (mode === 'disabled') source.disabled = true;
      if (mode === 'owner-changed') state.auth.user = { userId: 'other', sessionId: 'other-session' };
      // O popup fecha no commit, e o Combobox só chama onAfterDismiss após o
      // timer de foco de 10 ms. Controle apenas esse fechamento para evitar
      // uma asserção negativa antes do callback em máquinas lentas.
      vi.useFakeTimers();
      usingFakeTimers = true;
      fireEvent.keyDown(search, { key: 'Escape', code: 'Escape' });
      await act(async () => { await vi.advanceTimersByTimeAsync(10); });
      // O popup some no commit; o Combobox chama onAfterDismiss após seu cleanup assíncrono.
      expect(screen.queryByRole('combobox')).not.toBeInTheDocument();
      if (mode === 'present') expect(source).toHaveFocus();
      else if (mode === 'owner-changed') expect(source).not.toHaveFocus();
      else expect(screen.getByRole('button', { name: 'commandPalette.title' })).toHaveFocus();
      expect(beginUICommand).not.toHaveBeenCalled();
    } finally {
      if (usingFakeTimers) vi.useRealTimers();
      view.unmount();
      source.remove();
    }
  });

  it.each(['allowed', 'other-tab', 'aba', 'profile-aba', 'profile-match', 'profile-mismatch'] as const)('condição visual usa a origem real e recusa contexto alterado: %s', async mode => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const originalWorkspace = state.workspace.workspace;
    state.workspace.workspace = { id: 'workspace-a', name: 'Workspace', activeTabId: 'tab-a', tabs: [{ id: 'tab-a', type: 'chat' }] };
    Object.assign(state.workspace.workspace, { profile: 'dev' });
    paletteContextScope = createCommandContextScope(undefined, locationState.pathname);
    const root = document.createElement('div');
    const source = document.createElement('textarea');
    root.append(source);
    document.body.append(root);
    const unregister = paletteContextScope.registerSurface('tab-a', { current: root }, () => ({
      surfaceId: 'tab-a', surfaceType: 'chat', snapshotVersion: 'chat-v1',
    }));
    state.loadMap.mockResolvedValue({ generation: 'conditional-palette', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [{ shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] }, commandId: 'navigation.palette.open', handler: 'local_ui' }],
      localPaletteCommands: ['navigation.palette.open'], localPaletteConditions: [mode.startsWith('profile-') && mode !== 'profile-aba' ? {
        commandId: 'navigation.settings.open', bySurface: {}, fallback: false,
        byProfile: { [mode === 'profile-match' ? 'dev' : 'other']: {
          commandId: 'navigation.settings.open', bySurface: { chat: true }, fallback: false,
        } },
      } : {
        commandId: 'navigation.settings.open', bySurface: { chat: false },
        bySurfaceId: { chat: { [mode === 'other-tab' ? 'tab-b' : 'tab-a']: true } }, fallback: false,
      }],
    });
    listCommandCatalog.mockResolvedValue([{ id: 'navigation.settings.open', name: 'Configurações', available: true }]);
    const view = render(<Topbar />);
    try {
      await act(async () => { await Promise.resolve(); });
      source.focus();
      await user.keyboard('{Control>}k{/Control}');
      const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
      await user.type(search, 'Configurações');
      const option = await screen.findByRole('option', { name: /Configurações/ });
      if (mode === 'other-tab' || mode === 'profile-mismatch') expect(option).toHaveAttribute('aria-disabled', 'true');
      else expect(option).not.toHaveAttribute('aria-disabled', 'true');
      if (mode === 'aba' || mode === 'profile-aba') {
        act(() => {
          if (mode === 'aba') state.workspace.workspace.activeTabId = 'tab-b';
          else Object.assign(state.workspace.workspace, { profile: 'other' });
          state.workspaceListeners.forEach(listener => listener());
          state.workspace.workspace.activeTabId = 'tab-a';
          Object.assign(state.workspace.workspace, { profile: 'dev' });
          state.workspaceListeners.forEach(listener => listener());
        });
      }
      navigate.mockClear();
      await user.keyboard('{Enter}');
      if (mode === 'allowed' || mode === 'profile-match') await waitFor(() => expect(navigate).toHaveBeenCalledExactlyOnceWith('/settings'));
      else expect(navigate).not.toHaveBeenCalled();
      expect(beginUICommand).not.toHaveBeenCalled();
      expect(state.dispatchLocalCommandKey).not.toHaveBeenCalled();
    } finally {
      view.unmount(); unregister(); paletteContextScope?.dispose(); paletteContextScope = null;
      root.remove(); state.workspace.workspace = originalWorkspace;
    }
  });

  it('recusa seleção local quando a camada expira com a paleta já aberta', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const deadline = Date.now() + 60_000;
    state.loadMap.mockResolvedValue({ generation: 'expiring-palette', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      validUntil: deadline, bindings: [], localPaletteCommands: ['navigation.settings.open'] });
    listCommandCatalog.mockResolvedValue([{ id: 'navigation.settings.open', name: 'Configurações', available: true }]);
    const view = render(<Topbar />);
    let clock: ReturnType<typeof vi.spyOn> | undefined;
    try {
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
      await user.type(search, 'Configurações');
      const option = await screen.findByRole('option', { name: /Configurações/ });
      expect(option).not.toHaveAttribute('aria-disabled', 'true');
      expect(option).toHaveClass('highlighted');
      navigate.mockClear();
      // Deliberately do not run the expiry timer: selection rechecks the deadline.
      clock = vi.spyOn(Date, 'now').mockReturnValue(deadline + 1);
      await user.keyboard('{Enter}');
      expect(navigate).not.toHaveBeenCalled();
      expect(beginUICommand).not.toHaveBeenCalled();
    } finally { clock?.mockRestore(); view.unmount(); }
  });
  it.each(['/','/settings'] as const)('seleciona workspace.create pela paleta em %s e faz um único commit contextual', async (pathname) => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    locationState.pathname = pathname;
    listCommandCatalog.mockResolvedValueOnce([{ id: 'workspace.create', name: 'Novo workspace', available: true }]);
    beginUICommand.mockResolvedValue({ ticket: 'palette-workspace-ticket', invocationId: 'palette-workspace-inv', commandId: 'workspace.create' });
    takeUICommand.mockResolvedValue({ ticket: 'palette-workspace-ticket', invocationId: 'palette-workspace-inv', commandId: 'workspace.create', handoffId: 'palette-workspace-handoff' });
    getUICommandResult.mockResolvedValue({ invocationId: 'palette-workspace-inv', status: 'succeeded' });
    state.commitBackendCommand.mockResolvedValue(undefined);
    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
    await user.type(search, 'Novo workspace');
    await user.keyboard('{ArrowDown}{Enter}');
    await waitFor(() => expect(state.commitBackendCommand).toHaveBeenCalledExactlyOnceWith('palette-workspace-ticket', 'palette-workspace-handoff'));
    expect(state.workspace.createWorkspace).not.toHaveBeenCalled();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(beginUICommand).toHaveBeenCalledExactlyOnceWith('workspace.create');
  });

  it('executa F1 help pelo mapa efetivo, sem handoff', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({
      generation: 'g-f1-help', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [{ shortcut: { version: 1, code: 'F1', modifiers: [] }, commandId: 'navigation.help.open', handler: 'local_ui' }],
      localPaletteCommands: catalog.map(item => item.id),
    });
    render(<Topbar />);
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
    fireEvent.keyDown(window, { key: 'F1', code: 'F1', bubbles: true, cancelable: true });
    await waitFor(() => expect(navigate).toHaveBeenCalledExactlyOnceWith('/help'));
    expect(beginUICommand).not.toHaveBeenCalled();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
  });

  it.each(['button', 'input'] as const)('mantém a exceção modal de F1 com foco real no %s, sem fechar o portal', async (control) => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({
      generation: `g-f1-modal-${control}`, ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [{ shortcut: { version: 1, code: 'F1', modifiers: [] }, commandId: 'navigation.help.open', handler: 'local_ui' }],
      localPaletteCommands: catalog.map(item => item.id),
    });
    render(<>
      <Topbar />
      <Modal isOpen onClose={() => undefined} title="Real test modal" allowClose={false} returnFocusOnClose={false}>
        {control === 'button' ? <button>modal action</button> : <input aria-label="modal input" />}
      </Modal>
    </>);
    await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
    expect(isModalOpen()).toBe(true);
    const focused = control === 'button'
      ? screen.getByRole('button', { name: 'modal action' })
      : screen.getByRole('textbox', { name: 'modal input' });
    focused.focus();
    navigate.mockClear();
    const event = new KeyboardEvent('keydown', { key: 'F1', code: 'F1', bubbles: true, cancelable: true });
    focused.dispatchEvent(event);
    await waitFor(() => expect(navigate).toHaveBeenCalledExactlyOnceWith('/help'));
    expect(event.defaultPrevented).toBe(true);
    expect(screen.getByRole('dialog', { name: 'Real test modal' })).toBeInTheDocument();
    expect(document.activeElement).toBe(focused);
    expect(beginUICommand).not.toHaveBeenCalled();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
  });

  it('bloqueia comando arbitrário remapeado para F1 dentro do Modal real', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({
      generation: 'g-f1-modal-blocked', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [{ shortcut: { version: 1, code: 'F1', modifiers: [] }, commandId: 'navigation.settings.open', handler: 'local_ui' }],
      localPaletteCommands: catalog.map(item => item.id),
    });
    render(<>
      <Topbar />
      <Modal isOpen onClose={() => undefined} title="Real blocked modal" allowClose={false} returnFocusOnClose={false}>
        <button>modal action</button>
      </Modal>
    </>);
    await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
    const focused = screen.getByRole('button', { name: 'modal action' });
    focused.focus();
    navigate.mockClear();
    focused.dispatchEvent(new KeyboardEvent('keydown', { key: 'F1', code: 'F1', bubbles: true, cancelable: true }));
    expect(navigate).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(focused);
    expect(isModalOpen()).toBe(true);
    expect(beginUICommand).not.toHaveBeenCalled();
  });

  it('permite help remapeado para Alt+F1 no Modal real, sem dispensar o foco', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({
      generation: 'g-alt-f1-modal-help', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [{ shortcut: { version: 1, code: 'F1', modifiers: ['Alt'] }, commandId: 'navigation.help.open', handler: 'local_ui' }],
      localPaletteCommands: catalog.map(item => item.id),
    });
    render(<>
      <Topbar />
      <Modal isOpen onClose={() => undefined} title="Real help modal" allowClose={false} returnFocusOnClose={false}>
        <input aria-label="modal input" />
      </Modal>
    </>);
    await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
    const focused = screen.getByRole('textbox', { name: 'modal input' });
    focused.focus();
    navigate.mockClear();
    focused.dispatchEvent(new KeyboardEvent('keydown', { key: 'F1', code: 'F1', altKey: true, bubbles: true, cancelable: true }));
    await waitFor(() => expect(navigate).toHaveBeenCalledExactlyOnceWith('/help'));
    expect(document.activeElement).toBe(focused);
    expect(screen.getByRole('dialog', { name: 'Real help modal' })).toBeInTheDocument();
    expect(beginUICommand).not.toHaveBeenCalled();
  });

  it('fecha ao trocar a sessão sem roubar foco pela restauração atrasada', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<Topbar />);
    const outside = document.createElement('button');
    document.body.append(outside);
    try {
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      await screen.findByRole('listbox');
      await waitFor(() => expect(screen.getByRole('combobox')).toHaveFocus());
      state.auth.user = { userId: 'user-a', sessionId: 'session-b' };
      view.rerender(<Topbar />);
      outside.focus();
      await act(async () => { await new Promise(resolve => setTimeout(resolve, 30)); });
      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
      expect(outside).toHaveFocus();
      expect(navigate).not.toHaveBeenCalled();
    } finally {
      view.unmount();
      outside.remove();
    }
  });

  it('não rouba o foco do novo controle ao dispensar a paleta após trocar rota e identidade', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const view = render(<Topbar />);
    try {
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      await screen.findByRole('listbox');
      await waitFor(() => expect(screen.getByRole('combobox')).toHaveFocus());

      state.auth.user = { userId: 'user-b', sessionId: 'session-b' };
      locationState.pathname = '/settings';
      view.rerender(<><Topbar /><button type="button" data-testid="new-route-control">Novo controle</button></>);
      const newControl = screen.getByTestId('new-route-control');
      newControl.focus();

      await act(async () => { await new Promise(resolve => setTimeout(resolve, 30)); });
      expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
      expect(newControl).toHaveFocus();
      expect(navigate).not.toHaveBeenCalled();
    } finally {
      view.unmount();
    }
  });

  it('permite descobrir indisponíveis sem executá-los e executa só a opção seguinte', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    listCommandCatalog.mockResolvedValueOnce(catalog.slice(0, 3).map((item, index) => ({
      ...item, available: index !== 1,
    })));
    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    await screen.findByRole('listbox');
    const search = screen.getByRole('combobox');
    await waitFor(() => expect(search).toHaveFocus());
    const options = screen.getAllByRole('option');
    await user.keyboard('{ArrowDown}');
    expect(search).toHaveAttribute('aria-activedescendant', options[1].id);
    expect(options[1]).toHaveAttribute('aria-disabled', 'true');
    await user.keyboard('{Enter}');
    expect(navigate).not.toHaveBeenCalled();
    expect(search).toHaveFocus();
    await user.keyboard('{ArrowDown}{Enter}');
    await waitFor(() => expect(navigate).toHaveBeenCalledExactlyOnceWith('/memories'));
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(beginUICommand).not.toHaveBeenCalled();
  });

  it.each(['input', 'textarea'] as const)('Ctrl+K abre a paleta de %s e percorre cada comando sem saltos', async (tag) => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const source = document.createElement(tag);
    source.value = 'Rascunho preservado';
    document.body.append(source);
    const view = render(<Topbar />);
    try {
      source.focus();
      await user.keyboard('{Control>}k{/Control}');
      const list = await screen.findByRole('listbox', { name: /commandPalette.shortTitle/ });
      const search = screen.getByRole('combobox', { name: /commandPalette.shortTitle/ });
      await waitFor(() => expect(search).toHaveFocus());
      const commands = Array.from(list.querySelectorAll<HTMLElement>('[role="option"]'));
      expect(commands).toHaveLength(catalog.length);
      expect(commands.every(command => command.getAttribute('aria-disabled') !== 'true')).toBe(true);
      expect(search).toHaveAttribute('aria-activedescendant', commands[0].id);
      for (const command of commands.slice(1)) {
        await user.keyboard('{ArrowDown}');
        expect(search).toHaveFocus();
        expect(search).toHaveAttribute('aria-activedescendant', command.id);
      }
      for (const command of commands.slice(0, -1).reverse()) {
        await user.keyboard('{ArrowUp}');
        expect(search).toHaveFocus();
        expect(search).toHaveAttribute('aria-activedescendant', command.id);
      }
      await user.keyboard('{ArrowUp}');
      expect(search).toHaveFocus();
      expect(source.value).toBe('Rascunho preservado');
      expect(navigate).not.toHaveBeenCalled();
      expect(beginUICommand).not.toHaveBeenCalled();
    } finally {
      view.unmount();
      source.remove();
    }
  });

  it.each([false, true])('abre o menu principal real com Alt+M (paleta indisponível aberta: %s)', async (paletteOpen) => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    if (paletteOpen) listCommandCatalog.mockResolvedValueOnce(catalog.map((item) => ({ ...item, available: false })));
    state.loadMap.mockResolvedValue({
      generation: 'g-alt-m',
      ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      localPaletteCommands: catalog.filter(item => item.id !== 'workspace.list').map(item => item.id),
      bindings: [{ shortcut: { version: 1, code: 'KeyM', modifiers: ['Alt'] }, commandId: 'navigation.menu.open', handler: 'local_ui' }],
    });
    render(<Topbar />);

    if (paletteOpen) {
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      await screen.findByRole('listbox', { name: /commandPalette.shortTitle/ });
      const search = screen.getByRole('combobox', { name: /commandPalette.shortTitle/ });
      await waitFor(() => expect(search).toHaveFocus());
    }

    if (!paletteOpen) screen.getByRole('button', { name: 'commandPalette.title' }).focus();

    await user.keyboard('{Alt>}m{/Alt}');

    const menu = await screen.findByRole('menu', { name: /^menu\.navLabelPlain/ });
    expect(menu).toBeVisible();
    expect(screen.getByRole('menuitem', { name: 'menu.history' })).toHaveFocus();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(takeUICommand).not.toHaveBeenCalled();
  });

  it('seleciona workspace.chat.open no Combobox real e apresenta o conversation_id do snapshot readonly', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const chatCommand = {
      id: 'workspace.chat.open', name: 'Abrir chat', available: true,
      description: 'Abrir chat contextual', category: 'navigation', aliases: ['chat'],
      risk: 'none', availabilityStatus: 'available', availabilityReason: '', readinessReason: '',
    };
    listCommandCatalog.mockResolvedValueOnce([...catalog, chatCommand]);
    state.loadMap.mockResolvedValue({
      generation: 'g-chat-picker', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [{ shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] }, commandId: 'navigation.palette.open', handler: 'local_ui' }],
      localPaletteCommands: [...catalog.map(item => item.id), 'workspace.chat.open'],
    });
    beginUICommand.mockResolvedValue({ ticket: 'chat-picker-ticket', invocationId: 'chat-picker-inv', commandId: 'workspace.chat.open' });
    takeUICommand.mockResolvedValue({ ticket: 'chat-picker-ticket', invocationId: 'chat-picker-inv', commandId: 'workspace.chat.open', handoffId: 'chat-picker-handoff' });
    getUICommandResult.mockResolvedValue({ invocationId: 'chat-picker-inv', status: 'succeeded' });
    state.commitBackendCommand.mockResolvedValue(undefined);
    state.chat.prepare.mockResolvedValue(state.chat.lease);
    state.chat.getActiveWorkspace.mockResolvedValue(activeWorkspaceSnapshot('conversation-from-picker'));

    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
    await user.type(search, 'Abrir chat');
    await user.keyboard('{ArrowDown}{Enter}');

    await waitFor(() => expect(state.commitBackendCommand)
      .toHaveBeenCalledExactlyOnceWith('chat-picker-ticket', 'chat-picker-handoff'));
    await waitFor(() => expect(state.chat.lease.present)
      .toHaveBeenCalledExactlyOnceWith('conversation-from-picker'));
    expect(beginUICommand).toHaveBeenCalledExactlyOnceWith('workspace.chat.open');
    expect(state.chat.getActiveWorkspace).toHaveBeenCalledOnce();
  });

  it('dispatcher do botão cancela chat preparado quando Escape chega antes do prepare', async () => {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    let resolvePrepare!: (lease: typeof state.chat.lease) => void;
    state.chat.prepare.mockReturnValue(new Promise(resolve => { resolvePrepare = resolve; }));
    render(<Topbar />);
    await waitFor(() => expect(state.chat.register).toHaveBeenCalledOnce());
    const dispatchChat = state.chat.register.mock.calls[0]?.[0] as ((tabID: string) => Promise<void>) | undefined;
    expect(dispatchChat).toBeDefined();

    let dispatchDone = false;
    const dispatchPromise = (dispatchChat ? dispatchChat('tab-a') : Promise.resolve())
      .finally(() => { dispatchDone = true; });
    await act(async () => { await Promise.resolve(); });
    fireEvent.keyDown(window, { key: 'Escape', code: 'Escape' });
    resolvePrepare(state.chat.lease);
    await act(async () => { await dispatchPromise; });

    expect(dispatchDone).toBe(true);
    expect(beginUICommand).not.toHaveBeenCalled();
    expect(state.commitBackendCommand).not.toHaveBeenCalled();
    expect(state.chat.lease.present).not.toHaveBeenCalled();
  });

  it('publica os 15 comandos da fixture, filtra e executa navegação sem handoff', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    listCommandCatalog.mockResolvedValueOnce(catalog);

    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));

    const menu = await screen.findByRole('listbox', { name: /commandPalette.shortTitle/ });
    await waitFor(() => expect(menu.querySelectorAll('[role="option"]')).toHaveLength(15));
    expect(screen.getAllByRole('option')).toHaveLength(15);
    expect(screen.getByText('Configurações')).toBeInTheDocument();
    expect(state.announce).toHaveBeenCalledWith('commandPalette.results');
    state.announce.mockClear();

    const search = screen.getByRole('combobox', { name: /commandPalette.shortTitle/ });
    await user.type(search, 'Configurações');
    await user.keyboard('{ArrowDown}{Enter}');

    await waitFor(() => expect(navigate).toHaveBeenCalledWith('/settings'));
    expect(beginUICommand).not.toHaveBeenCalled();
    expect(takeUICommand).not.toHaveBeenCalled();
    expect(completeUICommand).not.toHaveBeenCalled();
    expect(getUICommandResult).not.toHaveBeenCalled();
    expect(listCommandCatalog).toHaveBeenCalledWith({ locale: 'pt-BR', source: 'palette' });
  });

  it('converge navigation.history.open no controller real, na paleta e no StreamDeck', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const history = catalog.find(item => item.id === 'navigation.history.open')!;
    state.loadMap.mockResolvedValue({
      generation: 'g-navigation-convergence',
      ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [{ shortcut: { version: 1, code: 'F1', modifiers: [] }, commandId: history.id, handler: 'local_ui' }],
      localPaletteCommands: [history.id],
    });
    listCommandCatalog.mockResolvedValueOnce([history]);
    const input = document.createElement('input');
    document.body.append(input);
    const view = render(<Topbar />);
    try {
      await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
      input.focus();

      fireEvent.keyDown(input, { key: 'F1', code: 'F1', bubbles: true, cancelable: true });
      fireEvent.keyUp(input, { key: 'F1', code: 'F1', bubbles: true });
      await waitFor(() => expect(navigate).toHaveBeenCalledExactlyOnceWith('/history'));

      // A static navigation command is not repeatable, and IME must not leak
      // the physical key into the same local_ui execution path.
      fireEvent.keyDown(input, { key: 'F1', code: 'F1', repeat: true, bubbles: true, cancelable: true });
      fireEvent.keyUp(input, { key: 'F1', code: 'F1', repeat: true, bubbles: true });
      fireEvent.keyDown(input, { key: 'F1', code: 'F1', isComposing: true, bubbles: true, cancelable: true });
      fireEvent.keyUp(input, { key: 'F1', code: 'F1', isComposing: true, bubbles: true });
      expect(navigate).toHaveBeenCalledTimes(1);

      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
      await user.type(search, 'Histórico');
      await user.keyboard('{ArrowDown}{Enter}');
      await waitFor(() => expect(navigate).toHaveBeenCalledTimes(2));

      act(() => deckEvents.get('command:deck-local-ui')?.({
        commandId: history.id,
        generation: 'g-navigation-convergence',
        userId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      }));
      await waitFor(() => expect(navigate).toHaveBeenCalledTimes(3));
      expect(navigate.mock.calls.map(([path]) => path)).toEqual(['/history', '/history', '/history']);
      expect(beginUICommand).not.toHaveBeenCalled();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    } finally {
      view.unmount();
      input.remove();
    }
  });

  it.each(navigationPaletteCases)('leva %s pela paleta real até %s sem execução durável', async (commandID, label, route) => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({
      generation: `g-palette-${commandID}`,
      ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [], localPaletteCommands: [commandID],
    });
    listCommandCatalog.mockResolvedValueOnce([catalog.find(item => item.id === commandID)!]);
    const view = render(<Topbar />);
    try {
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
      await user.type(search, label);
      await user.keyboard('{ArrowDown}{Enter}');

      await waitFor(() => expect(navigate).toHaveBeenCalledExactlyOnceWith(route));
      expect(beginUICommand).not.toHaveBeenCalled();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
      expect(state.dispatchLocalCommandKey).not.toHaveBeenCalled();
      expect(takeUICommand).not.toHaveBeenCalled();
      expect(completeUICommand).not.toHaveBeenCalled();
    } finally {
      view.unmount();
    }
  });

  it('não executa navigation.about.open pela paleta quando o mapa capturado fica stale', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const about = catalog.find(item => item.id === 'navigation.about.open')!;
    state.loadMap.mockResolvedValue({
      generation: 'g-about-old', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [], localPaletteCommands: [about.id],
    });
    listCommandCatalog.mockResolvedValueOnce([about]);
    const view = render(<Topbar />);
    try {
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
      await user.type(search, about.name);
      const loadsBeforeRefresh = state.loadMap.mock.calls.length;
      state.loadMap.mockResolvedValue({
        generation: 'g-about-new', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
        bindings: [], localPaletteCommands: [],
      });
      await act(async () => { deckEvents.get('command:keyboard-map-changed')?.(); });
      await waitFor(() => expect(state.loadMap).toHaveBeenCalledTimes(loadsBeforeRefresh + 1));
      await user.keyboard('{ArrowDown}{Enter}');

      expect(navigate).not.toHaveBeenCalled();
      expect(beginUICommand).not.toHaveBeenCalled();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    } finally {
      view.unmount();
    }
  });

  it('bloqueia navigation.about.open pela paleta enquanto um modal ocupa o topo', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const about = catalog.find(item => item.id === 'navigation.about.open')!;
    state.loadMap.mockResolvedValue({
      generation: 'g-about-modal', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [], localPaletteCommands: [about.id],
    });
    listCommandCatalog.mockResolvedValueOnce([about]);
    const view = render(<Topbar />);
    // O registro reconcilia a stack com os overlays reais presentes no DOM.
    const overlay = document.createElement('div');
    overlay.className = 'modal-overlay';
    overlay.dataset.modalId = 'navigation-about-modal';
    try {
      await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
      const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
      await user.type(search, about.name);
      act(() => {
        document.body.appendChild(overlay);
        registerOpenModal('navigation-about-modal');
      });
      expect(getModalRegistrySnapshot().topID).toBe('navigation-about-modal');
      await user.keyboard('{ArrowDown}{Enter}');

      expect(getModalRegistrySnapshot().topID).toBe('navigation-about-modal');
      expect(navigate).not.toHaveBeenCalled();
      expect(beginUICommand).not.toHaveBeenCalled();
      expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    } finally {
      unregisterOpenModal('navigation-about-modal');
      overlay.remove();
      view.unmount();
    }
  });

  it('usa o Ctrl+K explícito do mapa e mantém o botão como abertura independente', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({
      generation: 'g-remapped', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [{
        shortcut: { version: 1, code: 'KeyK', modifiers: ['Control'] },
        commandId: 'navigation.menu.open', handler: 'local_ui',
      }],
      localPaletteCommands: catalog.map(item => item.id),
    });
    render(<Topbar />);
    const button = screen.getByRole('button', { name: 'commandPalette.title' });
    button.focus();
    await user.keyboard('{Control>}k{/Control}');
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(screen.getByRole('menu', { name: /^menu\.navLabelPlain/ })).toBeVisible();
    expect(beginUICommand).not.toHaveBeenCalled();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    await user.keyboard('{Escape}');
    await user.click(button);
    await screen.findByRole('listbox', { name: /commandPalette.shortTitle/ });
  });

  it('suprime Ctrl+K sem binding, mas o botão ainda abre a paleta', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.loadMap.mockResolvedValue({
      generation: 'g-suppressed', ownerId: 'user-a', sessionId: 'session-a', workspaceId: 'workspace-a',
      bindings: [], localPaletteCommands: catalog.map(item => item.id),
    });
    render(<Topbar />);
    const button = screen.getByRole('button', { name: 'commandPalette.title' });
    button.focus();
    await user.keyboard('{Control>}k{/Control}');
    expect(screen.queryByRole('listbox')).not.toBeInTheDocument();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(beginUICommand).not.toHaveBeenCalled();
    await user.click(button);
    await screen.findByRole('listbox', { name: /commandPalette.shortTitle/ });
  });

  it.each([
    ['navigation.data.export.open', 'Exportar dados', '/settings/data?action=export'],
    ['navigation.data.import.open', 'Importar dados', '/settings/data?action=import'],
  ] as const)('executa %s pela paleta exatamente uma vez, sem handoff', async (commandID, label, route) => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
    await user.type(search, label);
    await user.keyboard('{ArrowDown}{Enter}');
    await waitFor(() => expect(navigate).toHaveBeenCalledExactlyOnceWith(route));
    expect(beginUICommand).not.toHaveBeenCalled();
    expect(takeUICommand).not.toHaveBeenCalled();
    expect(completeUICommand).not.toHaveBeenCalled();
    expect(state.commitBackendCommand).not.toHaveBeenCalled();
    expect(listCommandCatalog).toHaveBeenCalledWith({ locale: 'pt-BR', source: 'palette' });
    expect(catalog.some(item => item.id === commandID)).toBe(true);
  });

  it('mantém navigation.palette.open utilizável quando selecionado dentro da própria paleta', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
    await user.type(search, 'Abrir paleta');
    await user.keyboard('{ArrowDown}{Enter}');
    await waitFor(() => expect(screen.getByRole('listbox', { name: /commandPalette.shortTitle/ })).toBeVisible());
    expect(beginUICommand).not.toHaveBeenCalled();
    expect(state.beginLocalCommandUIKey).not.toHaveBeenCalled();
    expect(navigate).not.toHaveBeenCalled();
  });

  it('coleta args pelo schema e usa ExecutePaletteCommand sem persistir o payload', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const commandID = 'tools.query.run';
    listCommandCatalog.mockResolvedValueOnce([{
      id: commandID, name: 'Consultar', description: 'Consulta', available: true,
      effect: 'read', risk: 'none', decision: 'none', allowedSources: ['palette'],
    }]);
    state.describeCommandCatalogItem.mockResolvedValue({
      id: commandID, name: 'Consultar', available: true, decision: 'none', allowedSources: ['palette'],
      argumentsSchema: { type: 'object', required: ['query'], properties: { query: { type: 'string' } } },
    });
    state.genericExecute.mockResolvedValue({ status: 'succeeded' });
    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
    await waitFor(() => expect(search).toHaveFocus());
    await user.keyboard('{ArrowDown}{Enter}');
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByRole('textbox', { name: /^query/ }), { target: { value: 'privado' } });
    await user.click(within(dialog).getByRole('button', { name: 'commandPalette.arguments.submit' }));
    await waitFor(() => expect(state.genericExecute).toHaveBeenCalledExactlyOnceWith(commandID, { query: 'privado' }));
    const preferenceKey = 'assistente.command-palette.v1.user-a.workspace-a';
    expect(localStorage.getItem(preferenceKey)).not.toContain('privado');
    expect(state.announce).toHaveBeenCalledWith('commandPalette.executionSucceeded');
  });

  it('executa tool destructive/interactive via backend com Schema Go real e guidance do catálogo', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const compactID = '018f123456787abc8def012345678901';
    const commandID = `tool.execute.t_${compactID}`;
    const runtimeID = '018f1234-5678-7abc-8def-012345678901';
    listCommandCatalog.mockResolvedValueOnce([{
      id: commandID, name: 'Executar ferramenta', available: true,
      effect: 'destructive', risk: 'high', decision: 'interactive', allowedSources: ['palette'],
    }]);
    state.describeCommandCatalogItem.mockResolvedValue({
      id: commandID, name: 'Executar ferramenta', available: true,
      decision: 'interactive', allowedSources: ['palette'],
      argumentsSchema: {
        Type: 'object', Optional: false, Nullable: false, Required: null,
        Properties: {
          arguments_json: { Type: 'string', Optional: false, Nullable: false, MinLength: 0 },
        },
      },
    });
    state.getRuntimeToolCatalog.mockResolvedValueOnce([{
      id: runtimeID, name: 'search', displayName: 'Pesquisa protegida', description: 'Busca no índice autorizado.',
      schema: Array.from(new TextEncoder().encode(JSON.stringify({ type: 'object', properties: { query: { type: 'string' } } }))),
    }]);
    state.genericExecute.mockResolvedValue({ status: 'succeeded', confirmation: { accepted: true } });

    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
    await user.type(search, 'Executar ferramenta');
    await user.keyboard('{ArrowDown}{Enter}');
    await waitFor(() => expect(state.describeCommandCatalogItem).toHaveBeenCalledWith(commandID, { locale: 'pt-BR', source: 'palette' }));
    const dialog = await screen.findByRole('dialog');
    expect(await within(dialog).findByText('commandPalette.arguments.toolSchemaTitle')).toBeVisible();
    expect(within(dialog).getByText('Busca no índice autorizado.')).toBeVisible();
    expect(state.getRuntimeToolCatalog).toHaveBeenCalledWith({ availabilityStatus: 'available', limit: 50, offset: 0 });
    fireEvent.change(within(dialog).getByRole('textbox', { name: /^arguments_json/ }), {
      target: { value: '{"query":"restrita"}' },
    });
    await user.click(within(dialog).getByRole('button', { name: 'commandPalette.arguments.submit' }));
    await waitFor(() => expect(state.genericExecute).toHaveBeenCalledExactlyOnceWith(commandID, { arguments_json: '{"query":"restrita"}' }));
    expect(state.announce).toHaveBeenCalledWith('commandPalette.executionSucceeded');
  });

  it('não deixa o finally da execução A apagar o prompt B após mudança de contexto', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const firstID = 'test.command.first';
    const secondID = 'test.command.second';
    listCommandCatalog.mockResolvedValue([
      { id: firstID, name: 'Comando A', available: true, effect: 'read', risk: 'none', decision: 'none', allowedSources: ['palette'] },
      { id: secondID, name: 'Comando B', available: true, effect: 'read', risk: 'none', decision: 'none', allowedSources: ['palette'] },
    ]);
    state.describeCommandCatalogItem.mockImplementation(async (commandID: string) => ({
      id: commandID, name: commandID === firstID ? 'Comando A' : 'Comando B', available: true,
      decision: 'none', allowedSources: ['palette'],
      argumentsSchema: { type: 'object', required: ['value'], properties: { value: { type: 'string' } } },
    }));
    let resolveA!: (value: unknown) => void;
    state.genericExecute.mockImplementationOnce(() => new Promise((resolve) => { resolveA = resolve; }));

    const view = render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    let search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
    await user.type(search, 'Comando A');
    await user.keyboard('{ArrowDown}{Enter}');
    let dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByRole('textbox', { name: /^value/ }), { target: { value: 'A' } });
    await user.click(within(dialog).getByRole('button', { name: 'commandPalette.arguments.submit' }));
    await waitFor(() => expect(state.genericExecute).toHaveBeenCalledWith(firstID, { value: 'A' }));

    locationState.pathname = '/history';
    view.rerender(<Topbar />);
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    locationState.pathname = '/';
    view.rerender(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
    await user.type(search, 'Comando B');
    await user.keyboard('{ArrowDown}{Enter}');
    dialog = await screen.findByRole('dialog');
    expect(within(dialog).getByRole('textbox', { name: /^value/ })).toBeVisible();

    await act(async () => { resolveA({ status: 'succeeded' }); });
    expect(screen.getByRole('dialog')).toBeVisible();
    expect(within(screen.getByRole('dialog')).getByRole('textbox', { name: /^value/ })).toBeVisible();
  });

  it('permite favoritar o item ativo e abre a configuração do comando vinculado', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    listCommandCatalog.mockResolvedValueOnce([{
      id: 'navigation.settings.open', name: 'Configurações', available: true,
      effect: 'read', risk: 'none', decision: 'none', allowedSources: ['palette'],
    }]);
    render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    const favorite = await screen.findByRole('button', { name: 'commandPalette.addFavorite: Configurações' });
    await user.click(favorite);
    expect(favorite).toHaveAttribute('aria-pressed', 'true');
    expect(localStorage.getItem('assistente.command-palette.v1.user-a.workspace-a')).toContain('navigation.settings.open');
    await user.click(screen.getByRole('button', { name: 'commandPalette.configure: Configurações' }));
    expect(navigate).toHaveBeenCalledWith('/settings/commands?commandId=navigation.settings.open');
  });

  it('descarta formulário de args ao mudar de rota, inclusive seus valores locais', async () => {
    const user = userEvent.setup();
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    const commandID = 'tools.query.run';
    listCommandCatalog.mockResolvedValueOnce([{
      id: commandID, name: 'Consultar', available: true, effect: 'read', risk: 'none', decision: 'none', allowedSources: ['palette'],
    }]);
    state.describeCommandCatalogItem.mockResolvedValue({
      id: commandID, name: 'Consultar', available: true, decision: 'none', allowedSources: ['palette'],
      argumentsSchema: { type: 'object', required: ['query'], properties: { query: { type: 'string' } } },
    });
    const view = render(<Topbar />);
    await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
    const search = await screen.findByRole('combobox', { name: /commandPalette.shortTitle/ });
    await waitFor(() => expect(search).toHaveFocus());
    await user.keyboard('{ArrowDown}{Enter}');
    const dialog = await screen.findByRole('dialog');
    fireEvent.change(within(dialog).getByRole('textbox', { name: /^query/ }), { target: { value: 'segredo temporário' } });
    locationState.pathname = '/history';
    view.rerender(<Topbar />);
    await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    expect(state.genericExecute).not.toHaveBeenCalled();
  });
});

describe('handoff UI externo — provider e handlers reais da Topbar', () => {
  function makeTree(createService: () => ExternalUIConnectionService) {
    return (
      <ExternalUIConnectionProvider createService={createService}>
        <div className="workspace-layout">
          <Topbar />
          <section className="ws-content__panel" data-tab-id={state.workspace.workspace.activeTabId}>
            <button type="button" data-testid="active-panel-focus">Painel ativo</button>
          </section>
        </div>
      </ExternalUIConnectionProvider>
    );
  }

  async function mount(service: ExternalUIConnectionService) {
    vi.spyOn(document, 'hasFocus').mockReturnValue(true);
    state.workspace.workspace = {
      ...state.workspace.workspace,
      activeTabId: 'tab-a',
      tabs: [{ id: 'tab-a', type: 'chat' }, { id: 'tab-b', type: 'editor' }],
    };
    const createService = () => service;
    let view: ReturnType<typeof render> | null = null;
    const rerender = () => view?.rerender(makeTree(createService));
    const originalNavigate = navigate.getMockImplementation();
    navigate.mockImplementation((path: string) => {
      const next = new URL(path, window.location.origin);
      window.history.pushState({}, '', `${next.pathname}${next.search}`);
      locationState.pathname = next.pathname;
      rerender();
    });
    view = render(makeTree(createService));
    await waitFor(() => expect(state.loadMap).toHaveBeenCalled());
    screen.getByRole('button', { name: 'commandPalette.title' }).focus();
    await waitFor(() => expect(document.activeElement).toBe(screen.getByRole('button', { name: 'commandPalette.title' })));
    await waitFor(() => expect(service.publishContext).toHaveBeenCalled());
    return {
      view,
      restoreNavigate: () => navigate.mockImplementation(originalNavigate ?? (() => undefined)),
    };
  }

  it('faz Take → navigation.settings.open real → Complete → publicação do novo contexto', async () => {
    window.history.pushState({}, '', '/');
    locationState.pathname = '/';
    const fixture = createExternalUIServiceFixture();
    const { view, restoreNavigate } = await mount(fixture.service);
    try {
      await act(async () => { fixture.emitReady('navigation.settings.open'); });
      await waitFor(() => expect(fixture.service.complete).toHaveBeenCalledTimes(1));
      await waitFor(() => expect(fixture.service.publishContext).toHaveBeenCalledTimes(2));
      expect(navigate).toHaveBeenCalledWith('/settings');
      expect(fixture.service.take).toHaveBeenCalledTimes(1);
      expect(fixture.service.complete).toHaveBeenCalledWith(
        expect.objectContaining({ commandId: 'navigation.settings.open' }),
        expect.objectContaining({ commandId: 'navigation.settings.open' }),
        'succeeded',
      );
      const takeIndex = fixture.order.indexOf('take');
      const completeIndex = fixture.order.indexOf('complete:succeeded');
      const postNavigationPublishIndex = fixture.order.findIndex((entry, index) => index > completeIndex && entry.startsWith('publish:'));
      expect(takeIndex).toBeGreaterThanOrEqual(0);
      expect(completeIndex).toBeGreaterThan(takeIndex);
      expect(postNavigationPublishIndex).toBeGreaterThan(completeIndex);
      expect(fixture.getSnapshot().target?.surface.snapshotVersion).toContain('/settings');
    } finally {
      view.unmount();
      restoreNavigate();
      window.history.pushState({}, '', '/');
      locationState.pathname = '/';
    }
  });

  it('faz Take → workspace.tab.next pelo handler real → Complete e liga a aba nova', async () => {
    window.history.pushState({}, '', '/');
    locationState.pathname = '/';
    const fixture = createExternalUIServiceFixture();
    state.workspace.setActiveTab.mockImplementation((tabId: string) => {
      state.workspace.workspace.activeTabId = tabId;
      state.workspaceListeners.forEach(listener => listener());
    });
    const { view, restoreNavigate } = await mount(fixture.service);
    try {
      await act(async () => { fixture.emitReady('workspace.tab.next'); });
      await waitFor(() => expect(fixture.service.complete).toHaveBeenCalledTimes(1));
      await waitFor(() => expect(fixture.service.publishContext).toHaveBeenCalledTimes(2));
      expect(state.workspace.setActiveTab).toHaveBeenCalledExactlyOnceWith('tab-b');
      expect(fixture.service.complete).toHaveBeenCalledWith(
        expect.objectContaining({ commandId: 'workspace.tab.next' }),
        expect.objectContaining({ commandId: 'workspace.tab.next' }),
        'succeeded',
      );
      expect(fixture.getSnapshot().target?.tabId).toBe('tab-b');
      expect(fixture.order.indexOf('complete:succeeded')).toBeGreaterThan(fixture.order.indexOf('take'));
    } finally {
      view.unmount();
      restoreNavigate();
      window.history.pushState({}, '', '/');
      locationState.pathname = '/';
    }
  });
});
