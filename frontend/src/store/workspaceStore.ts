import { logger } from '../utils/logger';
import { create } from 'zustand';
import { useShallow } from 'zustand/shallow';
import {
  GetActiveWorkspace,
  ListWorkspaces,
  CreateWorkspace,
  SwitchWorkspace,
  RenameWorkspace,
  DeleteWorkspace,
  SetWorkspaceProfile,
  AddWorkspaceTab,
  RemoveWorkspaceTab,
  UpdateWorkspaceTab,
  ReorderWorkspaceTabs,
  MoveWorkspaceTabTo,
  ExportWorkspace,
  ImportWorkspace,
} from '@wailsjs/go/wailsapi/Workspace';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { workspace } from '../../wailsjs/go/models';
import i18next from 'i18next';
import { announce } from '../hooks/useAnnouncer';
import { isModalOpen } from '../lib/modalRegistry';
import { waitForWailsBridge } from '../lib/waitForWailsBridge';
import { setActiveWorkspaceTabForWorkspace } from '../lib/workspaceNavigationWails';
import { useAuthStore } from './authStore';
import {
  compareWorkspaceSnapshots,
  parseWorkspaceSnapshot,
  type ParsedWorkspaceSnapshot,
  type WorkspaceSnapshotPayload,
} from '../lib/workspaceSnapshot';

export type TabType = 'chat' | 'editor' | 'terminal' | 'tasklist';

// Registry: handlers called when user renames a tab via F2 (tab → content)
// The id passed to the handler is type-specific: conversationId (chat), tabId (editor), tasklistId (tasklist), sessionId (terminal)
const tabRenameHandlers = new Map<TabType, (id: string, newTitle: string) => void>();

export function registerTabRenameHandler(
  type: TabType,
  handler: (id: string, newTitle: string) => void,
): () => void {
  tabRenameHandlers.set(type, handler);
  return () => { tabRenameHandlers.delete(type); };
}

export interface WorkspaceTab {
  id: string;
  type: TabType;
  conversationId?: string;
  title: string;
  position: number;
  profileOverride?: Record<string, unknown>;
  state?: Record<string, unknown>;
}

export interface WorkspaceData {
  id: string;
  name: string;
  profile?: string;
  tabs: WorkspaceTab[];
  activeTabId: string | null;
}

interface BackendWorkspaceTabPayload {
  id: string;
  type: string;
  conversation_id?: string;
  title?: string;
  position: number;
  profile_override?: unknown;
  state?: unknown;
}

export interface BackendWorkspacePayload extends WorkspaceSnapshotPayload {
  id: string;
  name: string;
  profile?: string;
  tabs?: {
    items?: BackendWorkspaceTabPayload[];
    active?: string;
  };
}

export interface WorkspaceTabUpdatedEvent {
  workspace: BackendWorkspacePayload;
  tabId: string;
  profileSlug: string;
}

function backendTabToFrontend(bt: BackendWorkspaceTabPayload): WorkspaceTab {
  const fallbackTitle = bt.type === 'chat'
    ? i18next.t('chat.newConversation')
    : bt.type === 'editor'
      ? i18next.t('editor.fallback.newDoc')
      : bt.type === 'tasklist'
        ? i18next.t('workspace.newTasklist')
        : '';
  return {
    id: bt.id,
    type: bt.type as TabType,
    conversationId: bt.conversation_id || undefined,
    title: bt.title || fallbackTitle,
    position: bt.position,
    profileOverride: bt.profile_override as Record<string, unknown> | undefined,
    state: bt.state as Record<string, unknown> | undefined,
  };
}

function backendWorkspaceToFrontend(bws: BackendWorkspacePayload): WorkspaceData {
  const tabs = (bws.tabs?.items || []).map(backendTabToFrontend);
  return {
    id: bws.id,
    name: bws.name,
    profile: bws.profile,
    tabs,
    activeTabId: bws.tabs?.active || null,
  };
}

function frontendTabToBackend(tab: WorkspaceTab): workspace.Tab {
  return new workspace.Tab({
    id: tab.id,
    type: tab.type,
    conversation_id: tab.conversationId || '',
    title: tab.title,
    position: tab.position,
    profile_override: tab.profileOverride,
    state: tab.state,
  });
}

interface WorkspaceStore {
  workspace: WorkspaceData | null;
  workspaces: workspace.WorkspaceInfo[];
  isInitialized: boolean;

  // Initialization
  initialize: () => Promise<void>;
  setupEventListeners: () => () => void;

