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
  SetActiveWorkspaceTab,
  UpdateWorkspaceTab,
  ReorderWorkspaceTabs,
  MoveWorkspaceTabTo,
  ExportWorkspace,
  ImportWorkspace,
} from '@wailsjs/go/app/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { workspace } from '../../wailsjs/go/models';
import i18next from 'i18next';
import { announce } from '../hooks/useAnnouncer';
import { isModalOpen } from '../components/ui/Modal';
import { waitForWailsBridge } from '../lib/waitForWailsBridge';

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

function backendTabToFrontend(bt: workspace.Tab): WorkspaceTab {
  return {
    id: bt.id,
    type: bt.type as TabType,
    conversationId: bt.conversation_id || undefined,
    title: bt.title,
    position: bt.position,
    profileOverride: bt.profile_override,
    state: bt.state,
  };
}

function backendWorkspaceToFrontend(bws: workspace.Workspace): WorkspaceData {
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

export const useWorkspaceStore = create<WorkspaceStore>()((set, get) => ({
  workspace: null,
  workspaces: [],
  isInitialized: false,

  initialize: async () => {
    if (initializingPromise) return initializingPromise;

    const run = async () => {
      try {
        clearInitializeRetryTimer();
        await waitForWailsBridge({ timeoutMs: WAILS_BRIDGE_INIT_TIMEOUT_MS });
        const [bws, list] = await Promise.all([
          GetActiveWorkspace(),
          ListWorkspaces(),
        ]);

        if (bws) {
          const ws = backendWorkspaceToFrontend(bws);

          if (ws.tabs.length === 0) {
            const tab: WorkspaceTab = {
              id: generateTabId(),
              type: 'chat',
              title: 'Nova conversa',
              position: 0,
            };
            const backendTab = frontendTabToBackend(tab);
            const updatedWs = await AddWorkspaceTab(backendTab);
            if (updatedWs) {
              set({
                workspace: backendWorkspaceToFrontend(updatedWs),
                workspaces: list || [],
                isInitialized: true,
              });
              return;
            }
          }

          set({
            workspace: ws,
            workspaces: list || [],
            isInitialized: true,
          });
        } else {
          set({ isInitialized: true, workspaces: list || [] });
        }
      } catch (error) {
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
        set({ isInitialized: true });
      } finally {
        initializingPromise = null;
      }
    };

    initializingPromise = run();
    return initializingPromise;
  },

  setupEventListeners: () => {
    const unsubs: Array<() => void> = [];

    unsubs.push(EventsOn('workspace:switched', (bws: workspace.Workspace) => {
      set({ workspace: backendWorkspaceToFrontend(bws) });
    }));

    unsubs.push(EventsOn('workspace:renamed', (bws: workspace.Workspace) => {
      set({ workspace: backendWorkspaceToFrontend(bws) });
      get().refreshWorkspaceList();
    }));

    unsubs.push(EventsOn('workspace:created', () => {
      get().refreshWorkspaceList();
    }));

    unsubs.push(EventsOn('workspace:deleted', () => {
      get().refreshWorkspaceList();
    }));

    unsubs.push(EventsOn('workspace:tab_added', (bws: workspace.Workspace) => {
      set({ workspace: backendWorkspaceToFrontend(bws) });
    }));

    unsubs.push(EventsOn('workspace:tab_removed', (bws: workspace.Workspace) => {
      set({ workspace: backendWorkspaceToFrontend(bws) });
    }));

    unsubs.push(EventsOn('workspace:tab_activated', (tabId: string) => {
      if (get().workspace?.activeTabId === tabId) return;
      set(state => ({
        workspace: state.workspace
          ? { ...state.workspace, activeTabId: tabId }
          : null,
      }));
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

    return () => {
      unsubs.forEach(fn => fn());
    };
  },

  createWorkspace: async (name) => {
    const bws = await CreateWorkspace(name);
    announce(`Workspace criado: ${name}`);
    return bws.id;
  },

  switchWorkspace: async (workspaceId) => {
    const bws = await SwitchWorkspace(workspaceId);
    set({ workspace: backendWorkspaceToFrontend(bws) });
    announce(`Workspace: ${bws.name}`);
  },

  renameWorkspace: async (newName) => {
    await RenameWorkspace(newName);
    set(state => ({
      workspace: state.workspace ? { ...state.workspace, name: newName } : null,
    }));
    announce(`Workspace renomeado: ${newName}`);
  },

  deleteWorkspace: async (workspaceId) => {
    await DeleteWorkspace(workspaceId);
    await get().refreshWorkspaceList();
    announce('Workspace removido');
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
      set({ workspace: backendWorkspaceToFrontend(updatedWs) });
    }
    announce(`Aba criada: ${title}`);
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
        title: 'Nova conversa',
        position: 0,
      });
      await AddWorkspaceTab(newTab);
    }

    const updatedWs = await RemoveWorkspaceTab(tabId);
    if (updatedWs) {
      set({ workspace: backendWorkspaceToFrontend(updatedWs) });
    }
    announce('Aba fechada');
  },

  setActiveTab: (tabId) => {
    if (get().workspace?.activeTabId === tabId) {
      return;
    }
    if (isModalOpen()) {
      announce(i18next.t('workspace.closeDialogBeforeChangingTabs'));
      return;
    }
    // Optimistic update: atualiza UI imediatamente, persiste em background
    const previousTabId = get().workspace?.activeTabId ?? null;
    const currentWorkspaceId = get().workspace?.id ?? null;
    const mySeq = ++activationSeqId;
    set(state => ({
      workspace: state.workspace
        ? { ...state.workspace, activeTabId: tabId }
        : null,
    }));
    // Fire-and-forget: UI já atualizada; rollback em caso de falha.
    // A persistência no backend é assíncrona e não bloqueia callers.
    void SetActiveWorkspaceTab(tabId).catch((err: unknown) => {
      logger.warn('[workspaceStore] SetActiveWorkspaceTab failed:', err);
      // Só faz rollback se nenhuma ativação mais recente ocorreu desde esta
      // e o workspace não mudou (evita alterar activeTabId de outro workspace).
      if (activationSeqId === mySeq && get().workspace?.id === currentWorkspaceId) {
        const tabs = get().workspace?.tabs ?? [];
        const rollbackId = tabs.some(t => t.id === previousTabId)
          ? previousTabId
          : (tabs[0]?.id ?? null);
        set(state => ({
          workspace: state.workspace
            ? { ...state.workspace, activeTabId: rollbackId }
            : null,
        }));
        announce(i18next.t('workspace.tabSwitchFailed'));
      }
    });
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
                  ...(updates.profile_override !== undefined ? { profileOverride: updates.profile_override as Record<string, unknown> } : {}),
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
      set({ workspace: backendWorkspaceToFrontend(updatedWs) });
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
    announce(`Workspace importado: ${bws.name}`);
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
}));

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
    initializingPromise = null;
    useWorkspaceStore.setState({ isInitialized: false, workspace: null, workspaces: [] });
  });
}
