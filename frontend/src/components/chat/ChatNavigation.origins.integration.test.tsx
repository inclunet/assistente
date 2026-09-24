import { act, cleanup, fireEvent, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { MemoryRouter } from 'react-router-dom';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { Topbar } from '../layout/Topbar';
import { ChatSessionView } from './ChatSessionView';
import { ChatSessionProvider } from './ChatSessionContext';
import { WorkspacePanelProvider } from '../workspace/WorkspacePanelContext';
import { useAuthStore } from '../../store/authStore';
import { useChatStore } from '../../store/chatStore';
import { useWorkspaceStore } from '../../store/workspaceStore';
import { useWorkspaceChatModalStore } from '../../store/workspaceChatModalStore';
import { createChatSurfaceIdentity, createEmptyChatSession } from '../../services/chatSessionRegistry';
import { chat } from '../../../wailsjs/go/models';

const state = vi.hoisted(() => ({
  mapReady: false,
  events: new Map<string, Set<(payload: unknown) => void>>(),
  beginUI: vi.fn(async () => null),
  takeUI: vi.fn(async () => null),
  completeUI: vi.fn(async () => undefined),
  t: (key: string) => key,
  noop: () => undefined,
}));

const commandIDs = ['chat.message.thread.expand', 'chat.message.thread.collapse'] as const;
const conversationID = '01926b90-7a5a-7c4e-8d3f-000000000101';
const firstID = '01926b90-7a5a-7c4e-8d3f-000000000111';
const secondID = '01926b90-7a5a-7c4e-8d3f-000000000112';
const firstChildID = '01926b90-7a5a-7c4e-8d3f-000000000121';
const secondChildID = '01926b90-7a5a-7c4e-8d3f-000000000122';
const surface = createChatSurfaceIdentity({ conversationId: conversationID, tabId: 'chat-tab', surfaceType: 'page' });

vi.mock('../../lib/commandGlobalOwnershipWails', () => ({
  acquireGlobalCommandOwnership: () => ({
    isReady: () => true,
    owns: () => false,
    dispose: () => {},
    ready: Promise.resolve(),
  }),
}));
vi.mock('../../services/commandCatalog', () => ({
  listCommandCatalog: vi.fn(async () => commandIDs.map(id => ({
    id,
    name: id,
    available: true,
    description: '',
    category: 'chat',
    aliases: [],
    icon: '',
    effect: 'ui',
    risk: 'none',
    decision: 'allow',
    availabilityStatus: 'available',
    availabilityReason: '',
    allowedSources: ['keyboard.local', 'palette', 'streamdeck.key', 'ui.action'],
    scopes: ['chat'],
    presentationVersion: '1',
  }))),
}));
vi.mock('../../lib/commandLocalKeyboardWails', () => ({
  createCommandLocalKeyboardWailsPort: () => ({
    loadMap: async () => {
      state.mapReady = true;
      return {
        generation: 'chat-navigation-generation',
        ownerId: 'owner',
        sessionId: 'session',
        workspaceId: 'workspace',
        localPaletteCommands: [...commandIDs],
        bindings: [...commandIDs.map((commandId, index) => ({
          shortcut: { version: 1 as const, code: `Digit${index + 1}`, modifiers: ['Control'] as const },
          commandId,
          handler: 'local_ui' as const,
        })), {
          shortcut: { version: 1 as const, code: 'KeyK', modifiers: ['Control'] as const },
          commandId: 'navigation.palette.open' as const,
          handler: 'local_ui' as const,
        }],
      };
    },
    dispatchLocalCommandKey: vi.fn(async () => null),
    beginLocalCommandUIKey: vi.fn(async () => null),
    resetLocalCommandKeyboard: vi.fn(async () => undefined),
  }),
}));
vi.mock('../../lib/commandUIExecutionWails', () => ({
  createCommandUIExecutionWailsPort: () => ({
    beginUICommand: state.beginUI,
    takeUICommand: state.takeUI,
    completeUICommand: state.completeUI,
    cancelUICommand: vi.fn(async () => undefined),
    getUICommandResult: vi.fn(async () => null),
    commitBackendCommand: vi.fn(async () => undefined),
  }),
}));
vi.mock('../../lib/commandWorkspaceTabWails', () => ({
  createCommandWorkspaceTabWailsPort: () => ({
    beginUICommand: state.beginUI,
    takeUICommand: state.takeUI,
    completeUICommand: state.completeUI,
    cancelUICommand: vi.fn(async () => undefined),
    getUICommandResult: vi.fn(async () => null),
    commitBackendCommand: vi.fn(async () => undefined),
  }),
}));
vi.mock('../../lib/commandBackendExecutionWails', () => ({ createCommandBackendExecutionWailsPort: () => ({ executeCommand: vi.fn() }) }));
vi.mock('@wailsjs/go/wailsapi/Workspace', () => ({ GetActiveWorkspace: vi.fn(async () => undefined) }));
vi.mock('@wailsjs/go/wailsapi/Profiles', () => ({ GetActiveProfile: vi.fn(async () => ({})), GetActiveProfileSlug: vi.fn(async () => 'default') }));
vi.mock('@wailsjs/go/wailsapi/Skills', () => ({ GetUserInvocableSkillsForProfile: vi.fn(async () => []) }));
vi.mock('@wailsjs/go/wailsapi/Chat', () => ({ SendMessage: vi.fn(async () => conversationID), RetryMessage: vi.fn(async () => conversationID) }));
vi.mock('@wailsjs/runtime/runtime', () => ({
  EventsOn: (name: string, callback: (payload: unknown) => void) => {
    const callbacks = state.events.get(name) ?? new Set<(payload: unknown) => void>();
    callbacks.add(callback);
    state.events.set(name, callbacks);
    return () => {
      callbacks.delete(callback);
      if (callbacks.size === 0) state.events.delete(name);
    };
  },
}));
vi.mock('react-i18next', () => ({
  initReactI18next: { type: '3rdParty', init() {} },
  useTranslation: () => ({ t: state.t, i18n: { language: 'pt-BR' } }),
}));
vi.mock('../../hooks/useAnnouncer', () => ({ announce: vi.fn(), useAnnouncer: () => ({ announce: vi.fn(), announceRequest: vi.fn() }) }));
vi.mock('../../services/audioFeedback', () => ({
  playBumpSound: vi.fn(), playMessageSound: vi.fn(), playSendSound: vi.fn(), playReceiveSound: vi.fn(), playChatErrorSound: vi.fn(),
}));
vi.mock('../../services/messageAudio', () => ({ messageAudioService: {
  isCurrentlyPlaying: () => false, stopCurrentAudio: vi.fn(), speakMessage: vi.fn(async () => true),
} }));
vi.mock('../../services/tts', () => ({ ttsService: {
  isEnabled: () => false, hasVoiceConfig: () => true, getVolume: () => 1, isSpeaking: () => false,
  isEnabledForUser: () => false, shouldUseAriaLiveForUser: () => false, shouldUseAriaLiveForAgent: () => false,
  isAutoReadEnabled: () => false, stop: vi.fn(), on: vi.fn(), off: vi.fn(), getVoiceContext: () => undefined,
} }));
vi.mock('./ChatToolbar', () => ({ ChatToolbar: () => null }));
vi.mock('./useAgentSessionCommands', () => ({ useAgentSessionCommands: () => [] }));
vi.mock('./VoiceButton', () => ({ VoiceButton: () => null }));
vi.mock('../ui/KeyboardShortcutsHelp', () => ({ KeyboardShortcutsHelp: () => null }));
vi.mock('../menu', () => ({ Menu: () => null, ContextMenu: () => null }));
vi.mock('../layout/MenuButton', async () => {
  const { forwardRef } = await import('react');
  return { MenuButton: forwardRef<HTMLButtonElement>(function MenuButton() { return null; }) };
});
vi.mock('../layout/ConnectionStatusIndicator', () => ({ ConnectionStatusIndicator: () => null }));
vi.mock('../../hooks/useAnchoredContextMenu', () => ({
  useAnchoredContextMenu: () => ({ menu: { visible: false, items: [], x: 0, y: 0, ariaLabel: '' }, openForTrigger: state.noop, openAtPoint: state.noop, closeMenu: state.noop, onSelectItem: state.noop }),
}));
vi.mock('../../hooks/useToolbarKeyboardNav', () => ({ useToolbarKeyboardNav: state.noop }));
vi.mock('../../lib/commandContextReact', () => ({ useCommandContextScope: () => null }));

function message(id: string, content: string, childID: string) {
  return new chat.MessageNode({
    message: new chat.EnrichedMessage({ id, conversationId: conversationID, role: 'assistant', content, reasoning: `reasoning-${id}` }),
    children: [new chat.MessageNode({
      message: new chat.EnrichedMessage({ id: childID, conversationId: conversationID, parentId: id, role: 'user', content: `child-${id}`, internal: true }),
      children: [], childCount: 0, hasChildren: false, level: 1,
    })],
    hasChildren: true,
    childCount: 1,
    level: 0,
  });
}

function seed() {
  const conversation = {
    id: conversationID,
    title: 'Chat',
    threadedMessages: [message(firstID, 'first', firstChildID), message(secondID, 'second', secondChildID)],
  };
  useChatStore.setState({
    timelinesByConversationId: { [conversationID]: conversation },
    sessionsByConversationId: { [conversationID]: { ...createEmptyChatSession(conversationID), conversation } },
    surfaceSessionsByKey: {},
    loadingConversationIds: new Set(),
  });
}

function root(id = firstID) {
  return document.querySelector<HTMLElement>(`.message-node[data-message-id="${id}"]`)!;
}

function deck(commandId: string) {
  act(() => {
    state.events.get('command:deck-local-ui')?.forEach(callback => callback({
      commandId,
      generation: 'chat-navigation-generation',
      userId: 'owner',
      sessionId: 'session',
      workspaceId: 'workspace',
    }));
  });
}

async function palette(commandId: string) {
  const user = userEvent.setup();
  await user.click(screen.getByRole('button', { name: 'commandPalette.title' }));
  await user.type(await screen.findByRole('combobox', { name: /commandPalette/ }), commandId);
  const shortcutNumber = commandIDs.indexOf(commandId as typeof commandIDs[number]) + 1;
  await user.click(await screen.findByRole('option', { name: `${commandId}. Ctrl+${shortcutNumber}` }));
}

async function keyboard(commandId: string) {
  const index = commandIDs.indexOf(commandId as typeof commandIDs[number]);
  const target = root();
  act(() => target.focus());
  fireEvent.keyDown(target, { key: String(index + 1), code: `Digit${index + 1}`, ctrlKey: true });
  fireEvent.keyUp(target, { key: String(index + 1), code: `Digit${index + 1}`, ctrlKey: true });
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 0)); });
}