  // Workspace CRUD
  createWorkspace: (name: string) => Promise<string>;
  switchWorkspace: (workspaceId: string) => Promise<void>;
  renameWorkspace: (newName: string) => Promise<void>;
  deleteWorkspace: (workspaceId: string) => Promise<void>;
  setProfile: (profileSlug: string) => Promise<void>;
  refreshWorkspaceList: () => Promise<void>;

  // Tab management
  addTab: (type: TabType, title: string, initialState?: Record<string, unknown>) => Promise<string>;
  removeTab: (tabId: string) => Promise<void>;
  setActiveTab: (tabId: string) => void;
  reconcileActiveSelection: () => Promise<boolean>;
  updateTab: (tabId: string, updates: Record<string, unknown>) => Promise<void>;
  reorderTabs: (orderedIds: string[]) => Promise<void>;
  moveTabToWorkspace: (tabId: string, targetWorkspaceId: string) => Promise<void>;

  // Content ↔ Tab title sync
  handleContentRenamed: (type: TabType, contentId: string, newTitle: string) => void;
  renameTabContent: (tabId: string, newTitle: string) => void;

  // Export/Import
  exportWorkspace: () => Promise<string>;
  importWorkspace: (yamlData: string) => Promise<string>;

  // Getters
  getActiveTab: () => WorkspaceTab | undefined;
  getTabsByType: (type: TabType) => WorkspaceTab[];
}

function generateTabId(): string {
  return `tab-${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 7)}`;
}

let initializingPromise: Promise<void> | null = null;
let initializeRetryTimer: ReturnType<typeof setTimeout> | null = null;
let activationSeqId = 0;
let snapshotAnchor: ParsedWorkspaceSnapshot | null = null;
let bootstrapInProgress = false;
let listenersReady = false;
let snapshotGeneration = 0;
let workspaceNavigationGeneration = 0;
let activationBusy = false;
const activationQueue: Array<(markFailed: () => void) => Promise<void>> = [];
let activationDrainPromise: Promise<boolean> = Promise.resolve(true);
let resolveActivationDrain: ((result: boolean) => void) | null = null;
let activationPumpToken = 0;
let pendingActivation: {
  workspaceId: string;
  tabId: string;
  requestId: number;
  settled: boolean;
} | null = null;

function enqueueActivation(task: (markFailed: () => void) => Promise<void>) {
  if (activationBusy) {
    // Only the latest intent not yet handed to Wails is retained. The
    // in-flight request remains untouched; intermediate key presses are UI
    // intent, not persistence history.
    activationQueue[0] = task;
    return;
  }
  const pumpToken = ++activationPumpToken;
  activationQueue[0] = task;
  activationBusy = true;
  let drainResult = true;
  let resolveDrain: ((result: boolean) => void) | null = null;
  activationDrainPromise = new Promise<boolean>((resolve) => {
    resolveDrain = resolve;
    resolveActivationDrain = resolve;
  });
  const pump = async () => {
    if (pumpToken !== activationPumpToken) return;
    const next = activationQueue.shift();
    if (!next) {
      if (pumpToken !== activationPumpToken) return;
      activationBusy = false;
      resolveDrain?.(drainResult);
      if (resolveActivationDrain === resolveDrain) resolveActivationDrain = null;
      return;
    }
    const markDrainFailed = () => { drainResult = false; };
    try {
      await next(markDrainFailed);
    } catch (error) {
      drainResult = false;
      logger.warn('[workspaceStore] activation queue task failed:', error);
    } finally {
      if (pumpToken === activationPumpToken) void pump();
    }
  };
  void pump();
}
const WAILS_BRIDGE_INIT_TIMEOUT_MS = 10000;
const WAILS_BRIDGE_RETRY_DELAY_MS = 1000;

function isWailsBridgeTimeoutError(error: unknown): boolean {
  return error instanceof Error && error.message.includes('Timed out waiting for Wails bridge');
}

function clearInitializeRetryTimer() {
  if (initializeRetryTimer !== null) {
    clearTimeout(initializeRetryTimer);
    initializeRetryTimer = null;
  }
}

function resetSnapshotOrdering() {
  snapshotGeneration += 1;
  snapshotAnchor = null;
  bootstrapInProgress = false;
  pendingActivation = null;
  workspaceNavigationGeneration += 1;
  activationQueue.length = 0;
  activationPumpToken += 1;
  if (activationBusy) {
    activationBusy = false;
    resolveActivationDrain?.(false);
    resolveActivationDrain = null;
  }
  activationDrainPromise = Promise.resolve(true);
}

