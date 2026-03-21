import { create } from 'zustand';
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
} from '@wailsjs/go/main/App';
import { EventsOn } from '@wailsjs/runtime/runtime';
import { workspace } from '../../wailsjs/go/models';
import { announce } from '../hooks/useAnnouncer';

export type TabType = 'chat' | 'editor' | 'terminal' | 'tasklist';

export interface WorkspaceTab {
  id: string;
  type: TabType;
  contentId: string;
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
    contentId: bt.content_id,
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
    content_id: tab.contentId,
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
  addTab: (type: TabType, contentId: string, title: string) => Promise<string>;
  removeTab: (tabId: string) => Promise<void>;
  setActiveTab: (tabId: string) => Promise<void>;
  updateTab: (tabId: string, updates: Record<string, unknown>) => Promise<void>;
  reorderTabs: (orderedIds: string[]) => Promise<void>;
  moveTabToWorkspace: (tabId: string, targetWorkspaceId: string) => Promise<void>;

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

export const useWorkspaceStore = create<WorkspaceStore>()((set, get) => ({
  workspace: null,
  workspaces: [],
  isInitialized: false,

  initialize: async () => {
    try {
      const [bws, list] = await Promise.all([
        GetActiveWorkspace(),
        ListWorkspaces(),
      ]);

      if (bws) {
        const ws = backendWorkspaceToFrontend(bws);

        // Se o workspace não tem tabs, cria uma aba de chat padrão
        if (ws.tabs.length === 0) {
          const tab: WorkspaceTab = {
            id: generateTabId(),
            type: 'chat',
            contentId: '',
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
      console.error('[Workspace] Error initializing:', error);
      set({ isInitialized: true });
    }
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
      console.error('[Workspace] Error refreshing list:', error);
    }
  },

  addTab: async (type, contentId, title) => {
    const tabId = generateTabId();
    const ws = get().workspace;
    const position = ws ? ws.tabs.length : 0;

    const tab: WorkspaceTab = {
      id: tabId,
      type,
      contentId,
      title,
      position,
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
        contentId: '',
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

  setActiveTab: async (tabId) => {
    await SetActiveWorkspaceTab(tabId);
    set(state => ({
      workspace: state.workspace
        ? { ...state.workspace, activeTabId: tabId }
        : null,
    }));

    const tab = get().workspace?.tabs.find(t => t.id === tabId);
    if (tab) {
      const idx = get().workspace?.tabs.findIndex(t => t.id === tabId) ?? 0;
      const total = get().workspace?.tabs.length ?? 0;
      announce(`${tab.title}, aba ${idx + 1} de ${total}`);
    }
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
                  ...(updates.content_id !== undefined ? { contentId: updates.content_id as string } : {}),
                  ...(updates.state !== undefined ? { state: updates.state as Record<string, unknown> } : {}),
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