async function commandFrom(origin: 'keyboard.local' | 'palette' | 'streamdeck.key', commandId: string) {
  if (origin === 'keyboard.local') return keyboard(commandId);
  if (origin === 'palette') return palette(commandId);
  deck(commandId);
}

function mount() {
  return render(<MemoryRouter initialEntries={['/']}>
    <Topbar />
    <WorkspacePanelProvider value={{ tab: { id: 'chat-tab', type: 'chat', title: 'Chat', position: 0, conversationId: conversationID }, isActive: true }}>
      <ChatSessionProvider surface={surface}>
        <ChatSessionView surface={surface} onSend={vi.fn()} />
      </ChatSessionProvider>
    </WorkspacePanelProvider>
  </MemoryRouter>);
}

async function expandVisibleThread(id: string) {
  fireEvent.click(root(id).querySelector<HTMLButtonElement>('.thread-indicator')!);
  await waitFor(() => expect(root(id)).toHaveAttribute('aria-expanded', 'true'));
}

async function prepareExpandedThreads() {
  await expandVisibleThread(firstID);
  await expandVisibleThread(secondID);
}

async function waitForMountedDispatcher() {
  await waitFor(() => expect(state.mapReady).toBe(true));
  await waitFor(() => expect(state.events.has('command:deck-local-ui')).toBe(true));
}