function projectSnapshotAnchor(anchor: ParsedWorkspaceSnapshot | null): WorkspaceData | null {
  return anchor
    ? backendWorkspaceToFrontend(anchor.payload as BackendWorkspacePayload)
    : null;
}

/**
 * Faz merge de `patch` no state existente de uma aba.
 * Isso garante que chaves como `filePath` não sejam perdidas quando
 * callers passam apenas um subset do state (ex: { scrollTop: 240 }).
 */
function mergeTabState(
  existing: Record<string, unknown> | undefined,
  patch: Record<string, unknown>,
): Record<string, unknown> {
  return { ...(existing ?? {}), ...patch };
}

function mergeProfileOverride(
  existing: Record<string, unknown> | undefined,
  patch: Record<string, unknown> | null,
): Record<string, unknown> | undefined {
  if (patch === null) return undefined;
  const merged = { ...(existing ?? {}) };
  for (const [key, value] of Object.entries(patch)) {
    if (value === null) {
      delete merged[key];
    } else if (value !== undefined) {
      merged[key] = value;
    }
  }
  return Object.keys(merged).length > 0 ? merged : undefined;
}

export const useWorkspaceStore = create<WorkspaceStore>()((set, get) => {
  const applyWorkspaceSnapshot = (raw: unknown, trustedBootstrap = false): boolean => {
    if (bootstrapInProgress && !trustedBootstrap) return false;
    const incoming = parseWorkspaceSnapshot(raw);
    if (!incoming) return false;
    if (!snapshotAnchor) {
      if (!trustedBootstrap) return false;
      snapshotAnchor = incoming;
    } else if (compareWorkspaceSnapshots(snapshotAnchor, incoming) !== 'newer') {
      return false;
    }
    snapshotAnchor = incoming;
    const nextWorkspace = backendWorkspaceToFrontend(incoming.payload as BackendWorkspacePayload);
    if (get().workspace && get().workspace?.id !== nextWorkspace.id) {
      workspaceNavigationGeneration += 1;
      activationSeqId += 1;
      pendingActivation = null;
    }
    const pending = pendingActivation;
    if (pending && pending.workspaceId === nextWorkspace.id) {
      if (!pending.settled && nextWorkspace.activeTabId !== pending.tabId
        && nextWorkspace.tabs.some(tab => tab.id === pending.tabId)) {
        nextWorkspace.activeTabId = pending.tabId;
      }
      if (pending.settled) pendingActivation = null;
    }
    set({ workspace: nextWorkspace });
    return true;
  };

  const reconcileActiveWorkspace = async (expected?: {
    generation: number;
    navigationGeneration: number;
    workspaceId: string | null;
  }) => {
    const generationAtStart = snapshotGeneration;
    const navigationGenerationAtStart = workspaceNavigationGeneration;
    const workspaceIdAtStart = get().workspace?.id ?? null;
    const epochAtStart = snapshotAnchor?.epoch;
    try {
      const latest = await GetActiveWorkspace();
      if (generationAtStart !== snapshotGeneration
        || navigationGenerationAtStart !== workspaceNavigationGeneration
        || workspaceIdAtStart !== (get().workspace?.id ?? null)
        || epochAtStart !== snapshotAnchor?.epoch
        || (expected && (expected.generation !== snapshotGeneration
          || expected.navigationGeneration !== workspaceNavigationGeneration
          || expected.workspaceId !== (get().workspace?.id ?? null)))) {
        return false;
      }
      const incoming = parseWorkspaceSnapshot(latest);
      if (!incoming || incoming.epoch !== epochAtStart
        || (expected && incoming.payload.id !== expected.workspaceId)) return false;
      return applyWorkspaceSnapshot(latest)
        || (snapshotAnchor !== null && compareWorkspaceSnapshots(snapshotAnchor, incoming) === 'same');
    } catch (error) {
      logger.warn('[Workspace] Error reconciling active snapshot:', error);
      return false;
    }
  };

  return ({
  workspace: null,
  workspaces: [],
  isInitialized: false,

  initialize: async () => {
    if (initializingPromise) return initializingPromise;

    let runPromise: Promise<void>;
    if (!get().workspace && !get().isInitialized) resetSnapshotOrdering();
    const runGeneration = snapshotGeneration;
    const run = async () => {
      try {
        bootstrapInProgress = true;
        clearInitializeRetryTimer();
        await waitForWailsBridge({ timeoutMs: WAILS_BRIDGE_INIT_TIMEOUT_MS });
        const [bws, list] = await Promise.all([
          GetActiveWorkspace(),
          ListWorkspaces(),
        ]);
        if (runGeneration !== snapshotGeneration) return;

        if (bws) {
          const snapshot = parseWorkspaceSnapshot(bws);
          if (!snapshot) throw new Error('Invalid versioned workspace bootstrap snapshot');
          snapshotAnchor = snapshot;
          const ws = backendWorkspaceToFrontend(snapshot.payload as BackendWorkspacePayload);

          if (ws.tabs.length === 0) {
            const tab: WorkspaceTab = {
              id: generateTabId(),
              type: 'chat',
              title: i18next.t('chat.newConversation'),
              position: 0,
            };
            const backendTab = frontendTabToBackend(tab);
            const updatedWs = await AddWorkspaceTab(backendTab);
            if (runGeneration !== snapshotGeneration) return;
            if (updatedWs) {
              applyWorkspaceSnapshot(updatedWs, true);
              set({ workspaces: list || [], isInitialized: true });
              bootstrapInProgress = false;
              if (listenersReady) await reconcileActiveWorkspace();
              return;
            }
          }

          set({
            workspace: ws,
            workspaces: list || [],
            isInitialized: true,
          });
        } else {
          resetSnapshotOrdering();
          set({ isInitialized: true, workspaces: list || [] });
        }
        set({ isInitialized: true });
        bootstrapInProgress = false;
        if (listenersReady && runGeneration === snapshotGeneration) await reconcileActiveWorkspace();
      } catch (error) {
        if (runGeneration !== snapshotGeneration) return;
        if (isWailsBridgeTimeoutError(error)) {
          logger.warn('[Workspace] Wails bridge timeout during initialize; retrying...', error);
          if (initializeRetryTimer === null) {
            initializeRetryTimer = setTimeout(() => {
              initializeRetryTimer = null;
              void get().initialize();
            }, WAILS_BRIDGE_RETRY_DELAY_MS);
          }
          return;
        }
        logger.error('[Workspace] Error initializing:', error);
        bootstrapInProgress = false;
        set({ isInitialized: true });
      } finally {
        if (initializingPromise === runPromise) initializingPromise = null;
      }
    };

    runPromise = run();
    initializingPromise = runPromise;
    return runPromise;
  },

  setupEventListeners: () => {
    const unsubs: Array<() => void> = [];
    listenersReady = true;

    unsubs.push(EventsOn('workspace:switched', (bws: unknown) => {
      applyWorkspaceSnapshot(bws);
    }));

    unsubs.push(EventsOn('workspace:renamed', (bws: unknown) => {
      applyWorkspaceSnapshot(bws);
      get().refreshWorkspaceList();
    }));

    unsubs.push(EventsOn('workspace:created', () => {
      get().refreshWorkspaceList();
    }));

    unsubs.push(EventsOn('workspace:deleted', () => {
      get().refreshWorkspaceList();
    }));

    unsubs.push(EventsOn('workspace:tab_added', (bws: unknown) => {
      applyWorkspaceSnapshot(bws);
    }));

    unsubs.push(EventsOn('workspace:tab_removed', (bws: unknown) => {
      applyWorkspaceSnapshot(bws);
    }));

    // Binding a contextual conversation is not a profile change or navigation.
    unsubs.push(EventsOn('workspace:conversation_bound', (bws: unknown) => {
      applyWorkspaceSnapshot(bws);
    }));

    unsubs.push(EventsOn('workspace:terminal_session_bound', (bws: unknown) => {
      applyWorkspaceSnapshot(bws);
    }));

    unsubs.push(EventsOn('workspace:tab_navigated', (bws: BackendWorkspacePayload) => {
      applyWorkspaceSnapshot(bws);
    }));

    unsubs.push(EventsOn('workspace:tab_updated', (event: WorkspaceTabUpdatedEvent) => {
      if (!applyWorkspaceSnapshot(event.workspace)) return;
      const profileSlug = event.profileSlug.trim();
      announce(profileSlug
        ? `${i18next.t('workspace.profileChanged')}: ${profileSlug}`
        : i18next.t('workspace.profileChanged'));
    }));

    unsubs.push(EventsOn('workspace:editor_mode_changed', (event: unknown) => {
      if (!event || typeof event !== 'object') return;
      const modeEvent = event as { workspace?: unknown; tabId?: unknown; mode?: unknown };
      if (typeof modeEvent.tabId !== 'string' || typeof modeEvent.mode !== 'string' || !['markdown', 'rich', 'view'].includes(modeEvent.mode)) return;
      // Snapshot versionado continua sendo a autoridade; o evento não altera
      // documentos locais nem anuncia falsamente uma mudança de perfil.
      applyWorkspaceSnapshot(modeEvent.workspace);
    }));

    unsubs.push(EventsOn('workspace:editor_file_changed', (event: unknown) => {
      if (!event || typeof event !== 'object') return;
      const fileEvent = event as { workspace?: unknown; tabId?: unknown };
      if (typeof fileEvent.tabId !== 'string' || !fileEvent.workspace) return;
      // Snapshot versionado é a fonte de seleção/aba; o loader hidrata apenas
      // documentos novos. O evento não sobrescreve conteúdo de documento vivo.
      applyWorkspaceSnapshot(fileEvent.workspace);
    }));

    unsubs.push(EventsOn('workspace:tab_activated', (bws: unknown) => {
      applyWorkspaceSnapshot(bws);
    }));

    // Content rename events → update matching tab title
    unsubs.push(EventsOn('conversation:renamed', (data: unknown) => {
      const ev = data as { conversationId?: string; newTitle?: string };
      if (ev.conversationId && ev.newTitle) {
        get().handleContentRenamed('chat', ev.conversationId, ev.newTitle);
      }
    }));

    unsubs.push(EventsOn('taskList:updated', (data: unknown) => {
      const ev = data as { id?: string; title?: string };
      if (ev.id && ev.title) {
        get().handleContentRenamed('tasklist', ev.id, ev.title);
      }
    }));

    if (get().isInitialized && !bootstrapInProgress) void reconcileActiveWorkspace();

    return () => {
      listenersReady = false;
      unsubs.forEach(fn => fn());
    };
  },

  createWorkspace: async (name) => {
    const bws = await CreateWorkspace(name);
    announce(i18next.t('workspace.announce.workspaceCreated', { name }));
    return bws.id;
  },

  switchWorkspace: async (workspaceId) => {
    workspaceNavigationGeneration += 1;
    activationSeqId += 1;
    pendingActivation = null;
    const bws = await SwitchWorkspace(workspaceId);
    applyWorkspaceSnapshot(bws);
    announce(i18next.t('workspace.announce.workspaceSwitched', { name: bws.name }));
  },

  renameWorkspace: async (newName) => {
    await RenameWorkspace(newName);
    set(state => ({
      workspace: state.workspace ? { ...state.workspace, name: newName } : null,
    }));
    announce(i18next.t('workspace.announce.workspaceRenamed', { name: newName }));
  },

  deleteWorkspace: async (workspaceId) => {
    await DeleteWorkspace(workspaceId);
    await get().refreshWorkspaceList();
    announce(i18next.t('workspace.announce.workspaceRemoved'));
  },

  setProfile: async (profileSlug) => {
    await SetWorkspaceProfile(profileSlug);
    set(state => ({
      workspace: state.workspace ? { ...state.workspace, profile: profileSlug } : null,
    }));
  },

  refreshWorkspaceList: async () => {
    try {
      const list = await ListWorkspaces();
      set({ workspaces: list || [] });
    } catch (error) {
      logger.error('[Workspace] Error refreshing list:', error);
    }
  },

  addTab: async (type, title, initialState?) => {
    const tabId = generateTabId();
    const ws = get().workspace;
    const position = ws ? ws.tabs.length : 0;

    // Extract conversationId from initialState if present
    let conversationId: string | undefined;
    let state: Record<string, unknown> | undefined;
    if (initialState) {
      const { conversationId: cid, ...rest } = initialState;
      conversationId = cid as string | undefined;
      state = Object.keys(rest).length > 0 ? rest : undefined;
    }

    const tab: WorkspaceTab = {
      id: tabId,
      type,
      title,
      position,
      conversationId,
      state,
    };

    const backendTab = frontendTabToBackend(tab);
    const updatedWs = await AddWorkspaceTab(backendTab);
    if (updatedWs) {
      applyWorkspaceSnapshot(updatedWs);
    }
    announce(i18next.t('workspace.announce.tabCreated', { title }));
    return tabId;
  },

  removeTab: async (tabId) => {
    const ws = get().workspace;
    if (!ws) return;

    // Não permite fechar a última aba — cria nova antes
    if (ws.tabs.length <= 1) {
      const newTabId = generateTabId();
      const newTab = frontendTabToBackend({
        id: newTabId,
        type: 'chat',
        title: i18next.t('chat.newConversation'),
        position: 0,
      });
      await AddWorkspaceTab(newTab);
    }

    const updatedWs = await RemoveWorkspaceTab(tabId);
    if (updatedWs) {
      applyWorkspaceSnapshot(updatedWs);
    }
    announce(i18next.t('workspace.announce.tabClosed'));
  },

  reconcileActiveSelection: () => reconcileActiveWorkspace({
    generation: snapshotGeneration,
    navigationGeneration: workspaceNavigationGeneration,
    workspaceId: get().workspace?.id ?? null,
  }),

  setActiveTab: (tabId) => {
    const currentWorkspace = get().workspace;
    if (!currentWorkspace || !currentWorkspace.tabs.some(tab => tab.id === tabId)) {
      return;
    }
    if (currentWorkspace.activeTabId === tabId) {
      return;
    }
    if (isModalOpen()) {
      announce(i18next.t('workspace.closeDialogBeforeChangingTabs'));
      return;
    }
    // Optimistic update: atualiza UI imediatamente, persiste em background
    const currentWorkspaceId = get().workspace?.id ?? null;
    const mySeq = ++activationSeqId;
    const requestGeneration = snapshotGeneration;
    const requestNavigationGeneration = workspaceNavigationGeneration;
    const requestEpoch = snapshotAnchor?.epoch;
    pendingActivation = { workspaceId: currentWorkspaceId ?? '', tabId, requestId: mySeq, settled: false };
    set(state => ({
      workspace: state.workspace
        ? { ...state.workspace, activeTabId: tabId }
        : null,
    }));
    // Fire-and-forget: UI já atualizada; rollback em caso de falha.
    // A persistência no backend é assíncrona e não bloqueia callers.
    const persistActivation = async (markFailed: () => void) => {
      if (requestGeneration !== snapshotGeneration
        || requestNavigationGeneration !== workspaceNavigationGeneration
        || currentWorkspaceId === null
        || get().workspace?.id !== currentWorkspaceId
        || requestEpoch !== snapshotAnchor?.epoch) {
        return;
      }
      try {
        const snapshot = await setActiveWorkspaceTabForWorkspace(currentWorkspaceId, tabId);
        if (requestGeneration !== snapshotGeneration
          || requestNavigationGeneration !== workspaceNavigationGeneration
          || get().workspace?.id !== currentWorkspaceId
          || requestEpoch !== snapshotAnchor?.epoch) {
          return;
        }
        // A controller emits the same stamped snapshot as the RPC result. It
        // is valid for the event to win the race and make apply return false
        // because this response is an already-applied duplicate.
        const parsedResponse = parseWorkspaceSnapshot(snapshot);
        const responseEpochMatchesRequest = parsedResponse !== null
          && requestEpoch !== undefined
          && parsedResponse.epoch === requestEpoch
          && parsedResponse.epoch === snapshotAnchor?.epoch;
        if (!parsedResponse
          || !responseEpochMatchesRequest
          || parsedResponse.payload.id !== currentWorkspaceId
          || parsedResponse.payload.tabs?.active !== tabId) {
          if (activationSeqId !== mySeq) return;
          // An invalid acknowledgement is not persistence success. Reuse the
          // guarded reconciliation/rollback path even if the follow-up read fails.
          throw new Error('Invalid workspace selection acknowledgement');
        }
        applyWorkspaceSnapshot(snapshot);
        const canonicalAnchor = snapshotAnchor;
        const canonicalWorkspace = projectSnapshotAnchor(canonicalAnchor);
        if (!canonicalWorkspace
          || canonicalWorkspace.id !== currentWorkspaceId
          || canonicalWorkspace.activeTabId !== tabId) {
          if (activationSeqId !== mySeq) return;
          markFailed();
          if (pendingActivation?.requestId === mySeq) {
            pendingActivation = null;
            set({ workspace: canonicalWorkspace });
          }
          return;
        }
        if (pendingActivation?.requestId === mySeq) {
          pendingActivation.settled = true;
          pendingActivation = null;
          set({ workspace: canonicalWorkspace });
        }
      } catch (err: unknown) {
        logger.warn('[workspaceStore] SetActiveWorkspaceTab failed:', err);
        if (requestGeneration !== snapshotGeneration
          || requestNavigationGeneration !== workspaceNavigationGeneration
          || get().workspace?.id !== currentWorkspaceId
          || requestEpoch !== snapshotAnchor?.epoch) {
          return;
        }
        markFailed();
        const isLatestIntent = activationSeqId === mySeq;
        if (isLatestIntent && pendingActivation?.requestId === mySeq) {
          pendingActivation = null;
        }
        await reconcileActiveWorkspace({
          generation: requestGeneration,
          navigationGeneration: requestNavigationGeneration,
          workspaceId: currentWorkspaceId,
        });
        const stillLatestIntent = activationSeqId === mySeq && pendingActivation === null;
        if (stillLatestIntent
          && requestGeneration === snapshotGeneration
          && requestNavigationGeneration === workspaceNavigationGeneration
          && get().workspace?.id === currentWorkspaceId
          && requestEpoch === snapshotAnchor?.epoch) {
          // Reconcile may fail, return no workspace, or return a duplicate/
          // older snapshot. In all of those cases, remove the optimistic
          // overlay from the newest trusted anchor. Recheck the intent after
          // the await so a newer local selection is never overwritten.
          const canonicalWorkspace = projectSnapshotAnchor(snapshotAnchor);
          if (canonicalWorkspace?.id === currentWorkspaceId) {
            set({ workspace: canonicalWorkspace });
          }
          const rollbackTabId = get().workspace?.activeTabId ?? null;
          if (rollbackTabId !== tabId) {
            window.dispatchEvent(new CustomEvent('workspace:tab-activation-rollback', {
              detail: { failedTabId: tabId, rollbackTabId },
            }));
            announce(i18next.t('workspace.tabSwitchFailed'));
          }
        }
      }
    };
    // The UI remains optimistic while only the in-flight request and the
    // latest not-yet-submitted intent are retained.
    enqueueActivation(persistActivation);
  },

  updateTab: async (tabId, updates) => {
    await UpdateWorkspaceTab(tabId, updates);
    set(state => {
      if (!state.workspace) return state;
      return {
        workspace: {
          ...state.workspace,
          tabs: state.workspace.tabs.map(t =>
            t.id === tabId
              ? {
                  ...t,
                  ...(updates.title !== undefined ? { title: updates.title as string } : {}),
                  ...(updates.conversation_id !== undefined ? { conversationId: updates.conversation_id as string } : {}),
                  ...(updates.state !== undefined ? { state: mergeTabState(t.state, updates.state as Record<string, unknown>) } : {}),
                  ...(updates.profile_override !== undefined
                    ? {
                        profileOverride: mergeProfileOverride(
                          t.profileOverride,
                          updates.profile_override as Record<string, unknown> | null,
                        ),
                      }
                    : {}),
                }
              : t
          ),
        },
      };
    });
  },

  reorderTabs: async (orderedIds) => {
    await ReorderWorkspaceTabs(orderedIds);
    set(state => {
      if (!state.workspace) return state;
      const tabMap = new Map(state.workspace.tabs.map(t => [t.id, t]));
      const reordered = orderedIds
        .map((id, i) => {
          const tab = tabMap.get(id);
          return tab ? { ...tab, position: i } : null;
        })
        .filter((t): t is WorkspaceTab => t !== null);
      return {
        workspace: { ...state.workspace, tabs: reordered },
      };
    });
  },

  moveTabToWorkspace: async (tabId, targetWorkspaceId) => {
    const updatedWs = await MoveWorkspaceTabTo(tabId, targetWorkspaceId);
    if (updatedWs) {
      applyWorkspaceSnapshot(updatedWs);
    }
    await get().refreshWorkspaceList();
  },

  exportWorkspace: async () => {
    const yaml = await ExportWorkspace();
    return yaml;
  },

  importWorkspace: async (yamlData) => {
    const bws = await ImportWorkspace(yamlData);
    await get().refreshWorkspaceList();
    announce(i18next.t('workspace.announce.workspaceImported', { name: bws.name }));
    return bws.id;
  },

  handleContentRenamed: (type, contentId, newTitle) => {
    const ws = get().workspace;
    if (!ws) return;
    for (const tab of ws.tabs) {
      if (tab.type !== type || tab.title === newTitle) continue;
      let matches = false;
      if (type === 'chat') {
        matches = tab.conversationId === contentId;
      } else if (type === 'editor') {
        matches = tab.id === contentId;
      } else if (type === 'tasklist') {
        matches = tab.state?.tasklistId === contentId;
      } else if (type === 'terminal') {
        matches = tab.state?.sessionId === contentId;
      }
      if (matches) {
        void get().updateTab(tab.id, { title: newTitle });
      }
    }
  },

  renameTabContent: (tabId, newTitle) => {
    const tab = get().workspace?.tabs.find(t => t.id === tabId);
    if (!tab) return;
    let ref: string | undefined;
    if (tab.type === 'chat' && tab.conversationId) {
      ref = tab.conversationId;
    } else if (tab.type === 'editor') {
      ref = tab.id;
    } else if (tab.type === 'tasklist') {
      ref = tab.state?.tasklistId as string | undefined;
    } else if (tab.type === 'terminal') {
      ref = tab.state?.sessionId as string | undefined;
    }
    if (!ref) return;
    const handler = tabRenameHandlers.get(tab.type);
    if (handler) handler(ref, newTitle);
  },

  getActiveTab: () => {
    const ws = get().workspace;
    if (!ws || !ws.activeTabId) return undefined;
    return ws.tabs.find(t => t.id === ws.activeTabId);
  },

  getTabsByType: (type) => {
    const ws = get().workspace;
    if (!ws) return [];
    return ws.tabs.filter(t => t.type === type);
  },
  });
});