beforeEach(() => {
  localStorage.removeItem('assistente.command-palette.v1.owner.workspace');
  vi.spyOn(document, 'hasFocus').mockReturnValue(true);
  state.mapReady = false;
  state.events.clear();
  state.beginUI.mockReset().mockResolvedValue(null);
  state.takeUI.mockReset().mockResolvedValue(null);
  state.completeUI.mockReset().mockResolvedValue(undefined);
  useAuthStore.setState({ isAuthenticated: true, user: { userId: 'owner', sessionId: 'session', role: 'admin' } });
  useWorkspaceStore.setState({ workspace: { id: 'workspace', activeTabId: 'chat-tab', tabs: [{ id: 'chat-tab', type: 'chat', conversationId: conversationID }] } as NonNullable<ReturnType<typeof useWorkspaceStore.getState>['workspace']> });
  useWorkspaceChatModalStore.setState({ isOpen: false, boundConversationId: null, boundTabId: null });
  seed();
  vi.spyOn(HTMLElement.prototype, 'scrollIntoView').mockImplementation(() => {});
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.clearAllMocks();
});

describe('Chat navigation origins — handlers reais até o DOM da thread', () => {
  it.each(['keyboard.local', 'palette', 'streamdeck.key'] as const)('%s expande/recolhe o nó capturado e alcança o filho real', async origin => {
    mount();
    await waitForMountedDispatcher();
    act(() => root().focus());
    await prepareExpandedThreads();
    act(() => root().focus());
    await commandFrom(origin, 'chat.message.thread.collapse');
    await waitFor(() => expect(root()).toHaveAttribute('aria-expanded', 'false'));
    expect(root(firstChildID)).not.toBeInTheDocument();
    expect(root(secondID)).toHaveAttribute('aria-expanded', 'true');
    expect(root(secondChildID)).toBeInTheDocument();

    act(() => root().focus());
    await commandFrom(origin, 'chat.message.thread.expand');
    await waitFor(() => expect(root()).toHaveAttribute('aria-expanded', 'true'));
    expect(root(firstChildID)).toBeInTheDocument();
    expect(root(secondID)).toHaveAttribute('aria-expanded', 'true');
    expect(root(secondChildID)).toBeInTheDocument();
    expect(state.beginUI).not.toHaveBeenCalled();
    expect(state.takeUI).not.toHaveBeenCalled();
    expect(state.completeUI).not.toHaveBeenCalled();
  });

  it('ui.action usa o botão da mensagem, não uma captura global, até o DOM do nó correto', async () => {
    mount();
    await waitForMountedDispatcher();
    const first = root();
    const second = root(secondID);
    act(() => first.focus());
    await prepareExpandedThreads();
    fireEvent.click(first.querySelector<HTMLButtonElement>('.thread-indicator')!);
    await waitFor(() => expect(first).toHaveAttribute('aria-expanded', 'false'));
    expect(root(firstChildID)).not.toBeInTheDocument();
    expect(second).toHaveAttribute('aria-expanded', 'true');
    expect(root(secondChildID)).toBeInTheDocument();
    expect(state.beginUI).not.toHaveBeenCalled();
    expect(state.takeUI).not.toHaveBeenCalled();
    expect(state.completeUI).not.toHaveBeenCalled();
  });
});