let authIdentity = (() => {
  const auth = useAuthStore.getState();
  return auth.isAuthenticated && auth.user
    ? `${auth.user.userId}:${auth.user.sessionId}`
    : auth.isAuthenticated ? 'authenticated-without-user' : null;
})();
const unsubscribeAuthStore = useAuthStore.subscribe((auth) => {
  const nextIdentity = auth.isAuthenticated && auth.user
    ? `${auth.user.userId}:${auth.user.sessionId}`
    : auth.isAuthenticated ? 'authenticated-without-user' : null;
  if (nextIdentity === authIdentity) return;
  authIdentity = nextIdentity;
  clearInitializeRetryTimer();
  initializingPromise = null;
  resetSnapshotOrdering();
  useWorkspaceStore.setState({ workspace: null, isInitialized: false });
});

export async function flushWorkspaceNavigation(): Promise<boolean> {
  const generationAtStart = snapshotGeneration;
  const navigationGenerationAtStart = workspaceNavigationGeneration;
  const workspaceIDAtStart = useWorkspaceStore.getState().workspace?.id ?? null;
  const activeTabAtStart = useWorkspaceStore.getState().workspace?.activeTabId ?? null;
  const wasBusy = activationBusy;
  const drainAtStart = activationDrainPromise;
  let result = await drainAtStart;
  if (!wasBusy && !result && !activationBusy) {
    // A failed selection cancels the dependent action, not every future action.
    // A fresh attempt must first confirm the actual backend selection; merely
    // clearing the error could target a tab saved before a lost acknowledgement.
    result = await useWorkspaceStore.getState().reconcileActiveSelection();
    if (result && !activationBusy && activationDrainPromise === drainAtStart) {
      activationDrainPromise = Promise.resolve(true);
    }
  }
  const currentWorkspaceID = useWorkspaceStore.getState().workspace?.id ?? null;
  const canonicalWorkspace = projectSnapshotAnchor(snapshotAnchor);
  const currentWorkspace = useWorkspaceStore.getState().workspace;
  return result
    && generationAtStart === snapshotGeneration
    && navigationGenerationAtStart === workspaceNavigationGeneration
    && workspaceIDAtStart === currentWorkspaceID
    && activeTabAtStart === (currentWorkspace?.activeTabId ?? null)
    && pendingActivation === null
    && (canonicalWorkspace === null
      || (canonicalWorkspace.id === currentWorkspace?.id
        && canonicalWorkspace.activeTabId === currentWorkspace.activeTabId));
}

/**
 * Lista vazia compartilhada para quando ainda não há workspace carregado.
 *
 * O zustand 5 usa o seletor como `getSnapshot` do `useSyncExternalStore`, sem
 * memoizar o resultado. Um `[]` literal dentro do seletor seria um valor novo a
 * cada chamada, o React concluiria que o snapshot mudou em todo commit e
 * reagendaria render até estourar em "Maximum update depth exceeded".
 */
const NO_TABS: readonly WorkspaceTab[] = Object.freeze([]);

/** Abas do workspace ativo; lista estável enquanto o workspace não carregou. */
export function useWorkspaceTabs(): readonly WorkspaceTab[] {
  return useWorkspaceStore((s) => s.workspace?.tabs ?? NO_TABS);
}

/**
 * Hook estável para obter a aba ativa sem causar re-render desnecessário.
 * Usa useShallow para evitar re-renders quando o conteúdo da aba não mudou.
 */
export function useActiveTab(): WorkspaceTab | undefined {
  return useWorkspaceStore(
    useShallow((s) => {
      const ws = s.workspace;
      if (!ws || !ws.activeTabId) return undefined;
      return ws.tabs.find(t => t.id === ws.activeTabId);
    })
  );
}

// HMR: reseta estado do módulo para que o workspace reinicialize após hot reload
if (import.meta.hot) {
  import.meta.hot.dispose(() => {
    unsubscribeAuthStore();
    clearInitializeRetryTimer();
    resetSnapshotOrdering();
    initializingPromise = null;
    useWorkspaceStore.setState({ isInitialized: false, workspace: null, workspaces: [] });
  });
}
